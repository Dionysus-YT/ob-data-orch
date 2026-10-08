package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/store"
)

var nodeFixtureTime = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

func TestHealthAndReadinessExposeG1Boundary(t *testing.T) {
	t.Parallel()

	handler := NewHandler(buildinfo.Info{Version: "test"})
	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["stage"] != "G1" || body["realExecutionEnabled"] != false {
				t.Fatalf("unexpected boundary response: %#v", body)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("health responses must not be cached")
			}
		})
	}
}

func TestCreateDataSourceRequiresSafetyChecksAndPassesOnlyEncryptedCredential(t *testing.T) {
	t.Parallel()
	keyring, err := credential.NewKeyring(map[string][]byte{"test-key": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	creator := &recordingCreator{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Roles: allowedRoleAuthorizer{}, Creator: creator,
		Encryptor: keyring, CSRF: allowedCSRF{}, CredentialKeyID: "test-key",
	})
	body := []byte(`{"displayName":"Created Source","environment":"TEST","connectionKind":"ODP","compatibilityMode":"MYSQL","host":"127.0.0.1","port":2881,"clusterName":"","tenantName":"synthetic-tenant","username":"synthetic-user","password":"synthetic-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources", bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "synthetic-idempotency-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || creator.input.DataSourceID == "" || creator.input.ClusterName != "" || creator.input.TenantName != "synthetic-tenant" || len(creator.input.Ciphertext) == 0 {
		t.Fatalf("create response=%d input=%#v", response.Code, creator.input)
	}
	serialized, _ := json.Marshal(creator.input)
	if bytes.Contains(serialized, []byte("synthetic-password")) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatal("plaintext password escaped create boundary")
	}
}

func TestCreateDataSourceReturnsFieldErrorsBeforeCredentialWork(t *testing.T) {
	t.Parallel()
	keyring, err := credential.NewKeyring(map[string][]byte{"test-key": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	creator := &recordingCreator{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Roles: allowedRoleAuthorizer{}, Creator: creator,
		Encryptor: keyring, CSRF: allowedCSRF{}, CredentialKeyID: "test-key",
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources", bytes.NewBufferString(`{"displayName":" ","environment":"TEST","connectionKind":"ODP","compatibilityMode":"MYSQL","host":"127.0.0.1","port":2881,"clusterName":"","tenantName":"synthetic-tenant","username":"","password":"synthetic-password"}`))
	request.Header.Set("Idempotency-Key", "synthetic-invalid-source")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity || creator.input.DataSourceID != "" || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"DATA_SOURCE_FIELDS_INVALID"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"field":"displayName"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"field":"username"`)) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatalf("field error response=%d body=%s input=%#v", response.Code, response.Body.String(), creator.input)
	}
}

func TestCreateDataSourceReturnsSafeNameUnavailableError(t *testing.T) {
	t.Parallel()
	keyring, err := credential.NewKeyring(map[string][]byte{"test-key": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Roles: allowedRoleAuthorizer{}, Creator: nameUnavailableCreator{},
		Encryptor: keyring, CSRF: allowedCSRF{}, CredentialKeyID: "test-key",
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources", bytes.NewBufferString(`{"displayName":"Synthetic Source","environment":"TEST","connectionKind":"ODP","compatibilityMode":"MYSQL","host":"192.0.2.40","port":2881,"clusterName":"synthetic-cluster","tenantName":"synthetic-tenant","username":"synthetic-user","password":"synthetic-password"}`))
	request.Header.Set("Idempotency-Key", "synthetic-name-unavailable")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity || !bytes.Contains(response.Body.Bytes(), []byte("DATA_SOURCE_NAME_UNAVAILABLE")) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatalf("name unavailable response=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		FieldErrors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"fieldErrors"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode name unavailable response: %v", err)
	}
	if len(body.FieldErrors) != 1 || body.FieldErrors[0].Field != "displayName" || body.FieldErrors[0].Code != "DATA_SOURCE_NAME_UNAVAILABLE" {
		t.Fatalf("name unavailable field errors = %#v", body.FieldErrors)
	}
}

func TestListDataSourcesFiltersUnauthorizedObjectsAndReturnsSafeShape(t *testing.T) {
	t.Parallel()
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity:   browserOnlyIdentityProvider{},
		Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: false},
		DataSources: staticDataSources{summaries: []store.DataSourceSummary{
			{DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 2, SysUser: "synthetic-sys-user", SysCredentialID: "synthetic-sys-credential", SysCredentialRevision: 1, LastTestStatus: "SUCCEEDED"},
			{DataSourceID: "source-denied", DisplayName: "Denied", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.2", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "denied-user", State: "ENABLED", Revision: 1, CredentialRevision: 3},
		}},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		RequestID string                   `json:"requestId"`
		Items     []dataSourceListResponse `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if body.RequestID == "" || len(body.Items) != 1 || body.Items[0].ID != "source-allowed" {
		t.Fatalf("unexpected data source list: %#v", body)
	}
	if body.Items[0].CredentialRevision != 2 || body.Items[0].Username != "synthetic-user" {
		t.Fatalf("unexpected safe projection: %#v", body.Items[0])
	}
	for _, forbidden := range []string{"synthetic-sys-user", "synthetic-user@synthetic-tenant#synthetic-cluster", `"sysUser"`, `"password"`, `"sysPassword"`, `"credentialId"`, `"ciphertext"`, `"nonce"`} {
		if bytes.Contains(response.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("数据源列表不得包含敏感身份或凭据字段 %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestListExecutionNodeCandidatesFiltersUnauthorizedNodes(t *testing.T) {
	t.Parallel()
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity:       browserOnlyIdentityProvider{},
		Authorizer:     sliceAuthorizer{},
		NodeCandidates: staticNodeCandidateReader{},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Items []executionNodeCandidateResponse `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0] != (executionNodeCandidateResponse{ID: "node-1", DisplayName: "Allowed Node", Platform: "WINDOWS_AMD64"}) {
		t.Fatalf("unexpected node candidates: %#v", body.Items)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("node-denied")) {
		t.Fatal("无权节点不得出现在候选列表中")
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes?eligibleFor=UNKNOWN", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid filter status = %d, want %d", invalid.Code, http.StatusBadRequest)
	}
}

func TestDataSourceConnectionTestNodeCandidatesAllowDisabledDiagnosticsOnly(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	onlineAt := now.Add(-time.Minute)
	agent := &store.ExecutionNodeAgent{
		AgentID: "agent-1", FactsRevision: 1, LastHeartbeatAt: &onlineAt,
		CapacityTotal: 1, CapacityUsed: 0,
		EnvironmentFacts: store.AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64"},
	}
	manager := &recordingNodeManagementStore{nodes: map[string]store.ExecutionNode{
		"node-disabled": {
			NodeID: "node-disabled", DisplayName: "Disabled Diagnostic", Platform: "WINDOWS_AMD64",
			ManagementState: "DISABLED", Agent: agent,
		},
		"node-maintenance": {
			NodeID: "node-maintenance", DisplayName: "Maintenance", Platform: "WINDOWS_AMD64",
			ManagementState: "MAINTENANCE", Agent: agent,
		},
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: diagnosticNodeAuthorizer{}, NodeManagement: manager,
		HeartbeatTTL: 5 * time.Minute,
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes?eligibleFor=DATA_SOURCE_CONNECTION_TEST", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items []executionNodeCandidateResponse `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != "node-disabled" {
		t.Fatalf("connection test candidates = %#v", body.Items)
	}
}

func TestCreateExecutionNodeDefaultsToDisabledAndRejectsInvalidRoots(t *testing.T) {
	t.Parallel()
	manager := &recordingNodeManagementStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Roles: allowedRoleAuthorizer{}, NodeManagement: manager, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"Windows Node","platform":"WINDOWS_AMD64","allowedRoots":["E:\\ob-data\\exports"],"toolHome":"E:\\tools\\ob-loader-dumper-4.3.5","javaPath":"C:\\Java\\bin\\java.exe"}`))
	request.Header.Set("Idempotency-Key", "synthetic-node-create-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || manager.created.NodeID == "" || manager.created.Platform != "WINDOWS_AMD64" || len(manager.created.AllowedRoots) != 1 {
		t.Fatalf("create node response=%d input=%#v body=%s", response.Code, manager.created, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("ENABLED")) {
		t.Fatalf("new node response must not claim enablement: %s", response.Body.String())
	}
	forged := httptest.NewRecorder()
	forgedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"Forged Node","platform":"WINDOWS_AMD64","allowedRoots":["E:\\ob-data\\exports"],"toolHome":"E:\\tools\\ob-loader-dumper-4.3.5","javaPath":"C:\\Java\\bin\\java.exe","managementState":"ENABLED","agentAssociationStatus":"ASSOCIATED","environmentStatus":"NORMAL","acceptsNewTasks":true}`))
	forgedRequest.Header.Set("Idempotency-Key", "synthetic-node-forged-facts")
	handler.ServeHTTP(forged, forgedRequest)
	if forged.Code != http.StatusBadRequest || len(manager.nodes) != 1 {
		t.Fatalf("forged node facts response=%d nodes=%#v body=%s", forged.Code, manager.nodes, forged.Body.String())
	}

	invalid := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"Linux Node","platform":"LINUX_AMD64","allowedRoots":["E:\\ob-data\\exports"],"toolHome":"/opt/ob-loader-dumper-4.3.5","javaPath":"/usr/bin/java"}`))
	invalidRequest.Header.Set("Idempotency-Key", "synthetic-node-invalid-key")
	handler.ServeHTTP(invalid, invalidRequest)
	if invalid.Code != http.StatusUnprocessableEntity || !bytes.Contains(invalid.Body.Bytes(), []byte(`"field":"allowedRoots"`)) {
		t.Fatalf("invalid node response=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestRequestIDIsStableAcrossNodePersistenceAndResponse(t *testing.T) {
	manager := &recordingNodeManagementStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Roles: allowedRoleAuthorizer{}, NodeManagement: manager, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"UUID Node","platform":"WINDOWS_AMD64","allowedRoots":["E:\\ob-data\\exports"],"toolHome":"E:\\tools\\ob-loader-dumper-4.3.5","javaPath":"C:\\Java\\bin\\java.exe"}`))
	request.Header.Set("Idempotency-Key", "synthetic-request-id-stability")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	var body struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusCreated || manager.created.RequestID == "" || body.RequestID != manager.created.RequestID {
		t.Fatalf("request ID is not stable: status=%d persisted=%q response=%q", response.Code, manager.created.RequestID, body.RequestID)
	}
	if !identifier.IsCanonicalUUIDV4(body.RequestID) {
		t.Fatalf("request ID is not a canonical UUIDv4: %q", body.RequestID)
	}
}

func TestRequestIDGenerationFailureStopsBeforeAuthenticationAndPersistence(t *testing.T) {
	provider := &countingIdentityProvider{}
	manager := &countingNodeManagementStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: provider, Roles: allowedRoleAuthorizer{}, NodeManagement: manager, CSRF: allowedCSRF{},
		RequestIDGenerator: func() (string, error) {
			return "", errors.New("synthetic request ID failure")
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"Unavailable UUID Node","platform":"WINDOWS_AMD64","allowedRoots":["E:\\ob-data\\exports"],"toolHome":"E:\\tools\\ob-loader-dumper-4.3.5","javaPath":"C:\\Java\\bin\\java.exe"}`))
	request.Header.Set("Idempotency-Key", "synthetic-request-id-unavailable")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failure response: %v", err)
	}
	if response.Code != http.StatusServiceUnavailable || body["code"] != "REQUEST_ID_UNAVAILABLE" || body["retryable"] != true {
		t.Fatalf("unexpected request ID failure response: status=%d body=%#v", response.Code, body)
	}
	if _, found := body["requestId"]; found {
		t.Fatalf("request ID failure response must omit requestId: %#v", body)
	}
	if provider.browserCalls != 0 || manager.createCalls != 0 {
		t.Fatalf("request ID failure crossed a protected boundary: browserCalls=%d createCalls=%d", provider.browserCalls, manager.createCalls)
	}
}

func TestExecutionNodeManagementFiltersUnauthorizedAndUsesRevision(t *testing.T) {
	t.Parallel()
	manager := &recordingNodeManagementStore{nodes: map[string]store.ExecutionNode{
		"node-allowed": {NodeID: "node-allowed", DisplayName: "Allowed Node", Platform: "WINDOWS_AMD64", ManagementState: "DISABLED", AllowedRoots: []string{`E:\ob-data\exports`}, Revision: 1, CreatedAt: nodeFixtureTime, UpdatedAt: nodeFixtureTime},
		"node-denied":  {NodeID: "node-denied", DisplayName: "Denied Node", Platform: "LINUX_AMD64", ManagementState: "DISABLED", AllowedRoots: []string{"/var/lib/ob-data-orch"}, Revision: 1, CreatedAt: nodeFixtureTime, UpdatedAt: nodeFixtureTime},
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-allowed"}, NodeManagement: manager, CSRF: allowedCSRF{},
	})
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes", nil))
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte("node-allowed")) || bytes.Contains(list.Body.Bytes(), []byte("node-denied")) {
		t.Fatalf("node list response=%d body=%s", list.Code, list.Body.String())
	}

	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes/node-denied", nil))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("denied node detail status=%d, want 404", denied.Code)
	}

	update := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/execution-nodes/node-allowed", bytes.NewBufferString(`{"displayName":"Allowed Node Updated","platform":"WINDOWS_AMD64","allowedRoots":["E:\\ob-data\\exports"],"toolHome":"E:\\tools\\ob-loader-dumper-4.3.5","javaPath":"C:\\Java\\bin\\java.exe"}`))
	updateRequest.Header.Set("If-Match", `"rev-1"`)
	handler.ServeHTTP(update, updateRequest)
	if update.Code != http.StatusOK || manager.updated.ExpectedRevision != 1 || manager.updated.DisplayName != "Allowed Node Updated" {
		t.Fatalf("node update response=%d input=%#v body=%s", update.Code, manager.updated, update.Body.String())
	}
}

// TestExecutionNodeEnvironmentCheckFailureCarriesDiagnosableCode 验证环境检查 FAILED 时
// 列表/详情响应透传具体失败码（TOOL_RUNTIME_INVALID），而不是只有泛化的"环境检查发现异常"。
func TestExecutionNodeEnvironmentCheckFailureCarriesDiagnosableCode(t *testing.T) {
	t.Parallel()
	now := nodeFixtureTime
	completedAt := now.Add(5 * time.Minute)
	node := store.ExecutionNode{
		NodeID: "node-failed-env", DisplayName: "Failed Env Node", Platform: "WINDOWS_AMD64",
		ManagementState: "DISABLED", AllowedRoots: []string{`E:\ob-data\exports`},
		ToolHome: `E:\tools\ob-loader-dumper-4.3.5`, JavaPath: `C:\Java\bin\java.exe`,
		EnvironmentCheck: store.ExecutionNodeEnvironmentCheck{
			CheckID: "check-1", Status: "FAILED", Code: "TOOL_RUNTIME_INVALID", FactsRevision: 3, CompletedAt: &completedAt,
		},
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	node.Agent = &store.ExecutionNodeAgent{
		AgentID: "agent-1", ProtocolVersion: "obdo-r1", BootID: "boot-1",
		LastHeartbeatAt: &now, FactsRevision: 3,
		EnvironmentFacts: store.AgentEnvironmentFacts{
			OperatingSystem: "WINDOWS", Architecture: "AMD64",
			// 运行时配置摘要与登记配置一致，才能进入环境检查结果分支（而非配置漂移分支）。
			RuntimeConfigurationDigest: store.ExecutionNodeRuntimeConfigurationDigest(node),
		},
	}
	manager := &recordingNodeManagementStore{nodes: map[string]store.ExecutionNode{"node-failed-env": node}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-failed-env"},
		NodeManagement: manager, CSRF: allowedCSRF{},
	})
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes", nil))
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(`"TOOL_RUNTIME_INVALID"`)) {
		t.Fatalf("failed env-check list response=%d body=%s", list.Code, list.Body.String())
	}
	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes/node-failed-env", nil))
	if detail.Code != http.StatusOK || !bytes.Contains(detail.Body.Bytes(), []byte(`"TOOL_RUNTIME_INVALID"`)) ||
		!bytes.Contains(detail.Body.Bytes(), []byte(`"environmentStatus":"ABNORMAL"`)) {
		t.Fatalf("failed env-check detail response=%d body=%s", detail.Code, detail.Body.String())
	}
}

func TestGetDataSourceHidesUnauthorizedAndMissingObjects(t *testing.T) {
	t.Parallel()
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity:    browserOnlyIdentityProvider{},
		Authorizer:  sourceAuthorizer{allowedID: "source-allowed"},
		DataSources: staticDataSourceReader{},
	})
	for _, testCase := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/api/v1/data-sources/source-allowed", wantStatus: http.StatusOK},
		{path: "/api/v1/data-sources/source-denied", wantStatus: http.StatusNotFound},
		{path: "/api/v1/data-sources/source-missing", wantStatus: http.StatusNotFound},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			if response.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, testCase.wantStatus, response.Body.String())
			}
		})
	}
}

func TestGetDataSourceReturnsBusinessUsernameOnlyWithManagementScope(t *testing.T) {
	t.Parallel()
	const businessUsername = "business-user"
	const sysUsername = "sys-user"
	const combinedUsername = "business-user@synthetic-tenant#synthetic-cluster"
	sources := staticDataSources{summaries: []store.DataSourceSummary{{
		DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "ODP",
		CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881, ClusterName: "synthetic-cluster",
		TenantName: "synthetic-tenant", Username: businessUsername, SysUser: sysUsername, State: "ENABLED",
		Revision: 1, CredentialRevision: 2,
	}}}
	for _, testCase := range []struct {
		name         string
		allowWrite   bool
		wantUsername bool
	}{
		{name: "管理范围", allowWrite: true, wantUsername: true},
		{name: "仅读取范围", allowWrite: false, wantUsername: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
				Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: testCase.allowWrite}, DataSources: sources,
			})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/data-sources/source-allowed", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
			}
			var body struct {
				Item map[string]json.RawMessage `json:"item"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode detail: %v", err)
			}
			username, hasUsername := body.Item["username"]
			if hasUsername != testCase.wantUsername {
				t.Fatalf("username presence = %t, want %t: %s", hasUsername, testCase.wantUsername, response.Body.String())
			}
			if hasUsername && string(username) != `"business-user"` {
				t.Fatalf("username = %s, want %q", username, businessUsername)
			}
			for _, forbidden := range []string{"password", "sysPassword", "sysUser", "combinedUsername"} {
				if _, exists := body.Item[forbidden]; exists {
					t.Fatalf("detail unexpectedly returned %s: %s", forbidden, response.Body.String())
				}
			}
			if bytes.Contains(response.Body.Bytes(), []byte(sysUsername)) || bytes.Contains(response.Body.Bytes(), []byte(combinedUsername)) {
				t.Fatalf("detail returned a forbidden connection identity: %s", response.Body.String())
			}
			list := httptest.NewRecorder()
			handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil))
			if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(businessUsername)) || bytes.Contains(list.Body.Bytes(), []byte(sysUsername)) || bytes.Contains(list.Body.Bytes(), []byte(combinedUsername)) || bytes.Contains(list.Body.Bytes(), []byte(`"sysUser"`)) {
				t.Fatalf("list response=%d body=%s", list.Code, list.Body.String())
			}
		})
	}
}

func TestChangeDataSourceStateRequiresCSRFAndObjectWriteScope(t *testing.T) {
	t.Parallel()
	changer := &recordingStateChanger{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		StateChanger: changer, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-allowed:disable", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || changer.input.TargetState != "DISABLED" || changer.input.DataSourceID != "source-allowed" {
		t.Fatalf("disable response=%d input=%#v", response.Code, changer.input)
	}
	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-denied:enable", nil)
	deniedRequest.Header.Set("If-Match", `"rev-1"`)
	handler.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("denied status=%d, want 404", denied.Code)
	}
	withoutCSRF := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true}, StateChanger: changer,
	})
	missingCSRF := httptest.NewRecorder()
	missingCSRFRequest := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-allowed:disable", nil)
	missingCSRFRequest.Header.Set("If-Match", `"rev-1"`)
	withoutCSRF.ServeHTTP(missingCSRF, missingCSRFRequest)
	if missingCSRF.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing CSRF status=%d, want 503", missingCSRF.Code)
	}
}

func TestDeleteDataSourceReturnsPermanentDeleteOutcome(t *testing.T) {
	t.Parallel()
	deleter := &recordingDataSourceDeleter{result: store.DataSourceDeletionResult{Outcome: "DELETED"}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		Deleter: deleter, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/data-sources/source-allowed", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"outcome":"DELETED"`)) || deleter.deleteInput.DataSourceID != "source-allowed" || deleter.deleteInput.ExpectedRevision != 1 {
		t.Fatalf("delete response=%d body=%s input=%#v", response.Code, response.Body.String(), deleter.deleteInput)
	}
}

func TestArchiveDataSourceUsesExplicitArchiveAction(t *testing.T) {
	t.Parallel()
	deleter := &recordingDataSourceDeleter{result: store.DataSourceDeletionResult{Outcome: "ARCHIVED", Revision: 2}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		Deleter: deleter, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-allowed:archive", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"outcome":"ARCHIVED"`)) || deleter.archiveInput.DataSourceID != "source-allowed" || deleter.archiveInput.ExpectedRevision != 1 {
		t.Fatalf("archive response=%d body=%s input=%#v", response.Code, response.Body.String(), deleter.archiveInput)
	}
}

