package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

func TestApplyCreatesStrictTwentyTableSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)

	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply() first run: %v", err)
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply() idempotent run: %v", err)
	}

	var tableCount, strictCount int
	err := db.QueryRowContext(ctx, `
        SELECT COUNT(*), COALESCE(SUM(strict), 0)
        FROM pragma_table_list
        WHERE schema = 'main' AND type = 'table' AND name NOT LIKE 'sqlite_%'
    `).Scan(&tableCount, &strictCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 20 || strictCount != 20 {
		t.Fatalf("schema tables = %d, strict tables = %d; want 20 and 20", tableCount, strictCount)
	}

	var migrationCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration count = %d, want 1", migrationCount)
	}

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign key check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key check returned a violation")
	}
}

func TestTasksAreImmutable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	insertSyntheticTaskFixture(t, db)

	if _, err := db.ExecContext(ctx, `UPDATE tasks SET planned_command_redacted = 'changed' WHERE task_id = 'task-1'`); err == nil {
		t.Fatal("updating an immutable task unexpectedly succeeded")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM tasks WHERE task_id = 'task-1'`); err == nil {
		t.Fatal("deleting an immutable task unexpectedly succeeded")
	}
}

func TestApplyRejectsChangedChecksum(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	tampered := fstest.MapFS{
		"0001_initial.sql": &fstest.MapFile{Data: []byte("CREATE TABLE tampered(value TEXT) STRICT;")},
	}
	if err := ApplyFS(ctx, db, tampered); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Apply() error = %v, want checksum mismatch", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = checksum WHERE version = 1`); err == nil {
		t.Fatal("updating immutable migration history unexpectedly succeeded")
	}
}

func TestApplyRejectsDisabledForeignKeys(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Apply(context.Background(), db); err == nil || !strings.Contains(err.Error(), "foreign_keys") {
		t.Fatalf("Apply() error = %v, want foreign_keys failure", err)
	}
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "metadata.db"))
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertSyntheticTaskFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	fingerprint := strings.Repeat("a", 64)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO auth_subjects VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, []any{"subject-1", "external-1", "Synthetic User", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO data_sources VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', ?, 2881, ?, ?, ?, 1, 'ENABLED', 1, NULL, NULL, NULL, ?, ?, ?)`, []any{"source-1", "Synthetic Source", "synthetic source", "127.0.0.1", "synthetic_user", "synthetic_db", "credential-1", "subject-1", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO credential_revisions VALUES (?, 1, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)`, []any{"credential-1", "source-1", "key-1", []byte{1, 2, 3}, []byte{4, 5, 6}, "2026-01-01T00:00:00Z"}},
		{`INSERT INTO execution_nodes VALUES (?, ?, ?, 'WINDOWS_AMD64', 'ENABLED', '[]', NULL, 1, ?, ?, ?)`, []any{"node-1", "Synthetic Node", "synthetic node", "subject-1", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO export_drafts VALUES (?, ?, ?, ?, 1, ?, ?, ?, '{}', ?, '[]', ?, ?)`, []any{"draft-1", "subject-1", "source-1", "node-1", "4.3.5-RELEASE", "obdumper-4.3.5-slice-v3", "export-odp-single-table-csv-v1", fingerprint, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO precheck_runs VALUES (?, ?, 1, ?, ?, ?, 1, ?, NULL, 'SUCCEEDED', NULL, NULL, NULL, '{}', 'COMPLETE', ?, ?, ?)`, []any{"precheck-1", "draft-1", fingerprint, "source-1", "credential-1", "node-1", "2026-01-01T01:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:01:00Z"}},
		{`INSERT INTO tasks VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, '{}', '[]', ?, ?)`, []any{"task-1", "subject-1", "source-1", "node-1", "precheck-1", "credential-1", fingerprint, "4.3.5-RELEASE", "obdumper-4.3.5-slice-v3", "export-odp-single-table-csv-v1", "obdumper --user ******", "2026-01-01T00:02:00Z"}},
	}
	for index, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("insert synthetic fixture statement %d: %v", index+1, err)
		}
	}
}
