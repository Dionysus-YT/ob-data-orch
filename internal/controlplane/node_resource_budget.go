package controlplane

import (
	"context"

	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/store"
)

// nodeExecutionQuotaResponse 使用 GiB 预算显示，不能将它解释为进程实时内存使用量。
type nodeExecutionQuotaResponse struct {
	TotalGiB    float64 `json:"totalGiB"`
	ReservedGiB float64 `json:"reservedGiB"`
	UsedGiB     float64 `json:"usedGiB"`
}

type nodeBudgetReader interface {
	ListNodeBudgetUsages(context.Context) (map[string]store.NodeBudgetUsage, error)
}

// attachNodeBudget 只在配置匹配、真实内存和全部分配均可核对时投影额度。
// 已离线节点仍展示最后可信总量和持久占用，离线不释放已有预算。
func attachNodeBudget(response *executionNodeListResponse, node store.ExecutionNode, usage store.NodeBudgetUsage) {
	if node.Agent == nil || usage.Unknown || node.Agent.EnvironmentFacts.RuntimeConfigurationDigest != store.ExecutionNodeRuntimeConfigurationDigest(node) {
		blockNodeBudget(response, "EXECUTION_BUDGET_UNKNOWN")
		return
	}
	total, err := node.BudgetPolicy.Total(node.Agent.EnvironmentFacts.MemoryTotalBytes)
	if err != nil || usage.UsedBytes > total || usage.ReservedBytes > total-usage.UsedBytes {
		blockNodeBudget(response, "EXECUTION_BUDGET_UNKNOWN")
		return
	}
	response.ExecutionQuota = &nodeExecutionQuotaResponse{
		TotalGiB: float64(total) / float64(executionbudget.GiB), ReservedGiB: float64(usage.ReservedBytes) / float64(executionbudget.GiB), UsedGiB: float64(usage.UsedBytes) / float64(executionbudget.GiB),
	}
	minimum, costErr := node.BudgetPolicy.Cost(1024)
	if costErr != nil || total-usage.UsedBytes-usage.ReservedBytes < minimum {
		response.AcceptsNewTasks = false
		response.UnavailableReasons = append(response.UnavailableReasons, "EXECUTION_BUDGET_EXHAUSTED")
	}
}

func blockNodeBudget(response *executionNodeListResponse, reason string) {
	response.AcceptsNewTasks = false
	response.UnavailableReasons = append(response.UnavailableReasons, reason)
}
