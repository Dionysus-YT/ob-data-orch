package contracts

import "testing"

// Test工具与兼容契约保持只读与严格投影 验证固定方法、无缓存和关闭未知字段。
func Test工具与兼容契约保持只读与严格投影(t *testing.T) {
	spec := loadOpenAPI(t)
	path := object(t, object(t, spec, "paths"), "/api/v1/settings/tools")
	if len(path) != 1 {
		t.Fatal("工具状态意外开放写操作")
	}
	get := object(t, path, "get")
	if _, exists := get["requestBody"]; exists || len(toolContractArray(t, get, "parameters")) != 0 {
		t.Fatal("工具读取接受了额外输入")
	}
	response := object(t, object(t, get, "responses"), "200")
	cache := object(t, object(t, object(t, response, "headers"), "Cache-Control"), "schema")
	if cache["const"] != "no-store" {
		t.Fatal("工具状态允许缓存")
	}
	schemas := object(t, object(t, spec, "components"), "schemas")
	for name, count := range map[string]int{"ToolCompatibilitySnapshot": 4, "ToolCompatibilityTool": 4, "ToolCompatibilityModule": 3, "ToolCatalogVersion": 2, "ToolCompatibilityResponse": 2} {
		schema := object(t, schemas, name)
		if schema["additionalProperties"] != false || len(toolContractArray(t, schema, "required")) != count || len(object(t, schema, "properties")) != count {
			t.Fatalf("%s 未关闭未知字段", name)
		}
	}
	properties := object(t, object(t, schemas, "ToolCompatibilitySnapshot"), "properties")
	if object(t, properties, "source")["const"] != "BUILT_IN_CATALOGS" || len(toolContractArray(t, object(t, properties, "tools"), "prefixItems")) != 2 {
		t.Fatal("缺少内置来源或两种固定工具边界")
	}
}

func toolContractArray(t *testing.T, parent map[string]any, key string) []any {
	t.Helper()
	items, ok := parent[key].([]any)
	if !ok {
		t.Fatalf("%s 应为数组", key)
	}
	return items
}
