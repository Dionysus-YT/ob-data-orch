package controlplane

import (
	"encoding/json"
	"net/http"

	"ob-data-orch/internal/buildinfo"
)

type Server struct {
	build buildinfo.Info
}

func NewHandler(build buildinfo.Info) http.Handler {
	server := &Server{build: build}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /version", server.version)
	return securityHeaders(mux)
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
