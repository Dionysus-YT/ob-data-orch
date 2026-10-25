package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/store"
)

func Test任务列表与执行接口使用同一安全进度且不改变终态(t *testing.T) {
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	percent, completed, running := 100.0, int64(10), int64(0)
	progress := &store.TaskProgress{Phase: "IMPORT_DATA", Percent: &percent, CompletedTasks: &completed, RunningTasks: &running, UpdatedAt: now}
	tasks := &recordingTaskStore{summary: store.TaskSummary{TaskID: "progress-task", TaskType: "OBLOADER_NORMAL_IMPORT", CreatorSubjectID: "synthetic-subject", DataSourceID: "source-allowed", NodeID: "node-1", State: "RUNNING", Progress: progress, SubmittedAt: now, UpdatedAt: now}, listItems: []store.TaskListItem{{TaskID: "progress-task", TaskType: "OBLOADER_NORMAL_IMPORT", CreatorSubjectID: "synthetic-subject", DataSourceID: "source-allowed", NodeID: "node-1", State: "RUNNING", Progress: progress, SubmittedAt: now, UpdatedAt: now}}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
	var earlier map[string]any
	for _, path := range []string{"/api/v1/tasks", "/api/v1/tasks/progress-task/execution"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("授权进度读取失败", response.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		item, _ := body["item"].(map[string]any)
		if item == nil {
			item = body["items"].([]any)[0].(map[string]any)
		}
		if item["state"] != "RUNNING" || item["stageEvidence"] != "RELIABLE" || item["progressEvidence"] != "PERCENT" {
			t.Fatal("百分比错误改变状态或证据级别")
		}
		fact := item["progress"].(map[string]any)
		if earlier != nil && (fact["phase"] != earlier["phase"] || fact["percent"] != earlier["percent"]) {
			t.Fatal("列表详情摘要不一致")
		}
		earlier = fact
		for _, key := range []string{"path", "command", "message", "credential", "sourceSeq", "bps"} {
			if _, exists := fact[key]; exists {
				t.Fatal("进度包含非展示字段", key)
			}
		}
	}
}
