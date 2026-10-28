package migrate

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"ob-data-orch/migrations"
)

func initialAuditLegacy(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDatabase(t)
	legacy := fstest.MapFS{}
	files, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() >= "0042_" {
			continue
		}
		data, err := migrations.Files.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		legacy[file.Name()] = &fstest.MapFile{Data: data}
	}
	if err := ApplyFS(context.Background(), db, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auth_subjects(subject_id,external_subject,display_name,account_status,created_at,updated_at) VALUES('synthetic-admin','synthetic-admin','合成管理员','ACTIVE','2026-10-26T00:00:00Z','2026-10-26T00:00:00Z'); INSERT INTO subject_permission_versions(subject_id,revision,deployment_owner) VALUES('synthetic-admin',7,1)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func Test首次审计迁移不授予且保留曾经授予证据(t *testing.T) {
	for _, name := range []string{"从未授予", "当前已有", "历史已撤销"} {
		t.Run(name, func(t *testing.T) {
			db := initialAuditLegacy(t)
			if name == "当前已有" {
				if _, err := db.Exec(`INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES('synthetic-admin','CAP_AUDIT_LOG','synthetic-admin','2026-10-26T00:00:00Z')`); err != nil {
					t.Fatal(err)
				}
			}
			if name == "历史已撤销" {
				if _, err := db.Exec(`INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES('synthetic-revoke','SUBJECT','synthetic-admin','SUBJECT_PERMISSIONS_CHANGED','SUBJECT','synthetic-admin','SUCCEEDED','synthetic-revoke','{"before":{"capabilities":["CAP_AUDIT_LOG"]},"after":{"capabilities":[]}}','2026-10-26T00:00:00Z')`); err != nil {
					t.Fatal(err)
				}
			}
			if err := Apply(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			var consumed, revision, grants int
			if err := db.QueryRow(`SELECT audit_bootstrap_consumed,revision,(SELECT count(*) FROM subject_grants WHERE grant_code='CAP_AUDIT_LOG') FROM subject_permission_versions WHERE subject_id='synthetic-admin'`).Scan(&consumed, &revision, &grants); err != nil {
				t.Fatal(err)
			}
			wantConsumed, wantGrants := 1, 0
			if name == "从未授予" {
				wantConsumed = 0
			}
			if name == "当前已有" {
				wantGrants = 1
			}
			if consumed != wantConsumed || grants != wantGrants || revision != 7 {
				t.Fatal("升级扩大权限或丢失历史", consumed, grants, revision)
			}
		})
	}
}

func Test首次审计迁移失败不留下部分列或历史(t *testing.T) {
	db := initialAuditLegacy(t)
	if _, err := db.Exec(`INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES('synthetic-admin','CAP_AUDIT_LOG','synthetic-admin','2026-10-26T00:00:00Z'); CREATE TRIGGER initial_audit_migration_failure BEFORE UPDATE ON subject_permission_versions BEGIN SELECT RAISE(ABORT,'合成迁移失败'); END`); err != nil {
		t.Fatal(err)
	}
	if err := Apply(context.Background(), db); err == nil {
		t.Fatal("失败迁移仍通过")
	}
	var columns, versions int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('subject_permission_versions') WHERE name='audit_bootstrap_consumed'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=42`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if columns != 0 || versions != 0 {
		t.Fatal("失败迁移留下部分结构或历史")
	}
}
