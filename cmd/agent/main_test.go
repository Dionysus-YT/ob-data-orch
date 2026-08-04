package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ob-data-orch/internal/agentconnectiontest"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/localmvp"
	"ob-data-orch/internal/store"
)

func TestReadEnrollmentMaterialAcceptsOnlyBoundedStandardInput(t *testing.T) {
	material, err := readEnrollmentMaterial(bytes.NewBufferString("  synthetic-enrollment-material-0123456789\n"))
	if err != nil {
		t.Fatalf("readEnrollmentMaterial() error = %v", err)
	}
	if string(material) != "synthetic-enrollment-material-0123456789" {
		t.Fatal("关联材料未按预期读取")
	}
	if _, err := readEnrollmentMaterial(bytes.NewBuffer(make([]byte, 4097))); err == nil {
		t.Fatal("超长关联材料被接受")
	}
}

func TestRunWithContextVersionDoesNotRequireIdentityState(t *testing.T) {
	var output bytes.Buffer
	err := runWithContext(context.Background(), []string{"-version"}, bytes.NewReader(nil), &output, func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatalf("runWithContext(-version) error = %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("版本输出为空")
	}
}

func TestRunWithContextRejectsEnrollmentArgumentsWithoutEnroll(t *testing.T) {
	err := runWithContext(context.Background(), []string{"-node-id", "node-1"}, bytes.NewReader(nil), &bytes.Buffer{}, func(string) (string, bool) {
		return "", false
	})
	if err == nil {
		t.Fatalf("非关联模式接受节点参数: %v", err)
	}
}

func TestRunWithContext在本机TLS控制面完成Agent关联和心跳(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("打开本机控制面存储失败: %v", err)
	}
	defer database.Close()
	if err := localmvp.PrepareStore(ctx, database); err != nil {
		t.Fatalf("初始化本机身份投影失败: %v", err)
	}
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{9}, 32)})
	if err != nil {
		t.Fatalf("创建测试密钥环失败: %v", err)
	}
	dependencies, err := localmvp.Dependencies(database, keyring, false, false)
	if err != nil {
		t.Fatalf("创建本机 MVP 依赖失败: %v", err)
	}
	server := httptest.NewTLSServer(controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "local-tls-test"}, dependencies))
	defer server.Close()

	createdNodeID := createLocalTLSExecutionNode(t, ctx, server)
	issued := issueLocalTLSEnrollment(t, ctx, server, createdNodeID)
	caFile := writeLocalTLSControlPlaneCA(t, server)
	stateDirectory := filepath.Join(t.TempDir(), "agent-security")
	environment := map[string]string{
		config.AgentControlPlaneURLEnvironmentVariable:    server.URL,
		config.AgentControlPlaneCAFileEnvironmentVariable: caFile,
		config.AgentStateDirectoryEnvironmentVariable:     stateDirectory,
	}
	lookupEnv := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}
	registrationCode := testRegistrationCode(t, createdNodeID, issued.EnrollmentID, issued.EnrollmentMaterial)
	if err := runWithContext(ctx, []string{"-register", "-once"}, bytes.NewBufferString(registrationCode), io.Discard, lookupEnv); err != nil {
		t.Fatalf("Agent 本机关联与心跳失败: %v", err)
	}
	node, err := database.GetExecutionNode(ctx, createdNodeID)
	if err != nil {
		t.Fatalf("读取关联后的节点失败: %v", err)
	}
	if node.Agent == nil || node.Agent.AgentID == "" || node.Agent.FactsRevision != 1 {
		t.Fatal("控制面未保存已关联 Agent 的首次心跳事实")
	}
	if node.Agent.EnvironmentFacts.OperatingSystem != "WINDOWS" || node.Agent.EnvironmentFacts.Architecture != "AMD64" || node.Agent.CapacityTotal != 1 || node.Agent.CapacityUsed != 0 {
		t.Fatal("控制面保存的 Agent 心跳事实不符合本机单次关联预期")
	}
}

