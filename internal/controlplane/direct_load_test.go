package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
	"ob-data-orch/internal/store"
)

type directLoadMemoryStore struct {
	item    store.DirectLoadDraft
	writes  int
	check   *store.DirectLoadCheck
	created *store.PrecheckCreate
}

func (s *directLoadMemoryStore) SaveDirectLoadDraft(_ context.Context, input store.DirectLoadDraftWrite) (store.DirectLoadDraft, error) {
	s.writes++
	s.item = input.DirectLoadDraft
	s.item.Revision = input.ExpectedRevision + 1
	return s.item, nil
}
func (s *directLoadMemoryStore) GetDirectLoadDraft(_ context.Context, id, owner string) (store.DirectLoadDraft, error) {
	if s.item.ID != id || s.item.OwnerSubjectID != owner {
		return store.DirectLoadDraft{}, store.ErrDataSourceNotFound
	}
	return s.item, nil
}
func (s *directLoadMemoryStore) ListDirectLoadDrafts(context.Context, string) ([]store.DirectLoadDraft, error) {
	return []store.DirectLoadDraft{s.item}, nil
}
func (s *directLoadMemoryStore) DeleteDirectLoadDraft(context.Context, string, string, int64, string, time.Time) error {
	return nil
}
func (s *directLoadMemoryStore) GetDirectLoadCheck(_ context.Context, id, owner string) (store.DirectLoadCheck, error) {
	if s.check != nil && s.check.Run.PrecheckID == id && owner == s.item.OwnerSubjectID {
		return *s.check, nil
	}
	return store.DirectLoadCheck{}, store.ErrDataSourceNotFound
}

func (s *directLoadMemoryStore) CreateDirectLoadPrecheck(_ context.Context, input store.PrecheckCreate) (store.PrecheckCreateResult, error) {
	s.created = &input
	return store.PrecheckCreateResult{PrecheckID: input.PrecheckID}, nil
}

