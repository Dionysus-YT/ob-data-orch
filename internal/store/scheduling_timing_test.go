package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/schedulingpolicy"
)

var timingTestNow = testTime.Add(3 * time.Minute)

func timingClaim(id string) ExecutionClaimNext {
	input := budgetClaim(id)
	input.HeartbeatFreshAfter = timingTestNow.Add(-2 * time.Minute)
	return input
}
func seedTimingNode(t *testing.T, s *Store) {
	t.Helper()
	seedBudgetNode(t, s, 256*executionbudget.GiB)
	if _, err := s.db.Exec("UPDATE agents SET last_heartbeat_at=? WHERE agent_id='agent-1'", utcText(timingTestNow)); err != nil {
		t.Fatal(err)
	}
}

func timingSeconds(value int) *int { return &value }
func timingChange() schedulingpolicy.TimingChange {
	return schedulingpolicy.TimingChange{ActorID: "subject-1", ExpectedRevision: 1, BaselineHeartbeatTimeout: 2 * time.Minute, HeartbeatTimeoutSeconds: 120, StartupTimeoutSeconds: timingSeconds(30), PollingIntervalSeconds: timingSeconds(5), Reason: "合成时间策略调整", Confirmed: true, IdempotencyKey: "synthetic-timing-key", RequestID: "synthetic-timing-request", At: testTime}
}
func setTestTiming(t *testing.T, s *Store, heartbeat, startup, polling *int) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE platform_scheduling_timing_policy SET heartbeat_seconds=?,startup_seconds=?,polling_seconds=?,modified_by='subject-1',modified_at=?`, heartbeat, startup, polling, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
}

func Test时间策略真实来源版本重放及重启(t *testing.T) {
	s, path := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	if err := s.BootstrapPermissionOwner(ctx, "subject-1", testTime); err != nil {
		t.Fatal(err)
	}
	policy, err := s.ReadAuthorizedSchedulingTiming(ctx, "subject-1", 2*time.Minute)
	if err != nil || policy.HeartbeatTimeoutSeconds != 120 || policy.HeartbeatTimeoutSource != "CONTROL_PLANE" || policy.StartupTimeoutSeconds != nil || policy.PollingIntervalSeconds != nil || policy.CandidateStrategy != "EXPLICIT_SELECTION" || policy.ModifiedAt != nil {
		t.Fatalf("初始投影=%+v %v", policy, err)
	}
	input := timingChange()
	receipt, err := s.ChangeSchedulingTiming(ctx, input)
	if err != nil || receipt.Revision != 2 || !receipt.Changed {
		t.Fatalf("保存=%+v %v", receipt, err)
	}
	policy, err = s.ReadAuthorizedSchedulingTiming(ctx, "subject-1", 3*time.Minute)
	if err != nil || policy.HeartbeatTimeoutSeconds != 180 || policy.HeartbeatTimeoutSource != "CONTROL_PLANE" || *policy.StartupTimeoutSeconds != 30 {
		t.Fatalf("同值误建心跳覆盖=%+v %v", policy, err)
	}
	input.ExpectedRevision = 2
	input.HeartbeatTimeoutSeconds = 180
	input.IdempotencyKey = "synthetic-timing-next"
	if _, err = s.ChangeSchedulingTiming(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	policy, err = s.ReadAuthorizedSchedulingTiming(ctx, "subject-1", 4*time.Minute)
	if err != nil || policy.HeartbeatTimeoutSeconds != 180 || policy.HeartbeatTimeoutSource != "PLATFORM_SETTING" || policy.Revision != 3 {
		t.Fatalf("重启=%+v %v", policy, err)
	}
	if receipt, err = s.ChangeSchedulingTiming(ctx, timingChange()); err != nil || !receipt.Replayed || receipt.Revision != 2 {
		t.Fatalf("旧回执=%+v %v", receipt, err)
	}
	input.ExpectedRevision = 3
	input.IdempotencyKey = "same-timing-value"
	if receipt, err = s.ChangeSchedulingTiming(ctx, input); err != nil || receipt.Changed || receipt.Revision != 3 {
		t.Fatalf("同值=%+v %v", receipt, err)
	}
	capPolicy, err := s.ReadAuthorizedSchedulingPolicy(ctx, "subject-1")
	if err != nil || capPolicy.Revision != 1 || capPolicy.MaxConcurrentExecutions != nil {
		t.Fatal("时间设置改写了并发域")
	}
}

func Test时间策略非法输入确认撤权与审计失败关闭(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	for index, change := range []func(*schedulingpolicy.TimingChange){
		func(c *schedulingpolicy.TimingChange) { c.HeartbeatTimeoutSeconds = 59 }, func(c *schedulingpolicy.TimingChange) { c.HeartbeatTimeoutSeconds = 3601 },
		func(c *schedulingpolicy.TimingChange) { c.StartupTimeoutSeconds = timingSeconds(14) }, func(c *schedulingpolicy.TimingChange) { c.PollingIntervalSeconds = timingSeconds(0) },
		func(c *schedulingpolicy.TimingChange) { c.Reason = "password=synthetic-only" },
	} {
		input := timingChange()
		change(&input)
		input.IdempotencyKey = fmt.Sprint("invalid-timing-", index)
		if _, err := s.ChangeSchedulingTiming(ctx, input); !errors.Is(err, schedulingpolicy.ErrInvalid) {
			t.Fatalf("非法输入=%v", err)
		}
	}
	input := timingChange()
	input.Confirmed = false
	if _, err := s.ChangeSchedulingTiming(ctx, input); !errors.Is(err, schedulingpolicy.ErrConfirmation) {
		t.Fatalf("首次收紧未确认=%v", err)
	}
	input.Confirmed = true
	if _, err := s.ChangeSchedulingTiming(ctx, input); err != nil {
		t.Fatal(err)
	}
	conflict := input
	conflict.Reason = "合成不同载荷"
	if _, err := s.ChangeSchedulingTiming(ctx, conflict); !errors.Is(err, schedulingpolicy.ErrIdempotency) {
		t.Fatalf("异载荷=%v", err)
	}
	reset := input
	reset.ExpectedRevision = 2
	reset.StartupTimeoutSeconds = nil
	reset.IdempotencyKey = "no-reset"
	if _, err := s.ChangeSchedulingTiming(ctx, reset); !errors.Is(err, schedulingpolicy.ErrInvalid) {
		t.Fatalf("清空已配置期限=%v", err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER synthetic_timing_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='SCHEDULING_TIMING_CHANGED' BEGIN SELECT RAISE(ABORT,'synthetic audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	input.ExpectedRevision = 2
	input.HeartbeatTimeoutSeconds = 240
	input.IdempotencyKey = "audit-rollback"
	if _, err := s.ChangeSchedulingTiming(ctx, input); err == nil {
		t.Fatal("审计失败仍保存")
	}
	policy, err := s.ReadAuthorizedSchedulingTiming(ctx, "subject-1", 2*time.Minute)
	if err != nil || policy.Revision != 2 || policy.HeartbeatTimeoutSeconds != 120 {
		t.Fatal("出现部分保存")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM request_idempotency WHERE operation='CHANGE_SCHEDULING_TIMING' AND idempotency_key='audit-rollback'`, 0)
	if _, err = s.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_SYSTEM_ADMIN'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangeSchedulingTiming(ctx, timingChange()); !errors.Is(err, schedulingpolicy.ErrDenied) {
		t.Fatalf("撤权重放=%v", err)
	}
	if _, err = s.ReadAuthorizedSchedulingTiming(ctx, "subject-1", 2*time.Minute); !errors.Is(err, schedulingpolicy.ErrDenied) {
		t.Fatalf("撤权读取=%v", err)
	}
}

