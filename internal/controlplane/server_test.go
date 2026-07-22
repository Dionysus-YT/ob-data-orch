package controlplane

import (
	"bytes"
	"context"
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
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

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
	body := []byte(`{"displayName":"Created Source","environment":"TEST","connectionKind":"OBSERVER_DIRECT","compatibilityMode":"MYSQL","host":"127.0.0.1","port":2881,"username":"synthetic-user","password":"synthetic-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources", bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "synthetic-idempotency-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || creator.input.DataSourceID == "" || len(creator.input.Ciphertext) == 0 {
		t.Fatalf("create response=%d input=%#v", response.Code, creator.input)
	}
	serialized, _ := json.Marshal(creator.input)
	if bytes.Contains(serialized, []byte("synthetic-password")) || bytes.Contains(response.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatal("plaintext password escaped create boundary")
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
	if body.Items[0].CredentialRevision != 2 || body.Items[0].Username != "synthetic-user" {
		t.Fatalf("unexpected safe projection: %#v", body.Items[0])
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

func TestUpdateDataSourceEncryptsPasswordAndKeepsItOutOfResponses(t *testing.T) {
	t.Parallel()
	keyring, err := credential.NewKeyring(map[string][]byte{"test-key": bytes.Repeat([]byte{9}, 32)})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	updater := &recordingUpdater{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sourceAuthorizer{allowedID: "source-allowed", allowWrite: true},
		DataSources: staticDataSourceReader{}, Updater: updater, CredentialRefs: staticCredentialReferenceReader{},
		Encryptor: keyring, CSRF: allowedCSRF{}, CredentialKeyID: "test-key",
	})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/data-sources/source-allowed", bytes.NewBufferString(`{"displayName":"Updated","password":"synthetic-rotated-password"}`))
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || updater.input.DisplayName != "Updated" || updater.input.Password == nil || len(updater.input.Password.Ciphertext) == 0 {
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
	coordinator, err := agentstate.NewCoordinator(testClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
		Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{},
		CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, Drafts: drafts, Prechecks: prechecks, Generator: generator, PrecheckTTL: time.Minute, Coordinator: coordinator, CSRF: allowedCSRF{},
	})
	body := `{"dataSourceId":"source-allowed","nodeId":"node-1","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"E:\\workespace\\ob-data-orch\\tmp\\synthetic-output"}`
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
	if previewed.Code != http.StatusOK || !bytes.Contains(previewed.Body.Bytes(), []byte("******")) || bytes.Contains(previewed.Body.Bytes(), []byte("synthetic_user")) || bytes.Contains(previewed.Body.Bytes(), []byte("--password")) {
		t.Fatalf("unsafe preview response=%d body=%s", previewed.Code, previewed.Body.String())
	}
	precheck := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:precheck", nil)
	precheck.Header.Set("If-Match", `"rev-1"`)
	precheck.Header.Set("Idempotency-Key", "synthetic-precheck-idempotency-key")
	prechecked := httptest.NewRecorder()
	handler.ServeHTTP(prechecked, precheck)
	if prechecked.Code != http.StatusAccepted || prechecks.created.DraftID != "draft-synthetic" || prechecks.created.CredentialRevision != 1 {
		t.Fatalf("precheck response=%d binding=%#v", prechecked.Code, prechecks.created)
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

type sourceAuthorizer struct {
	allowedID  string
	allowWrite bool
}

type sliceAuthorizer struct{}

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

type staticDataSourceReader struct{}

func (staticDataSourceReader) ListDataSourceSummaries(context.Context) ([]store.DataSourceSummary, error) {
	return []store.DataSourceSummary{
		{DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "OBSERVER_DIRECT", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881, Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 2},
		{DataSourceID: "source-denied", DisplayName: "Denied", Environment: "TEST", ConnectionKind: "OBSERVER_DIRECT", CompatibilityMode: "MYSQL", Host: "127.0.0.2", Port: 2881, Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 3},
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

func mustStaticDataSourceReader() staticDataSources {
	return staticDataSources{summaries: []store.DataSourceSummary{
		{DataSourceID: "source-allowed", DisplayName: "Allowed", Environment: "TEST", ConnectionKind: "OBSERVER_DIRECT", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881, Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 2},
		{DataSourceID: "source-denied", DisplayName: "Denied", Environment: "TEST", ConnectionKind: "OBSERVER_DIRECT", CompatibilityMode: "MYSQL", Host: "127.0.0.2", Port: 2881, Username: "synthetic-user", State: "ENABLED", Revision: 1, CredentialRevision: 3},
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

type recordingStateChanger struct{ input store.DataSourceStateChange }

func (c *recordingStateChanger) ChangeDataSourceState(_ context.Context, input store.DataSourceStateChange) (store.DataSourceStateChangeResult, error) {
	c.input = input
	return store.DataSourceStateChangeResult{State: input.TargetState, Revision: 2}, nil
}

type staticCredentialReferenceReader struct{}

func (staticCredentialReferenceReader) GetDataSourceCredentialReference(context.Context, string) (store.DataSourceCredentialReference, error) {
	return store.DataSourceCredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}, nil
}

type recordingUpdater struct{ input store.DataSourceUpdate }

func (u *recordingUpdater) UpdateDataSource(_ context.Context, input store.DataSourceUpdate) (store.DataSourceUpdateResult, error) {
	u.input = input
	credentialRevision := int64(2)
	if input.Password == nil {
		credentialRevision = 1
	}
	return store.DataSourceUpdateResult{Revision: input.ExpectedRevision + 1, CredentialRevision: credentialRevision}, nil
}

type staticNodeReader struct{}

func (staticNodeReader) GetExecutionNodeFact(context.Context, string) (ExecutionNodeFact, error) {
	return ExecutionNodeFact{NodeID: "node-1", Platform: commandgen.PlatformWindowsAMD64, FactsVersion: "node-facts-1", FactsRevision: 1}, nil
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
	s.created.ConfigJSON, s.created.ConfigFingerprint, s.created.Revision = input.ConfigJSON, input.ConfigFingerprint, input.ExpectedRevision+1
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
