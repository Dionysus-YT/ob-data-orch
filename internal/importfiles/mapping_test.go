package importfiles

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ob-data-orch/internal/importdomain"
)

func Test有界前缀切断UTF8尾字符不影响前面的完整记录(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "synthetic_table.csv"), []byte("1,'合成'\n"+strings.Repeat("合", 100000)), 0600); err != nil {
		t.Fatal(err)
	}
	c := importdomain.Config{FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE"}
	manifest, err := Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := InspectMapping(context.Background(), c, manifest, []string{root})
	if err != nil || !plan.Complete || len(plan.Tables) != 1 || plan.Tables[0].Columns != 2 {
		t.Fatal("字符边界导致有效首条记录无法检查", err)
	}
}

func Test映射只提取有界列结构并复核原清单摘要(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "synthetic_table.csv")
	if err := os.WriteFile(path, []byte("1,'synthetic,value'\n2,'synthetic'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := importdomain.Config{FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE", Mapping: &importdomain.Mapping{Kind: "POSITION"}}
	m, err := Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := InspectMapping(context.Background(), c, m, []string{root})
	if err != nil || !plan.Complete || len(plan.Tables) != 1 || plan.Tables[0].Columns != 2 || plan.Tables[0].Name != "synthetic_table" || len(plan.Tables[0].Names) != 0 {
		t.Fatal("列结构未正确提取", err)
	}
	info, _ := os.Stat(path)
	if err := os.WriteFile(path, []byte("3,'synthetic,value'\n4,'synthetic'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectMapping(context.Background(), c, m, []string{root}); err == nil {
		t.Fatal("相同长度与时间的输入变化未被摘要校验拒绝")
	}
}

func Test三格式首条结构与CSV列名映射(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		whole   bool
		config  importdomain.Config
		count   int
		names   []string
	}{
		{name: "CSV默认单引号", content: "1,'synthetic,a'\n", whole: true, config: importdomain.Config{Format: "CSV"}, count: 2},
		{name: "CSV多字符分隔与引号内换行", content: "1||'synthetic\nvalue'\n", whole: true, config: importdomain.Config{Format: "CSV", Parser: importdomain.Parser{ColumnSeparator: "||"}}, count: 2},
		{name: "CSV自动列名", content: "'id','note'\n1,'synthetic'\n", whole: true, config: importdomain.Config{Format: "CSV", Mapping: &importdomain.Mapping{Kind: "AUTO"}}, count: 2, names: []string{"id", "note"}},
		{name: "CUT显式分隔与尾分隔", content: "1|##|synthetic|##|\n", whole: true, config: importdomain.Config{Format: "CUT", Parser: importdomain.Parser{ColumnSplitter: "|##|", TrailDelimiter: true}}, count: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			count, names, ok := delimitedColumns([]byte(test.content), test.whole, test.config)
			if !ok || count != test.count || !reflect.DeepEqual(names, test.names) {
				t.Fatal("合成文件列结构不符合预期")
			}
		})
	}
	for _, input := range []string{"INSERT INTO `synthetic_table` (`id`,`note`) VALUES (1,'synthetic,a');", "-- synthetic\nINSERT INTO `synthetic_db`.`synthetic_table` (`id`,`note`) VALUES (1,concat('a', 'b'));"} {
		count, names, ok := insertColumns(input, "synthetic_db", "synthetic_table")
		if !ok || count != 2 || !reflect.DeepEqual(names, []string{"id", "note"}) {
			t.Fatal("SQL只读结构识别失败")
		}
	}
	for _, input := range []string{"DELETE FROM synthetic_table;", "INSERT INTO other_table VALUES(1,2);", "INSERT INTO synthetic_table VALUES('unterminated", "INSERT INTO synthetic_table VALUES();", "INSERT INTO synthetic_table VALUES(1,,2);", "INSERT INTO synthetic_table VALUES(1,);"} {
		if _, _, ok := insertColumns(input, "synthetic_db", "synthetic_table"); ok {
			t.Fatal("未知或非当前目标的SQL结构被接受")
		}
	}
	if _, _, ok := delimitedColumns([]byte("1,'unterminated"), false, importdomain.Config{Format: "CSV"}); ok {
		t.Fatal("截断记录被当成完整结构")
	}
	if _, _, ok := delimitedColumns([]byte("1|synthetic\n"), true, importdomain.Config{Format: "CUT"}); ok {
		t.Fatal("未核实的CUT默认分隔被猜测")
	}
}

func Test新建表与替换表的DDL结构来自同名冻结文件(t *testing.T) {
	ddl := "CREATE TABLE `synthetic_table` (`id` BIGINT AUTO_INCREMENT, `note` VARCHAR(20) DEFAULT 'synthetic', `computed` INT AS (`id`+1) VIRTUAL, CONSTRAINT `pk` PRIMARY KEY (`id`));"
	columns, required, ok := definitionColumns(ddl, "synthetic_db", "synthetic_table")
	if !ok || !reflect.DeepEqual(columns, []string{"id", "note"}) || len(required) != 0 {
		t.Fatal("DDL默认值、计算列与主键提取错误")
	}
	columns, required, ok = definitionColumns("CREATE TABLE synthetic_table (id INT, note VARCHAR(20) NOT NULL, PRIMARY KEY(id));", "synthetic_db", "synthetic_table")
	if !ok || len(columns) != 2 || len(required) != 2 {
		t.Fatal("必填列提取错误")
	}
	for _, input := range []string{"DROP TABLE other_table; CREATE TABLE synthetic_table(id INT);", "CREATE TABLE other_table(id INT);", "CREATE TABLE synthetic_table(id INT); ALTER TABLE synthetic_table ADD note INT;", "CREATE TABLE synthetic_table(id INT); DROP TABLE synthetic_table;"} {
		if _, _, ok := definitionColumns(input, "synthetic_db", "synthetic_table"); ok {
			t.Fatal("未核实的DDL结构被接受")
		}
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "synthetic_table.csv"), []byte("1,'synthetic'\n"), 0o600)
	os.WriteFile(filepath.Join(root, "synthetic_table-schema.sql"), []byte(ddl), 0o600)
	c := importdomain.Config{FilePath: root, Content: "DDL_AND_DATA", Format: "CSV", Scope: "ALL", Cleanup: "NONE"}
	m, err := Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := InspectMapping(context.Background(), c, m, []string{root})
	if err != nil || !plan.Complete || len(plan.Tables) != 1 || len(plan.Tables[0].TargetColumns) != 2 {
		t.Fatal("冻结DDL结构未与数据文件关联", err)
	}
}
