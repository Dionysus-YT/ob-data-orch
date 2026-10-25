package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/store"
)

type normalReuseTemplates struct{ *recordingTemplateStore }

func (s normalReuseTemplates) ListConfigTemplates(ctx context.Context, owner string) ([]store.ExportConfigTemplate, error) {
	return s.ListExportConfigTemplates(ctx, owner)
}

func normalReuseSource(t *testing.T) store.TaskDerivationSource {
	t.Helper()
	var request normalImportWriteRequest
	if err := json.Unmarshal([]byte(normalImportBody()), &request); err != nil {
		t.Fatal(err)
	}
	files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: 1, SHA256: strings.Repeat("a", 64)}}
	raw, _ := json.Marshal(files)
	digest := sha256.Sum256(raw)
	snapshot, _ := json.Marshal(store.NormalImportSnapshot{ConfigVersion: importdomain.ConfigVersion, TaskType: importdomain.TaskType, Config: request.Config, InputManifest: importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(digest[:])}})
	return store.TaskDerivationSource{TaskID: "synthetic-normal-task", TaskType: importdomain.TaskType, DataSourceID: request.Config.DataSourceID, NodeID: request.Config.NodeID, State: "SUCCEEDED", SnapshotVersion: importdomain.ConfigVersion, SnapshotJSON: string(snapshot), MetadataVersion: importdomain.MetadataVersion, CapabilityVersion: importdomain.CapabilityVersion, ToolVersion: importdomain.ToolVersion}
}

func normalReuseHandler(t *testing.T, source store.TaskDerivationSource, csrf CSRFValidator) (http.Handler, *normalImportMemoryStore, *recordingTemplateStore) {
	t.Helper()
	imports := &normalImportMemoryStore{}
	templates := newRecordingTemplateStore()
	generator, err := commandgen.NewNormalImport()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{}, CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, NormalImportDrafts: imports, NormalImportGenerator: generator, CSRF: csrf, Drafts: &recordingDraftStore{}, Tasks: &recordingTaskStore{derivationSource: source}, Templates: normalReuseTemplates{templates}})
	return h, imports, templates
}

func Test普通导入历史复用端到端类型与门禁(t *testing.T) {
	for _, kind := range []string{"REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH"} {
		t.Run(kind, func(t *testing.T) {
			source := normalReuseSource(t)
			if kind == "RERUN_FROM_SCRATCH" {
				source.State = "FAILED"
			}
			h, drafts, _ := normalReuseHandler(t, source, allowedCSRF{})
			response := normalImportRequest(h, "POST", "/api/v1/tasks/"+source.TaskID+":rebuild-draft", `{"derivation":"`+kind+`"}`, "")
			if response.Code != 201 || !strings.Contains(response.Body.String(), `"taskType":"OBLOADER_NORMAL_IMPORT"`) || drafts.item.SourceTaskID != source.TaskID || drafts.item.SourceDerivation != kind || drafts.writes != 1 {
				t.Fatal("未创建独立普通导入草稿", response.Code, response.Body.String())
			}
			config, _ := importdomain.Decode(drafts.item.ConfigJSON)
			if config.TaskName != "" || strings.Contains(drafts.item.ConfigJSON, "inputManifest") {
				t.Fatal("复制了名称或输入证据")
			}
		})
	}
	for _, scenario := range []string{"核对中", "成功重跑", "版本不符", "CSRF"} {
		t.Run(scenario, func(t *testing.T) {
			source := normalReuseSource(t)
			csrf := CSRFValidator(allowedCSRF{})
			kind := "REBUILD_FROM_CONFIG"
			if scenario == "核对中" {
				source.ReconciliationRequired = true
			}
			if scenario == "成功重跑" {
				kind = "RERUN_FROM_SCRATCH"
			}
			if scenario == "版本不符" {
				source.SnapshotVersion = "v2"
			}
			if scenario == "CSRF" {
				csrf = deniedCSRF{}
			}
			h, drafts, _ := normalReuseHandler(t, source, csrf)
			response := normalImportRequest(h, "POST", "/api/v1/tasks/"+source.TaskID+":rebuild-draft", `{"derivation":"`+kind+`"}`, "")
			if response.Code < 400 || drafts.writes != 0 {
				t.Fatal("越过复用门禁", response.Code)
			}
		})
	}
}

func Test普通导入模板保存目录绑定与旧路由隔离(t *testing.T) {
	source := normalReuseSource(t)
	h, drafts, templates := normalReuseHandler(t, source, allowedCSRF{})
	saved := normalImportRequest(h, "POST", "/api/v1/tasks/"+source.TaskID+":save-template", `{"displayName":"合成普通导入模板"}`, "")
	if saved.Code != 201 || templates.created.TaskType != importdomain.TaskType {
		t.Fatal("保存模板类型错误", saved.Code, saved.Body.String())
	}
	c, _ := importdomain.Decode(templates.created.ConfigJSON)
	if c.DataSourceID != "" || c.NodeID != "" || c.TaskName != "" {
		t.Fatal("模板携带绑定或名称")
	}
	list := normalImportRequest(h, "GET", "/api/v1/config-templates", "", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"taskType":"OBLOADER_NORMAL_IMPORT"`) || strings.Contains(list.Body.String(), "filePath") {
		t.Fatal("模板目录不安全或丢失类型", list.Code)
	}
	request := `{"dataSourceId":"source-allowed","nodeId":"node-1"}`
	for _, suffix := range []string{":create-draft", ""} {
		method := "POST"
		if suffix == "" {
			method = "DELETE"
		}
		legacy := normalImportRequest(h, method, "/api/v1/export-config-templates/template-synthetic"+suffix, request, `"rev-1"`)
		if legacy.Code != 404 || drafts.writes != 0 || templates.deleted != "" {
			t.Fatal("旧导出路由跨类型", legacy.Code)
		}
	}
	created := normalImportRequest(h, "POST", "/api/v1/config-templates/template-synthetic:create-draft", request, "")
	if created.Code != 201 || drafts.writes != 1 || !strings.Contains(created.Body.String(), `"taskType":"OBLOADER_NORMAL_IMPORT"`) || drafts.item.SourceTaskID != "" {
		t.Fatal("模板未承接普通导入草稿", created.Code, created.Body.String())
	}
	denied := normalImportRequest(h, "POST", "/api/v1/config-templates/template-synthetic:create-draft", `{"dataSourceId":"source-denied","nodeId":"node-1"}`, "")
	if denied.Code != 404 || drafts.writes != 1 {
		t.Fatal("模板扩大数据源权限", denied.Code)
	}
}

func Test普通导入操作资格与检查点隔离(t *testing.T) {
	for _, state := range []string{"SUCCEEDED", "FAILED", "CANCELLED"} {
		item := store.TaskListItem{TaskType: importdomain.TaskType, SnapshotVersion: importdomain.ConfigVersion, State: state, Operable: true, CheckpointPresent: true}
		actions := taskListActions(item, true, true)
		if !slices.Contains(actions, "REBUILD_FROM_CONFIG") || slices.Contains(actions, "CHECKPOINT_RESUME") || slices.Contains(actions, "SAVE_TEMPLATE") != (state == "SUCCEEDED") || slices.Contains(actions, "RERUN_FROM_SCRATCH") != (state != "SUCCEEDED") {
			t.Fatal("普通导入资格错误", state, actions)
		}
		item.ReconciliationRequired = true
		if actions = taskListActions(item, true, true); slices.Contains(actions, "REBUILD_FROM_CONFIG") || slices.Contains(actions, "SAVE_TEMPLATE") {
			t.Fatal("核对中仍可复用")
		}
	}
}
