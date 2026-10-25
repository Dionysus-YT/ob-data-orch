package importfiles

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ob-data-orch/internal/importdomain"
)

func Test受控压缩映射完整摘要与损坏负例(t *testing.T) {
	for _, codec := range []string{"gzip", "zlib"} {
		t.Run(codec, func(t *testing.T) {
			root := t.TempDir()
			var content bytes.Buffer
			if codec == "gzip" {
				writer := gzip.NewWriter(&content)
				_, _ = writer.Write([]byte("1,alpha,plain\n"))
				_ = writer.Close()
			} else {
				writer := zlib.NewWriter(&content)
				_, _ = writer.Write([]byte("1,alpha,plain\n"))
				_ = writer.Close()
			}
			suffix := map[string]string{"gzip": ".gz", "zlib": ".zz"}[codec]
			path := filepath.Join(root, "synthetic_table.csv"+suffix)
			if err := os.WriteFile(path, content.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			c := importdomain.Config{FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Parser: importdomain.Parser{Compress: true, CompressionAlgo: codec, ColumnQuote: `"`}}
			m, err := Scan(context.Background(), c, []string{root})
			if err != nil {
				t.Fatal(err)
			}
			p, err := InspectMapping(context.Background(), c, m, []string{root})
			if err != nil || !p.Complete || p.Tables[0].Columns != 3 {
				t.Fatal("标准压缩流未正确核验", p, err)
			}
			broken := append([]byte(nil), content.Bytes()...)
			broken[len(broken)-1] ^= 1
			_ = os.WriteFile(path, broken, 0600)
			m, err = Scan(context.Background(), c, []string{root})
			if err != nil {
				t.Fatal(err)
			}
			p, err = InspectMapping(context.Background(), c, m, []string{root})
			if err != nil || p.Complete {
				t.Fatal("损坏压缩流获得通过", p, err)
			}
			c.Parser.CompressionAlgo = "zstd"
			p, err = InspectMapping(context.Background(), c, m, []string{root})
			if err == nil && p.Complete {
				t.Fatal("未识别压缩被猜测通过")
			}
		})
	}
}

func Test控制文件仅允许固定列映射并冻结辅助摘要(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "synthetic_table.csv"), []byte("alpha,1,plain\n"), 0600)
	control := filepath.Join(root, "synthetic_table.ctrl")
	_ = os.WriteFile(control, []byte(`lang=java (id "NONE" map(2), label "NONE" map(1), note "NONE" map(3));`), 0600)
	c := importdomain.Config{FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Mapping: &importdomain.Mapping{Kind: "CONTROL", CtlPath: root}}
	m, err := Scan(context.Background(), c, []string{root})
	if err != nil || m.ControlDigest == "" {
		t.Fatal(err)
	}
	p, err := InspectMapping(context.Background(), c, m, []string{root})
	if err != nil || !p.Complete || p.Tables[0].Names[0] != "label" || p.Tables[0].Names[1] != "id" {
		t.Fatal("控制映射未对应源位置", p, err)
	}
	_ = os.WriteFile(control, []byte(`lang=java (id, label, note);`), 0600)
	newManifest, err := Scan(context.Background(), c, []string{root})
	if err != nil || newManifest.Digest == m.Digest {
		t.Fatal("控制文件变化未绑定任务清单")
	}
	if _, err := InspectMapping(context.Background(), c, m, []string{root}); err == nil {
		t.Fatal("旧控制摘要被接受")
	}
	for _, bad := range []string{`lang=java (id "CONSTANT('synthetic')");`, `lang=java (id map(2));`, `lang=java (id map(1),note map(1));`, `lang=java (id,id);`, `lang=java (id map(1),note);`, `lang=java (id); SELECT 1;`} {
		if _, ok := parseControl([]byte(bad)); ok {
			t.Fatal("未知或有副作用的控制内容未阻断")
		}
	}
}

func Test压缩尾随内容解压上限及取消均失败关闭(t *testing.T) {
	for _, variant := range []string{"tail", "limit", "cancel"} {
		t.Run(variant, func(t *testing.T) {
			root := t.TempDir()
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			if variant == "limit" {
				block := bytes.Repeat([]byte("a"), 1024*1024)
				for i := 0; i < 33; i++ {
					_, _ = writer.Write(block)
				}
			} else {
				_, _ = writer.Write([]byte("1,alpha\n"))
			}
			_ = writer.Close()
			if variant == "tail" {
				compressed.WriteString("unverified-tail")
			}
			_ = os.WriteFile(filepath.Join(root, "synthetic.csv.gz"), compressed.Bytes(), 0600)
			c := importdomain.Config{FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Parser: importdomain.Parser{Compress: true, CompressionAlgo: "gzip"}}
			m, err := Scan(context.Background(), c, []string{root})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if variant == "cancel" {
				cancel()
			}
			p, err := InspectMapping(ctx, c, m, []string{root})
			if err == nil && p.Complete {
				t.Fatal("未核验输入或取消后仍获得通过")
			}
		})
	}
}

