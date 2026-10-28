package controlplane

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test结构策略冲突在草稿拒绝且显式False不进入纯数据(t *testing.T) {
	for _, test := range []struct{ content, options string }{
		{"DDL_ONLY", `"retainSchema":true,"compactSchema":true`},
		{"DATA_ONLY", `"compactSchema":false`},
		{"DDL_ONLY", `"compactSchema":false`},
	} {
		drafts := &recordingDraftStore{}
		handler := newGeneralizedFlowHandler(t, drafts, &recordingPrecheckStore{}, &recordingTaskStore{})
		body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"table_one"}]},"contentSelection":{"contentKind":"` + test.content + `"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/synthetic-output"},"ddlBehavior":{` + test.options + `}}}`
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		request.Header.Set("Idempotency-Key", "synthetic-schema-conflict-key")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" {
			t.Fatalf("冲突未在保存前拒绝: %d", response.Code)
		}
	}
}

func Test旧结构草稿只读重放不追加策略写入新修订才升级(t *testing.T) {
	drafts := &recordingDraftStore{}
	handler := newGeneralizedFlowHandler(t, drafts, &recordingPrecheckStore{}, &recordingTaskStore{})
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"table_one"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/synthetic-output"},"ddlBehavior":{"retainSchema":true}}}`
	created := httptest.NewRecorder()
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-schema-upgrade-key")
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatal("合成结构草稿未创建")
	}
	// 仅合成存储夹具切到历史版本，以核对既有冻结版本的只读重放；真实数据库不改写。
	drafts.created.MetadataVersion = exportMetadataV7
	preview := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	handler.ServeHTTP(preview, request)
	if preview.Code != http.StatusOK || strings.Contains(preview.Body.String(), "compact-schema=") {
		t.Fatal("历史预览被静默补参")
	}
	update := httptest.NewRequest(http.MethodPatch, "/api/v1/export-drafts/draft-synthetic", bytes.NewBufferString(body))
	update.Header.Set("If-Match", `"rev-1"`)
	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, update)
	if updated.Code != http.StatusOK || drafts.created.MetadataVersion != exportMetadataSchemaBoolean || drafts.created.Revision != 2 {
		t.Fatal("新修订未按固定策略升级")
	}
}
