package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
	"strings"
	"testing"
	"time"
)

func Test旁路同一预检只冻结一个任务且保留结果核对(t *testing.T) {
	s, w, c := directLoadFixture(t)
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

	first, err := s.SubmitTaskIdempotent(ctx, submission, "synthetic-first-submit-key", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	submission.TaskID = "synthetic-second-task"
	if _, err = s.SubmitTaskIdempotent(ctx, submission, "synthetic-retry-submit-key", strings.Repeat("a", 64)); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("换键重复创建未拒绝", err)
	}
	if err = s.SubmitTask(ctx, submission); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("非幂等入口绕过单次证明", err)
	}
	if _, err = s.db.Exec("UPDATE precheck_runs SET valid_until=? WHERE precheck_id=?", utcText(testTime.Add(-time.Minute)), create.PrecheckID); err != nil {
		t.Fatal(err)
	}
	replay, err := s.SubmitTaskIdempotent(ctx, submission, "synthetic-first-submit-key", strings.Repeat("a", 64))
	if err != nil || !replay.Replayed || replay.TaskID != first.TaskID {
		t.Fatal("原键未找回过期证明的结果", err)
	}
	if _, err = s.SubmitTaskIdempotent(ctx, submission, "synthetic-first-submit-key", strings.Repeat("b", 64)); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("原键异内容未拒绝", err)
	}
	check, err := s.GetDirectLoadCheck(ctx, create.PrecheckID, "subject-1")
	if err != nil || check.SubmittedTaskID != first.TaskID || check.SubmittedTaskName != "合成旁路任务" {
		t.Fatal("原任务核对缺失", err)
	}
	if _, err = s.GetDirectLoadCheck(ctx, create.PrecheckID, "other-subject"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("核对泄漏他人任务", err)
	}
	assertCount(t, s.db, "SELECT COUNT(*) FROM tasks WHERE precheck_id='normal-run'", 1)
}
