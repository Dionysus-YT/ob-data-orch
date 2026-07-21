package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
)

type Server struct {
	build buildinfo.Info
}

func NewHandler(build buildinfo.Info) http.Handler {
	return NewHandlerWithIdentity(build, nil)
}

// NewHandlerWithIdentity exists for integration tests and future deployment
// adapters. Passing nil is the production default until a real identity source
// is selected and configured.
func NewHandlerWithIdentity(build buildinfo.Info, provider identity.Provider) http.Handler {
	server := &Server{build: build}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /version", server.version)
	// DEV-04 starts with executable API-domain boundaries. No request may be
	// treated as authenticated until the deployment identity contract exists.
	if provider == nil {
		mux.HandleFunc("/api/", server.browserUnavailable)
		mux.HandleFunc("/agent/", server.agentUnavailable)
	} else {
		mux.HandleFunc("/api/", server.browserAuthenticated(provider))
		mux.HandleFunc("/agent/", server.agentAuthenticated(provider))
	}
	mux.HandleFunc("/", notFound)
	return securityHeaders(mux)
}

func (s *Server) browserAuthenticated(provider identity.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := provider.AuthenticateBrowser(r)
		if err != nil || identity.Validate(principal, identity.BrowserPrincipal) != nil {
			writeError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "身份认证失败", false)
			return
		}
		notFound(w, r)
	}
}

func (s *Server) agentAuthenticated(provider identity.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := provider.AuthenticateAgent(r)
		if err != nil || identity.Validate(principal, identity.AgentPrincipal) != nil {
			writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
			return
		}
		notFound(w, r)
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
