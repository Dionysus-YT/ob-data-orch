package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/store"
)

// TestCsvNoneRequiresExplicitEscape 验证 NONE 模式在进入工具前满足官方序列化依赖。
func TestCsvNoneRequiresExplicitEscape(t *testing.T) {
	if validateCsvOptions(store.CsvOptions{ColumnQuoteMode: "none"}) == nil {
		t.Fatal("缺少转义字符的 NONE 模式不得进入真实工具")
	}
	if err := validateCsvOptions(store.CsvOptions{ColumnQuoteMode: "none", EscapeCharacter: "\\"}); err != nil {
		t.Fatal("合法 NONE 模式应接受显式单字节转义字符")
	}
}

// TestPendingPrecheckProjectionHasCompleteEvidence 验证等待报告也具有完整固定项与非空证据码。
func TestPendingPrecheckProjectionHasCompleteEvidence(t *testing.T) {
	results := browserPrecheckResults(nil)
	checks := agentpreflight.FixedChecks()
	if len(results) != len(checks) {
		t.Fatal("等待中的预检查缺少固定项")
	}
	for index, result := range results {
		if result.Check != string(checks[index]) || result.Status != "UNKNOWN" || result.EvidenceCode == "" {
			t.Fatal("等待中的预检查投影不符合浏览器契约")
		}
	}
}

// TestSequenceOnlyPolicyDraft 验证指定序列的策略资格不依赖整库或表对象。
func TestSequenceOnlyPolicyDraft(t *testing.T) {
	body := []byte(`{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["SEQUENCE"],"expressions":[{"name":"synthetic_seq","objectType":"SEQUENCE"}]},"contentSelection":{"contentKind":"DDL_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/output"},"ddlBehavior":{"sequencePolicy":"PRESERVE"}}}`)
	var request exportDraftWriteRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeExportDraftRequest(request); err != nil {
		t.Fatal(err)
	}
	drafts, prechecks, tasks := &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{}
	handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, prechecks, tasks, nil, mustStaticDataSourceReader())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "synthetic-csrf")
	req.Header.Set("Idempotency-Key", "sequence-only-policy")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, req)
	if created.Code != http.StatusCreated {
		t.Fatalf("序列策略草稿未创建: %d", created.Code)
	}
}

// TestDuplicateColumnSelectionRejected 验证重复列在创建草稿时阻断且不会写入任务配置。
func TestDuplicateColumnSelectionRejected(t *testing.T) {
	for _, field := range []string{"includeColumnNames", "excludeColumnNames"} {
		drafts, prechecks, tasks := &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{}
		handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, prechecks, tasks, nil, mustStaticDataSourceReader())
		body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/output"},"filterConfig":{"` + field + `":["id","id"]}}}`
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", "synthetic-csrf")
		request.Header.Set("Idempotency-Key", "duplicate-"+field)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("重复列未在草稿入口拒绝: %d", response.Code)
		}
	}
}
