package agentwire

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestPrepareEnrollmentEncryptsSecretsBeforeExchange(t *testing.T) {
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	material := []byte("synthetic-enrollment-material-0123456789")
	if err := store.PrepareEnrollment(EnrollmentConfig{
		ControlPlaneURL:    "https://control.example.test",
		EnrollmentID:       "enrollment-1",
		NodeID:             "node-1",
		EnrollmentMaterial: material,
	}); err != nil {
		t.Fatalf("PrepareEnrollment() error = %v", err)
	}
	stateContent, err := os.ReadFile(store.statePath)
	if err != nil {
		t.Fatalf("read protected state: %v", err)
	}
	defer zeroBytes(stateContent)
	if bytes.Contains(stateContent, material) {
		t.Fatal("关联材料以明文写入身份状态")
	}
	state, found, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	defer state.destroy()
	if !found || !state.pendingEnrollment() || !bytes.Equal(state.EnrollmentMaterial, material) || len(state.MachineCredential) < 32 || state.ExchangeRequestID == "" {
		t.Fatal("受保护关联状态不完整")
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(store.statePath); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("身份状态权限不安全: %v", err)
		}
	}
}

func TestPrepareEnrollmentReplacesCompletedIdentityOnlyWhenExplicitlyRequested(t *testing.T) {
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	completed := identityState{
		FormatVersion:     identityStateFormatVersion,
		ControlPlaneURL:   "https://control.example.test",
		ProtocolVersion:   Version,
		AgentID:           "agent-old",
		NodeID:            "node-1",
		MachineCredential: []byte("synthetic-machine-credential-0123456789"),
	}
	if err := store.saveState(&completed); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	defer completed.destroy()
	input := EnrollmentConfig{
		ControlPlaneURL:    "https://control.example.test",
		EnrollmentID:       "enrollment-2",
		NodeID:             "node-1",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-0123456789"),
	}
	if err := store.PrepareEnrollment(input); !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("PrepareEnrollment() error = %v, want ErrIdentityUnavailable", err)
	}
	input.ReplaceExisting = true
	if err := store.PrepareEnrollment(input); err != nil {
		t.Fatalf("PrepareEnrollment() explicit replacement error = %v", err)
	}
	state, found, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	defer state.destroy()
	if !found || !state.pendingEnrollment() || state.EnrollmentID != input.EnrollmentID || state.AgentID == completed.AgentID {
		t.Fatal("显式重新注册没有生成新的待关联身份")
	}
}

func TestPrepareEnrollmentRejectsRootKeySymlinkBeforeReadingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 创建符号链接需要测试环境的额外权限")
	}
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		t.Fatalf("create private directory: %v", err)
	}
	externalTarget := filepath.Join(t.TempDir(), "external-root-key")
	if err := os.WriteFile(externalTarget, []byte("not-an-agent-root-key"), 0o600); err != nil {
		t.Fatalf("write external target: %v", err)
	}
	if err := os.Symlink(externalTarget, store.rootKeyPath); err != nil {
		t.Fatalf("create root key symlink: %v", err)
	}
	if err := store.PrepareEnrollment(EnrollmentConfig{
		ControlPlaneURL: "https://control.example.test", EnrollmentID: "enrollment-1", NodeID: "node-1",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-0123456789"),
	}); !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("PrepareEnrollment() error = %v, want ErrIdentityUnavailable", err)
	}
}

func TestEnrollmentRetriesSameRequestIDAfterLostResponse(t *testing.T) {
	var mutex sync.Mutex
	var requests []enrollmentExchangeRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/v1/enrollments:exchange" || r.Method != http.MethodPost {
			t.Fatalf("意外关联请求: %s %s", r.Method, r.URL.Path)
		}
		var request enrollmentExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode enrollment request: %v", err)
		}
		mutex.Lock()
		requests = append(requests, request)
		attempt := len(requests)
		mutex.Unlock()
		if attempt == 1 {
			closeResponseConnection(t, w)
			return
		}
		writeProtocolResponse(t, w, "ENROLLED", map[string]any{
			"agentId": request.AgentID, "nodeId": request.NodeID, "protocolVersion": Version,
			"replayed": true, "realExecutionEnabled": false,
		})
	}))
	defer server.Close()
	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(requests) != 2 || requests[0].RequestID == "" || requests[0].RequestID != requests[1].RequestID {
		t.Fatalf("关联重试次数或 requestId 不符合预期: %d", len(requests))
	}
	if requests[0].MachineCredential != requests[1].MachineCredential || requests[0].EnrollmentMaterial != requests[1].EnrollmentMaterial {
		t.Fatal("关联重试改变了持久化机器材料")
	}
	state, found, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	defer state.destroy()
	if !found || state.pendingEnrollment() {
		t.Fatal("关联成功后待关联材料未清除")
	}
}

