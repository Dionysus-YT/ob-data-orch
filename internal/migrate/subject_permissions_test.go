package migrate

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"

	"ob-data-orch/migrations"
)

func Test权限迁移保留身份范围与历史不可变任务(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	legacy := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name[:4] >= "0035" {
			continue
		}
		data, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		legacy[name] = &fstest.MapFile{Data: data}
	}
	if err := ApplyFS(ctx, db, legacy); err != nil {
		t.Fatal(err)
	}
	insertSyntheticTaskFixture(t, db)
	if _, err := db.Exec(`INSERT INTO subject_object_scopes(subject_id,scope_type,object_id,granted_by,granted_at) VALUES('subject-1','NODE_USE','node-1','subject-1','2026-10-23T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	var identityBefore string
	if err := db.QueryRow(`SELECT external_subject FROM auth_subjects WHERE subject_id='subject-1'`).Scan(&identityBefore); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	var identityAfter string
	var revision, owner, count int
	if err := db.QueryRow(`SELECT a.external_subject,v.revision,v.deployment_owner FROM auth_subjects a JOIN subject_permission_versions v USING(subject_id) WHERE a.subject_id='subject-1'`).Scan(&identityAfter, &revision, &owner); err != nil || identityAfter != identityBefore || revision != 1 || owner != 0 {
		t.Fatal("迁移改变认证或授予部署身份", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM subject_object_scopes WHERE subject_id='subject-1' AND scope_type='NODE_USE' AND object_id='node-1'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("原有范围丢失", err)
	}
	var frozenRevision sql.NullInt64
	var frozenScope sql.NullString
	if err := db.QueryRow(`SELECT submitted_permission_revision,submitted_permission_scope_json FROM tasks WHERE task_id='task-1'`).Scan(&frozenRevision, &frozenScope); err != nil || frozenRevision.Valid || frozenScope.Valid {
		t.Fatal("伪造回填历史授权", err)
	}
	if _, err := db.Exec(`UPDATE tasks SET submitted_permission_revision=1 WHERE task_id='task-1'`); err == nil {
		t.Fatal("任务不可变保护丢失")
	}
	if _, err := db.Exec(`INSERT INTO subject_object_scopes VALUES('subject-1','TEMPLATE_USE','synthetic-template','subject-1','2026-10-23T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO subject_object_scopes VALUES('subject-1','UNKNOWN_SCOPE','synthetic','subject-1','2026-10-23T00:00:00Z')`); err == nil {
		t.Fatal("未知范围未阻断")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() || rows.Err() != nil {
		t.Fatal("迁移外键损坏")
	}
}
