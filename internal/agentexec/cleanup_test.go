package agentexec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/credential"
)

func Test终态证据完成后清理合成安全材料与启动意图(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent-security")
	workspace, err := credential.CreateWorkspace(root, "execution-1")
	if err != nil {
		t.Fatalf("CreateWorkspace() 错误 = %v", err)
	}
	secret := []byte("synthetic-cleanup-secret")
	material, err := credential.GenerateSecurityMaterial(secret)
	credential.Zero(secret)
	if err != nil {
		t.Fatalf("GenerateSecurityMaterial() 错误 = %v", err)
	}
	if _, err := workspace.WriteSecurityMaterial(&material); err != nil {
		t.Fatalf("WriteSecurityMaterial() 错误 = %v", err)
	}
	material.Destroy()
	if err := WriteStartIntent(workspace, validStartIntent()); err != nil {
		t.Fatalf("WriteStartIntent() 错误 = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace.RawLogDirectory(), "synthetic-tool.log"), []byte("synthetic raw log"), 0o600); err != nil {
		t.Fatalf("写入合成原始日志: %v", err)
	}
	result, err := CleanupSynthetic(workspace, true)
	if err != nil || !result.Cleaned || result.Residual {
		t.Fatalf("CleanupSynthetic() = %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "executions", "execution-1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("清理后 execution 目录仍存在: %v", err)
	}
}

func Test证据未完成时保留材料(t *testing.T) {
	workspace := createWorkspace(t)
	if err := WriteStartIntent(workspace, validStartIntent()); err != nil {
		t.Fatalf("WriteStartIntent() 错误 = %v", err)
	}
	result, err := CleanupSynthetic(workspace, false)
	if !errors.Is(err, ErrCleanupNotReady) || result.Cleaned || result.Residual {
		t.Fatalf("CleanupSynthetic() = %#v, %v", result, err)
	}
	if _, found, err := ReadStartIntent(workspace); err != nil || !found {
		t.Fatalf("未完成证据后的启动意图 = found:%t err:%v", found, err)
	}
	if err := RemoveStartIntent(workspace); err != nil {
		t.Fatalf("RemoveStartIntent() 错误 = %v", err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatalf("Cleanup() 错误 = %v", err)
	}
}

func Test清理遇到残留时隔离而不伪造成功(t *testing.T) {
	workspace := createWorkspace(t)
	if err := WriteStartIntent(workspace, validStartIntent()); err != nil {
		t.Fatalf("WriteStartIntent() 错误 = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace.EvidenceDirectory(), "unexpected-residual"), []byte("synthetic-residual"), 0o600); err != nil {
		t.Fatalf("写入合成残留: %v", err)
	}
	result, err := CleanupSynthetic(workspace, true)
	if !errors.Is(err, ErrCleanupFailed) || result.Cleaned || !result.Residual {
		t.Fatalf("CleanupSynthetic() = %#v, %v", result, err)
	}
	if _, statErr := os.Stat(workspace.EvidenceDirectory()); statErr != nil {
		t.Fatalf("隔离目录被错误删除: %v", statErr)
	}
}

func Test进程已停止时未完成事件只保留恢复证据不保留秘密(t *testing.T) {
	root := t.TempDir()
	w, err := credential.CreateWorkspace(root, "synthetic-stopped")
	if err != nil {
		t.Fatal(err)
	}
	content, err := credential.GenerateStorageConfiguration("S3", []byte("synthetic-access"), []byte("synthetic-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Zero(content)
	dir, err := w.WriteStorageConfiguration(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteStartIntent(w, validStartIntent()); err != nil {
		t.Fatal(err)
	}
	_, err = CleanupStoppedExecution(w, false)
	if !errors.Is(err, ErrCleanupNotReady) {
		t.Fatal("事件未完成却清除了恢复证据", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "core-site.xml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("恢复证据延长了存储秘密生命周期", err)
	}
	if _, found, err := ReadStartIntent(w); err != nil || !found {
		t.Fatal("待核对事件的启动意图丢失", err)
	}
	if err := credential.CheckWorkspaceRoot(root); !errors.Is(err, credential.ErrWorkspaceResidual) {
		t.Fatal("待恢复工作区没有阻断", err)
	}
}
