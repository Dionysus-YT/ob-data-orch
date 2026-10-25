package store

import (
	"context"
	"errors"
	"testing"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/schedulingpolicy"
)

func candidateChange() schedulingpolicy.CandidateChange {
	return schedulingpolicy.CandidateChange{ActorID: "subject-1", ExpectedRevision: 1, Order: schedulingpolicy.OnlineFirst, Reason: "合成候选排序验证", Confirmed: true, IdempotencyKey: "synthetic-candidate-key", RequestID: "synthetic-candidate-request", At: testTime}
}

func Test任务候选快照隔离隐藏坏节点和预算(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	input := permissionChange()
	input.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.BasicUser, accesscontrol.NodeAdmin}, Scopes: []accesscontrol.ObjectScope{{Type: accesscontrol.NodeManage, ObjectID: "node-1"}}}
	input.Confirmed = true
	if _, err := s.ChangeSubjectPermissions(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO execution_nodes(node_id,display_name,normalized_name,platform,management_state,allowed_roots_json,revision,created_by,created_at,updated_at) VALUES('candidate-hidden','合成隐藏节点','candidate hidden','WINDOWS_AMD64','ENABLED','[1]',1,'subject-1',?,?)`, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	nodes, _, err := s.ReadTaskNodeSelection(ctx, "permission-user")
	if err != nil || len(nodes) != 1 || nodes[0].NodeID != "node-1" {
		t.Fatalf("隐藏坏事实影响候选=%d %v", len(nodes), err)
	}
	if _, err := s.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='permission-user'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReadTaskNodeSelection(ctx, "permission-user"); !errors.Is(err, schedulingpolicy.ErrDenied) {
		t.Fatal("停用主体仍可读取")
	}
}

func Test候选策略确认版本幂等独立事务与重启(t *testing.T) {
	s, path := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	if err := s.BootstrapPermissionOwner(ctx, "subject-1", testTime); err != nil {
		t.Fatal(err)
	}
	initial, err := s.ReadAuthorizedCandidatePolicy(ctx, "subject-1")
	if err != nil || initial.Order != schedulingpolicy.ExistingOrder || initial.Revision != 1 || initial.ModifiedAt != nil {
		t.Fatalf("初值=%+v %v", initial, err)
	}
	input := candidateChange()
	input.Confirmed = false
	if _, err = s.ChangeCandidatePolicy(ctx, input); !errors.Is(err, schedulingpolicy.ErrConfirmation) {
		t.Fatalf("确认=%v", err)
	}
	input.Confirmed = true
	receipt, err := s.ChangeCandidatePolicy(ctx, input)
	if err != nil || receipt.Revision != 2 || !receipt.Changed {
		t.Fatalf("保存=%+v %v", receipt, err)
	}
	if replay, err := s.ChangeCandidatePolicy(ctx, input); err != nil || !replay.Replayed || replay.Revision != 2 {
		t.Fatalf("重放=%+v %v", replay, err)
	}
	changed := input
	changed.Order = schedulingpolicy.ExistingOrder
	if _, err := s.ChangeCandidatePolicy(ctx, changed); !errors.Is(err, schedulingpolicy.ErrIdempotency) {
		t.Fatalf("异内容=%v", err)
	}
	changed.IdempotencyKey = "candidate-revert-key"
	changed.ExpectedRevision = 2
	if _, err := s.ChangeCandidatePolicy(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	current, err := s.ReadAuthorizedCandidatePolicy(ctx, "subject-1")
	if err != nil || current.Order != schedulingpolicy.ExistingOrder || current.Revision != 3 || current.ModifiedAt == nil {
		t.Fatalf("回退重启=%+v %v", current, err)
	}
	if replay, err := s.ChangeCandidatePolicy(ctx, input); err != nil || !replay.Replayed || replay.Revision != 2 {
		t.Fatalf("历史回执=%+v %v", replay, err)
	}
	changed.ExpectedRevision = 3
	changed.IdempotencyKey = "candidate-noop-key"
	changed.Confirmed = false
	if noop, err := s.ChangeCandidatePolicy(ctx, changed); err != nil || noop.Changed || noop.Revision != 3 {
		t.Fatalf("无变更=%+v %v", noop, err)
	}
	maximum, err := s.ReadAuthorizedSchedulingPolicy(ctx, "subject-1")
	if err != nil || maximum.Revision != 1 || maximum.MaxConcurrentExecutions != nil {
		t.Fatal("候选修改触及并发")
	}
	timing, err := s.ReadCurrentSchedulingTiming(ctx, schedulingpolicy.DefaultHeartbeatTimeout)
	if err != nil || timing.Revision != 1 || timing.StartupTimeoutSeconds != nil {
		t.Fatal("候选修改触及时间")
	}
}

func Test候选策略拒绝权限非法字段旧版本且审计失败回滚(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	if _, err := s.ReadAuthorizedCandidatePolicy(ctx, "permission-user"); !errors.Is(err, schedulingpolicy.ErrDenied) {
		t.Fatalf("读取权限=%v", err)
	}
	input := candidateChange()
	input.ActorID = "permission-user"
	if _, err := s.ChangeCandidatePolicy(ctx, input); !errors.Is(err, schedulingpolicy.ErrDenied) {
		t.Fatalf("保存权限=%v", err)
	}
	for _, change := range []func(*schedulingpolicy.CandidateChange){func(v *schedulingpolicy.CandidateChange) { v.Order = "AUTO_SELECT" }, func(v *schedulingpolicy.CandidateChange) { v.Reason = "" }, func(v *schedulingpolicy.CandidateChange) { v.Reason = "password=synthetic-placeholder" }, func(v *schedulingpolicy.CandidateChange) { v.ExpectedRevision = 2 }} {
		input = candidateChange()
		change(&input)
		if _, err := s.ChangeCandidatePolicy(ctx, input); err == nil {
			t.Fatal("非法变更被接受")
		}
	}
	if _, err := s.db.Exec(`CREATE TRIGGER candidate_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='CANDIDATE_ORDER_CHANGED' BEGIN SELECT RAISE(ABORT,'synthetic audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCandidatePolicy(ctx, candidateChange()); err == nil {
		t.Fatal("审计失败被忽略")
	}
	current, err := s.ReadCurrentCandidatePolicy(ctx)
	if err != nil || current.Revision != 1 || current.Order != schedulingpolicy.ExistingOrder {
		t.Fatal("审计回滚失效")
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM request_idempotency WHERE operation='CHANGE_CANDIDATE_ORDER'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("失败留下成功回执")
	}
}

func Test候选心跳只读取固定事实且不继承停用身份(t *testing.T) {
	s, _ := openTestStore(t)
	seedTimingNode(t, s)
	ctx := context.Background()
	stamp, err := s.ReadCandidateHeartbeat(ctx, "node-1")
	if err != nil || stamp == nil || !stamp.Equal(timingTestNow) {
		t.Fatalf("心跳=%v %v", stamp, err)
	}
	if _, err := s.db.Exec(`UPDATE agents SET status='REVOKED' WHERE agent_id='agent-1'`); err != nil {
		t.Fatal(err)
	}
	if stamp, err = s.ReadCandidateHeartbeat(ctx, "node-1"); err != nil || stamp != nil {
		t.Fatal("撤销 Agent 仍被判在线")
	}
	if stamp, err = s.ReadCandidateHeartbeat(ctx, "missing-node"); err != nil || stamp != nil {
		t.Fatal("未知节点不是空事实")
	}
}
