package controlplane

import (
	"context"
	"net/netip"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/nodefacts"
	"ob-data-orch/internal/store"
)

// nodeEnvironmentCheckResponse 仅返回已有固定检查记录，不从版本信息或心跳推导检查成功。
type nodeEnvironmentCheckResponse struct {
	Inspection    *nodefacts.RuntimeInspection `json:"inspection,omitempty"`
	Source        string                       `json:"source,omitempty"`
	CheckID       string                       `json:"checkId,omitempty"`
	Status        string                       `json:"status"`
	Code          string                       `json:"code,omitempty"`
	FactsRevision int64                        `json:"factsRevision"`
	RequestedAt   *time.Time                   `json:"requestedAt,omitempty"`
	CompletedAt   *time.Time                   `json:"completedAt,omitempty"`
}

// nodeAgentDiagnosticsResponse 是节点对象范围内的安全身份、版本和队列投影。
// 配置摘要不是凭据；工具版本只来自安装元数据，不授予执行资格。
type nodeAgentDiagnosticsResponse struct {
	Host                               *nodeHostInventoryResponse `json:"host,omitempty"`
	AgentID                            string                     `json:"agentId"`
	ProtocolVersion                    string                     `json:"protocolVersion"`
	FactsRevision                      int64                      `json:"factsRevision"`
	StartedAt                          *time.Time                 `json:"startedAt,omitempty"`
	RuntimeConfigurationDigest         string                     `json:"runtimeConfigurationDigest,omitempty"`
	ExpectedRuntimeConfigurationDigest string                     `json:"expectedRuntimeConfigurationDigest"`
	RuntimeVersions                    *nodefacts.RuntimeVersions `json:"runtimeVersions,omitempty"`
	LogQueue                           *nodefacts.LogQueue        `json:"logQueue,omitempty"`
}

type nodeHostInventoryResponse struct {
	Hostname        string   `json:"hostname,omitempty"`
	AddressPrefixes []string `json:"addressPrefixes"`
	OSName          string   `json:"osName,omitempty"`
	OSVersion       string   `json:"osVersion,omitempty"`
	CPUModel        string   `json:"cpuModel,omitempty"`
	LogicalCPUs     int      `json:"logicalCpus"`
	PhysicalCores   *int     `json:"physicalCores,omitempty"`
}

type nodeRecentTaskResponse struct {
	ID                     string     `json:"id"`
	Type                   string     `json:"type"`
	State                  string     `json:"state"`
	ReconciliationRequired bool       `json:"reconciliationRequired"`
	SubmittedAt            time.Time  `json:"submittedAt"`
	StartedAt              *time.Time `json:"startedAt,omitempty"`
	FinishedAt             *time.Time `json:"finishedAt,omitempty"`
}

type nodeRecentTasksResponse struct {
	Scope string                   `json:"scope"`
	Items []nodeRecentTaskResponse `json:"items"`
}

func nodeHostInventory(host *nodefacts.HostInventory) *nodeHostInventoryResponse {
	if host == nil {
		return nil
	}
	result := &nodeHostInventoryResponse{Hostname: host.Hostname, OSName: host.OSName, OSVersion: host.OSVersion, CPUModel: host.CPUModel, LogicalCPUs: host.LogicalCPUs, PhysicalCores: host.PhysicalCores, AddressPrefixes: make([]string, 0, len(host.Addresses))}
	seen := make(map[string]bool)
	for _, address := range host.Addresses {
		ip, err := netip.ParseAddr(address)
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		bits := 64
		if ip.Is4() {
			bits = 24
		}
		prefix := netip.PrefixFrom(ip, bits).Masked().String()
		if !seen[prefix] {
			result.AddressPrefixes = append(result.AddressPrefixes, prefix)
			seen[prefix] = true
		}
	}
	return result
}

type nodeActiveExecutionResponse struct {
	nodefacts.ActiveExecution
	TaskID string `json:"taskId"`
}

type nodeActiveExecutionsResponse struct {
	Scope string                        `json:"scope"`
	Items []nodeActiveExecutionResponse `json:"items"`
}

func nodeAgentDiagnostics(node store.ExecutionNode) *nodeAgentDiagnosticsResponse {
	if node.Agent == nil {
		return nil
	}
	agent, facts := node.Agent, node.Agent.EnvironmentFacts
	response := &nodeAgentDiagnosticsResponse{
		Host:    nodeHostInventory(facts.Host),
		AgentID: agent.AgentID, ProtocolVersion: agent.ProtocolVersion, FactsRevision: agent.FactsRevision,
		StartedAt: facts.StartedAt, RuntimeConfigurationDigest: facts.RuntimeConfigurationDigest,
		ExpectedRuntimeConfigurationDigest: store.ExecutionNodeRuntimeConfigurationDigest(node), LogQueue: facts.LogQueue,
	}
	if facts.RuntimeConfigurationDigest == response.ExpectedRuntimeConfigurationDigest {
		response.RuntimeVersions = facts.RuntimeVersions
	}
	return response
}

