package importfiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/importdomain"
)

func Test官方单表删除建表前缀可提取列与权限事实(t *testing.T) {
	for _, prefix := range []string{"DROP TABLE IF EXISTS `synthetic_table`;\n", "-- 合成导出\nSET NAMES utf8mb4;\nDROP TABLE IF EXISTS `synthetic_db`.`synthetic_table`;\n/* 合成表 */\n", "drop table synthetic_table;\n"} {
		definition, ok := inspectTableDefinition(prefix+"CREATE TABLE `synthetic_table` (`id` BIGINT NOT NULL, `note` VARCHAR(30) DEFAULT 'synthetic', PRIMARY KEY (`id`), KEY synthetic_index (`note`), KEY `another_index` (`note`)) DEFAULT CHARSET=utf8mb4;", "synthetic_db", "synthetic_table")
		if !ok || !definition.dropBeforeCreate || !definition.permissionsKnown || len(definition.columns) != 2 || len(definition.required) != 1 {
			t.Fatalf("普通单表定义未提取正确的结构与删除事实: ok=%t drop=%t known=%t columns=%d required=%d", ok, definition.dropBeforeCreate, definition.permissionsKnown, len(definition.columns), len(definition.required))
		}
	}
	for _, text := range []string{"DROP TABLE other_table; CREATE TABLE synthetic_table(id INT);", "DROP TABLE synthetic_table, other_table; CREATE TABLE synthetic_table(id INT);", "DROP TABLE other_db.synthetic_table; CREATE TABLE synthetic_table(id INT);", "DROP TABLE synthetic_table; DROP TABLE synthetic_table; CREATE TABLE synthetic_table(id INT);", "CREATE TABLE synthetic_table(id INT); DROP TABLE synthetic_table;", "SET @dynamic = synthetic_func(); CREATE TABLE synthetic_table(id INT);", "/*! DROP TABLE other_table */ CREATE TABLE synthetic_table(id INT);"} {
		if _, ok := inspectTableDefinition(text, "synthetic_db", "synthetic_table"); ok {
			t.Fatal("外来对象、额外操作或自由会话表达式被当成已识别定义")
		}
	}
}

func Test复杂定义不据普通建表权限冒充完整验证(t *testing.T) {
	for _, text := range []string{"CREATE TABLE synthetic_table(id INT, parent_id INT REFERENCES parent_table(id));", "CREATE TABLE synthetic_table(id INT, computed INT AS (synthetic_function(id)));", "CREATE TABLE synthetic_table(id INT, computed INT AS (`synthetic_function`(id)));"} {
		definition, ok := inspectTableDefinition(text, "synthetic_db", "synthetic_table")
		if !ok || definition.permissionsKnown {
			t.Fatal("有外部对象或未知函数的定义未保持权限未知")
		}
	}
}

func Test仅DDL也保留逐表事实且定义不完整不会提升权限(t *testing.T) {
	for _, supported := range []bool{true, false} {
		root := t.TempDir()
		ddl := "DROP TABLE IF EXISTS synthetic_table; CREATE TABLE synthetic_table(id INT NOT NULL);"
		if !supported {
			ddl += "ALTER TABLE synthetic_table ADD note INT;"
		}
		if err := os.WriteFile(filepath.Join(root, "synthetic_table-schema.sql"), []byte(ddl), 0600); err != nil {
			t.Fatal(err)
		}
		c := importdomain.Config{FilePath: root, Content: "DDL_ONLY", Scope: "ALL", Cleanup: "NONE"}
		manifest, err := Scan(context.Background(), c, []string{root})
		if err != nil {
			t.Fatal(err)
		}
		plan, err := InspectMapping(context.Background(), c, manifest, []string{root})
		if err != nil || plan.DDLComplete != supported || len(plan.Tables) != 1 || !plan.Tables[0].HasDefinition || plan.Tables[0].HasData {
			t.Fatalf("DDL 事实没有独立于数据文件保留: supported=%t complete=%t tables=%+v err=%v", supported, plan.DDLComplete, plan.Tables, err)
		}
	}
}
