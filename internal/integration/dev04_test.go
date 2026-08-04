package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/store"

	_ "modernc.org/sqlite"
)

// TestDEV04SyntheticSQLiteHTTPChain 以临时 SQLite 和假 Agent 证明首条 API 链路。
// 该测试不监听端口、不解析真实凭据、不连接数据库，也不启动任何官方工具进程。
func TestDEV04SyntheticSQLiteHTTPChain(t *testing.T) {
	t.Parallel()
	metadata, databasePath := openSyntheticStore(t)
	seedSyntheticMetadata(t, databasePath)
	coordinator, err := agentstate.NewCoordinator(integrationClock{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	generator, err := commandgen.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "synthetic-test"}, controlplane.Dependencies{
		Identity:       integrationIdentityProvider{},
		Authorizer:     integrationAuthorizer{},
		DataSources:    metadata,
		CredentialRefs: metadata,
		Drafts:         metadata,
		Prechecks:      metadata,
		Tasks:          metadata,
		Executions:     metadata,
		Nodes:          integrationNodeReader{},
		Generator:      generator,
		PrecheckTTL:    time.Minute,
		Coordinator:    coordinator,
		CSRF:           integrationCSRF{},
		LogLedger:      logstream.NewBatchLedger(),
	})

	draftID := requestID(t, handler, http.MethodPost, "/api/v1/export-drafts", `{"dataSourceId":"source-1","nodeId":"node-1","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/workespace/ob-data-orch/tmp/synthetic-output"}`, map[string]string{"Idempotency-Key": "synthetic-draft-idempotency"}, http.StatusCreated)
	preview := request(t, handler, http.MethodPost, "/api/v1/export-drafts/"+draftID+":preview-command", "", map[string]string{"If-Match": `"rev-1"`}, http.StatusOK)
	if !bytes.Contains(preview, []byte("-usynthetic-user@synthetic-tenant#synthetic-cluster")) || !bytes.Contains(preview, []byte("-p ******")) || bytes.Contains(preview, []byte("--password")) {
		t.Fatalf("preview did not preserve non-password command fields: %s", preview)
	}
	precheckID := requestID(t, handler, http.MethodPost, "/api/v1/export-drafts/"+draftID+":precheck", "", map[string]string{"If-Match": `"rev-1"`, "Idempotency-Key": "synthetic-precheck-idempotency"}, http.StatusAccepted)

	precheckClaim := request(t, handler, http.MethodPost, "/agent/v1/prechecks:claim", `{"requestId":"precheck-claim-1","precheckId":"`+precheckID+`","nodeId":"node-1","leaseId":"precheck-lease-1"}`, nil, http.StatusOK)
	var precheckGrant struct {
		Grant struct {
			LeaseEpoch int64 `json:"leaseEpoch"`
		} `json:"grant"`
	}
	decodeJSON(t, precheckClaim, &precheckGrant)
	request(t, handler, http.MethodPost, "/agent/v1/prechecks/"+precheckID+":complete", `{"requestId":"precheck-complete-1","leaseId":"precheck-lease-1","leaseEpoch":`+itoa(precheckGrant.Grant.LeaseEpoch)+`,"succeeded":true}`, nil, http.StatusOK)

	taskID := requestID(t, handler, http.MethodPost, "/api/v1/export-drafts/"+draftID+":submit", `{"precheckId":"`+precheckID+`"}`, map[string]string{"If-Match": `"rev-1"`, "Idempotency-Key": "synthetic-task-idempotency"}, http.StatusCreated)
	overview := request(t, handler, http.MethodGet, "/api/v1/tasks/"+taskID, "", nil, http.StatusOK)
	if !bytes.Contains(overview, []byte(`"type":"OBDUMPER_EXPORT"`)) || bytes.Contains(overview, []byte("plannedCommand")) || bytes.Contains(overview, []byte("WAITING_SCHEDULE")) {
		t.Fatalf("task overview did not preserve its projection boundary: %s", overview)
	}
	command := request(t, handler, http.MethodGet, "/api/v1/tasks/"+taskID+"/command-evidence", "", nil, http.StatusOK)
	if !bytes.Contains(command, []byte("-usynthetic-user@synthetic-tenant#synthetic-cluster")) || !bytes.Contains(command, []byte("-p ******")) || bytes.Contains(command, []byte("--password")) {
		t.Fatalf("task command did not preserve the password-only display boundary: %s", command)
	}
	execution := request(t, handler, http.MethodGet, "/api/v1/tasks/"+taskID+"/execution", "", nil, http.StatusOK)
	if !bytes.Contains(execution, []byte("WAITING_SCHEDULE")) || bytes.Contains(execution, []byte("plannedCommand")) {
		t.Fatalf("task execution did not preserve its projection boundary: %s", execution)
	}

	executionClaim := request(t, handler, http.MethodPost, "/agent/v1/executions:claim", `{"requestId":"execution-claim-1","taskId":"`+taskID+`","executionId":"execution-1","nodeId":"node-1","leaseId":"execution-lease-1"}`, nil, http.StatusOK)
	var executionGrant struct {
		Grant agentstate.LeaseGrant `json:"grant"`
	}
	decodeJSON(t, executionClaim, &executionGrant)
	request(t, handler, http.MethodPost, "/agent/v1/executions/execution-1:events:append", `{"eventId":"event-started-1","leaseId":"execution-lease-1","leaseEpoch":`+itoa(executionGrant.Grant.LeaseEpoch)+`,"sequence":2,"type":"PROCESS_STARTED"}`, nil, http.StatusOK)
	request(t, handler, http.MethodPost, "/agent/v1/executions/execution-1:logs:append", `{"leaseId":"execution-lease-1","leaseEpoch":`+itoa(executionGrant.Grant.LeaseEpoch)+`,"batch":{"streamId":"execution-1","sourceEpoch":1,"firstSeq":1,"lastSeq":1,"previousDigest":"","policyVersion":"synthetic-v1","records":[{"streamId":"execution-1","sourceEpoch":1,"sourceSeq":1,"kind":"LOG","message":"safe synthetic log","policyVersion":"synthetic-v1"}]}}`, nil, http.StatusAccepted)
	logs := request(t, handler, http.MethodGet, "/api/v1/tasks/"+taskID+"/logs", "", nil, http.StatusOK)
	if !bytes.Contains(logs, []byte("safe synthetic log")) || !bytes.Contains(logs, []byte("SYNTHETIC_MEMORY")) {
		t.Fatalf("synthetic logs were not safely projected: %s", logs)
	}

	assertCount(t, databasePath, "SELECT COUNT(*) FROM tasks", 1)
	assertCount(t, databasePath, "SELECT COUNT(*) FROM task_executions", 1)
	assertCount(t, databasePath, "SELECT COUNT(*) FROM execution_events", 2)
}

