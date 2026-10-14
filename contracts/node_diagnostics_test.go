package contracts

import (
	"testing"
)

func Test节点诊断契约保持有界只读且详情不降为通用对象(t *testing.T) {
	spec := loadOpenAPI(t)
	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	for _, name := range []string{"InstalledRuntimeVersion", "NodeRuntimeVersions", "AgentActiveExecution", "AgentExecutionObservation", "NodeLogQueue", "NodeEnvironmentCheck", "NodeAgentDiagnostics", "NodeActiveExecutions", "ExecutionNodeDetail", "AgentHostInventory", "NodeHostInventory", "NodeInspectionItem", "NodeDirectLoadConfiguration", "NodeRuntimeInspection", "NodeActivityItem", "NodeActivitySummary", "NodeRecentTask", "NodeRecentTasks", "AgentNodeEnvironmentInspectionCompletion"} {
		schema, ok := schemas[name].(map[string]any)
		if !ok || schema["additionalProperties"] != false {
			t.Fatalf("诊断契约允许任意字段: %s", name)
		}
		props := schema["properties"].(map[string]any)
		for _, forbidden := range []string{"rawOutput", "command", "machineCredential", "password", "message", "sql"} {
			if _, found := props[forbidden]; found {
				t.Fatalf("诊断契约扩大敏感数据范围: %s", name)
			}
		}
	}
	payload := schemas["AgentHeartbeatPayload"].(map[string]any)["properties"].(map[string]any)
	for _, name := range []string{"startedAt", "runtimeVersions", "executionObservation", "logQueue", "host"} {
		if _, ok := payload[name]; !ok {
			t.Fatalf("缺少采集契约: %s", name)
		}
	}
	paths := spec["paths"].(map[string]any)
	response := paths["/api/v1/execution-nodes/{nodeId}"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)
	if response["$ref"] != "#/components/responses/ExecutionNodeDetail" {
		t.Fatal("详情仍使用无边界通用响应")
	}
	completion := paths["/agent/v1/execution-node-environment-checks/{checkId}:complete"].(map[string]any)["post"].(map[string]any)["requestBody"].(map[string]any)
	if completion["$ref"] != "#/components/requestBodies/AgentNodeEnvironmentInspectionCompletion" {
		t.Fatal("完整检查仍使用允许自由载荷的占位请求体")
	}
	payload = schemas["AgentNodeEnvironmentInspectionCompletion"].(map[string]any)["properties"].(map[string]any)["payload"].(map[string]any)
	if payload["additionalProperties"] != false || payload["properties"].(map[string]any)["inspection"].(map[string]any)["$ref"] != "#/components/schemas/NodeRuntimeInspection" {
		t.Fatal("版本回执没有完整严格清单")
	}
	items := schemas["NodeRuntimeInspection"].(map[string]any)["properties"].(map[string]any)["items"].(map[string]any)
	if items["minItems"] != float64(10) || items["maxItems"] != float64(10) {
		t.Fatal("固定检查清单允许漏项或自由扩展")
	}
	host := schemas["NodeHostInventory"].(map[string]any)["properties"].(map[string]any)
	if _, raw := host["addresses"]; raw || host["addressPrefixes"] == nil {
		t.Fatal("节点浏览器契约允许完整主机地址")
	}
}