func TestDeleteDataSourceIneligibleReturnsCurrentLifecycleEligibility(t *testing.T) {
	t.Parallel()
	deleter := &recordingDataSourceDeleter{err: store.ErrDataSourceDeleteIneligible}
	sources := staticDataSources{summaries: []store.DataSourceSummary{{
		DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "PRODUCTION", ConnectionKind: "ODP", CompatibilityMode: "MYSQL",
		Host: "127.0.0.1", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 1,
		LifecycleEligibility: store.DataSourceLifecycleEligibility{
			Enable:  store.DataSourceLifecycleActionEligibility{ReasonCode: "ALREADY_ENABLED", Reason: "数据源当前已启用。"},
			Disable: store.DataSourceLifecycleActionEligibility{Allowed: true},
			Delete:  store.DataSourceLifecycleActionEligibility{ReasonCode: "UNFINISHED_TASKS_EXIST", Reason: "存在未完成任务，请等待任务结束后删除。"},
			Archive: store.DataSourceLifecycleActionEligibility{Allowed: true, ReferenceCount: 3},
		},
	}}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		DataSources: sources, Deleter: deleter, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/data-sources/source-allowed", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"DATA_SOURCE_DELETE_INELIGIBLE"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"reasonCode":"UNFINISHED_TASKS_EXIST"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"archive":{"allowed":true`)) {
		t.Fatalf("delete conflict response=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDeleteExecutionNodeReturnsActualOutcome(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name     string
		outcome  string
		revision int64
		revoked  bool
	}{
		{name: "physical delete", outcome: "DELETED", revision: 0},
		{name: "archive referenced", outcome: "ARCHIVED", revision: 2, revoked: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			deleter := &recordingExecutionNodeDeleter{result: store.ExecutionNodeDeletionResult{Outcome: testCase.outcome, Revision: testCase.revision, AgentAccessRevoked: testCase.revoked}}
			handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
				Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-allowed"},
				NodeDeleter: deleter, CSRF: allowedCSRF{},
			})
			request := httptest.NewRequest(http.MethodDelete, "/api/v1/execution-nodes/node-allowed", nil)
			request.Header.Set("If-Match", `"rev-1"`)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			var body struct {
				Outcome            string `json:"outcome"`
				Revision           int64  `json:"revision"`
				AgentAccessRevoked bool   `json:"agentAccessRevoked"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode deletion response: %v", err)
			}
			if response.Code != http.StatusOK || body.Outcome != testCase.outcome || body.Revision != testCase.revision || body.AgentAccessRevoked != testCase.revoked || deleter.input.NodeID != "node-allowed" || deleter.input.ExpectedRevision != 1 {
				t.Fatalf("delete response=%d body=%#v input=%#v", response.Code, body, deleter.input)
			}
		})
	}
}

func TestDeleteExecutionNodeRejectsRunningTask(t *testing.T) {
	t.Parallel()
	deleter := &recordingExecutionNodeDeleter{err: store.ErrExecutionNodeHasRunningTask}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-allowed"},
		NodeDeleter: deleter, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/execution-nodes/node-allowed", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("EXECUTION_NODE_RUNNING_TASK")) {
		t.Fatalf("running node delete response=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEnableDataSourceRequiresSuccessfulConnectionTest(t *testing.T) {
	t.Parallel()
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		StateChanger: rejectedEnableStateChanger{}, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-allowed:enable", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !bytes.Contains(response.Body.Bytes(), []byte("DATA_SOURCE_CONNECTION_TEST_REQUIRED")) {
		t.Fatalf("enable response=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDataSourceConnectionTestCreatesNodeBoundAgentRequest(t *testing.T) {
	t.Parallel()
	tester := &recordingDataSourceConnectionTestStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: connectionTestAuthorizer{}, CSRF: allowedCSRF{}, ConnectionTests: tester,
		ConnectionTestTTL: time.Minute, HeartbeatTTL: 5 * time.Minute,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-allowed:test-connection", strings.NewReader(`{"nodeId":"node-1"}`))
	request.Header.Set("If-Match", `"rev-1"`)
	request.Header.Set("Idempotency-Key", "connection-test-key-0001")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || tester.created.DataSourceID != "source-allowed" || tester.created.NodeID != "node-1" ||
		tester.created.ExpectedDataSourceRevision != 1 || tester.created.VerificationSource != "G2_SYNTHETIC" ||
		tester.created.HeartbeatFreshAfter.IsZero() || !tester.created.HeartbeatFreshAfter.Before(tester.created.CreatedAt) ||
		!identifier.IsCanonicalUUIDV4(tester.created.ConnectionTestID) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-user")) {
		t.Fatalf("connection test response=%d body=%s input=%#v", response.Code, response.Body.String(), tester)
	}
}

func TestDataSourceConnectionTest在显式授权时签发AgentJDBC租约(t *testing.T) {
	t.Parallel()
	tester := &recordingDataSourceConnectionTestStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: connectionTestAuthorizer{}, CSRF: allowedCSRF{}, ConnectionTests: tester,
		ConnectionTestTTL: time.Minute, HeartbeatTTL: 5 * time.Minute, AgentJDBCConnectionTestEnabled: true,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-allowed:test-connection", strings.NewReader(`{"nodeId":"node-1"}`))
	request.Header.Set("If-Match", `"rev-1"`)
	request.Header.Set("Idempotency-Key", "connection-test-jdbc-key-0001")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || tester.created.VerificationSource != "AGENT_JDBC" {
		t.Fatalf("显式 JDBC 测试创建响应=%d，来源=%q", response.Code, tester.created.VerificationSource)
	}
}

func TestDataSourceConnectionTestRejectsMissingNodeAtField(t *testing.T) {
	t.Parallel()
	tester := &recordingDataSourceConnectionTestStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: connectionTestAuthorizer{}, CSRF: allowedCSRF{}, ConnectionTests: tester,
		ConnectionTestTTL: time.Minute, HeartbeatTTL: 5 * time.Minute,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/source-allowed:test-connection", strings.NewReader(`{}`))
	request.Header.Set("If-Match", `"rev-1"`)
	request.Header.Set("Idempotency-Key", "connection-test-key-0002")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !bytes.Contains(response.Body.Bytes(), []byte(`"field":"nodeId"`)) || tester.created.ConnectionTestID != "" {
		t.Fatalf("missing node response=%d body=%s input=%#v", response.Code, response.Body.String(), tester.created)
	}
}

func TestDataSourceConnectionTestStatusKeepsSyntheticResultNonReal(t *testing.T) {
	t.Parallel()
	completedAt := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	tester := &recordingDataSourceConnectionTestStore{run: store.DataSourceConnectionTestRun{
		ConnectionTestID: "connection-test-1", DataSourceID: "source-allowed", NodeID: "node-1", NodeFactsRevision: 3,
		Status: "SUCCEEDED", ResultCode: "DATABASE_CONNECTED", VerificationSource: "G2_SYNTHETIC",
		CreatedAt: completedAt.Add(-time.Minute), CompletedAt: completedAt,
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: connectionTestAuthorizer{}, ConnectionTests: tester,
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/data-source-connection-tests/connection-test-1", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"realConnectionVerified":false`)) || bytes.Contains(response.Body.Bytes(), []byte("safeSummary")) {
		t.Fatalf("connection test status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDataSourceResponseIncludesLastTestTimeWithoutConnectionIdentity(t *testing.T) {
	t.Parallel()
	testedAt := time.Date(2026, time.July, 24, 3, 0, 0, 0, time.UTC)
	response := newDataSourceResponse(store.DataSourceSummary{
		DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL",
		Host: "127.0.0.1", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user",
		State: "ENABLED", Revision: 1, CredentialRevision: 2, LastTestStatus: "SUCCEEDED", LastTestedAt: &testedAt,
	}, false)
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal data source response: %v", err)
	}
	if !bytes.Contains(body, []byte(`"lastTestedAt":"2026-07-24T03:00:00Z"`)) || bytes.Contains(body, []byte("synthetic-user")) {
		t.Fatalf("unexpected data source response: %s", body)
	}
}

func TestUpdateDataSourceEncryptsPasswordAndKeepsItOutOfResponses(t *testing.T) {
	t.Parallel()
	keyring, err := credential.NewKeyring(map[string][]byte{"test-key": bytes.Repeat([]byte{9}, 32)})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	updater := &recordingUpdater{connectionTestInvalidated: true}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		DataSources: staticDataSourceReader{}, Updater: updater, CredentialRefs: staticCredentialReferenceReader{},
		Encryptor: keyring, CSRF: allowedCSRF{}, CredentialKeyID: "test-key",
	})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/data-sources/source-allowed", bytes.NewBufferString(`{"displayName":"Updated","password":"synthetic-rotated-password"}`))
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || updater.input.DisplayName != "Updated" || updater.input.Password == nil || len(updater.input.Password.Ciphertext) == 0 || !bytes.Contains(response.Body.Bytes(), []byte(`"state":"DISABLED"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"username":"synthetic-user"`)) {
		t.Fatalf("update response=%d input=%#v", response.Code, updater.input)
	}
	serialized, _ := json.Marshal(updater.input)
	if bytes.Contains(serialized, []byte("synthetic-rotated-password")) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-rotated-password")) || bytes.Contains(response.Body.Bytes(), []byte(`"sysUser"`)) {
		t.Fatal("plaintext password escaped update boundary")
	}
}

func TestUpdateDataSourceReturnsFieldErrorsForMergedFields(t *testing.T) {
	t.Parallel()
	updater := &recordingUpdater{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		DataSources: staticDataSourceReader{}, Updater: updater, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/data-sources/source-allowed", bytes.NewBufferString(`{"username":" ","port":70000}`))
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity || updater.input.DataSourceID != "" || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"DATA_SOURCE_FIELDS_INVALID"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"field":"username"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"field":"port"`)) {
		t.Fatalf("field error response=%d body=%s input=%#v", response.Code, response.Body.String(), updater.input)
	}
}

func TestUpdateDataSourceReturnsRefreshedLifecycleEligibility(t *testing.T) {
	t.Parallel()
	updater := &recordingUpdater{connectionTestInvalidated: true}
	reader := &sequenceDataSourceReader{summaries: []store.DataSourceSummary{
		{
			DataSourceID: "source-allowed", DisplayName: "test", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL",
			Host: "192.168.2.53", Port: 2883, ClusterName: "rlc_cdpV4", TenantName: "test", Username: "root",
			State: "ENABLED", Revision: 9, CredentialRevision: 2, LastTestStatus: "SUCCEEDED",
			LifecycleEligibility: store.DataSourceLifecycleEligibility{Enable: store.DataSourceLifecycleActionEligibility{ReasonCode: "ALREADY_ENABLED"}},
		},
		{
			DataSourceID: "source-allowed", DisplayName: "test", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL",
			Host: "192.168.2.184", Port: 2881, ClusterName: "", TenantName: "gth_mysql", Username: "test",
			State: "DISABLED", Revision: 10, CredentialRevision: 2,
			LifecycleEligibility: store.DataSourceLifecycleEligibility{
				Enable:  store.DataSourceLifecycleActionEligibility{ReasonCode: "CONNECTION_TEST_REQUIRED", Reason: "当前连接配置尚未通过基础连接测试，不能启用。"},
				Disable: store.DataSourceLifecycleActionEligibility{ReasonCode: "ALREADY_DISABLED", Reason: "数据源当前已禁用。"},
			},
		},
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		DataSources: reader, Updater: updater, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/data-sources/source-allowed", bytes.NewBufferString(`{"host":"192.168.2.184","port":2881,"clusterName":"","tenantName":"gth_mysql","username":"test","defaultDatabase":null}`))
	request.Header.Set("If-Match", `"rev-9"`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"state":"DISABLED"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"revision":10`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"reasonCode":"CONNECTION_TEST_REQUIRED"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"reasonCode":"ALREADY_ENABLED"`)) {
		t.Fatalf("refreshed update response=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUpdateDataSource普通字段更新保留sys账号(t *testing.T) {
	t.Parallel()
	updater := &recordingUpdater{}
	sources := staticDataSources{summaries: []store.DataSourceSummary{{
		DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "ODP",
		CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881, ClusterName: "synthetic-cluster",
		TenantName: "synthetic-tenant", Username: "synthetic-user", State: "DISABLED", Revision: 1,
		CredentialRevision: 1, SysUser: "root", SysCredentialID: "sys-credential-1", SysCredentialRevision: 1,
	}}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		DataSources: sources, Updater: updater, CSRF: allowedCSRF{},
	})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/data-sources/source-allowed", bytes.NewBufferString(`{"displayName":"Updated"}`))
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || updater.input.SysUser != "root" || updater.input.SysPassword != nil || updater.input.ClearSysCredential || !bytes.Contains(response.Body.Bytes(), []byte(`"username":"synthetic-user"`)) || bytes.Contains(response.Body.Bytes(), []byte("root")) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-user@synthetic-tenant#synthetic-cluster")) {
		t.Fatalf("ordinary update response=%d input=%#v body=%s", response.Code, updater.input, response.Body.String())
	}
}

func TestCreateDataSource幂等摘要区分sys账号(t *testing.T) {
	t.Parallel()
	base := dataSourceCreateRequest{
		DisplayName: "Synthetic", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL",
		Host: "127.0.0.1", Port: 2881, ClusterName: "cluster", TenantName: "tenant", Username: "user",
		Password: "synthetic-password", SysPassword: "synthetic-sys-password",
	}
	first, second := base, base
	first.SysUser, second.SysUser = "root", "operator"
	if createRequestDigest(first) == createRequestDigest(second) {
		t.Fatal("不同 sys 账号不能共享数据源创建幂等摘要")
	}
}

func TestExportDraftCreateAndPreviewStayWithinSyntheticCSVSlice(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	coordinator, err := agentstate.NewCoordinator(testClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Prechecks: prechecks, Tasks: tasks, Generator: generator, PrecheckTTL: time.Minute, Coordinator: coordinator, CSRF: allowedCSRF{},
	})
	body := `{"dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-draft-idempotency-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated || drafts.created.DraftID == "" || strings.Contains(drafts.created.ConfigJSON, "password") {
		t.Fatalf("create response=%d draft=%#v", created.Code, drafts.created)
	}
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusOK || !bytes.Contains(previewed.Body.Bytes(), []byte("-h127.0.0.1")) || !bytes.Contains(previewed.Body.Bytes(), []byte("-P2881")) || !bytes.Contains(previewed.Body.Bytes(), []byte("-usynthetic-user@synthetic-tenant#synthetic-cluster")) || !bytes.Contains(previewed.Body.Bytes(), []byte("-p ******")) || bytes.Contains(previewed.Body.Bytes(), []byte("--password")) {
		t.Fatalf("preview response does not preserve non-password parameters=%d body=%s", previewed.Code, previewed.Body.String())
	}
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-precheck-idempotency-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted || prechecks.created.DraftID != "draft-synthetic" || prechecks.created.CredentialRevision != 1 {
		t.Fatalf("precheck response=%d binding=%#v", prechecked.Code, prechecks.created)
	}
	precheckRead := httptest.NewRecorder()
	handler.ServeHTTP(precheckRead, httptest.NewRequest(http.MethodGet, "/api/v1/prechecks/precheck-synthetic", nil))
	if precheckRead.Code != http.StatusOK || !bytes.Contains(precheckRead.Body.Bytes(), []byte(`"results":[{"check":"DATABASE_CONNECTIVITY","status":"UNKNOWN","evidenceCode":"DATABASE_CONNECTION_UNAVAILABLE"}`)) {
		t.Fatalf("pending precheck query=%d body=%s", precheckRead.Code, precheckRead.Body.String())
	}
	tasks.run = prechecks.created
	tasks.run.Status, tasks.run.IntegrityStatus = "SUCCEEDED", "COMPLETE"
	submit := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:submit", bytes.NewBufferString(`{"precheckId":"precheck-synthetic"}`))
	submit.Header.Set("If-Match", `"rev-1"`)
	submit.Header.Set("Idempotency-Key", "synthetic-task-submit-key")
	submitted := httptest.NewRecorder()
	handler.ServeHTTP(submitted, submit)
	if submitted.Code != http.StatusCreated || tasks.input.TaskID == "" || !strings.Contains(tasks.input.PlannedArgvJSON, "-usynthetic-user@synthetic-tenant#synthetic-cluster") || strings.Contains(tasks.input.PlannedArgvJSON, "\"-p\"") {
		t.Fatalf("submit response=%d task=%#v", submitted.Code, tasks.input)
	}
	task := httptest.NewRecorder()
	handler.ServeHTTP(task, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+tasks.input.TaskID, nil))
	if task.Code != http.StatusOK || bytes.Contains(task.Body.Bytes(), []byte("synthetic_user")) || bytes.Contains(task.Body.Bytes(), []byte("plannedCommand")) || !bytes.Contains(task.Body.Bytes(), []byte(`"type":"OBDUMPER_EXPORT"`)) {
		t.Fatalf("task response=%d body=%s", task.Code, task.Body.String())
	}
}

func TestTaskListReturnsOnlySafeProjectionAndOpaqueCursor(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 7, 31, 4, 0, 0, 0, time.UTC)
	tasks := &recordingTaskStore{}
	for index := 0; index < taskListDefaultPageSize+1; index++ {
		tasks.listItems = append(tasks.listItems, store.TaskListItem{
			TaskID: fmt.Sprintf("task-%02d", index), CreatorSubjectID: "synthetic-subject",
			DataSourceID: "source-allowed", NodeID: "node-1", TaskType: "OBDUMPER_EXPORT",
			Database: "synthetic_db", Table: "synthetic_table", State: "RUNNING",
			ReconciliationRequired: index == 0, SubmittedAt: base.Add(-time.Duration(index) * time.Minute),
			UpdatedAt: base.Add(-time.Duration(index) * time.Minute),
		})
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("task list response=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	var body struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"nextCursor"`
		TotalPages int              `json:"totalPages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	if len(body.Items) != taskListDefaultPageSize || body.NextCursor == "" || body.TotalPages != 2 || tasks.listQuery.SubjectID != "synthetic-subject" || tasks.listQuery.Limit != taskListDefaultPageSize || tasks.countSubject != "synthetic-subject" {
		t.Fatalf("unexpected task page: items=%d cursor=%q query=%#v", len(body.Items), body.NextCursor, tasks.listQuery)
	}
	requestedSize := httptest.NewRecorder()
	handler.ServeHTTP(requestedSize, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?limit=20", nil))
	if requestedSize.Code != http.StatusOK || tasks.listQuery.Limit != 20 {
		t.Fatalf("requested task page size response=%d query=%#v", requestedSize.Code, tasks.listQuery)
	}
	serialized := response.Body.String()
	for _, forbidden := range []string{"plannedCommand", "snapshot", "configFingerprint", "error", "log", "creatorSubjectId"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("task list leaked forbidden field %q: %s", forbidden, serialized)
		}
	}
	if body.Items[0]["stageEvidence"] != "UNAVAILABLE" || body.Items[0]["progressEvidence"] != "UNAVAILABLE" || body.Items[0]["reconciliationRequired"] != true {
		t.Fatalf("task list invented evidence or lost reconciliation: %#v", body.Items[0])
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?cursor=not-a-cursor", nil))
	if invalid.Code != http.StatusBadRequest || tasks.listQuery.BeforeTaskID != "" {
		t.Fatalf("invalid cursor response=%d query=%#v", invalid.Code, tasks.listQuery)
	}
	if _, ok := decodeTaskListCursor(encodeTaskListCursor("subject-a", base, "task-1"), "subject-b"); ok {
		t.Fatal("task list cursor was reusable by another subject")
	}
}

func TestTaskListRejectsUnsupportedPageSize(t *testing.T) {
	t.Parallel()
	tasks := &recordingTaskStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
	for _, query := range []string{"?limit=9", "?limit=51", "?limit=10&limit=20", "?limit="} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks"+query, nil))
		if response.Code != http.StatusBadRequest || tasks.listQuery.Limit != 0 {
			t.Fatalf("unsupported task page size %q response=%d query=%#v", query, response.Code, tasks.listQuery)
		}
	}
}

func TestTaskListFailsClosedWhenAuthorizedPageCountIsUnavailable(t *testing.T) {
	t.Parallel()
	tasks := &recordingTaskStore{countErr: errors.New("synthetic count failure")}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if response.Code != http.StatusServiceUnavailable || tasks.listQuery.Limit != 0 {
		t.Fatalf("unavailable count response=%d query=%#v", response.Code, tasks.listQuery)
	}
}

func Test任务详情读取拆分为授权安全投影(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 31, 5, 0, 0, 0, time.UTC)
	tasks := &recordingTaskStore{summary: store.TaskSummary{
		TaskID: "task-read-1", CreatorSubjectID: "synthetic-subject", DataSourceID: "source-allowed", NodeID: "node-1", PrecheckID: "precheck-1",
		ConfigFingerprint: "synthetic-fingerprint", ToolVersion: "4.3.5-RELEASE", MetadataVersion: "metadata-v1", CapabilityVersion: "capability-v1",
		Database: "synthetic_db", Table: "synthetic_table", Format: "CSV", PlannedCommandRedacted: "obdumper --user ****** -p ****** --table synthetic_table",
		State: "RUNNING", ExecutionID: "execution-1", ReconciliationRequired: true, SubmittedAt: now.Add(-time.Minute), StartedAt: now.Add(-30 * time.Second), UpdatedAt: now,
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})

	for path, forbidden := range map[string][]string{
		"/api/v1/tasks/task-read-1":                  {"configFingerprint", "plannedCommand", "executionId", "snapshot"},
		"/api/v1/tasks/task-read-1/snapshot":         {"filePath", "logPath", "credential", "plannedCommand", "executionId"},
		"/api/v1/tasks/task-read-1/command-evidence": {"argv", "credential", "snapshot", "executionId"},
		"/api/v1/tasks/task-read-1/execution":        {"plannedCommand", "snapshot", "processEvidence", "resultSummary"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read %s response=%d headers=%v body=%s", path, response.Code, response.Header(), response.Body.String())
		}
		for _, field := range forbidden {
			if strings.Contains(response.Body.String(), field) {
				t.Fatalf("read %s leaked forbidden field %q: %s", path, field, response.Body.String())
			}
		}
	}
	command := httptest.NewRecorder()
	handler.ServeHTTP(command, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-read-1/command-evidence", nil))
	if !bytes.Contains(command.Body.Bytes(), []byte(`"kind":"PLANNED"`)) || !bytes.Contains(command.Body.Bytes(), []byte(`"redaction":"PASSWORD_ONLY"`)) {
		t.Fatalf("command evidence body=%s", command.Body.String())
	}
	execution := httptest.NewRecorder()
	handler.ServeHTTP(execution, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-read-1/execution", nil))
	if !bytes.Contains(execution.Body.Bytes(), []byte(`"reconciliationRequired":true`)) || !bytes.Contains(execution.Body.Bytes(), []byte(`"stageEvidence":"UNAVAILABLE"`)) {
		t.Fatalf("execution body=%s", execution.Body.String())
	}
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-read-1/snapshot/extra", nil))
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("invalid task read path response=%d body=%s", invalid.Code, invalid.Body.String())
	}
	tasks.summary.PlannedCommandRedacted = "obdumper -p synthetic-password"
	unsafeCommand := httptest.NewRecorder()
	handler.ServeHTTP(unsafeCommand, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-read-1/command-evidence", nil))
	if unsafeCommand.Code != http.StatusServiceUnavailable || bytes.Contains(unsafeCommand.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatalf("unsafe command response=%d body=%s", unsafeCommand.Code, unsafeCommand.Body.String())
	}
}

func TestExportDraftRejectsEnabledSourceWithoutSuccessfulConnectionTest(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	drafts := &recordingDraftStore{}
	source := mustStaticDataSourceReader()
	source.summaries[0].LastTestStatus = "FAILED"
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: source,
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Generator: generator, CSRF: allowedCSRF{},
	})
	body := `{"dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-draft-unverified-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
		t.Fatalf("create response=%d draft=%#v", created.Code, drafts.created)
	}

	drafts.created = store.ExportDraft{DraftID: "draft-unverified", OwnerSubjectID: "synthetic-subject", DataSourceID: "source-allowed", NodeID: "node-1", Revision: 1, ConfigVersion: "v5", ConfigJSON: body}
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-unverified:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusUnprocessableEntity {
		t.Fatalf("preview response=%d body=%s", previewed.Code, previewed.Body.String())
	}
}

// syntheticV6CSVBody 是 v6 泛化配置表达已验证单表 CSV 能力的固定合成请求体。
// rawInput 故意携带秘密样式的自由文本，验证控制面只持久化服务端规范值。
const syntheticV6CSVBody = `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"schema":"synthetic_db","name":"synthetic_table","rawInput":"synthetic-secret=leak-sample"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`

func TestExportDraftV6SingleTableCSVFollowsVerifiedSlice(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	coordinator, err := agentstate.NewCoordinator(testClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Prechecks: prechecks, Tasks: tasks, Generator: generator, PrecheckTTL: time.Minute, Coordinator: coordinator, CSRF: allowedCSRF{},
	})
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(syntheticV6CSVBody))
	create.Header.Set("Idempotency-Key", "synthetic-v6-draft-idempotency-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated || drafts.created.ConfigVersion != "v6" {
		t.Fatalf("create response=%d draft=%#v", created.Code, drafts.created)
	}
	// v6 标准文档必须内嵌扁平投影键与嵌套配置，且不含秘密字段。
	for _, required := range []string{`"configVersion":"v6"`, `"database":"synthetic_db"`, `"table":"synthetic_table"`, `"format":"CSV"`, `"config":{`} {
		if !strings.Contains(drafts.created.ConfigJSON, required) {
			t.Fatalf("v6 config json missing %s: %s", required, drafts.created.ConfigJSON)
		}
	}
	if strings.Contains(drafts.created.ConfigJSON, "password") || strings.Contains(drafts.created.ConfigJSON, "leak-sample") || strings.Contains(drafts.created.ObjectScopeJSON, "leak-sample") {
		t.Fatalf("v6 config json leaked secret material: %s / %s", drafts.created.ConfigJSON, drafts.created.ObjectScopeJSON)
	}
	if !strings.Contains(drafts.created.ConfigJSON, `"rawInput":"synthetic_db.synthetic_table"`) {
		t.Fatalf("v6 config json did not replace rawInput with canonical value: %s", drafts.created.ConfigJSON)
	}
	if drafts.created.ObjectScopeJSON == "{}" || drafts.created.DataFormatJSON == "{}" || drafts.created.OutputConfigJSON == "{}" || drafts.created.PerformanceConfigJSON == "{}" {
		t.Fatalf("v6 structured columns not persisted: %#v", drafts.created)
	}
	if !strings.Contains(draftResponse(drafts.created)["configVersion"].(string), "v6") {
		t.Fatalf("draft response missing configVersion")
	}
	// 预览命令必须与等价 v5 草稿一致：相同连接参数与密码脱敏形态。
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusOK || !bytes.Contains(previewed.Body.Bytes(), []byte("-h127.0.0.1")) || !bytes.Contains(previewed.Body.Bytes(), []byte("-usynthetic-user@synthetic-tenant#synthetic-cluster")) || !bytes.Contains(previewed.Body.Bytes(), []byte("-p ******")) || bytes.Contains(previewed.Body.Bytes(), []byte("--password")) {
		t.Fatalf("v6 preview does not match verified slice=%d body=%s", previewed.Code, previewed.Body.String())
	}
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-v6-precheck-idempotency-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted || prechecks.created.DraftID != "draft-synthetic" {
		t.Fatalf("v6 precheck response=%d binding=%#v", prechecked.Code, prechecks.created)
	}
	tasks.run = prechecks.created
	tasks.run.Status, tasks.run.IntegrityStatus = "SUCCEEDED", "COMPLETE"
	submit := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:submit", bytes.NewBufferString(`{"precheckId":"precheck-synthetic"}`))
	submit.Header.Set("If-Match", `"rev-1"`)
	submit.Header.Set("Idempotency-Key", "synthetic-v6-task-submit-key")
	submitted := httptest.NewRecorder()
	handler.ServeHTTP(submitted, submit)
	if submitted.Code != http.StatusCreated || tasks.input.SnapshotVersion != "v2" || tasks.input.SnapshotJSON != drafts.created.ConfigJSON {
		t.Fatalf("v6 submit response=%d task=%#v", submitted.Code, tasks.input)
	}
	snapshot := httptest.NewRecorder()
	handler.ServeHTTP(snapshot, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+tasks.input.TaskID+"/snapshot", nil))
	if snapshot.Code != http.StatusOK || !bytes.Contains(snapshot.Body.Bytes(), []byte(`"snapshotVersion":"v2"`)) || !bytes.Contains(snapshot.Body.Bytes(), []byte(`"capabilityVersion":"export-odp-single-table-csv-v1"`)) {
		t.Fatalf("v6 task snapshot response=%d body=%s", snapshot.Code, snapshot.Body.String())
	}
	for _, forbidden := range []string{"parentTaskId", "derivedFromTaskId", "templateId", "filePath", "logPath"} {
		if bytes.Contains(snapshot.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("v6 task snapshot leaked forbidden field %q: %s", forbidden, snapshot.Body.String())
		}
	}
}