func createLocalTLSExecutionNode(t *testing.T, ctx context.Context, server *httptest.Server) string {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"Local TLS Node","platform":"WINDOWS_AMD64","allowedRoots":["E:\\synthetic\\exports"],"toolHome":"E:\\synthetic\\ob-loader-dumper-4.3.5","javaPath":"C:\\synthetic\\java\\bin\\java.exe"}`))
	if err != nil {
		t.Fatalf("创建节点请求失败: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "local-tls-node-create")
	request.Header.Set("X-CSRF-Token", localmvp.CSRFToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("发送创建节点请求失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("创建节点响应不符合失败关闭边界: status=%d", response.StatusCode)
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.ID == "" {
		t.Fatal("创建节点响应未返回安全节点标识")
	}
	return payload.ID
}

func issueLocalTLSEnrollment(t *testing.T, ctx context.Context, server *httptest.Server, nodeID string) struct {
	EnrollmentID       string `json:"enrollmentId"`
	EnrollmentMaterial string `json:"enrollmentMaterial"`
} {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/execution-nodes/"+nodeID+":enrollments", nil)
	if err != nil {
		t.Fatalf("创建关联材料请求失败: %v", err)
	}
	request.Header.Set("X-CSRF-Token", localmvp.CSRFToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("发送关联材料请求失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("关联材料响应不符合失败关闭边界: status=%d", response.StatusCode)
	}
	var payload struct {
		EnrollmentID       string `json:"enrollmentId"`
		EnrollmentMaterial string `json:"enrollmentMaterial"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.EnrollmentID == "" || len(payload.EnrollmentMaterial) < 32 {
		t.Fatal("关联材料响应未返回可用的一次性材料")
	}
	return payload
}

func writeLocalTLSControlPlaneCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("本机 TLS 控制面未提供证书")
	}
	parsed, err := x509.ParseCertificate(certificate.Raw)
	if err != nil {
		t.Fatalf("解析本机 TLS 证书失败: %v", err)
	}
	path := filepath.Join(t.TempDir(), "control-plane-ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: parsed.Raw}), 0o600); err != nil {
		t.Fatalf("写入本机 TLS CA 失败: %v", err)
	}
	return path
}

func TestNewBootIDReturnsCanonicalUUIDV4(t *testing.T) {
	bootID, err := newBootID()
	if err != nil {
		t.Fatalf("newBootID() error = %v", err)
	}
	if len(bootID) != 36 || bootID[8] != '-' || bootID[13] != '-' || bootID[18] != '-' || bootID[23] != '-' || bootID[14] != '4' || (bootID[19] != '8' && bootID[19] != '9' && bootID[19] != 'a' && bootID[19] != 'b') {
		t.Fatalf("boot ID 不是规范 UUIDv4: %q", bootID)
	}
	compact := bootID[0:8] + bootID[9:13] + bootID[14:18] + bootID[19:23] + bootID[24:36]
	if _, err := hex.DecodeString(compact); err != nil {
		t.Fatalf("boot ID 十六进制无效: %q, %v", bootID, err)
	}
}

func TestRunHeartbeatLoop单次模式先确认心跳再运行一条连接测试(t *testing.T) {
	calls := make([]string, 0, 3)
	heartbeat := &heartbeatProtocolStub{calls: &calls}
	runner := &connectionTestRunnerStub{calls: &calls, found: true}
	err := runHeartbeatLoop(
		context.Background(), heartbeat, runner, "boot-1", "WINDOWS", "AMD64",
		time.Minute, true, testLogger(),
	)
	if err != nil {
		t.Fatalf("runHeartbeatLoop() error = %v", err)
	}
	if want := []string{"enroll", "heartbeat", "connection-test"}; !sameAgentCalls(calls, want) {
		t.Fatalf("调用顺序 = %#v，期望 %#v", calls, want)
	}
	if heartbeat.input.BootID != "boot-1" || heartbeat.input.Facts.OperatingSystem != "WINDOWS" || heartbeat.input.Facts.Architecture != "AMD64" {
		t.Fatalf("心跳输入 = %#v", heartbeat.input)
	}
}

func TestRunHeartbeatLoop心跳失败时不领取连接测试(t *testing.T) {
	calls := make([]string, 0, 2)
	wantErr := errors.New("synthetic heartbeat failure")
	heartbeat := &heartbeatProtocolStub{calls: &calls, heartbeatErr: wantErr}
	runner := &connectionTestRunnerStub{calls: &calls}
	err := runHeartbeatLoop(
		context.Background(), heartbeat, runner, "boot-1", "WINDOWS", "AMD64",
		time.Minute, true, testLogger(),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runHeartbeatLoop() error = %v", err)
	}
	if want := []string{"enroll", "heartbeat"}; !sameAgentCalls(calls, want) {
		t.Fatalf("心跳失败后的调用 = %#v，期望 %#v", calls, want)
	}
}

