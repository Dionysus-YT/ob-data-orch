package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/store"
)

func Test任务结果只为本地核验投影数量(t *testing.T) {
	for _, kind := range []string{"LOCAL", "OSS", "S3", "COS", "OBS", "UNAVAILABLE", "UNKNOWN", ""} {
		t.Run(kind, func(t *testing.T) {
			for _, historicalCount := range []uint64{0, 2} {
				tasks := &recordingTaskStore{summary: store.TaskSummary{
					TaskID: "result-task", TaskType: "OBDUMPER_EXPORT", CreatorSubjectID: "synthetic-subject", State: "SUCCEEDED", UpdatedAt: time.Now().UTC(),
					ResultSummary: &store.ExecutionResultSummary{Result: "VERIFIED", FileCount: historicalCount, TotalBytes: historicalCount * 5, CheckpointPresent: true, ObservedAt: time.Now().UTC()},
				}}
				if historicalCount > 0 {
					tasks.summary.ResultSummary.Files = []store.ExecutionResultFile{{Path: "synthetic.csv", Size: 10}}
				}
				if kind != "" {
					tasks.summary.OutputLocation = &store.TaskOutputLocation{Kind: kind, Redaction: "KIND_ONLY"}
				}
				handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/result-task/execution", nil))
				if response.Code != 200 {
					t.Fatal(response.Code)
				}
				var parsed struct {
					Item struct {
						State  string                     `json:"state"`
						Result map[string]json.RawMessage `json:"resultSummary"`
					} `json:"item"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &parsed); err != nil || parsed.Item.State != "SUCCEEDED" {
					t.Fatal("执行状态丢失", err)
				}
				switch kind {
				case "LOCAL":
					for _, key := range []string{"fileCount", "totalBytes", "files"} {
						if len(parsed.Item.Result[key]) == 0 || string(parsed.Item.Result[key]) == "null" {
							t.Fatal("本地事实被置空", key)
						}
					}
					var count uint64
					if err := json.Unmarshal(parsed.Item.Result["fileCount"], &count); err != nil || count != historicalCount {
						t.Fatal("本地准确数量发生变化", err)
					}
				case "OSS", "S3", "COS", "OBS":
					for _, key := range []string{"fileCount", "totalBytes", "files"} {
						if string(parsed.Item.Result[key]) != "null" {
							t.Fatal("远端未经核验却返回数量或文件", key)
						}
					}
					if string(parsed.Item.Result["result"]) != `"VERIFIED"` || strings.Contains(response.Body.String(), "synthetic.csv") {
						t.Fatal("远端完成结论或边界错误")
					}
				default:
					if parsed.Item.Result != nil {
						t.Fatal("未知输出类型仍展示结果证明")
					}
				}
			}
		})
	}
}

func Test两类导入没有输出位置仍保留完成结论(t *testing.T) {
	for _, kind := range []string{"OBLOADER_NORMAL_IMPORT", "OBLOADER_DIRECT_LOAD"} {
		tasks := &recordingTaskStore{summary: store.TaskSummary{TaskID: "import-result", TaskType: kind, State: "SUCCEEDED", UpdatedAt: time.Now().UTC(), ResultSummary: &store.ExecutionResultSummary{Result: "VERIFIED", ObservedAt: time.Now().UTC()}}}
		handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/import-result/execution", nil))
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"result":"VERIFIED"`) || !strings.Contains(response.Body.String(), `"state":"SUCCEEDED"`) || strings.Contains(response.Body.String(), "outputLocation") {
			t.Fatal("导入完成证据发生回归", kind, response.Code)
		}
	}
}
