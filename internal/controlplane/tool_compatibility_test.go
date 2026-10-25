package controlplane

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/toolcompatibility"
)

type toolStatusReader struct {
	item      toolcompatibility.Snapshot
	err       error
	calls     int
	afterRead func()
}

func (s *toolStatusReader) Read(context.Context) (toolcompatibility.Snapshot, error) {
	s.calls++
	if s.afterRead != nil {
		s.afterRead()
	}
	return s.item, s.err
}

type toolStatusAuthorizer struct{ allowed bool }

func (s *toolStatusAuthorizer) Authorize(_ context.Context, p identity.Principal, scope identity.Scope, object string) error {
	if s.allowed && p.Type == identity.BrowserPrincipal && scope == identity.ScopeToolCompatibilityRead && object == "platform" {
		return nil
	}
	return identity.ErrDenied
}

// Test工具状态只读不接受查询正文或写操作 验证固定读取端点没有额外输入或写入能力。
func Test工具状态只读不接受查询正文或写操作(t *testing.T) {
	service, _ := toolcompatibility.New(time.Now().Add(-time.Second))
	item, _ := service.Read(context.Background())
	reader := &toolStatusReader{item: item}
	auth := &toolStatusAuthorizer{allowed: true}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, ToolCompatibility: reader})
	r := permissionRequest(handler, "GET", "/api/v1/settings/tools", "")
	if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" || !strings.Contains(r.Body.String(), `"metadataSelection":"TASK_CONFIGURATION"`) || strings.Contains(r.Body.String(), "installedVersion") {
		t.Fatal("工具投影或缓存边界错误")
	}
	before := reader.calls
	for _, input := range []struct{ method, path, body string }{{"GET", "/api/v1/settings/tools?path=synthetic", ""}, {"GET", "/api/v1/settings/tools", `{}`}} {
		if r := permissionRequest(handler, input.method, input.path, input.body); r.Code != 400 {
			t.Fatal("额外读取字段未拒绝")
		}
	}
	unknown := httptest.NewRequest("GET", "/api/v1/settings/tools", strings.NewReader(`{}`))
	unknown.ContentLength = -1
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, unknown)
	if w.Code != 400 {
		t.Fatal("未知长度 GET 正文未拒绝")
	}
	for _, method := range []string{"POST", "PATCH", "DELETE"} {
		if r := permissionRequest(handler, method, "/api/v1/settings/tools", `{}`); r.Code != 404 {
			t.Fatal("写请求被放行")
		}
	}
	if before != reader.calls {
		t.Fatal("非法请求读取了目录")
	}
}

// Test工具状态先授权并在成功或失败响应前复验 验证无权先拒绝及撤权后的错误遮蔽。
func Test工具状态先授权并在成功或失败响应前复验(t *testing.T) {
	auth := &toolStatusAuthorizer{}
	reader := &toolStatusReader{err: errors.New("synthetic-private-catalog-path")}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, ToolCompatibility: reader})
	if r := permissionRequest(handler, "GET", "/api/v1/settings/tools", ""); r.Code != 404 || reader.calls != 0 {
		t.Fatal("无权读取进入领域服务")
	}
	auth.allowed = true
	if r := permissionRequest(handler, "GET", "/api/v1/settings/tools", ""); r.Code != 503 || strings.Contains(r.Body.String(), "synthetic-private-catalog-path") {
		t.Fatal("依赖错误未遮蔽")
	}
	reader.afterRead = func() { auth.allowed = false }
	for _, err := range []error{nil, errors.New("synthetic-private-catalog-path")} {
		auth.allowed, reader.err = true, err
		if r := permissionRequest(handler, "GET", "/api/v1/settings/tools", ""); r.Code != 404 || strings.Contains(r.Body.String(), "synthetic-private-catalog-path") {
			t.Fatal("撤权后仍有响应差异")
		}
	}
	auth.allowed = true
	missing := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth})
	if r := permissionRequest(missing, "GET", "/api/v1/settings/tools", ""); r.Code != 503 {
		t.Fatal("缺失目录依赖返回猜测事实")
	}
}