func TestExportDraftV6DetectsInconsistentStoredConfiguration(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	drafts := &recordingDraftStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Generator: generator, CSRF: allowedCSRF{},
	})
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(syntheticV6CSVBody))
	create.Header.Set("Idempotency-Key", "synthetic-v6-inconsistency-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}

	// 扁平投影键与嵌套配置冲突时必须失败关闭，不得只按扁平字段继续执行。
	corrupted := store.ExportDraft{DraftID: drafts.created.DraftID, OwnerSubjectID: drafts.created.OwnerSubjectID, DataSourceID: drafts.created.DataSourceID, NodeID: drafts.created.NodeID, Revision: drafts.created.Revision, ConfigVersion: "v6", ConfigJSON: strings.Replace(drafts.created.ConfigJSON, `"table":"synthetic_table"`, `"table":"other_table"`, 1)}
	drafts.created = corrupted
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusServiceUnavailable || !bytes.Contains(previewed.Body.Bytes(), []byte("DRAFT_CONFIGURATION_UNAVAILABLE")) {
		t.Fatalf("inconsistent v6 preview response=%d body=%s", previewed.Code, previewed.Body.String())
	}

	// 标准文档内数据源绑定与草稿事实不一致时同样失败关闭。
	rebound := corrupted
	rebound.ConfigJSON = strings.Replace(corrupted.ConfigJSON, `"dataSourceId":"source-allowed"`, `"dataSourceId":"source-other"`, 1)
	drafts.created = rebound
	reboundResponse := httptest.NewRecorder()
	handler.ServeHTTP(reboundResponse, preview)
	if reboundResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("rebound v6 preview response=%d body=%s", reboundResponse.Code, reboundResponse.Body.String())
	}
}

// newGeneralizedFlowHandler 装配同时持有冻结与泛化生成器的测试处理器。
// prechecks 使用接口以便在门禁测试中注入真实 SQLite 仓储链。
func newGeneralizedFlowHandler(t *testing.T, drafts *recordingDraftStore, prechecks ExportPrecheckStore, tasks *recordingTaskStore) http.Handler {
	return newGeneralizedFlowHandlerWithStorageCredentials(t, drafts, prechecks, tasks, nil)
}

// newGeneralizedFlowHandlerWithStorageCredentials 为对象存储引用测试额外装配受控凭据读取边界。
func newGeneralizedFlowHandlerWithStorageCredentials(t *testing.T, drafts *recordingDraftStore, prechecks ExportPrecheckStore, tasks *recordingTaskStore, storageCredentials StorageCredentialStore) http.Handler {
	return newGeneralizedFlowHandlerWithDataSources(t, drafts, prechecks, tasks, storageCredentials, staticDataSourceReader{})
}

// newGeneralizedFlowHandlerWithDataSources 为租户互斥参数测试注入合成兼容模式。
func newGeneralizedFlowHandlerWithDataSources(t *testing.T, drafts *recordingDraftStore, prechecks ExportPrecheckStore, tasks *recordingTaskStore, storageCredentials StorageCredentialStore, sources DataSourceReader) http.Handler {
	t.Helper()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	generalized, err := commandgen.NewGeneralized()
	if err != nil {
		t.Fatalf("NewGeneralized() error = %v", err)
	}
	coordinator, err := agentstate.NewCoordinator(testClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	return NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: sources,
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Prechecks: prechecks, Tasks: tasks,
		StorageCredentials: storageCredentials, Generator: generator, GeneralizedGenerator: generalized, PrecheckTTL: time.Minute, Coordinator: coordinator, CSRF: allowedCSRF{},
	})
}

// runV6CapabilityFlow 执行创建→预览→预检查→提交→快照的完整流程并返回预览与快照响应体。
func runV6CapabilityFlow(t *testing.T, handler http.Handler, drafts *recordingDraftStore, prechecks *recordingPrecheckStore, tasks *recordingTaskStore, body, idempotencySuffix string) (string, string) {
	t.Helper()
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-exi2-"+idempotencySuffix+"-draft-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusOK {
		t.Fatalf("preview response=%d body=%s", previewed.Code, previewed.Body.String())
	}
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-exi2-"+idempotencySuffix+"-precheck-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted || prechecks.created.DraftID != "draft-synthetic" {
		t.Fatalf("precheck response=%d binding=%#v", prechecked.Code, prechecks.created)
	}
	tasks.run = prechecks.created
	tasks.run.Status, tasks.run.IntegrityStatus = "SUCCEEDED", "COMPLETE"
	submit := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:submit", bytes.NewBufferString(`{"precheckId":"precheck-synthetic"}`))
	submit.Header.Set("If-Match", `"rev-1"`)
	submit.Header.Set("Idempotency-Key", "synthetic-exi2-"+idempotencySuffix+"-submit-key")
	submitted := httptest.NewRecorder()
	handler.ServeHTTP(submitted, submit)
	if submitted.Code != http.StatusCreated {
		t.Fatalf("submit response=%d body=%s", submitted.Code, submitted.Body.String())
	}
	snapshot := httptest.NewRecorder()
	handler.ServeHTTP(snapshot, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+tasks.input.TaskID+"/snapshot", nil))
	if snapshot.Code != http.StatusOK {
		t.Fatalf("snapshot response=%d body=%s", snapshot.Code, snapshot.Body.String())
	}
	return previewed.Body.String(), snapshot.Body.String()
}

// TestHistoricalV6DraftReplaysFrozenMetadata 验证升级到 v7 后，已持久化的 v6 草稿仍按旧目录
// 重算 argv 与指纹，预检查不会因静默切换元数据版本而失效。
func TestHistoricalV6DraftReplaysFrozenMetadata(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"where":"id > 0"}}}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-historical-v6-draft-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated || drafts.created.MetadataVersion != exportMetadataV7 {
		t.Fatalf("create current draft response=%d draft=%#v body=%s", created.Code, drafts.created, created.Body.String())
	}
	currentFingerprint := drafts.created.ConfigFingerprint
	// 模拟升级前已由 v6 目录持久化的同一配置；配置本身不做任何迁移或重写。
	drafts.created.MetadataVersion = exportMetadataV6
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	var previewBody struct {
		ArgvTemplate      []string `json:"argvTemplate"`
		ConfigFingerprint string   `json:"configFingerprint"`
	}
	if err := json.Unmarshal(previewed.Body.Bytes(), &previewBody); err != nil || previewBody.ConfigFingerprint == "" {
		t.Fatalf("decode historical v6 preview: %v body=%s", err, previewed.Body.String())
	}
	argv := strings.Join(previewBody.ArgvTemplate, "\x00")
	if previewed.Code != http.StatusOK || !strings.Contains(argv, "-tsynthetic_table") || strings.Contains(argv, "--table") {
		t.Fatalf("historical v6 preview response=%d argv=%#v body=%s", previewed.Code, previewBody.ArgvTemplate, previewed.Body.String())
	}
	if previewBody.ConfigFingerprint == currentFingerprint {
		t.Fatal("v6 replay unexpectedly reused the v7 fingerprint")
	}
	drafts.created.ConfigFingerprint = previewBody.ConfigFingerprint
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-historical-v6-precheck-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted || prechecks.created.ConfigFingerprint != previewBody.ConfigFingerprint {
		t.Fatalf("historical v6 precheck response=%d binding=%#v body=%s", prechecked.Code, prechecks.created, prechecked.Body.String())
	}
}

func TestExportDraftV6FullCSVAllScopeFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "full-all")
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v7" {
		t.Fatalf("draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	for _, token := range []string{`"--all"`, `"--csv"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("full-csv preview missing %s: %s", token, previewBody)
		}
	}
	if strings.Contains(previewBody, `"--table"`) || strings.Contains(previewBody, "--password") {
		t.Fatalf("full-csv preview leaked table or password tokens: %s", previewBody)
	}
	if !strings.Contains(snapshotBody, `"snapshotVersion":"v2"`) || !strings.Contains(snapshotBody, `"format":"CSV"`) || !strings.Contains(snapshotBody, `"capabilityVersion":"export-odp-full-csv-v1"`) {
		t.Fatalf("full-csv snapshot unexpected: %s", snapshotBody)
	}
	if tasks.input.SnapshotVersion != "v2" || !strings.Contains(tasks.input.SnapshotJSON, `"scopeKind":"ALL"`) {
		t.Fatalf("full-csv task submission unexpected: %#v", tasks.input)
	}
}

func TestExportDraftV6FullCSVMultiTableWithExclusionsFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"table_one"},{"name":"table_two"}],"excludeTables":["table_tmp"]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "full-multi")
	if !strings.Contains(previewBody, `"--table","table_one,table_two"`) || !strings.Contains(previewBody, `"--exclude-table"`) || !strings.Contains(previewBody, `"table_tmp"`) {
		t.Fatalf("multi-table preview unexpected: %s", previewBody)
	}
	if !strings.Contains(snapshotBody, `"format":"CSV"`) || !strings.Contains(tasks.input.SnapshotJSON, `"table":"table_one,table_two"`) {
		t.Fatalf("multi-table snapshot unexpected: %s / %s", snapshotBody, tasks.input.SnapshotJSON)
	}
}

func TestExportDraftV6DDLOnlyFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"table_one"},{"name":"table_two"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "ddl-only")
	if drafts.created.CapabilityVersion != "export-odp-ddl-v1" {
		t.Fatalf("ddl draft capability=%s", drafts.created.CapabilityVersion)
	}
	if !strings.Contains(previewBody, `"--ddl"`) || strings.Contains(previewBody, `"--csv"`) {
		t.Fatalf("ddl-only preview must contain --ddl and no data format parameters: %s", previewBody)
	}
	if !strings.Contains(snapshotBody, `"format":"DDL"`) || !strings.Contains(snapshotBody, `"capabilityVersion":"export-odp-ddl-v1"`) {
		t.Fatalf("ddl-only snapshot unexpected: %s", snapshotBody)
	}
}

// TestExportDraftV6MixedObjectTypesDDLFlow 验证跨分类草稿到命令预览、预检查及快照的完整合成链路。
func TestExportDraftV6MixedObjectTypesDDLFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE","VIEW","FUNCTION","PROCEDURE","SEQUENCE"],"expressions":[{"objectType":"TABLE","name":"table_one"},{"objectType":"VIEW","name":"view_one"},{"objectType":"FUNCTION","name":"fn_one"},{"objectType":"PROCEDURE","name":"proc_one"},{"objectType":"SEQUENCE","name":"seq_one"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "mixed-ddl")
	if drafts.created.MetadataVersion != exportMetadataCombinedObjects {
		t.Fatalf("混合对象元数据版本 = %s", drafts.created.MetadataVersion)
	}
	for _, fragment := range []string{`"--table","table_one"`, `"--view","view_one"`, `"--function","fn_one"`, `"--procedure","proc_one"`, `"--sequence","seq_one"`} {
		if !strings.Contains(previewBody, fragment) {
			t.Fatalf("混合对象参数缺失 %s：%s", fragment, previewBody)
		}
	}
	if !strings.Contains(drafts.created.ObjectScopeJSON, `"objectTypes":["TABLE","VIEW","FUNCTION","PROCEDURE","SEQUENCE"]`) || tasks.input.SnapshotJSON != drafts.created.ConfigJSON || !strings.Contains(snapshotBody, exportMetadataCombinedObjects) {
		t.Fatalf("混合对象持久化或快照丢失分类：%s / %s", drafts.created.ObjectScopeJSON, snapshotBody)
	}
}

// TestExportDraftV6MixedObjectsWithTableDataFlow 验证表数据与其他对象定义可一同进入预览和冻结快照。
func TestExportDraftV6MixedObjectsWithTableDataFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE","VIEW","SEQUENCE"],"expressions":[{"objectType":"TABLE","name":"table_one"},{"objectType":"VIEW","name":"view_one"},{"objectType":"SEQUENCE","name":"seq_one"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "mixed-ddl-csv")
	if drafts.created.MetadataVersion != exportMetadataCombinedObjects || drafts.created.CapabilityVersion != "export-odp-ddl-csv-v1" {
		t.Fatalf("组合草稿版本错误：%s / %s", drafts.created.MetadataVersion, drafts.created.CapabilityVersion)
	}
	for _, fragment := range []string{`"--table","table_one"`, `"--view","view_one"`, `"--sequence","seq_one"`, `"--ddl"`, `"--csv"`} {
		if !strings.Contains(previewBody, fragment) {
			t.Fatalf("组合参数缺失 %s：%s", fragment, previewBody)
		}
	}
	if !strings.Contains(snapshotBody, exportMetadataCombinedObjects) {
		t.Fatalf("组合快照缺少元数据版本：%s", snapshotBody)
	}
}

func TestExportDraftV6DDLAndCSVFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"table_one"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "ddl-csv")
	if drafts.created.CapabilityVersion != "export-odp-ddl-csv-v1" {
		t.Fatalf("ddl-csv draft capability=%s", drafts.created.CapabilityVersion)
	}
	if !strings.Contains(previewBody, `"--ddl"`) || !strings.Contains(previewBody, `"--csv"`) || !strings.Contains(previewBody, `"--table","table_one"`) {
		t.Fatalf("ddl-csv preview unexpected: %s", previewBody)
	}
	if !strings.Contains(snapshotBody, `"format":"DDL_CSV"`) {
		t.Fatalf("ddl-csv snapshot unexpected: %s", snapshotBody)
	}
}

// TestExportDraftV6DDLTextFormatsFlow 验证结构与 CUT/SQL 数据组合的草稿、预览和冻结快照。
func TestExportDraftV6DDLTextFormatsFlow(t *testing.T) {
	for _, testCase := range []struct {
		format     string
		capability string
		flag       string
	}{
		{format: "CUT", capability: "export-odp-ddl-cut-v1", flag: "--cut"},
		{format: "SQL", capability: "export-odp-ddl-sql-v1", flag: "--sql"},
	} {
		t.Run(testCase.format, func(t *testing.T) {
			drafts := &recordingDraftStore{}
			prechecks := &recordingPrecheckStore{}
			tasks := &recordingTaskStore{}
			handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
			body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE","VIEW"],"expressions":[{"objectType":"TABLE","name":"table_one"},{"objectType":"VIEW","name":"view_one"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"` + testCase.format + `","csvOptions":{"fileEncoding":"UTF-8"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"performanceConfig":{"blockSize":"512"}}}`
			previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "ddl-"+strings.ToLower(testCase.format))
			if drafts.created.MetadataVersion != exportMetadataDDLTextFormats || drafts.created.CapabilityVersion != testCase.capability {
				t.Fatalf("组合草稿版本错误：%s / %s", drafts.created.MetadataVersion, drafts.created.CapabilityVersion)
			}
			for _, fragment := range []string{`"--table","table_one"`, `"--view","view_one"`, `"--ddl"`, `"` + testCase.flag + `"`, `"--file-encoding","UTF-8"`, `"--block-size","512"`} {
				if !strings.Contains(previewBody, fragment) {
					t.Fatalf("组合参数缺失 %s：%s", fragment, previewBody)
				}
			}
			if strings.Contains(previewBody, `"--csv"`) || strings.Contains(snapshotBody, `"format":"DDL_CSV"`) || !strings.Contains(snapshotBody, `"format":"DDL_`+testCase.format+`"`) {
				t.Fatalf("组合格式投影错误：%s / %s", previewBody, snapshotBody)
			}
		})
	}
}

// TestExportDraftV6DDLBehaviorFlow 验证 EX-I7 DDL 行为（2026-08-10）：
// 前置 DROP 与保留 Schema 仅随 DDL 内容发射，能力保持 ddl/ddl-csv。
func TestExportDraftV6DDLBehaviorFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"table_one"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"ddlBehavior":{"dropObject":true,"retainSchema":true}}}`
	previewBody, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi7-ddl")
	if drafts.created.CapabilityVersion != "export-odp-ddl-v1" {
		t.Fatalf("ddl behavior draft capability=%s", drafts.created.CapabilityVersion)
	}
	for _, token := range []string{`"--ddl"`, `"--drop-object"`, `"--retain-schema"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("ddl behavior preview missing %s: %s", token, previewBody)
		}
	}
}

// TestExportDraftV6DDLBehaviorDDLAndDataFlow 验证 DDL + 数据携带 DDL 行为时随 ddl-csv 能力发射。
func TestExportDraftV6DDLBehaviorDDLAndDataFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"table_one"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"ddlBehavior":{"dropObject":true}}}`
	previewBody, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi7-ddl-csv")
	if drafts.created.CapabilityVersion != "export-odp-ddl-csv-v1" {
		t.Fatalf("ddl-csv behavior draft capability=%s", drafts.created.CapabilityVersion)
	}
	for _, token := range []string{`"--ddl"`, `"--csv"`, `"--drop-object"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("ddl-csv behavior preview missing %s: %s", token, previewBody)
		}
	}
}

// TestExportDraftV6DDLBehaviorFailClosed 验证 DDL 行为越界失败关闭：
// 仅数据内容携带 DDL 行为、序列策略携带均拒绝；--add-extra-message 缺少 sys 权限预检查，
// 即使作用域为 DDL 内容也必须失败关闭。
func TestExportDraftV6DDLBehaviorFailClosed(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	cases := map[string]string{
		"仅数据携带 DDL 行为":   `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"ddlBehavior":{"dropObject":true}}}`,
		"仅数据携带紧凑 Schema": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"ddlBehavior":{"compactSchema":true}}}`,
		"仅数据携带附加对象信息":    `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"ddlBehavior":{"addExtraMessage":true}}}`,
		"携带序列策略":         `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"ddlBehavior":{"sequencePolicy":"RESTART"}}}`,
	}
	for name, body := range cases {
		create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		create.Header.Set("Idempotency-Key", "synthetic-ddl-behavior-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, create)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6Batch2ParametersFlow 验证 EX-I7 第二批的收缩边界：
// 只发射已观察到效果的分区、类型排除和 MySQL DATE/DATETIME 格式；其余能力继续失败关闭。
func TestExportDraftV6Batch2ParametersFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	dataBody := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"dateValueFormat":"yyyy/MM/dd","datetimeValueFormat":"yyyy/MM/dd HH:mm:ss"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"filterConfig":{"partition":"p0,p2","excludeDataTypes":["BLOB","decimal"]}}}`
	dataPreview, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, dataBody, "exi7-batch2-data")
	for _, expected := range []string{`"--partition"`, `"p0,p2"`, `"--exclude-data-types"`, `"BLOB,decimal"`, `"--date-value-format"`, `"yyyy/MM/dd"`, `"--datetime-value-format"`, `"yyyy/MM/dd HH:mm:ss"`} {
		if !strings.Contains(dataPreview, expected) {
			t.Fatalf("batch2 data preview missing %s: %s", expected, dataPreview)
		}
	}
	// --partition 与 --query-sql 互斥（官方约束，EX-F072）。
	conflictBody := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"partition":"p0","querySql":"select 1"}}}`
	conflict := httptest.NewRecorder()
	conflictRequest := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(conflictBody))
	conflictRequest.Header.Set("Idempotency-Key", "synthetic-exi7-batch2-conflict-key-000")
	handler.ServeHTTP(conflict, conflictRequest)
	if conflict.Code != http.StatusUnprocessableEntity {
		t.Fatalf("partition conflicts with query sql response=%d body=%s", conflict.Code, conflict.Body.String())
	}

	for name, body := range map[string]string{
		"隐藏主键缺少预检查":     `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"enableHiddenPk":true}}}`,
		"附加信息缺少 sys 权限": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"ddlBehavior":{"addExtraMessage":true}}}`,
		"未观察 TIME 行为":   `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"timeValueFormat":"HH:mm:ss"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`,
		"时间格式包含制表符":     `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"dateValueFormat":"yyyy\tMM"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`,
		"时间格式包含回车":      `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"dateValueFormat":"yyyy\rMM"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`,
		"时间格式包含换行":      `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"dateValueFormat":"yyyy\nMM"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		request.Header.Set("Idempotency-Key", "synthetic-exi7-gated-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s response=%d body=%s", name, response.Code, response.Body.String())
		}
	}
}

// TestExportDraftAcceptsOrdinaryQuerySQL 验证 --query-sql 作为普通高级筛选可创建草稿。
func TestExportDraftAcceptsOrdinaryQuerySQL(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"querySql":"select 1"}}}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	request.Header.Set("Idempotency-Key", "synthetic-query-sql-ordinary-key-000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || drafts.created.DraftID == "" {
		t.Fatalf("query-sql ordinary response=%d draft=%#v body=%s", response.Code, drafts.created, response.Body.String())
	}
}

// TestExportDraftQueryResultScope 验证结果集模式不需要对象参数，且条数上限进入固定命令预览。
func TestExportDraftQueryResultScope(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	handler := newGeneralizedFlowHandler(t, drafts, &recordingPrecheckStore{}, &recordingTaskStore{})
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"QUERY_RESULT"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"querySql":"SELECT id FROM synthetic_table","queryResultLimit":1000}}}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	request.Header.Set("Idempotency-Key", "synthetic-query-result-draft-key-000")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, request)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusOK || !strings.Contains(previewed.Body.String(), "LIMIT 1000") || !strings.Contains(previewed.Body.String(), "--query-sql") || strings.Contains(previewed.Body.String(), "--table") || strings.Contains(previewed.Body.String(), "--all") {
		t.Fatalf("query result preview=%d body=%s", previewed.Code, previewed.Body.String())
	}
}

// TestExportDraftQueryResultRejectsInvalidCombinations 验证范围、内容及 SQL 限制由服务端复核。
func TestExportDraftQueryResultRejectsInvalidCombinations(t *testing.T) {
	t.Parallel()
	handler := newGeneralizedFlowHandler(t, &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{})
	base := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"QUERY_RESULT"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"querySql":"SELECT id FROM synthetic_table","queryResultLimit":1000}}}`
	cases := map[string]string{
		"缺少条数上限":     strings.Replace(base, `,"queryResultLimit":1000`, "", 1),
		"多条 SQL":     strings.Replace(base, "SELECT id FROM synthetic_table", "SELECT 1; SELECT 2", 1),
		"仅结构":        strings.Replace(base, `"DATA_ONLY"`, `"DDL_ONLY"`, 1),
		"携带对象":       strings.Replace(base, `"scopeKind":"QUERY_RESULT"`, `"scopeKind":"QUERY_RESULT","objectTypes":["TABLE"]`, 1),
		"携带筛选":       strings.Replace(base, `"queryResultLimit":1000`, `"queryResultLimit":1000,"where":"id > 0"`, 1),
		"普通范围携带条数上限": strings.Replace(base, `"scopeKind":"QUERY_RESULT"`, `"scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]`, 1),
	}
	for name, body := range cases {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		request.Header.Set("Idempotency-Key", "synthetic-query-result-reject-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s response=%d body=%s", name, response.Code, response.Body.String())
		}
	}
}

// TestNormalizeExportConfigV6AllowsOrdinaryQuerySQL 验证归一化层保留普通查询筛选并交由统一生成器处理。
func TestNormalizeExportConfigV6AllowsOrdinaryQuerySQL(t *testing.T) {
	config := &store.ExportConfig{
		ObjectScope:      store.ObjectScope{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectTypes: []string{"TABLE"}, Expressions: []store.ObjectExpression{{Name: "synthetic_table"}}},
		ContentSelection: store.ContentSelection{ContentKind: "DATA_ONLY"},
		DataFormat:       store.DataFormat{FormatKind: "CSV"},
		OutputConfig:     store.OutputConfig{OutputKind: "LOCAL", FilePath: "/E:/tmp/out"},
		FilterConfig:     store.FilterConfig{QuerySql: "select 1"},
	}
	normalized, err := normalizeExportConfigV6(config, "source-allowed", "node-1")
	if err != nil || normalized.QuerySql != "select 1" {
		t.Fatalf("query-sql normalized=%#v err=%v", normalized, err)
	}
}

// TestExportDraftV7TimestampFormatsRequireMySQL 验证当前仅有 MySQL 行为证据的格式参数
// 不会在 Oracle 兼容模式下被错误发射。
func TestExportDraftV7TimestampFormatsRequireMySQL(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	generalized, err := commandgen.NewGeneralized()
	if err != nil {
		t.Fatalf("NewGeneralized() error = %v", err)
	}
	sources := mustStaticDataSourceReader()
	sources.summaries[0].CompatibilityMode = "ORACLE"
	drafts := &recordingDraftStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: sources,
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts,
		Generator: generator, GeneralizedGenerator: generalized, CSRF: allowedCSRF{},
	})
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"dateValueFormat":"yyyy/MM/dd"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	request.Header.Set("Idempotency-Key", "synthetic-oracle-timestamp-gate-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
		t.Fatalf("oracle timestamp format response=%d draft=%#v body=%s", response.Code, drafts.created, response.Body.String())
	}
}

