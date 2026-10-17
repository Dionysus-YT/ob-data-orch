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

	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

func Test普通导入显式领取租约秘密与独立结果闭环(t *testing.T) {
	s, w, c := normalImportFixture(t)
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
	draft, err := s.SaveNormalImportDraft(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	create := PrecheckCreate{CheckKind: "NORMAL_IMPORT_PREFLIGHT", PrecheckRun: PrecheckRun{PrecheckID: "normal-run", DraftID: draft.ID, DraftRevision: 1, ConfigFingerprint: draft.ConfigFingerprint, DataSourceID: c.DataSourceID, CredentialID: "credential-1", CredentialRevision: 1, NodeID: c.NodeID, CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute)}, CreatorSubjectID: "subject-1", RequestID: "check-create", IdempotencyKey: "normal-check-key-0001", RequestDigest: strings.Repeat("b", 64)}
	if _, err = s.CreateNormalImportPrecheck(ctx, create); err != nil {
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
	completion := AgentPrecheckCompletion{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: pre.LeaseID, LeaseEpoch: pre.LeaseEpoch, BindingDigest: pre.Binding.BindingDigest, RequestID: "check-complete", RequestDigest: strings.Repeat("e", 64), InputManifest: &manifest, Now: testTime.Add(time.Second)}
	for _, check := range precheckcontract.NormalImportChecks(create.CheckKind) {
		completion.Results = append(completion.Results, PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: "SYNTHETIC_OK"})
	}
	if _, err = s.CompleteAgentPrecheck(ctx, completion); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(NormalImportSnapshot{ConfigVersion: importdomain.ConfigVersion, TaskType: importdomain.TaskType, Config: c, InputManifest: manifest})
	argv := []string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic_user", "--database", c.Database, "--no-sys", "--file-path", c.FilePath, "--csv", "--all"}
	args, _ := json.Marshal(argv)
	submission := validTaskSubmission("normal-task")
	submission.TaskType = importdomain.TaskType
	submission.PrecheckID = create.PrecheckID
	submission.MetadataVersion = importdomain.MetadataVersion
	submission.CapabilityVersion = importdomain.CapabilityVersion
	submission.SnapshotVersion = importdomain.ConfigVersion
	submission.SnapshotJSON = string(snapshot)
	submission.PlannedArgvJSON = string(args)
	if err = s.SubmitTask(ctx, submission); err != nil {
		t.Fatal(err)
	}
	claim := budgetClaim("normal")
	claim.Now = testTime.Add(3 * time.Second)
	if _, found, err := s.ClaimNextExecution(ctx, claim); err != nil || found {
		t.Fatal("默认导出领取了普通导入", err)
	}
	claim.TaskType = importdomain.TaskType
	grant, found, err := s.ClaimNextExecution(ctx, claim)
	if err != nil || !found || grant.NormalImport == nil || grant.TaskType != importdomain.TaskType {
		t.Fatal("显式领取失败", err)
	}
	if replay, found, err := s.ClaimNextExecution(ctx, claim); err != nil || !found || replay.ExecutionID != grant.ExecutionID {
		t.Fatal("重复领取改变执行", err)
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
	control, err := s.PollExecutionControl(ctx, ExecutionControlPoll{AgentID: "agent-1", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: 1, EnvelopeDigest: grant.EnvelopeDigest, Now: claim.Now})
	if err != nil || control.CancelRequested {
		t.Fatal("导入租约摘要不匹配", err)
	}
	secret, err := s.ResolveExecutionDatabaseConnection(ctx, ExecutionSecretResolutionRequest{AgentID: "agent-1", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: 1, EnvelopeDigest: grant.EnvelopeDigest, RequestID: "normal-secret", RequestDigest: strings.Repeat("f", 64), Now: claim.Now})
	if err != nil {
		t.Fatal("普通导入不能解析业务槽位", err)
	}
	secret.Destroy()
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
		t.Fatal("导入接受导出文件结果", err)
	}
	if err = appendEvent("RESULT_FACTS_OBSERVED", map[string]any{"result": "VERIFIED", "normalImport": true, "checkpointPresent": false}); err != nil {
		t.Fatal(err)
	}
	summary, err := s.GetTaskSummary(ctx, submission.TaskID)
	if err != nil || summary.State != "SUCCEEDED" || summary.ResultSummary != nil {
		t.Fatal("结果投影伪造导出计数", err, summary.State)
	}
}
