package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/localmvp"
	"ob-data-orch/internal/store"
)

func Test重新注册已删除节点后普通启动复用新身份(t *testing.T) {
	fixture := newRegistrationLifecycleFixture(t)
	ctx := context.Background()
	oldNodeID := createLocalTLSExecutionNode(t, ctx, fixture.server)
	fixture.register(t, oldNodeID)
	oldIdentity, err := fixture.state.AgentIdentity()
	if err != nil {
		t.Fatal("读取合成旧身份失败")
	}
	rootKey := fixture.readStateFile(t, "agent-identity-root-key.json")
	queueDirectory, err := fixture.state.LogQueueDirectory()
	if err != nil || os.MkdirAll(queueDirectory, 0o700) != nil {
		t.Fatal("创建合成日志队列失败")
	}
	queuePath := filepath.Join(queueDirectory, "synthetic-evidence")
	if err := os.WriteFile(queuePath, []byte("original-sanitized-evidence"), 0o600); err != nil {
		t.Fatal("写入合成日志证据失败")
	}
	node, err := fixture.database.GetExecutionNode(ctx, oldNodeID)
	if err != nil {
		t.Fatal("读取合成旧节点失败")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fixture.server.URL+"/api/v1/execution-nodes/"+oldNodeID+":delete", nil)
	if err != nil {
		t.Fatal("构造合成删除请求失败")
	}
	request.Header.Set("If-Match", fmt.Sprintf(`"rev-%d"`, node.Revision))
	request.Header.Set("X-CSRF-Token", localmvp.CSRFToken)
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal("发送合成删除请求失败")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("合成节点删除响应=%d", response.StatusCode)
	}
	// 删除只撤销控制面访问，不改变执行机器上保存的旧身份文件。
	now := time.Now().UTC()
	if _, err := fixture.state.SendHeartbeat(ctx, agentwire.Heartbeat{
		BootID: "synthetic-boot", SentAt: now,
		Facts: agentwire.EnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, CapacityTotal: 1},
	}); !errors.Is(err, agentwire.ErrAgentAuthenticationDenied) {
		t.Fatalf("已删除节点的旧身份未拒绝机器认证: %v", err)
	}
	newNodeID := createLocalTLSExecutionNode(t, ctx, fixture.server)
	fixture.register(t, newNodeID)
	newIdentity, err := fixture.state.AgentIdentity()
	if err != nil || newNodeID == oldNodeID || newIdentity.NodeID != newNodeID || newIdentity.AgentID == oldIdentity.AgentID {
		t.Fatal("重新注册没有切换到独立的新节点身份")
	}
	if !bytes.Equal(rootKey, fixture.readStateFile(t, "agent-identity-root-key.json")) {
		t.Fatal("重新注册改变了身份保护根密钥")
	}
	if content, err := os.ReadFile(queuePath); err != nil || string(content) != "original-sanitized-evidence" {
		t.Fatal("重新注册丢失了旧日志证据")
	}
	newQueueDirectory, err := fixture.state.LogQueueDirectory()
	if err != nil || newQueueDirectory == queueDirectory || filepath.Dir(newQueueDirectory) != filepath.Join(fixture.directory, "log-queues") {
		t.Fatal("新身份复用了旧身份的上传队列")
	}
	stateContent := fixture.readStateFile(t, "agent-identity.json")
	if err := runWithContext(ctx, []string{"-initialize"}, bytes.NewReader(nil), io.Discard, fixture.lookup); err != nil {
		t.Fatalf("普通启动初始化新身份失败: %v", err)
	}
	if !bytes.Equal(stateContent, fixture.readStateFile(t, "agent-identity.json")) {
		t.Fatal("普通启动重新生成了已完成的新身份")
	}
	if err := runWithContext(ctx, []string{"-once"}, bytes.NewReader(nil), io.Discard, fixture.lookup); err != nil {
		t.Fatalf("新身份的合成心跳失败: %v", err)
	}
	newNode, err := fixture.database.GetExecutionNode(ctx, newNodeID)
	if err != nil || newNode.Agent == nil || newNode.Agent.AgentID != newIdentity.AgentID || newNode.Agent.LastHeartbeatAt == nil {
		t.Fatal("控制面没有收到新身份的心跳")
	}
}