// TestExportDraftV6RemainingParametersFlow 验证 EX-I7 剩余参数第一批（2026-08-11 受控实测定版）：
// --compact-schema 随表 DDL 内容发射；--where/--snapshot 随明确表范围的 CSV 数据发射。
func TestExportDraftV6RemainingParametersFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	ddlBody := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"ddlBehavior":{"compactSchema":true}}}`
	ddlPreview, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, ddlBody, "exi7-remaining-ddl")
	if !strings.Contains(ddlPreview, `"--compact-schema"`) {
		t.Fatalf("compact schema preview missing --compact-schema: %s", ddlPreview)
	}
	dataBody := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"filterConfig":{"where":"id > 0","snapshot":true}}}`
	dataPreview, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, dataBody, "exi7-remaining-data")
	// 值中的特殊字符在 JSON 预览中会被转义（如 > → \u003e），断言参数名与相邻值片段即可。
	for _, token := range []string{`"--where"`, `"--snapshot"`} {
		if !strings.Contains(dataPreview, token) {
			t.Fatalf("remaining parameters preview missing %s: %s", token, dataPreview)
		}
	}
}

// TestExportDraftV6RemainingParametersFailClosed 验证第一批参数的越界失败关闭：
// --where 仅限明确表范围且与 query-sql 互斥；--snapshot 与闪回互斥；
// --compact-schema 仅限表 DDL；--weak-read/--retry 在专用预检查与恢复链路完成前保持关闭。
func TestExportDraftV6RemainingParametersFailClosed(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	cases := map[string]string{
		"查询与条件互斥":            `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"querySql":"select * from synthetic_table","where":"id > 0"}}}`,
		"查询文件引用":             `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"querySql":"file:///E:/tmp/query.sql"}}}`,
		"全部范围携带条件筛选":         `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"where":"id > 0"}}}`,
		"一致性快照与闪回组合":         `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"snapshot":true,"flashbackScn":100}}}`,
		"数据内容携带紧凑 Schema":    `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"ddlBehavior":{"compactSchema":true}}}`,
		"视图 DDL 携带紧凑 Schema": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["VIEW"],"expressions":[{"name":"synthetic_view"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"ddlBehavior":{"compactSchema":true}}}`,
		"备库弱读缺少预检查":          `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"weakRead":true}}}`,
		"新建草稿携带保存点续跑":        `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"retry":true}}}`,
	}
	for name, body := range cases {
		create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		create.Header.Set("Idempotency-Key", "synthetic-remaining-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, create)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6BlockSizeFlow 验证 EX-I7 文件拆分（2026-08-10）：
// --block-size 显式传值（MB/ROW）随可读格式能力发射（2026-08-07 受控实测确认 MB/ROW 生效）。
func TestExportDraftV6BlockSizeFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"performanceConfig":{"blockSize":"1024MB"}}}`
	previewBody, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi7-block-size")
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" {
		t.Fatalf("block-size draft capability=%s", drafts.created.CapabilityVersion)
	}
	if !strings.Contains(previewBody, `"--block-size"`) || !strings.Contains(previewBody, `"1024MB"`) {
		t.Fatalf("block-size preview missing --block-size 1024MB: %s", previewBody)
	}
}

// TestExportDraftV6BlockSizeFailClosed 验证 --block-size 越界失败关闭：
// 非法值（非官方 MB/ROW 表达）与结构化格式携带均拒绝。
func TestExportDraftV6BlockSizeFailClosed(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	cases := map[string]string{
		"非法值 1GB":    `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"blockSize":"1GB"}}}`,
		"非法值 0":      `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"blockSize":"0"}}}`,
		"PARQUET 携带": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"PARQUET"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"blockSize":"1024MB"}}}`,
	}
	for name, body := range cases {
		create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		create.Header.Set("Idempotency-Key", "synthetic-block-size-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, create)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6CompressionLevelFlow 验证 EX-I7 压缩等级（2026-08-10）：
// --compression-level 按官方算法范围随压缩发射。
func TestExportDraftV6CompressionLevelFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","compress":true,"compressionAlgo":"zstd","compressionLevel":5}}}`
	previewBody, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi7-compression-level")
	for _, token := range []string{`"--compress"`, `"--compression-algo"`, `"zstd"`, `"--compression-level"`, `"5"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("compression-level preview missing %s: %s", token, previewBody)
		}
	}
}

// TestExportDraftV6CompressionLevelFailClosed 验证 --compression-level 越界失败关闭：
// 未启用压缩/算法未选择、算法越界（zstd 0/23、zlib 10）、gzip/snappy 携带等级均拒绝。
func TestExportDraftV6CompressionLevelFailClosed(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	cases := map[string]string{
		"未启用压缩携带等级":  `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compressionLevel":5}}}`,
		"zstd 越界 0":  `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compress":true,"compressionAlgo":"zstd","compressionLevel":0}}}`,
		"zstd 越界 23": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compress":true,"compressionAlgo":"zstd","compressionLevel":23}}}`,
		"zlib 越界 10": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compress":true,"compressionAlgo":"zlib","compressionLevel":10}}}`,
		"gzip 携带等级":  `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compress":true,"compressionAlgo":"gzip","compressionLevel":5}}}`,
	}
	for name, body := range cases {
		create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		create.Header.Set("Idempotency-Key", "synthetic-compression-level-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, create)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6CSVOptionsFlow 验证 EX-I3 全量选项进入命令且能力仍为 full-csv。
func TestExportDraftV6CSVOptionsFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","csvOptions":{"skipHeader":true,"columnSeparator":"|","columnQuote":"'","columnQuoteMode":"minimal","escapeCharacter":"\\","lineSeparator":"\\r\\n","nullString":"NULL","fileEncoding":"UTF-8","withTrim":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","noNestedDir":true,"maxFileSize":1048576,"retainEmptyFiles":true,"compress":true,"compressionAlgo":"zstd"},"filterConfig":{"includeColumnNames":["col_a","col_b"],"excludeVirtualColumns":true,"flashbackScn":100},"performanceConfig":{"thread":4,"pageSize":1000,"parallelMacro":8,"jvmMemory":"4G"}}}`
	previewBody, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi3-options")
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" {
		t.Fatalf("options draft capability=%s", drafts.created.CapabilityVersion)
	}
	for _, token := range []string{`"--skip-header"`, `"--column-separator"`, `"|"`, `"--column-quote-mode"`, `"minimal"`, `"--escape-character"`, `"--line-separator"`, `"--null-string"`, `"NULL"`, `"--file-encoding"`, `"UTF-8"`, `"--with-trim"`, `"--compress"`, `"--compression-algo"`, `"zstd"`, `"--no-nested-dir"`, `"--max-file-size"`, `"1048576"`, `"--retain-empty-files"`, `"--include-column-names"`, `"col_a,col_b"`, `"--exclude-virtual-columns"`, `"--flashback-scn"`, `"100"`, `"--thread"`, `"4"`, `"--page-size"`, `"1000"`, `"--parallel-macro"`, `"8"`, `"--mem"`, `"4G"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("EX-I3 preview missing %s: %s", token, previewBody)
		}
	}
}

// TestExportDraftV6CSVOptionsFailClosed 验证 EX-I3 选项的边界与互斥规则失败关闭。
func TestExportDraftV6CSVOptionsFailClosed(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	generalized, err := commandgen.NewGeneralized()
	if err != nil {
		t.Fatalf("NewGeneralized() error = %v", err)
	}
	newHandler := func() (http.Handler, *recordingDraftStore) {
		drafts := &recordingDraftStore{}
		handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
			Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
			CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts,
			Generator: generator, GeneralizedGenerator: generalized, CSRF: allowedCSRF{},
		})
		return handler, drafts
	}
	cases := map[string]struct {
		body string
	}{
		"非法包围模式":          {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","csvOptions":{"columnQuoteMode":"bogus"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"转义字符多字符":         {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","csvOptions":{"escapeCharacter":"ab"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"CSV 分隔符多字符":      {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","csvOptions":{"columnSeparator":"||"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"CSV 包围符多字符":      {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","csvOptions":{"columnQuote":"''"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"MySQL 游标抓取行数":    {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"fetchSize":100}}}`},
		"仅 DDL 携带 CSV 选项": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"dataFormat":{"csvOptions":{"skipHeader":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"算法未启用压缩":         {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compressionAlgo":"zstd"}}}`},
		"非法压缩算法":          {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compress":true,"compressionAlgo":"brotli"}}}`},
		"零值导出上限":          {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","maxFileSize":0}}}`},
		"查询与闪回互斥":         {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"querySql":"select 1","flashbackScn":100}}}`},
		"包含与排除列互斥":        {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"includeColumnNames":["a"],"excludeColumnNames":["b"]}}}`},
		"列名通配符":           {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"includeColumnNames":["a_*"]}}}`},
		"非法内存表达":          {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"jvmMemory":"4GX"}}}`},
		"零值资源参数":          {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"thread":0}}}`},
		"超长查询语句":          {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"querySql":"` + strings.Repeat("a", 70<<10) + `"}}}`},
	}
	for name, testCase := range cases {
		handler, drafts := newHandler()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(testCase.body))
		request.Header.Set("Idempotency-Key", "synthetic-exi3-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6CUTFlow 验证 EX-I4 CUT 格式：CUT 专属、共享文本与压缩选项进入命令，能力为 cut。
func TestExportDraftV6CUTFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CUT","cutOptions":{"trailDelimiter":true,"removeNewline":true},"csvOptions":{"columnSplitter":"||","escapeCharacter":"\\","lineSeparator":"\\n","nullString":"NULL","fileEncoding":"UTF-8","withTrim":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","compress":true,"compressionAlgo":"zstd","noNestedDir":true,"maxFileSize":1048576,"retainEmptyFiles":true},"filterConfig":{"includeColumnNames":["col_a","col_b"]},"performanceConfig":{"thread":4,"pageSize":1000,"jvmMemory":"4G"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi4-cut")
	if drafts.created.CapabilityVersion != "export-odp-cut-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v7" {
		t.Fatalf("cut draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	for _, token := range []string{`"--cut"`, `"--column-splitter"`, `"||"`, `"--trail-delimiter"`, `"--remove-newline"`, `"--escape-character"`, `"--line-separator"`, `"--null-string"`, `"NULL"`, `"--file-encoding"`, `"UTF-8"`, `"--with-trim"`, `"--compress"`, `"--compression-algo"`, `"zstd"`, `"--no-nested-dir"`, `"--max-file-size"`, `"1048576"`, `"--retain-empty-files"`, `"--include-column-names"`, `"col_a,col_b"`, `"--thread"`, `"4"`, `"--page-size"`, `"1000"`, `"--mem"`, `"4G"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("cut preview missing %s: %s", token, previewBody)
		}
	}
	for _, forbidden := range []string{`"--csv"`, `"--skip-header"`, `"--column-separator"`, `"--column-quote"`} {
		if strings.Contains(previewBody, forbidden) {
			t.Fatalf("cut preview must not contain %s: %s", forbidden, previewBody)
		}
	}
	if !strings.Contains(snapshotBody, `"format":"CUT"`) || !strings.Contains(snapshotBody, `"capabilityVersion":"export-odp-cut-v1"`) {
		t.Fatalf("cut snapshot unexpected: %s", snapshotBody)
	}
}

// TestExportDraftV6SQLFlow 验证 EX-I4 SQL 格式：仅行分隔符与文件编码进入命令，能力为 sql。
func TestExportDraftV6SQLFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"SQL","csvOptions":{"lineSeparator":"\\n","fileEncoding":"UTF-8"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","compress":true,"compressionAlgo":"gzip","retainEmptyFiles":true},"filterConfig":{"flashbackScn":100,"excludeColumnNames":["col_c"]},"performanceConfig":{"thread":4,"jvmMemory":"4G"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi4-sql")
	if drafts.created.CapabilityVersion != "export-odp-sql-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v7" {
		t.Fatalf("sql draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	for _, token := range []string{`"--sql"`, `"--line-separator"`, `"--file-encoding"`, `"UTF-8"`, `"--compress"`, `"--compression-algo"`, `"gzip"`, `"--retain-empty-files"`, `"--flashback-scn"`, `"100"`, `"--exclude-column-names"`, `"col_c"`, `"--thread"`, `"4"`, `"--mem"`, `"4G"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("sql preview missing %s: %s", token, previewBody)
		}
	}
	for _, forbidden := range []string{`"--csv"`, `"--cut"`, `"--trail-delimiter"`, `"--remove-newline"`, `"--with-trim"`, `"--escape-character"`, `"--null-string"`, `"--skip-header"`} {
		if strings.Contains(previewBody, forbidden) {
			t.Fatalf("sql preview must not contain %s: %s", forbidden, previewBody)
		}
	}
	if !strings.Contains(snapshotBody, `"format":"SQL"`) || !strings.Contains(snapshotBody, `"capabilityVersion":"export-odp-sql-v1"`) {
		t.Fatalf("sql snapshot unexpected: %s", snapshotBody)
	}
}

// TestExportDraftV6POSFlow 验证 EX-I4 POS 定版（2026-08-07 实测）：
// 独立 --pos 必须搭配 --ctl-path 控制文件目录；通用文件布局/筛选/性能参数随 POS 能力进入命令。
func TestExportDraftV6POSFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"POS"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","controlFilePath":"/E:/workespace/ob-data-orch/tmp/synthetic-controls","compress":true,"compressionAlgo":"zstd","noNestedDir":true},"performanceConfig":{"thread":4,"jvmMemory":"4G"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi4-pos")
	if drafts.created.CapabilityVersion != "export-odp-pos-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v7" {
		t.Fatalf("pos draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	for _, token := range []string{`"--pos"`, `"--ctl-path"`, `"/E:/workespace/ob-data-orch/tmp/synthetic-controls"`, `"--compress"`, `"--compression-algo"`, `"zstd"`, `"--no-nested-dir"`, `"--thread"`, `"4"`, `"--mem"`, `"4G"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("pos preview missing %s: %s", token, previewBody)
		}
	}
	for _, forbidden := range []string{`"--csv"`, `"--cut"`, `"--sql"`, `"--skip-header"`, `"--trail-delimiter"`} {
		if strings.Contains(previewBody, forbidden) {
			t.Fatalf("pos preview must not contain %s: %s", forbidden, previewBody)
		}
	}
	if !strings.Contains(snapshotBody, `"format":"POS"`) || !strings.Contains(snapshotBody, `"capabilityVersion":"export-odp-pos-v1"`) {
		t.Fatalf("pos snapshot unexpected: %s", snapshotBody)
	}
}

