package controlplane

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/store"
)

func Test永久删除动作入口保留安全边界(t *testing.T) {
	for _, fixture := range []struct {
		name, id, revision string
		csrf               CSRFValidator
		want               int
		length             int64
	}{{"success", "node-allowed", `"rev-1"`, allowedCSRF{}, http.StatusOK, 0}, {"unauthorized", "node-denied", `"rev-1"`, allowedCSRF{}, http.StatusNotFound, 0}, {"missing revision", "node-allowed", "", allowedCSRF{}, http.StatusBadRequest, 0}, {"csrf rejected", "node-allowed", `"rev-1"`, deniedCSRF{}, http.StatusUnauthorized, 0}, {"unexpected body", "node-allowed", `"rev-1"`, allowedCSRF{}, http.StatusBadRequest, 2}, {"chunked body", "node-allowed", `"rev-1"`, allowedCSRF{}, http.StatusBadRequest, -1}} {
		t.Run(fixture.name, func(t *testing.T) {
			deleter := &recordingExecutionNodeDeleter{result: store.ExecutionNodeDeletionResult{Outcome: "DELETED", AgentAccessRevoked: true}}
			handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-allowed"}, CSRF: fixture.csrf, NodeDeleter: deleter})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/execution-nodes/"+fixture.id+":delete", nil)
			request.Header.Set("If-Match", fixture.revision)
			request.ContentLength = fixture.length
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != fixture.want {
				t.Fatalf("永久删除响应=%d want=%d body=%s", response.Code, fixture.want, response.Body.String())
			}
			if fixture.want != http.StatusOK && deleter.input.NodeID != "" {
				t.Fatal("安全校验失败仍调用写事务")
			}
		})
	}
}

func Test永久删除不得返回自动归档成功(t *testing.T) {
	deleter := &recordingExecutionNodeDeleter{result: store.ExecutionNodeDeletionResult{Outcome: "ARCHIVED", Revision: 2, AgentAccessRevoked: true}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-allowed"}, CSRF: allowedCSRF{}, NodeDeleter: deleter})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/execution-nodes/node-allowed", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || bytes.Contains(response.Body.Bytes(), []byte(`"outcome":"ARCHIVED"`)) {
		t.Fatalf("归档被标为永久删除成功：%d %s", response.Code, response.Body.String())
	}
}
