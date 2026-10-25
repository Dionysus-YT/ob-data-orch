package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logretention"
)

func retentionChange() logretention.Change {
	return logretention.Change{ActorID: "subject-1", ExpectedRevision: 1, RetentionDays: 7, Reason: "合成日志保留验证", Confirmed: true, IdempotencyKey: "synthetic-retention-key", RequestID: "synthetic-retention-request", At: testTime}
}

func Test保留策略持久化重启来源版本及历史回执(t *testing.T) {
	database, path := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	if err := database.BootstrapPermissionOwner(ctx, "subject-1", testTime); err != nil {
		t.Fatal(err)
	}
	service, _ := logretention.New(database, 60)
	policy, err := service.Read(ctx, "subject-1")
	if err != nil || policy.RetentionDays != 60 || policy.Source != "DEPLOYMENT_CONFIG" {
		t.Fatalf("初值=%+v %v", policy, err)
	}
	input := retentionChange()
	result, err := service.Change(ctx, input)
	if err != nil || result.Revision != 2 || !result.Changed {
		t.Fatalf("保存=%+v %v", result, err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	service, _ = logretention.New(reopened, 365)
	policy, err = service.Read(ctx, "subject-1")
	if err != nil || policy.RetentionDays != 7 || policy.Source != "PLATFORM_SETTING" || policy.ModifiedBy != "subject-1" || policy.ModifiedAt == nil {
		t.Fatalf("重启=%+v %v", policy, err)
	}
	next := input
	next.RetentionDays = 90
	next.ExpectedRevision = 2
	next.IdempotencyKey = "synthetic-next-key"
	next.RequestID = "synthetic-next-request"
	if _, err = service.Change(ctx, next); err != nil {
		t.Fatal(err)
	}
	if result, err = service.Change(ctx, input); err != nil || !result.Replayed || result.Revision != 2 {
		t.Fatalf("历史回执=%+v %v", result, err)
	}
	policy, _ = service.Read(ctx, "subject-1")
	if policy.RetentionDays != 90 || policy.Revision != 3 {
		t.Fatal("重放覆盖当前策略")
	}
	input.Reason = "不同合成原因"
	if _, err = service.Change(ctx, input); !errors.Is(err, logretention.ErrIdempotency) {
		t.Fatalf("同键异内容=%v", err)
	}
	assertCount(t, reopened.db, `SELECT COUNT(*) FROM audit_events WHERE action='LOG_RETENTION_CHANGED' AND result='SUCCEEDED'`, 2)
}

func Test保留策略拒绝越界缺确认旧版本及秘密原因(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*logretention.Change)
		want   error
	}{
		{"下界", func(c *logretention.Change) { c.RetentionDays = 6 }, logretention.ErrInvalid},
		{"上界", func(c *logretention.Change) { c.RetentionDays = 366 }, logretention.ErrInvalid},
		{"零值", func(c *logretention.Change) { c.RetentionDays = 0 }, logretention.ErrInvalid},
		{"缺确认", func(c *logretention.Change) { c.Confirmed = false }, logretention.ErrConfirmation},
		{"旧版本", func(c *logretention.Change) { c.ExpectedRevision = 2 }, logretention.ErrRevision},
		{"空原因", func(c *logretention.Change) { c.Reason = " " }, logretention.ErrInvalid},
		{"秘密原因", func(c *logretention.Change) { c.Reason = "password=synthetic-only-value" }, logretention.ErrInvalid},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db := permissionFixture(t)
			service, _ := logretention.New(db, 0)
			input := retentionChange()
			scenario.change(&input)
			if _, err := service.Change(context.Background(), input); !errors.Is(err, scenario.want) {
				t.Fatalf("拒绝=%v", err)
			}
			record, _ := db.ReadCurrentLogRetention(context.Background())
			if record.Revision != 1 || record.Days != nil {
				t.Fatal("失败部分保存")
			}
			assertCount(t, db.db, `SELECT COUNT(*) FROM request_idempotency WHERE operation='CHANGE_LOG_RETENTION'`, 0)
		})
	}
}

