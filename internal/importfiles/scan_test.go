package importfiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/importdomain"
)

func Test输入清单按格式内容筛选并可复核(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"synthetic_table.csv", "synthetic_table.0.csv", "synthetic_table-schema.sql", "synthetic_table.dat", "ignored.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c := importdomain.Config{FilePath: root, Content: "DDL_AND_DATA", Format: "CSV", Scope: "ALL"}
	m, err := Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if m.FileCount != 3 || !m.Valid(c) {
		t.Fatal("清单未准确包含定义和数据")
	}
	m.Files[0].SHA256 = "bad"
	if m.Valid(c) {
		t.Fatal("篡改清单未拒绝")
	}
	c.Content = "DATA_ONLY"
	c.Format = "CUT"
	m, err = Scan(context.Background(), c, []string{root})
	if err != nil || m.FileCount != 1 {
		t.Fatal("CUT 默认后缀应为 dat")
	}
}

func Test辅助清单变化使冻结摘要失效且不冒充数据文件(t *testing.T) {
	root := t.TempDir()
	c := importdomain.Config{FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL"}
	_ = os.WriteFile(filepath.Join(root, "synthetic.csv"), []byte("1,synthetic\n"), 0600)
	_ = os.WriteFile(filepath.Join(root, ".MANIFEST.bin"), []byte("synthetic-metadata"), 0600)
	first, err := Scan(context.Background(), c, []string{root})
	if err != nil || first.FileCount != 1 || first.MetadataDigest == "" || !first.Valid(c) {
		t.Fatal("辅助清单未绑定", err)
	}
	_ = os.WriteFile(filepath.Join(root, ".MANIFEST.bin"), []byte("changed-metadata"), 0600)
	second, err := Scan(context.Background(), c, []string{root})
	if err != nil || second.Digest == first.Digest {
		t.Fatal("辅助清单变化未使证据失效", err)
	}
	first.MetadataDigest = "invalid"
	if first.Valid(c) {
		t.Fatal("损坏辅助摘要被接受")
	}
}

func Test输入越界空匹配取消与链接失败关闭(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	c := importdomain.Config{FilePath: outside, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL"}
	if _, err := Scan(context.Background(), c, []string{root}); err == nil {
		t.Fatal("允许根目录被绕过")
	}
	c.FilePath = root
	if _, err := Scan(context.Background(), c, []string{root}); err == nil {
		t.Fatal("空匹配被接受")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, c, []string{root}); err == nil {
		t.Fatal("取消后继续扫描")
	}
	target := filepath.Join(outside, "synthetic.csv")
	if err := os.WriteFile(target, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "synthetic.csv")); err == nil {
		if _, err := Scan(context.Background(), c, []string{root}); err == nil {
			t.Fatal("链接未拒绝")
		}
	}
}
