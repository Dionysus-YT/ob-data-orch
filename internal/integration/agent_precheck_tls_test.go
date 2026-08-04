package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/agentworker"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identity"
)

// TestAuthenticatedPrecheckOverTLS 覆盖 G2 受认证 Agent 的固定预检查闭环。
// 该测试只使用临时 SQLite、合成机器凭据与固定检查报告，不连接 ODP、不解析真实数据库密码且不启动工具。
func TestAuthenticatedPrecheckOverTLS(t *testing.T) {
	metadata, databasePath := openSyntheticStore(t)
	seedSyntheticMetadata(t, databasePath)
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "synthetic-test"}, controlplane.Dependencies{
		Identity:        authenticatedPrecheckBrowserIdentity{},
		Authorizer:      authenticatedPrecheckAuthorizer{},
		Roles:           authenticatedPrecheckAuthorizer{},
		DataSources:     metadata,
		CredentialRefs:  metadata,
		Drafts:          metadata,
		Prechecks:       metadata,
		AgentProtocol:   metadata,
		AgentPrechecks:  metadata,
		PrecheckSecrets: metadata,
		Decryptor:       authenticatedPrecheckSyntheticDecryptor{},
		Nodes:           integrationNodeReader{},
		Generator:       generator,
		PrecheckTTL:     2 * time.Minute,
		EnrollmentTTL:   10 * time.Minute,
		HeartbeatTTL:    2 * time.Minute,
		CSRF:            integrationCSRF{},
	})
	server := httptest.NewTLSServer(handler)
	defer server.Close()

	enrollment := issueAuthenticatedPrecheckEnrollment(t, server)
	state, err := agentwire.OpenStateStore(filepath.Join(t.TempDir(), "agent-security"))
	if err != nil {
		t.Fatalf("OpenStateStore() error = %v", err)
	}
	if err := state.PrepareEnrollment(agentwire.EnrollmentConfig{
		ControlPlaneURL:    server.URL,
		CAFile:             writeAgentWireTestCA(t, server),
		EnrollmentID:       enrollment.EnrollmentID,
		NodeID:             enrollment.NodeID,
		EnrollmentMaterial: []byte(enrollment.EnrollmentMaterial),
	}); err != nil {
		t.Fatalf("PrepareEnrollment() error = %v", err)
	}
	if err := state.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	now := time.Now().UTC()
	heartbeat, err := state.SendHeartbeat(context.Background(), agentwire.Heartbeat{
		BootID: "precheck-tls-boot",
		SentAt: now,
		Facts: agentwire.EnvironmentFacts{
			OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic-agent-v1",
			ObservedAt: now, CapacityTotal: 1, CapacityUsed: 0,
		},
	})
	if err != nil || heartbeat.FactsRevision != 1 {
		t.Fatalf("SendHeartbeat() = %#v, %v", heartbeat, err)
	}

	draftID := requestID(t, handler, http.MethodPost, "/api/v1/export-drafts", `{"dataSourceId":"source-1","nodeId":"node-1","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}`, map[string]string{"Idempotency-Key": "authenticated-precheck-draft-key"}, http.StatusCreated)
	precheckID := requestID(t, handler, http.MethodPost, "/api/v1/export-drafts/"+draftID+":precheck", "", map[string]string{"If-Match": `"rev-1"`, "Idempotency-Key": "authenticated-precheck-create-key"}, http.StatusAccepted)

	var probe *authenticatedPrecheckSyntheticProbe
	worker := &agentworker.Worker{
		Protocol: state,
		ProbeFactory: func(resolver agentworker.SecretResolver) agentpreflight.Probe {
			probe = &authenticatedPrecheckSyntheticProbe{resolver: resolver}
			return probe
		},
		Clock:         func() time.Time { return time.Now().UTC() },
		BootID:        "precheck-tls-boot",
		LocalPlatform: commandgen.PlatformWindowsAMD64,
	}
	outcome, found, err := worker.RunNext(context.Background())
	if err != nil || !found || outcome.State != agentwire.PrecheckSucceeded || !outcome.Report.Succeeded {
		t.Fatalf("Worker.RunNext() = %#v, %t, %v", outcome, found, err)
	}
	if probe == nil || !probe.resolvedSlot || len(probe.checks) != len(agentpreflight.FixedChecks()) {
		t.Fatalf("Worker synthetic probe = %#v", probe)
	}
	expectedProbeChecks := []agentpreflight.CheckID{
		agentpreflight.CheckToolEnvironment,
		agentpreflight.CheckOutputPath,
		agentpreflight.CheckOutputEmpty,
		agentpreflight.CheckAvailableSpace,
		agentpreflight.CheckDatabaseConnectivity,
		agentpreflight.CheckObjectAccess,
	}
	for index, check := range expectedProbeChecks {
		if probe.checks[index] != check {
			t.Fatalf("precheck probe order %d = %q, want %q", index, probe.checks[index], check)
		}
	}
	for index, check := range agentpreflight.FixedChecks() {
		if outcome.Report.Results[index].Check != check || outcome.Report.Results[index].Status != agentpreflight.StatusPassed {
			t.Fatalf("fixed precheck result %d = %#v", index, outcome.Report.Results[index])
		}
	}
	run, err := metadata.GetPrecheckRun(context.Background(), precheckID)
	machine, identityErr := state.AgentIdentity()
	if err != nil || identityErr != nil || run.Status != "SUCCEEDED" || run.IntegrityStatus != "COMPLETE" || run.BindingDigest == "" || run.BindingAgentID != machine.AgentID || run.NodeFactsRevision != heartbeat.FactsRevision {
		t.Fatalf("GetPrecheckRun() = %#v, %v", run, err)
	}
	assertCount(t, databasePath, "SELECT COUNT(*) FROM agent_precheck_secret_resolution_receipts WHERE status = 'RESOLVED'", 1)
}