func Test保留策略同值不创建覆盖且审计失败回滚(t *testing.T) {
	db := permissionFixture(t)
	ctx := context.Background()
	service, _ := logretention.New(db, 60)
	input := retentionChange()
	input.RetentionDays = 60
	input.Confirmed = false
	result, err := service.Change(ctx, input)
	if err != nil || result.Changed || result.Revision != 1 {
		t.Fatalf("同值=%+v %v", result, err)
	}
	policy, _ := service.Read(ctx, "subject-1")
	if policy.Source != "DEPLOYMENT_CONFIG" || policy.ModifiedAt != nil {
		t.Fatal("同值改变来源")
	}
	if _, err = db.db.Exec(`CREATE TRIGGER synthetic_retention_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='LOG_RETENTION_CHANGED' BEGIN SELECT RAISE(ABORT,'synthetic audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	input = retentionChange()
	input.IdempotencyKey = "rollback-key"
	if _, err = service.Change(ctx, input); err == nil {
		t.Fatal("审计失败被忽略")
	}
	policy, _ = service.Read(ctx, "subject-1")
	if policy.Revision != 1 || policy.RetentionDays != 60 {
		t.Fatal("审计失败未回滚")
	}
	assertCount(t, db.db, `SELECT COUNT(*) FROM request_idempotency WHERE idempotency_key='rollback-key'`, 0)
}

func Test保留策略当前权限复验阻止重放和读取(t *testing.T) {
	db := permissionFixture(t)
	ctx := context.Background()
	service, _ := logretention.New(db, 0)
	if _, err := service.Change(ctx, retentionChange()); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []identity.Scope{identity.ScopeLogRetentionRead, identity.ScopeLogRetentionWrite} {
		if err := db.Authorize(ctx, identity.Principal{Type: identity.BrowserPrincipal, ID: "permission-user"}, scope, "platform"); err == nil {
			t.Fatal("普通用户取得设置权限")
		}
	}
	if _, err := db.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_SYSTEM_ADMIN'`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(ctx, retentionChange()); !errors.Is(err, logretention.ErrDenied) {
		t.Fatalf("撤权重放=%v", err)
	}
	if _, err := service.Read(ctx, "subject-1"); !errors.Is(err, logretention.ErrDenied) {
		t.Fatalf("撤权读取=%v", err)
	}
}

type observedRetentionBatch struct {
	policy    *logretention.Service
	completed chan error
}

func (o observedRetentionBatch) WithPolicy(ctx context.Context, batch func(int) error) error {
	err := o.policy.WithPolicy(ctx, batch)
	o.completed <- err
	return err
}

func Test保留设置实际进入清理器并只删除合成日志(t *testing.T) {
	ended := time.Now().UTC().AddDate(0, 0, -20)
	db, persistent, _, _ := retentionFixture(t, "SUCCEEDED", &ended, false)
	ctx := context.Background()
	if err := db.BootstrapPermissionOwner(ctx, "subject-1", testTime); err != nil {
		t.Fatal(err)
	}
	service, _ := logretention.New(db, 0)
	runBatch := func() {
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		observed := observedRetentionBatch{service, make(chan error, 1)}
		go func() { defer close(done); persistent.RunRetention(runCtx, observed, nil) }()
		if err := <-observed.completed; err != nil {
			t.Fatal(err)
		}
		cancel()
		<-done
	}
	runBatch()
	assertCount(t, db.db, `SELECT COUNT(*) FROM log_segments`, 1)
	if _, err := service.Change(ctx, retentionChange()); err != nil {
		t.Fatal(err)
	}
	runBatch()
	assertCount(t, db.db, `SELECT COUNT(*) FROM log_segments`, 0)
	assertCount(t, db.db, `SELECT COUNT(*) FROM tasks`, 1)
	assertCount(t, db.db, `SELECT COUNT(*) FROM audit_events WHERE action='LOG_RETENTION_CHANGED' AND result='SUCCEEDED'`, 1)
	// 缺失策略不得退回内置期限或清理新内容。
	if _, err := db.db.Exec(`DELETE FROM ordinary_log_retention_policy`); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := service.WithPolicy(ctx, func(int) error { called = true; return nil }); err == nil || called {
		t.Fatal("缺失策略仍执行清理")
	}
}
