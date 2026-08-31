package exportdomain

import (
	"errors"
	"strings"

	"ob-data-orch/internal/commandgen"
)

// BuildGeneralizedFields 将已归一化的泛化草稿映射为命令生成器字段。
//
// 该函数只负责字段顺序、参数名和字段来源，不读取数据源、节点或凭据，
// 也不决定元数据版本、能力版本和执行状态。调用方必须先完成归一化；
// 本函数不会把秘密明文转换成字段。
func BuildGeneralizedFields(input Draft) ([]commandgen.FieldInput, error) {
	fields := make([]commandgen.FieldInput, 0, 32)
	appendString := func(name, value string) {
		if value == "" {
			return
		}
		fields = append(fields, commandgen.FieldInput{
			Name: name, Source: commandgen.SourceUser,
			Value: commandgen.Value{Kind: commandgen.ValueString, String: value},
		})
	}
	appendBool := func(name string, value bool) {
		if !value {
			return
		}
		fields = append(fields, commandgen.FieldInput{
			Name: name, Source: commandgen.SourceUser,
			Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true},
		})
	}
	appendInt64 := func(name string, value *int64) {
		if value == nil {
			return
		}
		fields = append(fields, commandgen.FieldInput{
			Name: name, Source: commandgen.SourceUser,
			Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: *value},
		})
	}

	switch input.ScopeKind {
	case "ALL":
		appendBool("--all", true)
	case "SPECIFIED":
		objectParameter := "--table"
		if input.ObjectType == "VIEW" {
			objectParameter = "--view"
		}
		appendString(objectParameter, strings.Join(input.Objects, ","))
	default:
		return nil, errors.New("export draft scope is unsupported")
	}
	if len(input.ExcludeTables) != 0 {
		appendString("--exclude-table", strings.Join(input.ExcludeTables, ","))
	}

	switch input.ContentKind {
	case "DATA_ONLY":
		// 数据格式单选，控制面按归一化结果提供对应格式标志。
		formatParameter := "--csv"
		switch input.Format {
		case "CUT":
			formatParameter = "--cut"
		case "SQL":
			formatParameter = "--sql"
		case "POS":
			formatParameter = "--pos"
		case "PARQUET":
			formatParameter = "--par"
		case "ORC":
			formatParameter = "--orc"
		case "AVRO":
			formatParameter = "--avro"
		}
		fields = append(fields, commandgen.FieldInput{
			Name: formatParameter, Source: commandgen.SourceFormat,
			Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true},
		})
	case "DDL_ONLY":
		fields = append(fields, commandgen.FieldInput{
			Name: "--ddl", Source: commandgen.SourceFormat,
			Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true},
		})
	case "DDL_AND_DATA":
		fields = append(fields,
			commandgen.FieldInput{
				Name: "--ddl", Source: commandgen.SourceFormat,
				Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true},
			},
			commandgen.FieldInput{
				Name: "--csv", Source: commandgen.SourceFormat,
				Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true},
			},
		)
	default:
		return nil, errors.New("export draft content kind is unsupported")
	}

	// DDL 行为只随包含 DDL 的内容发射，适用性已在归一化阶段校验。
	appendBool("--drop-object", input.DropObject)
	appendBool("--retain-schema", input.RetainSchema)
	appendBool("--compact-schema", input.CompactSchema)
	appendString("--file-path", input.FilePath)
	appendString("--log-path", input.LogPath)
	appendString("--ctl-path", input.ControlFilePath)
	appendString("--tmp-path", input.TmpPath)
	// 该开关在泛化命令中始终显式存在，以保持既有归一化字段与指纹稳定。
	fields = append(fields, commandgen.FieldInput{
		Name: "--skip-check-dir", Source: commandgen.SourceUser,
		Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: input.SkipCheckDir},
	})

	// 序列化选项按格式适用性发射；越界字段已在归一化阶段失败关闭。
	switch input.Format {
	case "CSV":
		csv := input.CsvOptions
		appendBool("--skip-header", csv.SkipHeader)
		for _, option := range []struct {
			name  string
			value string
		}{
			{"--column-separator", csv.ColumnSeparator},
			{"--column-quote", csv.ColumnQuote},
			{"--column-quote-mode", csv.ColumnQuoteMode},
			{"--escape-character", csv.EscapeCharacter},
			{"--line-separator", csv.LineSeparator},
			{"--null-string", csv.NullString},
			{"--file-encoding", csv.FileEncoding},
		} {
			appendString(option.name, option.value)
		}
		appendBool("--with-trim", csv.WithTrim)
	case "CUT":
		csv := input.CsvOptions
		for _, option := range []struct {
			name  string
			value string
		}{
			{"--column-splitter", csv.ColumnSplitter},
			{"--escape-character", csv.EscapeCharacter},
			{"--line-separator", csv.LineSeparator},
			{"--null-string", csv.NullString},
			{"--file-encoding", csv.FileEncoding},
		} {
			appendString(option.name, option.value)
		}
		appendBool("--with-trim", csv.WithTrim)
		appendBool("--trail-delimiter", input.CutOptions.TrailDelimiter)
		appendBool("--remove-newline", input.CutOptions.RemoveNewline)
	case "SQL":
		csv := input.CsvOptions
		appendString("--line-separator", csv.LineSeparator)
		appendString("--file-encoding", csv.FileEncoding)
	case "POS":
		// POS 首版无序列化选项，只有 --pos 与 --ctl-path。
	case "PARQUET", "ORC", "AVRO":
		// 结构化格式只发射官方格式表列出的文件编码。
		appendString("--file-encoding", input.CsvOptions.FileEncoding)
	}

	appendBool("--no-nested-dir", input.NoNestedDir)
	appendInt64("--max-file-size", input.MaxFileSize)
	appendBool("--retain-empty-files", input.RetainEmptyFiles)
	appendBool("--compress", input.Compress)
	appendString("--compression-algo", input.CompressionAlgo)
	// 压缩等级只在归一化确认算法范围后发射。
	appendInt64("--compression-level", input.CompressionLevel)
	if input.QuerySql != "" {
		// query-sql 仅可由已归一化的固定 OBDUMPER 导出信封发射；
		// 它是普通高级筛选参数，不构成通用 SQL 执行入口。
		appendString("--query-sql", input.QuerySql)
	}
	appendString("--where", input.Where)
	appendBool("--snapshot", input.Snapshot)
	appendString("--partition", input.Partition)
	if len(input.ExcludeDataTypes) != 0 {
		appendString("--exclude-data-types", strings.Join(input.ExcludeDataTypes, ","))
	}
	appendString("--date-value-format", input.TimestampFormats.DateValueFormat)
	appendString("--datetime-value-format", input.TimestampFormats.DateTimeValueFormat)
	if len(input.IncludeColumns) != 0 {
		appendString("--include-column-names", strings.Join(input.IncludeColumns, ","))
	}
	if len(input.ExcludeColumns) != 0 {
		appendString("--exclude-column-names", strings.Join(input.ExcludeColumns, ","))
	}
	appendBool("--exclude-virtual-columns", input.ExcludeVirtualColumns)
	appendInt64("--flashback-scn", input.FlashbackScn)
	appendString("--flashback-timestamp", input.FlashbackTimestamp)
	for _, option := range []struct {
		name  string
		value *int
	}{
		{"--thread", input.Thread},
		{"--page-size", input.PageSize},
		{"--parallel-macro", input.ParallelMacro},
		{"--fetch-size", input.FetchSize},
	} {
		if option.value != nil {
			value := int64(*option.value)
			appendInt64(option.name, &value)
		}
	}
	appendString("--mem", input.JvmMemory)
	appendString("--block-size", input.BlockSize)

	return fields, nil
}
