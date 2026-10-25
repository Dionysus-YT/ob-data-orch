package controlplane

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/schedulingpolicy"
	"ob-data-orch/internal/store"
)

type candidateHTTPRepository struct {
	policy    schedulingpolicy.CandidatePolicy
	change    schedulingpolicy.CandidateChange
	writes    int
	err       error
	afterRead func()
}

func (v *candidateHTTPRepository) ReadCurrentCandidatePolicy(context.Context) (schedulingpolicy.CandidatePolicy, error) {
	return v.policy, v.err
}
func (v *candidateHTTPRepository) ReadAuthorizedCandidatePolicy(context.Context, string) (schedulingpolicy.CandidatePolicy, error) {
	if v.afterRead != nil {
		v.afterRead()
	}
	return v.policy, v.err
}
func (v *candidateHTTPRepository) ChangeCandidatePolicy(_ context.Context, change schedulingpolicy.CandidateChange) (schedulingpolicy.Receipt, error) {
	v.change = change
	v.writes++
	return schedulingpolicy.Receipt{Revision: 2, Changed: true}, v.err
}
func candidateHTTPPolicy() schedulingpolicy.CandidatePolicy {
	return schedulingpolicy.CandidatePolicy{Order: schedulingpolicy.ExistingOrder, Revision: 1, SelectionMode: "EXPLICIT_SELECTION", ApplicationMode: "NEXT_CANDIDATE_READ"}
}

func Test候选设置严格接口与响应前撤权(t *testing.T) {
	repo := &candidateHTTPRepository{policy: candidateHTTPPolicy()}
	auth := &schedulingHTTPAuthorizer{allowed: true}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, CSRF: allowedCSRF{}, SchedulingCandidates: repo})
	r := permissionRequest(handler, "GET", "/api/v1/settings/scheduling/candidates", "")
	if r.Code != 200 || r.Header().Get("ETag") != `"rev-1"` || r.Header().Get("Cache-Control") != "no-store" || !strings.Contains(r.Body.String(), `"selectionMode":"EXPLICIT_SELECTION"`) {
		t.Fatalf("读取=%d %s", r.Code, r.Body.String())
	}
	for _, body := range []string{`{}`, `{"order":"AUTO_SELECT","reason":"合成原因","confirmed":true}`, `{"order":"ONLINE_FIRST","reason":"合成原因"}`, `{"order":"ONLINE_FIRST","reason":"合成原因","confirmed":true,"nodeId":"synthetic"}`, `{"order":"ONLINE_FIRST","reason":"合成原因","confirmed":true} {}`} {
		if r = permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling/candidates", body); r.Code != 400 {
			t.Fatalf("正文=%d", r.Code)
		}
	}
	if repo.writes != 0 {
		t.Fatal("非法正文进入存储")
	}
	r = permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling/candidates", `{"order":"ONLINE_FIRST","reason":"合成原因","confirmed":true}`)
	if r.Code != 200 || repo.change.ActorID != "subject-1" || repo.change.ExpectedRevision != 1 || repo.change.At.IsZero() || repo.change.Order != schedulingpolicy.OnlineFirst {
		t.Fatalf("可信写入=%d %+v", r.Code, repo.change)
	}
	for _, csrf := range []CSRFValidator{nil, rejectedPermissionCSRF{}} {
		guarded := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, CSRF: csrf, SchedulingCandidates: repo})
		before := repo.writes
		if response := permissionRequest(guarded, "PATCH", "/api/v1/settings/scheduling/candidates", `{"order":"ONLINE_FIRST","reason":"合成原因","confirmed":true}`); response.Code < 400 || repo.writes != before {
			t.Fatal("安全上下文缺失或错误未阻断")
		}
	}
	repo.afterRead = func() { auth.allowed = false }
	if r = permissionRequest(handler, "GET", "/api/v1/settings/scheduling/candidates", ""); r.Code != 404 {
		t.Fatal("读取期间撤权未阻断")
	}
	before := repo.writes
	if r = permissionRequest(handler, "PATCH", "/api/v1/settings/scheduling/candidates", `{}`); r.Code != 404 || before != repo.writes {
		t.Fatal("未授权写入泄露")
	}
}

type candidateRankAuthorizer struct{ revoked bool }

