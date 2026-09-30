package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test数据源删除复验任务并保留历史(t *testing.T) {
	for _, state := range []string{"WAITING_SCHEDULE", "STARTING", "RUNNING", "CANCELLING", "SUCCEEDED", "FAILED", "CANCELLED", "RECONCILING"} {
		t.Run(state, func(t *testing.T) {
			s, _ := openTestStore(t)
			seedBaseFixture(t, s)
			ctx := context.Background()
			// 模拟页面先取得可删除资格，随后有任务提交；删除写事务必须重新检查。
			before, err := s.GetDataSourceSummary(ctx, "source-1")
			if err != nil || !before.LifecycleEligibility.Delete.Allowed {
				t.Fatalf("初始删除资格不正确: %v", err)
			}
			if err := s.SubmitTask(ctx, validTaskSubmission("task-delete-check")); err != nil {
				t.Fatal(err)
			}
			if state != "WAITING_SCHEDULE" {
				executionState, reconciling := state, 0
				if state == "RECONCILING" {
					executionState, reconciling = "FAILED", 1
				}
				_, err := s.db.Exec(`INSERT INTO task_executions(execution_id, task_id, node_id, agent_id, state, revision, reconciliation_required, created_at, updated_at)
					VALUES ('execution-delete-check', 'task-delete-check', 'node-1', 'agent-1', ?, 1, ?, ?, ?)`, executionState, reconciling, utcText(testTime), utcText(testTime))
				if err != nil {
					t.Fatal(err)
				}
			}
			allowed := state == "SUCCEEDED" || state == "FAILED" || state == "CANCELLED"
			summary, err := s.GetDataSourceSummary(ctx, "source-1")
			if err != nil || summary.LifecycleEligibility.Delete.Allowed != allowed {
				t.Fatalf("任务状态 %s 的删除资格错误: %v", state, err)
			}
			input := DataSourceDeletion{DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 2, RequestID: "delete-stale", DeletedAt: testTime.Add(time.Minute)}
			if _, err := s.DeleteDataSource(ctx, input); !errors.Is(err, ErrRevisionConflict) {
				t.Fatalf("旧版本删除未被拒绝: %v", err)
			}
			input.ExpectedRevision, input.RequestID = 1, "delete-current"
			_, err = s.DeleteDataSource(ctx, input)
			if !allowed {
				if !errors.Is(err, ErrDataSourceDeleteIneligible) {
					t.Fatalf("活动任务未阻止删除: %v", err)
				}
				assertCount(t, s.db, "SELECT COUNT(*) FROM data_sources", 1)
				assertCount(t, s.db, "SELECT COUNT(*) FROM credential_revisions", 1)
				assertCount(t, s.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_DELETED'", 0)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertCount(t, s.db, "SELECT COUNT(*) FROM data_sources", 0)
			assertCount(t, s.db, "SELECT COUNT(*) FROM credential_revisions", 0)
			assertCount(t, s.db, "SELECT COUNT(*) FROM tasks", 1)
			assertCount(t, s.db, "SELECT COUNT(*) FROM task_executions", 1)
			assertCount(t, s.db, "SELECT COUNT(*) FROM export_drafts", 1)
			assertCount(t, s.db, "SELECT COUNT(*) FROM precheck_runs WHERE status = 'INVALIDATED'", 1)
			assertCount(t, s.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_DELETED'", 1)
		})
	}
}

func Test删除数据源使未完成连接测试失效(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	requestSyntheticDataSourceConnectionTest(t, s, "test-delete-history", "G2_SYNTHETIC")
	_, err := s.DeleteDataSource(context.Background(), DataSourceDeletion{DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 1, RequestID: "delete-test-history", DeletedAt: testTime.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	assertCount(t, s.db, "SELECT COUNT(*) FROM data_source_connection_test_runs WHERE status = 'INVALIDATED'", 1)
	assertCount(t, s.db, "SELECT COUNT(*) FROM data_sources", 0)
}
