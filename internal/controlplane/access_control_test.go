package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

type permissionProvider struct{ subject string }

func (p permissionProvider) AuthenticateBrowser(*http.Request) (identity.Principal, error) {
	return identity.Principal{Type: identity.BrowserPrincipal, ID: p.subject}, nil
}
func (permissionProvider) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthenticated
}

type permissionAuthorizer struct{ allowed bool }

type rejectedPermissionCSRF struct{}

func (rejectedPermissionCSRF) ValidateCSRF(*http.Request) error { return identity.ErrDenied }

func (a *permissionAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, id string) error {
	if a.allowed && scope == identity.ScopePermissionManage && id == "platform" {
		return nil
	}
	return identity.ErrDenied
}

type permissionPort struct {
	reads   int
	changed accesscontrol.Change
	err     error
	onRead  func()
	more    bool
}

func (p *permissionPort) ListPermissionSubjects(context.Context, string, accesscontrol.SubjectQuery) (accesscontrol.SubjectPage, error) {
	p.reads++
	if p.onRead != nil {
		p.onRead()
	}
	return accesscontrol.SubjectPage{Items: []accesscontrol.Profile{permissionTestProfile()}, HasMore: p.more}, p.err
}
func (p *permissionPort) ReadPermissionProfile(context.Context, string, string) (accesscontrol.Profile, error) {
	p.reads++
	return permissionTestProfile(), p.err
}
func (p *permissionPort) ListPermissionCandidates(context.Context, string, accesscontrol.ScopeType, string, string, int) ([]accesscontrol.Candidate, bool, error) {
	p.reads++
	return []accesscontrol.Candidate{{ID: "source-synthetic", DisplayName: "合成数据源", Kind: "DATA_SOURCE", Available: true}}, false, p.err
}
func (p *permissionPort) ChangeSubjectPermissions(_ context.Context, c accesscontrol.Change) (accesscontrol.ChangeResult, error) {
	p.changed = c
	return accesscontrol.ChangeResult{Revision: 2, Changed: true}, p.err
}
func permissionTestProfile() accesscontrol.Profile {
	return accesscontrol.Profile{Subject: accesscontrol.Subject{ID: "permission-user", Login: "synthetic-user", DisplayName: "合成用户", AccountStatus: "ACTIVE"}, Assignment: accesscontrol.Canonical(accesscontrol.Assignment{}), Revision: 1, Editable: true, ScopeObjects: []accesscontrol.ScopeObject{}}
}

func permissionTestHandler(port *permissionPort, authorizer *permissionAuthorizer) http.Handler {
	return NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: authorizer, AccessControl: port, CSRF: allowedCSRF{}})
}
func permissionRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("If-Match", `"rev-1"`)
	request.Header.Set("Idempotency-Key", "permission-synthetic-request")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func Test权限接口先鉴权不枚举主体和授权候选(t *testing.T) {
	port := &permissionPort{}
	a := &permissionAuthorizer{}
	handler := permissionTestHandler(port, a)
	for _, path := range []string{"/api/v1/access-control/subjects", "/api/v1/access-control/subjects/permission-user", "/api/v1/access-control/candidates?type=DATA_SOURCE_USE"} {
		r := permissionRequest(handler, "GET", path, "")
		if r.Code != 404 || port.reads != 0 || strings.Contains(r.Body.String(), "synthetic-user") {
			t.Fatal("鉴权前读取或泄露目录", r.Code, port.reads)
		}
	}
	a.allowed = true
	r := permissionRequest(handler, "GET", "/api/v1/access-control/subjects", "")
	if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(r.Code, r.Body.String())
	}
	port.onRead = func() { a.allowed = false }
	r = permissionRequest(handler, "GET", "/api/v1/access-control/subjects", "")
	if r.Code != 404 || strings.Contains(r.Body.String(), "synthetic-user") {
		t.Fatal("读取期间撤权仍返回旧事实")
	}
}

