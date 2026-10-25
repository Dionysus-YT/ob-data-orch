package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/schedulingpolicy"
)

func schedulingChange(maximum int) schedulingpolicy.Change {
	return schedulingpolicy.Change{ActorID: "subject-1", ExpectedRevision: 1, MaxConcurrentExecutions: maximum, Reason: "合成调度策略验证", Confirmed: true, IdempotencyKey: "synthetic-scheduling-key", RequestID: "synthetic-scheduling-request", At: testTime}
}

// setTestSchedulingLimit 仅给临时夹具安装合法策略，不经过真实部署或网络。
func setTestSchedulingLimit(t *testing.T, s *Store, maximum int) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE platform_scheduling_policy SET max_concurrent_executions=?,modified_by='subject-1',modified_at=? WHERE policy_id='platform-executions'`, maximum, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
}

func Test调度设置未配置及数量权限收缩(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	policy, err := s.ReadAuthorizedSchedulingPolicy(ctx, "subject-1")
	if err != nil || policy.MaxConcurrentExecutions != nil || policy.Source != "UNCONFIGURED" || policy.ModifiedAt != nil || policy.Usage.Visibility != "AVAILABLE" {
		t.Fatalf("初值=%+v %v", policy, err)
	}
	input := permissionChange()
	input.Confirmed = true
	input.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.SystemAdmin}}
	if _, err := s.ChangeSubjectPermissions(ctx, input); err != nil {
		t.Fatal(err)
	}
	policy, err = s.ReadAuthorizedSchedulingPolicy(ctx, "permission-user")
	if err != nil || policy.Usage.Visibility != "RESTRICTED" || policy.Usage.HeldExecutions != nil || policy.Usage.RunningExecutions != nil {
		t.Fatalf("泄露数量=%+v %v", policy, err)
	}
	if _, err = s.ChangeSchedulingPolicy(ctx, schedulingpolicy.Change{ActorID: "permission-user", ExpectedRevision: 1, MaxConcurrentExecutions: 1, Reason: "合成收紧", Confirmed: true, IdempotencyKey: "restricted-key", RequestID: "restricted-request", At: testTime}); err != nil {
		t.Fatal(err)
	}
}

func Test调度策略版本幂等重启及审计回滚(t *testing.T) {
	s, path := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	if err := s.BootstrapPermissionOwner(ctx, "subject-1", testTime); err != nil {
		t.Fatal(err)
	}
	input := schedulingChange(8)
	result, err := s.ChangeSchedulingPolicy(ctx, input)
	if err != nil || result.Revision != 2 || !result.Changed {
		t.Fatalf("保存=%+v %v", result, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	policy, err := s.ReadAuthorizedSchedulingPolicy(ctx, "subject-1")
	if err != nil || policy.MaxConcurrentExecutions == nil || *policy.MaxConcurrentExecutions != 8 {
		t.Fatalf("重启=%+v %v", policy, err)
	}
	next := input
	next.ExpectedRevision = 2
	next.MaxConcurrentExecutions = 12
	next.IdempotencyKey = "synthetic-next-key"
	next.RequestID = "synthetic-next-request"
	if _, err = s.ChangeSchedulingPolicy(ctx, next); err != nil {
		t.Fatal(err)
	}
	if replay, err := s.ChangeSchedulingPolicy(ctx, input); err != nil || !replay.Replayed || replay.Revision != 2 {
		t.Fatalf("重放=%+v %v", replay, err)
	}
	input.MaxConcurrentExecutions = 7
	if _, err = s.ChangeSchedulingPolicy(ctx, input); !errors.Is(err, schedulingpolicy.ErrIdempotency) {
		t.Fatalf("同键异内容=%v", err)
	}
	next.ExpectedRevision = 3
	next.MaxConcurrentExecutions = 12
	next.IdempotencyKey = "same-value-key"
	if same, err := s.ChangeSchedulingPolicy(ctx, next); err != nil || same.Changed || same.Revision != 3 {
		t.Fatalf("同值=%+v %v", same, err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER synthetic_scheduling_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='SCHEDULING_POLICY_CHANGED' BEGIN SELECT RAISE(ABORT,'synthetic audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	next.MaxConcurrentExecutions = 1
	next.IdempotencyKey = "rollback-key"
	if _, err = s.ChangeSchedulingPolicy(ctx, next); err == nil {
		t.Fatal("审计失败未回滚")
	}
	policy, _ = s.ReadAuthorizedSchedulingPolicy(ctx, "subject-1")
	if policy.Revision != 3 || *policy.MaxConcurrentExecutions != 12 {
		t.Fatal("出现部分保存")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM request_idempotency WHERE operation='CHANGE_SCHEDULING_POLICY' AND idempotency_key='rollback-key'`, 0)
}

