package agentwire

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/identifier"
)

func Test旧身份保留原队列重新注册隔离历史证据(t *testing.T) {
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatal("打开合成身份存储失败")
	}
	legacy := identityState{
		FormatVersion: identityStateFormatVersion, ControlPlaneURL: "https://control.example.test", ProtocolVersion: Version,
		AgentID: "legacy-agent", NodeID: "legacy-node", MachineCredential: bytes.Repeat([]byte{7}, 32),
	}
	defer legacy.destroy()
	if err := store.saveState(&legacy); err != nil {
		t.Fatal("保存合成旧身份失败")
	}
	oldQueue, err := store.LogQueueDirectory()
	if err != nil || oldQueue != filepath.Join(store.directory, "log-queue") {
		t.Fatal("普通启动改变了旧队列位置")
	}
	if err := os.MkdirAll(oldQueue, 0o700); err != nil {
		t.Fatal("创建合成历史队列失败")
	}
	evidenceFile := filepath.Join(oldQueue, "synthetic-evidence")
	if err := os.WriteFile(evidenceFile, []byte("preserved-sanitized-evidence"), 0o600); err != nil {
		t.Fatal("写入合成历史证据失败")
	}
	if err := store.PrepareEnrollment(EnrollmentConfig{
		ControlPlaneURL: "https://control.example.test", EnrollmentID: "synthetic-enrollment", NodeID: "new-node",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-0123456789"), ReplaceExisting: true,
	}); err != nil {
		t.Fatal("准备显式重新关联失败")
	}
	state, found, err := store.loadState()
	if err != nil || !found {
		t.Fatal("读取新身份的待关联状态失败")
	}
	defer state.destroy()
	newQueue, err := store.LogQueueDirectory()
	if err != nil || state.LogQueueNamespace != state.AgentID || newQueue != filepath.Join(store.directory, "log-queues", state.AgentID) {
		t.Fatal("新身份没有独立队列位置")
	}
	if info, err := os.Stat(newQueue); err != nil || !info.IsDir() {
		t.Fatal("注册时未准备环境检查需要的私有队列")
	}
	if content, err := os.ReadFile(evidenceFile); err != nil || string(content) != "preserved-sanitized-evidence" {
		t.Fatal("隔离新队列时删除或改写了历史证据")
	}
}

func Test新队列不可创建时不替换旧身份(t *testing.T) {
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatal("打开合成身份存储失败")
	}
	legacy := identityState{
		FormatVersion: identityStateFormatVersion, ControlPlaneURL: "https://control.example.test", ProtocolVersion: Version,
		AgentID: "legacy-agent", NodeID: "legacy-node", MachineCredential: bytes.Repeat([]byte{7}, 32),
	}
	defer legacy.destroy()
	if err := store.saveState(&legacy); err != nil {
		t.Fatal("保存合成旧身份失败")
	}
	before, err := os.ReadFile(store.statePath)
	if err != nil {
		t.Fatal("读取合成旧状态失败")
	}
	if err := os.WriteFile(filepath.Join(store.directory, "log-queues"), []byte("synthetic-path-obstruction"), 0o600); err != nil {
		t.Fatal("构造合成队列路径阻断失败")
	}
	if err := store.PrepareEnrollment(EnrollmentConfig{
		ControlPlaneURL: "https://control.example.test", EnrollmentID: "synthetic-enrollment", NodeID: "new-node",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-0123456789"), ReplaceExisting: true,
	}); !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatal("队列路径受阻时仍替换身份")
	}
	if after, err := os.ReadFile(store.statePath); err != nil || !bytes.Equal(before, after) {
		t.Fatal("队列创建失败改变了原身份文件")
	}
}

func Test身份日志队列拒绝跨身份和目录穿越绑定(t *testing.T) {
	agentID, err := identifier.NewUUIDV4()
	if err != nil {
		t.Fatal("生成合成身份标识失败")
	}
	otherID, err := identifier.NewUUIDV4()
	if err != nil {
		t.Fatal("生成另一个合成身份标识失败")
	}
	for _, namespace := range []string{"../outside", `..\outside`, "/absolute", otherID, "not-a-uuid"} {
		t.Run(namespace, func(t *testing.T) {
			store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
			if err != nil {
				t.Fatal("打开合成身份存储失败")
			}
			state := identityState{
				FormatVersion: identityStateFormatVersion, ControlPlaneURL: "https://control.example.test", ProtocolVersion: Version,
				AgentID: agentID, NodeID: "synthetic-node", MachineCredential: bytes.Repeat([]byte{7}, 32), LogQueueNamespace: namespace,
			}
			defer state.destroy()
			if err := store.saveState(&state); !errors.Is(err, ErrIdentityUnavailable) {
				t.Fatal("非法队列绑定未拒绝落盘")
			}
			// 构造认证加密有效但绑定非法的状态，读取端也必须拒绝生成路径。
			state.LogQueueNamespace = agentID
			if err := store.saveState(&state); err != nil {
				t.Fatal("保存合成合法身份失败")
			}
			rootKey, err := store.loadOrCreateRootKey()
			if err != nil {
				t.Fatal("读取合成保护根失败")
			}
			defer zeroBytes(rootKey)
			state.LogQueueNamespace = namespace
			content, err := encryptIdentityState(state, rootKey, identityAAD(store.statePath))
			if err != nil {
				t.Fatal("构造合成认证状态失败")
			}
			defer zeroBytes(content)
			if err := writePrivateFileAtomically(store.statePath, content); err != nil {
				t.Fatal("写入合成认证状态失败")
			}
			if path, err := store.LogQueueDirectory(); path != "" || !errors.Is(err, ErrIdentityUnavailable) {
				t.Fatal("读取端接受了非法队列路径或回退到旧目录")
			}
		})
	}
}
