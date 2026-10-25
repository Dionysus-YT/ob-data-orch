package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/store"
)

type normalImportMemoryStore struct {
	item    store.NormalImportDraft
	writes  int
	check   *store.NormalImportCheck
	saveErr error
}

func (s *normalImportMemoryStore) SaveNormalImportDraft(_ context.Context, input store.NormalImportDraftWrite) (store.NormalImportDraft, error) {
	if s.saveErr != nil {
		return store.NormalImportDraft{}, s.saveErr
	}
	s.writes++
	s.item = input.NormalImportDraft
	s.item.Revision = input.ExpectedRevision + 1
	return s.item, nil
}

func Test普通导入显式Strict生成同令牌布尔值(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		var input normalImportWriteRequest
		_ = json.Unmarshal([]byte(normalImportBody()), &input)
		input.Config.Execution.Strict = value
		raw, _ := json.Marshal(input)
		s := &normalImportMemoryStore{}
		h := normalImportHandler(t, s, allowedCSRF{})
		created := normalImportRequest(h, "POST", "/api/v1/normal-import-drafts", string(raw), "")
		if created.Code != 201 {
			t.Fatal("创建失败", created.Code)
		}
		preview := normalImportRequest(h, "POST", "/api/v1/normal-import-drafts/"+s.item.ID+":preview-command", `{}`, `"rev-1"`)
		if preview.Code != 200 || !strings.Contains(preview.Body.String(), "--strict="+value) || strings.Contains(preview.Body.String(), "--strict ") {
			t.Fatal("严格布尔值未使用唯一规范表达", preview.Code)
		}
	}
}

func Test普通导入重跑配置拒绝映射为不可重试业务错误(t *testing.T) {
	s := &normalImportMemoryStore{}
	h := normalImportHandler(t, s, allowedCSRF{})
	if response := normalImportRequest(h, "POST", "/api/v1/normal-import-drafts", normalImportBody(), ""); response.Code != 201 {
		t.Fatal(response.Code)
	}
	s.saveErr = store.ErrPrecheckInvalid
	response := normalImportRequest(h, "PATCH", "/api/v1/normal-import-drafts/"+s.item.ID, normalImportBody(), `"rev-1"`)
	if response.Code != 422 || !strings.Contains(response.Body.String(), "NORMAL_IMPORT_CONFIG_LOCKED") || strings.Contains(response.Body.String(), `"retryable":true`) || s.writes != 1 {
		t.Fatal("锁定被误报为服务故障", response.Code)
	}
}
func (s *normalImportMemoryStore) GetNormalImportDraft(_ context.Context, id, owner string) (store.NormalImportDraft, error) {
	if s.item.ID != id || s.item.OwnerSubjectID != owner {
		return store.NormalImportDraft{}, store.ErrDataSourceNotFound
	}
	return s.item, nil
}
func (s *normalImportMemoryStore) ListNormalImportDrafts(context.Context, string) ([]store.NormalImportDraft, error) {
	return []store.NormalImportDraft{s.item}, nil
}
func (s *normalImportMemoryStore) DeleteNormalImportDraft(context.Context, string, string, int64, string, time.Time) error {
	return nil
}
func (s *normalImportMemoryStore) GetNormalImportCheck(_ context.Context, id, owner string) (store.NormalImportCheck, error) {
	if s.check != nil && s.check.Run.PrecheckID == id && owner == s.item.OwnerSubjectID {
		return *s.check, nil
	}
	return store.NormalImportCheck{}, store.ErrDataSourceNotFound
}

