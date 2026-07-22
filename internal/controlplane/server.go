package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

type Server struct {
	build        buildinfo.Info
	provider     identity.Provider
	authorizer   identity.Authorizer
	roles        identity.RoleAuthorizer
	dataSource   DataSourceReader
	creator      DataSourceCreator
	stateChanger DataSourceStateChanger
	encryptor    CredentialEncryptor
	csrf         CSRFValidator
	keyID        string
}

// DataSourceReader exposes only a non-sensitive data-source projection. The
// write API is deferred until credential encryption and audit are transactional.
type DataSourceReader interface {
	ListDataSourceSummaries(context.Context) ([]store.DataSourceSummary, error)
	GetDataSourceSummary(context.Context, string) (store.DataSourceSummary, error)
}

// DataSourceCreator is the transaction boundary that stores a source together
// with its encrypted credential, audit event, and idempotency record.
type DataSourceCreator interface {
	CreateDataSource(context.Context, store.DataSourceCreate) (store.DataSourceCreateResult, error)
}

// DataSourceStateChanger 将启停动作收敛为独立的原子存储边界。
// HTTP 层不能自行更新状态或绕过审计。
type DataSourceStateChanger interface {
	ChangeDataSourceState(context.Context, store.DataSourceStateChange) (store.DataSourceStateChangeResult, error)
}

// CredentialEncryptor keeps raw root-key material out of the HTTP package.
type CredentialEncryptor interface {
	Encrypt(string, credential.Reference, []byte) (credential.Envelope, error)
}

// CSRFValidator is intentionally separate from authentication because a valid
// browser session alone must not authorize a state-changing request.
type CSRFValidator interface {
	ValidateCSRF(*http.Request) error
}

// Dependencies make the HTTP boundary testable without creating a runtime
// authentication bypass. Nil production dependencies continue to fail closed.
type Dependencies struct {
	Identity        identity.Provider
	Authorizer      identity.Authorizer
	Roles           identity.RoleAuthorizer
	DataSources     DataSourceReader
	Creator         DataSourceCreator
	StateChanger    DataSourceStateChanger
	Encryptor       CredentialEncryptor
	CSRF            CSRFValidator
	CredentialKeyID string
}

func NewHandler(build buildinfo.Info) http.Handler {
	return NewHandlerWithDependencies(build, Dependencies{})
}

// NewHandlerWithIdentity exists for integration tests and future deployment
// adapters. Passing nil is the production default until a real identity source
// is selected and configured.
func NewHandlerWithIdentity(build buildinfo.Info, provider identity.Provider) http.Handler {
	return NewHandlerWithDependencies(build, Dependencies{Identity: provider})
}

// NewHandlerWithDependencies is used by the composition root and integration
// tests. Supplying an identity provider alone does not grant access to data.
func NewHandlerWithDependencies(build buildinfo.Info, dependencies Dependencies) http.Handler {
	server := &Server{build: build, provider: dependencies.Identity, authorizer: dependencies.Authorizer, roles: dependencies.Roles, dataSource: dependencies.DataSources, creator: dependencies.Creator, stateChanger: dependencies.StateChanger, encryptor: dependencies.Encryptor, csrf: dependencies.CSRF, keyID: dependencies.CredentialKeyID}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /version", server.version)
	// DEV-04 starts with executable API-domain boundaries. No request may be
	// treated as authenticated until the deployment identity contract exists.
	if server.provider == nil {
		mux.HandleFunc("/api/", server.browserUnavailable)
		mux.HandleFunc("/agent/", server.agentUnavailable)
	} else {
		mux.HandleFunc("/api/", server.browserAuthenticated)
		mux.HandleFunc("/agent/", server.agentAuthenticated)
	}
	mux.HandleFunc("/", notFound)
	return securityHeaders(mux)
}

