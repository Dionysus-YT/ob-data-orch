package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/store"
)

type taskManagementFixture struct {
	recordingTaskStore
	reads int
}

func (s *taskManagementFixture) GetAuthorizedTaskSummary(_ context.Context, id, subject string) (store.TaskSummary, error) {
	if id != "A" && id != "B" {
		return store.TaskSummary{}, store.ErrDataSourceNotFound
	}
	return store.TaskSummary{TaskID: id, CreatorSubjectID: subject, State: "SUCCEEDED", SubmittedAt: nodeFixtureTime}, nil
}
func (s *taskManagementFixture) ListTaskOperations(_ context.Context, q store.TaskListQuery, _ time.Time, _ string) ([]store.TaskOperation, error) {
	s.reads++
	items := []store.TaskOperation{}
	for index := 0; index < 51; index++ {
		items = append(items, store.TaskOperation{ID: "audit:" + string(rune('a'+index)), ActorType: "SUBJECT", ActorName: "合成用户", Action: "TASK_SUBMITTED", Result: "SUCCEEDED", OccurredAt: nodeFixtureTime})
	}
	return items, nil
}

func Test操作记录游标绑定任务与授权且拒绝重复参数(t *testing.T) {
	tasks := &taskManagementFixture{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/A/operations", nil))
	var page struct {
		Items      []store.TaskOperation `json:"items"`
		NextCursor string                `json:"nextCursor"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || first.Code != 200 || len(page.Items) != 50 || page.NextCursor == "" {
		t.Fatal("操作记录页错误", first.Code, err)
	}
	for _, path := range []string{"/api/v1/tasks/B/operations?cursor=" + url.QueryEscape(page.NextCursor), "/api/v1/tasks/A/operations?cursor=x&cursor=y", "/api/v1/tasks/unknown/operations"} {
		reads := tasks.reads
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 400 && response.Code != 404 {
			t.Fatal("游标或授权未关闭", response.Code)
		}
		if tasks.reads != reads {
			t.Fatal("拒绝后仍读取操作记录")
		}
	}
}

type draftManagementFixture struct {
	recordingDraftStore
	query  store.ExportDraftListQuery
	writes int
	err    error
}

func (s *draftManagementFixture) ListOwnedExportDrafts(_ context.Context, q store.ExportDraftListQuery) ([]store.ExportDraftListItem, int, error) {
	s.query = q
	return []store.ExportDraftListItem{}, 0, nil
}
func (s *draftManagementFixture) DeleteOwnedExportDraft(context.Context, string, string, int64, string, string, time.Time) (bool, error) {
	s.writes++
	return false, s.err
}

func Test草稿列表固定主体与删除写门禁(t *testing.T) {
	drafts := &draftManagementFixture{}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Drafts: drafts, CSRF: allowedCSRF{}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/export-drafts?keyword=orders&limit=20", nil))
	if response.Code != 200 || drafts.query.SubjectID != "synthetic-subject" || drafts.query.Keyword != "orders" || drafts.query.Limit != 20 {
		t.Fatal("草稿列表未固定主体", response.Code, drafts.query)
	}
	for _, path := range []string{"/api/v1/export-drafts?scope=ALL", "/api/v1/export-drafts?owner=another", "/api/v1/export-drafts?limit=999", "/api/v1/export-drafts?cursor=x&cursor=y"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != 400 {
			t.Fatal("草稿筛选未失败关闭", path, r.Code)
		}
	}
	for _, tc := range []struct {
		revision, key string
		status        int
	}{{"", "", 412}, {`"rev-1"`, "", 400}, {`"rev-1"`, "synthetic-delete-key", 200}} {
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/export-drafts/owned-draft", nil)
		request.Header.Set("If-Match", tc.revision)
		request.Header.Set("Idempotency-Key", tc.key)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatal("草稿写门禁错误", response.Code, response.Body.String())
		}
	}
	if drafts.writes != 1 {
		t.Fatal("无版本或幂等请求进入了写事务")
	}
	drafts.err = store.ErrExportDraftDeleteIneligible
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/export-drafts/owned-draft", nil)
	request.Header.Set("If-Match", `"rev-1"`)
	request.Header.Set("Idempotency-Key", "synthetic-delete-key")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 409 {
		t.Fatal("活动草稿删除未阻断", response.Code)
	}
}

type expiredLogIndex struct {
	emptyLogBatchIndex
	reads int
}

func (i *expiredLogIndex) ListLogCleanup(context.Context, time.Time, int) ([]logstream.CleanupSegment, error) {
	return nil, nil
}
func (i *expiredLogIndex) BeginLogCleanup(context.Context, logstream.CleanupSegment, time.Time, time.Time) (bool, error) {
	return false, nil
}
func (i *expiredLogIndex) CompleteLogCleanup(context.Context, string, time.Time) error { return nil }
func (i *expiredLogIndex) TaskLogsExpired(context.Context, string) (bool, error) {
	i.reads++
	return true, nil
}
func Test日志正常过期投影与未授权对象隔离(t *testing.T) {
	index := &expiredLogIndex{}
	persistent, err := logstream.NewPersistentStore(t.TempDir(), index)
	if err != nil {
		t.Fatal(err)
	}
	defer persistent.Close()
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: &taskManagementFixture{}, PersistentLogs: persistent})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/A/logs", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"integrity":"EXPIRED"`) {
		t.Fatal("正常过期被误写为采集失败", response.Code, response.Body.String())
	}
	reads := index.reads
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/unknown/logs", nil))
	if response.Code != 404 || index.reads != reads {
		t.Fatal("未授权对象泄露清理事实")
	}
}
