package importfiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/importdomain"
)

func TestCUT换行严格匹配与平台默认不修改输入配置(t *testing.T) {
	for _, item := range []struct {
		name, platform, configured, content string
		complete                            bool
	}{
		{"Windows默认拒绝LF尾分隔", "WINDOWS_AMD64", "", "1|alpha|plain|\n", false},
		{"Windows默认接受CRLF尾分隔", "WINDOWS_AMD64", "", "1|alpha|plain|\r\n", true},
		{"Windows显式LF", "WINDOWS_AMD64", `\n`, "1|alpha|plain|\n", true},
		{"LinuxAMD64默认LF", "LINUX_AMD64", "", "1|alpha|plain|\n", true},
		{"LinuxARM64默认LF", "LINUX_ARM64", "", "1|alpha|plain|\n", true},
		{"显式CRLF拒绝LF", "WINDOWS_AMD64", `\r\n`, "1|alpha|plain|\n", false},
		{"显式LF拒绝CRLF残留", "WINDOWS_AMD64", `\n`, "1|alpha|plain|\r\n", false},
	} {
		t.Run(item.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "synthetic_table.dat"), []byte(item.content), 0600); err != nil {
				t.Fatal(err)
			}
			c := importdomain.Config{FilePath: root, Content: "DATA_ONLY", Format: "CUT", Scope: "ALL", Parser: importdomain.Parser{ColumnSplitter: "|", TrailDelimiter: true, LineSeparator: item.configured}, Cleanup: "NONE"}
			manifest, err := Scan(context.Background(), c, []string{root})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := InspectMappingForPlatform(context.Background(), c, manifest, []string{root}, item.platform)
			if err != nil || plan.Complete != item.complete {
				t.Fatalf("换行判定不符：%v %v", plan, err)
			}
			if plan.Complete && (len(plan.Tables) != 1 || plan.Tables[0].Columns != 3) {
				t.Fatal("尾分隔列数不符")
			}
			if c.Parser.LineSeparator != item.configured {
				t.Fatal("检查改写了冻结配置")
			}
		})
	}
	if _, _, ok := delimitedColumns([]byte("1|alpha|plain|\n"), true, importdomain.Config{Format: "CUT", Parser: importdomain.Parser{ColumnSplitter: "|", TrailDelimiter: true}}); ok {
		t.Fatal("未知执行平台时猜测了CUT默认换行")
	}
}