func Test普通导入冻结提交复验写权限风险确认与完整用途(t *testing.T) {
	for _, scenario := range []string{"未确认", "已确认", "自定义名称", "无效名称", "只读身份", "文件用途", "缺少输入证明"} {
		t.Run(scenario, func(t *testing.T) {
			s := &normalImportMemoryStore{}
			tasks := &recordingTaskStore{}
			generator, err := commandgen.NewNormalImport()
			if err != nil {
				t.Fatal(err)
			}
			var authorizer identity.Authorizer = connectionTestAuthorizer{}
			if scenario == "只读身份" {
				authorizer = sliceAuthorizer{}
			}
			h := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: authorizer, Roles: allowedRoleAuthorizer{}, DataSources: staticDataSourceReader{}, CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, NormalImportDrafts: s, NormalImportGenerator: generator, CSRF: allowedCSRF{}, RealExecutionEnabled: true, NormalImportExecutionEnabled: true, Tasks: tasks})
			body := strings.Replace(normalImportBody(), `"cleanup":"NONE"`, `"cleanup":"NONE","replaceData":true`, 1)
			created := normalImportRequest(h, "POST", "/api/v1/normal-import-drafts", body, "")
			if created.Code != 201 {
				t.Fatal(created.Code, created.Body.String())
			}
			files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: 1, SHA256: strings.Repeat("a", 64)}}
			raw, _ := json.Marshal(files)
			digest := sha256.Sum256(raw)
			manifest := &importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(digest[:])}
			s.check = &store.NormalImportCheck{CheckKind: "NORMAL_IMPORT_PREFLIGHT", Run: store.PrecheckRun{PrecheckID: "synthetic-check", DraftID: s.item.ID, DraftRevision: 1, ConfigFingerprint: s.item.ConfigFingerprint, Status: "SUCCEEDED", IntegrityStatus: "COMPLETE", ValidUntil: time.Now().Add(time.Minute), CredentialID: "synthetic-credential", CredentialRevision: 1}, InputManifest: manifest}
			if scenario == "文件用途" {
				s.check.CheckKind = "NORMAL_IMPORT_FILES"
			}
			if scenario == "缺少输入证明" {
				s.check.InputManifest = nil
			}
			requestBody := fmt.Sprintf(`{"precheckId":"synthetic-check","confirmedConfigFingerprint":%q}`, s.item.ConfigFingerprint)
			if scenario == "未确认" {
				requestBody = `{"precheckId":"synthetic-check"}`
			}
			if scenario == "自定义名称" {
				requestBody = strings.TrimSuffix(requestBody, "}") + `,"taskName":"  合成自定义名称  "}`
			}
			if scenario == "无效名称" {
				requestBody = strings.TrimSuffix(requestBody, "}") + `,"taskName":"synthetic\nname"}`
			}
			response := normalImportRequest(h, "POST", "/api/v1/normal-import-drafts/"+s.item.ID+":submit", requestBody, `"rev-1"`)
			want := 422
			if scenario == "已确认" || scenario == "自定义名称" {
				want = 201
			} else if scenario == "只读身份" {
				want = 403
			}
			if response.Code != want {
				t.Fatal(response.Code, response.Body.String())
			}
			if scenario == "已确认" || scenario == "自定义名称" {
				if tasks.input.TaskType != importdomain.TaskType || tasks.input.SnapshotVersion != importdomain.ConfigVersion || strings.Contains(tasks.input.PlannedArgvJSON, "password") || !strings.Contains(tasks.input.PlannedArgvJSON, "--replace-data") {
					t.Fatal("冻结了错误类型或秘密参数")
				}
				var frozen store.NormalImportSnapshot
				if json.Unmarshal([]byte(tasks.input.SnapshotJSON), &frozen) != nil || frozen.TaskName == "" || frozen.Config.TaskName != "合成任务" {
					t.Fatal("提交名称未独立冻结或改写了预检查配置")
				}
				if scenario == "自定义名称" && frozen.TaskName != "合成自定义名称" {
					t.Fatal("自定义名称未保存")
				}
				if scenario == "已确认" && !strings.HasPrefix(frozen.TaskName, "普通导入-synthetic_db-CSV-") {
					t.Fatal("未按规则自动生成名称")
				}
			} else if tasks.input.TaskID != "" {
				t.Fatal("未通过门禁仍冻结任务")
			}
		})
	}
}
func (s *normalImportMemoryStore) CreateNormalImportPrecheck(context.Context, store.PrecheckCreate) (store.PrecheckCreateResult, error) {
	return store.PrecheckCreateResult{}, store.ErrPrecheckInvalid
}

