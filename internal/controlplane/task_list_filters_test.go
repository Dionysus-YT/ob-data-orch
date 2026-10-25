package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/store"
)

func Test任务列表筛选参数解析与失败关闭(t *testing.T) {
	valid := url.Values{
		"keyword": {" orders "}, "type": {"OBDUMPER_EXPORT"}, "state": {"RUNNING,FAILED,RUNNING"},
		"createdFrom": {"2026-10-01T00:00:00+08:00"}, "createdTo": {"2026-10-02T00:00:00+08:00"},
		"dataSourceId": {"source-1"}, "environment": {"PRODUCTION"}, "creatorId": {"subject-1"},
	}
	var query store.TaskListQuery
	if err := parseTaskListFilters(valid, &query); err != nil {
		t.Fatal(err)
	}
	if query.Keyword != "orders" || !slices.Equal(query.States, []string{"RUNNING", "FAILED"}) || query.Environment != "PRODUCTION" ||
		!query.CreatedFrom.Equal(time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("解析结果错误: %#v", query)
	}
	for name, values := range map[string]url.Values{
		"未知状态":     {"state": {"DRAFT"}},
		"未知类型":     {"type": {"UNKNOWN_IMPORT"}},
		"单值重复":     {"keyword": {"a", "b"}},
		"多值参数重复":   {"state": {"RUNNING", "FAILED"}},
		"过多状态":     {"state": {"RUNNING,FAILED,SUCCEEDED,CANCELLED,STARTING,CANCELLING,WAITING_SCHEDULE,RUNNING,FAILED"}},
		"非法时间":     {"createdFrom": {"2026-10-01"}},
		"时间区间倒置":   {"createdFrom": {"2026-10-02T00:00:00Z"}, "createdTo": {"2026-10-01T00:00:00Z"}},
		"未知环境":     {"environment": {"LOCAL"}},
		"超长关键字":    {"keyword": {string(make([]byte, 129))}},
		"关键字含换行":   {"keyword": {"a\nb"}},
		"数据源标识含路径": {"dataSourceId": {"../x"}},
	} {
		var rejected store.TaskListQuery
		if err := parseTaskListFilters(values, &rejected); err == nil {
			t.Fatalf("%s 应被拒绝", name)
		}
	}
}

func Test任务列表按创建人筛选需要全局读取(t *testing.T) {
	for _, tc := range []struct {
		global bool
		query  string
		want   int
	}{
		{false, "creatorId=other-subject", http.StatusForbidden},
		{false, "creatorId=synthetic-subject", http.StatusOK},
		{true, "creatorId=other-subject", http.StatusOK},
	} {
		tasks := &recordingTaskStore{}
		auth := scopeTestAuthorizer{globalRole: tc.global, globalRead: tc.global}
		handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks, Authorizer: auth, Roles: auth})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?"+tc.query, nil))
		if response.Code != tc.want {
			t.Fatalf("global=%v %s => %d body=%s", tc.global, tc.query, response.Code, response.Body.String())
		}
		if tc.want == http.StatusForbidden && tasks.listQuery.SubjectID != "" {
			t.Fatal("被拒绝的请求不应查询仓储")
		}
	}
}

