package migrate

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"ob-data-orch/migrations"
)

func Test固定复验授权迁移保留旧身份与材料且默认不扩大权限(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	legacy := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name[:4] >= "0031" {
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
	for _, statement := range []string{
		`INSERT INTO agents(agent_id,node_id,credential_digest,credential_revision,status,protocol_version,created_at) VALUES ('agent-1','node-1',X'010203',1,'ACTIVE','agent-v1','2026-01-01T00:00:00Z')`,
		`INSERT INTO agent_enrollment_tokens(enrollment_id,node_id,token_digest,status,expires_at,created_by,created_at) VALUES ('enrollment-1','node-1',X'040506','ACTIVE','2027-01-01T00:00:00Z','subject-1','2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"agents", "agent_enrollment_tokens"} {
		var count, granted int
		if err := db.QueryRow("SELECT COUNT(*),SUM(environment_inspection_authorized) FROM "+table).Scan(&count, &granted); err != nil || count != 1 || granted != 0 {
			t.Fatal("迁移丢失旧绑定或默认为持续授权", table, err)
		}
		if _, err := db.Exec("UPDATE " + table + " SET environment_inspection_authorized=2"); err == nil {
			t.Fatal("授权字段接受非布尔值", table)
		}
	}
	var credential, token string
	if err := db.QueryRow(`SELECT hex(credential_digest) FROM agents WHERE agent_id='agent-1'`).Scan(&credential); err != nil || credential != "010203" {
		t.Fatal("迁移改写机器身份", err)
	}
	if err := db.QueryRow(`SELECT hex(token_digest) FROM agent_enrollment_tokens WHERE enrollment_id='enrollment-1'`).Scan(&token); err != nil || token != "040506" {
		t.Fatal("迁移改写关联材料", err)
	}
}
