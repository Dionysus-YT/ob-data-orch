package directloadfiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/directload"
)

func Test旁路文件集合必须完整且属于唯一目标表(t *testing.T) {
	root := t.TempDir()
	c := directload.Config{Table: "synthetic", Format: "CSV", FilePath: root}
	for _, name := range []string{"synthetic.1.csv", "synthetic.2.csv"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("1,synthetic\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Scan(context.Background(), c, []string{root})
	if err != nil || m.FileCount != 2 || !Valid(m, c) {
		t.Fatal("同表分片未通过", err)
	}
	for _, mutate := range []func(){func() { m.Files[0].Object = "foreign" }, func() { m.Files[0].Path = "../synthetic.csv" }, func() { m.Files[0].SHA256 = "unknown" }, func() { m.TotalBytes++ }, func() { m.FileCount-- }} {
		fresh, _ := Scan(context.Background(), c, []string{root})
		m = fresh
		mutate()
		if Valid(m, c) {
			t.Fatal("清单篡改未阻断")
		}
	}
	if err := os.WriteFile(filepath.Join(root, "other.csv"), []byte("2,synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(context.Background(), c, []string{root}); err == nil {
		t.Fatal("其他表文件被静默过滤")
	}
}

func Test旁路文件扫描拒绝越界零匹配与取消(t *testing.T) {
	root := t.TempDir()
	c := directload.Config{Table: "synthetic", Format: "CSV", FilePath: root}
	if _, err := Scan(context.Background(), c, []string{root}); err == nil {
		t.Fatal("零匹配未阻断")
	}
	if err := os.WriteFile(filepath.Join(root, "synthetic.csv"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(context.Background(), c, []string{t.TempDir()}); err == nil {
		t.Fatal("越界未阻断")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, c, []string{root}); err == nil {
		t.Fatal("取消后仍扫描")
	}
	if _, err := Scan(nil, c, []string{root}); err == nil {
		t.Fatal("空上下文未阻断")
	}
}