func (v *candidateRankAuthorizer) Authorize(_ context.Context, _ identity.Principal, _ identity.Scope, id string) error {
	if id == "node-hidden" || id == "node-2" && v.revoked {
		return identity.ErrDenied
	}
	return nil
}

type candidateRankReader struct {
	ExecutionNodeManagementStore
	heartbeatsRead []string
	afterHeartbeat func()
	now            time.Time
}

func (v *candidateRankReader) ReadTaskNodeSelection(ctx context.Context, _ string) ([]store.ExecutionNode, map[string]store.NodeBudgetUsage, error) {
	nodes, err := v.ListExecutionNodes(ctx)
	return nodes, map[string]store.NodeBudgetUsage{}, err
}

func (v *candidateRankReader) ListExecutionNodeSummaries(context.Context) ([]store.ExecutionNodeSummary, error) {
	return []store.ExecutionNodeSummary{{NodeID: "node-1", DisplayName: "A", Platform: "WINDOWS_AMD64"}, {NodeID: "node-hidden", DisplayName: "Hidden", Platform: "WINDOWS_AMD64"}, {NodeID: "node-2", DisplayName: "B", Platform: "WINDOWS_AMD64"}}, nil
}
func (v *candidateRankReader) ListExecutionNodes(context.Context) ([]store.ExecutionNode, error) {
	return []store.ExecutionNode{{NodeID: "node-1", DisplayName: "A", Platform: "WINDOWS_AMD64", ManagementState: "ENABLED"}, {NodeID: "node-hidden", DisplayName: "Hidden", Platform: "WINDOWS_AMD64", ManagementState: "ENABLED"}, {NodeID: "node-2", DisplayName: "B", Platform: "WINDOWS_AMD64", ManagementState: "ENABLED"}}, nil
}
func (v *candidateRankReader) ReadCandidateHeartbeat(_ context.Context, id string) (*time.Time, error) {
	v.heartbeatsRead = append(v.heartbeatsRead, id)
	if id == "node-hidden" {
		return nil, schedulingpolicy.ErrUnavailable
	}
	if id == "node-2" {
		if v.afterHeartbeat != nil {
			v.afterHeartbeat()
		}
		stamp := v.now.Add(-time.Second)
		return &stamp, nil
	}
	return nil, nil
}

func Test三类候选只排序授权节点且不改变管理顺序(t *testing.T) {
	now := time.Now().UTC()
	policy := candidateHTTPPolicy()
	policy.Order = schedulingpolicy.OnlineFirst
	policy.Revision = 2
	policy.ModifiedBy = "subject-1"
	policy.ModifiedAt = &now
	repo := &candidateHTTPRepository{policy: policy}
	auth := &candidateRankAuthorizer{}
	nodes := &candidateRankReader{now: now}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, Dependencies{Identity: permissionProvider{"subject-1"}, Authorizer: auth, SchedulingCandidates: repo, NodeCandidates: nodes, NodeManagement: nodes, HeartbeatTTL: 2 * time.Minute})
	read := func(url string) []string {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", url, nil))
		if r.Code != 200 {
			t.Fatalf("读取=%d %s", r.Code, r.Body.String())
		}
		var body struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, item := range body.Items {
			ids = append(ids, item.ID)
		}
		return ids
	}
	for _, url := range []string{"/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT", "/api/v1/execution-nodes?candidateOrder=platform"} {
		ids := read(url)
		if strings.Join(ids, ",") != "node-2,node-1" {
			t.Fatalf("候选顺序=%v", ids)
		}
	}
	if ids := read("/api/v1/execution-nodes"); strings.Join(ids, ",") != "node-1,node-2" {
		t.Fatalf("管理列表被重排=%v", ids)
	}
	for _, id := range nodes.heartbeatsRead {
		if id == "node-hidden" {
			t.Fatal("读取了隐藏事实")
		}
	}
	nodes.afterHeartbeat = func() { auth.revoked = true }
	if ids := read("/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT"); strings.Join(ids, ",") != "node-1" {
		t.Fatal("排序期间撤权未过滤")
	}
	repo.err = schedulingpolicy.ErrUnavailable
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest("GET", "/api/v1/execution-nodes?candidateOrder=platform", nil))
	if r.Code != 503 {
		t.Fatalf("配置故障未关闭=%d", r.Code)
	}
}