func Test重新注册无效输入保留旧身份网络失败保留待关联请求(t *testing.T) {
	fixture := newRegistrationLifecycleFixture(t)
	ctx := context.Background()
	nodeID := createLocalTLSExecutionNode(t, ctx, fixture.server)
	fixture.register(t, nodeID)
	stateContent := fixture.readStateFile(t, "agent-identity.json")
	if err := runWithContext(ctx, []string{"-register", "-initialize"}, bytes.NewBufferString("invalid-code\n"), io.Discard, fixture.lookup); err == nil {
		t.Fatal("无效注册码被接受")
	}
	if !bytes.Equal(stateContent, fixture.readStateFile(t, "agent-identity.json")) {
		t.Fatal("无效注册码覆盖了旧身份")
	}
	// 同一节点重新签发材料会先撤销旧凭据；临时网络失败不得换用其他待关联请求。
	issued := issueLocalTLSEnrollment(t, ctx, fixture.server, nodeID)
	code := testRegistrationCode(t, nodeID, issued.EnrollmentID, issued.EnrollmentMaterial)
	fixture.unavailable.Store(true)
	var output bytes.Buffer
	if err := runWithContext(ctx, []string{"-register", "-initialize"}, bytes.NewBufferString(code+"\n"), &output, fixture.lookup); !errors.Is(err, agentwire.ErrControlPlaneUnavailable) {
		t.Fatalf("网络不可用未阻断重新注册: %v", err)
	}
	if bytes.Contains(output.Bytes(), []byte(code)) || bytes.Contains(output.Bytes(), []byte(issued.EnrollmentMaterial)) {
		t.Fatal("重新注册泄露了合成注册码或关联材料")
	}
	if _, err := fixture.state.AgentIdentity(); !errors.Is(err, agentwire.ErrEnrollmentPending) {
		t.Fatal("网络失败后没有保留待关联状态")
	}
	pendingContent := fixture.readStateFile(t, "agent-identity.json")
	pendingQueue, err := fixture.state.LogQueueDirectory()
	if err != nil {
		t.Fatal("待关联请求没有固定日志队列")
	}
	otherCode := testRegistrationCode(t, nodeID, "synthetic-other-enrollment", "synthetic-other-material-012345678901")
	if err := runWithContext(ctx, []string{"-register", "-initialize"}, bytes.NewBufferString(otherCode+"\n"), io.Discard, fixture.lookup); err == nil {
		t.Fatal("不同注册码覆盖了尚可重试的请求")
	}
	if !bytes.Equal(pendingContent, fixture.readStateFile(t, "agent-identity.json")) {
		t.Fatal("拒绝不同注册码后改变了待关联状态")
	}
	fixture.unavailable.Store(false)
	if err := runWithContext(ctx, []string{"-initialize"}, bytes.NewReader(nil), io.Discard, fixture.lookup); err != nil {
		t.Fatalf("普通启动未恢复原待关联请求: %v", err)
	}
	if identity, err := fixture.state.AgentIdentity(); err != nil || identity.NodeID != nodeID {
		t.Fatal("重试没有完成原待关联身份")
	}
	if queue, err := fixture.state.LogQueueDirectory(); err != nil || queue != pendingQueue {
		t.Fatal("关联重试改变了待关联身份的日志队列")
	}
}

type registrationLifecycleFixture struct {
	database    *store.Store
	server      *httptest.Server
	state       *agentwire.StateStore
	directory   string
	lookup      func(string) (string, bool)
	unavailable atomic.Bool
}

func newRegistrationLifecycleFixture(t *testing.T) *registrationLifecycleFixture {
	t.Helper()
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatal("打开合成存储失败")
	}
	t.Cleanup(func() { database.Close() })
	if err := localmvp.PrepareStore(context.Background(), database); err != nil {
		t.Fatal("初始化合成控制面失败")
	}
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{9}, 32)})
	if err != nil {
		t.Fatal("创建合成密钥环失败")
	}
	dependencies, err := localmvp.Dependencies(database, keyring, false, false)
	if err != nil {
		t.Fatal("创建合成控制面依赖失败")
	}
	fixture := &registrationLifecycleFixture{database: database, directory: filepath.Join(t.TempDir(), "agent-security")}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "synthetic"}, dependencies)
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fixture.unavailable.Load() && r.URL.Path == "/agent/v1/enrollments:exchange" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(fixture.server.Close)
	caFile := writeLocalTLSControlPlaneCA(t, fixture.server)
	values := map[string]string{
		config.AgentControlPlaneURLEnvironmentVariable: fixture.server.URL, config.AgentControlPlaneCAFileEnvironmentVariable: caFile,
		config.AgentStateDirectoryEnvironmentVariable: fixture.directory,
	}
	fixture.lookup = func(key string) (string, bool) { value, ok := values[key]; return value, ok }
	fixture.state, err = agentwire.OpenStateStore(fixture.directory)
	if err != nil {
		t.Fatal("打开合成 Agent 状态失败")
	}
	return fixture
}

func (f *registrationLifecycleFixture) register(t *testing.T, nodeID string) {
	t.Helper()
	issued := issueLocalTLSEnrollment(t, context.Background(), f.server, nodeID)
	code := testRegistrationCode(t, nodeID, issued.EnrollmentID, issued.EnrollmentMaterial)
	if err := runWithContext(context.Background(), []string{"-register", "-initialize"}, bytes.NewBufferString(code+"\n"), io.Discard, f.lookup); err != nil {
		t.Fatalf("合成前台注册失败: %v", err)
	}
}

func (f *registrationLifecycleFixture) readStateFile(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(f.directory, name))
	if err != nil {
		t.Fatal("读取合成受保护状态文件失败")
	}
	return content
}
