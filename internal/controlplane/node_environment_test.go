package controlplane

import (
	"testing"
	"time"

	"ob-data-orch/internal/store"
)

func Test节点复验分别投影待授权待核验与环境异常(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name, checkStatus, code, want, reason string
		authorized                            bool
	}{
		{"未授权的旧通过结论", "PASSED", "TOOL_RUNTIME_READY", "UNCONFIRMED", "ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED", false},
		{"未授权的自动检查", "PENDING", "", "UNCONFIRMED", "ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED", false},
		{"必需证据未知", "FAILED", "TOOL_RUNTIME_UNAVAILABLE", "UNCONFIRMED", "TOOL_RUNTIME_UNAVAILABLE", true},
		{"真实检查失败", "FAILED", "TOOL_RUNTIME_INVALID", "ABNORMAL", "TOOL_RUNTIME_INVALID", true},
		{"当前完整检查通过", "PASSED", "TOOL_RUNTIME_READY", "NORMAL", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := store.ExecutionNode{NodeID: "synthetic-node", Platform: "WINDOWS_AMD64", ManagementState: "ENABLED", EnvironmentCheck: store.ExecutionNodeEnvironmentCheck{Status: tc.checkStatus, Code: tc.code, FactsRevision: 2, CompletedAt: &now}, Agent: &store.ExecutionNodeAgent{AgentID: "synthetic-agent", LastHeartbeatAt: &now, FactsRevision: 2, CapacityTotal: 1, EnvironmentInspectionAuthorized: tc.authorized, EnvironmentFacts: store.AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64"}}}
			response := (&Server{heartbeatTTL: time.Minute}).newExecutionNodeListResponse(node)
			if response.EnvironmentStatus != tc.want || response.AcceptsNewTasks != (tc.want == "NORMAL") {
				t.Fatalf("环境投影或接收资格不正确：%s %t", response.EnvironmentStatus, response.AcceptsNewTasks)
			}
			if tc.reason != "" && (len(response.UnavailableReasons) != 1 || response.UnavailableReasons[0] != tc.reason) {
				t.Fatalf("阻断原因不正确：%v", response.UnavailableReasons)
			}
			node.ManagementState = "DISABLED"
			if response := (&Server{heartbeatTTL: time.Minute}).newExecutionNodeListResponse(node); response.AcceptsNewTasks || response.ManagementState != "DISABLED" {
				t.Fatal("完整检查通过隐式启用了禁用节点")
			}
		})
	}
}
