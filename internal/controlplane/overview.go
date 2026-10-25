package controlplane

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/overview"
	"ob-data-orch/internal/store"
)

// OverviewReader 是首页固定只读快照端口，不具有执行、秘密解析或任意查询能力。
type OverviewReader interface {
	ReadOverviewNodeIDs(context.Context, string) ([]string, error)
	ReadOverviewFacts(context.Context, overview.Query) (store.OverviewFacts, error)
}

// getOverview 只处理固定参数与 HTTP 错误，快照读取和业务投影分别由存储及 overview 所有。
func (s *Server) getOverview(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.overview == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "OVERVIEW_UNAVAILABLE", "首页快照依赖尚未配置，请联系管理员检查控制面版本。", false)
		return
	}
	values, parseErr := url.ParseQuery(r.URL.RawQuery)
	if parseErr != nil {
		writeError(w, http.StatusBadRequest, "OVERVIEW_QUERY_INVALID", "首页时间范围无效。", false)
		return
	}
	for key, value := range values {
		if key != "range" || len(value) != 1 {
			writeError(w, http.StatusBadRequest, "OVERVIEW_QUERY_INVALID", "首页时间范围无效。", false)
			return
		}
	}
	period := "24h"
	if value, exists := values["range"]; exists {
		period = value[0]
	}
	query, err := overview.NewQuery(principal.ID, s.hasGlobalTaskRead(r.Context(), principal), time.Now().UTC(), period)
	if err != nil {
		writeError(w, http.StatusBadRequest, "OVERVIEW_QUERY_INVALID", "首页时间范围无效。", false)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	s.prepareOverviewNodeScope(ctx, principal, &query)
	facts, err := s.overview.ReadOverviewFacts(ctx, query)
	if errors.Is(err, store.ErrOverviewScopeDenied) {
		writeError(w, http.StatusForbidden, "OVERVIEW_SCOPE_DENIED", "当前身份没有可查看的首页范围。请核对账号与对象授权。", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "OVERVIEW_UNAVAILABLE", "无法读取首页快照，请稍后重试。", true)
		return
	}
	projection := s.projectOverview(ctx, principal, query, facts)
	if ctx.Err() != nil {
		writeError(w, http.StatusServiceUnavailable, "OVERVIEW_UNAVAILABLE", "首页快照读取超时，请重试。", true)
		return
	}
	if projection.Metrics.Status == "UNAVAILABLE" && projection.Trend.Status == "UNAVAILABLE" && projection.Nodes.Status == "UNAVAILABLE" && projection.MyTasks.Status == "UNAVAILABLE" && projection.Attention.Status == "UNAVAILABLE" {
		// 没有任何可信分区时按整次读取失败处理，使页面保留上一份整快照而非用全未知覆盖。
		writeError(w, http.StatusServiceUnavailable, "OVERVIEW_UNAVAILABLE", "所有首页事实分区暂不可用，请重试。", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": projection})
}

// prepareOverviewNodeScope 在快照事务前完成节点授权，只把通过的标识交给事实读取端。
func (s *Server) prepareOverviewNodeScope(ctx context.Context, principal identity.Principal, query *overview.Query) {
	ids, err := s.overview.ReadOverviewNodeIDs(ctx, principal.ID)
	if err != nil {
		query.NodeReadUnavailable = true
		return
	}
	query.NodeIDs = []string{}
	for _, id := range ids {
		if identity.Can(ctx, s.authorizer, principal, identity.ScopeNodeManage, id) == nil {
			query.NodeIDs = append(query.NodeIDs, id)
		}
	}
}

// projectOverview 在事务结束后再次校验节点权限，隔离读取期间的撤销；授权服务不在 SQLite 事务内执行。
// 统计与异常只使用通过相同授权的节点；首页不因管理员角色自动扩大对象范围。
func (s *Server) projectOverview(ctx context.Context, principal identity.Principal, query overview.Query, facts store.OverviewFacts) overview.Snapshot {
	input := overview.Facts{TasksRestricted: facts.TasksRestricted, MyTasksRestricted: facts.MyTasksRestricted, Metrics: facts.Metrics, Trend: facts.Trend, MyTasks: overviewTasks(facts.MyTasks), Failures: overviewTasks(facts.Failures)}
	heartbeatTTL, timingErr := s.currentHeartbeatTimeout(ctx)
	if facts.Nodes != nil && timingErr == nil {
		nodes := []overview.Node{}
		for _, raw := range *facts.Nodes {
			if identity.Can(ctx, s.authorizer, principal, identity.ScopeNodeManage, raw.NodeID) != nil {
				continue
			}
			projection := s.newExecutionNodeListResponseAt(raw, query.Now, heartbeatTTL)
			attachNodeBudget(&projection, raw, facts.Budgets[raw.NodeID])
			node := overview.Node{ID: raw.NodeID, DisplayName: raw.DisplayName, ManagementState: projection.ManagementState,
				HeartbeatStatus: projection.HeartbeatStatus, EnvironmentStatus: projection.EnvironmentStatus, CapacityStatus: projection.CapacityStatus,
				AcceptsNewTasks: projection.AcceptsNewTasks, UnavailableReasons: projection.UnavailableReasons, LastHeartbeatAt: projection.LastHeartbeatAt, UpdatedAt: raw.UpdatedAt}
			if raw.Agent != nil {
				if raw.Agent.CapacityTotal > 0 {
					node.CapacityTotal, node.CapacityUsed = &raw.Agent.CapacityTotal, &raw.Agent.CapacityUsed
				}
				if !raw.Agent.EnvironmentFacts.ObservedAt.IsZero() {
					node.ObservedAt = &raw.Agent.EnvironmentFacts.ObservedAt
					node.CPUUsagePercent, node.MemoryUsagePercent = raw.Agent.EnvironmentFacts.CPUUsagePercent, raw.Agent.EnvironmentFacts.MemoryUsagePercent
				}
			}
			nodes = append(nodes, node)
		}
		input.Nodes = &nodes
	}
	return overview.Build(query, input)
}

func overviewTasks(items *[]store.TaskListItem) *[]overview.Task {
	if items == nil {
		return nil
	}
	result := []overview.Task{}
	for _, item := range *items {
		stage, progress := taskProgressEvidence(item.Progress)
		task := overview.Task{ID: item.TaskID, DisplayName: taskDisplayName(item.Database, item.Table, item.ScopeKind), Type: item.TaskType, State: item.State,
			ReconciliationRequired: item.ReconciliationRequired, StageEvidence: stage, ProgressEvidence: progress, UpdatedAt: item.UpdatedAt}
		if !item.StartedAt.IsZero() {
			at := item.StartedAt
			task.StartedAt = &at
		}
		if !item.FinishedAt.IsZero() {
			at := item.FinishedAt
			task.FinishedAt = &at
		}
		if item.Progress != nil {
			p := item.Progress
			task.Progress = &overview.Progress{Phase: p.Phase, Percent: p.Percent, CompletedTasks: p.CompletedTasks, RunningTasks: p.RunningTasks, UpdatedAt: p.UpdatedAt, Incomplete: p.Incomplete}
		}
		result = append(result, task)
	}
	return &result
}
