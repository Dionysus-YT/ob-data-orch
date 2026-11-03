package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

type templateActionAuthorizer struct{ use, manage bool }

// Authorize 仅返回合成职责，不读取真实授权或网络。
func (a templateActionAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, _ string) error {
	if scope == identity.ScopeTemplateUse && a.use || scope == identity.ScopeTemplateManage && a.manage {
		return nil
	}
	return identity.ErrDenied
}

// Test模板操作资格不能从可见或所有者推导 验证使用与管理隔离及依赖缺失失败关闭。
func Test模板操作资格不能从可见或所有者推导(t *testing.T) {
	principal := identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-owner"}
	item := store.ExportConfigTemplate{TemplateID: "synthetic-template", OwnerSubjectID: principal.ID}
	cases := []struct {
		name                string
		use, manage, legacy bool
		want                []string
	}{
		{"仅使用", true, false, false, []string{"CREATE_DRAFT"}},
		{"仅管理", false, true, false, []string{"RENAME", "DELETE"}},
		{"无职责", false, false, false, []string{}},
		{"两种职责", true, true, false, []string{"CREATE_DRAFT", "RENAME", "DELETE"}},
		{"兼容自有对象", false, false, true, []string{"CREATE_DRAFT", "RENAME", "DELETE"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{authorizer: templateActionAuthorizer{use: tc.use, manage: tc.manage}}
			// 只标记持久授权已配置，投影不调用该合成存储的任何方法。
			if !tc.legacy {
				s.permissions = struct{ PermissionStore }{}
			}
			if got := s.templateActions(context.Background(), principal, item); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("操作资格不符: %v", got)
			}
		})
	}
	s := &Server{}
	if got := s.templateActions(context.Background(), principal, item); len(got) != 0 {
		t.Fatal("安全依赖缺失仍开放操作")
	}
}

type replayTemplateStore struct {
	*recordingTemplateStore
	digest string
}

// CreateExportConfigTemplate 在合成端口模拟现有仓储的同键重放与冲突规则。
func (s *replayTemplateStore) CreateExportConfigTemplate(ctx context.Context, input store.ExportConfigTemplateCreate) (store.ExportConfigTemplateCreateResult, error) {
	if s.digest != "" {
		if s.digest != input.RequestDigest {
			return store.ExportConfigTemplateCreateResult{}, store.ErrIdempotencyConflict
		}
		return store.ExportConfigTemplateCreateResult{TemplateID: "template-synthetic", Replayed: true}, nil
	}
	s.digest = input.RequestDigest
	return s.recordingTemplateStore.CreateExportConfigTemplate(ctx, input)
}

// Test模板保存原键重放与异名称冲突 验证 HTTP 入口确实纳入名称身份。
func Test模板保存原键重放与异名称冲突(t *testing.T) {
	source := store.TaskDerivationSource{TaskID: "synthetic-done", State: "SUCCEEDED", SnapshotVersion: "v2", SnapshotJSON: `{"config":{"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/synthetic/output"}}}`, ConfigFingerprint: strings.Repeat("a", 64), CapabilityVersion: "synthetic-capability"}
	templates := &replayTemplateStore{recordingTemplateStore: newRecordingTemplateStore()}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: sliceAuthorizer{}, CSRF: allowedCSRF{}, Tasks: &recordingTaskStore{derivationSource: source}, Templates: templates})
	save := func(name string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"displayName": name})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/synthetic-done:save-template", bytes.NewReader(body))
		request.Header.Set("Idempotency-Key", "synthetic-stable-template-key")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := save("合成名称一"); response.Code != 201 {
		t.Fatalf("首写状态: %d", response.Code)
	}
	if response := save("合成名称一"); response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte(`"replayed":true`)) {
		t.Fatalf("未稳定重放: %d", response.Code)
	}
	if response := save("合成名称二"); response.Code != 409 {
		t.Fatalf("异名称未冲突: %d", response.Code)
	}
}

// Test模板派生请求身份包含全部绑定且无分隔符碰撞 验证替换引用不能冒充旧请求。
func Test模板派生请求身份包含全部绑定且无分隔符碰撞(t *testing.T) {
	original := templateWriteDigest("CREATE_DRAFT", "template", "source", "node")
	if original != templateWriteDigest("CREATE_DRAFT", "template", "source", "node") {
		t.Fatal("同一请求身份不稳定")
	}
	for _, other := range []string{
		templateWriteDigest("CREATE_DRAFT", "template", "source-other", "node"),
		templateWriteDigest("CREATE_DRAFT", "template", "source", "node-other"),
		templateWriteDigest("CREATE_DRAFT", "template-other", "source", "node"),
		templateWriteDigest("SAVE_TEMPLATE", "template", "source", "node"),
	} {
		if original == other {
			t.Fatal("不同业务请求共用摘要")
		}
	}
	if templateWriteDigest("CREATE_DRAFT", "template", "a/b", "c") == templateWriteDigest("CREATE_DRAFT", "template", "a", "b/c") {
		t.Fatal("输入分隔符碰撞")
	}
}
