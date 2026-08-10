package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	body := []byte(`{"displayName":"Created Source","environment":"TEST","connectionKind":"ODP","compatibilityMode":"MYSQL","host":"127.0.0.1","port":2881,"clusterName":"synthetic-cluster","tenantName":"synthetic-tenant","username":"synthetic-user","password":"synthetic-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources", bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "synthetic-idempotency-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || creator.input.DataSourceID == "" || creator.input.ClusterName != "synthetic-cluster" || creator.input.TenantName != "synthetic-tenant" || len(creator.input.Ciphertext) == 0 {
		t.Fatalf("create response=%d input=%#v", response.Code, creator.input)
	}
	serialized, _ := json.Marshal(creator.input)
	if bytes.Contains(serialized, []byte("synthetic-password")) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatal("plaintext password escaped create boundary")
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
		Identity:    browserOnlyIdentityProvider{},
		Authorizer:  sourceAuthorizer{allowedID: "source-allowed"},
		DataSources: staticDataSourceReader{},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		RequestID string               `json:"requestId"`
		Items     []dataSourceResponse `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if body.RequestID == "" || len(body.Items) != 1 || body.Items[0].ID != "source-allowed" {
		t.Fatalf("unexpected data source list: %#v", body)
	}
	if body.Items[0].CredentialRevision != 2 {
		t.Fatalf("unexpected safe projection: %#v", body.Items[0])
	}
	if bytes.Contains(response.Body.Bytes(), []byte("synthetic-user")) {
		t.Fatal("浏览器数据源响应不得包含用户名")
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

func TestDeleteDataSourceReturnsActualOutcome(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name     string
		outcome  string
		revision int64
	}{
		{name: "physical delete", outcome: "DELETED", revision: 0},
		{name: "archive referenced", outcome: "ARCHIVED", revision: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			deleter := &recordingDataSourceDeleter{result: store.DataSourceDeletionResult{Outcome: testCase.outcome, Revision: testCase.revision}}
			handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
				Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
				Deleter: deleter, CSRF: allowedCSRF{},
			})
			request := httptest.NewRequest(http.MethodDelete, "/api/v1/data-sources/source-allowed", nil)
			request.Header.Set("If-Match", `"rev-1"`)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			var body struct {
				Outcome  string `json:"outcome"`
				Revision int64  `json:"revision"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode deletion response: %v", err)
			}
			if response.Code != http.StatusOK || body.Outcome != testCase.outcome || body.Revision != testCase.revision || deleter.input.DataSourceID != "source-allowed" || deleter.input.ExpectedRevision != 1 {
				t.Fatalf("delete response=%d body=%#v input=%#v", response.Code, body, deleter.input)
			}
		})
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
	})
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
	if response.Code != http.StatusOK || updater.input.DisplayName != "Updated" || updater.input.Password == nil || len(updater.input.Password.Ciphertext) == 0 || !bytes.Contains(response.Body.Bytes(), []byte(`"state":"DISABLED"`)) {
		t.Fatalf("update response=%d input=%#v", response.Code, updater.input)
	}
	serialized, _ := json.Marshal(updater.input)
	if bytes.Contains(serialized, []byte("synthetic-rotated-password")) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-rotated-password")) {
		t.Fatal("plaintext password escaped update boundary")
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
		Generator: generator, GeneralizedGenerator: generalized, PrecheckTTL: time.Minute, Coordinator: coordinator, CSRF: allowedCSRF{},
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

func TestExportDraftV6FullCSVAllScopeFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "full-all")
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v6" {
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
	if !strings.Contains(previewBody, `"-ttable_one,table_two"`) || !strings.Contains(previewBody, `"--exclude-table"`) || !strings.Contains(previewBody, `"table_tmp"`) {
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
	if !strings.Contains(previewBody, `"--ddl"`) || !strings.Contains(previewBody, `"--csv"`) || !strings.Contains(previewBody, `"-ttable_one"`) {
		t.Fatalf("ddl-csv preview unexpected: %s", previewBody)
	}
	if !strings.Contains(snapshotBody, `"format":"DDL_CSV"`) {
		t.Fatalf("ddl-csv snapshot unexpected: %s", snapshotBody)
	}
}

// TestExportDraftV6CSVOptionsFlow 验证 EX-I3 全量选项进入命令且能力仍为 full-csv。
func TestExportDraftV6CSVOptionsFlow(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	prechecks := &recordingPrecheckStore{}
	tasks := &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV","csvOptions":{"skipHeader":true,"columnSeparator":"|","columnQuote":"'","columnQuoteMode":"minimal","escapeCharacter":"\\","lineSeparator":"\\r\\n","nullString":"NULL","fileEncoding":"UTF-8","withTrim":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","noNestedDir":true,"maxFileSize":1048576,"retainEmptyFiles":true,"compress":true,"compressionAlgo":"zstd"},"filterConfig":{"includeColumnNames":["col_a","col_b"],"excludeVirtualColumns":true,"flashbackScn":100},"performanceConfig":{"thread":4,"pageSize":1000,"parallelMacro":8,"fetchSize":100,"jvmMemory":"4G"}}}`
	previewBody, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi3-options")
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" {
		t.Fatalf("options draft capability=%s", drafts.created.CapabilityVersion)
	}
	for _, token := range []string{`"--skip-header"`, `"--column-separator"`, `"|"`, `"--column-quote-mode"`, `"minimal"`, `"--escape-character"`, `"--line-separator"`, `"--null-string"`, `"NULL"`, `"--file-encoding"`, `"UTF-8"`, `"--with-trim"`, `"--compress"`, `"--compression-algo"`, `"zstd"`, `"--no-nested-dir"`, `"--max-file-size"`, `"1048576"`, `"--retain-empty-files"`, `"--include-column-names"`, `"col_a,col_b"`, `"--exclude-virtual-columns"`, `"--flashback-scn"`, `"100"`, `"--thread"`, `"4"`, `"--page-size"`, `"1000"`, `"--parallel-macro"`, `"8"`, `"--fetch-size"`, `"100"`, `"--mem"`, `"4G"`} {
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
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CUT","cutOptions":{"trailDelimiter":true,"removeNewline":true},"csvOptions":{"escapeCharacter":"\\","lineSeparator":"\\n","nullString":"NULL","fileEncoding":"UTF-8","withTrim":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","compress":true,"compressionAlgo":"zstd","noNestedDir":true,"maxFileSize":1048576,"retainEmptyFiles":true},"filterConfig":{"querySql":"select 1","includeColumnNames":["col_a","col_b"]},"performanceConfig":{"thread":4,"pageSize":1000,"jvmMemory":"4G"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi4-cut")
	if drafts.created.CapabilityVersion != "export-odp-cut-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v6" {
		t.Fatalf("cut draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	for _, token := range []string{`"--cut"`, `"--trail-delimiter"`, `"--remove-newline"`, `"--escape-character"`, `"--line-separator"`, `"--null-string"`, `"NULL"`, `"--file-encoding"`, `"UTF-8"`, `"--with-trim"`, `"--compress"`, `"--compression-algo"`, `"zstd"`, `"--no-nested-dir"`, `"--max-file-size"`, `"1048576"`, `"--retain-empty-files"`, `"--query-sql"`, `"select 1"`, `"--include-column-names"`, `"col_a,col_b"`, `"--thread"`, `"4"`, `"--page-size"`, `"1000"`, `"--mem"`, `"4G"`} {
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
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"SQL","csvOptions":{"lineSeparator":"\\n","fileEncoding":"UTF-8"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","compress":true,"compressionAlgo":"gzip","retainEmptyFiles":true},"filterConfig":{"flashbackScn":100,"excludeColumnNames":["col_c"]},"performanceConfig":{"thread":4,"fetchSize":100,"jvmMemory":"4G"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi4-sql")
	if drafts.created.CapabilityVersion != "export-odp-sql-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v6" {
		t.Fatalf("sql draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	for _, token := range []string{`"--sql"`, `"--line-separator"`, `"--file-encoding"`, `"UTF-8"`, `"--compress"`, `"--compression-algo"`, `"gzip"`, `"--retain-empty-files"`, `"--flashback-scn"`, `"100"`, `"--exclude-column-names"`, `"col_c"`, `"--thread"`, `"4"`, `"--fetch-size"`, `"100"`, `"--mem"`, `"4G"`} {
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
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"POS"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output","controlFilePath":"/E:/workespace/ob-data-orch/tmp/synthetic-controls","compress":true,"compressionAlgo":"zstd","noNestedDir":true},"filterConfig":{"querySql":"select 1"},"performanceConfig":{"thread":4,"jvmMemory":"4G"}}}`
	previewBody, snapshotBody := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "exi4-pos")
	if drafts.created.CapabilityVersion != "export-odp-pos-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v6" {
		t.Fatalf("pos draft capability=%s metadata=%s", drafts.created.CapabilityVersion, drafts.created.MetadataVersion)
	}
	for _, token := range []string{`"--pos"`, `"--ctl-path"`, `"/E:/workespace/ob-data-orch/tmp/synthetic-controls"`, `"--compress"`, `"--compression-algo"`, `"zstd"`, `"--no-nested-dir"`, `"--query-sql"`, `"select 1"`, `"--thread"`, `"4"`, `"--mem"`, `"4G"`} {
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
		if drafts.created.CapabilityVersion != testCase.capability || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v6" {
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
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v6" {
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
	// 存储专用预检查完成前，预检查与提交均被功能门禁拦截（STORAGE_PRECHECK_UNAVAILABLE）。
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-exi6-oss-precheck-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusUnprocessableEntity || !strings.Contains(prechecked.Body.String(), "STORAGE_PRECHECK_UNAVAILABLE") {
		t.Fatalf("oss precheck gate response=%d body=%s", prechecked.Code, prechecked.Body.String())
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
		"SQL 携带转义字符":      {`{` + base + `,"dataFormat":{"formatKind":"SQL","csvOptions":{"escapeCharacter":"\\"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"SQL 携带 CUT 专属选项": {`{` + base + `,"dataFormat":{"formatKind":"SQL","cutOptions":{"removeNewline":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
		"DDL 与数据固定 CSV":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"CUT"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`},
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
	if drafts.created.CapabilityVersion != "export-odp-full-csv-v1" || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v6" {
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
		"DDL+数据声明 CUT":   {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"CUT"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"仅 DDL 内容声明无效格式": {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"dataFormat":{"formatKind":"CUT"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}}`, http.StatusUnprocessableEntity},
		"对象存储输出":         {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"OSS","filePath":"oss://bucket/out?access-key=x&secret-key=y"}}}`, http.StatusUnprocessableEntity},
		"筛选配置未启用":        {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"filterConfig":{"where":"id > 0"}}}`, http.StatusUnprocessableEntity},
		"重试选项未启用":        {`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"},"performanceConfig":{"retry":true}}}`, http.StatusUnprocessableEntity},
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
	input  store.DataSourceDeletion
	result store.DataSourceDeletionResult
	err    error
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

func (d *recordingDataSourceDeleter) DeleteOrArchiveDataSource(_ context.Context, input store.DataSourceDeletion) (store.DataSourceDeletionResult, error) {
	d.input = input
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
	finished   []store.PrecheckSecretResolutionOutcome
}

func (s *recordingPrecheckSecretStore) ResolvePrecheckDatabaseConnection(_ context.Context, _ store.PrecheckSecretResolutionRequest) (store.EncryptedPrecheckDatabaseConnection, error) {
	return s.connection, nil
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

// TestExportDraftStorageOutputPrecheckGate 验证对象存储输出的草稿在存储专用预检查
// （凭据、网络、权限、空间）完成前，固定预检查与任务提交均返回稳定的功能门禁错误，
// 且真实 SQLite 预检查仓储不产生任何记录（门禁必须阻止 CreatePrecheck 被真实调用）。
// 该测试使用真实 SQLite 仓储链而非记录型假预检查存储。
func TestExportDraftStorageOutputPrecheckGate(t *testing.T) {
	t.Parallel()
	drafts := &recordingDraftStore{}
	tasks := &recordingTaskStore{}
	databasePath := filepath.Join(t.TempDir(), "metadata.db")
	realStore, err := store.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open real sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = realStore.Close() })
	handler := newGeneralizedFlowHandler(t, drafts, realStore, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"OSS","filePath":"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com","tmpPath":"/E:/workespace/ob-data-orch/tmp/synthetic-staging"}}}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-storage-gate-draft-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response=%d body=%s", created.Code, created.Body.String())
	}
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-storage-gate-precheck-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusUnprocessableEntity || !strings.Contains(prechecked.Body.String(), "STORAGE_PRECHECK_UNAVAILABLE") {
		t.Fatalf("storage precheck gate response=%d body=%s", prechecked.Code, prechecked.Body.String())
	}
	// 真实 SQLite 仓储链验证：门禁错误发生后 precheck_runs 表必须保持零记录。
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(databasePath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open database for verification: %v", err)
	}
	defer db.Close()
	var precheckCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM precheck_runs`).Scan(&precheckCount); err != nil {
		t.Fatalf("count precheck runs: %v", err)
	}
	if precheckCount != 0 {
		t.Fatalf("precheck_runs count = %d, want 0（门禁必须阻止真实仓储写入）", precheckCount)
	}
}
