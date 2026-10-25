package contracts

import (
	"fmt"
	"strings"
	"testing"
)

func Test任务草稿与模板复用显式类型边界(t *testing.T) {
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	for _, path := range []string{"/api/v1/task-drafts", "/api/v1/config-templates", "/api/v1/config-templates/{templateId}", "/api/v1/config-templates/{templateId}:create-draft"} {
		if _, exists := paths[path]; !exists {
			t.Fatalf("缺少任务复用入口 %s", path)
		}
	}
	schemas := object(t, object(t, spec, "components"), "schemas")
	for _, name := range []string{"TaskDerivedDraftEnvelope", "TemplateDraftCreatedEnvelope", "ConfigTemplateSummary"} {
		schema := object(t, schemas, name)
		assertRequiredProperties(t, schema, "taskType")
		types := fmt.Sprint(object(t, object(t, schema, "properties"), "taskType")["enum"])
		if !strings.Contains(types, "OBDUMPER_EXPORT") || !strings.Contains(types, "OBLOADER_NORMAL_IMPORT") || strings.Contains(types, "DIRECT_LOAD") != (name == "TaskDerivedDraftEnvelope") {
			t.Fatalf("%s 类型边界错误", name)
		}
	}
	properties := object(t, object(t, schemas, "ConfigTemplateSummary"), "properties")
	for _, name := range []string{"config", "command", "filePath", "credential"} {
		if _, exists := properties[name]; exists {
			t.Fatal("模板摘要包含完整配置或敏感上下文")
		}
	}
}