func directLoadHandler(t *testing.T, s *directLoadMemoryStore, csrf CSRFValidator) http.Handler {
	t.Helper()
	generator, err := commandgen.NewDirectLoad()
	if err != nil {
		t.Fatal(err)
	}
	return NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: connectionTestAuthorizer{}, DataSources: staticDataSourceReader{}, CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, DirectLoadDrafts: s, DirectLoadGenerator: generator, CSRF: csrf, RealExecutionEnabled: true})
}
func directLoadRequest(handler http.Handler, method, path, body, revision string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "direct-load-synthetic-key")
	if revision != "" {
		r.Header.Set("If-Match", revision)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func directLoadBody() string {
	c := directload.Config{DataSourceID: "source-allowed", NodeID: "node-1", Database: "synthetic_db", FilePath: `E:\synthetic-input`, Scenario: "PRIVATE_ODP", RPCPort: 2885, Table: "synthetic", Format: "CSV", Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	raw, _ := json.Marshal(directLoadWriteRequest{ConfigVersion: directload.ConfigVersion, Config: c})
	return string(raw)
}

func Test旁路导入接口保存恢复与确定性命令(t *testing.T) {
	s := &directLoadMemoryStore{}
	h := directLoadHandler(t, s, allowedCSRF{})
	created := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts", directLoadBody(), "")
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	if s.writes != 1 || len(s.item.ConfigFingerprint) != 64 {
		t.Fatal("草稿未绑定指纹")
	}
	path := "/api/v1/direct-load-drafts/" + s.item.ID
	read := directLoadRequest(h, "GET", path, "", "")
	if read.Code != 200 || read.Header().Get("ETag") != `"rev-1"` {
		t.Fatal("读取修订失败", read.Code)
	}
	first := directLoadRequest(h, "POST", path+":preview-command", `{}`, `"rev-1"`)
	second := directLoadRequest(h, "POST", path+":preview-command", `{}`, `"rev-1"`)
	if first.Code != 200 || second.Code != 200 {
		t.Fatal("命令生成失败", first.Code, first.Body.String())
	}
	var a, b map[string]any
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a["redactedCommand"] != b["redactedCommand"] || a["configFingerprint"] != s.item.ConfigFingerprint || a["executionEnabled"] != false || !strings.Contains(a["redactedCommand"].(string), "-p ******") {
		t.Fatal("命令与冻结绑定或门禁不一致")
	}
	for _, suffix := range []string{":submit", ":precheck"} {
		response := directLoadRequest(h, "POST", path+suffix, `{}`, `"rev-1"`)
		if response.Code != 422 && response.Code != 503 {
			t.Fatal("未装配能力没有失败关闭", suffix, response.Code)
		}
	}
	stale := directLoadRequest(h, "PATCH", path, directLoadBody(), `"rev-999"`)
	if stale.Code != 412 || s.writes != 1 {
		t.Fatal("旧修订触发保存")
	}
}

func Test旁路导入接口拒绝越权CSRF未知参数与无修订写入(t *testing.T) {
	for _, test := range []struct {
		name, body, revision string
		csrf                 CSRFValidator
		status               int
	}{
		{"CSRF", directLoadBody(), "", deniedCSRF{}, 401},
		{"普通导入恢复参数", strings.Replace(directLoadBody(), `"rpcPort":2885`, `"rpcPort":2885,"retry":true`, 1), "", allowedCSRF{}, 400},
		{"重复旁路参数", strings.Replace(directLoadBody(), `"rpcPort":2885`, `"rpcPort":2885,"rpcPort":2882`, 1), "", allowedCSRF{}, 400},
		{"空值", strings.Replace(directLoadBody(), `"rpcPort":2885`, `"rpcPort":null`, 1), "", allowedCSRF{}, 400},
		{"未知格式", strings.Replace(directLoadBody(), `"format":"CSV"`, `"format":"MIX"`, 1), "", allowedCSRF{}, 422},
		{"数据源越权", strings.Replace(directLoadBody(), `"source-allowed"`, `"source-denied"`, 1), "", allowedCSRF{}, 404},
		{"路径越界", strings.Replace(directLoadBody(), `E:\\synthetic-input`, `s3://synthetic/input`, 1), "", allowedCSRF{}, 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &directLoadMemoryStore{}
			h := directLoadHandler(t, s, test.csrf)
			response := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts", test.body, test.revision)
			if response.Code != test.status || s.writes != 0 {
				t.Fatal(response.Code, response.Body.String())
			}
		})
	}
	s := &directLoadMemoryStore{}
	h := directLoadHandler(t, s, allowedCSRF{})
	_ = directLoadRequest(h, "POST", "/api/v1/direct-load-drafts", directLoadBody(), "")
	s.item.OwnerSubjectID = "other-subject"
	if response := directLoadRequest(h, "GET", "/api/v1/direct-load-drafts/"+s.item.ID, "", ""); response.Code != 404 {
		t.Fatal("泄露其他所有者草稿")
	}
	if response := directLoadRequest(h, "PATCH", "/api/v1/direct-load-drafts/"+s.item.ID, directLoadBody(), ""); response.Code != 428 {
		t.Fatal("缺少修订仍写入")
	}
}

func Test旁路待检查回执完整目录与真实执行门控(t *testing.T) {
	s := &directLoadMemoryStore{}
	h := directLoadHandler(t, s, allowedCSRF{})
	created := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts", directLoadBody(), "")
	if created.Code != 201 {
		t.Fatal(created.Code)
	}
	s.check = &store.DirectLoadCheck{CheckKind: "DIRECT_LOAD_PREFLIGHT", Run: store.PrecheckRun{PrecheckID: "direct-check", DraftID: s.item.ID, DraftRevision: 1, DataSourceID: s.item.DataSourceID, NodeID: s.item.NodeID, ConfigFingerprint: s.item.ConfigFingerprint, Status: "PENDING", IntegrityStatus: "UNKNOWN", ValidUntil: time.Now().Add(time.Minute)}}
	read := directLoadRequest(h, "GET", "/api/v1/direct-load-prechecks/direct-check", "", "")
	if read.Code != 200 {
		t.Fatal(read.Code, read.Body.String())
	}
	var body struct {
		Results          []store.PrecheckCheckResult `json:"results"`
		ExecutionEnabled bool                        `json:"executionEnabled"`
	}
	if json.Unmarshal(read.Body.Bytes(), &body) != nil || body.ExecutionEnabled || len(body.Results) != 9 {
		t.Fatal("待检查响应不完整")
	}
	for _, result := range body.Results {
		if result.Status != "UNKNOWN" || result.EvidenceCode != "DIRECT_LOAD_UNAVAILABLE" {
			t.Fatal("待检查混入其他模块结果")
		}
	}
	metadata := directLoadRequest(h, "GET", "/api/v1/direct-load-parameter-metadata", "", "")
	var catalog struct {
		Definitions     []any  `json:"definitions"`
		MetadataVersion string `json:"metadataVersion"`
	}
	if metadata.Code != 200 || json.Unmarshal(metadata.Body.Bytes(), &catalog) != nil || len(catalog.Definitions) != 103 || catalog.MetadataVersion != directload.MetadataVersion {
		t.Fatal("目录缺项或混用版本", metadata.Code)
	}
	path := "/api/v1/direct-load-drafts/" + s.item.ID + ":submit"
	response := directLoadRequest(h, "POST", path, `{"precheckId":"direct-check","taskName":"合成旁路任务","confirmedConfigFingerprint":"`+s.item.ConfigFingerprint+`"}`, `"rev-1"`)
	var failure struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &failure)
	if response.Code != 422 || failure.Code != "DIRECT_LOAD_EXECUTION_GATED" {
		t.Fatal("借用导出开关开放了旁路", response.Code, response.Body.String())
	}
}

