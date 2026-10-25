package contracts

import "testing"

func Test任务进度契约保留降级和固定安全口径(t *testing.T) {
	t.Parallel()
	root := loadOpenAPI(t)
	schemas := object(t, object(t, root, "components"), "schemas")
	progress := object(t, schemas, "TaskProgressRead")
	if progress["additionalProperties"] != false {
		t.Fatal("进度摘要允许任意文本")
	}
	properties := object(t, progress, "properties")
	assertRequiredProperties(t, progress, "phase", "updatedAt", "incomplete")
	if len(properties) != 6 {
		t.Fatal("进度摘要超出六个已确认字段")
	}
	for _, field := range []string{"phase", "percent", "completedTasks", "runningTasks", "updatedAt", "incomplete"} {
		if properties[field] == nil {
			t.Fatal("固定进度字段缺失", field)
		}
	}
	for _, field := range []string{"message", "command", "path", "eta", "rows", "bytes", "credential"} {
		if properties[field] != nil {
			t.Fatal("进度包含未确认口径或原文", field)
		}
	}
	assertExactStringEnum(t, object(t, properties, "phase"), []string{"EXPORT_DATA", "IMPORT_DATA", "EXPORT_SCHEMA_COMPLETE", "IMPORT_SCHEMA_COMPLETE", "EXPORT_RECORD_COMPLETE", "IMPORT_RECORD_COMPLETE"})
	percent := object(t, properties, "percent")
	if percent["type"] != "number" || percent["minimum"] != float64(0) || percent["maximum"] != float64(100) || percent["multipleOf"] != 0.01 {
		t.Fatal("百分比边界或精度缺失")
	}
	for _, field := range []string{"completedTasks", "runningTasks"} {
		count := object(t, properties, field)
		if count["type"] != "integer" || count["minimum"] != float64(0) || count["maximum"] != float64(9007199254740991) {
			t.Fatal("子任务计数超出安全整数边界", field)
		}
	}
	shapes := progress["oneOf"].([]any)
	if len(shapes) != 2 {
		t.Fatal("数据进度与阶段事实没有互斥")
	}
	data := shapes[0].(map[string]any)
	assertRequiredProperties(t, data, "percent", "completedTasks", "runningTasks")
	assertExactStringEnum(t, object(t, object(t, data, "properties"), "phase"), []string{"EXPORT_DATA", "IMPORT_DATA"})
	stage := shapes[1].(map[string]any)
	assertExactStringEnum(t, object(t, object(t, stage, "properties"), "phase"), []string{"EXPORT_SCHEMA_COMPLETE", "IMPORT_SCHEMA_COMPLETE", "EXPORT_RECORD_COMPLETE", "IMPORT_RECORD_COMPLETE"})
	for index, field := range []string{"percent", "completedTasks", "runningTasks"} {
		forbidden := object(t, stage, "not")["anyOf"].([]any)
		assertRequiredProperties(t, forbidden[index].(map[string]any), field)
	}
	for _, name := range []string{"TaskListItem", "TaskExecutionRead"} {
		schema := object(t, schemas, name)
		assertRequiredProperties(t, schema, "stageEvidence", "progressEvidence")
		if object(t, object(t, schema, "properties"), "progress")["$ref"] != "#/components/schemas/TaskProgressRead" {
			t.Fatal("列表和详情未共用进度结构", name)
		}
		variants := schema["oneOf"].([]any)
		if len(variants) != 3 {
			t.Fatal("未知、阶段和百分比三种证据缺少互斥约束", name)
		}
		for index, evidence := range [][2]string{{"UNAVAILABLE", "UNAVAILABLE"}, {"RELIABLE", "PERCENT"}, {"RELIABLE", "UNAVAILABLE"}} {
			variant := variants[index].(map[string]any)
			values := object(t, variant, "properties")
			if object(t, values, "stageEvidence")["const"] != evidence[0] || object(t, values, "progressEvidence")["const"] != evidence[1] {
				t.Fatal("证据级别出现未确认组合", name)
			}
			if index == 0 {
				assertRequiredProperties(t, object(t, variant, "not"), "progress")
				continue
			}
			assertRequiredProperties(t, variant, "progress")
			amount := object(t, values, "progress")
			if index == 2 {
				amount = object(t, amount, "not")
			}
			assertRequiredProperties(t, amount, "percent")
		}
	}
}
