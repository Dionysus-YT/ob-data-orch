package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

type Server struct {
	build      buildinfo.Info
	provider   identity.Provider
	authorizer identity.Authorizer
	dataSource DataSourceReader
}

// DataSourceReader exposes only a non-sensitive data-source projection. The
// write API is deferred until credential encryption and audit are transactional.
type DataSourceReader interface {
	ListDataSourceSummaries(context.Context) ([]store.DataSourceSummary, error)
	GetDataSourceSummary(context.Context, string) (store.DataSourceSummary, error)
}

// Dependencies make the HTTP boundary testable without creating a runtime
// authentication bypass. Nil production dependencies continue to fail closed.
type Dependencies struct {
	Identity    identity.Provider
	Authorizer  identity.Authorizer
	DataSources DataSourceReader
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
	server := &Server{build: build, provider: dependencies.Identity, authorizer: dependencies.Authorizer, dataSource: dependencies.DataSources}
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
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		dataSourceID := strings.TrimPrefix(r.URL.Path, "/api/v1/data-sources/")
		if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
			s.getDataSource(w, r, principal, dataSourceID)
			return
		}
	}
	notFound(w, r)
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
