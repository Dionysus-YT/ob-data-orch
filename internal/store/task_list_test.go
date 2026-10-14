package store

import (
	"context"
	"testing"
	"time"
)

// seedTaskListFixture 提交三个合成任务：两个属于 subject-1，一个属于 subject-other。
func seedTaskListFixture(t *testing.T) *Store {
	t.Helper()
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	if err := s.EnsureAuthSubject(ctx, AuthSubject{SubjectID: "subject-other", ExternalSubject: "external-other", DisplayName: "Synthetic Other", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	for index, spec := range []struct{ id, creator, table string }{
		{"task-alpha", "subject-1", "orders_100%"},
		{"task-beta", "subject-1", "customers"},
		{"task-gamma", "subject-other", "private_table"},
	} {
		submission := validTaskSubmission(spec.id)
		submission.CreatorSubjectID, submission.AuditActorID = spec.creator, spec.creator
		submission.SnapshotJSON = `{"database":"synthetic_db","table":"` + spec.table + `","scopeKind":"SPECIFIED"}`
		submission.SubmittedAt = testTime.Add(time.Duration(index+3) * time.Minute)
		submission.RequestID = "request-" + spec.id
		if err := s.SubmitTask(ctx, submission); err != nil {
			t.Fatalf("SubmitTask(%s): %v", spec.id, err)
		}
	}
	return s
}

func taskIDs(items []TaskListItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.TaskID)
	}
	return ids
}

