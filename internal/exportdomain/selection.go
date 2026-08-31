package exportdomain

import (
	"errors"

	"ob-data-orch/internal/store"
)

// Selection 是导出对象、内容和数据格式经过领域规则归一后的选择结果。
// 它不包含输出路径、凭据引用或高级参数，避免把请求适配与执行安全事实混在同一层。
type Selection struct {
	Database      string
	ScopeKind     string
	ObjectType    string
	Objects       []string
	ExcludeTables []string
	ContentKind   string
	Format        string
}

// NormalizeSelection 校验并归一导出范围、内容和格式的最小领域组合。
// 该函数只处理不依赖数据源、节点和运行时权限的规则，调用方仍必须执行后续路径、凭据和能力复验。
func NormalizeSelection(config *store.ExportConfig, maxObjects int, validateObjectName func(string) error) (Selection, error) {
	if config == nil {
		return Selection{}, errors.New("export config is required")
	}
	if maxObjects < 1 {
		return Selection{}, errors.New("export object limit is invalid")
	}
	scope := config.ObjectScope
	selection := Selection{Database: scope.Database, ContentKind: config.ContentSelection.ContentKind}
	if scope.Database == "" || len(scope.Database) > 512 {
		return Selection{}, errors.New("v6 object scope requires a database")
	}
	switch scope.ScopeKind {
	case "ALL":
		if len(scope.ObjectTypes) != 0 || len(scope.Expressions) != 0 {
			return Selection{}, errors.New("v6 ALL scope must not carry object types or expressions")
		}
		selection.ScopeKind = "ALL"
	case "SPECIFIED":
		if len(scope.ObjectTypes) != 1 || (scope.ObjectTypes[0] != "TABLE" && scope.ObjectTypes[0] != "VIEW") {
			return Selection{}, errors.New("v6 object scope must specify exactly one supported object type")
		}
		if len(scope.Expressions) == 0 || len(scope.Expressions) > maxObjects {
			return Selection{}, errors.New("v6 object expressions must number between 1 and 100")
		}
		selection.ScopeKind = "SPECIFIED"
		selection.ObjectType = scope.ObjectTypes[0]
		for index, expression := range scope.Expressions {
			// schema 前缀只允许缺省或与范围数据库一致；跨库表达式未取证，失败关闭。
			if expression.Schema != "" && expression.Schema != scope.Database {
				return Selection{}, errors.New("v6 multi-database schema prefix is not enabled")
			}
			if validateObjectName != nil {
				if err := validateObjectName(expression.Name); err != nil {
					return Selection{}, errors.New("v6 object expression name is invalid")
				}
			}
			if expression.Schema != "" {
				config.ObjectScope.Expressions[index].RawInput = expression.Schema + "." + expression.Name
			} else {
				config.ObjectScope.Expressions[index].RawInput = expression.Name
			}
			selection.Objects = append(selection.Objects, expression.Name)
		}
	default:
		return Selection{}, errors.New("v6 object scope kind is unsupported")
	}
	if len(scope.ExcludeTables) != 0 {
		if selection.ObjectType == "VIEW" {
			return Selection{}, errors.New("v6 exclude tables require table scope")
		}
		if len(scope.ExcludeTables) > maxObjects {
			return Selection{}, errors.New("v6 exclude tables exceed the limit")
		}
		for _, name := range scope.ExcludeTables {
			if validateObjectName != nil {
				if err := validateObjectName(name); err != nil {
					return Selection{}, errors.New("v6 exclude table name is invalid")
				}
			}
		}
		selection.ExcludeTables = append([]string(nil), scope.ExcludeTables...)
	}

	switch selection.ContentKind {
	case "DATA_ONLY":
		if selection.ObjectType == "VIEW" {
			return Selection{}, errors.New("v6 views cannot export data")
		}
		if !isDataFormat(config.DataFormat.FormatKind) {
			return Selection{}, errors.New("v6 data format is unsupported")
		}
		selection.Format = config.DataFormat.FormatKind
	case "DDL_ONLY":
		if config.DataFormat.FormatKind != "" && config.DataFormat.FormatKind != "CSV" {
			return Selection{}, errors.New("v6 ddl-only content must not declare a data format")
		}
	case "DDL_AND_DATA":
		if selection.ObjectType == "VIEW" {
			return Selection{}, errors.New("v6 views cannot export data")
		}
		if config.DataFormat.FormatKind != "CSV" {
			return Selection{}, errors.New("v6 ddl-and-data content requires csv format")
		}
		selection.Format = "CSV"
	default:
		return Selection{}, errors.New("v6 content selection is unsupported")
	}
	return selection, nil
}

// CapabilityFor 按内容与数据格式选择命令生成能力版本。
// 未知格式沿用 CSV 默认值只作为兼容兜底；调用方必须先通过 NormalizeSelection。
func CapabilityFor(contentKind, format string) string {
	switch contentKind {
	case "DDL_ONLY":
		return "export-odp-ddl-v1"
	case "DDL_AND_DATA":
		return "export-odp-ddl-csv-v1"
	default:
		switch format {
		case "CUT":
			return "export-odp-cut-v1"
		case "SQL":
			return "export-odp-sql-v1"
		case "POS":
			return "export-odp-pos-v1"
		case "PARQUET":
			return "export-odp-parquet-v1"
		case "ORC":
			return "export-odp-orc-v1"
		case "AVRO":
			return "export-odp-avro-v1"
		default:
			return "export-odp-full-csv-v1"
		}
	}
}

// DisplayFormat 返回快照与摘要投影使用的稳定格式标识。
func DisplayFormat(contentKind, format string) string {
	switch contentKind {
	case "DDL_ONLY":
		return "DDL"
	case "DDL_AND_DATA":
		return "DDL_CSV"
	default:
		return format
	}
}

func isDataFormat(format string) bool {
	switch format {
	case "CSV", "CUT", "POS", "SQL", "PARQUET", "ORC", "AVRO":
		return true
	default:
		return false
	}
}
