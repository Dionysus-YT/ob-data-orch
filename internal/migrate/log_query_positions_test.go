package migrate

import (
	"testing"

	"ob-data-orch/migrations"
)

// Test日志查询水位迁移保留旧批次且清理后不重用 验证旧批次仅回填索引、级联清理与单调水位。
func Test日志查询水位迁移保留旧批次且清理后不重用(t *testing.T) {
	db := openTestDatabase(t)
	if _, err := db.Exec(`CREATE TABLE log_batches(batch_id TEXT PRIMARY KEY,received_at TEXT NOT NULL) STRICT,WITHOUT ROWID;
        INSERT INTO log_batches VALUES('old-b','2026-10-25T00:00:01Z'),('old-a','2026-10-25T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	source, err := migrations.Files.ReadFile("0040_log_query_positions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(source)); err != nil {
		t.Fatal(err)
	}
	var first int
	var count int
	if err = db.QueryRow(`SELECT sequence FROM log_query_positions WHERE batch_id='old-a'`).Scan(&first); err != nil || first != 1 {
		t.Fatal("旧批次水位错误", err)
	}
	if _, err = db.Exec(`DELETE FROM log_batches WHERE batch_id='old-b';INSERT INTO log_batches VALUES('new-c','2026-10-24T00:00:00Z');INSERT INTO log_query_positions(batch_id)VALUES('new-c')`); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT sequence FROM log_query_positions WHERE batch_id='new-c'`).Scan(&first); err != nil || first != 3 {
		t.Fatal("晚到批次重用了水位", err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM log_query_positions`).Scan(&count); err != nil || count != 2 {
		t.Fatal("级联清理失败", err)
	}
}
