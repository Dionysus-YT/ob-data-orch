package credential

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func Test对象存储私有材料随工作区完整清理(t *testing.T) {
	root := t.TempDir()
	w, err := CreateWorkspace(root, "synthetic-storage")
	if err != nil {
		t.Fatal(err)
	}
	content, err := GenerateStorageConfiguration("S3", []byte("synthetic-access"), []byte("synthetic-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer Zero(content)
	dir, err := w.WriteStorageConfiguration(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Cleanup(); err != nil {
		t.Fatal("存储材料未完整清理", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "core-site.xml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("存储配置残留")
	}
	if err := CheckWorkspaceRoot(root); err != nil {
		t.Fatal("完整清理后不应残留阻断", err)
	}
}

func Test清理失败隔离且不能通过更换执行标识绕过(t *testing.T) {
	root := t.TempDir()
	w, err := CreateWorkspace(root, "synthetic-failed")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.securityDir, "hadoop-conf"), []byte("synthetic-residual"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.Cleanup(); err == nil {
		t.Fatal("不合法材料目录未触发失败关闭")
	}
	if err := CheckWorkspaceRoot(root); !errors.Is(err, ErrWorkspaceResidual) {
		t.Fatal("清理失败未阻断根目录", err)
	}
	if _, err := CreateWorkspace(root, "synthetic-next"); !errors.Is(err, ErrWorkspaceResidual) {
		t.Fatal("更换执行标识绕过残留", err)
	}
	if _, err := os.Stat(filepath.Join(w.securityDir, "hadoop-conf")); err != nil {
		t.Fatal("失败残留未保留供核对", err)
	}
}

func Test工作区护栏跨进程保持且活动并发不被误阻断(t *testing.T) {
	if root := os.Getenv("OBDO_TEST_WORKSPACE_GUARD_ROOT"); root != "" {
		if err := CheckWorkspaceRoot(root); !errors.Is(err, ErrWorkspaceResidual) {
			t.Fatal("新进程忽略未清理护栏", err)
		}
		return
	}
	root := t.TempDir()
	a, err := CreateWorkspace(root, "synthetic-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateWorkspace(root, "synthetic-b")
	if err != nil {
		t.Fatal("正常并发被错误阻断", err)
	}
	if err := CheckWorkspaceRoot(root); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^Test工作区护栏跨进程保持且活动并发不被误阻断$")
	command.Env = append(os.Environ(), "OBDO_TEST_WORKSPACE_GUARD_ROOT="+root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("合成进程复验失败: %v %s", err, output)
	}
	if err := a.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := b.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkspaceRoot(root); err != nil {
		t.Fatal(err)
	}
}

func Test升级前没有护栏的执行残留仍阻断(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "executions", "synthetic-legacy", "security", "hadoop-conf")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "core-site.xml"), []byte("synthetic legacy material"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkspaceRoot(root); !errors.Is(err, ErrWorkspaceResidual) {
		t.Fatal("旧版残留没有阻断", err)
	}
}
