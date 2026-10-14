package controlplane

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ob-data-orch/internal/store"
)

func TestExpertExportFlowFreezesSysWithoutBrowserDisclosure(t *testing.T) {
	drafts, prechecks, tasks := &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{}
	sources := mustStaticDataSourceReader()
	sources.summaries[0].SysUser = "synthetic_private_sys"
	sources.summaries[0].SysCredentialID = "33333333-3333-4333-8333-333333333333"
	sources.summaries[0].SysCredentialRevision = 2
	handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, prechecks, tasks, nil, sources)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL"},"contentSelection":{"contentKind":"DDL_AND_DATA"},"dataFormat":{"formatKind":"CSV","timestampFormats":{"timeValueFormat":"HH:mm:ss","timestampValueFormat":"yyyy-MM-dd HH:mm:ss","preserveZeroDatetime":true}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/output"},"filterConfig":{"weakRead":true,"enableHiddenPk":true},"ddlBehavior":{"addExtraMessage":true,"sequencePolicy":"RESTART"}}}`
	preview, snapshot := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "expert-parameters")
	if drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v11-expert-parameters" || prechecks.created.SysCredentialRevision != 2 {
		t.Fatal("新版元数据或预检查 sys 引用没有冻结")
	}
	for _, flag := range []string{"--weak-read", "--enable-hidden-pk", "--add-extra-message", "--sequence-policy", "--preserve-zero-datetime", "--time-value-format", "--timestamp-value-format"} {
		if !strings.Contains(preview, flag) {
			t.Fatalf("预览缺少 %s", flag)
		}
	}
	for _, browser := range []string{preview, snapshot} {
		if strings.Contains(browser, "synthetic_private_sys") || strings.Contains(browser, sources.summaries[0].SysCredentialID) || strings.Contains(browser, "_sysCredential") {
			t.Fatal("浏览器暴露了 sys 信息")
		}
	}
	if !strings.Contains(tasks.input.SnapshotJSON, "_sysCredential") || !strings.Contains(tasks.input.PlannedArgvJSON, "synthetic_private_sys") {
		t.Fatal("任务没有保存内部 sys 绑定")
	}
}

func TestOracleTimestampExportFlowUsesV11(t *testing.T) {
	drafts, prechecks, tasks := &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{}
	sources := mustStaticDataSourceReader()
	sources.summaries[0].CompatibilityMode = "ORACLE"
	handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, prechecks, tasks, nil, sources)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CUT","timestampFormats":{"dateValueFormat":"yyyy-MM-dd HH:mm:ss","timestampTzValueFormat":"yyyy-MM-dd HH:mm:ss","timestampLtzValueFormat":"yyyy-MM-dd HH:mm:ss","nlsDateFormat":"YYYY-MM-DD HH24:MI:SS","nlsTimestampFormat":"YYYY-MM-DD HH24:MI:SS:FF9","nlsTimestampTzFormat":"YYYY-MM-DD HH24:MI:SS:FF9 TZR"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/output"}}}`
	preview, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "oracle-expert-formats")
	for _, flag := range []string{"--date-value-format", "--timestamp-tz-value-format", "--timestamp-ltz-value-format", "--nls-date-format", "--nls-timestamp-format", "--nls-timestamp-tz-format"} {
		if !strings.Contains(preview, flag) {
			t.Fatalf("Oracle 格式缺少 %s", flag)
		}
	}
	if prechecks.created.SysCredentialID != "" {
		t.Fatal("无 sys 参数的草稿解析了 sys 材料")
	}
}

func TestOldPrecheckPayloadOmitsInactiveExpertFlags(t *testing.T) {
	payload := precheckExecutionContextPayload(store.PrecheckExecutionContext{})
	for _, flag := range []string{"weakRead", "enableHiddenPk", "addExtraMessage"} {
		if _, found := payload[flag]; found {
			t.Fatal("旧协议出现未启用的新字段")
		}
	}
}

func TestEditingOldDraftAtomicallyUpgradesMetadata(t *testing.T) {
	drafts := &recordingDraftStore{}
	handler := newGeneralizedFlowHandler(t, drafts, &recordingPrecheckStore{}, &recordingTaskStore{})
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/output"}}}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts", bytes.NewBufferString(body))
	create.Header.Set("Idempotency-Key", "synthetic-expert-upgrade-key")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatal("旧基线草稿未创建")
	}
	body = strings.Replace(body, `"dataFormat":{"formatKind":"CSV"}`, `"dataFormat":{"formatKind":"CSV","timestampFormats":{"timeValueFormat":"HH:mm:ss"}}`, 1)
	update := httptest.NewRequest(http.MethodPatch, "/api/v1/export-drafts/draft-synthetic", bytes.NewBufferString(body))
	update.Header.Set("If-Match", `"rev-1"`)
	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, update)
	if updated.Code != http.StatusOK || drafts.created.Revision != 2 || drafts.created.MetadataVersion != "obdumper-4.3.5-slice-v11-expert-parameters" {
		t.Fatalf("新版元数据未随更新保存: %d", updated.Code)
	}
	preview := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:preview-command", nil)
	preview.Header.Set("If-Match", `"rev-2"`)
	previewed := httptest.NewRecorder()
	handler.ServeHTTP(previewed, preview)
	if previewed.Code != http.StatusOK || !strings.Contains(previewed.Body.String(), "--time-value-format") {
		t.Fatal("更新后的草稿无法按 v11 重放")
	}
}

func TestOracleNlsSQLExportEmitsSessionFormats(t *testing.T) {
	drafts, prechecks, tasks := &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{}
	sources := mustStaticDataSourceReader()
	sources.summaries[0].CompatibilityMode = "ORACLE"
	handler := newGeneralizedFlowHandlerWithDataSources(t, drafts, prechecks, tasks, nil, sources)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"SQL","timestampFormats":{"nlsDateFormat":"YYYY-MM-DD HH24:MI:SS"}},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/output"}}}`
	preview, _ := runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "oracle-nls-sql")
	if !strings.Contains(preview, "--nls-date-format") {
		t.Fatal("SQL 数据丢失了 Oracle 会话格式")
	}
}
