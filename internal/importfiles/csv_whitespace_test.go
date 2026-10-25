package importfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ob-data-orch/internal/importdomain"
)

func TestCSV全流空白防护跨块且不改写引用值(t *testing.T) {
	for _, tc := range []struct {
		name, input   string
		loss, invalid bool
	}{
		{"前空格", "1, padded\n", true, false},
		{"后空格", "1,padded \n", true, false},
		{"空白值", "1, \n", true, false},
		{"内部空格", "1,pad ded\n", false, false},
		{"引用空格", "1,\" padded \"\n", false, false},
		{"引用多行与反斜杠", "1,\"comma,quote\"\"backslash\\\",\"first\nsecond\",\" \"\n", false, false},
		{"未闭合", "1,\"unfinished", false, true},
		{"没有末尾换行", "1,padded ", true, false},
		{"BOM与回车换行", "\xef\xbb\xbf1,\" padded \"\r\n2,plain\r\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for chunk := 1; chunk <= 9; chunk++ {
				guard := newCSVWhitespaceGuard(",", "\"", "", false)
				for i := 0; i < len(tc.input); i += chunk {
					guard.feed([]byte(tc.input[i:min(i+chunk, len(tc.input))]), false)
				}
				guard.feed(nil, true)
				if guard.loss != tc.loss || guard.invalid != tc.invalid || len(guard.pending) != 0 {
					t.Fatalf("跨块扫描不一致：chunk=%d loss=%v invalid=%v", chunk, guard.loss, guard.invalid)
				}
			}
		})
	}
}

func TestCSV空白检查覆盖前缀之外并遵守显式Trim(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "synthetic.csv")
	data := "id,label\n" + strings.Repeat("1,synthetic\n", 30000) + "2, padded \n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c := importdomain.Config{FilePath: root, Database: "synthetic", Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE", Parser: importdomain.Parser{SkipHeader: true, ColumnQuote: "\""}}
	manifest, err := Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := InspectMapping(context.Background(), c, manifest, []string{root})
	if err != nil || !plan.CSVWhitespaceLoss || plan.Complete {
		t.Fatal("尾部空白没有失败关闭", err)
	}
	c.Parser.WithTrim = true
	plan, err = InspectMapping(context.Background(), c, manifest, []string{root})
	if err != nil || plan.CSVWhitespaceLoss || !plan.Complete {
		t.Fatal("显式删除空白仍被拒绝", err)
	}
}