func TestRunHeartbeatLoop长期模式隔离连接测试错误(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := make([]string, 0, 3)
	heartbeat := &heartbeatProtocolStub{calls: &calls}
	runner := &connectionTestRunnerStub{
		calls: &calls,
		err:   agentconnectiontest.ErrGrantRejected,
		afterRun: func() {
			cancel()
		},
	}
	err := runHeartbeatLoop(ctx, heartbeat, runner, "boot-1", "WINDOWS", "AMD64", time.Hour, false, testLogger())
	if err != nil {
		t.Fatalf("runHeartbeatLoop() error = %v", err)
	}
	if want := []string{"enroll", "heartbeat", "connection-test"}; !sameAgentCalls(calls, want) {
		t.Fatalf("长期模式调用 = %#v，期望 %#v", calls, want)
	}
}

func TestRunHeartbeatLoop单次模式暴露连接测试错误(t *testing.T) {
	calls := make([]string, 0, 3)
	heartbeat := &heartbeatProtocolStub{calls: &calls}
	runner := &connectionTestRunnerStub{calls: &calls, err: agentconnectiontest.ErrGrantRejected}
	err := runHeartbeatLoop(
		context.Background(), heartbeat, runner, "boot-1", "WINDOWS", "AMD64",
		time.Minute, true, testLogger(),
	)
	if !errors.Is(err, agentconnectiontest.ErrGrantRejected) {
		t.Fatalf("runHeartbeatLoop() error = %v", err)
	}
}

func TestRunAgentProtocolLoop连接测试不等待下一次心跳(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make([]string, 0, 4)
	connectionAttempts := 0
	heartbeat := &heartbeatProtocolStub{calls: &calls}
	runner := &connectionTestRunnerStub{
		calls: &calls,
		afterRun: func() {
			connectionAttempts++
			if connectionAttempts == 2 {
				cancel()
			}
		},
	}

	err := runAgentProtocolLoop(ctx, heartbeat, runner, "boot-1", "WINDOWS", "AMD64", time.Hour, time.Millisecond, false, testLogger())
	if err != nil {
		t.Fatalf("runAgentProtocolLoop() error = %v", err)
	}
	if want := []string{"enroll", "heartbeat", "connection-test", "connection-test"}; !sameAgentCalls(calls, want) {
		t.Fatalf("连接测试未在独立周期领取: %#v，期望 %#v", calls, want)
	}
}

func TestRunAgentProtocolLoop拒绝无效连接测试周期(t *testing.T) {
	err := runAgentProtocolLoop(context.Background(), &heartbeatProtocolStub{calls: &[]string{}}, &connectionTestRunnerStub{calls: &[]string{}}, "boot-1", "WINDOWS", "AMD64", time.Minute, 0, false, testLogger())
	if err == nil {
		t.Fatal("零连接测试周期未被拒绝")
	}
}

type heartbeatProtocolStub struct {
	calls         *[]string
	enrollmentErr error
	heartbeatErr  error
	input         agentwire.Heartbeat
}

func (s *heartbeatProtocolStub) EnsureEnrollment(context.Context) error {
	*s.calls = append(*s.calls, "enroll")
	return s.enrollmentErr
}

func (s *heartbeatProtocolStub) SendHeartbeat(_ context.Context, input agentwire.Heartbeat) (agentwire.HeartbeatResult, error) {
	*s.calls = append(*s.calls, "heartbeat")
	s.input = input
	return agentwire.HeartbeatResult{FactsRevision: 1}, s.heartbeatErr
}

type connectionTestRunnerStub struct {
	calls    *[]string
	found    bool
	err      error
	afterRun func()
}

func (s *connectionTestRunnerStub) RunNext(context.Context) (agentconnectiontest.Outcome, bool, error) {
	*s.calls = append(*s.calls, "connection-test")
	if s.afterRun != nil {
		s.afterRun()
	}
	return agentconnectiontest.Outcome{}, s.found, s.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sameAgentCalls(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