func normalImportHandler(t *testing.T, s *normalImportMemoryStore, csrf CSRFValidator) http.Handler {
	t.Helper()
	generator, err := commandgen.NewNormalImport()
	if err != nil {
		t.Fatal(err)
	}
	return NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, DataSources: staticDataSourceReader{}, CredentialRefs: staticCredentialReferenceReader{}, Nodes: staticNodeReader{}, NormalImportDrafts: s, NormalImportGenerator: generator, CSRF: csrf, RealExecutionEnabled: true})
}
func normalImportRequest(handler http.Handler, method, path, body, revision string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "normal-import-synthetic-key")
	if revision != "" {
		r.Header.Set("If-Match", revision)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func normalImportBody() string {
	c := importdomain.Config{DataSourceID: "source-allowed", NodeID: "node-1", Database: "synthetic_db", FilePath: `E:\synthetic-input`, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE", TaskName: "合成任务"}
	raw, _ := json.Marshal(normalImportWriteRequest{ConfigVersion: importdomain.ConfigVersion, Config: c})
	return string(raw)
}

func Test普通导入接口保存恢复与确定性命令(t *testing.T) {
	s := &normalImportMemoryStore{}
	h := normalImportHandler(t, s, allowedCSRF{})
	created := normalImportRequest(h, "POST", "/api/v1/normal-import-drafts", normalImportBody(), "")
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	if s.writes != 1 || len(s.item.ConfigFingerprint) != 64 {
		t.Fatal("草稿未绑定指纹")
	}
	path := "/api/v1/normal-import-drafts/" + s.item.ID
	read := normalImportRequest(h, "GET", path, "", "")
	if read.Code != 200 || read.Header().Get("ETag") != `"rev-1"` {
		t.Fatal("读取修订失败", read.Code)
	}
	first := normalImportRequest(h, "POST", path+":preview-command", `{}`, `"rev-1"`)
	second := normalImportRequest(h, "POST", path+":preview-command", `{}`, `"rev-1"`)
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
		response := normalImportRequest(h, "POST", path+suffix, `{}`, `"rev-1"`)
		if response.Code != 422 && response.Code != 503 {
			t.Fatal("未装配能力没有失败关闭", suffix, response.Code)
		}
	}
	stale := normalImportRequest(h, "PATCH", path, normalImportBody(), `"rev-999"`)
	if stale.Code != 412 || s.writes != 1 {
		t.Fatal("旧修订触发保存")
	}
}

func Test普通导入接口拒绝越权CSRF未知参数与无修订写入(t *testing.T) {
	for _, test := range []struct {
		name, body, revision string
		csrf                 CSRFValidator
		status               int
	}{
		{"CSRF", normalImportBody(), "", deniedCSRF{}, 401},
		{"旁路参数", strings.Replace(normalImportBody(), `"cleanup":"NONE"`, `"cleanup":"NONE","direct":true`, 1), "", allowedCSRF{}, 400},
		{"未知格式", strings.Replace(normalImportBody(), `"format":"CSV"`, `"format":"MIX"`, 1), "", allowedCSRF{}, 422},
		{"数据源越权", strings.Replace(normalImportBody(), `"source-allowed"`, `"source-denied"`, 1), "", allowedCSRF{}, 404},
		{"路径越界", strings.Replace(normalImportBody(), `E:\\synthetic-input`, `s3://synthetic/input`, 1), "", allowedCSRF{}, 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &normalImportMemoryStore{}
			h := normalImportHandler(t, s, test.csrf)
			response := normalImportRequest(h, "POST", "/api/v1/normal-import-drafts", test.body, test.revision)
			if response.Code != test.status || s.writes != 0 {
				t.Fatal(response.Code, response.Body.String())
			}
		})
	}
	s := &normalImportMemoryStore{}
	h := normalImportHandler(t, s, allowedCSRF{})
	_ = normalImportRequest(h, "POST", "/api/v1/normal-import-drafts", normalImportBody(), "")
	s.item.OwnerSubjectID = "other-subject"
	if response := normalImportRequest(h, "GET", "/api/v1/normal-import-drafts/"+s.item.ID, "", ""); response.Code != 404 {
		t.Fatal("泄露其他所有者草稿")
	}
	if response := normalImportRequest(h, "PATCH", "/api/v1/normal-import-drafts/"+s.item.ID, normalImportBody(), ""); response.Code != 428 {
		t.Fatal("缺少修订仍写入")
	}
}
