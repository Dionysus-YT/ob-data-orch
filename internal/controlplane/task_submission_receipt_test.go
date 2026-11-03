package controlplane

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/store"
)

func Test导出原提交在证明过期后恢复且不重新冻结(t *testing.T) {
	drafts, prechecks, tasks := &recordingDraftStore{}, &recordingPrecheckStore{}, &recordingTaskStore{}
	handler := newGeneralizedFlowHandler(t, drafts, prechecks, tasks)
	body := `{"configVersion":"v6","dataSourceId":"source-allowed","nodeId":"node-1","config":{"objectScope":{"database":"synthetic_db","scopeKind":"ALL"},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/synthetic-output"}}}`
	runV6CapabilityFlow(t, handler, drafts, prechecks, tasks, body, "receipt")
	read := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	for _, path := range []string{"/api/v1/export-drafts/draft-synthetic", "/api/v1/prechecks/precheck-synthetic"} {
		response := read(path)
		if response.Code != 200 || strings.Contains(response.Body.String(), "submittedTaskId") {
			t.Fatal("未提交字段没有省略", path, response.Code)
		}
	}
	drafts.created.SubmittedTaskID = tasks.input.TaskID
	if response := read("/api/v1/prechecks/precheck-synthetic"); response.Code != 200 || !strings.Contains(response.Body.String(), "submittedTaskId") {
		t.Fatal("同修订未返回受理结果", response.Code)
	}
	drafts.created.Revision = 2
	drafts.created.SubmittedTaskID = "synthetic-new-revision-task"
	if response := read("/api/v1/prechecks/precheck-synthetic"); response.Code != 200 || strings.Contains(response.Body.String(), "submittedTaskId") || strings.Contains(response.Body.String(), "synthetic-new-revision-task") {
		t.Fatal("旧检查混入新修订结果", response.Code)
	}
	drafts.created.Revision = 1

	tasks.run.ValidUntil, tasks.run.Status = time.Now().Add(-time.Hour), "EXPIRED"
	tasks.lookup = func(input store.TaskSubmissionLookup) (store.TaskSubmissionResult, error) {
		if input.IdempotencyKey != tasks.submitKey || input.RequestDigest != tasks.submitDigest || input.DraftID != tasks.run.DraftID || input.PrecheckID != tasks.run.PrecheckID {
			return store.TaskSubmissionResult{}, store.ErrTaskSubmissionConflict
		}
		return store.TaskSubmissionResult{TaskID: tasks.input.TaskID, Replayed: true}, nil
	}
	request := func(key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/export-drafts/draft-synthetic:submit", bytes.NewBufferString(`{"precheckId":"precheck-synthetic"}`))
		r.Header.Set("If-Match", `"rev-1"`)
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	response := request(tasks.submitKey)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"replayed":true`) || tasks.submitCount != 1 {
		t.Fatal("原键未恢复", response.Code)
	}
	response = request("synthetic-different-submit-key")
	if response.Code != 409 || !strings.Contains(response.Body.String(), "SUBMISSION_CONFLICT") || tasks.submitCount != 1 {
		t.Fatal("新键未冲突", response.Code)
	}
	tasks.lookup = func(store.TaskSubmissionLookup) (store.TaskSubmissionResult, error) {
		return store.TaskSubmissionResult{}, store.ErrClaimIneligible
	}
	if response := request(tasks.submitKey); response.Code != 404 {
		t.Fatal("撤权后泄露回执", response.Code)
	}
}
