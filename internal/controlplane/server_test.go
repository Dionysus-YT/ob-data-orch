package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ob-data-orch/internal/buildinfo"
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
