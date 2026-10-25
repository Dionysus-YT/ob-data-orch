package commandgen

import (
	"math"
	"ob-data-orch/internal/parammeta"
	"strconv"
	"strings"
)

// NewNormalImport 只加载普通导入参数目录，不注册导出或旁路能力。
func NewNormalImport() (*Generator, error) {
	catalog, err := parammeta.LoadNormalImport()
	if err != nil {
		return nil, err
	}
	return newGenerator(catalog)
}

// validateNormalImportFields 在唯一生成器内复核普通导入组合，避免调用方绕过领域校验。
func validateNormalImportFields(request Request) []Issue {
	fields := map[string]FieldInput{}
	for _, field := range request.Fields {
		fields[field.Name] = field
	}
	has := func(name string) bool {
		f, ok := fields["--"+name]
		return ok && (f.Value.Kind != ValueBoolean || f.Value.Boolean)
	}
	value := func(name string) string { return fields["--"+name].Value.String }
	var issues []Issue
	block := func(name string) {
		issues = append(issues, Issue{Code: "IMPORT_PARAMETER_CONFLICT", Field: "--" + name})
	}
	// 分离 argv 不能阻止 CLI 将开头的选项重新解释为参数；受控文本值不接受选项令牌。
	for _, field := range request.Fields {
		if field.Value.Kind == ValueString && (strings.HasPrefix(field.Value.String, "--") || len(field.Value.String) > 1 && field.Value.String[0] == '-' && (field.Value.String[1] >= 'A' && field.Value.String[1] <= 'Z' || field.Value.String[1] >= 'a' && field.Value.String[1] <= 'z')) {
			block(strings.TrimPrefix(field.Name, "--"))
		}
	}
	data := has("csv") || has("cut") || has("sql")
	if !data && !has("ddl") {
		block("ddl")
	}
	objects := false
	for _, name := range []string{"table", "table-group", "view", "trigger", "obj-user", "role", "sequence", "synonym", "type", "type-body", "package", "package-body", "function", "procedure"} {
		if !has(name) {
			continue
		}
		objects = true
		if name != "table" && !has("ddl") {
			block(name)
		}
		if strings.HasPrefix(value(name), "-") || strings.ContainsAny(value(name), "\x00\r\n;") {
			block(name)
		}
	}
	if has("all") == objects {
		block("all")
	}
	for _, name := range []string{"max-discards", "max-errors", "strict", "rw", "batch", "block-size", "max-tps", "max-wait-timeout", "replace-data", "truncate-table", "delete-from-table", "compress", "compression-algo", "compression-level", "auto-column-mapping", "ctl-path", "include-column-names", "exclude-column-names", "exclude-data-types", "null-string", "line-separator"} {
		if has(name) && !data {
			block(name)
		}
	}
	for _, name := range []string{"column-quote", "column-separator", "auto-column-mapping"} {
		if has(name) && !has("csv") {
			block(name)
		}
	}
	for _, name := range []string{"column-splitter", "skip-footer", "ignore-escape", "trail-delimiter"} {
		if has(name) && !has("cut") {
			block(name)
		}
	}
	for _, name := range []string{"skip-header", "with-trim", "escape-character", "empty-string", "date-format", "default-date"} {
		if has(name) && !(has("csv") || has("cut")) {
			block(name)
		}
	}
	if has("ctl-path") && (has("auto-column-mapping") || has("include-column-names") || has("exclude-column-names")) || has("auto-column-mapping") && (has("include-column-names") || has("exclude-column-names")) || has("include-column-names") && has("exclude-column-names") {
		block("ctl-path")
	}
	if has("truncate-table") && has("delete-from-table") {
		block("truncate-table")
	}
	if (has("replace-object") || has("compat-mode")) && !has("ddl") {
		block("replace-object")
	}
	if has("compat-mode") && !contains([]string{"mysql56", "mysql57", "mysql80"}, value("compat-mode")) {
		block("compat-mode")
	}
	if has("compression-algo") || has("compression-level") {
		if !has("compress") {
			block("compress")
		}
	}
	if has("compression-algo") && !contains([]string{"zstd", "zlib", "gzip", "snappy"}, value("compression-algo")) {
		block("compression-algo")
	}
	if has("compression-level") {
		n, err := strconv.ParseInt(value("compression-level"), 10, 64)
		min, max := int64(1), int64(22)
		if value("compression-algo") == "zlib" {
			min, max = -1, 9
		}
		if err != nil || n < min || n > max || contains([]string{"gzip", "snappy"}, value("compression-algo")) {
			block("compression-level")
		}
	}
	if has("rw") {
		v, err := strconv.ParseFloat(value("rw"), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1 {
			block("rw")
		}
	}
	if has("strict") && !contains([]string{"true", "false"}, value("strict")) {
		block("strict")
	}
	for _, name := range []string{"thread", "batch", "block-size", "max-tps", "max-wait-timeout", "max-errors", "max-discards"} {
		if f, ok := fields["--"+name]; ok {
			min := int64(1)
			if name == "max-errors" || name == "max-discards" {
				min = -1
			}
			if f.Value.Kind != ValueInteger || f.Value.Integer < min || f.Value.Integer > 9007199254740991 {
				block(name)
			}
		}
	}
	return issues
}
