package contracts

import (
	"strings"
	"testing"
)

func Test首页只读快照与受限分区契约(t *testing.T) {
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	operation := object(t, object(t, paths, "/api/v1/overview"), "get")
	if operation["operationId"] != "getOverviewSnapshot" {
		t.Fatal("首页只读入口缺失")
	}
	if len(object(t, paths, "/api/v1/overview")) != 1 {
		t.Fatal("首页不应提供写操作")
	}
	schemas := object(t, object(t, spec, "components"), "schemas")
	snapshot := object(t, schemas, "OverviewSnapshot")
	assertRequiredProperties(t, snapshot, "snapshotAt", "timeZone", "timeZoneSource", "metrics", "trend", "nodes", "myTasks", "attention")
	props := object(t, snapshot, "properties")
	if object(t, props, "timeZone")["const"] != "UTC" || object(t, props, "timeZoneSource")["const"] != "BUILTIN_UTC" {
		t.Fatal("未明确实际时区来源")
	}
	for _, name := range []string{"OverviewMetricsSection", "OverviewTrendSection", "OverviewNodesSection", "OverviewMyTasksSection", "OverviewAttentionSection"} {
		variants := object(t, schemas, name)["oneOf"].([]any)
		restricted := variants[1].(map[string]any)
		if _, exists := object(t, restricted, "properties")["data"]; exists || restricted["additionalProperties"] != false {
			t.Fatal("未知或受限分区允许返回旧数据")
		}
	}
	for _, name := range []string{"OverviewTask", "OverviewNode", "OverviewAttentionItem"} {
		fields := object(t, object(t, schemas, name), "properties")
		for key := range fields {
			for _, forbidden := range []string{"password", "credential", "command", "toolHome", "javaPath", "allowedRoots"} {
				if strings.Contains(key, forbidden) {
					t.Fatalf("首页字段扩展出范围：%s.%s", name, key)
				}
			}
		}
	}
	parameters := object(t, object(t, paths, "/api/v1/tasks"), "get")["parameters"].([]any)
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if name, ok := parameter.(map[string]any)["name"].(string); ok {
			seen[name] = true
		}
	}
	if !seen["finishedFrom"] || !seen["finishedTo"] {
		t.Fatal("首页终态下钻没有对应列表筛选")
	}
}
