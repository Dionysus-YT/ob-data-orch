package contracts

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func Test旁路活动配置仅本地场景且历史读取不改写(t *testing.T) {
	t.Parallel()
	defs := object(t, loadDirectLoadContract(t), "$defs")
	local, ok := object(t, defs, "LocalConfig")["allOf"].([]any)
	if !ok || len(local) != 2 || variantObject(t, local[0])["$ref"] != "#/$defs/Config" {
		t.Fatal("本地配置未复用严格读取结构")
	}
	properties := object(t, variantObject(t, local[1]), "properties")
	assertExactStringEnum(t, object(t, properties, "scenario"), []string{"", "PRIVATE_ODP", "OBSERVER_DIRECT"})
	assertExactStringEnum(t, object(t, properties, "format"), []string{"", "CSV"})
	for _, name := range []string{"Write", "ExecutionInput"} {
		if object(t, object(t, object(t, defs, name), "properties"), "config")["$ref"] != "#/$defs/LocalConfig" {
			t.Fatal("活动写入或执行未收紧到本地范围", name)
		}
	}
	for _, name := range []string{"Draft", "Snapshot"} {
		if object(t, object(t, object(t, defs, name), "properties"), "config")["$ref"] != "#/$defs/Config" {
			t.Fatal("历史读取不应改写保存配置", name)
		}
	}
}

func Test旁路草稿来源锁定与独立门禁响应(t *testing.T) {
	t.Parallel()
	spec := loadDirectLoadContract(t)
	defs := object(t, spec, "$defs")
	draft := object(t, defs, "Draft")
	properties := object(t, draft, "properties")
	assertExactStringEnum(t, object(t, properties, "sourceDerivation"), []string{"REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH"})
	if object(t, properties, "sourceTaskId")["type"] != "string" || object(t, properties, "parametersLocked")["type"] != "boolean" {
		t.Fatal("旁路派生来源与参数锁定字段不完整")
	}
	for _, name := range []string{"Draft", "Check"} {
		flag := object(t, object(t, object(t, defs, name), "properties"), "executionEnabled")
		if flag["type"] != "boolean" || flag["default"] != false {
			t.Fatal("执行策略必须为默认关闭的独立布尔值", name)
		}
		if _, exists := flag["const"]; exists {
			t.Fatal("执行响应不能仍冻结为常量", name)
		}
	}
	submit := object(t, defs, "Submit")
	assertRequiredProperties(t, submit, "precheckId", "taskName", "confirmedConfigFingerprint")
	if submit["additionalProperties"] != false || len(object(t, submit, "properties")) != 3 {
		t.Fatal("旁路提交接受了浏览器伪造配置或事实")
	}
	root := loadOpenAPI(t)
	paths := object(t, root, "paths")
	operation := object(t, object(t, paths, "/api/v1/direct-load-drafts/{draftId}:submit"), "post")
	responses := object(t, operation, "responses")
	if responses["201"] == nil || responses["422"] == nil || strings.Contains(operation["description"].(string), "始终返回") {
		t.Fatal("提交契约没有区分默认门禁与显式开启后的安全提交")
	}
	schemas := object(t, object(t, root, "components"), "schemas")
	assertExactStringEnum(t, object(t, object(t, object(t, schemas, "TaskDraftListItem"), "properties"), "type"), []string{"OBDUMPER_EXPORT", "OBLOADER_NORMAL_IMPORT", "OBLOADER_DIRECT_LOAD"})
}