func Test旁路文件发现要求最小绑定但不要求RPC与完整适用确认(t *testing.T) {
	for _, field := range []string{"", "dataSourceId", "nodeId", "database", "scenario", "filePath", "table", "format"} {
		t.Run(field, func(t *testing.T) {
			s := &directLoadMemoryStore{}
			h := directLoadHandler(t, s, allowedCSRF{})
			if response := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts", directLoadBody(), ""); response.Code != 201 {
				t.Fatal(response.Code)
			}
			var config map[string]any
			if json.Unmarshal([]byte(s.item.ConfigJSON), &config) != nil {
				t.Fatal("合成配置无效")
			}
			config["rpcPort"] = 0
			config["applicability"] = map[string]bool{"singleTable": false, "wholeTableCommit": false, "noCheckpoint": false, "structureLimits": false}
			if field != "" {
				config[field] = ""
			}
			raw, _ := json.Marshal(config)
			s.item.ConfigJSON = string(raw)
			h = NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: connectionTestAuthorizer{}, DataSources: staticDataSourceReader{}, CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, DirectLoadDrafts: s, CSRF: allowedCSRF{}, Prechecks: &recordingPrecheckStore{}, AgentPrechecks: unreachableAgentPrecheckStore{}, PrecheckTTL: time.Minute})
			response := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts/"+s.item.ID+":discover-files", `{}`, `"rev-1"`)
			if field == "" {
				if response.Code != 202 || s.created == nil || s.created.CheckKind != "DIRECT_LOAD_FILES" {
					t.Fatal("文件发现误要求RPC或完整适用确认", response.Code, response.Body.String())
				}
			} else if response.Code != 422 || s.created != nil {
				t.Fatal("缺少绑定仍排队文件发现", field, response.Code, response.Body.String())
			}
		})
	}
}

