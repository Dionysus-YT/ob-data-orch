package importdomain

import (
	"ob-data-orch/internal/commandgen"
	"strconv"
)

// Fields 将已经通过 Normalize 的活动配置映射为唯一生成器输入；不读取秘密或拼接 Shell。
func Fields(c Config) []commandgen.FieldInput {
	var fields []commandgen.FieldInput
	stringValue := func(name, value string) {
		if value != "" {
			fields = append(fields, commandgen.FieldInput{Name: "--" + name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: value}})
		}
	}
	flag := func(name string, value bool, source commandgen.ValueSource) {
		if value {
			fields = append(fields, commandgen.FieldInput{Name: "--" + name, Source: source, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
	}
	flag("all", c.Scope == "ALL", commandgen.SourceUser)
	for _, kind := range objectTypes {
		stringValue(kind, c.Objects[kind])
	}
	flag("ddl", c.HasDDL(), commandgen.SourceFormat)
	if c.HasData() {
		flag(map[string]string{"CSV": "csv", "CUT": "cut", "SQL": "sql"}[c.Format], true, commandgen.SourceFormat)
	}
	for _, f := range []struct{ name, value string }{
		{"file-path", c.FilePath}, {"log-path", c.LogPath}, {"file-suffix", c.FileSuffix}, {"file-encoding", c.FileEncoding}, {"file-regular-expression", c.FileRegex}, {"exclude-table", c.ExcludeTables},
		{"column-separator", c.Parser.ColumnSeparator}, {"column-quote", c.Parser.ColumnQuote}, {"column-splitter", c.Parser.ColumnSplitter}, {"line-separator", c.Parser.LineSeparator},
		{"escape-character", c.Parser.EscapeCharacter}, {"null-string", c.Parser.NullString}, {"empty-string", c.Parser.EmptyString}, {"compression-algo", c.Parser.CompressionAlgo}, {"compression-level", c.Parser.CompressionLevel}, {"date-format", c.Parser.DateFormat}, {"default-date", c.Parser.DefaultDate},
		{"strict", c.Execution.Strict}, {"rw", c.Execution.RW}, {"mem", c.Execution.Memory}, {"nls-date-format", c.Execution.NLSDateFormat}, {"nls-timestamp-format", c.Execution.NLSTimestampFormat}, {"nls-timestamp-tz-format", c.Execution.NLSTimestampTZFormat},
	} {
		stringValue(f.name, f.value)
	}
	for _, f := range []struct {
		name  string
		value bool
	}{{"skip-header", c.Parser.SkipHeader}, {"skip-footer", c.Parser.SkipFooter}, {"ignore-escape", c.Parser.IgnoreEscape}, {"trail-delimiter", c.Parser.TrailDelimiter}, {"with-trim", c.Parser.WithTrim}, {"compress", c.Parser.Compress}, {"replace-data", c.ReplaceData}, {"replace-object", c.ReplaceObject}, {"truncate-table", c.Cleanup == "TRUNCATE"}, {"delete-from-table", c.Cleanup == "DELETE"}} {
		flag(f.name, f.value, commandgen.SourceUser)
	}
	if c.CompatMysql {
		stringValue("compat-mode", "mysql")
	}
	if c.Mapping != nil {
		stringValue("ctl-path", c.Mapping.CtlPath)
		stringValue("include-column-names", c.Mapping.IncludeColumns)
		stringValue("exclude-column-names", c.Mapping.ExcludeColumns)
		stringValue("exclude-data-types", c.Mapping.ExcludeDataTypes)
		flag("auto-column-mapping", c.Mapping.Kind == "AUTO", commandgen.SourceUser)
	}
	for _, f := range []struct{ name, value string }{{"max-discards", c.Execution.MaxDiscards}, {"max-errors", c.Execution.MaxErrors}, {"thread", c.Execution.Thread}, {"batch", c.Execution.Batch}, {"block-size", c.Execution.BlockSize}, {"max-tps", c.Execution.MaxTPS}, {"max-wait-timeout", c.Execution.MaxWaitTimeout}} {
		if f.value != "" {
			value, _ := strconv.ParseInt(f.value, 10, 64)
			fields = append(fields, commandgen.FieldInput{Name: "--" + f.name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: value}})
		}
	}
	return fields
}