func Test旁路九项检查与完整参数目录不借用普通导入(t *testing.T) {
	t.Parallel()
	spec := loadDirectLoadContract(t)
	defs := object(t, spec, "$defs")
	results := object(t, object(t, object(t, defs, "Check"), "properties"), "results")
	if results["maxItems"] != float64(9) {
		t.Fatal("完整旁路预检查不能被旧六项容量截断")
	}
	checks := object(t, object(t, results, "items"), "properties")
	assertExactStringEnum(t, object(t, checks, "check"), []string{"TOOL_ENVIRONMENT", "NODE_RESOURCE", "INPUT_FILES", "DATABASE_CONNECTIVITY", "RPC_CONNECTIVITY", "DIRECT_LOAD_VERSION", "WRITE_ACCESS", "DIRECT_LOAD_STRUCTURE", "DIRECT_LOAD_CONFIG"})
	assertExactStringEnum(t, object(t, checks, "status"), []string{"PASSED", "FAILED", "UNKNOWN"})
	assertExactStringEnum(t, object(t, checks, "evidenceCode"), []string{"DIRECT_LOAD_VERIFIED", "DIRECT_LOAD_REJECTED", "DIRECT_LOAD_UNAVAILABLE", "DIRECT_LOAD_PROBE_NOT_CONFIGURED", "PRECHECK_BLOCKED_BY_TOOL", "PRECHECK_BLOCKED_BY_DATABASE", "DIRECT_LOAD_TABLE_NOT_FOUND", "DIRECT_LOAD_BIT_BLOCKED", "DIRECT_LOAD_BINARY_BLOCKED", "DIRECT_LOAD_VIRTUAL_BLOCKED", "DIRECT_LOAD_REPLACE_INDEX_BLOCKED", "DIRECT_LOAD_REPLACE_LOB_BLOCKED", "DIRECT_LOAD_TABLE_FACT_UNAVAILABLE"})
	checkProperties := object(t, object(t, defs, "Check"), "properties")
	if object(t, checkProperties, "directLoadConfigFingerprint")["$ref"] != "#/$defs/Digest" {
		t.Fatal("旁路节点三键安全投影没有独立摘要约束")
	}
	manifestFiles := object(t, object(t, object(t, defs, "Manifest"), "properties"), "files")
	if object(t, object(t, object(t, manifestFiles, "items"), "properties"), "kind")["const"] != "DATA" {
		t.Fatal("旁路输入文件接受了建表或其他对象类型")
	}
	catalog, ok := spec["parameterCatalog"].([]any)
	if !ok || len(catalog) != 103 {
		t.Fatal("旁路目录没有覆盖全部103参数")
	}
	seen := map[string]bool{}
	for _, raw := range catalog {
		row := variantObject(t, raw)
		name, valid := row["name"].(string)
		if !valid || !strings.HasPrefix(name, "--") || seen[name] {
			t.Fatal("旁路目录参数名重复或未保留canonical前缀")
		}
		seen[name] = true
		if name == "--retry" && row["supportState"] != "UNSUPPORTED" {
			t.Fatal("旁路目录开放了检查点重试")
		}
	}
	config := object(t, object(t, defs, "Config"), "properties")
	for _, forbidden := range []string{"taskName", "retry", "cleanup", "mapping", "execution", "objects"} {
		if config[forbidden] != nil {
			t.Fatal("旁路配置混入其他能力的字段", forbidden)
		}
	}
}