func Test任务列表筛选传给列表与计数并绑定游标(t *testing.T) {
	tasks := &recordingTaskStore{}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 11; index++ {
		tasks.listItems = append(tasks.listItems, store.TaskListItem{TaskID: "task-" + string(rune('a'+index)), CreatorSubjectID: "synthetic-subject", TaskType: "OBDUMPER_EXPORT", State: "FAILED", SubmittedAt: base.Add(-time.Duration(index) * time.Minute), UpdatedAt: base})
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?state=FAILED&keyword=orders", nil))
	if response.Code != http.StatusOK || tasks.listQuery.Keyword != "orders" || tasks.countQuery.Keyword != "orders" || !slices.Equal(tasks.countQuery.States, []string{"FAILED"}) {
		t.Fatalf("筛选未同时作用于列表与计数: %d list=%#v count=%#v", response.Code, tasks.listQuery, tasks.countQuery)
	}
	var body struct {
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.NextCursor == "" {
		t.Fatalf("缺少下一页游标: %v", err)
	}
	same := httptest.NewRecorder()
	handler.ServeHTTP(same, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?state=FAILED&keyword=orders&cursor="+url.QueryEscape(body.NextCursor), nil))
	if same.Code != http.StatusOK {
		t.Fatalf("同条件游标应可用: %d", same.Code)
	}
	changed := httptest.NewRecorder()
	handler.ServeHTTP(changed, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?state=FAILED&keyword=other&cursor="+url.QueryEscape(body.NextCursor), nil))
	if changed.Code != http.StatusBadRequest {
		t.Fatalf("条件变化后旧游标应失效: %d", changed.Code)
	}
}

func Test任务列表行操作资格(t *testing.T) {
	failed := store.TaskListItem{State: "FAILED", SnapshotVersion: "v2", CheckpointPresent: true, Operable: true}
	cases := []struct {
		name string
		item store.TaskListItem
		real bool
		want []string
	}{
		{"等待调度可取消", store.TaskListItem{State: "WAITING_SCHEDULE", Operable: true}, true, []string{"COPY_COMMAND", "CANCEL"}},
		{"合成环境不提供取消", store.TaskListItem{State: "RUNNING", Operable: true}, false, []string{"COPY_COMMAND", "VIEW_LOGS"}},
		{"核对中不提供取消", store.TaskListItem{State: "RUNNING", Operable: true, ReconciliationRequired: true}, true, []string{"COPY_COMMAND", "VIEW_LOGS"}},
		{"已请求取消不重复", store.TaskListItem{State: "RUNNING", Operable: true, CancellationRequested: true}, true, []string{"COPY_COMMAND", "VIEW_LOGS"}},
		{"失败任务派生与检查点", failed, true, []string{"COPY_COMMAND", "VIEW_LOGS", "REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH", "CHECKPOINT_RESUME"}},
		{"检查点继续任务不再继续", store.TaskListItem{State: "FAILED", SnapshotVersion: "v2", CheckpointPresent: true, Operable: true, DerivationKind: "CHECKPOINT_RESUME"}, true, []string{"COPY_COMMAND", "VIEW_LOGS", "REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH"}},
		{"v1 快照不可派生", store.TaskListItem{State: "FAILED", SnapshotVersion: "v1", Operable: true}, true, []string{"COPY_COMMAND", "VIEW_LOGS"}},
		{"成功可存模板", store.TaskListItem{State: "SUCCEEDED", SnapshotVersion: "v2", Operable: true}, true, []string{"COPY_COMMAND", "VIEW_LOGS", "REBUILD_FROM_CONFIG", "SAVE_TEMPLATE"}},
		{"全局只读无写操作", store.TaskListItem{State: "FAILED", SnapshotVersion: "v2", CheckpointPresent: true}, true, []string{"COPY_COMMAND", "VIEW_LOGS"}},
		{"取消中只读", store.TaskListItem{State: "CANCELLING", Operable: true}, true, []string{"COPY_COMMAND", "VIEW_LOGS"}},
	}
	for _, tc := range cases {
		tc.item.TaskType = "OBDUMPER_EXPORT"
		if got := taskListActions(tc.item, tc.real, true); !slices.Equal(got, tc.want) {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	if got := taskListActions(store.TaskListItem{TaskType: "OBDUMPER_EXPORT", State: "SUCCEEDED", SnapshotVersion: "v2", Operable: true}, true, false); slices.Contains(got, "SAVE_TEMPLATE") {
		t.Fatal("未配置模板依赖时不应提供保存模板")
	}
}

func Test任务展示名派生(t *testing.T) {
	for _, tc := range []struct{ database, table, scope, want string }{
		{"db", "orders", "SPECIFIED", "db.orders"},
		{"db", "a,b,c", "SPECIFIED", "db（3 个对象）"},
		{"db", "", "ALL", "db（全部对象）"},
		{"db", "", "QUERY_RESULT", "db · 查询结果集"},
		{"", "", "", "导出任务"},
	} {
		if got := taskDisplayName(tc.database, tc.table, tc.scope); got != tc.want {
			t.Fatalf("taskDisplayName(%q,%q,%q)=%q want %q", tc.database, tc.table, tc.scope, got, tc.want)
		}
	}
}

type filterOptionsTaskStore struct {
	recordingTaskStore
	optionsQuery store.TaskListQuery
}

func (s *filterOptionsTaskStore) ListTaskFilterOptions(_ context.Context, q store.TaskListQuery) (store.TaskListFilterOptions, error) {
	s.optionsQuery = q
	return store.TaskListFilterOptions{
		DataSources:  []store.TaskListFilterOption{{ID: "source-1", Label: "Synthetic Source"}},
		Creators:     []store.TaskListFilterOption{{ID: "other-subject", Label: "Other"}},
		Environments: []string{"TEST"},
	}, nil
}

func Test任务筛选候选非全局不返回创建人(t *testing.T) {
	for _, global := range []bool{false, true} {
		tasks := &filterOptionsTaskStore{}
		auth := scopeTestAuthorizer{globalRole: global, globalRead: global}
		handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks, Authorizer: auth, Roles: auth})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/filter-options", nil))
		var body struct {
			Creators               []map[string]string `json:"creators"`
			DataSources            []map[string]string `json:"dataSources"`
			CreatorFilterAvailable bool                `json:"creatorFilterAvailable"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
			t.Fatalf("filter options %d %s", response.Code, response.Body.String())
		}
		if tasks.optionsQuery.GlobalRead != global || body.CreatorFilterAvailable != global || (len(body.Creators) > 0) != global || len(body.DataSources) != 1 {
			t.Fatalf("global=%v 候选投影错误: %s", global, response.Body.String())
		}
		rejected := httptest.NewRecorder()
		handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/filter-options?scope=ALL", nil))
		if rejected.Code != http.StatusBadRequest {
			t.Fatal("候选接口不应接受范围参数")
		}
	}
}
