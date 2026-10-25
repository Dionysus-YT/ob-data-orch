package commandgen

import (
	"strings"
	"unicode"

	"ob-data-orch/internal/parammeta"
)

const directLoadCapability = "direct-load-local-v1"

// NewDirectLoad 只加载旁路目录；普通导入和导出不能通过此入口获取旁路命令资格。
func NewDirectLoad() (*Generator, error) {
	catalog, err := parammeta.LoadDirectLoad()
	if err != nil {
		return nil, err
	}
	return newGenerator(catalog)
}

// validateDirectLoadFields 独立复验旁路参数、连接场景和选项令牌，调用方不能绕过唯一生成器扩展能力。
// 真实版本、结构、风险确认和文件事实仍由预检及提交门禁复核，不接受客户端参数代替。
func validateDirectLoadFields(request Request, definitions map[string]parammeta.Definition) []Issue {
	fields := map[string]FieldInput{}
	var issues []Issue
	block := func(name, code string) { issues = append(issues, Issue{Code: code, Field: name}) }
	for _, field := range request.Fields {
		fields[field.Name] = field
		definition, known := definitions[field.Name]
		if !known || definition.SupportState != "ENABLED" {
			block(field.Name, "DIRECT_LOAD_PARAMETER_UNAVAILABLE")
		}
		if field.Value.Kind == ValueString && (strings.HasPrefix(field.Value.String, "-") || len(field.Value.String) > 4096 || strings.ContainsFunc(field.Value.String, unicode.IsControl)) {
			block(field.Name, "PARAMETER_VALUE_INVALID")
		}
	}
	has := func(name string) bool {
		field, ok := fields["--"+name]
		return ok && (field.Value.Kind != ValueBoolean || field.Value.Boolean)
	}
	value := func(name string) string { return fields["--"+name].Value.String }
	if !has("direct") {
		block("--direct", "REQUIRED_PARAMETER_MISSING")
	}
	if !has("csv") {
		block("format", "REQUIRED_PARAMETER_MISSING")
	}
	if has("sql") || has("par") || has("orc") {
		block("format", "FORMAT_SCENARIO_UNVERIFIED")
	}
	if !has("csv") && (has("column-separator") || has("external-data")) {
		block("format", "DIRECT_LOAD_PARAMETER_INACTIVE")
	}
	if !singleDirectIdentifier(value("table")) {
		block("--table", "TABLE_SCOPE_NOT_SINGLE")
	}
	if !singleDirectIdentifier(value("database")) {
		block("--database", "PARAMETER_VALUE_INVALID")
	}
	for _, name := range []string{"rpc-port", "thread", "parallel"} {
		if field, ok := fields["--"+name]; ok {
			max := int64(2147483647)
			if name == "rpc-port" {
				max = 65535
			}
			if field.Value.Kind != ValueInteger || field.Value.Integer < 1 || field.Value.Integer > max {
				block(field.Name, "PARAMETER_VALUE_OUT_OF_RANGE")
			}
		}
	}
	// 连接参数按唯一场景派生；不存在用 SQL 端口推测 RPC 的旁路路径。
	switch request.ConnectionKind {
	case ConnectionODP:
		if !has("tenant") || !has("cluster") || has("public-cloud") {
			block("connectionKind", "DIRECT_LOAD_CONNECTION_CONFLICT")
		}
	case ConnectionObserverDirect:
		// 固定 4.3.5 RPC Builder 对直连也要求 tenant；集群后缀只属于代理路径。
		if !has("tenant") || has("cluster") || has("public-cloud") {
			block("connectionKind", "DIRECT_LOAD_CONNECTION_CONFLICT")
		}
	default:
		block("connectionKind", "CONNECTION_KIND_NOT_IN_SLICE")
	}
	if has("yes") != has("truncate-table") {
		block("--yes", "DIRECT_LOAD_CONFIRMATION_CONFLICT")
	}
	if request.OutputKind != "" && request.OutputKind != "LOCAL" {
		block("outputKind", "DIRECT_LOAD_LOCAL_INPUT_REQUIRED")
	}
	return issues
}

func singleDirectIdentifier(value string) bool {
	if value == "" || len(value) > 256 || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' && r != '-' {
			return false
		}
	}
	return true
}