// TestExportDraftV6POSFailClosed 验证 POS 越界组合在服务端失败关闭：
// 其他格式携带控制文件路径、POS 携带序列化选项、POS 缺少控制文件。
func TestExportDraftV6POSFailClosed(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	cases := map[string]string{
		"CSV 携带控制文件路径":  `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","controlFilePath":"/E:/tmp/controls"}}}`,
		"POS 携带 CSV 选项": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"POS","csvOptions":{"skipHeader":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","controlFilePath":"/E:/tmp/controls"}}}`,
		"POS 携带 CUT 选项": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"POS","cutOptions":{"trailDelimiter":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","controlFilePath":"/E:/tmp/controls"}}}`,
		"POS 控制文件路径含换行": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"POS"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","controlFilePath":"/E:/tmp/ctrl\npath"}}}`,
	}
	for name, body := range cases {
		create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		create.Header.Set("Idempotency-Key", "synthetic-pos-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, create)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6StructuredFlow 验证 EX-I5 结构化格式（2026-08-07）：
// --par/--orc/--avro 各自能力进入命令，文件编码与通用筛选/性能参数随官方格式表活动，压缩不适用。
func TestExportDraftV6StructuredFlow(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		formatKind string
		capability string
		formatArg  string
	}{
		{"parquet", "PARQUET", "export-odp-parquet-v1", `"--par"`},
		{"orc", "ORC", "export-odp-orc-v1", `"--orc"`},
		{"avro", "AVRO", "export-odp-avro-v1", `"--avro"`},
	} {
		drafts := &recordingDraftStore{}
		prechecks := &recordingPrecheckStore{}
		tasks := &recordingTaskStore{}
		handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
		body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"` + testCase.formatKind + `","csvOptions":{"fileEncoding":"UTF-8"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","noNestedDir":true},"filterConfig":{"flashbackScn":100},"performanceConfig":{"thread":4,"jvmMemory":"4G"}}}`
		previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi5-"+testCase.name)
		if drafts.created.CapabilityVersion != testCase.capability || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v7" {
			t.Fatalf("%s draft capability=%s metadata=%s", testCase.name, drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
		}
		for _, token := range []string{testCase.formatArg, `"--file-encoding"`, `"UTF-8"`, `"--no-nested-dir"`, `"--flashback-scn"`, `"100"`, `"--thread"`, `"4"`, `"--mem"`, `"4G"`} {
			if !strings.Contains(previewBody, token) {
				t.Fatalf("%s preview missing %s: %s", testCase.name, token, previewBody)
			}
		}
		for _, forbidden := range []string{`"--csv"`, `"--cut"`, `"--pos"`, `"--sql"`, `"--compress"`, `"--escape-character"`} {
			if strings.Contains(previewBody, forbidden) {
				t.Fatalf("%s preview must not contain %s: %s", testCase.name, forbidden, previewBody)
			}
		}
		if !strings.Contains(snapshotBody, `"format":"`+testCase.formatKind+`"`) || !strings.Contains(snapshotBody, `"capabilityVersion":"`+testCase.capability+`"`) {
			t.Fatalf("%s snapshot unexpected: %s", testCase.name, snapshotBody)
		}
	}
}

// TestExportDraftV6StructuredFailClosed 验证结构化格式越界组合在服务端失败关闭：
// 压缩、序列化选项、CUT 选项与缺失格式均拒绝。
func TestExportDraftV6StructuredFailClosed(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	cases := map[string]string{
		"PARQUET 携带压缩":   `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"PARQUET"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out","compress":true,"compressionAlgo":"zstd"}}}`,
		"ORC 携带转义字符":     `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"ORC","csvOptions":{"escapeCharacter":"\\"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`,
		"AVRO 携带 CUT 选项": `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"AVRO","cutOptions":{"trailDelimiter":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`,
		"PARQUET 缺少格式":   `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`,
	}
	for name, body := range cases {
		create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		create.Header.Set("Idempotency-Key", "synthetic-structured-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, create)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6ObjectStorageFlow 验证 EX-I6 对象存储（2026-08-07，门禁 2026-08-10）：
// 受控 URI（无密钥参数）随输出类型进入命令，--tmp-path 本地临时分块目录随能力发射；
// 存储专用预检查（凭据、网络、权限、空间）完成前，固定预检查与提交保持功能门禁阻断。
func TestExportDraftV6ObjectStorageFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"OSS","filePath":"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou-internal.aliyuncs.com","tmpPath":"/E:/workespace/ob-data-orch/tmp/synthetic-upload-buffer"}}}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-exi6-oss-draft-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusOK {
		t.Fatalf("preview response=%d body=%s", previewed.Code, previewed.Body.String())
	}
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v7" {
		t.Fatalf("oss draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	previewBody := previewed.Body.String()
	for _, token := range []string{`"--csv"`, `"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou-internal.aliyuncs.com"`, `"--tmp-path"`, `"/E:/workespace/ob-data-orch/tmp/synthetic-upload-buffer"`} {
		if !strings.Contains(previewBody, token) {
			t.Fatalf("oss preview missing %s: %s", token, previewBody)
		}
	}
	if strings.Contains(previewBody, "access-key") || strings.Contains(previewBody, "secret-key") {
		t.Fatalf("oss preview must not carry storage credentials: %s", previewBody)
	}
	// EX-I6（2026-08-14）：对象存储草稿允许发起预检查；提交门禁改为按存储检查结果失败关闭。
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-exi6-oss-precheck-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted {
		t.Fatalf("oss precheck response=%d body=%s", prechecked.Code, prechecked.Body.String())
	}
	// 预检查结果没有通过的两项存储检查时，提交必须返回结果驱动门禁而非冻结任务。
	tasks.run = prechecks.created
	tasks.run.Status, tasks.run.IntegrityStatus = "SUCCEEDED", "COMPLETE"
	submit := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:submit", bytes.NewBufferString(`{"precheckId":"precheck-synthetic"}`))
	submit.Header.Set("If-Match", `"rev-1"`)
	submit.Header.Set("Idempotency-Key", "synthetic-exi6-oss-submit-key")
	submitted := httptest.NewRecorder()
	handler.ServeHTTP(submitted, submit)
	if submitted.Code != http.StatusUnprocessableEntity || !strings.Contains(submitted.Body.String(), "STORAGE_PRECHECK_REQUIRED") || tasks.input.TaskID != "" {
		t.Fatalf("oss submit gate response=%d body=%s task=%#v", submitted.Code, submitted.Body.String(), tasks.input)
	}
}

func TestObjectStoragePrecheckFreezesAuthorizedStorageCredential(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	storageCredentials := newRecordingStorageCredentialStore()
	storageCredentials.credentials["storage-precheck-1"] = store.StorageCredential{
		StorageCredentialID: "storage-precheck-1", OwnerSubjectID: "synthetic-subject", Provider: "OSS", CurrentRevision: 1, Revision: 1,
	}
	handler := newGeneralizedFlowHandlerWithStorageCredentials(t, drafts, prechecks, tasks, storageCredentials)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"OSS","filePath":"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com","tmpPath":"/E:/workespace/ob-data-orch/tmp/synthetic-staging","storageCredential":{"storageCredentialId":"storage-precheck-1","revision":1}}}}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-storage-credential-precheck-draft-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-storage-credential-precheck-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted || prechecks.created.StorageCredentialID != "storage-precheck-1" || prechecks.created.StorageCredentialRevision != 1 {
		t.Fatalf("precheck response=%d binding=%#v body=%s", prechecked.Code, prechecks.created, prechecked.Body.String())
	}
}

// TestExportDraftV6ObjectStorageFailClosed 验证对象存储 URI 越界在服务端失败关闭：
// 密钥参数进 URI、未知 scheme、未知参数、缺 bucket、scheme 与输出类型不符均拒绝。
func TestExportDraftV6ObjectStorageFailClosed(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	const base = `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{`
	cases := map[string]string{
		"URI 携带密钥参数":   base + `"outputKind":"OSS","filePath":"oss://bucket/path?endpoint=x&access-key=AK&secret-key=SK"}}}`,
		"未知 scheme":    base + `"outputKind":"S3","filePath":"ftp://bucket/path"}}}`,
		"scheme 与类型不符": base + `"outputKind":"OSS","filePath":"s3://bucket/path?endpoint=x"}}}`,
		"未知查询参数":       base + `"outputKind":"S3","filePath":"s3://bucket/path?region=x&unknown=y"}}}`,
		"缺 bucket":     base + `"outputKind":"COS","filePath":"cos:///path?region=x"}}}`,
		"路径非 / 开头":     base + `"outputKind":"OBS","filePath":"obs://bucket"}}}`,
		"临时目录含换行":      base + `"outputKind":"OSS","filePath":"oss://bucket/path?endpoint=x","tmpPath":"/E:/tmp/ctrl\npath"}}}`,
		"未知输出类型":       base + `"outputKind":"S3S","filePath":"/E:/tmp/out"}}}`,
	}
	for name, body := range cases {
		create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		create.Header.Set("Idempotency-Key", "synthetic-storage-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, create)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6CUTSQLFailClosed 验证 EX-I4 格式互斥与越界选项在服务端失败关闭。
func TestExportDraftV6CUTSQLFailClosed(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	generalized, err := commandgen.NewGeneralized()
	if err != nil {
		t.Fatalf("NewGeneralized() error = %v", err)
	}
	newHandler := func() (http.Handler, *recordingDraftStore) {
		drafts := &recordingDraftStore{}
		handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
			Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
			CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts,
			Generator: generator, GeneralizedGenerator: generalized, CSRF: allowedCSRF{},
		})
		return handler, drafts
	}
	const base = `"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"}`
	cases := map[string]struct {
		body string
	}{
		"CUT 携带 CSV 专属选项": {`{` + base + `,"dataFormat":{"formatKind":"CUT","csvOptions":{"skipHeader":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"CSV 携带 CUT 专属选项": {`{` + base + `,"dataFormat":{"formatKind":"CSV","cutOptions":{"trailDelimiter":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"SQL 携带修剪选项":      {`{` + base + `,"dataFormat":{"formatKind":"SQL","csvOptions":{"withTrim":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"SQL 携带 NULL 替换":  {`{` + base + `,"dataFormat":{"formatKind":"SQL","csvOptions":{"nullString":"NULL"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"SQL 携带转义字符":      {`{` + base + `,"dataFormat":{"formatKind":"SQL","csvOptions":{"escapeCharacter":"\\"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"SQL 携带 CUT 专属选项": {`{` + base + `,"dataFormat":{"formatKind":"SQL","cutOptions":{"removeNewline":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"DDL 与数据拒绝 POS":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"POS"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"仅 DDL 声明数据格式":    {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"dataFormat":{"formatKind":"SQL"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"POS 缺少控制文件":      {`{` + base + `,"dataFormat":{"formatKind":"POS"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
	}
	for name, testCase := range cases {
		handler, drafts := newHandler()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(testCase.body))
		request.Header.Set("Idempotency-Key", "synthetic-exi4-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d draft=%#v body=%s", name, response.Code, drafts.created, response.Body.String())
		}
	}
}

// TestExportDraftV6SingleTableWithOptionsLeavesFrozenPath 验证单表 + 选项改走泛化能力。
func TestExportDraftV6SingleTableWithOptionsLeavesFrozenPath(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","csvOptions":{"skipHeader":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	_, _ = runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi3-route")
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v7" {
		t.Fatalf("options draft route=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
}

// TestExportDraftV6FlashbackTimestampRequiresOracle 验证闪回时间点仅 Oracle 兼容模式。
func TestExportDraftV6FlashbackTimestampRequiresOracle(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"},"filterConfig":{"flashbackTimestamp":"2026-08-06 00:00:00"}}}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-exi3-flashback-key-0001")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
		t.Fatalf("mysql flashback timestamp response=%d draft=%#v body=%s", created.Code, drafts.created, created.Body.String())
	}
}

// TestExportDraftV6OracleFetchSizeFlow 验证 Oracle 租户可发送游标抓取行数，而 MySQL 负例由 CSV 选项门禁覆盖。
func TestExportDraftV6OracleFetchSizeFlow(t *testing.T) {
	t.Parallel()
	source, err := (staticDataSourceReader{}).GetDataSourceSummary(context.Background(), "source-allowed")
	if err != nil {
		t.Fatalf("GetDataSourceSummary() error = %v", err)
	}
	source.CompatibilityMode = "ORACLE"
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, prechecks, tasks, nil, staticDataSources{summaries: []store.DataSourceSummary{source}})
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"fetchSize":100}}}`
	previewBody, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "oracle-fetch-size")
	if !strings.Contains(previewBody, `"--fetch-size"`) || !strings.Contains(previewBody, `"100"`) {
		t.Fatalf("oracle preview missing fetch size: %s", previewBody)
	}
}

func TestExportDraftV6ViewDDLFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["VIEW"],"expressions":[{"name":"view_one"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "view-ddl")
	if !strings.Contains(previewBody, `"--view"`) || !strings.Contains(previewBody, `"view_one"`) || strings.Contains(previewBody, `"--table"`) {
		t.Fatalf("view ddl preview unexpected: %s", previewBody)
	}
	if !strings.Contains(snapshotBody, `"capabilityVersion":"export-odp-ddl-v1"`) || !strings.Contains(tasks.input.SnapshotJSON, `"table":"view_one"`) {
		t.Fatalf("view ddl snapshot unexpected: %s / %s", snapshotBody, tasks.input.SnapshotJSON)
	}
}

func TestExportDraftVersionRoutingFailsClosed(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	newHandler := func() (http.Handler, *recordingDraftStore) {
		drafts := &recordingDraftStore{}
		handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
			Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
			CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Generator: generator, CSRF: allowedCSRF{},
		})
		return handler, drafts
	}
	cases := map[string]struct {
		body string
		code int
	}{
		// 未验证能力必须 422 失败关闭。
		"全部对象范围":         {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"POS 缺少控制文件":     {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"POS"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"未声明数据格式":        {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"DDL+数据声明 POS":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"POS"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"仅 DDL 内容声明无效格式": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"dataFormat":{"formatKind":"CUT"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"对象存储输出":         {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"OSS","filePath":"oss://bucket/out?access-key=x&secret-key=y"}}}`, http.StatusUnprocessableEntity},
		// EX-F072 分区筛选（2026-08-13 实测定版）已启用：仅拒绝非表范围、DDL 内容与非法分区名。
		"分区筛选需表范围": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"partition":"p0"}}}`, http.StatusUnprocessableEntity},
		"分区名非法":    {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"partition":"p0; DROP"}}}`, http.StatusUnprocessableEntity},
		// EX-F075 类型排除（2026-08-13 实测定版）已启用：仅拒绝非法类型名与 DDL 内容。
		"排除数据类型需数据内容": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"excludeDataTypes":["BLOB"]}}}`, http.StatusUnprocessableEntity},
		"排除数据类型名非法":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"excludeDataTypes":["DECIMAL(10,2)"]}}}`, http.StatusUnprocessableEntity},
		// EX-F077 隐藏主键仍缺表结构、版本和权限预检查，任何 true 值均失败关闭。
		"隐藏主键缺少预检查": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"enableHiddenPk":true}}}`, http.StatusUnprocessableEntity},
		// 时间格式只启用 MySQL DATE/DATETIME 两项；即使已启用字段也必须满足格式与内容边界。
		"时间戳格式需 CSV/CUT": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"SQL","timestampFormats":{"dateValueFormat":"yyyy/MM/dd"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"时间戳格式串非法":       {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"dateValueFormat":"yyyy;DROP"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		// EX-I2 对象矩阵负例：gated 类型、跨库前缀、通配符、超限、非法组合均失败关闭。
		"gated 对象类型":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TRIGGER"],"expressions":[{"name":"synthetic_trigger"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"跨库 schema 前缀": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"schema":"other_db","name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"通配对象名":        {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_*"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"ALL 携带表达式":    {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL","expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"视图导出数据":       {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["VIEW"],"expressions":[{"name":"synthetic_view"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"视图携带排除表":      {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["VIEW"],"expressions":[{"name":"synthetic_view"}],"excludeTables":["other_view"]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"缺少范围数据库":      {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		// 版本路由与未知字段必须 400 失败关闭。
		"v5 携带泛化配置":   {`{"configVersion":"v5","dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/tmp/out","config":{"dataFormat":{"formatKind":"CSV"}}}`, http.StatusBadRequest},
		"v6 携带扁平字段":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","config":{"dataFormat":{"formatKind":"CSV"}}}`, http.StatusBadRequest},
		"v6 缺少泛化配置":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1"}`, http.StatusBadRequest},
		"未知配置版本":      {`{"configVersion":"v7","dataSourceId":"source-allowed","nodeId":"node-1","config":{"dataFormat":{"formatKind":"CSV"}}}`, http.StatusBadRequest},
		"v6 配置未知字段":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"dataFormat":{"formatKind":"CSV","mystery":1}}}`, http.StatusBadRequest},
		"v6 秘密字段失败关闭": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"outputConfig":{"filePath":"/E:/tmp/out","password":"synthetic-secret"}}}`, http.StatusBadRequest},
	}
	for name, testCase := range cases {
		handler, drafts := newHandler()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(testCase.body))
		request.Header.Set("Idempotency-Key", "synthetic-v6-negative-"+name+"-key-000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != testCase.code || drafts.created.DraftID != "" {
			t.Fatalf("%s response=%d want=%d draft=%#v body=%s", name, response.Code, testCase.code, drafts.created, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "synthetic-secret") {
			t.Fatalf("%s echoed secret in error body: %s", name, response.Body.String())
		}
	}
}

func TestExportDraftV5PersistenceRemainsByteIdentical(t *testing.T) {
	t.Parallel()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	drafts := &recordingDraftStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Generator: generator, CSRF: allowedCSRF{},
	})
	body := `{"dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","logPath":"","skipCheckDir":false}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	request.Header.Set("Idempotency-Key", "synthetic-v5-regression-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || drafts.created.ConfigVersion != "v5" {
		t.Fatalf("v5 create response=%d draft=%#v", response.Code, drafts.created)
	}
	// 存量 config_json 必须与首条切片字节级一致，结构化列保持默认空对象。
	if drafts.created.ConfigJSON != body {
		t.Fatalf("v5 config json changed: %s", drafts.created.ConfigJSON)
	}
	if drafts.created.ObjectScopeJSON != "" || drafts.created.DataFormatJSON != "" || drafts.created.DDLBehaviorJSON != "" {
		t.Fatalf("v5 draft must not carry structured columns: %#v", drafts.created)
	}
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-1"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusOK || bytes.Contains(previewed.Body.Bytes(), []byte(`"--log-path"`)) {
		t.Fatalf("blank v5 log path changed argv response=%d body=%s", previewed.Code, previewed.Body.String())
	}
	var previewBody struct {
		ConfigFingerprint string `json:"configFingerprint"`
	}
	if err := json.Unmarshal(previewed.Body.Bytes(), &previewBody); err != nil || previewBody.ConfigFingerprint != drafts.created.ConfigFingerprint {
		t.Fatalf("v5 fingerprint changed after replay: %v preview=%#v draft=%#v", err, previewBody, drafts.created)
	}
}

func TestSyntheticAgentPrecheckRequiresMachineIdentityAndLease(t *testing.T) {
	t.Parallel()
	coordinator, err := agentstate.NewCoordinator(testClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	binding := agentstate.PrecheckBinding{PrecheckID: "precheck-agent", NodeID: "node-1", DraftRevision: 1, ConfigFingerprint: strings.Repeat("a", 64), CredentialRevision: 1, NodeFactsVersion: 1}
	if err := coordinator.SchedulePrecheck(binding); err != nil {
		t.Fatalf("SchedulePrecheck() error = %v", err)
	}
	prechecks := &recordingPrecheckStore{created: store.PrecheckRun{PrecheckID: binding.PrecheckID, Status: "PENDING"}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: dualIdentityProvider{}, Coordinator: coordinator, Prechecks: prechecks, PrecheckTTL: time.Minute})
	claimBody := `{"requestId":"claim-1","precheckId":"precheck-agent","nodeId":"node-1","leaseId":"lease-1"}`
	claimed := httptest.NewRecorder()
	handler.ServeHTTP(claimed, httptest.NewRequest(http.MethodPost, "/agent/v1/prechecks:claim", bytes.NewBufferString(claimBody)))
	if claimed.Code != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", claimed.Code, claimed.Body.String())
	}
	var claim struct {
		Grant struct {
			LeaseEpoch int64 `json:"leaseEpoch"`
		} `json:"grant"`
	}
	if err := json.Unmarshal(claimed.Body.Bytes(), &claim); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	completeBody := fmt.Sprintf(`{"requestId":"complete-1","leaseId":"lease-1","leaseEpoch":%d,"succeeded":true}`, claim.Grant.LeaseEpoch)
	completed := httptest.NewRecorder()
	handler.ServeHTTP(completed, httptest.NewRequest(http.MethodPost, "/agent/v1/prechecks/precheck-agent:complete", bytes.NewBufferString(completeBody)))
	if completed.Code != http.StatusOK || prechecks.created.Status != "SUCCEEDED" {
		t.Fatalf("complete status=%d run=%#v", completed.Code, prechecks.created)
	}
}

func TestSyntheticAgentExecutionClaimAndEventStayWithinG2(t *testing.T) {
	t.Parallel()
	coordinator, err := agentstate.NewCoordinator(testClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	if err := coordinator.Schedule(agentstate.TaskSchedule{TaskID: "task-agent", NodeID: "node-1"}); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	executions := &recordingExecutionStore{}
	tasks := &recordingTaskStore{input: store.TaskSubmission{TaskID: "task-agent", CreatorSubjectID: "synthetic-subject", PlannedCommandRedacted: "obdumper --password ******", SubmittedAt: time.Now().UTC()}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: dualIdentityProvider{}, Coordinator: coordinator, Executions: executions, Tasks: tasks, LogLedger: logstream.NewBatchLedger(), PrecheckTTL: time.Minute})
	claim := httptest.NewRecorder()
	handler.ServeHTTP(claim, httptest.NewRequest(http.MethodPost, "/agent/v1/executions:claim", bytes.NewBufferString(`{"requestId":"claim-execution-1","taskId":"task-agent","executionId":"execution-agent","nodeId":"node-1","leaseId":"lease-agent"}`)))
	if claim.Code != http.StatusOK || executions.claim.ExecutionID != "execution-agent" || executions.claim.EventID != "scheduled-execution-agent" {
		t.Fatalf("claim response=%d record=%#v", claim.Code, executions.claim)
	}
	var grant struct {
		Grant agentstate.LeaseGrant `json:"grant"`
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &grant); err != nil {
		t.Fatalf("decode claim grant: %v", err)
	}
	renew := httptest.NewRecorder()
	renewBody := fmt.Sprintf(`{"requestId":"renew-execution-1","leaseId":"lease-agent","leaseEpoch":%d}`, grant.Grant.LeaseEpoch)
	handler.ServeHTTP(renew, httptest.NewRequest(http.MethodPost, "/agent/v1/executions/execution-agent:renew", bytes.NewBufferString(renewBody)))
	if renew.Code != http.StatusOK || executions.renew.ExecutionID != "execution-agent" {
		t.Fatalf("renew response=%d record=%#v", renew.Code, executions.renew)
	}
	eventBody := fmt.Sprintf(`{"eventId":"event-started","leaseId":"lease-agent","leaseEpoch":%d,"sequence":2,"type":"PROCESS_STARTED"}`, grant.Grant.LeaseEpoch)
	event := httptest.NewRecorder()
	handler.ServeHTTP(event, httptest.NewRequest(http.MethodPost, "/agent/v1/executions/execution-agent:events:append", bytes.NewBufferString(eventBody)))
	if event.Code != http.StatusOK || executions.event.EventSeq != 2 || executions.event.PayloadJSON != `{"mode":"synthetic"}` {
		t.Fatalf("event response=%d record=%#v", event.Code, executions.event)
	}
	eventGap := httptest.NewRecorder()
	eventGapBody := fmt.Sprintf(`{"eventId":"event-gap","leaseId":"lease-agent","leaseEpoch":%d,"sequence":5,"type":"PROCESS_EXITED"}`, grant.Grant.LeaseEpoch)
	handler.ServeHTTP(eventGap, httptest.NewRequest(http.MethodPost, "/agent/v1/executions/execution-agent:events:append", bytes.NewBufferString(eventGapBody)))
	if eventGap.Code != http.StatusConflict {
		t.Fatalf("event gap response=%d body=%s", eventGap.Code, eventGap.Body.String())
	}
	logBody := fmt.Sprintf(`{"leaseId":"lease-agent","leaseEpoch":%d,"batch":{"streamId":"execution-agent","sourceEpoch":1,"firstSeq":1,"lastSeq":1,"previousDigest":"","policyVersion":"policy-v1","records":[{"streamId":"execution-agent","sourceEpoch":1,"sourceSeq":1,"kind":"LOG","message":"safe synthetic log","policyVersion":"policy-v1"}]}}`, grant.Grant.LeaseEpoch)
	logged := httptest.NewRecorder()
	handler.ServeHTTP(logged, httptest.NewRequest(http.MethodPost, "/agent/v1/executions/execution-agent:logs:append", bytes.NewBufferString(logBody)))
	if logged.Code != http.StatusAccepted {
		t.Fatalf("log response=%d body=%s", logged.Code, logged.Body.String())
	}
	unsafeLogBody := fmt.Sprintf(`{"leaseId":"lease-agent","leaseEpoch":%d,"batch":{"streamId":"execution-agent","sourceEpoch":1,"firstSeq":2,"lastSeq":2,"previousDigest":"","policyVersion":"policy-v1","records":[{"streamId":"execution-agent","sourceEpoch":1,"sourceSeq":2,"kind":"LOG","message":"password=unsafe-value","policyVersion":"policy-v1"}]}}`, grant.Grant.LeaseEpoch)
	unsafeLogged := httptest.NewRecorder()
	handler.ServeHTTP(unsafeLogged, httptest.NewRequest(http.MethodPost, "/agent/v1/executions/execution-agent:logs:append", bytes.NewBufferString(unsafeLogBody)))
	if unsafeLogged.Code != http.StatusBadRequest || bytes.Contains(unsafeLogged.Body.Bytes(), []byte("unsafe-value")) {
		t.Fatalf("unsafe log response=%d body=%s", unsafeLogged.Code, unsafeLogged.Body.String())
	}
	gapBody := fmt.Sprintf(`{"leaseId":"lease-agent","leaseEpoch":%d,"gap":{"streamId":"execution-agent","sourceKind":"OBDUMPER_STDOUT","sourceEpoch":1,"firstSeq":2,"lastSeq":3,"reasonCode":"SYNTHETIC_GAP","policyVersion":"policy-v1","parserVersion":"synthetic-parser-v1"}}`, grant.Grant.LeaseEpoch)
	gapped := httptest.NewRecorder()
	handler.ServeHTTP(gapped, httptest.NewRequest(http.MethodPost, "/agent/v1/executions/execution-agent:logs:gap", bytes.NewBufferString(gapBody)))
	if gapped.Code != http.StatusAccepted {
		t.Fatalf("log gap response=%d body=%s", gapped.Code, gapped.Body.String())
	}
	logs := httptest.NewRecorder()
	handler.ServeHTTP(logs, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-agent/logs", nil))
	if logs.Code != http.StatusOK || !bytes.Contains(logs.Body.Bytes(), []byte("safe synthetic log")) || !bytes.Contains(logs.Body.Bytes(), []byte("SYNTHETIC_MEMORY")) || !bytes.Contains(logs.Body.Bytes(), []byte(`"sourceSeq":1`)) || bytes.Contains(logs.Body.Bytes(), []byte(`"SourceSeq"`)) {
		t.Fatalf("logs response=%d body=%s", logs.Code, logs.Body.String())
	}
}

func Test任务日志流穿透请求标识包装并支持刷新(t *testing.T) {
	persistent, err := logstream.NewPersistentStore(t.TempDir(), emptyLogBatchIndex{})
	if err != nil {
		t.Fatalf("NewPersistentStore() error = %v", err)
	}
	defer persistent.Close()

	tasks := &recordingTaskStore{summary: store.TaskSummary{
		TaskID: "task-running", CreatorSubjectID: "synthetic-subject", State: "RUNNING", SubmittedAt: nodeFixtureTime,
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Tasks: tasks, PersistentLogs: persistent,
	})
	context, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-running/logs/stream", nil).WithContext(context)
	response := newStreamingResponseWriter()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()

	select {
	case <-response.flushed:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("日志流未在初始重试事件后刷新响应")
	}
	if response.status != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" || response.Body.String() != "retry: 2000\n\n" {
		t.Fatalf("日志流初始响应 status=%d headers=%#v body=%q", response.status, response.Header(), response.Body.String())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("取消请求后日志流未结束")
	}
}

func Test任务日志流拒绝不支持刷新的响应(t *testing.T) {
	persistent, err := logstream.NewPersistentStore(t.TempDir(), emptyLogBatchIndex{})
	if err != nil {
		t.Fatalf("NewPersistentStore() error = %v", err)
	}
	defer persistent.Close()

	tasks := &recordingTaskStore{summary: store.TaskSummary{
		TaskID: "task-running", CreatorSubjectID: "synthetic-subject", State: "RUNNING", SubmittedAt: nodeFixtureTime,
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Tasks: tasks, PersistentLogs: persistent,
	})
	response := newNonStreamingResponseWriter()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-running/logs/stream", nil))
	if response.status != http.StatusServiceUnavailable || !bytes.Contains(response.Body.Bytes(), []byte("LOG_STREAM_UNAVAILABLE")) {
		t.Fatalf("不支持刷新的日志流响应 status=%d body=%s", response.status, response.Body.String())
	}
}

func Test任务日志流通过最后事件标识续传新增记录(t *testing.T) {
	index := &reconnectLogBatchIndex{}
	persistent, err := logstream.NewPersistentStore(t.TempDir(), index)
	if err != nil {
		t.Fatalf("NewPersistentStore() error = %v", err)
	}
	defer persistent.Close()

	previousDigest := appendReconnectLogBatch(t, persistent, 1, "", "第一条合成续传日志")
	tasks := &recordingTaskStore{summary: store.TaskSummary{
		TaskID: "task-running", CreatorSubjectID: "synthetic-subject", State: "RUNNING", SubmittedAt: nodeFixtureTime,
	}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Tasks: tasks, PersistentLogs: persistent,
	})

	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstRequest := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-running/logs/stream", nil).WithContext(firstContext)
	firstResponse := newStreamingResponseWriter()
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(firstResponse, firstRequest)
		close(firstDone)
	}()
	waitForTaskLogStreamFlush(t, firstResponse)
	waitForTaskLogStreamFlush(t, firstResponse)
	firstBody := firstResponse.Body.String()
	firstEventID := taskLogEventID(firstBody)
	if firstResponse.status != http.StatusOK || firstEventID == "" || !strings.Contains(firstBody, "第一条合成续传日志") {
		cancelFirst()
		<-firstDone
		t.Fatalf("首次日志流响应 status=%d body=%q", firstResponse.status, firstBody)
	}
	cancelFirst()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("首次日志流取消后未结束")
	}

	appendReconnectLogBatch(t, persistent, 2, previousDigest, "第二条合成续传日志")
	secondContext, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	secondRequest := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-running/logs/stream", nil).WithContext(secondContext)
	secondRequest.Header.Set("Last-Event-ID", firstEventID)
	secondResponse := newStreamingResponseWriter()
	secondDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(secondResponse, secondRequest)
		close(secondDone)
	}()
	waitForTaskLogStreamFlush(t, secondResponse)
	waitForTaskLogStreamFlush(t, secondResponse)
	secondBody := secondResponse.Body.String()
	if secondResponse.status != http.StatusOK || !strings.Contains(secondBody, "第二条合成续传日志") || strings.Contains(secondBody, "第一条合成续传日志") {
		cancelSecond()
		<-secondDone
		t.Fatalf("续传日志流响应 status=%d body=%q", secondResponse.status, secondBody)
	}
	cancelSecond()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("续传日志流取消后未结束")
	}
}

func appendReconnectLogBatch(t *testing.T, persistent *logstream.PersistentStore, sequence int64, previousDigest, message string) string {
	t.Helper()
	result, err := persistent.Append(context.Background(), "execution-running", logstream.Batch{
		StreamID: "synthetic-stream", SourceEpoch: 1, FirstSeq: sequence, LastSeq: sequence,
		PreviousDigest: previousDigest, PolicyVersion: "policy-v1",
		Records: []logstream.Record{{
			StreamID: "synthetic-stream", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: sequence,
			Kind: logstream.RecordLog, Message: message, PolicyVersion: "policy-v1", ParserVersion: "synthetic-parser-v1", ReceivedAt: nodeFixtureTime,
		}},
	})
	if err != nil || result.Decision != logstream.BatchAccepted {
		t.Fatalf("追加合成日志批次 result=%#v error=%v", result, err)
	}
	return result.LastDigest
}

func waitForTaskLogStreamFlush(t *testing.T, response *streamingResponseWriter) {
	t.Helper()
	select {
	case <-response.flushed:
	case <-time.After(time.Second):
		t.Fatal("日志流未在预期事件后刷新响应")
	}
}

func taskLogEventID(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "id: ") {
			return strings.TrimPrefix(line, "id: ")
		}
	}
	return ""
}

func Test任务日志游标绑定主体和任务(t *testing.T) {
	cursor := &logstream.PageCursor{
		Snapshot:     logstream.BatchPosition{ReceivedAt: time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC), BatchID: strings.Repeat("a", 64)},
		Position:     logstream.BatchPosition{ReceivedAt: time.Date(2026, time.July, 31, 12, 0, 1, 0, time.UTC), BatchID: strings.Repeat("b", 64)},
		RecordOffset: 3,
	}
	encoded := encodeTaskLogCursor(cursor, "subject-1", "task-1")
	decoded, ok := decodeTaskLogCursor(encoded, "subject-1", "task-1")
	if !ok || decoded == nil || decoded.RecordOffset != 3 || decoded.Position.BatchID != cursor.Position.BatchID {
		t.Fatalf("decodeTaskLogCursor() = %#v, %v", decoded, ok)
	}
	if _, ok := decodeTaskLogCursor(encoded, "subject-2", "task-1"); ok {
		t.Fatal("other subject accepted a task log cursor")
	}
	if _, ok := decodeTaskLogCursor(encoded, "subject-1", "task-2"); ok {
		t.Fatal("other task accepted a task log cursor")
	}
}

func TestInjectedIdentityIsDomainSeparatedAndDoesNotCreateAPIAccess(t *testing.T) {
	t.Parallel()
	provider := staticIdentityProvider{}
	handler := NewHandlerWithIdentity(buildinfo.Info{Version: "test"}, provider)
	for _, testCase := range []struct {
		path       string
		wantStatus int
		wantCode   string
	}{
		{path: "/api/v1/data-sources", wantStatus: http.StatusServiceUnavailable, wantCode: "API_DEPENDENCY_NOT_CONFIGURED"},
		{path: "/agent/v1/heartbeats", wantStatus: http.StatusUnauthorized, wantCode: "AGENT_AUTHENTICATION_FAILED"},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Code != testCase.wantStatus || body["code"] != testCase.wantCode {
				t.Fatalf("response status=%d body=%#v", response.Code, body)
			}
		})
	}
}

func TestAgentEnrollmentAndHeartbeatUseStrictSecretSafeBoundaries(t *testing.T) {
	protocol := &recordingAgentProtocol{identity: store.AgentIdentity{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1"}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-1"}, AgentProtocol: protocol,
		CSRF: allowedCSRF{}, EnrollmentTTL: 10 * time.Minute,
	})

	issue := httptest.NewRecorder()
	handler.ServeHTTP(issue, httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes/node-1:enrollments", nil))
	if issue.Code != http.StatusCreated || issue.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("enrollment issue response=%d headers=%#v body=%s", issue.Code, issue.Header(), issue.Body.String())
	}
	var issued struct {
		EnrollmentID       string `json:"enrollmentId"`
		NodeID             string `json:"nodeId"`
		EnrollmentMaterial string `json:"enrollmentMaterial"`
		DisplayedOnce      bool   `json:"displayedOnce"`
	}
	if err := json.Unmarshal(issue.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode enrollment issue response: %v", err)
	}
	issuedDigest := sha256.Sum256([]byte(issued.EnrollmentMaterial))
	if issued.EnrollmentID == "" || issued.NodeID != "node-1" || !issued.DisplayedOnce || len(issued.EnrollmentMaterial) < 32 || !bytes.Equal(protocol.issued.TokenDigest, issuedDigest[:]) {
		t.Fatalf("unsafe enrollment issue response=%#v persisted=%#v", issued, protocol.issued)
	}

	machineCredential := "synthetic-agent-machine-credential-with-minimum-length"
	exchangeBody, err := json.Marshal(map[string]string{
		"requestId": "agent-enrollment-exchange-request", "enrollmentId": issued.EnrollmentID, "enrollmentMaterial": issued.EnrollmentMaterial,
		"agentId": "agent-1", "nodeId": "node-1", "machineCredential": machineCredential, "protocolVersion": "agent-v1",
	})
	if err != nil {
		t.Fatalf("encode enrollment exchange: %v", err)
	}
	exchange := httptest.NewRecorder()
	handler.ServeHTTP(exchange, httptest.NewRequest(http.MethodPost, "/agent/v1/enrollments:exchange", bytes.NewReader(exchangeBody)))
	credentialDigest := sha256.Sum256([]byte(machineCredential))
	if exchange.Code != http.StatusOK || !bytes.Equal(protocol.exchange.EnrollmentMaterialDigest, issuedDigest[:]) || !bytes.Equal(protocol.exchange.CredentialDigest, credentialDigest[:]) {
		t.Fatalf("enrollment exchange response=%d input=%#v body=%s", exchange.Code, protocol.exchange, exchange.Body.String())
	}
	if bytes.Contains(exchange.Body.Bytes(), []byte(issued.EnrollmentMaterial)) || bytes.Contains(exchange.Body.Bytes(), []byte(machineCredential)) {
		t.Fatal("enrollment exchange response leaked secret material")
	}

	heartbeatBody := []byte(`{"protocolVersion":"agent-v1","agentId":"agent-1","nodeId":"node-1","bootId":"boot-1","requestId":"heartbeat-request-1","sentAt":"2026-01-02T03:04:05Z","payloadType":"HEARTBEAT","payload":{"operatingSystem":"WINDOWS","architecture":"AMD64","agentVersion":"agent-test-v1","observedAt":"2026-01-02T03:04:05Z","capacityTotal":1,"capacityUsed":0,"cpuUsagePercent":12,"memoryUsagePercent":48}}`)
	heartbeatRequest := httptest.NewRequest(http.MethodPost, "/agent/v1/heartbeats", bytes.NewReader(heartbeatBody))
	heartbeatRequest.Header.Set("Authorization", "Bearer "+machineCredential)
	heartbeat := httptest.NewRecorder()
	handler.ServeHTTP(heartbeat, heartbeatRequest)
	if heartbeat.Code != http.StatusOK || protocol.heartbeat.AgentID != "agent-1" || protocol.heartbeat.Facts.OperatingSystem != "WINDOWS" || !bytes.Equal(protocol.authDigest, credentialDigest[:]) {
		t.Fatalf("heartbeat response=%d input=%#v body=%s", heartbeat.Code, protocol.heartbeat, heartbeat.Body.String())
	}
	if bytes.Contains(heartbeat.Body.Bytes(), []byte(machineCredential)) {
		t.Fatal("heartbeat response leaked machine credential")
	}

	missingCredential := httptest.NewRecorder()
	handler.ServeHTTP(missingCredential, httptest.NewRequest(http.MethodPost, "/agent/v1/heartbeats", bytes.NewReader(heartbeatBody)))
	if missingCredential.Code != http.StatusUnauthorized {
		t.Fatalf("missing credential heartbeat status=%d body=%s", missingCredential.Code, missingCredential.Body.String())
	}
	unknownFieldBody := append(append([]byte(nil), heartbeatBody[:len(heartbeatBody)-1]...), []byte(`,"unexpected":true}`)...)
	unknownFieldRequest := httptest.NewRequest(http.MethodPost, "/agent/v1/heartbeats", bytes.NewReader(unknownFieldBody))
	unknownFieldRequest.Header.Set("Authorization", "Bearer "+machineCredential)
	unknownField := httptest.NewRecorder()
	handler.ServeHTTP(unknownField, unknownFieldRequest)
	if unknownField.Code != http.StatusBadRequest {
		t.Fatalf("unknown heartbeat field status=%d body=%s", unknownField.Code, unknownField.Body.String())
	}
	protocol.heartbeatErr = store.ErrAgentHeartbeatConflict
	conflictRequest := httptest.NewRequest(http.MethodPost, "/agent/v1/heartbeats", bytes.NewReader(heartbeatBody))
	conflictRequest.Header.Set("Authorization", "Bearer "+machineCredential)
	conflict := httptest.NewRecorder()
	handler.ServeHTTP(conflict, conflictRequest)
	if conflict.Code != http.StatusConflict || !bytes.Contains(conflict.Body.Bytes(), []byte("AGENT_HEARTBEAT_CONFLICT")) {
		t.Fatalf("conflicting heartbeat status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestAgentPrecheckSecretSlotEndpointFailsClosedWithoutPrecheckDependencies(t *testing.T) {
	t.Parallel()
	protocol := &recordingAgentProtocol{identity: store.AgentIdentity{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1"}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{AgentProtocol: protocol})
	secret := "synthetic-secret-must-not-return"
	request := httptest.NewRequest(http.MethodPost, "/agent/v1/prechecks/precheck-1/secret-slots:resolve", bytes.NewBufferString(`{"requestId":"slot-request-1","payload":{"password":"`+secret+`"}}`))
	request.Header.Set("Authorization", "Bearer synthetic-machine-credential")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" || !bytes.Contains(response.Body.Bytes(), []byte("AGENT_PRECHECK_NOT_CONFIGURED")) {
		t.Fatalf("secret-slot response=%d headers=%#v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(secret)) {
		t.Fatal("secret-slot rejection leaked synthetic secret")
	}
}

func TestAgentPrecheckStorageSecretSlotRevalidatesAndReturnsOnlyStoragePayload(t *testing.T) {
	t.Parallel()
	secrets := &recordingPrecheckSecretStore{storage: store.EncryptedExecutionStorageCredential{
		StorageCredentialID: "storage-credential-1", Provider: "OSS", Revision: 1,
		OwnerSubjectID: "subject-1", DataSourceID: "source-allowed", NodeID: "node-1",
		AccessKeyCredentialID: "access-envelope-1", AccessKeyKeyID: "key-1", AccessKeyNonce: []byte{1}, AccessKeyCiphertext: []byte{2},
		SecretKeyCredentialID: "secret-envelope-1", SecretKeyKeyID: "key-1", SecretKeyNonce: []byte{3}, SecretKeyCiphertext: []byte{4},
	}}
	decryptor := &recordingPrecheckStorageDecryptor{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Authorizer: sliceAuthorizer{},
		AgentProtocol: &recordingAgentProtocol{identity: store.AgentIdentity{
			AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1",
		}},
		AgentPrechecks:  unreachableAgentPrecheckStore{},
		PrecheckSecrets: secrets,
		Decryptor:       decryptor,
		PrecheckTTL:     time.Minute,
	})
	requestBody := fmt.Sprintf(`{"protocolVersion":"agent-v1","agentId":"agent-1","nodeId":"node-1","bootId":"boot-1","requestId":"precheck-storage-secret-request","sentAt":"2026-01-02T03:04:05Z","payloadType":"EXPORT_PREFLIGHT_RESOLVE_SECRET_SLOTS","payload":{"leaseId":"lease-1","leaseEpoch":1,"bindingDigest":"%s","slot":"STORAGE_CREDENTIAL"}}`, strings.Repeat("a", 64))
	request := httptest.NewRequest(http.MethodPost, "/agent/v1/prechecks/precheck-1/secret-slots:resolve", strings.NewReader(requestBody))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("m", 32))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"slot":"STORAGE_CREDENTIAL"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"storageCredential"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"connection"`)) {
		t.Fatalf("storage secret response=%d headers=%#v body=%s", response.Code, response.Header(), response.Body.String())
	}
	for _, forbidden := range []string{"subject-1", "source-allowed", "node-1", "storage-credential-1"} {
		if bytes.Contains(response.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("storage secret response exposed protected binding %q: %s", forbidden, response.Body.String())
		}
	}
	if len(decryptor.envelopes) != 2 ||
		decryptor.envelopes[0].Reference != (credential.Reference{CredentialID: "access-envelope-1", Revision: 1, SecretType: credential.StorageAccessKey, DataSourceID: "storage-credential-1"}) ||
		decryptor.envelopes[1].Reference != (credential.Reference{CredentialID: "secret-envelope-1", Revision: 1, SecretType: credential.StorageSecretKey, DataSourceID: "storage-credential-1"}) {
		t.Fatalf("storage decrypt references = %#v", decryptor.envelopes)
	}
	if len(secrets.finished) != 1 || !secrets.finished[0].Succeeded {
		t.Fatalf("FinishPrecheckSecretResolution() = %#v, want one successful outcome", secrets.finished)
	}
}

func TestAgentPrecheckSecretSlotRevalidatesFrozenOwnerPermissionsBeforeDecrypt(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		authorizer identity.Authorizer
		denied     *precheckPermissionAuthorizer
	}{
		{name: "权限已撤销", denied: &precheckPermissionAuthorizer{}},
		{name: "授权服务不可用"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.denied != nil {
				testCase.authorizer = testCase.denied
			}
			secrets := &recordingPrecheckSecretStore{connection: store.EncryptedPrecheckDatabaseConnection{
				Host: "192.0.2.70", Port: 2881, Username: []byte("synthetic-user"), DataSourceID: "source-1",
				OwnerSubjectID: "subject-1", NodeID: "node-1", CredentialID: "credential-1", Revision: 1,
				KeyID: "key-1", Nonce: []byte{1}, Ciphertext: []byte{2},
			}}
			decryptor := &countingPrecheckDecryptor{}
			handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
				Authorizer: testCase.authorizer,
				AgentProtocol: &recordingAgentProtocol{identity: store.AgentIdentity{
					AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1",
				}},
				AgentPrechecks:  unreachableAgentPrecheckStore{},
				PrecheckSecrets: secrets,
				Decryptor:       decryptor,
				PrecheckTTL:     time.Minute,
			})
			requestBody := fmt.Sprintf(`{"protocolVersion":"agent-v1","agentId":"agent-1","nodeId":"node-1","bootId":"boot-1","requestId":"precheck-secret-request","sentAt":"2026-01-02T03:04:05Z","payloadType":"EXPORT_PREFLIGHT_RESOLVE_SECRET_SLOTS","payload":{"leaseId":"lease-1","leaseEpoch":1,"bindingDigest":"%s","slot":"DATABASE_CONNECTION"}}`, strings.Repeat("a", 64))
			request := httptest.NewRequest(http.MethodPost, "/agent/v1/prechecks/precheck-1/secret-slots:resolve", strings.NewReader(requestBody))
			request.Header.Set("Authorization", "Bearer "+strings.Repeat("m", 32))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("PRECHECK_LEASE_REJECTED")) {
				t.Fatalf("permission drift response=%d body=%s", response.Code, response.Body.String())
			}
			if decryptor.calls != 0 {
				t.Fatalf("Decrypt() calls = %d, want 0", decryptor.calls)
			}
			if len(secrets.finished) != 1 || secrets.finished[0].Succeeded {
				t.Fatalf("FinishPrecheckSecretResolution() = %#v, want one failed outcome", secrets.finished)
			}
			if bytes.Contains(response.Body.Bytes(), []byte("192.0.2.70")) || bytes.Contains(response.Body.Bytes(), []byte("subject-1")) || bytes.Contains(response.Body.Bytes(), []byte("node-1")) {
				t.Fatalf("permission drift response exposed slot binding: %s", response.Body.String())
			}
			if testCase.denied != nil {
				if len(testCase.denied.calls) != 2 ||
					testCase.denied.calls[0] != (precheckPermissionCall{Principal: "subject-1", Scope: identity.ScopeDataSourceRead, ObjectID: "source-1"}) ||
					testCase.denied.calls[1] != (precheckPermissionCall{Principal: "subject-1", Scope: identity.ScopeNodeUse, ObjectID: "node-1"}) {
					t.Fatalf("permission revalidation calls = %#v", testCase.denied.calls)
				}
			}
		})
	}
}