func TestHeartbeatRetriesWithSameRequestIDAndMachineCredential(t *testing.T) {
	var mutex sync.Mutex
	var enrolled enrollmentExchangeRequest
	var heartbeats []heartbeatRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/v1/enrollments:exchange":
			var request enrollmentExchangeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode enrollment request: %v", err)
			}
			mutex.Lock()
			enrolled = request
			mutex.Unlock()
			writeProtocolResponse(t, w, "ENROLLED", map[string]any{
				"agentId": request.AgentID, "nodeId": request.NodeID, "protocolVersion": Version,
				"replayed": false, "realExecutionEnabled": false,
			})
		case "/agent/v1/heartbeats":
			var request heartbeatRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode heartbeat request: %v", err)
			}
			mutex.Lock()
			if r.Header.Get("Authorization") != "Bearer "+enrolled.MachineCredential {
				mutex.Unlock()
				t.Fatal("心跳未携带已关联机器凭据")
			}
			heartbeats = append(heartbeats, request)
			attempt := len(heartbeats)
			mutex.Unlock()
			if attempt == 1 {
				closeResponseConnection(t, w)
				return
			}
			writeProtocolResponse(t, w, "ACCEPTED", map[string]any{
				"factsRevision": 2, "environmentStatus": "NOT_CHECKED", "realExecutionEnabled": false,
			})
		default:
			t.Fatalf("意外 Agent 请求路径: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	now := time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC)
	result, err := store.SendHeartbeat(context.Background(), Heartbeat{
		BootID: "boot-1",
		SentAt: now,
		Facts: EnvironmentFacts{
			OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "test-agent", ObservedAt: now,
			CapacityTotal: 1, CapacityUsed: 0,
		},
	})
	if err != nil {
		t.Fatalf("SendHeartbeat() error = %v", err)
	}
	if result.FactsRevision != 2 {
		t.Fatalf("facts revision = %d", result.FactsRevision)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(heartbeats) != 2 || heartbeats[0].RequestID == "" || heartbeats[0].RequestID != heartbeats[1].RequestID {
		t.Fatalf("心跳重试未复用 requestId: %#v", heartbeats)
	}
	if heartbeats[0].ProtocolVersion != Version || heartbeats[0].PayloadType != "HEARTBEAT" || heartbeats[0].Payload.CapacityTotal != 1 || heartbeats[0].Payload.CapacityUsed != 0 {
		t.Fatalf("心跳信封不严格: %#v", heartbeats[0])
	}
}

func TestEnrollmentRejectsUntrustedTLSCertificateAndKeepsPendingState(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("未受信任 TLS 服务不应收到关联请求")
	}))
	defer server.Close()
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	if err := store.PrepareEnrollment(EnrollmentConfig{
		ControlPlaneURL: server.URL, EnrollmentID: "enrollment-1", NodeID: "node-1",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-0123456789"),
	}); err != nil {
		t.Fatalf("PrepareEnrollment() error = %v", err)
	}
	if err := store.EnsureEnrollment(context.Background()); !errors.Is(err, ErrControlPlaneUnavailable) {
		t.Fatalf("EnsureEnrollment() error = %v, want TLS failure", err)
	}
	state, found, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	defer state.destroy()
	if !found || !state.pendingEnrollment() {
		t.Fatal("TLS 校验失败后关联材料被错误清除")
	}
}

func TestEnrollmentRejectsResponseWithoutExecutionDisabledAssertion(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request enrollmentExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode enrollment request: %v", err)
		}
		writeProtocolResponse(t, w, "ENROLLED", map[string]any{
			"agentId": request.AgentID, "nodeId": request.NodeID, "protocolVersion": Version, "replayed": false,
		})
	}))
	defer server.Close()
	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); !errors.Is(err, ErrProtocolRejected) {
		t.Fatalf("EnsureEnrollment() error = %v, want protocol rejection", err)
	}
	state, found, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	defer state.destroy()
	if !found || !state.pendingEnrollment() {
		t.Fatal("协议响应缺少执行关闭断言后关联状态被错误清除")
	}
}

func TestLoadStateRejectsSymbolicLinkBeforeReadingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 创建符号链接需要额外的系统权限")
	}
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		t.Fatalf("create state directory: %v", err)
	}
	target := filepath.Join(t.TempDir(), "unrelated-state")
	if err := os.WriteFile(target, []byte("unrelated-content"), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, store.statePath); err != nil {
		t.Fatalf("create state symlink: %v", err)
	}
	if _, _, err := store.loadState(); !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("loadState() error = %v, want protected-path rejection", err)
	}
}

func prepareTLSEnrollment(t *testing.T, server *httptest.Server) *StateStore {
	t.Helper()
	caFile := writeServerCA(t, server)
	store, err := OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	if err := store.PrepareEnrollment(EnrollmentConfig{
		ControlPlaneURL:    server.URL,
		CAFile:             caFile,
		EnrollmentID:       "enrollment-1",
		NodeID:             "node-1",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-0123456789"),
	}); err != nil {
		t.Fatalf("PrepareEnrollment() error = %v", err)
	}
	return store
}

func writeServerCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("TLS test server certificate is unavailable")
	}
	parsed, err := x509.ParseCertificate(certificate.Raw)
	if err != nil {
		t.Fatalf("parse TLS test certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "control-plane-ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: parsed.Raw}), 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	return path
}

func closeResponseConnection(t *testing.T, writer http.ResponseWriter) {
	t.Helper()
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		t.Fatal("TLS test response writer does not support hijacking")
	}
	connection, _, err := hijacker.Hijack()
	if err != nil {
		t.Fatalf("hijack test response: %v", err)
	}
	_ = connection.Close()
}

func writeProtocolResponse(t *testing.T, writer http.ResponseWriter, status string, payload map[string]any) {
	t.Helper()
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"requestId": "server-request", "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": status, "payload": payload,
	}); err != nil {
		t.Fatalf("write protocol response: %v", err)
	}
}