func Test任务提交冻结展示名称且删除后仍可读取(t *testing.T) {
	s := seedTaskListFixture(t)
	ctx := context.Background()
	items, err := s.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-1", Limit: 10})
	if err != nil || len(items) != 2 {
		t.Fatalf("list = %v, %v", taskIDs(items), err)
	}
	first := items[0]
	if first.DataSourceName != "Synthetic Source" || first.Environment != "TEST" || first.NodeName != "Synthetic Node" || first.CreatorName != "Synthetic User" {
		t.Fatalf("展示事实未冻结: %#v", first)
	}
	if !first.Operable || first.ScopeKind != "SPECIFIED" || first.SnapshotVersion != "v1" {
		t.Fatalf("资格或快照事实错误: %#v", first)
	}
	// 冻结后修改当前名称不得改写历史展示名称。
	if _, err := s.db.ExecContext(ctx, `UPDATE data_sources SET display_name = 'Renamed Source' WHERE data_source_id = 'source-1'`); err != nil {
		t.Fatal(err)
	}
	items, err = s.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-1", Limit: 10})
	if err != nil || items[0].DataSourceName != "Synthetic Source" {
		t.Fatalf("历史名称被当前配置覆盖: %#v, %v", items, err)
	}
	if items[0].DataSourceDeleted || items[0].NodeDeleted {
		t.Fatalf("现存对象不应标注已删除: %#v", items[0])
	}
	// 模拟数据源配置被永久删除：历史名称保留，同时标注已删除。
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM data_sources WHERE data_source_id = 'source-1'`); err != nil {
		t.Fatal(err)
	}
	items, err = s.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-1", Limit: 10})
	if err != nil || items[0].DataSourceName != "Synthetic Source" || !items[0].DataSourceDeleted {
		t.Fatalf("删除后应保留历史名称并标注: %#v, %v", items, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE task_display_facts SET node_name = 'tampered'`); err == nil {
		t.Fatal("展示事实应不可改写")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM task_display_facts`); err == nil {
		t.Fatal("展示事实应不可删除")
	}
}

func Test任务列表筛选只收窄授权范围(t *testing.T) {
	s := seedTaskListFixture(t)
	ctx := context.Background()
	cases := []struct {
		name  string
		query TaskListQuery
		want  []string
	}{
		{"本人范围不含他人", TaskListQuery{}, []string{"task-beta", "task-alpha"}},
		{"关键字按字面匹配百分号", TaskListQuery{Keyword: "100%"}, []string{"task-alpha"}},
		{"关键字下划线不作通配符", TaskListQuery{Keyword: "m_r"}, []string{}},
		{"关键字匹配任务 ID", TaskListQuery{Keyword: "beta"}, []string{"task-beta"}},
		{"关键字不能发现他人任务", TaskListQuery{Keyword: "private"}, []string{}},
		{"创建人筛选不能越权", TaskListQuery{CreatorID: "subject-other"}, []string{}},
		{"等待调度状态", TaskListQuery{States: []string{"WAITING_SCHEDULE"}}, []string{"task-beta", "task-alpha"}},
		{"未知状态得到空集", TaskListQuery{States: []string{"RUNNING"}}, []string{}},
		{"其他任务类型得到空集", TaskListQuery{Types: []string{"OBLOADER_DIRECT_LOAD"}}, []string{}},
		{"环境筛选", TaskListQuery{Environment: "TEST"}, []string{"task-beta", "task-alpha"}},
		{"环境不匹配", TaskListQuery{Environment: "PRODUCTION"}, []string{}},
		{"数据源筛选", TaskListQuery{DataSourceID: "source-1"}, []string{"task-beta", "task-alpha"}},
		{"创建时间半开区间", TaskListQuery{CreatedFrom: testTime.Add(4 * time.Minute), CreatedTo: testTime.Add(5 * time.Minute)}, []string{"task-beta"}},
		{"未开始任务不命中开始时间", TaskListQuery{StartedFrom: testTime}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := tc.query
			query.SubjectID, query.Limit = "subject-1", 10
			items, err := s.ListTaskSummaries(ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			got := taskIDs(items)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
			count, err := s.CountFilteredTaskSummaries(ctx, query)
			if err != nil || count != len(tc.want) {
				t.Fatalf("计数 %d 与列表 %d 不一致: %v", count, len(tc.want), err)
			}
		})
	}
}

func Test全局只读不授予写资格且候选不越权(t *testing.T) {
	s := seedTaskListFixture(t)
	ctx := context.Background()
	items, err := s.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-1", GlobalRead: true, Limit: 10})
	if err != nil || len(items) != 3 {
		t.Fatalf("global list = %v, %v", taskIDs(items), err)
	}
	for _, item := range items {
		if item.TaskID == "task-gamma" && item.Operable {
			t.Fatal("全局只读不应获得他人任务写资格")
		}
	}
	options, err := s.ListTaskFilterOptions(ctx, TaskListQuery{SubjectID: "subject-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Creators) != 1 || options.Creators[0].ID != "subject-1" {
		t.Fatalf("非全局候选泄露其他创建人: %#v", options.Creators)
	}
	if len(options.DataSources) != 1 || options.DataSources[0].Label != "Synthetic Source" || len(options.Environments) != 1 {
		t.Fatalf("候选值错误: %#v", options)
	}
	unscoped, err := s.ListTaskFilterOptions(ctx, TaskListQuery{SubjectID: "subject-missing"})
	if err != nil || len(unscoped.DataSources) != 0 || len(unscoped.Creators) != 0 || len(unscoped.Environments) != 0 {
		t.Fatalf("无效主体不应得到候选: %#v, %v", unscoped, err)
	}
	global, err := s.ListTaskFilterOptions(ctx, TaskListQuery{SubjectID: "subject-1", GlobalRead: true})
	if err != nil || len(global.Creators) != 2 {
		t.Fatalf("全局候选应包含全部创建人: %#v, %v", global.Creators, err)
	}
}

func Test任务列表拒绝过多筛选值(t *testing.T) {
	s := seedTaskListFixture(t)
	states := make([]string, taskListMaxFilterValues+1)
	for i := range states {
		states[i] = "RUNNING"
	}
	if _, err := s.ListTaskSummaries(context.Background(), TaskListQuery{SubjectID: "subject-1", Limit: 10, States: states}); err == nil {
		t.Fatal("超过上限的多选值应被拒绝")
	}
}