func (s *Server) newAuthorizedExecutionNodeDetailResponse(ctx context.Context, principal identity.Principal, node store.ExecutionNode) executionNodeDetailResponse {
	response := s.newExecutionNodeDetailResponse(node)
	s.enrichNodeDetail(ctx, principal, node, &response)
	return response
}

// enrichNodeDetail 让详情复用已有预算与任务统计读取能力；读取失败保持未知。
// 诊断降级不能把已成功的管理写事务误报为失败，也不能用空值伪造零占用。
func (s *Server) enrichNodeDetail(ctx context.Context, principal identity.Principal, node store.ExecutionNode, response *executionNodeDetailResponse) {
	globalRead := s.hasGlobalTaskRead(ctx, principal)
	if node.Agent != nil && node.Agent.EnvironmentFacts.RuntimeInspection != nil && node.Agent.EnvironmentFacts.RuntimeInspection.RuntimeConfigurationDigest == store.ExecutionNodeRuntimeConfigurationDigest(node) {
		response.EnvironmentCheck.Inspection = node.Agent.EnvironmentFacts.RuntimeInspection
	}
	if reader, ok := s.nodeManagement.(interface {
		ReadNodeActivity(context.Context, string, string, string) (store.NodeActivitySummary, error)
	}); ok {
		if activity, err := reader.ReadNodeActivity(ctx, node.NodeID, principal.ID, node.EnvironmentCheck.CheckID); err == nil {
			response.Activity = &activity
			response.EnvironmentCheck.Source = activity.CheckSource
		}
	}
	if s.tasks != nil {
		if tasks, err := s.tasks.ListTaskSummaries(ctx, store.TaskListQuery{SubjectID: principal.ID, NodeID: node.NodeID, GlobalRead: globalRead, Limit: 5}); err == nil {
			list := &nodeRecentTasksResponse{Scope: "READABLE", Items: make([]nodeRecentTaskResponse, 0)}
			if globalRead {
				list.Scope = "ALL"
			}
			if len(tasks) > 5 {
				tasks = tasks[:5]
			}
			for _, task := range tasks {
				item := nodeRecentTaskResponse{ID: task.TaskID, Type: task.TaskType, State: task.State, ReconciliationRequired: task.ReconciliationRequired, SubmittedAt: task.SubmittedAt}
				if !task.StartedAt.IsZero() {
					value := task.StartedAt
					item.StartedAt = &value
				}
				if !task.FinishedAt.IsZero() {
					value := task.FinishedAt
					item.FinishedAt = &value
				}
				list.Items = append(list.Items, item)
			}
			response.RecentTasks = list
		}
	}
	if reader, ok := s.nodeManagement.(nodeBudgetReader); ok {
		if usages, err := reader.ListNodeBudgetUsages(ctx); err == nil {
			attachNodeBudget(&response.executionNodeListResponse, node, usages[node.NodeID])
		} else {
			blockNodeBudget(&response.executionNodeListResponse, "EXECUTION_BUDGET_UNKNOWN")
		}
	}
	if reader, ok := s.tasks.(interface {
		ListNodeTaskSummaries(context.Context, string, bool) (map[string]store.NodeTaskSummary, error)
	}); ok {
		if summaries, err := reader.ListNodeTaskSummaries(ctx, principal.ID, globalRead); err == nil {
			summary := summaries[node.NodeID]
			summary.Scope = "MINE"
			if globalRead {
				summary.Scope = "ALL"
			}
			response.TaskSummary = &summary
		}
	}
	if node.Agent == nil || node.Agent.EnvironmentFacts.ExecutionObservation == nil {
		return
	}
	reader, ok := s.tasks.(interface {
		ListAuthorizedNodeExecutionBindings(context.Context, string, string, string, bool) ([]store.NodeExecutionBinding, error)
	})
	if !ok {
		return
	}
	bindings, err := reader.ListAuthorizedNodeExecutionBindings(ctx, node.NodeID, node.Agent.AgentID, principal.ID, globalRead)
	if err != nil {
		return
	}
	// 自报 ID 不能直接投影；同时匹配当前 Agent、节点、授权任务和租约三元组才允许出现。
	allowed := make(map[string]store.NodeExecutionBinding, len(bindings))
	for _, binding := range bindings {
		allowed[binding.ExecutionID] = binding
	}
	active := &nodeActiveExecutionsResponse{Scope: "READABLE", Items: make([]nodeActiveExecutionResponse, 0)}
	if globalRead {
		active.Scope = "ALL"
	}
	for _, observation := range node.Agent.EnvironmentFacts.ExecutionObservation.Active {
		if binding, found := allowed[observation.ExecutionID]; found && binding.LeaseID == observation.LeaseID && binding.LeaseEpoch == observation.LeaseEpoch {
			active.Items = append(active.Items, nodeActiveExecutionResponse{ActiveExecution: observation, TaskID: binding.TaskID})
		}
	}
	response.ActiveExecutions = active
}
