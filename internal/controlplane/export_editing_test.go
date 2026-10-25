package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/store"
)

// 编辑测试的持久化替身保留修订与引用规则，不启动真实节点或读取秘密。
type editingDraftStore struct{ recordingDraftStore }

func (s *editingDraftStore) UpdateDraft(ctx context.Context, input store.DraftUpdate) (int64, error) {
	if input.ExpectedRevision != s.created.Revision {
		return 0, store.ErrRevisionConflict
	}
	if input.RebindEditing {
		s.created.DataSourceID, s.created.NodeID = input.DataSourceID, input.NodeID
	}
	return s.recordingDraftStore.UpdateDraft(ctx, input)
}

func editingBody() map[string]any {
	return map[string]any{"configVersion": "v6", "draftState": "EDITING", "dataSourceId": "source-allowed", "nodeId": "", "config": map[string]any{
		"objectScope":      map[string]any{"scopeKind": "SPECIFIED", "database": ""},
		"contentSelection": map[string]any{"contentKind": ""},
		"dataFormat":       map[string]any{"formatKind": "CSV"},
		"outputConfig":     map[string]any{"outputKind": "LOCAL", "filePath": ""},
	}}
}

func editingRequest(t *testing.T, handler http.Handler, method, path string, body any, revision string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Idempotency-Key", "synthetic-editing-draft-request")
	if revision != "" {
		request.Header.Set("If-Match", revision)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func Test未完成导出草稿不依赖生成器且可补齐为同一草稿(t *testing.T) {
	drafts := &editingDraftStore{}
	dependencies := Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{}, Drafts: drafts, CSRF: allowedCSRF{}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, dependencies)
	body := editingBody()
	created := editingRequest(t, handler, http.MethodPost, "/api/v1/export-drafts", body, "")
	if created.Code != http.StatusCreated || drafts.created.NodeID != "" || drafts.created.CapabilityVersion != store.ExportEditingCapabilityVersion {
		t.Fatalf("未完成草稿保存失败: status=%d", created.Code)
	}
	read := editingRequest(t, handler, http.MethodGet, "/api/v1/export-drafts/draft-synthetic", nil, "")
	if read.Code != http.StatusOK || !bytes.Contains(read.Body.Bytes(), []byte(`"draftState":"EDITING"`)) {
		t.Fatal("未恢复编辑状态")
	}
	dependencies.Nodes, dependencies.CredentialRefs = staticNodeReader{}, staticCredentialReferenceReader{}
	dependencies.Generator, _ = commandgen.NewDefault()
	dependencies.Prechecks, dependencies.Tasks = &recordingPrecheckStore{}, &recordingTaskStore{}
	dependencies.Coordinator, _ = agentstate.NewCoordinator(testClock{})
	dependencies.PrecheckTTL = time.Minute
	handler = NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, dependencies)
	for _, action := range []string{"preview-command", "precheck", "submit"} {
		blocked := editingRequest(t, handler, http.MethodPost, "/api/v1/export-drafts/draft-synthetic:"+action, map[string]any{"precheckId": "synthetic-precheck"}, `"rev-1"`)
		if blocked.Code != http.StatusUnprocessableEntity || !bytes.Contains(blocked.Body.Bytes(), []byte("EXPORT_DRAFT_INCOMPLETE")) {
			t.Fatalf("编辑草稿进入执行准备: %s", action)
		}
	}
	stale := editingRequest(t, handler, http.MethodPatch, "/api/v1/export-drafts/draft-synthetic", body, `"rev-2"`)
	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("修订冲突未阻断: %d", stale.Code)
	}
	config := body["config"].(map[string]any)
	config["objectScope"] = map[string]any{"scopeKind": "SPECIFIED", "database": "synthetic_db", "objectTypes": []string{"TABLE"}, "expressions": []any{map[string]any{"objectType": "TABLE", "name": "synthetic_table"}}}
	config["outputConfig"] = map[string]any{"outputKind": "LOCAL", "filePath": "/E:/synthetic-output"}
	body["nodeId"], body["draftState"] = "node-1", "READY"
	dependencies.Nodes, dependencies.CredentialRefs = staticNodeReader{}, staticCredentialReferenceReader{}
	dependencies.Generator, _ = commandgen.NewDefault()
	handler = NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, dependencies)
	withoutContent := editingRequest(t, handler, http.MethodPatch, "/api/v1/export-drafts/draft-synthetic", body, `"rev-1"`)
	if withoutContent.Code != http.StatusUnprocessableEntity || drafts.created.Revision != 1 {
		t.Fatal("未选择内容仍被提升为完整配置")
	}
	config["contentSelection"] = map[string]any{"contentKind": "DATA_ONLY"}
	updated := editingRequest(t, handler, http.MethodPatch, "/api/v1/export-drafts/draft-synthetic", body, `"rev-1"`)
	if updated.Code != http.StatusOK || drafts.created.Revision != 2 || drafts.created.NodeID != "node-1" || drafts.created.CapabilityVersion == store.ExportEditingCapabilityVersion {
		t.Fatalf("补齐草稿未升级: status=%d body=%s", updated.Code, updated.Body.String())
	}
	preview := editingRequest(t, handler, http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil, `"rev-2"`)
	if preview.Code != http.StatusOK {
		t.Fatalf("完整草稿无法预览: %d", preview.Code)
	}
}

func Test未完成导出草稿字段权限与安全校验(t *testing.T) {
	for _, scenario := range []string{"未知状态", "旧结构", "无权数据源", "无权节点", "未知秘密字段", "重试参数", "未开放格式", "失效CSRF", "外部主体"} {
		t.Run(scenario, func(t *testing.T) {
			drafts := &editingDraftStore{}
			deps := Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{}, Drafts: drafts, CSRF: allowedCSRF{}}
			body := editingBody()
			expected := http.StatusBadRequest
			method, path, revision := http.MethodPost, "/api/v1/export-drafts", ""
			switch scenario {
			case "未知状态":
				body["draftState"] = "RUNNING"
				expected = http.StatusNotFound
			case "旧结构":
				body["configVersion"] = "v5"
			case "无权数据源":
				body["dataSourceId"], expected = "source-denied", http.StatusNotFound
			case "无权节点":
				body["nodeId"], expected = "node-denied", http.StatusNotFound
			case "未知秘密字段":
				body["config"].(map[string]any)["password"] = "synthetic-secret"
			case "重试参数":
				body["config"].(map[string]any)["performanceConfig"], expected = map[string]any{"retry": true}, http.StatusUnprocessableEntity
			case "未开放格式":
				body["config"].(map[string]any)["dataFormat"], expected = map[string]any{"formatKind": "PAR"}, http.StatusUnprocessableEntity
			case "失效CSRF":
				deps.CSRF, expected = nil, http.StatusServiceUnavailable
			case "外部主体":
				drafts.created = store.ExportDraft{DraftID: "draft-synthetic", OwnerSubjectID: "foreign"}
				method, path, revision, expected = http.MethodPatch, "/api/v1/export-drafts/draft-synthetic", `"rev-1"`, http.StatusNotFound
			}
			response := editingRequest(t, NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, deps), method, path, body, revision)
			if response.Code != expected {
				t.Fatalf("未失败关闭: got=%d want=%d", response.Code, expected)
			}
			if drafts.created.ConfigJSON != "" {
				t.Fatal("拒绝请求仍保存了输入")
			}
		})
	}
}
