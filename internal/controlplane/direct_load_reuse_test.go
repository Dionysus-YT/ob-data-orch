package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
	"ob-data-orch/internal/store"
)

func directLoadReuseSource(t *testing.T) store.TaskDerivationSource {
	t.Helper()
	var request directLoadWriteRequest
	if err := json.Unmarshal([]byte(directLoadBody()), &request); err != nil {
		t.Fatal(err)
	}
	files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: request.Config.Table, Size: 2, Modified: 1, SHA256: strings.Repeat("a", 64)}}
	raw, _ := json.Marshal(files)
	digest := sha256.Sum256(raw)
	proof := store.DirectLoadPrecheckProof{ID: "synthetic-source-check", DraftRevision: 1, ConfigFingerprint: strings.Repeat("b", 64)}
	for _, check := range precheckcontract.DirectLoadChecks("DIRECT_LOAD_PREFLIGHT") {
		proof.Results = append(proof.Results, store.PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: "DIRECT_LOAD_VERIFIED"})
	}
	snapshot, _ := json.Marshal(store.DirectLoadSnapshot{ConfigVersion: directload.ConfigVersion, TaskType: directload.TaskType, TaskName: "合成来源", Config: request.Config, InputManifest: importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(digest[:])}, PrecheckProof: proof, DirectLoadConfigFingerprint: strings.Repeat("9", 64)})
	return store.TaskDerivationSource{TaskID: "synthetic-direct-task", TaskType: directload.TaskType, DataSourceID: request.Config.DataSourceID, NodeID: request.Config.NodeID, State: "FAILED", SnapshotVersion: directload.ConfigVersion, SnapshotJSON: string(snapshot), MetadataVersion: directload.MetadataVersion, CapabilityVersion: directload.CapabilityVersion, ToolVersion: directload.ToolVersion, PrecheckID: proof.ID, ConfigFingerprint: proof.ConfigFingerprint}
}

func directLoadReuseHandler(t *testing.T, source store.TaskDerivationSource, csrf CSRFValidator) (http.Handler, *directLoadMemoryStore) {
	t.Helper()
	drafts := &directLoadMemoryStore{}
	generator, err := commandgen.NewDirectLoad()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{}, CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, DirectLoadDrafts: drafts, DirectLoadGenerator: generator, CSRF: csrf, Tasks: &recordingTaskStore{derivationSource: source}})
	return h, drafts
}

func Test旁路历史复用端点与从头执行参数锁(t *testing.T) {
	for _, kind := range []string{"REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH"} {
		t.Run(kind, func(t *testing.T) {
			source := directLoadReuseSource(t)
			h, drafts := directLoadReuseHandler(t, source, allowedCSRF{})
			response := directLoadRequest(h, "POST", "/api/v1/tasks/"+source.TaskID+":rebuild-draft", `{"derivation":"`+kind+`"}`, "")
			if response.Code != 201 || !strings.Contains(response.Body.String(), `"taskType":"OBLOADER_DIRECT_LOAD"`) || drafts.item.SourceTaskID != source.TaskID || drafts.item.SourceDerivation != kind {
				t.Fatal("派生类型或来源丢失", response.Code, response.Body.String())
			}
			if strings.Contains(drafts.item.ConfigJSON, "inputManifest") || strings.Contains(drafts.item.ConfigJSON, "precheckProof") {
				t.Fatal("草稿复制运行证明")
			}
			read := directLoadRequest(h, "GET", "/api/v1/direct-load-drafts/"+drafts.item.ID, "", "")
			if read.Code != 200 || !strings.Contains(read.Body.String(), `"parametersLocked":`+map[bool]string{true: "true", false: "false"}[kind == "RERUN_FROM_SCRATCH"]) {
				t.Fatal("读取未提供正确参数锁", read.Code)
			}
			if kind == "RERUN_FROM_SCRATCH" {
				unchanged := directLoadRequest(h, "PATCH", "/api/v1/direct-load-drafts/"+drafts.item.ID, directLoadBody(), `"rev-1"`)
				if unchanged.Code != 200 || drafts.item.SourceTaskID != source.TaskID {
					t.Fatal("同配置不能重绑当前事实", unchanged.Code, unchanged.Body.String())
				}
				changed := strings.Replace(directLoadBody(), `"rpcPort":2885`, `"rpcPort":2886`, 1)
				denied := directLoadRequest(h, "PATCH", "/api/v1/direct-load-drafts/"+drafts.item.ID, changed, `"rev-2"`)
				if denied.Code != 422 || !strings.Contains(denied.Body.String(), "DIRECT_LOAD_PARAMETERS_LOCKED") || drafts.writes != 2 {
					t.Fatal("从头执行允许改值", denied.Code, denied.Body.String())
				}
			}
		})
	}
	for _, scenario := range []string{"核对中", "成功重跑", "证明错配", "CSRF"} {
		t.Run(scenario, func(t *testing.T) {
			source := directLoadReuseSource(t)
			csrf := CSRFValidator(allowedCSRF{})
			if scenario == "核对中" {
				source.ReconciliationRequired = true
			}
			if scenario == "成功重跑" {
				source.State = "SUCCEEDED"
			}
			if scenario == "证明错配" {
				source.PrecheckID = "different-proof"
			}
			if scenario == "CSRF" {
				csrf = deniedCSRF{}
			}
			h, drafts := directLoadReuseHandler(t, source, csrf)
			r := directLoadRequest(h, "POST", "/api/v1/tasks/"+source.TaskID+":rebuild-draft", `{"derivation":"RERUN_FROM_SCRATCH"}`, "")
			if r.Code < 400 || drafts.writes != 0 {
				t.Fatal("越过来源门禁", r.Code)
			}
		})
	}
}

func Test旁路任务操作不取得模板取消检查点资格(t *testing.T) {
	for _, state := range []string{"SUCCEEDED", "FAILED", "CANCELLED", "RUNNING"} {
		item := store.TaskListItem{TaskType: directload.TaskType, SnapshotVersion: directload.ConfigVersion, State: state, Operable: true, CheckpointPresent: true}
		actions := taskListActions(item, true, true)
		if slices.Contains(actions, "CANCEL") || slices.Contains(actions, "CHECKPOINT_RESUME") || slices.Contains(actions, "SAVE_TEMPLATE") || slices.Contains(actions, "REBUILD_FROM_CONFIG") != (state != "RUNNING") || slices.Contains(actions, "RERUN_FROM_SCRATCH") != (state == "FAILED" || state == "CANCELLED") {
			t.Fatal("旁路动作越界", state, actions)
		}
		item.ReconciliationRequired = true
		if slices.Contains(taskListActions(item, true, true), "REBUILD_FROM_CONFIG") {
			t.Fatal("核对中仍复用")
		}
	}
}
