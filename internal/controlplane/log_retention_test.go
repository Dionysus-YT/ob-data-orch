package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logretention"
)

type retentionHTTPRepository struct {
	reads, writes int
	change        logretention.Change
	err           error
	onRead        func()
}

func (p *retentionHTTPRepository) ReadAuthorizedLogRetention(context.Context, string) (logretention.Record, error) {
	p.reads++
	if p.onRead != nil {
		p.onRead()
	}
	return logretention.Record{Revision: 1}, p.err
}
func (p *retentionHTTPRepository) ReadCurrentLogRetention(context.Context) (logretention.Record, error) {
	return logretention.Record{Revision: 1}, p.err
}
func (p *retentionHTTPRepository) ChangeLogRetention(_ context.Context, c logretention.Change, _ int) (logretention.Receipt, error) {
	p.writes++
	p.change = c
	return logretention.Receipt{Revision: 2, Changed: true}, p.err
}

type retentionHTTPAuthorizer struct{ allowed bool }

func (a *retentionHTTPAuthorizer) Authorize(_ context.Context, p identity.Principal, scope identity.Scope, id string) error {
	if a.allowed && p.Type == identity.BrowserPrincipal && id == "platform" && (scope == identity.ScopeLogRetentionRead || scope == identity.ScopeLogRetentionWrite) {
		return nil
	}
	return identity.ErrDenied
}
func retentionHTTPHandler(repo *retentionHTTPRepository, auth *retentionHTTPAuthorizer, csrf CSRFValidator) http.Handler {
	policy, _ := logretention.New(repo, 0)
	return NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, CSRF: csrf, LogRetention: policy})
}
func Test普通日志接口鉴权隔离与安全投影(t *testing.T) {
	repo := &retentionHTTPRepository{}
	auth := &retentionHTTPAuthorizer{}
	handler := retentionHTTPHandler(repo, auth, allowedCSRF{})
	for _, method := range []string{"GET", "PATCH"} {
		if r := permissionRequest(handler, method, "/api/v1/settings/log-retention", `{}`); r.Code != 404 {
			t.Fatalf("无权=%d", r.Code)
		}
	}
	if repo.reads != 0 || repo.writes != 0 {
		t.Fatal("鉴权前访问配置")
	}
	auth.allowed = true
	r := permissionRequest(handler, "GET", "/api/v1/settings/log-retention", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"retentionDays":30`) || !strings.Contains(r.Body.String(), `"source":"BUILT_IN_DEFAULT"`) || r.Header().Get("ETag") != `"rev-1"` || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("投影=%d %s", r.Code, r.Body.String())
	}
	for _, unsafe := range []string{"passwordHash", "Password", "settings.json", "data-dir"} {
		if strings.Contains(r.Body.String(), unsafe) {
			t.Fatal("泄露配置")
		}
	}
	repo.onRead = func() { auth.allowed = false }
	if r = permissionRequest(handler, "GET", "/api/v1/settings/log-retention", ""); r.Code != 404 {
		t.Fatalf("响应前撤权=%d", r.Code)
	}
}
func Test普通日志接口严格输入及安全写入(t *testing.T) {
	repo := &retentionHTTPRepository{}
	auth := &retentionHTTPAuthorizer{true}
	handler := retentionHTTPHandler(repo, auth, allowedCSRF{})
	for _, body := range []string{`{}`, `{"retentionDays":7,"reason":"合成原因"}`, `{"retentionDays":7.5,"reason":"合成原因","confirmed":true}`, `{"retentionDays":7,"reason":"合成原因","confirmed":true,"actorId":"other"}`, `{"retentionDays":7,"reason":"合成原因","confirmed":true} {}`} {
		if r := permissionRequest(handler, "PATCH", "/api/v1/settings/log-retention", body); r.Code != 400 {
			t.Fatalf("非法正文=%d", r.Code)
		}
	}
	if repo.writes != 0 {
		t.Fatal("不完整正文进入仓储")
	}
	valid := `{"retentionDays":7,"reason":"合成原因","confirmed":true}`
	r := permissionRequest(handler, "PATCH", "/api/v1/settings/log-retention", valid)
	if r.Code != 200 || repo.change.ActorID != "subject-1" || repo.change.ExpectedRevision != 1 || repo.change.RequestID == "" || repo.change.At.IsZero() {
		t.Fatalf("写入=%d %+v", r.Code, repo.change)
	}
	for _, csrf := range []CSRFValidator{nil, rejectedPermissionCSRF{}} {
		before := repo.writes
		if r = permissionRequest(retentionHTTPHandler(repo, auth, csrf), "PATCH", "/api/v1/settings/log-retention", valid); r.Code < 400 || repo.writes != before {
			t.Fatalf("CSRF未阻断=%d", r.Code)
		}
	}
	request := httptest.NewRequest("PATCH", "/api/v1/settings/log-retention", strings.NewReader(valid))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 412 {
		t.Fatalf("缺版本=%d", response.Code)
	}
	for _, item := range []struct {
		err    error
		status int
	}{{logretention.ErrRevision, 412}, {logretention.ErrIdempotency, 409}, {logretention.ErrConfirmation, 422}, {logretention.ErrInvalid, 400}, {logretention.ErrDenied, 404}, {logretention.ErrUnavailable, 503}} {
		repo.err = item.err
		if r = permissionRequest(handler, "PATCH", "/api/v1/settings/log-retention", valid); r.Code != item.status {
			t.Fatalf("错误映射=%d want%d", r.Code, item.status)
		}
	}
}
