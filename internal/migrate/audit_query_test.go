package migrate

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"ob-data-orch/migrations"
)

// Test审计水位迁移保留历史及原子追加 验证 40→41 回填、旧生产端自动登记及事务回滚不留孤儿。
func Test审计水位迁移保留历史及原子追加(t *testing.T) {
	ctx := context.Background()
	db := openTestDatabase(t)
	legacy := fstest.MapFS{}
	files, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "0041_") {
			continue
		}
		data, err := migrations.Files.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		legacy[file.Name()] = &fstest.MapFile{Data: data}
	}
	if err = ApplyFS(ctx, db, legacy); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SYSTEM','synthetic-system','LOG_RETENTION_CHANGED','PLATFORM_SETTING','ordinary-task-logs','SUCCEEDED','synthetic-request','{}','2026-10-25T00:00:00Z')`
	if _, err = db.ExecContext(ctx, insert, "old-audit"); err != nil {
		t.Fatal(err)
	}
	if err = Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	var oldPosition, newPosition int64
	if err = db.QueryRowContext(ctx, `SELECT sequence FROM audit_query_positions WHERE audit_event_id='old-audit'`).Scan(&oldPosition); err != nil || oldPosition < 1 {
		t.Fatal("历史事件未回填", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, insert, "rolled-back-audit"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_query_positions WHERE audit_event_id='rolled-back-audit'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("回滚留下孤儿次序", err)
	}
	if _, err = db.ExecContext(ctx, insert, "new-audit"); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT sequence FROM audit_query_positions WHERE audit_event_id='new-audit'`).Scan(&newPosition); err != nil || newPosition <= oldPosition {
		t.Fatal("新事件未自动追加水位", err)
	}
}
