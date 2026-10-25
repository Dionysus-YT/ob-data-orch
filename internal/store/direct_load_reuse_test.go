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
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

// directLoadReuseFixture 仅构造独立冻结证明与终态，不启动工具或取得真实数据库事实。
func directLoadReuseFixture(t *testing.T, state string) (*Store, TaskDerivationSource) {
	t.Helper()
	s, _, c := directLoadFixture(t)
	files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: c.Table, Size: 2, Modified: 1, SHA256: strings.Repeat("a", 64)}}
	raw, _ := json.Marshal(files)
	digest := sha256.Sum256(raw)
	proof := DirectLoadPrecheckProof{ID: "precheck-1", DraftRevision: 1, ConfigFingerprint: testFingerprint}
	for _, check := range precheckcontract.DirectLoadChecks("DIRECT_LOAD_PREFLIGHT") {
		proof.Results = append(proof.Results, PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: "DIRECT_LOAD_VERIFIED"})
	}
	snapshot, _ := json.Marshal(DirectLoadSnapshot{ConfigVersion: directload.ConfigVersion, TaskType: directload.TaskType, TaskName: "合成来源", Config: c, InputManifest: importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(digest[:])}, PrecheckProof: proof, DirectLoadConfigFingerprint: strings.Repeat("9", 64)})
	_, err := s.db.Exec(`INSERT INTO tasks(task_id,creator_subject_id,data_source_id,node_id,precheck_id,credential_id,credential_revision,config_fingerprint,tool_version,metadata_version,capability_version,snapshot_version,snapshot_json,planned_argv_json,planned_command_redacted,submitted_at,task_type) VALUES('direct-reuse-task','subject-1','source-1','node-1','precheck-1','credential-1',1,?,?,?,?,?,?,'[]','obloader -p ******',?,'OBLOADER_DIRECT_LOAD')`, testFingerprint, directload.ToolVersion, directload.MetadataVersion, directload.CapabilityVersion, directload.ConfigVersion, string(snapshot), utcText(testTime))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO task_executions(execution_id,task_id,node_id,agent_id,state,revision,created_at,updated_at) VALUES('direct-reuse-execution','direct-reuse-task','node-1','agent-1',?,1,?,?)`, state, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	source, err := s.GetAuthorizedTaskDerivationSource(context.Background(), "direct-reuse-task", "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	return s, source
}

func Test旁路派生来源幂等锁定与当前事实重绑(t *testing.T) {
	s, source := directLoadReuseFixture(t, "FAILED")
	ctx := context.Background()
	c, err := DirectLoadReusableConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(c)
	w := DirectLoadDraftWrite{DirectLoadDraft: DirectLoadDraft{ID: "derived-direct", OwnerSubjectID: "subject-1", DataSourceID: c.DataSourceID, NodeID: c.NodeID, ConfigJSON: string(raw), ConfigFingerprint: testFingerprint, SourceTaskID: source.TaskID, SourceDerivation: "RERUN_FROM_SCRATCH", UpdatedAt: testTime.Add(time.Minute)}, RequestID: "derive-direct", IdempotencyKey: "synthetic-derived-direct", RequestDigest: strings.Repeat("d", 64)}
	draft, err := s.SaveDirectLoadDraft(ctx, w)
	if err != nil || draft.SourceTaskID != source.TaskID || draft.SourceDerivation != w.SourceDerivation || !draft.ParametersLocked {
		t.Fatal("来源锁定缺失", err)
	}
	w.ID = "replay-direct"
	if replay, err := s.SaveDirectLoadDraft(ctx, w); err != nil || replay.ID != draft.ID || !replay.Replayed {
		t.Fatal("派生未重放", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='TASK_DRAFT_DERIVED'`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM precheck_runs WHERE draft_id='derived-direct'`, 0)
	w.ID, w.ExpectedRevision, w.RequestID, w.ConfigFingerprint = draft.ID, 1, "rebind-direct", strings.Repeat("b", 64)
	if rebound, err := s.SaveDirectLoadDraft(ctx, w); err != nil || rebound.Revision != 2 || rebound.ConfigFingerprint != w.ConfigFingerprint || !rebound.ParametersLocked {
		t.Fatal("同配置不能重绑当前事实", err)
	}
	w.ExpectedRevision = 2
	c.Thread = "2"
	raw, _ = json.Marshal(c)
	w.ConfigJSON = string(raw)
	if _, err := s.SaveDirectLoadDraft(ctx, w); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("从头执行可改配置", err)
	}
	if _, err := s.GetDirectLoadDraft(ctx, draft.ID, "foreign"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("来源草稿跨主体可见", err)
	}
	if _, err := s.db.Exec(`UPDATE task_executions SET reconciliation_required=1 WHERE execution_id='direct-reuse-execution'`); err != nil {
		t.Fatal(err)
	}
	c.Thread = ""
	raw, _ = json.Marshal(c)
	w.ID, w.ExpectedRevision, w.ConfigJSON, w.IdempotencyKey = "untrusted-direct", 0, string(raw), "synthetic-untrusted-direct"
	if _, err := s.SaveDirectLoadDraft(ctx, w); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("核对中来源仍能派生", err)
	}
}

func Test旁路快照严格序列化与证明身份(t *testing.T) {
	_, source := directLoadReuseFixture(t, "SUCCEEDED")
	if _, ok := directLoadSnapshot(source.SnapshotJSON); !ok {
		t.Fatal("合成固定证明无效")
	}
	for _, raw := range []string{
		strings.Replace(source.SnapshotJSON, `"taskType":`, `"taskType":"OBLOADER_DIRECT_LOAD","taskType":`, 1),
		strings.Replace(source.SnapshotJSON, `"taskType":`, `"TaskType":`, 1),
		strings.Replace(source.SnapshotJSON, `"rpcPort":`, `"RPCPort":`, 1),
		strings.Replace(source.SnapshotJSON, `"rpcPort":`, `"rpcPort":2885,"rpcPort":`, 1),
		strings.Replace(source.SnapshotJSON, `"singleTable":true`, `"singleTable":null`, 1),
		strings.Replace(source.SnapshotJSON, `"files":`, `"Files":`, 1),
		strings.Replace(source.SnapshotJSON, `"status":`, `"Status":`, 1),
		strings.Replace(source.SnapshotJSON, `"directLoadConfigFingerprint":`, `"directLoadConfigFingerprint":null,"directLoadConfigFingerprint":`, 1),
	} {
		if _, ok := directLoadSnapshot(raw); ok {
			t.Fatal("接受重复、别名或空值快照")
		}
	}
	for _, mutate := range []func(*TaskDerivationSource){
		func(s *TaskDerivationSource) { s.TaskType = "OBLOADER_NORMAL_IMPORT" },
		func(s *TaskDerivationSource) { s.MetadataVersion = "unknown" },
		func(s *TaskDerivationSource) { s.PrecheckID = "different-proof" },
		func(s *TaskDerivationSource) { s.ConfigFingerprint = strings.Repeat("b", 64) },
	} {
		bad := source
		mutate(&bad)
		if _, err := DirectLoadReusableConfig(bad); err == nil {
			t.Fatal("来源类型、版本或证明身份未校验")
		}
	}
}
