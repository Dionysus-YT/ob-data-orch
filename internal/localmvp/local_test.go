package localmvp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/store"
)

func TestPrepareStoreAllowsLoopbackDataSourceCreate(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "local-mvp.db"))
	if err != nil {
		t.Fatalf("打开本机 MVP 数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := PrepareStore(context.Background(), database); err != nil {
		t.Fatalf("PrepareStore() 失败: %v", err)
	}
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("创建测试密钥环失败: %v", err)
	}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, mustDependencies(t, database, keyring))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources", bytes.NewBufferString(`{"displayName":"Synthetic Source","environment":"TEST","connectionKind":"ODP","compatibilityMode":"MYSQL","host":"192.0.2.10","port":2881,"clusterName":"synthetic-cluster","tenantName":"synthetic-tenant","username":"synthetic-user","password":"synthetic-password"}`))
	request.RemoteAddr = "127.0.0.1:12000"
	request.Header.Set("Idempotency-Key", "synthetic-local-mvp-create")
	request.Header.Set("X-CSRF-Token", CSRFToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("本机 MVP 创建数据源状态=%d，响应=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatal("本机 MVP 创建响应泄露了密码")
	}
}

func TestPrepareStoreAllowsDisabledExecutionNodeRegistration(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "local-mvp.db"))
	if err != nil {
		t.Fatalf("打开本机 MVP 数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := PrepareStore(context.Background(), database); err != nil {
		t.Fatalf("PrepareStore() 失败: %v", err)
	}
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("创建测试密钥环失败: %v", err)
	}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, mustDependencies(t, database, keyring))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"Local Windows Node","platform":"WINDOWS_AMD64","toolHome":"E:\\tools\\ob-loader-dumper","javaPath":"C:\\Java\\jdk8\\bin\\java.exe","allowedRoots":["E:\\ob-data\\exports"]}`))
	request.RemoteAddr = "127.0.0.1:12000"
	request.Header.Set("Idempotency-Key", "synthetic-local-mvp-node-create")
	request.Header.Set("X-CSRF-Token", CSRFToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || !bytes.Contains(response.Body.Bytes(), []byte(`"managementState":"DISABLED"`)) {
		t.Fatalf("本机 MVP 创建节点状态=%d，响应=%s", response.Code, response.Body.String())
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes", nil)
	listRequest.RemoteAddr = "127.0.0.1:12000"
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !bytes.Contains(listResponse.Body.Bytes(), []byte(`"acceptsNewTasks":false`)) {
		t.Fatalf("本机 MVP 节点列表状态=%d，响应=%s", listResponse.Code, listResponse.Body.String())
	}
}

func TestLocalMVPConnectionTestFailsClosedWithoutAgent(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "local-mvp.db"))
	if err != nil {
		t.Fatalf("打开本机 MVP 数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := PrepareStore(context.Background(), database); err != nil {
		t.Fatalf("PrepareStore() 失败: %v", err)
	}
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("创建测试密钥环失败: %v", err)
	}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, mustDependencies(t, database, keyring))
	dataSourceCreate := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources", bytes.NewBufferString(`{"displayName":"Synthetic Source","environment":"TEST","connectionKind":"ODP","compatibilityMode":"MYSQL","host":"192.0.2.10","port":2881,"clusterName":"synthetic-cluster","tenantName":"synthetic-tenant","username":"synthetic-user","password":"synthetic-password"}`))
	dataSourceCreate.RemoteAddr = "127.0.0.1:12000"
	dataSourceCreate.Header.Set("Idempotency-Key", "synthetic-local-mvp-source-create")
	dataSourceCreate.Header.Set("X-CSRF-Token", CSRFToken)
	dataSourceResponse := httptest.NewRecorder()
	handler.ServeHTTP(dataSourceResponse, dataSourceCreate)
	if dataSourceResponse.Code != http.StatusCreated {
		t.Fatalf("本机 MVP 创建数据源状态=%d，响应=%s", dataSourceResponse.Code, dataSourceResponse.Body.String())
	}
	var createdDataSource struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(dataSourceResponse.Body).Decode(&createdDataSource); err != nil || createdDataSource.ID == "" {
		t.Fatalf("本机 MVP 数据源创建响应无效: %#v, %v", createdDataSource, err)
	}

	nodeCreate := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes", bytes.NewBufferString(`{"displayName":"Unassociated Windows Node","platform":"WINDOWS_AMD64","toolHome":"E:\\tools\\ob-loader-dumper","javaPath":"C:\\Java\\jdk8\\bin\\java.exe","allowedRoots":["E:\\ob-data\\exports"]}`))
	nodeCreate.RemoteAddr = "127.0.0.1:12000"
	nodeCreate.Header.Set("Idempotency-Key", "synthetic-local-mvp-node-create")
	nodeCreate.Header.Set("X-CSRF-Token", CSRFToken)
	nodeResponse := httptest.NewRecorder()
	handler.ServeHTTP(nodeResponse, nodeCreate)
	if nodeResponse.Code != http.StatusCreated {
		t.Fatalf("本机 MVP 创建节点状态=%d，响应=%s", nodeResponse.Code, nodeResponse.Body.String())
	}
	var createdNode struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(nodeResponse.Body).Decode(&createdNode); err != nil || createdNode.ID == "" {
		t.Fatalf("本机 MVP 节点创建响应无效: %#v, %v", createdNode, err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/"+createdDataSource.ID+":test-connection", bytes.NewBufferString(`{"nodeId":"`+createdNode.ID+`"}`))
	request.RemoteAddr = "127.0.0.1:12000"
	request.Header.Set("X-CSRF-Token", CSRFToken)
	request.Header.Set("Idempotency-Key", "synthetic-local-mvp-connection-test")
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity || !bytes.Contains(response.Body.Bytes(), []byte("AGENT_CONNECTION_TEST_NODE_UNAVAILABLE")) || bytes.Contains(response.Body.Bytes(), []byte("SUCCEEDED")) {
		t.Fatalf("本机 MVP 无 Agent 连接测试响应=%d，响应=%s", response.Code, response.Body.String())
	}
}

// TestDependenciesConfigureExportDraft 验证本机 MVP 的草稿入口不会因装配遗漏而退化为 503。
func TestDependenciesConfigureExportDraft(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "local-mvp.db"))
	if err != nil {
		t.Fatalf("打开本机 MVP 数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("创建测试密钥环失败: %v", err)
	}

	dependencies := mustDependencies(t, database, keyring)
	if dependencies.Drafts == nil || dependencies.Nodes == nil || dependencies.Generator == nil || dependencies.Prechecks == nil || dependencies.AgentPrechecks == nil || dependencies.PrecheckSecrets == nil || dependencies.PrecheckTTL <= 0 {
		t.Fatal("本机 MVP 未完整装配导出草稿和预检查依赖")
	}
	if _, err := dependencies.Nodes.GetExecutionNodeFact(context.Background(), "missing-node"); err == nil {
		t.Fatal("缺失节点不应被适配为导出草稿节点事实")
	}
}

// TestLocalMVPExportDraftPassesDependencyBoundary 验证草稿请求会进入业务校验，不因本机装配遗漏返回 503。
func TestLocalMVPExportDraftPassesDependencyBoundary(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "local-mvp.db"))
	if err != nil {
		t.Fatalf("打开本机 MVP 数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := PrepareStore(context.Background(), database); err != nil {
		t.Fatalf("PrepareStore() 失败: %v", err)
	}
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("创建测试密钥环失败: %v", err)
	}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, mustDependencies(t, database, keyring))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(`{"dataSourceId":"missing-source","nodeId":"missing-node","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"E:\\tmp\\ob-data-orch-export"}`))
	request.RemoteAddr = "127.0.0.1:12000"
	request.Header.Set("Idempotency-Key", "local-mvp-export-draft-dependency")
	request.Header.Set("X-CSRF-Token", CSRFToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || bytes.Contains(response.Body.Bytes(), []byte("API_DEPENDENCY_NOT_CONFIGURED")) {
		t.Fatalf("本机 MVP 草稿依赖边界响应=%d，响应=%s", response.Code, response.Body.String())
	}
}

// TestDependenciesConfigureStorageCredentials 验证本机 MVP 已装配对象存储凭据管理（EX-I6 存储凭据槽位），
// 避免 EX-V1 存储验证链路因装配遗漏退化为 503 API_DEPENDENCY_NOT_CONFIGURED。
func TestDependenciesConfigureStorageCredentials(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "local-mvp.db"))
	if err != nil {
		t.Fatalf("打开本机 MVP 数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := PrepareStore(context.Background(), database); err != nil {
		t.Fatalf("PrepareStore() 失败: %v", err)
	}
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatalf("创建测试密钥环失败: %v", err)
	}

	dependencies := mustDependencies(t, database, keyring)
	if dependencies.StorageCredentials == nil {
		t.Fatal("本机 MVP 未装配对象存储凭据管理")
	}
	handler := controlplane.NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, dependencies)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/storage-credentials", nil)
	request.RemoteAddr = "127.0.0.1:12000"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte("API_DEPENDENCY_NOT_CONFIGURED")) {
		t.Fatalf("本机 MVP 存储凭据列表响应=%d，响应=%s", response.Code, response.Body.String())
	}
}

// mustDependencies 统一将本机 MVP 启动依赖的初始化错误转换为测试失败。
func mustDependencies(t *testing.T, database *store.Store, keyring *credential.Keyring) controlplane.Dependencies {
	t.Helper()
	dependencies, err := Dependencies(database, keyring, false, false)
	if err != nil {
		t.Fatalf("创建本机 MVP 依赖失败: %v", err)
	}
	return dependencies
}
