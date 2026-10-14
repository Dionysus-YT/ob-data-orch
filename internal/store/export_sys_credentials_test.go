package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const expertTestConfig = `{"configVersion":"v6","database":"synthetic_db","scopeKind":"SPECIFIED","contentKind":"DDL_AND_DATA","table":"synthetic_table","format":"CSV","filePath":"/E:/tmp/output","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/output"},"filterConfig":{"weakRead":true,"enableHiddenPk":true},"ddlBehavior":{"addExtraMessage":true}}}`

func expertPrecheckFixture(t *testing.T) (*Store, PrecheckLeaseGrant, PrecheckSecretResolutionRequest) {
	t.Helper()
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	if _, err := s.db.Exec(`INSERT INTO sys_credential_revisions VALUES ('sys-1', 1, 'source-1', 'key-1', X'0102', X'0304', '{}', 'ACTIVE', ?, NULL)`, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE data_sources SET sys_user='synthetic_sys', sys_credential_id='sys-1', sys_credential_revision=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE export_drafts SET config_version='v6', config_json=? WHERE draft_id='draft-1'`, expertTestConfig); err != nil {
		t.Fatal(err)
	}
	run := PrecheckRun{PrecheckID: "expert-precheck", DraftID: "draft-1", DraftRevision: 1, ConfigFingerprint: testFingerprint, DataSourceID: "source-1", CredentialID: "credential-1", CredentialRevision: 1, NodeID: "node-1", SysCredentialID: "sys-1", SysCredentialRevision: 1, CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute)}
	if _, err := s.CreatePrecheck(context.Background(), PrecheckCreate{PrecheckRun: run, CreatorSubjectID: "subject-1", RequestID: "expert-create", IdempotencyKey: "expert-create-key", RequestDigest: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	grant, err := s.ClaimPrecheck(context.Background(), PrecheckClaim{AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "expert-lease", RequestID: "expert-claim", RequestDigest: strings.Repeat("b", 64), LeaseTTL: 2 * time.Minute, Now: testTime.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if !grant.ExecutionContext.WeakRead || !grant.ExecutionContext.EnableHiddenPk || !grant.ExecutionContext.AddExtraMessage || grant.Binding.SysCredentialID != "sys-1" {
		t.Fatal("参数或 sys 引用未冻结")
	}
	if _, err := s.AcknowledgePrecheck(context.Background(), PrecheckAcknowledgement{AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "expert-ack", RequestDigest: strings.Repeat("c", 64), Now: testTime.Add(time.Minute + time.Second)}); err != nil {
		t.Fatal(err)
	}
	return s, grant, PrecheckSecretResolutionRequest{AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "expert-secret", RequestDigest: strings.Repeat("d", 64), Now: testTime.Add(time.Minute + 2*time.Second)}
}

func TestExpertPrecheckFreezesSysAndRejectsRotationBeforeResponse(t *testing.T) {
	s, _, input := expertPrecheckFixture(t)
	connection, err := s.ResolvePrecheckDatabaseConnection(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Destroy()
	if connection.SysCredential == nil || string(connection.SysCredential.Username) != "synthetic_sys@sys#synthetic-cluster" {
		t.Fatal("未解析冻结 sys 身份")
	}
	if _, err := s.db.Exec(`UPDATE sys_credential_revisions SET status='REVOKED' WHERE credential_id='sys-1'`); err != nil {
		t.Fatal(err)
	}
	err = s.FinishPrecheckSecretResolution(context.Background(), PrecheckSecretResolutionOutcome{AgentID: input.AgentID, PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest, RequestID: input.RequestID, RequestDigest: input.RequestDigest, Succeeded: true, Now: input.Now.Add(time.Second)})
	if !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("撤销后仍允许响应: %v", err)
	}
	if _, err := s.ResolvePrecheckDatabaseConnection(context.Background(), input); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatal("撤销后仍能解析旧 sys 材料")
	}
}

func TestExpertPrecheckRequiresDedicatedEvidence(t *testing.T) {
	s, grant, input := expertPrecheckFixture(t)
	completion := AgentPrecheckCompletion{AgentID: input.AgentID, PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest, RequestID: "expert-complete", RequestDigest: strings.Repeat("e", 64), Results: passedPrecheckResults(), Now: input.Now.Add(time.Second)}
	if _, err := s.CompleteAgentPrecheck(context.Background(), completion); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatal("普通对象证据替代了参数检查")
	}
	for index := range completion.Results {
		if completion.Results[index].Check == "OBJECT_ACCESS" {
			completion.Results[index].EvidenceCode = "PARAMETER_REQUIREMENTS_VERIFIED"
		}
	}
	if _, err := s.CompleteAgentPrecheck(context.Background(), completion); err != nil {
		t.Fatal(err)
	}
	snapshot, err := FreezeTaskSysCredential(expertTestConfig, grant.Binding.SysCredentialID, grant.Binding.SysCredentialRevision)
	if err != nil {
		t.Fatal(err)
	}
	reference, ok := snapshotSysCredential(snapshot)
	if !ok || reference.CredentialID != "sys-1" || reference.Revision != 1 {
		t.Fatal("任务内部引用不能恢复")
	}
	if _, ok := snapshotSysCredential(expertTestConfig); ok {
		t.Fatal("没有内部引用的 sys 任务被接受")
	}
	inputTask := validTaskSubmission("expert-task")
	inputTask.PrecheckID, inputTask.SnapshotVersion, inputTask.SnapshotJSON = grant.PrecheckID, "v2", snapshot
	inputTask.MetadataVersion, inputTask.CapabilityVersion = "obdumper-4.3.5-slice-v11-expert-parameters", "export-odp-ddl-csv-v1"
	inputTask.PlannedArgvJSON = strings.TrimSuffix(inputTask.PlannedArgvJSON, "]") + `,"--ddl","--weak-read","--enable-hidden-pk","--add-extra-message","--sys-user","synthetic_sys"]`
	if err := s.SubmitTask(context.Background(), inputTask); err != nil {
		t.Fatalf("sys 内部引用不能通过实际提交仓储: %v", err)
	}
	if _, err := s.GetTaskSummary(context.Background(), inputTask.TaskID); err != nil {
		t.Fatal(err)
	}
}

func TestSysPrecheckRejectsCurrentRevisionDrift(t *testing.T) {
	s, _, input := expertPrecheckFixture(t)
	connection, err := s.ResolvePrecheckDatabaseConnection(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Destroy()
	if _, err := s.db.Exec(`INSERT INTO sys_credential_revisions VALUES ('sys-1', 2, 'source-1', 'key-1', X'0102', X'0304', '{}', 'ACTIVE', ?, NULL)`, utcText(input.Now)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE sys_credential_revisions SET status='SUPERSEDED' WHERE credential_id='sys-1' AND revision=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE data_sources SET sys_credential_revision=2 WHERE data_source_id='source-1'`); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishPrecheckSecretResolution(context.Background(), PrecheckSecretResolutionOutcome{AgentID: input.AgentID, PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest, RequestID: input.RequestID, RequestDigest: input.RequestDigest, Succeeded: true, Now: input.Now.Add(time.Second)}); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatal("修订变化仍能返回旧材料")
	}
}

func TestSysPrecheckMigrationRejectsHalfReference(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	for _, query := range []string{`UPDATE precheck_runs SET sys_credential_id='sys-1'`, `UPDATE precheck_runs SET sys_credential_revision=1`} {
		if _, err := s.db.Exec(query); err == nil {
			t.Fatal("SQLite 接受了单侧 sys 引用")
		}
	}
}

func TestSysPasswordCannotEnterStoredArgv(t *testing.T) {
	for _, argv := range []string{`["--sys-password","synthetic_password"]`, `["--sys-password=synthetic_password"]`} {
		if validateArgvJSON(argv) == nil {
			t.Fatal("sys 密码能进入冻结 argv")
		}
	}
}
