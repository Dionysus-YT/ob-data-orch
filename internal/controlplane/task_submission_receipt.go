package controlplane

import (
	"errors"
	"net/http"

	"ob-data-orch/internal/store"
)

// replayTaskSubmission 在新冻结前找回原意图；调用前完成浏览器身份、对象范围与确认校验。
func (s *Server) replayTaskSubmission(w http.ResponseWriter, r *http.Request, input store.TaskSubmissionLookup) bool {
	result, err := s.tasks.LookupTaskSubmission(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrClaimIneligible), errors.Is(err, store.ErrDataSourceNotFound):
			notFound(w, r)
		case errors.Is(err, store.ErrIdempotencyConflict):
			writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		case errors.Is(err, store.ErrTaskSubmissionConflict):
			writeError(w, http.StatusConflict, "SUBMISSION_CONFLICT", "当前草稿版本已提交，请查看已提交任务", false)
		default:
			writeError(w, http.StatusServiceUnavailable, "TASK_SUBMISSION_UNAVAILABLE", "提交结果暂时无法核对，请使用原请求重试", true)
		}
		return true
	}
	if result.TaskID == "" {
		return false
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "id": result.TaskID, "replayed": true, "state": "WAITING_SCHEDULE", "type": input.TaskType, "realExecutionEnabled": s.realExecutionEnabled})
	return true
}