func Test启动期限冻结只核对不释放并发且重复扫描幂等(t *testing.T) {
	s, _ := openTestStore(t)
	seedTimingNode(t, s)
	setTestSchedulingLimit(t, s, 1)
	setTestTiming(t, s, nil, timingSeconds(15), nil)
	submitBudgetTask(t, s, "timed-start", "4G")
	submitBudgetTask(t, s, "timed-wait", "4G")
	ctx := context.Background()
	grant, found, err := s.ClaimNextExecution(ctx, timingClaim("timed-start"))
	if err != nil || !found {
		t.Fatalf("领取=%v %v", found, err)
	}
	var deadline string
	if err = s.db.QueryRow(`SELECT startup_deadline FROM task_executions WHERE execution_id=?`, grant.ExecutionID).Scan(&deadline); err != nil || deadline != utcText(timingTestNow.Add(15*time.Second)) {
		t.Fatalf("冻结期限=%s %v", deadline, err)
	}
	setTestTiming(t, s, nil, timingSeconds(60), nil)
	if _, err = s.db.Exec(`UPDATE task_executions SET startup_deadline=? WHERE execution_id=?`, utcText(timingTestNow.Add(time.Minute)), grant.ExecutionID); err == nil {
		t.Fatal("旧期限可改写")
	}
	if n, err := s.ReconcileSchedulingTimeouts(ctx, timingTestNow.Add(14*time.Second), 2*time.Minute); err != nil || n != 0 {
		t.Fatalf("过早核对=%d %v", n, err)
	}
	if n, err := s.ReconcileSchedulingTimeouts(ctx, timingTestNow.Add(15*time.Second), 2*time.Minute); err != nil || n != 1 {
		t.Fatalf("到期核对=%d %v", n, err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM task_executions WHERE state='STARTING' AND reconciliation_required=1 AND finished_at IS NULL`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM execution_resource_allocations`, 1)
	if n, err := s.ReconcileSchedulingTimeouts(ctx, timingTestNow.Add(16*time.Second), 2*time.Minute); err != nil || n != 0 {
		t.Fatalf("重复扫描=%d %v", n, err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='TASK_SCHEDULING_RECONCILIATION_REQUIRED'`, 1)
	if _, found, err = s.ClaimNextExecution(ctx, timingClaim("still-held")); err != nil || found {
		t.Fatalf("启动超时释放额度=%v %v", found, err)
	}
}

func Test新领取在事务内复核最新心跳阈值(t *testing.T) {
	s, _ := openTestStore(t)
	seedTimingNode(t, s)
	submitBudgetTask(t, s, "heartbeat-bound", "4G")
	if _, err := s.db.Exec(`UPDATE agents SET last_heartbeat_at=? WHERE agent_id='agent-1'`, utcText(timingTestNow.Add(-90*time.Second))); err != nil {
		t.Fatal(err)
	}
	setTestTiming(t, s, timingSeconds(60), nil, nil)
	input := timingClaim("stale-heartbeat")
	input.HeartbeatFreshAfter = timingTestNow.Add(-2 * time.Minute)
	if _, found, err := s.ClaimNextExecution(context.Background(), input); err != nil || found {
		t.Fatalf("旧阈值绕过=%v %v", found, err)
	}
	setTestTiming(t, s, timingSeconds(180), nil, nil)
	input = timingClaim("fresh-heartbeat")
	input.HeartbeatFreshAfter = timingTestNow.Add(-time.Minute)
	if _, found, err := s.ClaimNextExecution(context.Background(), input); err != nil || !found {
		t.Fatalf("新阈值未生效=%v %v", found, err)
	}
	setTestTiming(t, s, timingSeconds(60), nil, nil)
	if n, err := s.ReconcileSchedulingTimeouts(context.Background(), timingTestNow, 2*time.Minute); err != nil || n != 1 {
		t.Fatalf("失联核对=%d %v", n, err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM task_executions WHERE state='STARTING' AND reconciliation_required=1 AND finished_at IS NULL`, 1)
}

func Test领取检查分类型持久计时且不阻断回执重放(t *testing.T) {
	s, path := openTestStore(t)
	seedTimingNode(t, s)
	setTestTiming(t, s, nil, nil, timingSeconds(10))
	for i := 0; i < 3; i++ {
		submitBudgetTask(t, s, fmt.Sprint("poll-task-", i), "4G")
	}
	ctx := context.Background()
	first := timingClaim("poll-first")
	grant, found, err := s.ClaimNextExecution(ctx, first)
	if err != nil || !found {
		t.Fatalf("首领=%v %v", found, err)
	}
	replay := first
	replay.Now = timingTestNow.Add(time.Second)
	if again, found, err := s.ClaimNextExecution(ctx, replay); err != nil || !found || again.ExecutionID != grant.ExecutionID {
		t.Fatalf("节流阻断重放=%v %v", found, err)
	}
	second := timingClaim("poll-second")
	second.Now = timingTestNow.Add(time.Second)
	if _, found, err := s.ClaimNextExecution(ctx, second); err != nil || found {
		t.Fatalf("未到间隔仍检查=%v %v", found, err)
	}
	other := timingClaim("poll-other-kind")
	other.TaskType = "OBLOADER_NORMAL_IMPORT"
	other.Now = second.Now
	if _, _, err = s.ClaimNextExecution(ctx, other); err != nil {
		t.Fatal(err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM agent_execution_poll_checks`, 2)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	second = timingClaim("poll-after-restart")
	second.Now = timingTestNow.Add(2 * time.Second)
	if _, found, err := s.ClaimNextExecution(ctx, second); err != nil || found {
		t.Fatalf("重启丢失间隔=%v %v", found, err)
	}
	setTestTiming(t, s, nil, nil, timingSeconds(1))
	second = timingClaim("poll-lower")
	second.Now = timingTestNow.Add(2 * time.Second)
	if _, found, err := s.ClaimNextExecution(ctx, second); err != nil || !found {
		t.Fatalf("新间隔未立即参与计算=%v %v", found, err)
	}
}

func Test时间策略缺失阻断领取且核对审计失败不部分标记(t *testing.T) {
	s, _ := openTestStore(t)
	seedTimingNode(t, s)
	setTestTiming(t, s, nil, timingSeconds(15), nil)
	submitBudgetTask(t, s, "monitor-rollback", "4G")
	ctx := context.Background()
	first := timingClaim("monitor-rollback")
	if _, found, err := s.ClaimNextExecution(ctx, first); err != nil || !found {
		t.Fatalf("领取=%v %v", found, err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER synthetic_monitor_failure BEFORE INSERT ON audit_events WHEN NEW.action='TASK_SCHEDULING_RECONCILIATION_REQUIRED' BEGIN SELECT RAISE(ABORT,'synthetic audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ReconcileSchedulingTimeouts(ctx, timingTestNow.Add(15*time.Second), 2*time.Minute); err == nil || n != 0 {
		t.Fatal("核对审计失败未回滚")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM task_executions WHERE reconciliation_required=1`, 0)
	if _, err := s.db.Exec(`DELETE FROM platform_scheduling_timing_policy`); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.ClaimNextExecution(ctx, first); err != nil || !found {
		t.Fatalf("策略缺失阻断了既有回执=%v %v", found, err)
	}
	if _, found, err := s.ClaimNextExecution(ctx, timingClaim("missing-timing")); err == nil || found {
		t.Fatal("缺失时间策略仍领取")
	}
}