func (s *Server) browserAuthenticated(w http.ResponseWriter, r *http.Request) {
	principal, err := s.provider.AuthenticateBrowser(r)
	if err != nil || identity.Validate(principal, identity.BrowserPrincipal) != nil {
		writeError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "身份认证失败", false)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/data-sources" {
		s.listDataSources(w, r, principal)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/data-sources" {
		s.createDataSource(w, r, principal)
		return
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		if dataSourceID, targetState, ok := parseDataSourceStateAction(r.URL.Path); ok {
			s.changeDataSourceState(w, r, principal, dataSourceID, targetState)
			return
		}
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		dataSourceID := strings.TrimPrefix(r.URL.Path, "/api/v1/data-sources/")
		if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
			s.getDataSource(w, r, principal, dataSourceID)
			return
		}
	}
	notFound(w, r)
}

// parseDataSourceStateAction 只识别已登记的启停动作，避免把路径后缀解释成通用命令。
func parseDataSourceStateAction(path string) (string, string, bool) {
	const prefix = "/api/v1/data-sources/"
	for suffix, targetState := range map[string]string{":disable": "DISABLED", ":enable": "ENABLED"} {
		if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) {
			dataSourceID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
			if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
				return dataSourceID, targetState, true
			}
		}
	}
	return "", "", false
}