func TestOpenAPI旁路机器预检显式分型且拒绝混合上下文(t *testing.T) {
	t.Parallel()
	schemas := object(t, object(t, loadOpenAPI(t), "components"), "schemas")
	direct := object(t, loadDirectLoadContract(t), "$defs")
	claim := object(t, object(t, object(t, schemas, "AgentPrecheckClaimResponseEnvelope"), "properties"), "payload")
	claimProperties := object(t, claim, "properties")
	assertExactReferenceBranches(t, object(t, claimProperties, "executionContext"), "oneOf", "#/components/schemas/AgentPrecheckExecutionContext", "direct-load.json#/$defs/PrecheckExecutionContext")
	assertExactReferenceBranches(t, object(t, claimProperties, "checkSet"), "oneOf", "#/components/schemas/AgentPrecheckCheckSet", "direct-load.json#/$defs/PrecheckCheckSet")
	completion := object(t, object(t, schemas, "AgentPrecheckCompletionPayload"), "properties")
	assertExactReferenceBranches(t, object(t, completion, "results"), "oneOf", "#/components/schemas/AgentPrecheckResults", "direct-load.json#/$defs/PrecheckResults")
	assertExactReferenceBranches(t, object(t, completion, "inputManifest"), "anyOf", "normal-import.json#/$defs/Manifest", "direct-load.json#/$defs/Manifest")
	if object(t, completion, "directLoadConfigFingerprint")["$ref"] != "#/components/schemas/SHA256Digest" {
		t.Fatal("旁路机器完成载荷缺少独立安全配置摘要")
	}

	exportContext := object(t, schemas, "AgentPrecheckExecutionContext")
	directContext := object(t, direct, "PrecheckExecutionContext")
	if exportContext["additionalProperties"] != false || directContext["additionalProperties"] != false {
		t.Fatal("两种执行上下文均须关闭未知字段，混合字段不能落入任一分支")
	}
	assertRequiredProperties(t, directContext, "directLoad", "checkKind", "compatibilityMode", "database", "contentKind", "targetPlatform", "allowedRoots", "resourceRequirement")
	for _, field := range []string{"directLoad", "checkKind"} {
		if object(t, exportContext, "properties")[field] != nil {
			t.Fatal("导出上下文接收了旁路字段", field)
		}
	}
	for _, field := range []string{"normalImport", "objects", "objectTypes", "outputPath", "logPath", "skipCheckDir", "outputKind", "storageTarget"} {
		if object(t, directContext, "properties")[field] != nil {
			t.Fatal("旁路上下文接收了其他模块字段", field)
		}
	}
	directProperties := object(t, directContext, "properties")
	if object(t, directProperties, "directLoad")["$ref"] != "#/$defs/LocalConfig" || object(t, directProperties, "contentKind")["const"] != "DATA_ONLY" {
		t.Fatal("旁路上下文必须绑定独立配置与纯数据用途")
	}
	assertExactStringEnum(t, object(t, directProperties, "checkKind"), []string{"DIRECT_LOAD_FILES", "DIRECT_LOAD_PREFLIGHT"})

	// 固定用途和固定顺序双重约束，避免合法的两种上下文与检查集被交叉组合。
	coupling, ok := claim["allOf"].([]any)
	if !ok || len(coupling) != 3 {
		t.Fatal("认领响应缺少上下文与检查清单的完整配对约束")
	}
	domainRule := variantObject(t, coupling[0])
	assertRequiredProperties(t, object(t, domainRule, "if"), "executionContext")
	assertRequiredProperties(t, object(t, object(t, object(t, domainRule, "if"), "properties"), "executionContext"), "directLoad")
	if object(t, object(t, object(t, domainRule, "then"), "properties"), "checkSet")["$ref"] != "direct-load.json#/$defs/PrecheckCheckSet" || object(t, object(t, object(t, domainRule, "else"), "properties"), "checkSet")["$ref"] != "#/components/schemas/AgentPrecheckCheckSet" {
		t.Fatal("导出和旁路的上下文与检查集仍可错误混合")
	}
	fullChecks := []string{"TOOL_ENVIRONMENT", "NODE_RESOURCE", "INPUT_FILES", "DATABASE_CONNECTIVITY", "RPC_CONNECTIVITY", "DIRECT_LOAD_VERSION", "WRITE_ACCESS", "DIRECT_LOAD_STRUCTURE", "DIRECT_LOAD_CONFIG"}
	for index, expected := range [][]string{{"INPUT_FILES"}, fullChecks} {
		rule := variantObject(t, coupling[index+1])
		assertRequiredProperties(t, object(t, rule, "if"), "executionContext")
		condition := object(t, object(t, object(t, rule, "if"), "properties"), "executionContext")
		assertRequiredProperties(t, condition, "directLoad", "checkKind")
		kind := []string{"DIRECT_LOAD_FILES", "DIRECT_LOAD_PREFLIGHT"}[index]
		if object(t, object(t, condition, "properties"), "checkKind")["const"] != kind {
			t.Fatal("旁路检查用途条件不固定", kind)
		}
		checks := object(t, object(t, object(t, rule, "then"), "properties"), "checkSet")["const"]
		want := make([]any, len(expected))
		for i, name := range expected {
			want[i] = name
		}
		if !reflect.DeepEqual(checks, want) {
			t.Fatal("旁路文件发现和完整预检仍可互换或省略检查", kind)
		}
	}
	checkSets, ok := object(t, direct, "PrecheckCheckSet")["oneOf"].([]any)
	if !ok || len(checkSets) != 2 {
		t.Fatal("旁路检查集只能包含固定1项和9项形态")
	}
	assertFixedPrecheckArray(t, variantObject(t, checkSets[0]), []string{"INPUT_FILES"})
	assertFixedPrecheckArray(t, variantObject(t, checkSets[1]), fullChecks)
	resultSets, ok := object(t, direct, "PrecheckResults")["oneOf"].([]any)
	if !ok || len(resultSets) != 2 {
		t.Fatal("旁路完成结果只能包含固定1项和9项形态")
	}
	for index, expected := range [][]string{{"INPUT_FILES"}, fullChecks} {
		set := variantObject(t, resultSets[index])
		prefix, ok := set["prefixItems"].([]any)
		if !ok || len(prefix) != len(expected) || set["minItems"] != float64(len(expected)) || set["maxItems"] != float64(len(expected)) || set["items"] != false {
			t.Fatal("旁路完成结果允许缺项或多项")
		}
		for itemIndex, check := range expected {
			parts, ok := variantObject(t, prefix[itemIndex])["allOf"].([]any)
			if !ok || len(parts) != 2 || variantObject(t, parts[0])["$ref"] != "#/$defs/CheckResult" || object(t, object(t, variantObject(t, parts[1]), "properties"), "check")["const"] != check {
				t.Fatal("旁路完成结果未绑定独立证据或固定检查顺序", check)
			}
		}
	}
}

// assertExactReferenceBranches 保留显式引用边界，禁止宽泛分支或交叉引用绕过固定契约。
func assertExactReferenceBranches(t *testing.T, schema map[string]any, keyword string, refs ...string) {
	t.Helper()
	variants, ok := schema[keyword].([]any)
	if !ok || len(variants) != len(refs) || len(schema) != 1 {
		t.Fatalf("%s 必须只包含固定引用分支: %#v", keyword, schema)
	}
	for index, ref := range refs {
		variant := variantObject(t, variants[index])
		if len(variant) != 1 || variant["$ref"] != ref {
			t.Fatalf("%s 分支 %d 未使用独立严格引用 %s", keyword, index, ref)
		}
	}
}

// loadDirectLoadContract 读取仓库冻结契约，不解析工具、数据库或真实输入。
func loadDirectLoadContract(t *testing.T) map[string]any {
	t.Helper()
	data, err := Files.ReadFile("direct-load.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
