package migrate

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"

	"ob-data-orch/migrations"
)

func Test任务展示事实迁移按当前名称回填已有任务(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	legacy := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name[:4] >= "0027" {
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
	var sourceName, environment, nodeName string
	if err := db.QueryRow(`SELECT ds.display_name, ds.environment, n.display_name FROM tasks t
        JOIN data_sources ds ON ds.data_source_id = t.data_source_id JOIN execution_nodes n ON n.node_id = t.node_id
        WHERE t.task_id = 'task-1'`).Scan(&sourceName, &environment, &nodeName); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	var frozenSource, frozenEnvironment, frozenNode sql.NullString
	if err := db.QueryRow(`SELECT data_source_name, environment, node_name FROM task_display_facts WHERE task_id = 'task-1'`).Scan(&frozenSource, &frozenEnvironment, &frozenNode); err != nil {
		t.Fatalf("已有任务未回填展示事实: %v", err)
	}
	if frozenSource.String != sourceName || frozenEnvironment.String != environment || frozenNode.String != nodeName {
		t.Fatalf("回填值与当前名称不一致: %v %v %v", frozenSource, frozenEnvironment, frozenNode)
	}
	if _, err := db.Exec(`UPDATE task_display_facts SET data_source_name = 'changed'`); err == nil {
		t.Fatal("展示事实应不可改写")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() || rows.Err() != nil {
		t.Fatal("展示事实外键不完整")
	}
}