type dataSourceCreateRequest struct {
	DisplayName       string `json:"displayName"`
	Environment       string `json:"environment"`
	ConnectionKind    string `json:"connectionKind"`
	CompatibilityMode string `json:"compatibilityMode"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Username          string `json:"username"`
	DefaultDatabase   string `json:"defaultDatabase"`
	Password          string `json:"password"`
}

// createDataSource never renders or persists the password itself. Its
// idempotency digest deliberately records only that a password was supplied,
// rather than retaining a password-derived hash.
func (s *Server) createDataSource(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.creator == nil || s.encryptor == nil || s.csrf == nil || s.roles == nil || strings.TrimSpace(s.keyID) == "" {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源创建依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if identity.HasRole(r.Context(), s.roles, principal, identity.RoleDataSourceAdmin) != nil {
		writeError(w, http.StatusForbidden, "ROLE_REQUIRED", "当前身份不具备数据源管理能力", false)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	var request dataSourceCreateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	password := []byte(request.Password)
	request.Password = ""
	defer credential.Zero(password)
	if len(password) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "PASSWORD_REQUIRED", "密码不能为空", false)
		return
	}
	dataSourceID, credentialID, err := newOpaqueID(), newOpaqueID(), error(nil)
	if dataSourceID == "" || credentialID == "" {
		err = errors.New("generate identifier")
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	envelope, err := s.encryptor.Encrypt(s.keyID, credential.Reference{CredentialID: credentialID, Revision: 1, SecretType: credential.DatabasePassword, DataSourceID: dataSourceID}, password)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_ENCRYPTION_UNAVAILABLE", "凭据安全上下文不可用", true)
		return
	}
	defer credential.Zero(envelope.Nonce)
	defer credential.Zero(envelope.Ciphertext)
	result, err := s.creator.CreateDataSource(r.Context(), store.DataSourceCreate{
		DataSourceID: dataSourceID, CredentialID: credentialID, CreatorSubjectID: principal.ID,
		DisplayName: request.DisplayName, NormalizedName: normalizeName(request.DisplayName),
		Environment: request.Environment, ConnectionKind: request.ConnectionKind, CompatibilityMode: request.CompatibilityMode,
		Host: request.Host, Port: request.Port, Username: request.Username, DefaultDatabase: request.DefaultDatabase,
		KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext,
		RequestID: requestID(), IdempotencyKey: idempotencyKey, RequestDigest: createRequestDigest(request), CreatedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "DATA_SOURCE_CREATE_REJECTED", "数据源字段不符合要求", false)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID(), "id": result.DataSourceID, "replayed": result.Replayed})
}

// changeDataSourceState 执行幂等的启停动作。对象授权先于存储读取，
// 因此无权主体无法通过状态接口枚举数据源是否存在。
func (s *Server) changeDataSourceState(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID, targetState string) {
	if s.stateChanger == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源状态依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceWrite, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	result, err := s.stateChanger.ChangeDataSourceState(r.Context(), store.DataSourceStateChange{
		DataSourceID: dataSourceID, ActorSubjectID: principal.ID, TargetState: targetState,
		RequestID: requestID(), ChangedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "DATA_SOURCE_STATE_CONFLICT", "数据源状态已发生变化，请刷新后重试", true)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_STATE_UNAVAILABLE", "数据源状态暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(), "id": dataSourceID, "state": result.State,
		"revision": result.Revision, "replayed": result.Replayed,
	})
}

func normalizeName(displayName string) string { return strings.ToLower(strings.TrimSpace(displayName)) }

func createRequestDigest(request dataSourceCreateRequest) string {
	// Password content is not incorporated; retaining a password hash would be
	// a new sensitive persistence surface. Presence still distinguishes omission.
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%s|%s|password=true", request.DisplayName, request.Environment, request.ConnectionKind, request.CompatibilityMode, request.Host, request.Port, request.Username, request.DefaultDatabase)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func newOpaqueID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

// getDataSource authorizes the requested ID before reading it, then maps both
// absence and an out-of-scope object to the same 404 response.
func (s *Server) getDataSource(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID string) {
	if s.dataSource == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源 API 依赖", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	summary, err := s.dataSource.GetDataSourceSummary(r.Context(), dataSourceID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_QUERY_UNAVAILABLE", "数据源暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "item": newDataSourceResponse(summary)})
}

func (s *Server) agentAuthenticated(w http.ResponseWriter, r *http.Request) {
	principal, err := s.provider.AuthenticateAgent(r)
	if err != nil || identity.Validate(principal, identity.AgentPrincipal) != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return
	}
	notFound(w, r)
}

// listDataSources filters each object before it reaches JSON. It never returns
// a total count, so unauthorized sources are not discoverable through the API.
func (s *Server) listDataSources(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.dataSource == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源 API 依赖", false)
		return
	}
	summaries, err := s.dataSource.ListDataSourceSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_QUERY_UNAVAILABLE", "数据源暂时不可用", true)
		return
	}
	items := make([]dataSourceResponse, 0, len(summaries))
	for _, summary := range summaries {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, summary.DataSourceID) != nil {
			continue
		}
		items = append(items, newDataSourceResponse(summary))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "items": items})
}

// dataSourceResponse keeps JSON camelCase and cannot inherit credential-only
// fields if the persistence model later grows additional columns.
type dataSourceResponse struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"displayName"`
	Environment        string `json:"environment"`
	ConnectionKind     string `json:"connectionKind"`
	CompatibilityMode  string `json:"compatibilityMode"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	DefaultDatabase    string `json:"defaultDatabase,omitempty"`
	State              string `json:"state"`
	Revision           int64  `json:"revision"`
	CredentialRevision int64  `json:"credentialRevision"`
	LastTestStatus     string `json:"lastTestStatus,omitempty"`
}

func newDataSourceResponse(summary store.DataSourceSummary) dataSourceResponse {
	return dataSourceResponse{
		ID: summary.DataSourceID, DisplayName: summary.DisplayName, Environment: summary.Environment,
		ConnectionKind: summary.ConnectionKind, CompatibilityMode: summary.CompatibilityMode,
		Host: summary.Host, Port: summary.Port, Username: summary.Username,
		DefaultDatabase: summary.DefaultDatabase, State: summary.State, Revision: summary.Revision,
		CredentialRevision: summary.CredentialRevision, LastTestStatus: summary.LastTestStatus,
	}
}

func (s *Server) browserUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusServiceUnavailable, "AUTHENTICATION_NOT_CONFIGURED", "当前环境尚未配置浏览器身份认证", false)
}

func (s *Server) agentUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusServiceUnavailable, "AGENT_AUTHENTICATION_NOT_CONFIGURED", "当前环境尚未配置 Agent 机器认证", false)
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "请求的资源不存在", false)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "ok",
		"stage":                "G1",
		"realExecutionEnabled": false,
	})
}

func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "ready",
		"scope":                "process-only",
		"stage":                "G1",
		"realExecutionEnabled": false,
	})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.build)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	writeJSON(w, status, map[string]any{
		"requestId":   requestID(),
		"code":        code,
		"message":     message,
		"retryable":   retryable,
		"fieldErrors": []any{},
		"safeDetails": map[string]any{},
	})
}

func requestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(bytes)
}
