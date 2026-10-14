package controlplane

import (
	"testing"

	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/store"
)

func Test执行额度只投影可信预算且不覆盖任务槽位类别(t *testing.T) {
	node := store.ExecutionNode{NodeID: "node-1", Platform: "WINDOWS_AMD64", ToolHome: "E:/synthetic", JavaPath: "E:/synthetic/java", AllowedRoots: []string{"/E:/synthetic"}, Agent: &store.ExecutionNodeAgent{}}
	node.Agent.EnvironmentFacts.MemoryTotalBytes = 16 * executionbudget.GiB
	node.Agent.EnvironmentFacts.RuntimeConfigurationDigest = store.ExecutionNodeRuntimeConfigurationDigest(node)
	for _, tc := range []struct {
		usage store.NodeBudgetUsage
		known bool
	}{{store.NodeBudgetUsage{UsedBytes: 5 * executionbudget.GiB, ReservedBytes: 5 * executionbudget.GiB}, true}, {store.NodeBudgetUsage{Unknown: true}, false}, {store.NodeBudgetUsage{UsedBytes: 17 * executionbudget.GiB}, false}} {
		response := executionNodeListResponse{CapacityStatus: "AVAILABLE", AcceptsNewTasks: true}
		attachNodeBudget(&response, node, tc.usage)
		if (response.ExecutionQuota != nil) != tc.known || response.CapacityStatus != "AVAILABLE" {
			t.Fatal("预算事实或类别被伪造", response)
		}
		if tc.known {
			if response.ExecutionQuota.UsedGiB != 5 || response.ExecutionQuota.ReservedGiB != 5 {
				t.Fatal("分配重复记账", response.ExecutionQuota)
			}
		} else if response.AcceptsNewTasks {
			t.Fatal("未知预算允许领取")
		}
	}
	node.Agent.EnvironmentFacts.MemoryTotalBytes = 0
	response := executionNodeListResponse{AcceptsNewTasks: true}
	attachNodeBudget(&response, node, store.NodeBudgetUsage{})
	if response.ExecutionQuota != nil || response.AcceptsNewTasks {
		t.Fatal("缺失采样显示零占用可用额度")
	}
}