func Test调度设置非法字段确认版本和撤权失败关闭(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	for i, scenario := range []struct {
		maximum   int
		confirmed bool
		revision  int64
		reason    string
		want      error
	}{
		{0, true, 1, "合成原因", schedulingpolicy.ErrInvalid}, {1025, true, 1, "合成原因", schedulingpolicy.ErrInvalid}, {1, false, 1, "合成原因", schedulingpolicy.ErrConfirmation}, {1, true, 2, "合成原因", schedulingpolicy.ErrRevision}, {1, true, 1, "password=synthetic-only", schedulingpolicy.ErrInvalid},
	} {
		input := schedulingChange(scenario.maximum)
		input.Confirmed = scenario.confirmed
		input.ExpectedRevision = scenario.revision
		input.Reason = scenario.reason
		input.IdempotencyKey = fmt.Sprint("case-", i)
		if _, err := s.ChangeSchedulingPolicy(ctx, input); !errors.Is(err, scenario.want) {
			t.Fatalf("拒绝=%v want%v", err, scenario.want)
		}
	}
	input := schedulingChange(8)
	if _, err := s.ChangeSchedulingPolicy(ctx, input); err != nil {
		t.Fatal(err)
	}
	lower := input
	lower.MaxConcurrentExecutions = 7
	lower.ExpectedRevision = 2
	lower.Confirmed = false
	lower.IdempotencyKey = "lower-key"
	if _, err := s.ChangeSchedulingPolicy(ctx, lower); !errors.Is(err, schedulingpolicy.ErrConfirmation) {
		t.Fatalf("降低未确认=%v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_SYSTEM_ADMIN'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeSchedulingPolicy(ctx, input); !errors.Is(err, schedulingpolicy.ErrDenied) {
		t.Fatalf("撤权重放=%v", err)
	}
	if _, err := s.ReadAuthorizedSchedulingPolicy(ctx, "subject-1"); !errors.Is(err, schedulingpolicy.ErrDenied) {
		t.Fatalf("撤权读取=%v", err)
	}
}

func Test平台并发原子领取重放降低及可靠释放(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 256*executionbudget.GiB)
	setTestSchedulingLimit(t, s, 3)
	for i := 0; i < 8; i++ {
		submitBudgetTask(t, s, fmt.Sprint("scheduled-", i), "4G")
	}
	var granted atomic.Int32
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func(id int) {
			defer group.Done()
			_, found, err := s.ClaimNextExecution(context.Background(), budgetClaim(fmt.Sprint(id)))
			if err != nil {
				failures <- err
			}
			if found {
				granted.Add(1)
			}
		}(i)
	}
	group.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if granted.Load() != 3 {
		t.Fatalf("超卖或漏领=%d", granted.Load())
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM task_executions`, 3)
	var request string
	if err := s.db.QueryRow(`SELECT request_id FROM execution_claim_receipts LIMIT 1`).Scan(&request); err != nil {
		t.Fatal(err)
	}
	replay := budgetClaim("replay")
	replay.RequestID = request
	if _, found, err := s.ClaimNextExecution(context.Background(), replay); err != nil || !found {
		t.Fatalf("满额重放=%v %v", found, err)
	}
	setTestSchedulingLimit(t, s, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM task_executions WHERE state<>'STARTING'`, 0)
	if _, found, err := s.ClaimNextExecution(context.Background(), replay); err != nil || !found {
		t.Fatalf("降低后既有回执不能重放=%v %v", found, err)
	}
	if _, err := s.db.Exec(`UPDATE task_executions SET state='FAILED',reconciliation_required=1`); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"OBDUMPER_EXPORT", "OBLOADER_NORMAL_IMPORT", "OBLOADER_DIRECT_LOAD"} {
		s.ConfigureDirectLoadCapabilities(true, true)
		input := budgetClaim("blocked-" + kind)
		input.TaskType = kind
		if _, found, err := s.ClaimNextExecution(context.Background(), input); err != nil || found {
			t.Fatalf("共享上限 %s=%v %v", kind, found, err)
		}
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM task_executions`, 3)
	// 失联和失败投影不能释放；只有既有租约下已接受的无进程拒绝才释放本例占用。
	if _, err := s.db.Exec(`INSERT INTO execution_events(event_id,execution_id,lease_id,lease_epoch,event_seq,event_type,payload_json,received_at,accepted) SELECT 'release-'||e.execution_id,e.execution_id,l.lease_id,1,2,'START_REJECTED','{}',?,1 FROM task_executions e JOIN execution_leases l ON l.execution_id=e.execution_id`, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.ClaimNextExecution(context.Background(), budgetClaim("after-release")); err != nil || !found {
		t.Fatalf("可靠释放=%v %v", found, err)
	}
}

func Test平台策略不可用阻断且旧合成入口不能绕过(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 64*executionbudget.GiB)
	setTestSchedulingLimit(t, s, 1)
	submitBudgetTask(t, s, "synthetic-first", "")
	submitBudgetTask(t, s, "synthetic-second", "")
	first := validClaim("legacy-first", "legacy-lease-1", "legacy-event-1", "legacy-request-1")
	first.TaskID = "synthetic-first"
	if err := s.ClaimTask(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := validClaim("legacy-second", "legacy-lease-2", "legacy-event-2", "legacy-request-2")
	second.TaskID = "synthetic-second"
	if err := s.ClaimTask(context.Background(), second); !errors.Is(err, schedulingpolicy.ErrCapacity) {
		t.Fatalf("合成绕过=%v", err)
	}
	summary, err := s.GetAuthorizedTaskSummary(context.Background(), second.TaskID, "subject-1")
	if err != nil || summary.SchedulingWaitReason != "PLATFORM_CONCURRENCY_LIMIT" {
		t.Fatalf("等待原因=%+v %v", summary, err)
	}
	if _, err := s.db.Exec(`DELETE FROM platform_scheduling_policy`); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.ClaimNextExecution(context.Background(), budgetClaim("missing-policy")); err == nil || found {
		t.Fatal("缺失策略仍领取")
	}
	summary, err = s.GetAuthorizedTaskSummary(context.Background(), second.TaskID, "subject-1")
	if err != nil || summary.SchedulingWaitReason != "SCHEDULING_POLICY_UNAVAILABLE" {
		t.Fatalf("未知策略=%+v %v", summary, err)
	}
}
