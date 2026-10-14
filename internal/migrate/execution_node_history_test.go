package migrate

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"ob-data-orch/migrations"
)

func Test节点历史解耦迁移保留配置和任务并允许删除(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	legacy := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name[:4] >= "0025" {
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
		`INSERT INTO agents(agent_id,node_id,credential_digest,credential_revision,status,protocol_version,created_at) VALUES ('agent-1','node-1',X'010203',1,'REVOKED','agent-v1','2026-01-01T00:00:00Z')`,
		`INSERT INTO task_executions(execution_id,task_id,node_id,agent_id,state,revision,created_at,updated_at) VALUES ('execution-1','task-1','node-1','agent-1','SUCCEEDED',1,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
		`INSERT INTO execution_resource_allocations VALUES ('execution-1',2,3,'2026-01-01T00:00:00Z')`,
		`INSERT INTO execution_claim_receipts VALUES ('agent-1','request-1','synthetic-digest','{}')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE execution_nodes SET tool_home='E:\synthetic\tools',java_path='E:\synthetic\java.exe',execution_budget_policy_json='{"systemReservedBytes":1,"taskOverheadBytes":2}',environment_check_status='PASSED',environment_check_code='TOOL_RUNTIME_READY' WHERE node_id='node-1'`); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	var home, java, policy, environment string
	if err := db.QueryRow(`SELECT tool_home,java_path,execution_budget_policy_json,environment_check_status FROM execution_nodes WHERE node_id='node-1'`).Scan(&home, &java, &policy, &environment); err != nil {
		t.Fatal(err)
	}
	if home != `E:\synthetic\tools` || java != `E:\synthetic\java.exe` || policy != `{"systemReservedBytes":1,"taskOverheadBytes":2}` || environment != "PASSED" {
		t.Fatal("升级改写了节点配置")
	}
	var columns int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('execution_node_references')`).Scan(&columns); err != nil || columns != 1 {
		t.Fatalf("历史 anchor 留下配置副本：columns=%d err=%v", columns, err)
	}
	if _, err := db.Exec(`DELETE FROM execution_nodes WHERE node_id='node-1'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"tasks", "precheck_runs", "export_drafts"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("历史 %s 未保留：%d %v", table, count, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM tasks`); err == nil {
		t.Fatal("任务不可变约束丢失")
	}
	for _, table := range []string{"execution_resource_allocations", "execution_claim_receipts"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("预算证据 %s 未保留：%d %v", table, count, err)
		}
		if _, err := db.Exec("DELETE FROM " + table); err == nil {
			t.Fatalf("预算证据 %s 可变", table)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() || rows.Err() != nil {
		t.Fatal("历史外键不完整")
	}
}
