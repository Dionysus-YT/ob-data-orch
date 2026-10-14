package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ob-data-orch/internal/logstream"
)

func Test永久删除节点保留历史并撤销访问(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	credential := sha256.Sum256([]byte("synthetic-permanent-delete-agent"))
	if _, err := s.db.Exec(`UPDATE agents SET credential_digest = ? WHERE agent_id = 'agent-1'`, credential[:]); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimTask(ctx, validClaim("execution-1", "lease-1", "event-1", "request-claim-1")); err != nil {
		t.Fatal(err)
	}
	persistent, err := logstream.NewPersistentStore(filepath.Join(t.TempDir(), "logs"), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persistent.Close() })
	batch, err := logstream.SealBatch(logstream.Batch{StreamID: "stream-delete-1", SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, PolicyVersion: "synthetic-v1", Records: []logstream.Record{{StreamID: "stream-delete-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 1, Kind: logstream.RecordLog, Message: "safe synthetic log", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistent.Append(ctx, "execution-1", batch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE task_executions SET state = 'SUCCEEDED', finished_at = ? WHERE execution_id = 'execution-1'`, utcText(testTime.Add(3*time.Minute))); err != nil {
		t.Fatal(err)
	}
	enrollment := sha256.Sum256([]byte("synthetic-permanent-delete-enrollment"))
	if _, err := s.db.Exec(`INSERT INTO agent_enrollment_tokens(enrollment_id,node_id,token_digest,status,expires_at,created_by,created_at)
		VALUES ('enrollment-1','node-1',?,'ACTIVE',?,'subject-1',?)`, enrollment[:], utcText(testTime.Add(time.Hour)), utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	var snapshot, command, precheckResult string
	if err := s.db.QueryRow(`SELECT snapshot_json, planned_argv_json FROM tasks WHERE task_id='task-1'`).Scan(&snapshot, &command); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT result_json FROM precheck_runs WHERE precheck_id='precheck-1'`).Scan(&precheckResult); err != nil {
		t.Fatal(err)
	}
	result, err := s.DeleteExecutionNode(ctx, ExecutionNodeDeletion{NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: 1, RequestID: "delete-permanent-1", DeletedAt: testTime.Add(4 * time.Minute)})
	if err != nil || result.Outcome != "DELETED" || result.Revision != 0 || !result.AgentAccessRevoked {
		t.Fatalf("永久删除结果=%#v err=%v", result, err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM execution_nodes WHERE node_id='node-1'`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM execution_node_references WHERE node_id='node-1'`, 1)
	for _, table := range []string{"tasks", "task_executions", "execution_leases", "execution_events", "precheck_runs", "export_drafts", "agents"} {
		assertCount(t, s.db, "SELECT COUNT(*) FROM "+table, 1)
	}
	var afterSnapshot, afterCommand, afterPrecheck, precheckStatus, agentStatus, enrollmentStatus string
	if err := s.db.QueryRow(`SELECT snapshot_json, planned_argv_json FROM tasks WHERE task_id='task-1'`).Scan(&afterSnapshot, &afterCommand); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT result_json,status FROM precheck_runs WHERE precheck_id='precheck-1'`).Scan(&afterPrecheck, &precheckStatus); err != nil {
		t.Fatal(err)
	}
	if afterSnapshot != snapshot || afterCommand != command || afterPrecheck != precheckResult || precheckStatus != "SUCCEEDED" {
		t.Fatal("历史任务、命令或已完成预检查被改写")
	}
	if err := s.db.QueryRow(`SELECT status FROM agents WHERE agent_id='agent-1'`).Scan(&agentStatus); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT status FROM agent_enrollment_tokens WHERE enrollment_id='enrollment-1'`).Scan(&enrollmentStatus); err != nil {
		t.Fatal(err)
	}
	if agentStatus != "REVOKED" || enrollmentStatus != "REVOKED" {
		t.Fatal("机器身份或未使用注册码未撤销")
	}
	if _, err := s.AuthenticateAgent(ctx, credential[:]); !errors.Is(err, ErrAgentAuthenticationFailed) {
		t.Fatalf("已删除节点仍可认证：%v", err)
	}
	if summary, err := s.GetAuthorizedTaskSummary(ctx, "task-1", "subject-1"); err != nil || summary.NodeID != "node-1" {
		t.Fatalf("历史任务授权读取失败：%#v %v", summary, err)
	}
	if list, err := s.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-1", Limit: 10}); err != nil || len(list) != 1 || list[0].NodeID != "node-1" {
		t.Fatalf("删除节点后历史列表丢失：%#v %v", list, err)
	}
	if source, err := s.GetAuthorizedTaskDerivationSource(ctx, "task-1", "subject-1"); err != nil || source.SnapshotJSON != snapshot || source.PlannedArgvJSON != command {
		t.Fatalf("删除节点后快照来源丢失：%#v %v", source, err)
	}
	if _, _, err := s.LatestTaskLogPosition(ctx, "task-1"); err != nil {
		t.Fatalf("删除节点后日志索引读取失败：%v", err)
	}
	if records, _, _, err := persistent.ReadPage(ctx, "task-1", nil); err != nil || len(records) != 1 || records[0].Message != "safe synthetic log" {
		t.Fatalf("删除节点后已接收日志丢失：%#v %v", records, err)
	}
	for _, statement := range []string{`UPDATE tasks SET snapshot_json='{}'`, `DELETE FROM tasks`, `DELETE FROM execution_events`, `DELETE FROM execution_node_references`, `UPDATE execution_node_references SET node_id='changed'`} {
		if _, err := s.db.Exec(statement); err == nil {
			t.Fatalf("不可变约束丢失：%s", statement)
		}
	}
	rows, err := s.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() || rows.Err() != nil {
		t.Fatal("永久删除后外键不完整")
	}
}

func Test删除节点保留排队任务但不能再领取(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 16<<30)
	ctx := context.Background()
	if err := s.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteExecutionNode(ctx, ExecutionNodeDeletion{NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: 1, RequestID: "delete-waiting", DeletedAt: testTime.Add(3 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if summary, err := s.GetAuthorizedTaskSummary(ctx, "task-1", "subject-1"); err != nil || summary.State != "WAITING_SCHEDULE" {
		t.Fatalf("排队任务历史读取失败：%#v %v", summary, err)
	}
	if _, found, err := s.ClaimNextExecution(ctx, budgetClaim("after-delete")); found || (err != nil && !errors.Is(err, ErrClaimIneligible)) {
		t.Fatalf("已删除节点仍能领取排队任务：found=%v err=%v", found, err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM task_executions`, 0)
}

func Test永久删除节点拒绝运行待核对及未释放预算(t *testing.T) {
	for _, fixture := range []struct {
		name, state string
		reconciling bool
		allocation  bool
	}{{"starting", "STARTING", false, false}, {"running", "RUNNING", false, false}, {"cancelling", "CANCELLING", false, false}, {"reconciling", "FAILED", true, false}, {"unreleased", "FAILED", false, true}} {
		t.Run(fixture.name, func(t *testing.T) {
			s, _ := openTestStore(t)
			seedBaseFixture(t, s)
			ctx := context.Background()
			if err := s.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
				t.Fatal(err)
			}
			if err := s.ClaimTask(ctx, validClaim("execution-1", "lease-1", "event-1", "claim-1")); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`UPDATE task_executions SET state=?, reconciliation_required=?`, fixture.state, fixture.reconciling); err != nil {
				t.Fatal(err)
			}
			if fixture.allocation {
				if _, err := s.db.Exec(`INSERT INTO execution_resource_allocations(execution_id,heap_bytes,budget_bytes,allocated_at) VALUES ('execution-1',1,2,?)`, utcText(testTime)); err != nil {
					t.Fatal(err)
				}
			}
			_, err := s.DeleteExecutionNode(ctx, ExecutionNodeDeletion{NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: 1, RequestID: "delete-blocked-1", DeletedAt: testTime.Add(time.Minute)})
			if !errors.Is(err, ErrExecutionNodeHasRunningTask) {
				t.Fatalf("删除未阻断：%v", err)
			}
			assertCount(t, s.db, `SELECT COUNT(*) FROM execution_nodes WHERE node_id='node-1' AND revision=1`, 1)
			assertCount(t, s.db, `SELECT COUNT(*) FROM agents WHERE status='ACTIVE'`, 1)
			assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='EXECUTION_NODE_DELETED'`, 0)
		})
	}
}

func Test永久删除保留创建幂等且不能复用标识(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	input := ExecutionNodeCreate{NodeID: "node-new", CreatorSubjectID: "subject-1", DisplayName: "Synthetic New", NormalizedName: "synthetic new", Platform: "WINDOWS_AMD64", AllowedRoots: []string{`E:\synthetic`}, ToolHome: `E:\synthetic\tools`, JavaPath: `E:\synthetic\java.exe`, RequestID: "create-new", IdempotencyKey: "synthetic-create-new", RequestDigest: testFingerprint, CreatedAt: testTime}
	if _, err := s.CreateExecutionNode(ctx, input); err != nil {
		t.Fatal(err)
	}
	deletion := ExecutionNodeDeletion{NodeID: input.NodeID, ActorSubjectID: "subject-1", ExpectedRevision: 1, RequestID: "delete-new", DeletedAt: testTime.Add(time.Minute)}
	stale := deletion
	stale.ExpectedRevision = 2
	if _, err := s.DeleteExecutionNode(ctx, stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("旧版本删除未拒绝：%v", err)
	}
	if _, err := s.DeleteExecutionNode(ctx, deletion); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateExecutionNode(ctx, input); err != nil {
		t.Fatalf("创建幂等重放失败：%v", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM execution_nodes WHERE node_id='node-new'`, 0)
	input.IdempotencyKey = "synthetic-new-key"
	input.RequestID = "create-with-new-key"
	if _, err := s.CreateExecutionNode(ctx, input); err == nil {
		t.Fatal("新的幂等键复用了已删除标识")
	}
	input.NodeID = "node-different"
	if _, err := s.CreateExecutionNode(ctx, input); err != nil {
		t.Fatalf("删除后名称未释放：%v", err)
	}
	if _, err := s.DeleteExecutionNode(ctx, deletion); !errors.Is(err, ErrExecutionNodeNotFound) {
		t.Fatalf("重复删除未返回不存在：%v", err)
	}
}