func TestAgentPrecheckClaimNextRejectsClientSuppliedIdentifiers(t *testing.T) {
	protocol := &recordingAgentProtocol{identity: store.AgentIdentity{
		AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1",
	}}
	prechecks := &recordingClaimNextPrecheckStore{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		AgentProtocol:  protocol,
		AgentPrechecks: prechecks,
		PrecheckTTL:    time.Minute,
	})
	validBody := `{"protocolVersion":"agent-v1","agentId":"agent-1","nodeId":"node-1","bootId":"boot-1","requestId":"claim-next-1","sentAt":"2026-01-02T03:04:05Z","payloadType":"EXPORT_PREFLIGHT_CLAIM_NEXT","payload":{"capability":"EXPORT_PREFLIGHT"}}`
	validRequest := httptest.NewRequest(http.MethodPost, "/agent/v1/prechecks:claim-next", strings.NewReader(validBody))
	validRequest.Header.Set("Authorization", "Bearer "+strings.Repeat("m", 32))
	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusNoContent || validResponse.Header().Get("Cache-Control") != "no-store" || len(validResponse.Body.Bytes()) != 0 {
		t.Fatalf("claim-next no-work response=%d headers=%#v body=%s", validResponse.Code, validResponse.Header(), validResponse.Body.String())
	}
	if len(prechecks.inputs) != 1 || prechecks.inputs[0].AgentID != "agent-1" || prechecks.inputs[0].NodeID != "node-1" || !identifier.IsCanonicalUUIDV4(prechecks.inputs[0].LeaseID) || len(prechecks.inputs[0].RequestDigest) != 64 {
		t.Fatalf("ClaimNextPrecheck input = %#v", prechecks.inputs)
	}

	for _, injected := range []string{
		`"precheckId":"precheck-injected"`,
		`"leaseId":"lease-injected"`,
		`"path":"E:\\injected"`,
		`"command":"injected"`,
		`"sql":"SELECT 1"`,
	} {
		requestBody := strings.Replace(validBody, `"capability":"EXPORT_PREFLIGHT"`, `"capability":"EXPORT_PREFLIGHT",`+injected, 1)
		request := httptest.NewRequest(http.MethodPost, "/agent/v1/prechecks:claim-next", strings.NewReader(requestBody))
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("m", 32))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("AGENT_REQUEST_INVALID")) {
			t.Fatalf("injected %s response=%d body=%s", injected, response.Code, response.Body.String())
		}
		if len(prechecks.inputs) != 1 {
			t.Fatalf("injected %s unexpectedly reached ClaimNextPrecheck", injected)
		}
	}
}

type staticIdentityProvider struct{}

func (staticIdentityProvider) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-subject"}, nil
}

func (staticIdentityProvider) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-subject"}, nil
}

// browserOnlyIdentityProvider represents an injected synthetic test identity;
// it is not registered by the production composition root.
type browserOnlyIdentityProvider struct{}

func (browserOnlyIdentityProvider) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-subject"}, nil
}

type dualIdentityProvider struct{}

func (dualIdentityProvider) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-subject"}, nil
}
func (dualIdentityProvider) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.AgentPrincipal, ID: "synthetic-agent"}, nil
}

func (browserOnlyIdentityProvider) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, errors.New("synthetic agent authentication denied")
}

// browserOtherIdentityProvider 返回与合成主体不同的另一主体（用于越权负例）。
type browserOtherIdentityProvider struct{}

func (browserOtherIdentityProvider) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-other-subject"}, nil
}

func (browserOtherIdentityProvider) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, errors.New("synthetic agent authentication denied")
}

type countingIdentityProvider struct {
	browserCalls int
}

func (p *countingIdentityProvider) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	p.browserCalls++
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-subject"}, nil
}

func (*countingIdentityProvider) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, errors.New("synthetic agent authentication denied")
}

type sourceAuthorizer struct {
	allowedID  string
	allowWrite bool
}

type connectionTestAuthorizer struct{}

type sliceAuthorizer struct{}

type diagnosticNodeAuthorizer struct{}

func (diagnosticNodeAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, objectID string) error {
	if scope == identity.ScopeNodeUse && (objectID == "node-disabled" || objectID == "node-maintenance") {
		return nil
	}
	return identity.ErrDenied
}

func (sliceAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, objectID string) error {
	if (scope == identity.ScopeDataSourceRead && objectID == "source-allowed") || (scope == identity.ScopeNodeUse && objectID == "node-1") {
		return nil
	}
	return errors.New("synthetic slice scope denied")
}

func (a sourceAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, objectID string) error {
	if objectID == a.allowedID && (scope == identity.ScopeDataSourceRead || (scope == identity.ScopeDataSourceWrite && a.allowWrite)) {
		return nil
	}
	return errors.New("synthetic object scope denied")
}

func (connectionTestAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, objectID string) error {
	if objectID == "source-allowed" && (scope == identity.ScopeDataSourceRead || scope == identity.ScopeDataSourceWrite) {
		return nil
	}
	if objectID == "node-1" && scope == identity.ScopeNodeUse {
		return nil
	}
	return identity.ErrDenied
}

type staticDataSourceReader struct{}

