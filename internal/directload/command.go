package directload

import (
	"ob-data-orch/internal/commandgen"
	"strconv"
)

// Fields 把已规范化配置映射给唯一生成器，不读取秘密、不拼接 Shell，不把客户端事实写入命令。
// 数据源连接、数据库与凭据引用由控制面使用已授权快照另外提供。
func Fields(c Config) []commandgen.FieldInput {
	var fields []commandgen.FieldInput
	text := func(name, value string) {
		if value != "" {
			fields = append(fields, commandgen.FieldInput{Name: "--" + name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: value}})
		}
	}
	flag := func(name string, value bool, source commandgen.ValueSource) {
		if value {
			fields = append(fields, commandgen.FieldInput{Name: "--" + name, Source: source, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
	}
	integer := func(name string, value int64) {
		fields = append(fields, commandgen.FieldInput{Name: "--" + name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: value}})
	}
	text("file-path", c.FilePath)
	text("table", c.Table)
	flag("direct", true, commandgen.SourceFormat)
	if c.Format == "CSV" {
		flag("csv", true, commandgen.SourceFormat)
	}
	integer("rpc-port", int64(c.RPCPort))
	if c.Format == "CSV" {
		text("column-separator", c.ColumnSeparator)
		flag("external-data", c.ExternalData, commandgen.SourceUser)
	}
	for _, f := range []struct{ name, value string }{{"thread", c.Thread}, {"parallel", c.Parallel}} {
		if f.value != "" {
			value, _ := strconv.ParseInt(f.value, 10, 64)
			integer(f.name, value)
		}
	}
	flag("replace-data", c.ReplaceData, commandgen.SourceUser)
	flag("truncate-table", c.TruncateTable, commandgen.SourceUser)
	return fields
}