func Test尾行跳过保留首条数据且不把唯一尾行当数据(t *testing.T) {
	for _, test := range []struct {
		text   string
		header bool
		pass   bool
	}{{"1|alpha\nfooter\n", false, true}, {"footer\n", false, false}, {"id|note\nfooter\n", true, false}, {"id|note\n1|alpha\nfooter\n", true, true}} {
		c := importdomain.Config{Format: "CUT", Parser: importdomain.Parser{LineSeparator: `\n`, SkipFooter: true, SkipHeader: test.header}}
		_, ok := withoutFooter([]byte(test.text), true, c)
		if ok != test.pass {
			t.Fatal("尾行结构判断错误")
		}
	}
}

func Test非表对象声明逐类识别且拒绝越界与尾随SQL(t *testing.T) {
	for _, test := range []struct{ kind, sql string }{
		{"table-group", "CREATE TABLEGROUP synthetic_object;"}, {"view", "CREATE VIEW synthetic_object AS SELECT 1 AS ID;"},
		{"trigger", "CREATE TRIGGER synthetic_object BEFORE INSERT ON synthetic_base FOR EACH ROW SET NEW.ID=NEW.ID;"},
		{"trigger", "DELIMITER /\nCREATE TRIGGER synthetic_object BEFORE INSERT ON synthetic_base FOR EACH ROW BEGIN :NEW.ID := :NEW.ID; END;\n/\nDELIMITER ;"},
		{"obj-user", "CREATE USER synthetic_object@'0.0.0.0';"}, {"role", "CREATE ROLE synthetic_object;"},
		{"sequence", "CREATE SEQUENCE synthetic_object START WITH 1 INCREMENT BY 1 NOCACHE;"}, {"synonym", "CREATE SYNONYM synthetic_object FOR synthetic_base;"},
		{"type", "CREATE TYPE synthetic_object AS OBJECT (ID NUMBER, MEMBER FUNCTION value1 RETURN NUMBER);"},
		{"type-body", "CREATE TYPE BODY synthetic_object AS MEMBER FUNCTION value1 RETURN NUMBER IS BEGIN RETURN 1; END; END; /"},
		{"package", "CREATE PACKAGE synthetic_object AS PROCEDURE p; END; /"},
		{"package-body", "CREATE PACKAGE BODY synthetic_object AS PROCEDURE p IS BEGIN NULL; END; END; /"},
		{"function", "CREATE FUNCTION synthetic_object() RETURNS INT RETURN 1;"}, {"procedure", "CREATE PROCEDURE synthetic_object() BEGIN END;"},
		{"procedure", "CREATE PROCEDURE synthetic_object() SELECT 1;"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			if test.kind == "type-body" || test.kind == "package" || test.kind == "package-body" || test.kind == "procedure" && strings.Contains(test.sql, " BEGIN") {
				test.sql = "DELIMITER /\n" + strings.TrimSuffix(test.sql, " /") + "\n/\nDELIMITER ;"
			}
			c := importdomain.Config{Content: "DDL_ONLY", Scope: "SPECIFIC", Objects: map[string]string{test.kind: "synthetic_object"}}
			object, ok := inspectObjectDefinition(test.sql, "synthetic_object", c)
			if !ok || object.Kind != test.kind {
				t.Fatal("声明未识别", object)
			}
			for _, bad := range []string{test.sql + " DROP TABLE other_table;", test.sql + " GRANT SELECT ON other_table TO other_user;"} {
				if _, ok := inspectObjectDefinition(bad, "synthetic_object", c); ok {
					t.Fatal("尾随 SQL 被放行")
				}
			}
			if _, ok := inspectObjectDefinition(test.sql, "other_object", c); ok {
				t.Fatal("对象名漂移未拒绝")
			}
		})
	}
	c := importdomain.Config{Scope: "SPECIFIC", Objects: map[string]string{"obj-user": "synthetic_object"}}
	for _, sql := range []string{"CREATE USER synthetic_object;", "CREATE USER synthetic_object@'%';", "CREATE USER synthetic_object IDENTIFIED BY 'synthetic-rejected';"} {
		if _, ok := inspectObjectDefinition(sql, "synthetic_object", c); ok {
			t.Fatal("可登录或含秘密的账号定义未阻断")
		}
	}
}
