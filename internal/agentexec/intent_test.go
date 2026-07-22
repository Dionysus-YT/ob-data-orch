package agentexec

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/credential"
)

func Test启动意图在进程创建前原子保存且不含合成秘密(t *testing.T) {
	workspace := createWorkspace(t)
	intent := validStartIntent()
	if err := WriteStartIntent(workspace, intent); err != nil {
		t.Fatalf("WriteStartIntent() 错误 = %v", err)
	}
	path := filepath.Join(workspace.EvidenceDirectory(), startIntentFileName)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取启动意图: %v", err)
	}
	if bytes.Contains(content, []byte("synthetic-secret-not-present")) {
		t.Fatal("启动意图包含合成秘密")
	}
	restored, found, err := ReadStartIntent(workspace)
	if err != nil || !found || restored != intent {
		t.Fatalf("ReadStartIntent() = %#v, %t, %v", restored, found, err)
	}
	if err := RemoveStartIntent(workspace); err != nil {
		t.Fatalf("RemoveStartIntent() 错误 = %v", err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatalf("Cleanup() 错误 = %v", err)
	}
}

func Test已有或损坏启动意图要求核对而不覆盖(t *testing.T) {
	workspace := createWorkspace(t)
	t.Cleanup(func() { _ = workspace.Cleanup() })
	if err := WriteStartIntent(workspace, validStartIntent()); err != nil {
		t.Fatalf("WriteStartIntent() 错误 = %v", err)
	}
	if err := WriteStartIntent(workspace, validStartIntent()); !errors.Is(err, ErrStartIntentExists) {
		t.Fatalf("重复 WriteStartIntent() 错误 = %v，期望 %v", err, ErrStartIntentExists)
	}
	if err := os.WriteFile(filepath.Join(workspace.EvidenceDirectory(), startIntentFileName), []byte(`{"executionId":"execution-1","unknown":true}`), 0o600); err != nil {
		t.Fatalf("写入损坏意图: %v", err)
	}
	if _, _, err := ReadStartIntent(workspace); !errors.Is(err, ErrStartIntentInvalid) {
		t.Fatalf("ReadStartIntent() 错误 = %v，期望 %v", err, ErrStartIntentInvalid)
	}
}

func Test启动意图拒绝缺少恢复身份的输入(t *testing.T) {
	workspace := createWorkspace(t)
	t.Cleanup(func() { _ = workspace.Cleanup() })
	intent := validStartIntent()
	intent.EnvelopeDigest = ""
	if err := WriteStartIntent(workspace, intent); !errors.Is(err, ErrStartIntentInvalid) {
		t.Fatalf("WriteStartIntent() 错误 = %v，期望 %v", err, ErrStartIntentInvalid)
	}
	if _, err := os.Stat(filepath.Join(workspace.EvidenceDirectory(), startIntentFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("无效输入仍创建了启动意图: %v", err)
	}
}

func Test启动意图拒绝可能携带秘密的摘要字段(t *testing.T) {
	workspace := createWorkspace(t)
	t.Cleanup(func() { _ = workspace.Cleanup() })
	intent := validStartIntent()
	intent.EnvelopeDigest = "synthetic-secret-not-allowed"
	if err := WriteStartIntent(workspace, intent); !errors.Is(err, ErrStartIntentInvalid) {
		t.Fatalf("WriteStartIntent() 错误 = %v，期望 %v", err, ErrStartIntentInvalid)
	}
}

func createWorkspace(t *testing.T) credential.Workspace {
	t.Helper()
	workspace, err := credential.CreateWorkspace(filepath.Join(t.TempDir(), "agent-security"), "execution-1")
	if err != nil {
		t.Fatalf("CreateWorkspace() 错误 = %v", err)
	}
	return workspace
}

func validStartIntent() StartIntent {
	return StartIntent{ExecutionID: "execution-1", TaskID: "task-1", LeaseID: "lease-1", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("a", 64), CreatedAt: time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)}
}