type authenticatedPrecheckEnrollment struct {
	EnrollmentID       string `json:"enrollmentId"`
	NodeID             string `json:"nodeId"`
	EnrollmentMaterial string `json:"enrollmentMaterial"`
}

func issueAuthenticatedPrecheckEnrollment(t *testing.T, server *httptest.Server) authenticatedPrecheckEnrollment {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/execution-nodes/node-1:enrollments", nil)
	if err != nil {
		t.Fatalf("create enrollment issue request: %v", err)
	}
	request.Header.Set("X-CSRF-Token", "synthetic-csrf")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("issue enrollment request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("issue enrollment status=%d headers=%#v", response.StatusCode, response.Header)
	}
	var enrollment authenticatedPrecheckEnrollment
	if err := json.NewDecoder(response.Body).Decode(&enrollment); err != nil {
		t.Fatalf("decode enrollment issue response: %v", err)
	}
	if enrollment.EnrollmentID == "" || enrollment.NodeID != "node-1" || len(enrollment.EnrollmentMaterial) < 32 {
		t.Fatalf("unsafe enrollment issue response: %#v", enrollment)
	}
	return enrollment
}

// authenticatedPrecheckSyntheticDecryptor 仅为 TLS 合成链路提供不可用于真实连接的短时占位字节。
// 测试仍经过控制面的权限复验、槽位回执和响应绑定，且不会读取真实密文或凭据。
type authenticatedPrecheckSyntheticDecryptor struct{}

func (authenticatedPrecheckSyntheticDecryptor) Decrypt(credential.Envelope) ([]byte, error) {
	return []byte("synthetic-placeholder"), nil
}

// authenticatedPrecheckSyntheticProbe 只消费一次已确认的数据库槽位并返回受控六项合成事实。
// 它不调用 JDBC、文件系统或工具进程，因此 TLS 测试不把合成成功表述为真实环境验证。
type authenticatedPrecheckSyntheticProbe struct {
	resolver     agentworker.SecretResolver
	checks       []agentpreflight.CheckID
	resolvedSlot bool
}

func (p *authenticatedPrecheckSyntheticProbe) Probe(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	p.checks = append(p.checks, check)
	if check == agentpreflight.CheckDatabaseConnectivity {
		connection, err := p.resolver.ResolveDatabaseConnection(ctx, request.Binding)
		if err != nil {
			return agentpreflight.Result{}, err
		}
		defer credential.Zero(connection.Username)
		defer credential.Zero(connection.Password)
		if connection.Port < 1 || len(connection.Username) == 0 || len(connection.Password) == 0 {
			return agentpreflight.Result{}, errors.New("合成数据库槽位无效")
		}
		p.resolvedSlot = true
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "DATABASE_CONNECTED"}, nil
	}
	switch check {
	case agentpreflight.CheckObjectAccess:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "OBJECT_ACCESSIBLE"}, nil
	case agentpreflight.CheckToolEnvironment, agentpreflight.CheckOutputPath, agentpreflight.CheckOutputEmpty, agentpreflight.CheckAvailableSpace:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "SYNTHETIC_OK"}, nil
	default:
		return agentpreflight.Result{}, agentpreflight.ErrInvalidRequest
	}
}

type authenticatedPrecheckBrowserIdentity struct{}

func (authenticatedPrecheckBrowserIdentity) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "subject-1"}, nil
}

func (authenticatedPrecheckBrowserIdentity) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthenticated
}

type authenticatedPrecheckAuthorizer struct{}

func (authenticatedPrecheckAuthorizer) Authorize(_ context.Context, principal identity.Principal, scope identity.Scope, objectID string) error {
	if principal.Type != identity.BrowserPrincipal || principal.ID != "subject-1" {
		return errors.New("synthetic authorization denied")
	}
	if (scope == identity.ScopeDataSourceRead && objectID == "source-1") ||
		((scope == identity.ScopeNodeUse || scope == identity.ScopeNodeManage) && objectID == "node-1") {
		return nil
	}
	return errors.New("synthetic authorization denied")
}

func (authenticatedPrecheckAuthorizer) AuthorizeRole(_ context.Context, principal identity.Principal, role identity.Role) error {
	if principal.Type == identity.BrowserPrincipal && principal.ID == "subject-1" && role == identity.RoleNodeAdmin {
		return nil
	}
	return errors.New("synthetic role denied")
}
