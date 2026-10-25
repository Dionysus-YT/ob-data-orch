package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/schedulingpolicy"
	"ob-data-orch/internal/store"
)

type timingHTTPRepository struct {
	reads, writes int
	change        schedulingpolicy.TimingChange
	err           error
	onRead        func()
	raw           schedulingpolicy.TimingOverrides
}

func (p *timingHTTPRepository) ReadCurrentSchedulingTiming(_ context.Context, baseline time.Duration) (schedulingpolicy.TimingPolicy, error) {
	if p.err != nil {
		return schedulingpolicy.TimingPolicy{}, p.err
	}
	raw := p.raw
	if raw.Revision == 0 {
		raw.Revision = 1
	}
	return schedulingpolicy.ResolveTiming(raw, baseline)
}
func (p *timingHTTPRepository) ReadAuthorizedSchedulingTiming(ctx context.Context, _ string, baseline time.Duration) (schedulingpolicy.TimingPolicy, error) {
	p.reads++
	if p.onRead != nil {
		p.onRead()
	}
	return p.ReadCurrentSchedulingTiming(ctx, baseline)
}
func (p *timingHTTPRepository) ChangeSchedulingTiming(_ context.Context, change schedulingpolicy.TimingChange) (schedulingpolicy.Receipt, error) {
	p.writes++
	p.change = change
	return schedulingpolicy.Receipt{Revision: 2, Changed: true}, p.err
}
func timingHTTPHandler(repo *timingHTTPRepository, auth *schedulingHTTPAuthorizer, csrf CSRFValidator) http.Handler {
	return NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, CSRF: csrf, SchedulingTiming: repo, HeartbeatTTL: 150 * time.Second})
}

func Test时间设置接口先鉴权且返回真实基线(t *testing.T) {
	repo := &timingHTTPRepository{}
	auth := &schedulingHTTPAuthorizer{}
	handler := timingHTTPHandler(repo, auth, allowedCSRF{})
	for _, method := range []string{"GET", "PATCH"} {
		if r := permissionRequest(handler, method, "/api/v1/settings/scheduling/timing", `{}`); r.Code != 404 {
			t.Fatalf("未授权=%d", r.Code)
		}
	}
	if repo.reads != 0 || repo.writes != 0 {
		t.Fatal("授权前读取或保存")
	}
	auth.allowed = true
	r := permissionRequest(handler, "GET", "/api/v1/settings/scheduling/timing", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"heartbeatTimeoutSeconds":150`) || !strings.Contains(r.Body.String(), `"startupTimeoutSeconds":null`) || !strings.Contains(r.Body.String(), `"pollingIntervalSeconds":null`) || r.Header().Get("ETag") != `"rev-1"` || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("时间投影=%d %s", r.Code, r.Body.String())
	}
	repo.onRead = func() { auth.allowed = false }
	if r = permissionRequest(handler, "GET", "/api/v1/settings/scheduling/timing", ""); r.Code != 404 {
		t.Fatalf("读取期间撤权=%d", r.Code)
	}
}

func Test时间设置接口空字段严格边界及写安全(t *testing.T) {
	repo := &timingHTTPRepository{}
	auth := &schedulingHTTPAuthorizer{allowed: true}
	handler := timingHTTPHandler(repo, auth, allowedCSRF{})
	for _, body := range []string{`{}`, `{"heartbeatTimeoutSeconds":120,"reason":"合成原因","confirmed":false}`, `{"heartbeatTimeoutSeconds":120,"startupTimeoutSeconds":15.5,"pollingIntervalSeconds":null,"reason":"合成原因","confirmed":true}`, `{"heartbeatTimeoutSeconds":120,"startupTimeoutSeconds":null,"pollingIntervalSeconds":null,"reason":"合成原因","confirmed":true,"candidateStrategy":"AUTO"}`, `{"heartbeatTimeoutSeconds":120,"startupTimeoutSeconds":null,"pollingIntervalSeconds":null,"reason":"合成原因","confirmed":true} {}`} {
		if r := permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling/timing", body); r.Code != 400 {
			t.Fatalf("严格正文=%d", r.Code)
		}
	}
	if repo.writes != 0 {
		t.Fatal("非法正文进入仓储")
	}
	valid := `{"heartbeatTimeoutSeconds":120,"startupTimeoutSeconds":null,"pollingIntervalSeconds":null,"reason":"合成原因","confirmed":true}`
	if r := permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling/timing", valid); r.Code != 200 || repo.change.ActorID != "subject-1" || repo.change.BaselineHeartbeatTimeout != 150*time.Second || repo.change.ExpectedRevision != 1 || repo.change.StartupTimeoutSeconds != nil || repo.change.PollingIntervalSeconds != nil || repo.change.At.IsZero() {
		t.Fatalf("写入元信息=%d %+v", r.Code, repo.change)
	}
	for _, csrf := range []CSRFValidator{nil, rejectedPermissionCSRF{}} {
		before := repo.writes
		if r := permissionRequest(timingHTTPHandler(repo, auth, csrf), "PATCH", "/api/v1/settings/scheduling/timing", valid); r.Code < 400 || repo.writes != before {
			t.Fatal("CSRF 未阻断")
		}
	}
	request := httptest.NewRequest("PATCH", "/api/v1/settings/scheduling/timing", strings.NewReader(valid))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 412 {
		t.Fatalf("缺版本=%d", response.Code)
	}
	for _, item := range []struct {
		err    error
		status int
	}{{schedulingpolicy.ErrInvalid, 400}, {schedulingpolicy.ErrRevision, 412}, {schedulingpolicy.ErrConfirmation, 422}, {schedulingpolicy.ErrIdempotency, 409}, {schedulingpolicy.ErrDenied, 404}, {schedulingpolicy.ErrUnavailable, 503}} {
		repo.err = item.err
		if r := permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling/timing", valid); r.Code != item.status {
			t.Fatalf("错误映射=%d", r.Code)
		}
	}
}

func Test节点投影读取当前时间策略且不可用不回退(t *testing.T) {
	now := time.Now().UTC()
	stamp := now.Add(-90 * time.Second)
	heartbeat := 60
	repo := &timingHTTPRepository{raw: schedulingpolicy.TimingOverrides{Revision: 2, HeartbeatSeconds: &heartbeat, ModifiedBy: "synthetic-admin", ModifiedAt: &now}}
	server := &Server{heartbeatTTL: 2 * time.Minute, schedulingTiming: repo}
	node := store.ExecutionNode{NodeID: "synthetic-node", ManagementState: "ENABLED", Agent: &store.ExecutionNodeAgent{LastHeartbeatAt: &stamp, CapacityTotal: 1}}
	ttl, err := server.currentHeartbeatTimeout(context.Background())
	if err != nil || server.newExecutionNodeListResponse(node, ttl).HeartbeatStatus != "OFFLINE" {
		t.Fatal("节点投影仍使用旧启动阈值")
	}
	heartbeat = 180
	ttl, err = server.currentHeartbeatTimeout(context.Background())
	if err != nil || server.newExecutionNodeListResponse(node, ttl).HeartbeatStatus != "ONLINE" {
		t.Fatal("提高阈值未生效")
	}
	repo.err = schedulingpolicy.ErrUnavailable
	if _, err = server.currentHeartbeatTimeout(context.Background()); err == nil {
		t.Fatal("策略不可用却回退启动值")
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/execution-nodes", nil)
	if _, ok := server.heartbeatTimeoutForRequest(response, request); ok || response.Code != 503 {
		t.Fatal("策略故障没有阻断资格读取")
	}
}
