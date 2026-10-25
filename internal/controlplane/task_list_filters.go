package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// taskListStates 是任务中心可筛选的已提交任务状态；草稿不属于已提交任务。
var taskListStates = map[string]bool{
	"WAITING_SCHEDULE": true, "STARTING": true, "RUNNING": true, "CANCELLING": true,
	"SUCCEEDED": true, "FAILED": true, "CANCELLED": true,
}

// taskListTypes 是当前已准入的任务类型；未准入的导入类型拒绝而非静默忽略。
var taskListTypes = map[string]bool{"OBDUMPER_EXPORT": true, "OBLOADER_NORMAL_IMPORT": true, "OBLOADER_DIRECT_LOAD": true}

var taskListEnvironments = map[string]bool{"DEVELOPMENT": true, "TEST": true, "STAGING": true, "PRODUCTION": true}

// errTaskScopeDenied 表示请求的创建人或全部范围超出当前身份可信读取范围。
var errTaskScopeDenied = fmt.Errorf("task scope denied")

// parseTaskListFilters 把浏览器查询参数解析为只收窄授权范围的筛选。
// 单值参数重复、未知枚举、超长或非法时间都失败关闭，不忽略后继续查询。
func parseTaskListFilters(values url.Values, query *store.TaskListQuery) error {
	single := func(name string, maxLength int) (string, error) {
		raw, present := values[name]
		if !present {
			return "", nil
		}
		if len(raw) != 1 {
			return "", fmt.Errorf("%s repeated", name)
		}
		value := strings.TrimSpace(raw[0])
		if len(value) > maxLength || strings.ContainsAny(value, "\r\n\x00") {
			return "", fmt.Errorf("%s invalid", name)
		}
		return value, nil
	}
	list := func(name string, allowed map[string]bool) ([]string, error) {
		raw := values[name]
		if len(raw) > 1 {
			return nil, fmt.Errorf("%s repeated", name)
		}
		if len(raw) == 0 || strings.TrimSpace(raw[0]) == "" {
			return nil, nil
		}
		parts := strings.Split(raw[0], ",")
		if len(parts) > 8 {
			return nil, fmt.Errorf("%s too many", name)
		}
		seen := map[string]bool{}
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !allowed[part] {
				return nil, fmt.Errorf("%s unsupported", name)
			}
			if !seen[part] {
				seen[part] = true
				result = append(result, part)
			}
		}
		return result, nil
	}
	instant := func(name string) (time.Time, error) {
		value, err := single(name, 64)
		if err != nil || value == "" {
			return time.Time{}, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s invalid", name)
		}
		return parsed.UTC(), nil
	}
	var err error
	if query.Keyword, err = single("keyword", 128); err != nil {
		return err
	}
	if query.Types, err = list("type", taskListTypes); err != nil {
		return err
	}
	if query.States, err = list("state", taskListStates); err != nil {
		return err
	}
	if query.CreatedFrom, err = instant("createdFrom"); err != nil {
		return err
	}
	if query.CreatedTo, err = instant("createdTo"); err != nil {
		return err
	}
	if query.StartedFrom, err = instant("startedFrom"); err != nil {
		return err
	}
	if query.StartedTo, err = instant("startedTo"); err != nil {
		return err
	}
	if query.FinishedFrom, err = instant("finishedFrom"); err != nil {
		return err
	}
	if query.FinishedTo, err = instant("finishedTo"); err != nil {
		return err
	}
	if (!query.CreatedFrom.IsZero() && !query.CreatedTo.IsZero() && !query.CreatedFrom.Before(query.CreatedTo)) ||
		(!query.StartedFrom.IsZero() && !query.StartedTo.IsZero() && !query.StartedFrom.Before(query.StartedTo)) ||
		(!query.FinishedFrom.IsZero() && !query.FinishedTo.IsZero() && !query.FinishedFrom.Before(query.FinishedTo)) {
		return fmt.Errorf("time range invalid")
	}
	if query.DataSourceID, err = single("dataSourceId", 256); err != nil || strings.ContainsAny(query.DataSourceID, "/\\") {
		return fmt.Errorf("dataSourceId invalid")
	}
	if query.Environment, err = single("environment", 32); err != nil || (query.Environment != "" && !taskListEnvironments[query.Environment]) {
		return fmt.Errorf("environment invalid")
	}
	if query.CreatorID, err = single("creatorId", 256); err != nil || strings.ContainsAny(query.CreatorID, "/\\") {
		return fmt.Errorf("creatorId invalid")
	}
	return nil
}

// authorizeTaskCreatorFilter 只允许可信全局读取者按任意创建人筛选；其他身份只能筛选本人。
func authorizeTaskCreatorFilter(query *store.TaskListQuery) error {
	if query.CreatorID == "" || query.CreatorID == query.SubjectID || query.GlobalRead {
		return nil
	}
	return errTaskScopeDenied
}

// taskListFilterDigestSubject 将主体与全部筛选绑定到游标，任何条件变化都使旧游标失效。
func taskListFilterDigestSubject(query store.TaskListQuery) string {
	encoded, _ := json.Marshal([]any{
		query.SubjectID, query.NodeID, query.OwnedOnly, query.GlobalRead, query.Keyword, query.Types, query.States,
		optionalTaskTime(query.CreatedFrom), optionalTaskTime(query.CreatedTo), optionalTaskTime(query.StartedFrom),
		optionalTaskTime(query.StartedTo), query.DataSourceID, query.Environment, query.CreatorID,
		optionalTaskTime(query.FinishedFrom), optionalTaskTime(query.FinishedTo),
	})
	return "task-list-filter-v2|" + string(encoded)
}