func Test旁路提交独立门禁写权限管理员确认与完整证明(t *testing.T) {
	for _, scenario := range []string{"完整冻结", "默认关闭", "探针关闭", "只读身份", "非管理员", "未确认", "文件检查", "缺少检查", "普通导入证据", "缺少名称", "缺少安全配置指纹"} {
		t.Run(scenario, func(t *testing.T) {
			drafts := &directLoadMemoryStore{}
			tasks := &recordingTaskStore{}
			generator, err := commandgen.NewDirectLoad()
			if err != nil {
				t.Fatal(err)
			}
			var authorizer identity.Authorizer = connectionTestAuthorizer{}
			if scenario == "只读身份" {
				authorizer = sliceAuthorizer{}
			}
			var roles identity.RoleAuthorizer = allowedRoleAuthorizer{}
			if scenario == "非管理员" {
				roles = nil
			}
			h := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: authorizer, Roles: roles, DataSources: staticDataSourceReader{}, CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, DirectLoadDrafts: drafts, DirectLoadGenerator: generator, CSRF: allowedCSRF{}, RealExecutionEnabled: true, DirectLoadProbeEnabled: scenario != "探针关闭", DirectLoadExecutionEnabled: scenario != "默认关闭", Tasks: tasks})
			if response := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts", directLoadBody(), ""); response.Code != 201 {
				t.Fatal(response.Code, response.Body.String())
			}
			files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: 1, SHA256: strings.Repeat("a", 64)}}
			raw, _ := json.Marshal(files)
			digest := sha256.Sum256(raw)
			manifest := &importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(digest[:])}
			check := &store.DirectLoadCheck{CheckKind: "DIRECT_LOAD_PREFLIGHT", Run: store.PrecheckRun{PrecheckID: "direct-check", DraftID: drafts.item.ID, DraftRevision: 1, ConfigFingerprint: drafts.item.ConfigFingerprint, Status: "SUCCEEDED", IntegrityStatus: "COMPLETE", DirectLoadConfigFingerprint: strings.Repeat("9", 64), ValidUntil: time.Now().Add(time.Minute), CredentialID: "synthetic-credential", CredentialRevision: 1}, InputManifest: manifest}
			for _, name := range precheckcontract.DirectLoadChecks(check.CheckKind) {
				check.Run.Results = append(check.Run.Results, store.PrecheckCheckResult{Check: name, Status: "PASSED", EvidenceCode: "DIRECT_LOAD_VERIFIED"})
			}
			drafts.check = check
			if scenario == "文件检查" {
				check.CheckKind = "DIRECT_LOAD_FILES"
			}
			if scenario == "缺少检查" {
				check.Run.Results = check.Run.Results[:8]
			}
			if scenario == "普通导入证据" {
				check.Run.Results[0].EvidenceCode = "IMPORT_VERIFIED"
			}
			if scenario == "缺少安全配置指纹" {
				check.Run.DirectLoadConfigFingerprint = ""
			}
			body := map[string]any{"precheckId": "direct-check", "taskName": "  合成旁路任务  ", "confirmedConfigFingerprint": drafts.item.ConfigFingerprint}
			if scenario == "未确认" {
				delete(body, "confirmedConfigFingerprint")
			}
			if scenario == "缺少名称" {
				delete(body, "taskName")
			}
			request, _ := json.Marshal(body)
			response := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts/"+drafts.item.ID+":submit", string(request), `"rev-1"`)
			want := 422
			if scenario == "完整冻结" {
				want = 201
			}
			if scenario == "只读身份" || scenario == "非管理员" {
				want = 403
			}
			if response.Code != want {
				t.Fatal(response.Code, response.Body.String())
			}
			if scenario == "完整冻结" {
				var frozen store.DirectLoadSnapshot
				if json.Unmarshal([]byte(tasks.input.SnapshotJSON), &frozen) != nil || frozen.TaskName != "合成旁路任务" || len(frozen.PrecheckProof.Results) != 9 || frozen.TaskType != directload.TaskType || tasks.input.TaskType != directload.TaskType || strings.Contains(tasks.input.PlannedArgvJSON, "--retry") || strings.Contains(tasks.input.PlannedArgvJSON, "password") {
					t.Fatal("旁路冻结类型或完整证明错误")
				}
				check.SubmittedTaskID, check.SubmittedTaskName = tasks.input.TaskID, frozen.TaskName
				check.SubmittedIdempotencyKey = "direct-load-synthetic-key"
				check.Run.ValidUntil, check.Run.Status = time.Now().Add(-time.Hour), "EXPIRED"
				replayed := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts/"+drafts.item.ID+":submit", string(request), `"rev-1"`)
				if replayed.Code != 201 || !strings.Contains(replayed.Body.String(), `"replayed":true`) || !strings.Contains(replayed.Body.String(), check.SubmittedTaskID) {
					t.Fatal("过期证明无法找回已提交结果", replayed.Code, replayed.Body.String())
				}
				body["taskName"] = "另一个合成任务"
				changed, _ := json.Marshal(body)
				if rejected := directLoadRequest(h, "POST", "/api/v1/direct-load-drafts/"+drafts.item.ID+":submit", string(changed), `"rev-1"`); rejected.Code != 409 {
					t.Fatal("已消费证明接受更改名称", rejected.Code)
				}
			} else if tasks.input.TaskID != "" {
				t.Fatal("旁路门禁失败后仍创建任务")
			}
		})
	}
}
