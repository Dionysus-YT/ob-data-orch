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
	"ob-data-orch/internal/securitypolicy"
)

type securityHTTPReader struct {
	item      securitypolicy.Snapshot
	err       error
	calls     int
	afterRead func()
}

type securityHTTPAuthorizer struct{ allowed bool }

func (s *securityHTTPAuthorizer) Authorize(_ context.Context, p identity.Principal, scope identity.Scope, object string) error {
	if s.allowed && p.Type == identity.BrowserPrincipal && scope == identity.ScopeSecurityPolicyRead && object == "platform" {
		return nil
	}
	return identity.ErrDenied
}

func (s *securityHTTPReader) Read(context.Context) (securitypolicy.Snapshot, error) {
	s.calls++
	if s.afterRead != nil {
		s.afterRead()
	}
	return s.item, s.err
}

func Test安全状态只读接口不接收查询正文或写操作(t *testing.T) {
	service, _ := securitypolicy.New(securitypolicy.Startup{LoadedAt: time.Now().Add(-time.Second), CredentialKeyID: "synthetic-key-v1"}, func() bool { return true })
	item, _ := service.Read(context.Background())
	reader := &securityHTTPReader{item: item}
	auth := &securityHTTPAuthorizer{allowed: true}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, SecurityPolicy: reader})
	response := permissionRequest(handler, "GET", "/api/v1/settings/security", "")
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"rootKeyStatus":"LOADED"`) || strings.Contains(response.Body.String(), "ciphertext") {
		t.Fatalf("读取=%d", response.Code)
	}
	before := reader.calls
	for _, path := range []string{"/api/v1/settings/security?path=synthetic", "/api/v1/settings/security?" + "scope=all"} {
		if r := permissionRequest(handler, "GET", path, ""); r.Code != 400 {
			t.Fatalf("非法查询=%d", r.Code)
		}
	}
	if r := permissionRequest(handler, "GET", "/api/v1/settings/security", `{}`); r.Code != 400 {
		t.Fatal("GET 正文未阻断")
	}
	unknownLength := httptest.NewRequest("GET", "/api/v1/settings/security", strings.NewReader(`{}`))
	unknownLength.ContentLength = -1
	unknownResponse := httptest.NewRecorder()
	handler.ServeHTTP(unknownResponse, unknownLength)
	if unknownResponse.Code != 400 {
		t.Fatal("未知长度的 GET 正文未阻断")
	}
	for _, method := range []string{"POST", "PATCH", "DELETE"} {
		if r := permissionRequest(handler, method, "/api/v1/settings/security", `{}`); r.Code != 404 {
			t.Fatalf("写操作=%d", r.Code)
		}
	}
	if reader.calls != before {
		t.Fatal("非法请求读取安全状态")
	}
}

func Test安全状态无权先拒绝并在响应前复验(t *testing.T) {
	auth := &securityHTTPAuthorizer{}
	reader := &securityHTTPReader{err: errors.New("synthetic-only-private-detail")}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, SecurityPolicy: reader})
	if r := permissionRequest(handler, "GET", "/api/v1/settings/security", ""); r.Code != 404 || reader.calls != 0 {
		t.Fatal("无权请求进入状态读取")
	}
	auth.allowed = true
	r := permissionRequest(handler, "GET", "/api/v1/settings/security", "")
	if r.Code != 503 || strings.Contains(r.Body.String(), "synthetic-only-private-detail") {
		t.Fatal("底层错误未安全遮蔽")
	}
	reader.afterRead = func() { auth.allowed = false }
	if r := permissionRequest(handler, "GET", "/api/v1/settings/security", ""); r.Code != 404 {
		t.Fatal("读取期间撤权未阻断")
	}
	auth.allowed = true
	missing := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth})
	if r := permissionRequest(missing, "GET", "/api/v1/settings/security", ""); r.Code != 503 {
		t.Fatal("缺失服务被默认值替代")
	}
}