type integrationIdentityProvider struct{}

func (integrationIdentityProvider) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "subject-1"}, nil
}

func (integrationIdentityProvider) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.AgentPrincipal, ID: "agent-1"}, nil
}

type integrationAuthorizer struct{}

func (integrationAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, objectID string) error {
	if (scope == identity.ScopeDataSourceRead && objectID == "source-1") || (scope == identity.ScopeNodeUse && objectID == "node-1") {
		return nil
	}
	return errors.New("synthetic scope denied")
}

type integrationNodeReader struct{}

func (integrationNodeReader) GetExecutionNodeFact(context.Context, string) (controlplane.ExecutionNodeFact, error) {
	return controlplane.ExecutionNodeFact{NodeID: "node-1", Platform: commandgen.PlatformWindowsAMD64, FactsVersion: "synthetic-node-v1", FactsRevision: 1}, nil
}

type integrationCSRF struct{}

func (integrationCSRF) ValidateCSRF(*http.Request) error { return nil }

type integrationClock struct{}

func (integrationClock) Now() time.Time { return time.Now().UTC() }

func openSyntheticStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "synthetic-dev04.db")
	metadata, err := store.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = metadata.Close() })
	return metadata, databasePath
}

func seedSyntheticMetadata(t *testing.T, databasePath string) {
	t.Helper()
	// 测试通过第二个 SQLite 连接写入固定元数据；运行时路径不暴露此播种能力。
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(databasePath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open synthetic SQLite seed connection: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Now().UTC().Format(time.RFC3339Nano)
	allowedRootsJSON := `["E:\\workespace\\ob-data-orch\\tmp"]`
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO auth_subjects VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, []any{"subject-1", "synthetic-external", "Synthetic User", now, now}},
		{`INSERT INTO data_sources VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', ?, 2881, ?, ?, ?, 1, 'ENABLED', 1, 'SUCCEEDED', ?, '{}', ?, ?, ?, 'synthetic-cluster', 'synthetic-tenant', 'AGENT_JDBC')`, []any{"source-1", "Synthetic Source", "synthetic source", "127.0.0.1", "synthetic-user", "synthetic_db", "11111111-1111-4111-8111-111111111111", now, "subject-1", now, now}},
		{`INSERT INTO credential_revisions VALUES (?, 1, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)`, []any{"11111111-1111-4111-8111-111111111111", "source-1", "synthetic-key", []byte{1, 2, 3}, []byte{4, 5, 6}, now}},
		{`INSERT INTO execution_nodes(node_id, display_name, normalized_name, platform, management_state, allowed_roots_json, tool_home, java_path, tool_config_ref, revision, created_by, created_at, updated_at) VALUES (?, ?, ?, 'WINDOWS_AMD64', 'ENABLED', ?, ?, ?, NULL, 1, ?, ?, ?)`, []any{"node-1", "Synthetic Node", "synthetic node", allowedRootsJSON, `E:\synthetic\ob-loader-dumper`, `C:\synthetic\java8\bin\java.exe`, "subject-1", now, now}},
		{`INSERT INTO agents(
            agent_id, node_id, credential_digest, credential_revision, status,
            protocol_version, boot_id, last_heartbeat_at, capacity_total, capacity_used,
            facts_json, facts_revision, created_at, revoked_at
        ) VALUES (?, ?, ?, 1, 'ACTIVE', ?, ?, ?, 1, 0, NULL, 1, ?, NULL)`, []any{"agent-1", "node-1", []byte{7, 8, 9}, "synthetic-agent-v1", "synthetic-boot-1", now, now}},
	}
	for index, statement := range statements {
		if _, err := database.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed synthetic metadata statement %d: %v", index+1, err)
		}
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string, want int) []byte {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, want, response.Body.String())
	}
	return response.Body.Bytes()
}

func requestID(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string, want int) string {
	t.Helper()
	var response struct {
		ID string `json:"id"`
	}
	decodeJSON(t, request(t, handler, method, path, body, headers, want), &response)
	if response.ID == "" {
		t.Fatal("response id is empty")
	}
	return response.ID
}

func decodeJSON(t *testing.T, payload []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatalf("decode JSON: %v; payload=%s", err, payload)
	}
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }

func assertCount(t *testing.T, databasePath, query string, want int) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(databasePath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open synthetic SQLite assertion connection: %v", err)
	}
	defer database.Close()
	var got int
	if err := database.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query synthetic SQLite count: %v", err)
	}
	if got != want {
		t.Fatalf("synthetic SQLite count=%d want=%d for %s", got, want, query)
	}
}