func (staticDataSourceReader) ListDataSourceSummaries(context.Context) ([]store.DataSourceSummary, error) {
	return []store.DataSourceSummary{
		{DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 2, LastTestStatus: "SUCCEEDED"},
		{DataSourceID: "source-denied", DisplayName: "Denied", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.2", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 3},
	}, nil
}

func (staticDataSourceReader) GetDataSourceSummary(_ context.Context, dataSourceID string) (store.DataSourceSummary, error) {
	for _, summary := range mustStaticDataSourceReader().summaries {
		if summary.DataSourceID == dataSourceID {
			return summary, nil
		}
	}
	return store.DataSourceSummary{}, store.ErrDataSourceNotFound
}

type staticDataSources struct{ summaries []store.DataSourceSummary }

func (sources staticDataSources) ListDataSourceSummaries(context.Context) ([]store.DataSourceSummary, error) {
	return append([]store.DataSourceSummary(nil), sources.summaries...), nil
}

func (sources staticDataSources) GetDataSourceSummary(_ context.Context, dataSourceID string) (store.DataSourceSummary, error) {
	for _, summary := range sources.summaries {
		if summary.DataSourceID == dataSourceID {
			return summary, nil
		}
	}
	return store.DataSourceSummary{}, store.ErrDataSourceNotFound
}

type sequenceDataSourceReader struct {
	summaries []store.DataSourceSummary
	reads     int
}

func (reader *sequenceDataSourceReader) ListDataSourceSummaries(context.Context) ([]store.DataSourceSummary, error) {
	return append([]store.DataSourceSummary(nil), reader.summaries...), nil
}

func (reader *sequenceDataSourceReader) GetDataSourceSummary(_ context.Context, dataSourceID string) (store.DataSourceSummary, error) {
	if len(reader.summaries) == 0 {
		return store.DataSourceSummary{}, store.ErrDataSourceNotFound
	}
	index := reader.reads
	if index >= len(reader.summaries) {
		index = len(reader.summaries) - 1
	}
	reader.reads++
	summary := reader.summaries[index]
	if summary.DataSourceID != dataSourceID {
		return store.DataSourceSummary{}, store.ErrDataSourceNotFound
	}
	return summary, nil
}

func mustStaticDataSourceReader() staticDataSources {
	return staticDataSources{summaries: []store.DataSourceSummary{
		{DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 2, LastTestStatus: "SUCCEEDED"},
		{DataSourceID: "source-denied", DisplayName: "Denied", Environment: "TEST", ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.2", Port: 2881, ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 3},
	}}
}

type allowedRoleAuthorizer struct{}

func (allowedRoleAuthorizer) AuthorizeRole(context.Context, identity.Principal, identity.Role) error {
	return nil
}

type allowedCSRF struct{}

func (allowedCSRF) ValidateCSRF(*http.Request) error { return nil }

// deniedCSRF 固定拒绝全部请求，用于验证存储凭据写操作在 CSRF 缺失时失败关闭。
type deniedCSRF struct{}

func (deniedCSRF) ValidateCSRF(*http.Request) error { return errors.New("csrf denied") }

type recordingCreator struct{ input store.DataSourceCreate }

func (c *recordingCreator) CreateDataSource(_ context.Context, input store.DataSourceCreate) (store.DataSourceCreateResult, error) {
	c.input = input
	return store.DataSourceCreateResult{DataSourceID: input.DataSourceID}, nil
}

type nameUnavailableCreator struct{}

func (nameUnavailableCreator) CreateDataSource(context.Context, store.DataSourceCreate) (store.DataSourceCreateResult, error) {
	return store.DataSourceCreateResult{}, store.ErrDataSourceNameUnavailable
}

type recordingStateChanger struct{ input store.DataSourceStateChange }

type recordingDataSourceDeleter struct {
	deleteInput  store.DataSourceDeletion
	archiveInput store.DataSourceArchive
	result       store.DataSourceDeletionResult
	err          error
}

type recordingExecutionNodeDeleter struct {
	input  store.ExecutionNodeDeletion
	result store.ExecutionNodeDeletionResult
	err    error
}

type rejectedEnableStateChanger struct{}

func (rejectedEnableStateChanger) ChangeDataSourceState(context.Context, store.DataSourceStateChange) (store.DataSourceStateChangeResult, error) {
	return store.DataSourceStateChangeResult{}, store.ErrDataSourceConnectionTestRequired
}

type recordingDataSourceConnectionTestStore struct {
	created store.DataSourceConnectionTestCreate
	run     store.DataSourceConnectionTestRun
	err     error
}

func (s *recordingDataSourceConnectionTestStore) RequestDataSourceConnectionTest(_ context.Context, input store.DataSourceConnectionTestCreate) (store.DataSourceConnectionTestCreateResult, error) {
	s.created = input
	if s.err != nil {
		return store.DataSourceConnectionTestCreateResult{}, s.err
	}
	if s.run.ConnectionTestID == "" {
		s.run = store.DataSourceConnectionTestRun{
			ConnectionTestID: input.ConnectionTestID, DataSourceID: input.DataSourceID, NodeID: input.NodeID,
			NodeFactsRevision: 1, Status: "PENDING", VerificationSource: input.VerificationSource, CreatedAt: input.CreatedAt,
		}
	}
	return store.DataSourceConnectionTestCreateResult{ConnectionTestID: s.run.ConnectionTestID}, nil
}

func (s *recordingDataSourceConnectionTestStore) GetDataSourceConnectionTestRun(_ context.Context, connectionTestID string) (store.DataSourceConnectionTestRun, error) {
	if s.err != nil || s.run.ConnectionTestID != connectionTestID {
		if s.err != nil {
			return store.DataSourceConnectionTestRun{}, s.err
		}
		return store.DataSourceConnectionTestRun{}, store.ErrDataSourceNotFound
	}
	return s.run, nil
}

func (c *recordingStateChanger) ChangeDataSourceState(_ context.Context, input store.DataSourceStateChange) (store.DataSourceStateChangeResult, error) {
	c.input = input
	return store.DataSourceStateChangeResult{State: input.TargetState, Revision: 2}, nil
}

func (d *recordingDataSourceDeleter) DeleteDataSource(_ context.Context, input store.DataSourceDeletion) (store.DataSourceDeletionResult, error) {
	d.deleteInput = input
	return d.result, d.err
}

func (d *recordingDataSourceDeleter) ArchiveDataSource(_ context.Context, input store.DataSourceArchive) (store.DataSourceDeletionResult, error) {
	d.archiveInput = input
	return d.result, d.err
}

func (d *recordingExecutionNodeDeleter) DeleteOrArchiveExecutionNode(_ context.Context, input store.ExecutionNodeDeletion) (store.ExecutionNodeDeletionResult, error) {
	d.input = input
	return d.result, d.err
}

type staticCredentialReferenceReader struct{}

func (staticCredentialReferenceReader) GetDataSourceCredentialReference(context.Context, string) (store.DataSourceCredentialReference, error) {
	return store.DataSourceCredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}, nil
}

type recordingUpdater struct {
	input                     store.DataSourceUpdate
	connectionTestInvalidated bool
}

func (u *recordingUpdater) UpdateDataSource(_ context.Context, input store.DataSourceUpdate) (store.DataSourceUpdateResult, error) {
	u.input = input
	credentialRevision := int64(2)
	if input.Password == nil {
		credentialRevision = 1
	}
	return store.DataSourceUpdateResult{Revision: input.ExpectedRevision + 1, CredentialRevision: credentialRevision, ConnectionTestInvalidated: u.connectionTestInvalidated}, nil
}

type staticNodeReader struct{}

func (staticNodeReader) GetExecutionNodeFact(context.Context, string) (ExecutionNodeFact, error) {
	return ExecutionNodeFact{NodeID: "node-1", Platform: commandgen.PlatformWindowsAMD64, FactsVersion: "node-facts-1", FactsRevision: 1}, nil
}

type recordingAgentProtocol struct {
	issued       store.AgentEnrollmentIssue
	exchange     store.AgentEnrollmentExchange
	heartbeat    store.AgentHeartbeat
	authDigest   []byte
	identity     store.AgentIdentity
	heartbeatErr error
}

func (p *recordingAgentProtocol) IssueAgentEnrollment(_ context.Context, input store.AgentEnrollmentIssue) error {
	p.issued = input
	return nil
}

func (p *recordingAgentProtocol) ExchangeAgentEnrollment(_ context.Context, input store.AgentEnrollmentExchange) (store.AgentEnrollmentResult, error) {
	p.exchange = input
	return store.AgentEnrollmentResult{AgentID: input.AgentID, NodeID: input.NodeID}, nil
}

func (p *recordingAgentProtocol) AuthenticateAgent(_ context.Context, credentialDigest []byte) (store.AgentIdentity, error) {
	p.authDigest = append(p.authDigest[:0], credentialDigest...)
	return p.identity, nil
}

func (p *recordingAgentProtocol) RecordAgentHeartbeat(_ context.Context, input store.AgentHeartbeat) (int64, error) {
	p.heartbeat = input
	if p.heartbeatErr != nil {
		return 0, p.heartbeatErr
	}
	return 9, nil
}

type unreachableAgentPrecheckStore struct{}

func (unreachableAgentPrecheckStore) ClaimNextPrecheck(context.Context, store.PrecheckClaimNext) (store.PrecheckLeaseGrant, bool, error) {
	return store.PrecheckLeaseGrant{}, false, store.ErrPrecheckLeaseRejected
}

func (unreachableAgentPrecheckStore) AcknowledgePrecheck(context.Context, store.PrecheckAcknowledgement) (store.PrecheckLeaseGrant, error) {
	return store.PrecheckLeaseGrant{}, store.ErrPrecheckLeaseRejected
}

func (unreachableAgentPrecheckStore) CompleteAgentPrecheck(context.Context, store.AgentPrecheckCompletion) (store.PrecheckCompletionResult, error) {
	return store.PrecheckCompletionResult{}, store.ErrPrecheckLeaseRejected
}

type recordingClaimNextPrecheckStore struct {
	inputs []store.PrecheckClaimNext
}

func (s *recordingClaimNextPrecheckStore) ClaimNextPrecheck(_ context.Context, input store.PrecheckClaimNext) (store.PrecheckLeaseGrant, bool, error) {
	s.inputs = append(s.inputs, input)
	return store.PrecheckLeaseGrant{}, false, nil
}

func (s *recordingClaimNextPrecheckStore) AcknowledgePrecheck(context.Context, store.PrecheckAcknowledgement) (store.PrecheckLeaseGrant, error) {
	return store.PrecheckLeaseGrant{}, store.ErrPrecheckLeaseRejected
}

func (s *recordingClaimNextPrecheckStore) CompleteAgentPrecheck(context.Context, store.AgentPrecheckCompletion) (store.PrecheckCompletionResult, error) {
	return store.PrecheckCompletionResult{}, store.ErrPrecheckLeaseRejected
}

type recordingPrecheckSecretStore struct {
	connection store.EncryptedPrecheckDatabaseConnection
	storage    store.EncryptedExecutionStorageCredential
	finished   []store.PrecheckSecretResolutionOutcome
}

func (s *recordingPrecheckSecretStore) ResolvePrecheckDatabaseConnection(_ context.Context, _ store.PrecheckSecretResolutionRequest) (store.EncryptedPrecheckDatabaseConnection, error) {
	return s.connection, nil
}

func (s *recordingPrecheckSecretStore) ResolvePrecheckStorageCredential(_ context.Context, _ store.PrecheckSecretResolutionRequest) (store.EncryptedExecutionStorageCredential, error) {
	return s.storage, nil
}

func (s *recordingPrecheckSecretStore) FinishPrecheckSecretResolution(_ context.Context, input store.PrecheckSecretResolutionOutcome) error {
	s.finished = append(s.finished, input)
	return nil
}

type countingPrecheckDecryptor struct{ calls int }

func (d *countingPrecheckDecryptor) Decrypt(credential.Envelope) ([]byte, error) {
	d.calls++
	return nil, errors.New("synthetic decryptor must not be called")
}

type recordingPrecheckStorageDecryptor struct{ envelopes []credential.Envelope }

func (d *recordingPrecheckStorageDecryptor) Decrypt(envelope credential.Envelope) ([]byte, error) {
	d.envelopes = append(d.envelopes, envelope)
	switch envelope.Reference.SecretType {
	case credential.StorageAccessKey:
		return []byte("synthetic-access-key"), nil
	case credential.StorageSecretKey:
		return []byte("synthetic-secret-key"), nil
	default:
		return nil, errors.New("synthetic storage secret type is invalid")
	}
}

type precheckPermissionCall struct {
	Principal string
	Scope     identity.Scope
	ObjectID  string
}

type precheckPermissionAuthorizer struct{ calls []precheckPermissionCall }

func (a *precheckPermissionAuthorizer) Authorize(_ context.Context, principal identity.Principal, scope identity.Scope, objectID string) error {
	a.calls = append(a.calls, precheckPermissionCall{Principal: principal.ID, Scope: scope, ObjectID: objectID})
	return identity.ErrDenied
}

type staticNodeCandidateReader struct{}

func (staticNodeCandidateReader) ListExecutionNodeSummaries(context.Context) ([]store.ExecutionNodeSummary, error) {
	return []store.ExecutionNodeSummary{
		{NodeID: "node-1", DisplayName: "Allowed Node", Platform: "WINDOWS_AMD64"},
		{NodeID: "node-denied", DisplayName: "Denied Node", Platform: "LINUX_AMD64"},
	}, nil
}

type nodeManageAuthorizer struct{ allowedID string }

func (a nodeManageAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, objectID string) error {
	if scope == identity.ScopeNodeManage && objectID == a.allowedID {
		return nil
	}
	return errors.New("synthetic node management scope denied")
}

type recordingNodeManagementStore struct {
	nodes   map[string]store.ExecutionNode
	created store.ExecutionNodeCreate
	updated store.ExecutionNodeUpdate
}

type countingNodeManagementStore struct {
	recordingNodeManagementStore
	createCalls int
}

func (s *countingNodeManagementStore) CreateExecutionNode(ctx context.Context, input store.ExecutionNodeCreate) (store.ExecutionNodeCreateResult, error) {
	s.createCalls++
	return s.recordingNodeManagementStore.CreateExecutionNode(ctx, input)
}

func (s *recordingNodeManagementStore) ListExecutionNodes(context.Context) ([]store.ExecutionNode, error) {
	items := make([]store.ExecutionNode, 0, len(s.nodes))
	for _, node := range s.nodes {
		items = append(items, node)
	}
	return items, nil
}

func (s *recordingNodeManagementStore) GetExecutionNode(_ context.Context, nodeID string) (store.ExecutionNode, error) {
	node, ok := s.nodes[nodeID]
	if !ok {
		return store.ExecutionNode{}, store.ErrExecutionNodeNotFound
	}
	return node, nil
}

func (s *recordingNodeManagementStore) CreateExecutionNode(_ context.Context, input store.ExecutionNodeCreate) (store.ExecutionNodeCreateResult, error) {
	s.created = input
	if s.nodes == nil {
		s.nodes = make(map[string]store.ExecutionNode)
	}
	s.nodes[input.NodeID] = store.ExecutionNode{
		NodeID: input.NodeID, DisplayName: input.DisplayName, Platform: input.Platform, ManagementState: "DISABLED",
		AllowedRoots: append([]string(nil), input.AllowedRoots...), Revision: 1, CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt,
	}
	return store.ExecutionNodeCreateResult{NodeID: input.NodeID}, nil
}

func (s *recordingNodeManagementStore) UpdateExecutionNode(_ context.Context, input store.ExecutionNodeUpdate) (int64, error) {
	node, ok := s.nodes[input.NodeID]
	if !ok {
		return 0, store.ErrExecutionNodeNotFound
	}
	if node.Revision != input.ExpectedRevision {
		return 0, store.ErrRevisionConflict
	}
	s.updated = input
	node.DisplayName, node.Platform, node.AllowedRoots, node.Revision, node.UpdatedAt = input.DisplayName, input.Platform, append([]string(nil), input.AllowedRoots...), input.ExpectedRevision+1, input.UpdatedAt
	s.nodes[input.NodeID] = node
	return node.Revision, nil
}

type recordingDraftStore struct{ created store.ExportDraft }

func (s *recordingDraftStore) CreateExportDraft(_ context.Context, input store.ExportDraftCreate) (store.ExportDraftCreateResult, error) {
	s.created = input.ExportDraft
	s.created.DraftID = "draft-synthetic"
	s.created.Revision = 1
	return store.ExportDraftCreateResult{DraftID: s.created.DraftID}, nil
}

func (s *recordingDraftStore) GetExportDraft(context.Context, string) (store.ExportDraft, error) {
	return s.created, nil
}

func (s *recordingDraftStore) UpdateDraft(_ context.Context, input store.DraftUpdate) (int64, error) {
	s.created.ConfigJSON, s.created.ConfigFingerprint, s.created.Revision, s.created.ConfigVersion = input.ConfigJSON, input.ConfigFingerprint, input.ExpectedRevision+1, input.ConfigVersion
	return s.created.Revision, nil
}

type recordingPrecheckStore struct{ created store.PrecheckRun }

func (s *recordingPrecheckStore) CreatePrecheck(_ context.Context, input store.PrecheckCreate) (store.PrecheckCreateResult, error) {
	s.created = input.PrecheckRun
	s.created.PrecheckID = "precheck-synthetic"
	s.created.Status = "PENDING"
	s.created.IntegrityStatus = "UNKNOWN"
	return store.PrecheckCreateResult{PrecheckID: s.created.PrecheckID}, nil
}

func (s *recordingPrecheckStore) GetPrecheckRun(context.Context, string) (store.PrecheckRun, error) {
	return s.created, nil
}

func (s *recordingPrecheckStore) CompletePrecheck(_ context.Context, input store.PrecheckCompletion) error {
	if input.Succeeded {
		s.created.Status = "SUCCEEDED"
	} else {
		s.created.Status = "FAILED"
	}
	s.created.IntegrityStatus = input.IntegrityStatus
	return nil
}

type recordingTaskStore struct {
	run               store.PrecheckRun
	input             store.TaskSubmission
	summary           store.TaskSummary
	derivationSource  store.TaskDerivationSource
	resumeInput       store.CheckpointResumeDerivation
	resumeErr         error
	listItems         []store.TaskListItem
	listQuery         store.TaskListQuery
	countSubject      string
	countErr          error
	authorizedSubject string
}

type emptyLogBatchIndex struct{}

// reconnectLogBatchIndex 仅模拟已登记的持久日志索引，覆盖 SSE 续传而不依赖 SQLite 或 Agent。
type reconnectLogBatchIndex struct {
	batches []logstream.IndexedBatch
}

func (emptyLogBatchIndex) PrepareLogAppend(context.Context, logstream.IndexedBatch) (logstream.AppendPlan, error) {
	return logstream.AppendPlan{}, errors.New("测试索引不支持写入")
}

func (emptyLogBatchIndex) CommitLogAppend(context.Context, logstream.IndexedBatch) (logstream.BatchResult, error) {
	return logstream.BatchResult{}, errors.New("测试索引不支持写入")
}

func (emptyLogBatchIndex) LatestTaskLogPosition(context.Context, string) (logstream.BatchPosition, bool, error) {
	return logstream.BatchPosition{}, false, nil
}

func (emptyLogBatchIndex) NextTaskLogBatch(context.Context, string, logstream.BatchPosition, logstream.BatchPosition, bool) (logstream.IndexedBatch, bool, error) {
	return logstream.IndexedBatch{}, false, nil
}

func (emptyLogBatchIndex) ListLogSegmentBatches(context.Context, string) ([]logstream.IndexedBatch, error) {
	return []logstream.IndexedBatch{}, nil
}

func (emptyLogBatchIndex) ListOpenLogSegments(context.Context) ([]logstream.IndexedSegment, error) {
	return []logstream.IndexedSegment{}, nil
}

func (emptyLogBatchIndex) SealLogSegment(context.Context, logstream.IndexedSegment, string, time.Time) error {
	return nil
}

func (index *reconnectLogBatchIndex) PrepareLogAppend(_ context.Context, entry logstream.IndexedBatch) (logstream.AppendPlan, error) {
	segmentLength := int64(0)
	expectedSequence := int64(1)
	lastDigest := ""
	if len(index.batches) > 0 {
		last := index.batches[len(index.batches)-1]
		segmentLength = last.SegmentOffsetEnd
		expectedSequence = last.LastSequence + 1
		lastDigest = last.Digest
	}
	if entry.FirstSequence != expectedSequence || entry.PreviousDigest != lastDigest {
		return logstream.AppendPlan{Decision: logstream.BatchNeedGap, ExpectedSequence: expectedSequence, LastDigest: lastDigest}, nil
	}
	return logstream.AppendPlan{
		Decision: logstream.BatchAccepted, ExpectedSequence: expectedSequence, LastDigest: lastDigest,
		SegmentID: strings.Repeat("a", 64), SegmentOrdinal: 1, SegmentByteLength: segmentLength,
	}, nil
}

func (index *reconnectLogBatchIndex) CommitLogAppend(_ context.Context, entry logstream.IndexedBatch) (logstream.BatchResult, error) {
	entry.SegmentState = "OPEN"
	entry.SegmentStorageKey = entry.SegmentID + ".open"
	index.batches = append(index.batches, entry)
	return logstream.BatchResult{Decision: logstream.BatchAccepted, ExpectedSeq: entry.LastSequence + 1, LastDigest: entry.Digest}, nil
}

func (index *reconnectLogBatchIndex) LatestTaskLogPosition(_ context.Context, taskID string) (logstream.BatchPosition, bool, error) {
	if taskID != "task-running" || len(index.batches) == 0 {
		return logstream.BatchPosition{}, false, nil
	}
	last := index.batches[len(index.batches)-1]
	return logstream.BatchPosition{ReceivedAt: last.ReceivedAt, BatchID: last.BatchID}, true, nil
}

func (index *reconnectLogBatchIndex) NextTaskLogBatch(_ context.Context, taskID string, snapshot, after logstream.BatchPosition, includeAfter bool) (logstream.IndexedBatch, bool, error) {
	if taskID != "task-running" {
		return logstream.IndexedBatch{}, false, nil
	}
	for _, batch := range index.batches {
		position := logstream.BatchPosition{ReceivedAt: batch.ReceivedAt, BatchID: batch.BatchID}
		if compareTaskLogBatchPosition(position, snapshot) > 0 {
			continue
		}
		if after.BatchID != "" {
			comparison := compareTaskLogBatchPosition(position, after)
			if comparison < 0 || (!includeAfter && comparison == 0) {
				continue
			}
		}
		return batch, true, nil
	}
	return logstream.IndexedBatch{}, false, nil
}

func (index *reconnectLogBatchIndex) ListLogSegmentBatches(_ context.Context, segmentID string) ([]logstream.IndexedBatch, error) {
	batches := make([]logstream.IndexedBatch, 0, len(index.batches))
	for _, batch := range index.batches {
		if batch.SegmentID == segmentID {
			batches = append(batches, batch)
		}
	}
	return batches, nil
}

func (*reconnectLogBatchIndex) ListOpenLogSegments(context.Context) ([]logstream.IndexedSegment, error) {
	return []logstream.IndexedSegment{}, nil
}

func (*reconnectLogBatchIndex) SealLogSegment(context.Context, logstream.IndexedSegment, string, time.Time) error {
	return nil
}

func compareTaskLogBatchPosition(left, right logstream.BatchPosition) int {
	if left.ReceivedAt.Before(right.ReceivedAt) {
		return -1
	}
	if left.ReceivedAt.After(right.ReceivedAt) {
		return 1
	}
	return strings.Compare(left.BatchID, right.BatchID)
}

type streamingResponseWriter struct {
	header  http.Header
	Body    bytes.Buffer
	status  int
	flushed chan struct{}
}

func newStreamingResponseWriter() *streamingResponseWriter {
	return &streamingResponseWriter{header: make(http.Header), flushed: make(chan struct{}, 4)}
}

func (w *streamingResponseWriter) Header() http.Header {
	return w.header
}

func (w *streamingResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *streamingResponseWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.Body.Write(value)
}

func (w *streamingResponseWriter) Flush() {
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

type nonStreamingResponseWriter struct {
	header http.Header
	Body   bytes.Buffer
	status int
}

func newNonStreamingResponseWriter() *nonStreamingResponseWriter {
	return &nonStreamingResponseWriter{header: make(http.Header)}
}

func (w *nonStreamingResponseWriter) Header() http.Header {
	return w.header
}

func (w *nonStreamingResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *nonStreamingResponseWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.Body.Write(value)
}

type recordingExecutionStore struct {
	claim store.Claim
	renew store.LeaseRenewal
	event store.ExecutionEvent
}

func (s *recordingExecutionStore) ClaimTask(_ context.Context, input store.Claim) error {
	s.claim = input
	return nil
}

func (s *recordingExecutionStore) RenewExecutionLease(_ context.Context, input store.LeaseRenewal) error {
	s.renew = input
	return nil
}

func (s *recordingExecutionStore) AppendExecutionEvent(_ context.Context, input store.ExecutionEvent) error {
	s.event = input
	return nil
}

func (s *recordingTaskStore) GetPrecheckRun(context.Context, string) (store.PrecheckRun, error) {
	return s.run, nil
}

func (s *recordingTaskStore) SubmitTaskIdempotent(_ context.Context, input store.TaskSubmission, _ string, _ string) (store.TaskSubmissionResult, error) {
	s.input = input
	return store.TaskSubmissionResult{TaskID: input.TaskID}, nil
}

// TestTaskExecutionProjectionIncludesResultSummary 验证 EX-I8 执行投影返回受控结果摘要，
// 相对路径与大小原样投影，且不包含任何秘密字段。
func TestTaskExecutionProjectionIncludesResultSummary(t *testing.T) {
	t.Parallel()
	tasks := &recordingTaskStore{}
	observed := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	tasks.summary = store.TaskSummary{
		TaskID: "task-1", CreatorSubjectID: "subject-1", DataSourceID: "source-allowed", NodeID: "node-1",
		State: "FAILED", ExecutionID: "execution-1", ReconciliationRequired: false,
		UpdatedAt: observed, SubmittedAt: observed.Add(-time.Hour),
		ResultSummary: &store.ExecutionResultSummary{
			Result: "FAILED", FileCount: 3, TotalBytes: 512, CheckpointPresent: true, ObservedAt: observed,
			Files: []store.ExecutionResultFile{{Path: "data_1.csv", Size: 100}, {Path: "dump.ckpt", Size: 412}},
		},
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/execution", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("execution response=%d body=%s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"result":"FAILED"`, `"fileCount":3`, `"totalBytes":512`, `"checkpointPresent":true`, `"path":"data_1.csv"`, `"size":412`} {
		if !bytes.Contains(response.Body.Bytes(), []byte(expected)) {
			t.Fatalf("execution projection missing %s: %s", expected, response.Body.String())
		}
	}
	for _, forbidden := range []string{"password", "secret", "ciphertext", "checksum", "content"} {
		if bytes.Contains(response.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("execution projection leaked %s: %s", forbidden, response.Body.String())
		}
	}
	// 未上报结果事实的任务不返回 resultSummary 字段（缺省，不伪造）。
	empty := &recordingTaskStore{}
	empty.summary = store.TaskSummary{TaskID: "task-2", CreatorSubjectID: "subject-1", State: "WAITING_SCHEDULE", UpdatedAt: observed, SubmittedAt: observed}
	handler = NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: empty})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-2/execution", nil))
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte("resultSummary")) {
		t.Fatalf("pending execution must omit resultSummary: %d %s", response.Code, response.Body.String())
	}
}

func (s *recordingTaskStore) GetAuthorizedTaskSummary(_ context.Context, _ string, subjectID string) (store.TaskSummary, error) {
	s.authorizedSubject = subjectID
	if s.summary.TaskID != "" {
		return s.summary, nil
	}
	// 模拟真实存储：从快照 JSON 的扁平投影键提取 database/table/format。
	var projection struct {
		Database string `json:"database"`
		Table    string `json:"table"`
		Format   string `json:"format"`
	}
	_ = json.Unmarshal([]byte(s.input.SnapshotJSON), &projection)
	return store.TaskSummary{TaskID: s.input.TaskID, CreatorSubjectID: s.input.CreatorSubjectID, DataSourceID: s.input.DataSourceID, NodeID: s.input.NodeID, PrecheckID: s.input.PrecheckID, ConfigFingerprint: s.input.ConfigFingerprint, ToolVersion: s.input.ToolVersion, MetadataVersion: s.input.MetadataVersion, CapabilityVersion: s.input.CapabilityVersion, SnapshotVersion: s.input.SnapshotVersion, Database: projection.Database, Table: projection.Table, Format: projection.Format, PlannedCommandRedacted: s.input.PlannedCommandRedacted, State: "WAITING_SCHEDULE", SubmittedAt: s.input.SubmittedAt}, nil
}

// derivationSource 是 EX-I8 派生操作测试用的冻结任务事实夹具。
func (s *recordingTaskStore) GetAuthorizedTaskDerivationSource(_ context.Context, taskID string, subjectID string) (store.TaskDerivationSource, error) {
	s.authorizedSubject = subjectID
	if s.derivationSource.TaskID == "" {
		return store.TaskDerivationSource{}, store.ErrDataSourceNotFound
	}
	return s.derivationSource, nil
}

// resumeResult 是检查点继续测试用的受控结果夹具。
func (s *recordingTaskStore) DeriveTaskFromCheckpoint(_ context.Context, input store.CheckpointResumeDerivation) (store.CheckpointResumeResult, error) {
	s.resumeInput = input
	if s.resumeErr != nil {
		return store.CheckpointResumeResult{}, s.resumeErr
	}
	return store.CheckpointResumeResult{TaskID: input.TaskID, NodeID: s.derivationSource.NodeID}, nil
}

func (s *recordingTaskStore) ListTaskSummaries(_ context.Context, input store.TaskListQuery) ([]store.TaskListItem, error) {
	s.listQuery = input
	return append([]store.TaskListItem(nil), s.listItems...), nil
}

func (s *recordingTaskStore) CountAuthorizedTaskSummaries(_ context.Context, subjectID string) (int, error) {
	s.countSubject = subjectID
	if s.countErr != nil {
		return 0, s.countErr
	}
	return len(s.listItems), nil
}

type testClock struct{}

func (testClock) Now() time.Time { return time.Now().UTC() }

func TestVersion(t *testing.T) {
	t.Parallel()

	handler := NewHandler(buildinfo.Info{Version: "1.2.3", Commit: "abc", BuildTime: "now"})
	request := httptest.NewRequest(http.MethodGet, "/version", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var body buildinfo.Info
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Version != "1.2.3" || body.Commit != "abc" {
		t.Fatalf("unexpected version response: %#v", body)
	}
}

func TestAPIDomainsFailClosedWithSafeErrorEnvelope(t *testing.T) {
	t.Parallel()
	handler := NewHandler(buildinfo.Info{Version: "test"})
	for _, testCase := range []struct {
		path string
		code string
	}{
		{path: "/api/v1/session", code: "AUTHENTICATION_NOT_CONFIGURED"},
		{path: "/agent/v1/heartbeats", code: "AGENT_AUTHENTICATION_NOT_CONFIGURED"},
		{path: "/unknown", code: "NOT_FOUND"},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			wantStatus := http.StatusServiceUnavailable
			if testCase.code == "NOT_FOUND" {
				wantStatus = http.StatusNotFound
			}
			if response.Code != wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, wantStatus)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body["code"] != testCase.code || body["requestId"] == "" || body["retryable"] != false {
				t.Fatalf("unsafe error body: %#v", body)
			}
		})
	}
}

// TestTaskDerivationEndpoints 验证 EX-I8 派生端点：
// 1) rebuild-draft 从失败任务冻结快照重建 v6 派生草稿（来源标记落库）；
// 2) 非失败任务与未知任务失败关闭；
// 3) resume-checkpoint 按仓储结论创建继续任务或返回稳定门禁错误。
func TestTaskDerivationEndpoints(t *testing.T) {
	t.Parallel()
	const snapshotConfig = `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","scopeKind":"SPECIFIED","table":"synthetic_table","contentKind":"DATA_ONLY","format":"CSV","filePath":"/E:/tmp/out","logPath":"","skipCheckDir":false,"config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`
	failedSource := store.TaskDerivationSource{
		TaskID: "task-failed", CreatorSubjectID: "subject-1", DataSourceID: "source-allowed", NodeID: "node-1",
		SnapshotVersion: "v2", SnapshotJSON: snapshotConfig, State: "FAILED", PlannedCommandRedacted: `obdumper --host 127.0.0.1 --port 2881 --user ****** --database synthetic_db --table synthetic_table --csv --file-path /E:/tmp/out`,
	}
	// 1) 正例：失败任务重建派生草稿。
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{derivationSource: failedSource}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	rebuild := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-failed:rebuild-draft", bytes.NewBufferString(`{"derivation":"RERUN_FROM_SCRATCH"}`))
	rebuild.Header.Set("Idempotency-Key", "synthetic-derivation-rebuild-key-0001")
	rebuilt := httptest.NewRecorder()
	handler.ServeHTTP(rebuilt, rebuild)
	if rebuilt.Code != http.StatusCreated || !bytes.Contains(rebuilt.Body.Bytes(), []byte(`"sourceTaskId":"task-failed"`)) || !bytes.Contains(rebuilt.Body.Bytes(), []byte(`"derivation":"RERUN_FROM_SCRATCH"`)) {
		t.Fatalf("rebuild response=%d body=%s", rebuilt.Code, rebuilt.Body.String())
	}
	if drafts.created.SourceTaskID != "task-failed" || drafts.created.SourceDerivation != "RERUN_FROM_SCRATCH" {
		t.Fatalf("derived draft source = %#v", drafts.created)
	}
	// 2) 非失败任务与未知任务失败关闭。
	pendingTasks := &recordingTaskStore{derivationSource: func() store.TaskDerivationSource {
		source := failedSource
		source.TaskID, source.State = "task-pending", "SUCCEEDED"
		return source
	}()}
	handler = newGeneralizedFlowHandler(t, drafts, prechecks, pendingTasks)
	rebuild = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-pending:rebuild-draft", bytes.NewBufferString(`{"derivation":"REBUILD_FROM_CONFIG"}`))
	rebuild.Header.Set("Idempotency-Key", "synthetic-derivation-rebuild-key-0002")
	rebuilt = httptest.NewRecorder()
	handler.ServeHTTP(rebuilt, rebuild)
	if rebuilt.Code != http.StatusUnprocessableEntity || !bytes.Contains(rebuilt.Body.Bytes(), []byte("TASK_NOT_FAILED")) {
		t.Fatalf("non-failed rebuild response=%d body=%s", rebuilt.Code, rebuilt.Body.String())
	}
	missingTasks := &recordingTaskStore{}
	handler = newGeneralizedFlowHandler(t, drafts, prechecks, missingTasks)
	rebuild = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-none:rebuild-draft", bytes.NewBufferString(`{"derivation":"REBUILD_FROM_CONFIG"}`))
	rebuild.Header.Set("Idempotency-Key", "synthetic-derivation-rebuild-key-0003")
	rebuilt = httptest.NewRecorder()
	handler.ServeHTTP(rebuilt, rebuild)
	if rebuilt.Code != http.StatusNotFound {
		t.Fatalf("missing rebuild response=%d body=%s", rebuilt.Code, rebuilt.Body.String())
	}
	// 3) 检查点继续：正例与门禁负例。
	resumeTasks := &recordingTaskStore{derivationSource: failedSource}
	handler = newGeneralizedFlowHandler(t, drafts, prechecks, resumeTasks)
	resume := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-failed:resume-checkpoint", nil)
	resume.Header.Set("Idempotency-Key", "synthetic-derivation-resume-key-0001")
	resumed := httptest.NewRecorder()
	handler.ServeHTTP(resumed, resume)
	if resumed.Code != http.StatusCreated || !bytes.Contains(resumed.Body.Bytes(), []byte(`"parentTaskId":"task-failed"`)) || !bytes.Contains(resumed.Body.Bytes(), []byte(`"derivationKind":"CHECKPOINT_RESUME"`)) {
		t.Fatalf("resume response=%d body=%s", resumed.Code, resumed.Body.String())
	}
	if resumeTasks.resumeInput.SourceTaskID != "task-failed" || resumeTasks.resumeInput.CreatorSubjectID != "synthetic-subject" {
		t.Fatalf("resume input = %#v", resumeTasks.resumeInput)
	}
	blockedTasks := &recordingTaskStore{derivationSource: failedSource, resumeErr: store.ErrCheckpointResumeUnavailable}
	handler = newGeneralizedFlowHandler(t, drafts, prechecks, blockedTasks)
	resume = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-failed:resume-checkpoint", nil)
	resume.Header.Set("Idempotency-Key", "synthetic-derivation-resume-key-0002")
	resumed = httptest.NewRecorder()
	handler.ServeHTTP(resumed, resume)
	if resumed.Code != http.StatusUnprocessableEntity || !bytes.Contains(resumed.Body.Bytes(), []byte("CHECKPOINT_RESUME_UNAVAILABLE")) {
		t.Fatalf("blocked resume response=%d body=%s", resumed.Code, resumed.Body.String())
	}
}

// TestExportDraftStorageOutputSubmitGate 验证对象存储输出的提交门禁按预检查结果驱动：
// STORAGE_CONNECTIVITY/STORAGE_AUTH 非 PASSED 时 422 STORAGE_PRECHECK_REQUIRED；
// 两项均 PASSED 时任务可以冻结（合成链验证，不连接真实对象存储）。
func TestExportDraftStorageOutputSubmitGate(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"OSS","filePath":"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com","tmpPath":"/E:/workespace/ob-data-orch/tmp/synthetic-staging"}}}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-storage-submit-draft-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-storage-submit-precheck-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted {
		t.Fatalf("storage precheck response=%d body=%s", prechecked.Code, prechecked.Body.String())
	}
	submit := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:submit", bytes.NewBufferString(`{"precheckId":"precheck-synthetic"}`))
		request.Header.Set("If-Match", `"rev-1"`)
		request.Header.Set("Idempotency-Key", "synthetic-storage-submit-task-key")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	// 负例：存储检查 UNKNOWN（探测未授权）必须阻断提交。
	tasks.run = prechecks.created
	tasks.run.Status, tasks.run.IntegrityStatus = "SUCCEEDED", "COMPLETE"
	tasks.run.Results = []store.PrecheckCheckResult{
		{Check: "DATABASE_CONNECTIVITY", Status: "PASSED", EvidenceCode: "DATABASE_CONNECTED"},
		{Check: "OBJECT_ACCESS", Status: "PASSED", EvidenceCode: "OBJECT_ACCESSIBLE"},
		{Check: "TOOL_ENVIRONMENT", Status: "PASSED", EvidenceCode: "TOOL_RUNTIME_READY"},
		{Check: "AVAILABLE_SPACE", Status: "PASSED", EvidenceCode: "OUTPUT_SPACE_SUFFICIENT"},
		{Check: "STORAGE_CONNECTIVITY", Status: "UNKNOWN", EvidenceCode: "STORAGE_CONNECTIVITY_UNAVAILABLE"},
		{Check: "STORAGE_AUTH", Status: "UNKNOWN", EvidenceCode: "STORAGE_AUTH_UNAVAILABLE"},
	}
	blocked := submit()
	if blocked.Code != http.StatusUnprocessableEntity || !strings.Contains(blocked.Body.String(), "STORAGE_PRECHECK_REQUIRED") || tasks.input.TaskID != "" {
		t.Fatalf("storage submit gate response=%d body=%s task=%#v", blocked.Code, blocked.Body.String(), tasks.input)
	}
	// 正例：两项存储检查 PASSED 后任务可以冻结（合成验证，不代表真实存储可用）。
	tasks.run.Results[4].Status, tasks.run.Results[4].EvidenceCode = "PASSED", "STORAGE_ENDPOINT_REACHABLE"
	tasks.run.Results[5].Status, tasks.run.Results[5].EvidenceCode = "PASSED", "STORAGE_CREDENTIAL_VERIFIED"
	allowed := submit()
	if allowed.Code != http.StatusCreated || tasks.input.TaskID == "" {
		t.Fatalf("storage submit allowed response=%d body=%s task=%#v", allowed.Code, allowed.Body.String(), tasks.input)
	}
	if tasks.input.StorageCredentialID != "" || tasks.input.StorageCredentialRevision != 0 {
		t.Fatalf("storage submit must not invent a credential binding: %#v", tasks.input)
	}
}

// recordingStorageCredentialStore 是存储凭据接口的内存记录实现（只用于测试）。
type recordingStorageCredentialStore struct {
	created     *store.StorageCredentialCreate
	rotated     *store.StorageCredentialRotate
	deleted     *store.StorageCredentialDeletion
	listOwner   string
	credentials map[string]store.StorageCredential
}

func newRecordingStorageCredentialStore() *recordingStorageCredentialStore {
	return &recordingStorageCredentialStore{credentials: map[string]store.StorageCredential{}}
}

func (r *recordingStorageCredentialStore) CreateStorageCredential(ctx context.Context, input store.StorageCredentialCreate) (store.StorageCredentialCreateResult, error) {
	r.created = &input
	r.credentials[input.StorageCredentialID] = store.StorageCredential{
		StorageCredentialID: input.StorageCredentialID, OwnerSubjectID: input.OwnerSubjectID, DisplayName: input.DisplayName,
		Provider: input.Provider, CurrentRevision: 1, Revision: 1, CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt,
	}
	return store.StorageCredentialCreateResult{StorageCredentialID: input.StorageCredentialID}, nil
}

func (r *recordingStorageCredentialStore) ListStorageCredentials(ctx context.Context, ownerSubjectID string) ([]store.StorageCredential, error) {
	r.listOwner = ownerSubjectID
	var items []store.StorageCredential
	for _, credential := range r.credentials {
		if credential.OwnerSubjectID == ownerSubjectID {
			items = append(items, credential)
		}
	}
	return items, nil
}

func (r *recordingStorageCredentialStore) GetStorageCredentialReference(ctx context.Context, storageCredentialID string) (store.StorageCredentialReference, error) {
	credential, ok := r.credentials[storageCredentialID]
	if !ok {
		return store.StorageCredentialReference{}, store.ErrStorageCredentialNotFound
	}
	return store.StorageCredentialReference{StorageCredentialID: credential.StorageCredentialID, Provider: credential.Provider, Revision: credential.CurrentRevision, OwnerSubjectID: credential.OwnerSubjectID}, nil
}

func (r *recordingStorageCredentialStore) RotateStorageCredential(ctx context.Context, input store.StorageCredentialRotate) (store.StorageCredential, error) {
	r.rotated = &input
	credential := r.credentials[input.StorageCredentialID]
	if credential.OwnerSubjectID != input.ActorSubjectID {
		return store.StorageCredential{}, store.ErrStorageCredentialForbidden
	}
	if credential.CurrentRevision != input.ExpectedRevision {
		return store.StorageCredential{}, store.ErrStorageCredentialRevision
	}
	credential.CurrentRevision++
	credential.Revision++
	credential.UpdatedAt = input.UpdatedAt
	r.credentials[input.StorageCredentialID] = credential
	return credential, nil
}

func (r *recordingStorageCredentialStore) DeleteStorageCredential(ctx context.Context, input store.StorageCredentialDeletion) error {
	r.deleted = &input
	credential, ok := r.credentials[input.StorageCredentialID]
	if !ok {
		return store.ErrStorageCredentialNotFound
	}
	if credential.OwnerSubjectID != input.ActorSubjectID {
		return store.ErrStorageCredentialForbidden
	}
	if credential.Revision != input.ExpectedRevision {
		return store.ErrStorageCredentialRevision
	}
	delete(r.credentials, input.StorageCredentialID)
	return nil
}

// syntheticStorageCredentialEncryptor 用内存合成密钥构造与真实实现同结构的信封（只用于测试）。
type syntheticStorageCredentialEncryptor struct{}

func (syntheticStorageCredentialEncryptor) Encrypt(keyID string, reference credential.Reference, plaintext []byte) (credential.Envelope, error) {
	return credential.Envelope{FormatVersion: credential.FormatVersion, KeyID: keyID, Reference: reference, Nonce: []byte("nonce"), Ciphertext: append([]byte(nil), plaintext...)}, nil
}

// TestStorageCredentialAPI 验证存储凭据 API 正负例：创建/列表/轮换/删除与秘密不回显。
func TestStorageCredentialAPI(t *testing.T) {
	t.Parallel()
	storage := newRecordingStorageCredentialStore()
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, CSRF: allowedCSRF{},
		StorageCredentials: storage, Encryptor: syntheticStorageCredentialEncryptor{}, CredentialKeyID: "test-key",
	})
	create := httptest.NewRequest(http.MethodPost, "/api/v1/storage-credentials", bytes.NewBufferString(`{"displayName":"合成 OSS 凭据","provider":"OSS","accessKey":"synthetic-access-key","secretKey":"synthetic-secret-key"}`))
	create.Header.Set("Idempotency-Key", "synthetic-storage-credential-create-0001")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated || !bytes.Contains(created.Body.Bytes(), []byte(`"provider":"OSS"`)) {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}
	// 响应与错误不得回显密钥明文。
	if bytes.Contains(created.Body.Bytes(), []byte("synthetic-access-key")) || bytes.Contains(created.Body.Bytes(), []byte("synthetic-secret-key")) {
		t.Fatalf("create response leaked storage secret: %s", created.Body.String())
	}
	// 存储层收到的必须是信封（密文来自合成加密器，与明文同值但经 Envelope 包装；测试断言字段完整性）。
	if storage.created == nil || storage.created.Provider != "OSS" || storage.created.AccessKey.KeyID != "test-key" || len(storage.created.SecretKey.Nonce) == 0 {
		t.Fatalf("stored create input = %#v", storage.created)
	}
	// CSRF 拒绝：写操作必须在入口失败关闭，不能因凭据对象独立绕过浏览器安全上下文。
	csrfDenied := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, CSRF: deniedCSRF{},
		StorageCredentials: storage, Encryptor: syntheticStorageCredentialEncryptor{}, CredentialKeyID: "test-key",
	})
	csrfCreate := httptest.NewRecorder()
	csrfRequest := httptest.NewRequest(http.MethodPost, "/api/v1/storage-credentials", bytes.NewBufferString(`{"displayName":"CSRF 拒绝凭据","provider":"OSS","accessKey":"access","secretKey":"secret"}`))
	csrfRequest.Header.Set("Idempotency-Key", "synthetic-storage-credential-csrf-0001")
	csrfDenied.ServeHTTP(csrfCreate, csrfRequest)
	if csrfCreate.Code != http.StatusUnauthorized || !bytes.Contains(csrfCreate.Body.Bytes(), []byte("CSRF_VALIDATION_FAILED")) {
		t.Fatalf("csrf-denied create response=%d body=%s", csrfCreate.Code, csrfCreate.Body.String())
	}
	// 轮换必须携带合法幂等键，缺失时入口 400 而不是仓储层 500。
	rotateMissingKey := httptest.NewRequest(http.MethodPost, "/api/v1/storage-credentials/"+storage.created.StorageCredentialID+":rotate", bytes.NewBufferString(`{"displayName":"合成 OSS 凭据","provider":"OSS","accessKey":"rotated-access-key","secretKey":"rotated-secret-key"}`))
	rotateMissingKey.Header.Set("If-Match", `"rev-1"`)
	missingKey := httptest.NewRecorder()
	handler.ServeHTTP(missingKey, rotateMissingKey)
	if missingKey.Code != http.StatusBadRequest || !bytes.Contains(missingKey.Body.Bytes(), []byte("IDEMPOTENCY_KEY_INVALID")) {
		t.Fatalf("rotate without idempotency key response=%d body=%s", missingKey.Code, missingKey.Body.String())
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/storage-credentials", nil))
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(`"provider":"OSS"`)) || bytes.Contains(list.Body.Bytes(), []byte("synthetic-access-key")) {
		t.Fatalf("list response=%d body=%s", list.Code, list.Body.String())
	}
	credentialID := storage.created.StorageCredentialID
	// 轮换：版本头 + 新密钥。
	rotate := httptest.NewRequest(http.MethodPost, "/api/v1/storage-credentials/"+credentialID+":rotate", bytes.NewBufferString(`{"displayName":"合成 OSS 凭据","provider":"OSS","accessKey":"rotated-access-key","secretKey":"rotated-secret-key"}`))
	rotate.Header.Set("If-Match", `"rev-1"`)
	rotate.Header.Set("Idempotency-Key", "synthetic-storage-credential-rotate-0001")
	rotated := httptest.NewRecorder()
	handler.ServeHTTP(rotated, rotate)
	if rotated.Code != http.StatusOK || !bytes.Contains(rotated.Body.Bytes(), []byte(`"currentRevision":2`)) {
		t.Fatalf("rotate response=%d body=%s", rotated.Code, rotated.Body.String())
	}
	if bytes.Contains(rotated.Body.Bytes(), []byte("rotated-access-key")) {
		t.Fatalf("rotate response leaked storage secret: %s", rotated.Body.String())
	}
	// 越权负例：非所有者操作返回 404 且不泄露存在性。
	foreign := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOtherIdentityProvider{}, CSRF: allowedCSRF{},
		StorageCredentials: storage, Encryptor: syntheticStorageCredentialEncryptor{}, CredentialKeyID: "test-key",
	})
	foreignDelete := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/storage-credentials/"+credentialID, nil)
	request.Header.Set("If-Match", `"rev-2"`)
	foreign.ServeHTTP(foreignDelete, request)
	if foreignDelete.Code != http.StatusNotFound {
		t.Fatalf("foreign delete response=%d body=%s", foreignDelete.Code, foreignDelete.Body.String())
	}
	// 所有者删除。
	del := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/storage-credentials/"+credentialID, nil)
	request.Header.Set("If-Match", `"rev-2"`)
	handler.ServeHTTP(del, request)
	if del.Code != http.StatusOK {
		t.Fatalf("delete response=%d body=%s", del.Code, del.Body.String())
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/storage-credentials", nil))
	if missing.Code != http.StatusOK || bytes.Contains(missing.Body.Bytes(), []byte(`"provider"`)) {
		t.Fatalf("list after delete response=%d body=%s", missing.Code, missing.Body.String())
	}
}

// recordingTemplateStore 是模板存储接口的内存记录实现（只用于测试）。
type recordingTemplateStore struct {
	created   store.ExportConfigTemplate
	templates map[string]store.ExportConfigTemplate
	renamed   store.ExportConfigTemplateUpdate
	deleted   string
}

func newRecordingTemplateStore() *recordingTemplateStore {
	return &recordingTemplateStore{templates: map[string]store.ExportConfigTemplate{}}
}

func (s *recordingTemplateStore) CreateExportConfigTemplate(_ context.Context, input store.ExportConfigTemplateCreate) (store.ExportConfigTemplateCreateResult, error) {
	s.created = input.ExportConfigTemplate
	s.created.TemplateID = "template-synthetic"
	s.created.Revision = 1
	s.templates[s.created.TemplateID] = s.created
	return store.ExportConfigTemplateCreateResult{TemplateID: s.created.TemplateID}, nil
}

func (s *recordingTemplateStore) ListExportConfigTemplates(_ context.Context, ownerSubjectID string) ([]store.ExportConfigTemplate, error) {
	var items []store.ExportConfigTemplate
	for _, template := range s.templates {
		if template.OwnerSubjectID == ownerSubjectID {
			items = append(items, template)
		}
	}
	return items, nil
}

func (s *recordingTemplateStore) GetAuthorizedExportConfigTemplate(_ context.Context, templateID, subjectID string) (store.ExportConfigTemplate, error) {
	template, ok := s.templates[templateID]
	if !ok || template.OwnerSubjectID != subjectID {
		return store.ExportConfigTemplate{}, store.ErrDataSourceNotFound
	}
	return template, nil
}

func (s *recordingTemplateStore) UpdateExportConfigTemplate(_ context.Context, input store.ExportConfigTemplateUpdate) (int64, error) {
	template, ok := s.templates[input.TemplateID]
	if !ok || template.OwnerSubjectID != input.ActorSubjectID {
		return 0, store.ErrDataSourceNotFound
	}
	if template.Revision != input.ExpectedRevision {
		return 0, store.ErrRevisionConflict
	}
	s.renamed = input
	return input.ExpectedRevision + 1, nil
}

func (s *recordingTemplateStore) DeleteExportConfigTemplate(_ context.Context, templateID, actorSubjectID string, expectedRevision int64, _ string, _ time.Time) error {
	template, ok := s.templates[templateID]
	if !ok || template.OwnerSubjectID != actorSubjectID {
		return store.ErrDataSourceNotFound
	}
	if template.Revision != expectedRevision {
		return store.ErrRevisionConflict
	}
	s.deleted = templateID
	delete(s.templates, templateID)
	return nil
}

// TestExportConfigTemplateEndpoints 验证 EX-I8 模板端点：
// 从成功任务保存模板（剥离凭据引用）、列表/改名/删除与由模板创建草稿。
func TestExportConfigTemplateEndpoints(t *testing.T) {
	t.Parallel()
	const snapshotConfig = `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","scopeKind":"SPECIFIED","table":"synthetic_table","contentKind":"DATA_ONLY","format":"CSV","filePath":"/E:/tmp/out","logPath":"","skipCheckDir":false,"config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`
	succeededSource := store.TaskDerivationSource{
		TaskID: "task-done", CreatorSubjectID: "subject-1", DataSourceID: "source-allowed", NodeID: "node-1",
		SnapshotVersion: "v2", SnapshotJSON: snapshotConfig, State: "SUCCEEDED",
		CapabilityVersion: "export-odp-full-csv-v1", ConfigFingerprint: strings.Repeat("a", 64),
		PlannedCommandRedacted: `obdumper --host 127.0.0.1 --port 2881 --user ****** --database synthetic_db --table synthetic_table --csv --file-path /E:/tmp/out`,
	}
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{derivationSource: succeededSource}
	templates := newRecordingTemplateStore()
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	handler = newHandlerWithTemplates(t, drafts, prechecks, tasks, templates)
	// 保存模板：成功任务 → 201；模板配置不包含凭据引用。
	save := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-done:save-template", bytes.NewBufferString(`{"displayName":"合成成功模板"}`))
	save.Header.Set("Idempotency-Key", "synthetic-template-save-key-0001")
	saved := httptest.NewRecorder()
	handler.ServeHTTP(saved, save)
	if saved.Code != http.StatusCreated || !bytes.Contains(saved.Body.Bytes(), []byte(`"sourceTaskId":"task-done"`)) {
		t.Fatalf("save template response=%d body=%s", saved.Code, saved.Body.String())
	}
	if templates.created.ConfigJSON == "" || strings.Contains(templates.created.ConfigJSON, "storageCredential") || templates.created.SourceTaskID != "task-done" {
		t.Fatalf("saved template = %#v", templates.created)
	}
	// 非成功任务不能保存。
	failedTasks := &recordingTaskStore{derivationSource: func() store.TaskDerivationSource {
		source := succeededSource
		source.TaskID, source.State = "task-failed-2", "FAILED"
		return source
	}()}
	handler = newHandlerWithTemplates(t, drafts, prechecks, failedTasks, templates)
	save = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-failed-2:save-template", bytes.NewBufferString(`{"displayName":"失败任务模板"}`))
	save.Header.Set("Idempotency-Key", "synthetic-template-save-key-0002")
	saved = httptest.NewRecorder()
	handler.ServeHTTP(saved, save)
	if saved.Code != http.StatusUnprocessableEntity || !bytes.Contains(saved.Body.Bytes(), []byte("TASK_NOT_SUCCEEDED")) {
		t.Fatalf("failed save response=%d body=%s", saved.Code, saved.Body.String())
	}
	// 列表/改名/删除。
	handler = newHandlerWithTemplates(t, drafts, prechecks, tasks, templates)
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/export-config-templates", nil))
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(`"displayName":"合成成功模板"`)) || bytes.Contains(list.Body.Bytes(), []byte("objectScope")) {
		t.Fatalf("template list response=%d body=%s", list.Code, list.Body.String())
	}
	rename := httptest.NewRequest(http.MethodPatch, "/api/v1/export-config-templates/template-synthetic", bytes.NewBufferString(`{"displayName":"改名模板"}`))
	rename.Header.Set("If-Match", `"rev-1"`)
	renamed := httptest.NewRecorder()
	handler.ServeHTTP(renamed, rename)
	if renamed.Code != http.StatusOK || !bytes.Contains(renamed.Body.Bytes(), []byte(`"revision":2`)) {
		t.Fatalf("rename response=%d body=%s", renamed.Code, renamed.Body.String())
	}
	// 由模板创建草稿：需要数据源与节点，模板配置不含凭据。
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-config-templates/template-synthetic:create-draft", bytes.NewBufferString(`{"dataSourceId":"source-allowed","nodeId":"node-1"}`))
	create.Header.Set("Idempotency-Key", "synthetic-template-draft-key-0001")
	createdDraft := httptest.NewRecorder()
	handler.ServeHTTP(createdDraft, create)
	if createdDraft.Code != http.StatusCreated || !bytes.Contains(createdDraft.Body.Bytes(), []byte(`"templateId":"template-synthetic"`)) {
		t.Fatalf("create draft from template response=%d body=%s", createdDraft.Code, createdDraft.Body.String())
	}
	// 删除。
	del := httptest.NewRequest(http.MethodDelete, "/api/v1/export-config-templates/template-synthetic", nil)
	del.Header.Set("If-Match", `"rev-1"`)
	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, del)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete response=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

// newHandlerWithTemplates 在泛化流程夹具上追加模板存储依赖。
func newHandlerWithTemplates(t *testing.T, drafts *recordingDraftStore, prechecks *recordingPrecheckStore, tasks *recordingTaskStore, templates *recordingTemplateStore) http.Handler {
	t.Helper()
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	generalized, err := commandgen.NewGeneralized()
	if err != nil {
		t.Fatalf("NewGeneralized() error = %v", err)
	}
	coordinator, err := agentstate.NewCoordinator(testClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	return NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Prechecks: prechecks, Tasks: tasks,
		Templates: templates, Generator: generator, GeneralizedGenerator: generalized, PrecheckTTL: time.Minute, Coordinator: coordinator, CSRF: allowedCSRF{},
	})
}