func Test权限接口拒绝未知重复参数和跨条件游标(t *testing.T) {
	port := &permissionPort{more: true}
	handler := permissionTestHandler(port, &permissionAuthorizer{allowed: true})
	for _, query := range []string{"subjectId=other", "role=ROLE_OPERATOR&role=ROLE_BASIC_USER", "status=INVALID", "capability=constructor", "role=constructor", "limit=51", "cursor=broken", "search=%zz"} {
		r := permissionRequest(handler, "GET", "/api/v1/access-control/subjects?"+query, "")
		if r.Code != 400 {
			t.Fatalf("非法查询未拒绝 %s %d", query, r.Code)
		}
	}
	r := permissionRequest(handler, "GET", "/api/v1/access-control/subjects?limit=1", "")
	var page map[string]any
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &page) != nil {
		t.Fatal(r.Code, r.Body.String())
	}
	cursor, ok := page["nextCursor"].(string)
	if !ok || cursor == "" {
		t.Fatal("分页未产生不透明游标")
	}
	if r = permissionRequest(handler, "GET", "/api/v1/access-control/subjects?limit=2&cursor="+cursor, ""); r.Code != 400 {
		t.Fatal("游标未绑定筛选", r.Code)
	}
	if r = permissionRequest(handler, "GET", "/api/v1/access-control/subjects?limit=1&cursor="+cursor, ""); r.Code != 200 {
		t.Fatal("同条件游标不可用", r.Code)
	}
	if r = permissionRequest(handler, "GET", "/api/v1/access-control/candidates?type=NODE_USE&limit=1&cursor="+cursor, ""); r.Code != 400 {
		t.Fatal("跨目录复用了游标", r.Code)
	}
}

func Test权限写入只接受完整配置并绑定可信操作者(t *testing.T) {
	port := &permissionPort{}
	handler := permissionTestHandler(port, &permissionAuthorizer{allowed: true})
	valid := `{"roles":["ROLE_BASIC_USER"],"capabilities":[],"scopes":[],"reason":"调整合成职责","confirmed":true}`
	for _, body := range []string{`{}`, `{"roles":null,"capabilities":[],"scopes":[],"reason":"调整","confirmed":false}`, strings.TrimSuffix(valid, "}") + `,"actorId":"other"}`, valid + `{}`, strings.Replace(valid, `"scopes":[]`, `"scopes":[{"type":"NODE_USE","objectId":"node-1","token":"synthetic"}]`, 1)} {
		if r := permissionRequest(handler, "PATCH", "/api/v1/access-control/subjects/permission-user", body); r.Code != 400 {
			t.Fatal("不完整或未知字段未拒绝", r.Code)
		}
	}
	r := permissionRequest(handler, "PATCH", "/api/v1/access-control/subjects/permission-user", valid)
	if r.Code != 200 || port.changed.ActorID != "subject-1" || port.changed.SubjectID != "permission-user" || port.changed.ExpectedRevision != 1 || port.changed.RequestID == "" || port.changed.IdempotencyKey != "permission-synthetic-request" {
		t.Fatal("写入边界丢失", r.Code, port.changed)
	}
	request := httptest.NewRequest("PATCH", "/api/v1/access-control/subjects/permission-user", strings.NewReader(valid))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 412 {
		t.Fatal("缺少版本仍写入", response.Code)
	}
}

func Test权限写入错误安全投影不展示内部原因(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{store.ErrPermissionDenied, 404}, {store.ErrPermissionSelfReadOnly, 403}, {store.ErrRevisionConflict, 412}, {store.ErrIdempotencyConflict, 409}, {store.ErrPermissionConfirmation, 422}, {store.ErrPermissionAdditionGated, 422}, {accesscontrol.ErrInactiveAddition, 409}, {accesscontrol.ErrInvalid, 400}, {errors.New("synthetic-internal-fault"), 503}} {
		port := &permissionPort{err: test.err}
		r := permissionRequest(permissionTestHandler(port, &permissionAuthorizer{allowed: true}), "PATCH", "/api/v1/access-control/subjects/permission-user", `{"roles":[],"capabilities":[],"scopes":[],"reason":"调整合成职责","confirmed":false}`)
		if r.Code != test.status || strings.Contains(r.Body.String(), "synthetic-internal-fault") {
			t.Fatal("错误映射不安全", test.err, r.Code, r.Body.String())
		}
	}
}

func Test权限写入必须经过当前会话CSRF校验(t *testing.T) {
	port := &permissionPort{}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: &permissionAuthorizer{allowed: true}, AccessControl: port, CSRF: rejectedPermissionCSRF{}})
	response := permissionRequest(handler, "PATCH", "/api/v1/access-control/subjects/permission-user", `{"roles":[],"capabilities":[],"scopes":[],"reason":"调整合成职责","confirmed":false}`)
	if response.Code != 401 || port.changed.ActorID != "" {
		t.Fatal("CSRF 失败进入权限事务", response.Code)
	}
}