// taskDisplayName 由冻结快照派生任务展示名，不新增持久名称（DEC-050）。
func taskDisplayName(database, table, scopeKind string) string {
	switch {
	case database != "" && scopeKind == "QUERY_RESULT":
		return database + " · 查询结果集"
	case database != "" && scopeKind == "ALL":
		return database + "（全部对象）"
	case database != "" && strings.Contains(table, ","):
		return fmt.Sprintf("%s（%d 个对象）", database, len(strings.Split(table, ",")))
	case database != "" && table != "":
		return database + "." + table
	case database != "":
		return database
	default:
		return "导出任务"
	}
}

// taskListActions 投影列表行可用操作。它只是展示资格：每个写操作仍由对应接口在事务内复验。
// 写操作要求当前主体可写（创建者或数据源任务操作范围），全局只读不授予写入口。
func taskListActions(item store.TaskListItem, realExecutionEnabled, templatesConfigured bool) []string {
	actions := []string{"COPY_COMMAND"}
	if item.State != "WAITING_SCHEDULE" {
		actions = append(actions, "VIEW_LOGS")
	}
	// 旁路只复用配置或从头重检，不接入导出检查点、模板与运行中取消。
	if item.TaskType == "OBLOADER_DIRECT_LOAD" {
		if item.Operable && !item.ReconciliationRequired && item.SnapshotVersion == "direct-load-v1" {
			switch item.State {
			case "SUCCEEDED":
				actions = append(actions, "REBUILD_FROM_CONFIG")
			case "FAILED", "CANCELLED":
				actions = append(actions, "REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH")
			}
		}
		return actions
	}
	// 普通导入使用自己的快照与草稿复用链，检查点证据尚未接入，不取得 dump.ckpt 资格。
	if item.TaskType == "OBLOADER_NORMAL_IMPORT" {
		if item.Operable && realExecutionEnabled && !item.ReconciliationRequired && !item.CancellationRequested && (item.State == "WAITING_SCHEDULE" || item.State == "STARTING" || item.State == "RUNNING") {
			actions = append(actions, "CANCEL")
		}
		if item.Operable && !item.ReconciliationRequired && item.SnapshotVersion == "normal-import-v1" {
			switch item.State {
			case "SUCCEEDED":
				actions = append(actions, "REBUILD_FROM_CONFIG")
				if templatesConfigured {
					actions = append(actions, "SAVE_TEMPLATE")
				}
			case "FAILED", "CANCELLED":
				actions = append(actions, "REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH")
			}
		}
		return actions
	}
	if item.TaskType != "OBDUMPER_EXPORT" {
		return actions
	}
	if !item.Operable {
		return actions
	}
	switch item.State {
	case "WAITING_SCHEDULE", "STARTING", "RUNNING":
		if realExecutionEnabled && !item.ReconciliationRequired && !item.CancellationRequested {
			actions = append(actions, "CANCEL")
		}
	case "SUCCEEDED":
		if item.SnapshotVersion == "v2" && !item.ReconciliationRequired {
			actions = append(actions, "REBUILD_FROM_CONFIG")
		}
		if templatesConfigured && item.SnapshotVersion == "v2" && !item.ReconciliationRequired {
			actions = append(actions, "SAVE_TEMPLATE")
		}
	case "FAILED", "CANCELLED":
		if item.ReconciliationRequired {
			break
		}
		if item.SnapshotVersion == "v2" {
			actions = append(actions, "REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH")
		}
		if item.State == "FAILED" && item.CheckpointPresent && item.DerivationKind != "CHECKPOINT_RESUME" {
			actions = append(actions, "CHECKPOINT_RESUME")
		}
	}
	return actions
}

// taskFilterOptionResponse 是筛选候选的浏览器投影。
type taskFilterOptionResponse struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// listTaskFilterOptions 返回当前授权范围内出现过的候选值。非全局身份不获得其他创建人标识。
func (s *Server) listTaskFilterOptions(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	reader, ok := s.tasks.(interface {
		ListTaskFilterOptions(context.Context, store.TaskListQuery) (store.TaskListFilterOptions, error)
	})
	if s.tasks == nil || !ok {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置任务查询依赖", false)
		return
	}
	if len(r.URL.Query()) > 0 {
		writeError(w, http.StatusBadRequest, "TASK_FILTER_INVALID", "任务筛选候选不接受查询参数", false)
		return
	}
	query := store.TaskListQuery{SubjectID: principal.ID, GlobalRead: s.hasGlobalTaskRead(r.Context(), principal)}
	options, err := reader.ListTaskFilterOptions(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_QUERY_UNAVAILABLE", "任务筛选候选暂时不可用", true)
		return
	}
	project := func(values []store.TaskListFilterOption) []taskFilterOptionResponse {
		result := make([]taskFilterOptionResponse, 0, len(values))
		for _, value := range values {
			result = append(result, taskFilterOptionResponse{ID: value.ID, Label: value.Label})
		}
		return result
	}
	creators := []taskFilterOptionResponse{}
	if query.GlobalRead {
		creators = project(options.Creators)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "dataSources": project(options.DataSources), "nodes": project(options.Nodes),
		"creators": creators, "environments": options.Environments, "creatorFilterAvailable": query.GlobalRead,
	})
}
