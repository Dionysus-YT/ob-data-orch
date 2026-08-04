package integration

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// TestAgentEnrollmentAndHeartbeatOverTLS 只验证 G2 合成关联与环境事实链路。
// 测试服务器不连接数据库目标、不解析真实凭据，也不允许领取或启动任务。
func TestAgentEnrollmentAndHeartbeatOverTLS(t *testing.T) {
	metadata, _ := openSyntheticStore(t)
	ctx := context.Background()
	if err := metadata.EnsureAuthSubject(ctx, store.AuthSubject{
		SubjectID: "node-admin", ExternalSubject: "synthetic-node-admin", DisplayName: "Synthetic Node Admin", AccountStatus: "ACTIVE",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("EnsureAuthSubject() error = %v", err)
	}
	if _, err := metadata.CreateExecutionNode(ctx, store.ExecutionNodeCreate{
		NodeID: "node-agent-tls", CreatorSubjectID: "node-admin", DisplayName: "Synthetic TLS Node", NormalizedName: "synthetic tls node",
		Platform: "WINDOWS_AMD64", ToolHome: `E:\synthetic\ob-loader-dumper`, JavaPath: `C:\synthetic\java8\bin\java.exe`, AllowedRoots: []string{`E:\synthetic\exports`}, RequestID: "node-agent-tls-create",
		IdempotencyKey: "node-agent-tls-create-idempotency", RequestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateExecutionNode() error = %v", err)
	}

	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "synthetic-test"}, controlplane.Dependencies{
		Identity: agentWireBrowserIdentity{}, Authorizer: agentWireNodeAuthorizer{}, AgentProtocol: metadata,
		CSRF: agentWireCSRF{}, EnrollmentTTL: 10 * time.Minute, HeartbeatTTL: 2 * time.Minute,
	})
	server := httptest.NewTLSServer(handler)
	defer server.Close()

	issueRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/execution-nodes/node-agent-tls:enrollments", nil)
	if err != nil {
		t.Fatalf("create enrollment issue request: %v", err)
	}
	issueRequest.Header.Set("X-CSRF-Token", "synthetic-csrf")
	issueResponse, err := server.Client().Do(issueRequest)
	if err != nil {
		t.Fatalf("issue enrollment request: %v", err)
	}
	defer issueResponse.Body.Close()
	if issueResponse.StatusCode != http.StatusCreated || issueResponse.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("issue enrollment status=%d headers=%#v", issueResponse.StatusCode, issueResponse.Header)
	}
	var issued struct {
		EnrollmentID       string `json:"enrollmentId"`
		NodeID             string `json:"nodeId"`
		EnrollmentMaterial string `json:"enrollmentMaterial"`
		DisplayedOnce      bool   `json:"displayedOnce"`
	}
	if err := json.NewDecoder(issueResponse.Body).Decode(&issued); err != nil {
		t.Fatalf("decode enrollment issue response: %v", err)
	}
	if issued.EnrollmentID == "" || issued.NodeID != "node-agent-tls" || len(issued.EnrollmentMaterial) < 32 || !issued.DisplayedOnce {
		t.Fatalf("unsafe enrollment issue response: %#v", issued)
	}

	caFile := writeAgentWireTestCA(t, server)
	state, err := agentwire.OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	if err := state.PrepareEnrollment(agentwire.EnrollmentConfig{
		ControlPlaneURL: server.URL, CAFile: caFile, EnrollmentID: issued.EnrollmentID, NodeID: issued.NodeID,
		EnrollmentMaterial: []byte(issued.EnrollmentMaterial),
	}); err != nil {
		t.Fatalf("PrepareEnrollment() error = %v", err)
	}
	if err := state.EnsureEnrollment(ctx); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	now := time.Now().UTC()
	result, err := state.SendHeartbeat(ctx, agentwire.Heartbeat{
		BootID: "synthetic-tls-boot", SentAt: now,
		Facts: agentwire.EnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic-agent-v1", ObservedAt: now, CapacityTotal: 1, CapacityUsed: 0},
	})
	if err != nil || result.FactsRevision != 1 {
		t.Fatalf("SendHeartbeat() = %#v, %v", result, err)
	}
	node, err := metadata.GetExecutionNode(ctx, "node-agent-tls")
	if err != nil || node.Agent == nil || node.Agent.AgentID == "" || node.Agent.FactsRevision != 1 || node.Agent.BootID != "synthetic-tls-boot" ||
		node.Agent.EnvironmentFacts.OperatingSystem != "WINDOWS" || node.Agent.EnvironmentFacts.Architecture != "AMD64" || node.Agent.EnvironmentFacts.AgentVersion != "synthetic-agent-v1" || !node.Agent.EnvironmentFacts.ObservedAt.Equal(now) ||
		node.Agent.EnvironmentFacts.CPUUsagePercent != nil || node.Agent.EnvironmentFacts.MemoryUsagePercent != nil || len(node.Agent.EnvironmentFacts.DataRootUsages) != 0 {
		t.Fatalf("GetExecutionNode() = %#v, %v", node, err)
	}
}

type agentWireBrowserIdentity struct{}

func (agentWireBrowserIdentity) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "node-admin"}, nil
}

func (agentWireBrowserIdentity) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthenticated
}

type agentWireNodeAuthorizer struct{}

func (agentWireNodeAuthorizer) Authorize(_ context.Context, principal identity.Principal, scope identity.Scope, objectID string) error {
	if principal.Type == identity.BrowserPrincipal && principal.ID == "node-admin" && scope == identity.ScopeNodeManage && objectID == "node-agent-tls" {
		return nil
	}
	return errors.New("synthetic node authorization denied")
}

type agentWireCSRF struct{}

func (agentWireCSRF) ValidateCSRF(request *http.Request) error {
	if request.Header.Get("X-CSRF-Token") != "synthetic-csrf" {
		return errors.New("synthetic CSRF rejected")
	}
	return nil
}

func writeAgentWireTestCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("TLS test certificate is unavailable")
	}
	parsed, err := x509.ParseCertificate(certificate.Raw)
	if err != nil {
		t.Fatalf("parse TLS test certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "agent-control-plane-ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: parsed.Raw}), 0o600); err != nil {
		t.Fatalf("write TLS test CA: %v", err)
	}
	return path
}
