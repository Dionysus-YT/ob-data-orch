package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/schedulingpolicy"
)

type schedulingHTTPRepository struct {
	reads, writes int
	change        schedulingpolicy.Change
	err           error
	onRead        func()
	usage         schedulingpolicy.Usage
}

func (p *schedulingHTTPRepository) ReadAuthorizedSchedulingPolicy(context.Context, string) (schedulingpolicy.Policy, error) {
	p.reads++
	if p.onRead != nil {
		p.onRead()
	}
	usage := p.usage
	if usage.Visibility == "" {
		usage = schedulingpolicy.Usage{Visibility: "RESTRICTED", ObservedAt: time.Now().UTC()}
	}
	return schedulingpolicy.Policy{Revision: 1, Source: "UNCONFIGURED", ApplicationMode: "NEXT_CLAIM", Usage: usage}, p.err
}
func (p *schedulingHTTPRepository) ChangeSchedulingPolicy(_ context.Context, c schedulingpolicy.Change) (schedulingpolicy.Receipt, error) {
	p.writes++
	p.change = c
	return schedulingpolicy.Receipt{Revision: 2, Changed: true}, p.err
}

type schedulingHTTPAuthorizer struct{ allowed, globalRead bool }

func (a *schedulingHTTPAuthorizer) Authorize(_ context.Context, p identity.Principal, scope identity.Scope, id string) error {
	if a.allowed && a.globalRead && p.Type == identity.BrowserPrincipal && scope == identity.ScopeTaskReadAll && id == "*" {
		return nil
	}
	if a.allowed && p.Type == identity.BrowserPrincipal && id == "platform" && (scope == identity.ScopeSchedulingPolicyRead || scope == identity.ScopeSchedulingPolicyWrite) {
		return nil
	}
	return identity.ErrDenied
}
func Test调度策略响应前独立复验全局数量权限(t *testing.T) {
	held, running := 3, 2
	repo := &schedulingHTTPRepository{usage: schedulingpolicy.Usage{Visibility: "AVAILABLE", HeldExecutions: &held, RunningExecutions: &running, ObservedAt: time.Now().UTC()}}
	auth := &schedulingHTTPAuthorizer{allowed: true, globalRead: true}
	handler := schedulingHTTPHandler(repo, auth, allowedCSRF{})
	if r := permissionRequest(handler, "GET", "/api/v1/settings/scheduling", ""); r.Code != 200 || !strings.Contains(r.Body.String(), `"heldExecutions":3`) {
		t.Fatalf("全局授权读取=%d %s", r.Code, r.Body.String())
	}
	repo.onRead = func() { auth.globalRead = false }
	r := permissionRequest(handler, "GET", "/api/v1/settings/scheduling", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"visibility":"RESTRICTED"`) || !strings.Contains(r.Body.String(), `"heldExecutions":null`) || !strings.Contains(r.Body.String(), `"runningExecutions":null`) {
		t.Fatalf("读取期间撤销全局权限=%d %s", r.Code, r.Body.String())
	}
}
func schedulingHTTPHandler(repo *schedulingHTTPRepository, auth *schedulingHTTPAuthorizer, csrf CSRFValidator) http.Handler {
	policy := repo
	return NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, CSRF: csrf, SchedulingPolicy: policy})
}
func Test调度策略接口鉴权隔离与安全投影(t *testing.T) {
	repo := &schedulingHTTPRepository{}
	auth := &schedulingHTTPAuthorizer{}
	handler := schedulingHTTPHandler(repo, auth, allowedCSRF{})
	for _, method := range []string{"GET", "PATCH"} {
		if r := permissionRequest(handler, method, "/api/v1/settings/scheduling", `{}`); r.Code != 404 {
			t.Fatalf("无权=%d", r.Code)
		}
	}
	if repo.reads != 0 || repo.writes != 0 {
		t.Fatal("鉴权前访问配置")
	}
	auth.allowed = true
	r := permissionRequest(handler, "GET", "/api/v1/settings/scheduling", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"maxConcurrentExecutions":null`) || !strings.Contains(r.Body.String(), `"source":"UNCONFIGURED"`) || r.Header().Get("ETag") != `"rev-1"` || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("投影=%d %s", r.Code, r.Body.String())
	}
	for _, unsafe := range []string{"passwordHash", "Password", "settings.json", "data-dir"} {
		if strings.Contains(r.Body.String(), unsafe) {
			t.Fatal("泄露配置")
		}
	}
	repo.onRead = func() { auth.allowed = false }
	if r = permissionRequest(handler, "GET", "/api/v1/settings/scheduling", ""); r.Code != 404 {
		t.Fatalf("响应前撤权=%d", r.Code)
	}
}
func Test调度策略接口严格输入及安全写入(t *testing.T) {
	repo := &schedulingHTTPRepository{}
	auth := &schedulingHTTPAuthorizer{allowed: true}
	handler := schedulingHTTPHandler(repo, auth, allowedCSRF{})
	for _, body := range []string{`{}`, `{"maxConcurrentExecutions":7,"reason":"合成原因"}`, `{"maxConcurrentExecutions":7.5,"reason":"合成原因","confirmed":true}`, `{"maxConcurrentExecutions":7,"reason":"合成原因","confirmed":true,"actorId":"other"}`, `{"maxConcurrentExecutions":7,"reason":"合成原因","confirmed":true} {}`} {
		if r := permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling", body); r.Code != 400 {
			t.Fatalf("非法正文=%d", r.Code)
		}
	}
	if repo.writes != 0 {
		t.Fatal("不完整正文进入仓储")
	}
	valid := `{"maxConcurrentExecutions":7,"reason":"合成原因","confirmed":true}`
	r := permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling", valid)
	if r.Code != 200 || repo.change.ActorID != "subject-1" || repo.change.ExpectedRevision != 1 || repo.change.RequestID == "" || repo.change.At.IsZero() {
		t.Fatalf("写入=%d %+v", r.Code, repo.change)
	}
	for _, csrf := range []CSRFValidator{nil, rejectedPermissionCSRF{}} {
		before := repo.writes
		if r = permissionRequest(schedulingHTTPHandler(repo, auth, csrf), "PATCH", "/api/v1/settings/scheduling", valid); r.Code < 400 || repo.writes != before {
			t.Fatalf("CSRF未阻断=%d", r.Code)
		}
	}
	request := httptest.NewRequest("PATCH", "/api/v1/settings/scheduling", strings.NewReader(valid))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 412 {
		t.Fatalf("缺版本=%d", response.Code)
	}
	for _, item := range []struct {
		err    error
		status int
	}{{schedulingpolicy.ErrRevision, 412}, {schedulingpolicy.ErrIdempotency, 409}, {schedulingpolicy.ErrConfirmation, 422}, {schedulingpolicy.ErrInvalid, 400}, {schedulingpolicy.ErrDenied, 404}, {schedulingpolicy.ErrUnavailable, 503}} {
		repo.err = item.err
		if r = permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling", valid); r.Code != item.status {
			t.Fatalf("错误映射=%d want%d", r.Code, item.status)
		}
	}
}
