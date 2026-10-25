package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/overview"
	"ob-data-orch/internal/store"
)

type overviewTestReader struct {
	query overview.Query
	err   error
	facts store.OverviewFacts
	calls int
}

func (reader *overviewTestReader) ReadOverviewNodeIDs(_ context.Context, _ string) ([]string, error) {
	ids := []string{}
	if reader.facts.Nodes != nil {
		for _, node := range *reader.facts.Nodes {
			ids = append(ids, node.NodeID)
		}
	}
	return ids, nil
}

func (reader *overviewTestReader) ReadOverviewFacts(_ context.Context, q overview.Query) (store.OverviewFacts, error) {
	reader.query = q
	reader.calls++
	return reader.facts, reader.err
}

type overviewNodeAuthorizer struct{}

func (overviewNodeAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, id string) error {
	if scope == identity.ScopeNodeManage && id == "visible-node" {
		return nil
	}
	return identity.ErrDenied
}

func Test首页接口参数与安全依赖失败关闭(t *testing.T) {
	for _, test := range []struct {
		path         string
		dependencies Dependencies
		want         int
	}{
		{"/api/v1/overview", Dependencies{}, 503},
		{"/api/v1/overview", Dependencies{Identity: browserOnlyIdentityProvider{}}, 503},
		{"/api/v1/overview?range=30d", Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: &overviewTestReader{}}, 400},
		{"/api/v1/overview?range=24h&range=7d", Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: &overviewTestReader{}}, 400},
		{"/api/v1/overview?subjectId=other", Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: &overviewTestReader{}}, 400},
		{"/api/v1/overview?range=%ZZ", Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: &overviewTestReader{}}, 400},
		{"/api/v1/overview", Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: &overviewTestReader{err: store.ErrOverviewScopeDenied}}, 403},
		{"/api/v1/overview", Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: &overviewTestReader{err: errors.New("synthetic internal failure")}}, 503},
	} {
		response := httptest.NewRecorder()
		NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, test.dependencies).ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.want || strings.Contains(response.Body.String(), "synthetic internal failure") {
			t.Fatalf("%s => %d，期望%d", test.path, response.Code, test.want)
		}
	}
}

func Test首页节点先授权再计数且不返回配置秘密(t *testing.T) {
	nodes := []store.ExecutionNode{{NodeID: "hidden-node", DisplayName: "不应出现的节点"}, {NodeID: "visible-node", DisplayName: "可见节点", ManagementState: "DISABLED"}}
	reader := &overviewTestReader{facts: store.OverviewFacts{Nodes: &nodes, Metrics: &overview.Metrics{Running: 2}}}
	response := httptest.NewRecorder()
	NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: reader}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/overview?range=7d", nil))
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || reader.query.SubjectID != "synthetic-subject" || reader.query.GlobalRead || reader.query.Range != "7d" {
		t.Fatalf("安全上下文错误：%d %#v", response.Code, reader.query)
	}
	var body struct {
		Item overview.Snapshot `json:"item"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Item.Nodes.Data.Total != 1 || body.Item.Nodes.Data.Items[0].ID != "visible-node" {
		t.Fatal("节点计数没有经过对象授权")
	}
	if len(reader.query.NodeIDs) != 1 || reader.query.NodeIDs[0] != "visible-node" {
		t.Fatal("未授权节点进入事实读取范围")
	}
	for _, value := range []string{"hidden-node", "不应出现的节点", "toolHome", "javaPath", "allowedRoots", "credential"} {
		if strings.Contains(response.Body.String(), value) {
			t.Fatalf("响应包含未授权或无关字段：%s", value)
		}
	}
}

type overviewRevokingAuthorizer struct{ calls int }

func (auth *overviewRevokingAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, _ string) error {
	if scope == identity.ScopeNodeManage {
		auth.calls++
		if auth.calls == 1 {
			return nil
		}
	}
	return identity.ErrDenied
}

func Test首页读取期间撤销节点授权不返回旧对象(t *testing.T) {
	nodes := []store.ExecutionNode{{NodeID: "visible-node", DisplayName: "撤销后不可见"}}
	reader := &overviewTestReader{facts: store.OverviewFacts{Nodes: &nodes, Metrics: &overview.Metrics{}}}
	auth := &overviewRevokingAuthorizer{}
	response := httptest.NewRecorder()
	NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: auth, Overview: reader}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	if response.Code != 200 || len(reader.query.NodeIDs) != 1 || strings.Contains(response.Body.String(), "撤销后不可见") {
		t.Fatal("迟到节点事实跨越授权撤销")
	}
}

func Test首页终态筛选拒绝非法边界并绑定游标(t *testing.T) {
	var query store.TaskListQuery
	request := httptest.NewRequest("GET", "/api/v1/tasks?finishedFrom=2026-10-23T00:00:00Z&finishedTo=2026-10-24T00:00:00Z", nil)
	if err := parseTaskListFilters(request.URL.Query(), &query); err != nil || query.FinishedFrom.IsZero() || query.FinishedTo.IsZero() {
		t.Fatal("终态筛选没有解析")
	}
	first := taskListFilterDigestSubject(query)
	query.FinishedTo = query.FinishedTo.Add(-1)
	if first == taskListFilterDigestSubject(query) {
		t.Fatal("终态筛选没有绑定游标")
	}
	bad := httptest.NewRequest("GET", "/api/v1/tasks?finishedFrom=2026-10-23T00:00:00Z&finishedTo=2026-10-23T00:00:00Z", nil)
	if parseTaskListFilters(bad.URL.Query(), &query) == nil {
		t.Fatal("空终态区间未拒绝")
	}
}

func Test首页全局读取必须同时具备可信角色与能力(t *testing.T) {
	for _, pair := range [][2]bool{{true, false}, {false, true}, {true, true}} {
		reader := &overviewTestReader{facts: store.OverviewFacts{Metrics: &overview.Metrics{}}}
		auth := scopeTestAuthorizer{globalRole: pair[0], globalRead: pair[1]}
		response := httptest.NewRecorder()
		NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: auth, Roles: auth, Overview: reader}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
		if response.Code != 200 || reader.query.GlobalRead != (pair[0] && pair[1]) {
			t.Fatal("首页全局范围绕过了双重授权")
		}
	}
}

func Test首页全部分区不可用返回整体失败(t *testing.T) {
	reader := &overviewTestReader{}
	response := httptest.NewRecorder()
	NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: overviewNodeAuthorizer{}, Overview: reader}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	if response.Code != 503 || strings.Contains(response.Body.String(), `"item"`) {
		t.Fatal("全未知快照错误覆盖最后可信数据")
	}
}
