package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ob-data-orch/internal/exportobject"
)

func TestOracle十类对象草稿预览预检查与冻结快照(t *testing.T) {
	for _, content := range []string{"DDL_ONLY", "DDL_AND_DATA"} {
		t.Run(content, func(t *testing.T) {
			drafts, checks, tasks := &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{}
			sources := mustStaticDataSourceReader()
			sources.summaries[0].CompatibilityMode = "ORACLE"
			handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, checks, tasks, nil, sources)
			body := oracleObjectDraftBody(t, content)
			preview, snapshot := runV6CapabilityFlow(t, handler, drafts, checks, tasks, body, "oracle-objects-"+content)
			if drafts.created.MetadataVersion != exportMetadataOracleObjects || !strings.Contains(snapshot, exportMetadataOracleObjects) {
				t.Fatal("新对象未冻结到独立参数版本")
			}
			for _, option := range []string{"--package", "--trigger", "--synonym", "--public-synonym", "--type"} {
				if !strings.Contains(preview, option) { t.Fatalf("预览缺少 %s", option) }
			}
			if tasks.input.SnapshotJSON != drafts.created.ConfigJSON { t.Fatal("提交快照丢失对象归属") }
		})
	}
}

func TestOracle新对象在MySQL和纯数据请求失败关闭(t *testing.T) {
	for _, mode := range []string{"MYSQL", "ORACLE"} {
		content := "DDL_ONLY"
		if mode == "ORACLE" { content = "DATA_ONLY" }
		drafts := &recordingDraftStore{}
		sources := mustStaticDataSourceReader()
		sources.summaries[0].CompatibilityMode = mode
		handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, &recordingPrecheckStore{}, &recordingTaskStore{}, nil, sources)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(oracleObjectDraftBody(t, content)))
		request.Header.Set("Idempotency-Key", "synthetic-oracle-boundary-"+mode)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity || drafts.created.DraftID != "" { t.Fatalf("非法组合未拒绝: %s / %d", mode, response.Code) }
	}
}

func oracleObjectDraftBody(t *testing.T, content string) string {
	t.Helper()
	var expressions []map[string]string
	for _, kind := range exportobject.Types() { expressions = append(expressions, map[string]string{"objectType": kind, "name": "synthetic_object"}) }
	body, err := json.Marshal(map[string]any{
		"configVersion": "v6", "dataSourceId": "source-allowed", "nodeId": "node-1",
		"config": map[string]any{
			"objectScope": map[string]any{"database": "synthetic_schema", "scopeKind": "SPECIFIED", "objectTypes": exportobject.Types(), "expressions": expressions},
			"contentSelection": map[string]string{"contentKind": content},
			"dataFormat": map[string]string{"formatKind": "CSV"},
			"outputConfig": map[string]string{"outputKind": "LOCAL", "filePath": "/E:/tmp/synthetic-output"},
		},
	})
	if err != nil { t.Fatal(err) }
	return string(body)
}
