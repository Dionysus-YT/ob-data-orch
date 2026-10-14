package controlplane

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// taskRelationResponse 只暴露可授权读取的派生任务标识与展示摘要。
type taskRelationResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	State       string `json:"state"`
}

// taskListProjection 为列表和详情复用同一展示与资格规则，避免详情按状态猜测权限。
func (s *Server) taskListProjection(item store.TaskListItem, subjectID string) taskListResponse {
	return taskListResponse{
		ID: item.TaskID, Type: item.TaskType, DataSourceID: item.DataSourceID, ObjectSummary: taskObjectSummary(item.Database, item.Table), State: item.State,
		StageEvidence: "UNAVAILABLE", ProgressEvidence: "UNAVAILABLE", ReconciliationRequired: item.ReconciliationRequired, NodeID: item.NodeID,
		OwnedByCurrentUser: item.CreatorSubjectID == subjectID, SubmittedAt: item.SubmittedAt.Format(time.RFC3339Nano), StartedAt: optionalTaskTime(item.StartedAt), FinishedAt: optionalTaskTime(item.FinishedAt), UpdatedAt: item.UpdatedAt.Format(time.RFC3339Nano),
		DisplayName: taskDisplayName(item.Database, item.Table, item.ScopeKind), DataSourceName: item.DataSourceName, DataSourceDeleted: item.DataSourceDeleted, Environment: item.Environment,
		NodeName: item.NodeName, NodeDeleted: item.NodeDeleted, CreatorName: item.CreatorName, CancelRequested: item.CancellationRequested || item.State == "CANCELLING", AvailableActions: taskListActions(item, s.realExecutionEnabled, s.templates != nil),
	}
}

// listTaskOperations 只在当前任务读取授权内返回安全动作字段；游标绑定主体、任务和固定资源。
func (s *Server) listTaskOperations(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	if _, ok := s.loadAuthorizedTaskSummary(w, r, principal, taskID); !ok {
		return
	}
	reader, ok := s.tasks.(interface {
		ListTaskOperations(context.Context, store.TaskListQuery, time.Time, string) ([]store.TaskOperation, error)
	})
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "TASK_OPERATIONS_UNAVAILABLE", "操作记录暂时不可读取", false)
		return
	}
	binding := principal.ID + "|task-operations|" + taskID
	if len(r.URL.Query()) > 1 || len(r.URL.Query()["cursor"]) > 1 {
		writeError(w, 400, "CURSOR_INVALID", "操作记录游标无效", false)
		return
	}
	for key := range r.URL.Query() {
		if key != "cursor" {
			writeError(w, 400, "CURSOR_INVALID", "操作记录游标无效", false)
			return
		}
	}
	var before time.Time
	var beforeID string
	if value := strings.TrimSpace(r.URL.Query().Get("cursor")); value != "" {
		position, valid := decodeTaskListCursor(value, binding)
		if !valid {
			writeError(w, http.StatusBadRequest, "CURSOR_INVALID", "操作记录游标无效", false)
			return
		}
		before, beforeID = position.BeforeSubmitted, position.BeforeTaskID
	}
	items, err := reader.ListTaskOperations(r.Context(), store.TaskListQuery{TaskID: taskID, SubjectID: principal.ID, GlobalRead: s.hasGlobalTaskRead(r.Context(), principal)}, before, beforeID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_OPERATIONS_UNAVAILABLE", "操作记录暂时不可读取", true)
		return
	}
	cursor := ""
	if len(items) > 50 {
		items = items[:50]
		last := items[len(items)-1]
		cursor = encodeTaskListCursor(binding, last.OccurredAt, last.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "items": items, "nextCursor": cursor})
}
