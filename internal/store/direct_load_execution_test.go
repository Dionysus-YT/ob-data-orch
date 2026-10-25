package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

func Test旁路显式能力租约凭据与提交结果闭环(t *testing.T) {
	for _, committed := range []bool{true, false} {
		t.Run(map[bool]string{true: "服务端提交已观察", false: "缺少提交需核对"}[committed], func(t *testing.T) {
			s, w, c := directLoadFixture(t)
			setTestSchedulingLimit(t, s, 1)
			ctx := context.Background()
			node, err := s.GetExecutionNode(ctx, c.NodeID)
			if err != nil {
				t.Fatal(err)
			}
			facts, err := encodeAgentEnvironmentFacts(AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: testTime, MemoryTotalBytes: 16 * executionbudget.GiB, MemoryAvailableBytes: 16 * executionbudget.GiB, RuntimeConfigurationDigest: ExecutionNodeRuntimeConfigurationDigest(node)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.db.Exec(`UPDATE agents SET facts_json=?,execution_worker_limit=2 WHERE agent_id='agent-1'`, facts); err != nil {
				t.Fatal(err)
			}
			if _, err = s.db.Exec(`UPDATE execution_nodes SET environment_check_status='PASSED',environment_check_code='TOOL_RUNTIME_READY',environment_check_facts_revision=1 WHERE node_id='node-1'`); err != nil {
				t.Fatal(err)
			}
			draft, err := s.SaveDirectLoadDraft(ctx, w)
			if err != nil {
				t.Fatal(err)
			}
			create := PrecheckCreate{CheckKind: "DIRECT_LOAD_PREFLIGHT", PrecheckRun: PrecheckRun{PrecheckID: "normal-run", DraftID: draft.ID, DraftRevision: 1, ConfigFingerprint: draft.ConfigFingerprint, DataSourceID: c.DataSourceID, CredentialID: "credential-1", CredentialRevision: 1, NodeID: c.NodeID, CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute)}, CreatorSubjectID: "subject-1", RequestID: "check-create", IdempotencyKey: "normal-check-key-0001", RequestDigest: strings.Repeat("b", 64)}
			if _, err = s.CreateDirectLoadPrecheck(ctx, create); err != nil {
				t.Fatal(err)
			}
			pre, err := s.ClaimPrecheck(ctx, PrecheckClaim{AgentID: "agent-1", NodeID: c.NodeID, PrecheckID: create.PrecheckID, LeaseID: "check-lease", RequestID: "check-claim", RequestDigest: strings.Repeat("c", 64), LeaseTTL: time.Minute, Now: testTime})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.AcknowledgePrecheck(ctx, PrecheckAcknowledgement{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: pre.LeaseID, LeaseEpoch: pre.LeaseEpoch, BindingDigest: pre.Binding.BindingDigest, RequestID: "check-ack", RequestDigest: strings.Repeat("d", 64), Now: testTime}); err != nil {
				t.Fatal(err)
			}
			files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: testTime.UnixNano(), SHA256: strings.Repeat("a", 64)}}
			raw, _ := json.Marshal(files)
			hash := sha256.Sum256(raw)
			manifest := importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(hash[:])}
			completion := AgentPrecheckCompletion{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: pre.LeaseID, LeaseEpoch: pre.LeaseEpoch, BindingDigest: pre.Binding.BindingDigest, RequestID: "check-complete", RequestDigest: strings.Repeat("e", 64), InputManifest: &manifest, DirectLoadConfigFingerprint: strings.Repeat("9", 64), Now: testTime.Add(time.Second)}
			for _, check := range precheckcontract.DirectLoadChecks(create.CheckKind) {
				completion.Results = append(completion.Results, PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: "DIRECT_LOAD_VERIFIED"})
			}
			if _, err = s.CompleteAgentPrecheck(ctx, completion); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := json.Marshal(DirectLoadSnapshot{ConfigVersion: directload.ConfigVersion, TaskType: directload.TaskType, TaskName: "合成旁路任务", Config: c, InputManifest: manifest, DirectLoadConfigFingerprint: completion.DirectLoadConfigFingerprint, PrecheckProof: DirectLoadPrecheckProof{ID: create.PrecheckID, DraftRevision: 1, ConfigFingerprint: draft.ConfigFingerprint, Results: completion.Results}})
			argv := []string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic_user", "--database", c.Database, "--tenant", "synthetic-tenant", "--cluster", "synthetic-cluster", "--file-path", c.FilePath, "--csv", "--direct", "--rpc-port", "2885", "--table", c.Table}
			args, _ := json.Marshal(argv)
			submission := validTaskSubmission("normal-task")
			submission.TaskType = directload.TaskType
			submission.PrecheckID = create.PrecheckID
			submission.MetadataVersion = directload.MetadataVersion
			submission.CapabilityVersion = directload.CapabilityVersion
			submission.SnapshotVersion = directload.ConfigVersion
			submission.SnapshotJSON = string(snapshot)
			submission.PlannedArgvJSON = string(args)
			if err = s.SubmitTask(ctx, submission); err != nil {
				t.Fatal(err)
			}
			claim := budgetClaim("normal")
			claim.Now = testTime.Add(3 * time.Second)
			if _, found, err := s.ClaimNextExecution(ctx, claim); err != nil || found {
				t.Fatal("默认导出领取了旁路导入", err)
			}
			claim.TaskType = directload.TaskType
			if _, found, err := s.ClaimNextExecution(ctx, claim); !errors.Is(err, ErrClaimIneligible) || found {
				t.Fatal("默认旁路执行门禁未关闭", err)
			}
			s.ConfigureDirectLoadCapabilities(false, true)
			if _, _, err := s.ClaimNextExecution(ctx, claim); !errors.Is(err, ErrClaimIneligible) {
				t.Fatal("探针关闭时执行仍可领取", err)
			}
			s.ConfigureDirectLoadCapabilities(true, true)
			grant, found, err := s.ClaimNextExecution(ctx, claim)
			if err != nil || !found || grant.DirectLoad == nil || grant.NormalImport != nil || grant.DirectLoad.CompatibilityMode != "MYSQL" || grant.TaskType != directload.TaskType {
				t.Fatal("显式领取失败", err)
			}
			if replay, found, err := s.ClaimNextExecution(ctx, claim); err != nil || !found || replay.ExecutionID != grant.ExecutionID {
				t.Fatal("重复领取改变执行", err)
			}
			deniedRequest := ExecutionSecretResolutionRequest{AgentID: "agent-1", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: 1, EnvelopeDigest: grant.EnvelopeDigest, RequestID: "unacknowledged-direct-secret", RequestDigest: strings.Repeat("f", 64), Now: claim.Now}
			if _, err := s.ResolveExecutionDatabaseConnection(ctx, deniedRequest); !errors.Is(err, ErrEventRejected) {
				t.Fatal("未ack租约取得旁路凭据", err)
			}
			if _, err := s.RequestTaskCancellation(ctx, TaskCancellationRequest{TaskID: submission.TaskID, SubjectID: "subject-1", RequestID: "direct-cancel", IdempotencyKey: "direct-cancel-key", RequestDigest: strings.Repeat("b", 64), ExecutionID: grant.ExecutionID, Now: claim.Now, Deadline: claim.Now.Add(time.Minute)}); !errors.Is(err, ErrEventRejected) {
				t.Fatal("旁路开放取消", err)
			}
			seq := int64(2)
			appendEvent := func(kind string, payload any) error {
				b, _ := json.Marshal(payload)
				_, err := s.AppendAuthenticatedExecutionEvent(ctx, "agent-1", ExecutionEvent{EventID: "normal-event-" + kind, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: 1, EventSeq: seq, EventType: kind, PayloadJSON: string(b), ReceivedAt: claim.Now})
				if err == nil {
					seq++
				}
				return err
			}
			if err = appendEvent("LEASE_ACKNOWLEDGED", map[string]any{}); err != nil {
				t.Fatal(err)
			}
			// 领取后原检查已过有效期不使正在运行的当前执行租约失效。
			if _, err := s.db.Exec(`UPDATE precheck_runs SET valid_until=? WHERE precheck_id=?`, utcText(claim.Now), create.PrecheckID); err != nil {
				t.Fatal(err)
			}
			if err := s.RenewExecutionLease(ctx, LeaseRenewal{ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, AgentID: "agent-1", EnvelopeDigest: grant.EnvelopeDigest, Now: claim.Now.Add(time.Second), ExpiresAt: claim.Now.Add(time.Minute)}); err != nil {
				t.Fatal("原检查过期阻断当前执行续租", err)
			}
			if _, err := s.db.Exec(`UPDATE precheck_runs SET valid_until=? WHERE precheck_id=?`, utcText(create.ValidUntil), create.PrecheckID); err != nil {
				t.Fatal(err)
			}
			control, err := s.PollExecutionControl(ctx, ExecutionControlPoll{AgentID: "agent-1", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: 1, EnvelopeDigest: grant.EnvelopeDigest, Now: claim.Now})
			if err != nil || control.CancelRequested {
				t.Fatal("导入租约摘要不匹配", err)
			}
			secret, err := s.ResolveExecutionDatabaseConnection(ctx, ExecutionSecretResolutionRequest{AgentID: "agent-1", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: 1, EnvelopeDigest: grant.EnvelopeDigest, RequestID: "normal-secret", RequestDigest: strings.Repeat("f", 64), Now: claim.Now})
			if err != nil {
				t.Fatal("旁路不能解析业务槽位", err)
			}
			if secret.TaskType != directload.TaskType || secret.SysCredential != nil {
				t.Fatal("旁路借用了其他凭据用途")
			}
			secret.Destroy()
			s.ConfigureDirectLoadCapabilities(true, false)
			if _, err := s.ResolveExecutionDatabaseConnection(ctx, ExecutionSecretResolutionRequest{AgentID: "agent-1", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: 1, EnvelopeDigest: grant.EnvelopeDigest, RequestID: "direct-secret-disabled", RequestDigest: strings.Repeat("e", 64), Now: claim.Now}); !errors.Is(err, ErrEventRejected) {
				t.Fatal("关闭旁路门禁后仍解析凭据", err)
			}
			s.ConfigureDirectLoadCapabilities(true, true)
			digest, _ := executionArgvDigest(argv)
			for _, e := range []struct {
				kind    string
				payload any
			}{{"PROCESS_STARTED", map[string]any{"pid": 1, "startedAt": utcText(claim.Now), "executableDigest": strings.Repeat("a", 64), "bootId": "boot-normal", "argvDigest": digest}}, {"PROCESS_EXITED", map[string]any{"exitCode": 0}}, {"TOOL_TERMINAL_OBSERVED", map[string]any{"terminal": "SUCCEEDED"}}} {
				if err = appendEvent(e.kind, e.payload); err != nil {
					t.Fatal(e.kind, err)
				}
			}
			if err = appendEvent("RESULT_FACTS_OBSERVED", map[string]any{"result": "VERIFIED", "fileCount": 1, "totalBytes": 2, "files": []any{}, "checkpointPresent": false}); !errors.Is(err, ErrEventRejected) {
				t.Fatal("旁路接受导出文件结果", err)
			}
			if err = appendEvent("RESULT_FACTS_OBSERVED", map[string]any{"result": "VERIFIED", "normalImport": true, "checkpointPresent": false}); !errors.Is(err, ErrEventRejected) {
				t.Fatal("旁路接受普通导入结果", err)
			}
			if err = appendEvent("RESULT_FACTS_OBSERVED", map[string]any{"result": "VERIFIED", "directLoad": true, "serverCommitObserved": false, "checkpointPresent": false}); !errors.Is(err, ErrEventRejected) {
				t.Fatal("未观察服务端提交仍成功", err)
			}
			result := map[string]any{"result": "VERIFIED", "directLoad": true, "serverCommitObserved": true, "checkpointPresent": false}
			if !committed {
				result = map[string]any{"result": "FAILED", "directLoad": true, "serverCommitObserved": false, "checkpointPresent": false, "errorCode": "RESULT_VERIFICATION_FAILED"}
			}
			if err = appendEvent("RESULT_FACTS_OBSERVED", result); err != nil {
				t.Fatal(err)
			}
			var persisted string
			if err := s.db.QueryRowContext(ctx, `SELECT result_summary_json FROM task_executions WHERE execution_id=?`, grant.ExecutionID).Scan(&persisted); err != nil {
				t.Fatal(err)
			}
			var durable map[string]any
			if json.Unmarshal([]byte(persisted), &durable) != nil || durable["serverCommitObserved"] != committed {
				t.Fatal("同表提交观察未随持久摘要保留")
			}
			if !committed && durable["errorCode"] != "RESULT_VERIFICATION_FAILED" {
				t.Fatal("失败原因未随持久摘要保留")
			}
			summary, err := s.GetTaskSummary(ctx, submission.TaskID)
			wantState := "SUCCEEDED"
			if !committed {
				wantState = "FAILED"
			}
			if err != nil || summary.State != wantState || summary.ResultSummary != nil || summary.OutputLocation != nil || summary.ReconciliationRequired == committed {
				t.Fatal("结果投影伪造导出计数", err, summary.State)
			}
		})
	}
}

func Test旁路提交事实结果矩阵不等同未观察为未提交(t *testing.T) {
	for _, observed := range []bool{false, true} {
		for _, code := range []string{ExecutionErrorResultVerification, ExecutionErrorProcessExitNonZero, ExecutionErrorProcessWaitFailed, ExecutionErrorEvidenceUnavailable} {
			raw, _ := json.Marshal(map[string]any{"result": "FAILED", "directLoad": true, "serverCommitObserved": observed, "checkpointPresent": false, "errorCode": code})
			if !validProjectedExecutionEvent(ExecutionEvent{EventType: "RESULT_FACTS_OBSERVED", PayloadJSON: string(raw)}) {
				t.Fatal("固定失败事实不能保留提交观察", code)
			}
		}
	}
}
