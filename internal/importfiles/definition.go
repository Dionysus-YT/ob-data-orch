package importfiles

import (
	"regexp"
	"strings"
)

var generatedColumn = regexp.MustCompile(`\bAS\s*\(`)
var definitionCall = regexp.MustCompile("([A-Z_][A-Z_0-9$]*)\\s*\\(")
var quotedDefinitionCall = regexp.MustCompile("(?:`(?:``|[^`])+`|\"(?:\"\"|[^\"])+\")\\s*\\(")
var definitionSetting = regexp.MustCompile(`^(?:NAMES [A-Z0-9_]+(?: COLLATE [A-Z0-9_]+)?|CHARACTER SET [A-Z0-9_]+|(?:FOREIGN_KEY_CHECKS|UNIQUE_CHECKS)\s*=\s*[01]|SQL_MODE\s*=\s*'[A-Z_, ]*')$`)

type tableDefinition struct {
	columns, required []string
	dropBeforeCreate  bool
	permissionsKnown  bool
}

// definitionColumns 沿用结构读取入口，权限事实另由 inspectTableDefinition 返回。
func definitionColumns(input, database, table string) ([]string, []string, bool) {
	definition, ok := inspectTableDefinition(input, database, table)
	return definition.columns, definition.required, ok
}

// inspectTableDefinition 只识别同名普通建表和可选的单表 DROP 前缀，不执行或返回原始 SQL。
func inspectTableDefinition(input, database, table string) (tableDefinition, bool) {
	definition := tableDefinition{permissionsKnown: true}
	text := strings.TrimSpace(strings.TrimPrefix(input, "\ufeff"))
	for count := 0; count < 2; count++ {
		var ok bool
		text, ok = definitionPrelude(text)
		if !ok {
			return tableDefinition{}, false
		}
		if !strings.HasPrefix(strings.ToUpper(text), "DROP ") {
			break
		}
		if definition.dropBeforeCreate {
			return tableDefinition{}, false
		}
		rest := strings.TrimSpace(text[5:])
		if !strings.HasPrefix(strings.ToUpper(rest), "TABLE ") {
			return tableDefinition{}, false
		}
		rest = strings.TrimSpace(rest[6:])
		if strings.HasPrefix(strings.ToUpper(rest), "IF EXISTS ") {
			rest = strings.TrimSpace(rest[10:])
		}
		name, tail, valid := definitionTarget(rest, database)
		if !valid || name != table || !strings.HasPrefix(strings.TrimSpace(tail), ";") {
			return tableDefinition{}, false
		}
		definition.dropBeforeCreate = true
		text = strings.TrimSpace(strings.TrimSpace(tail)[1:])
	}
	columns, required, ok := createTableColumns(text, database, table)
	if !ok {
		return tableDefinition{}, false
	}
	definition.columns, definition.required = columns, required
	// 外部对象引用与未核验函数可能额外需要权限，列结构可以读取，写权限必须保持未知。
	outside := outsideLiterals(text)
	if strings.Contains(outside, " REFERENCES ") || strings.Contains(outside, " FOREIGN KEY ") {
		definition.permissionsKnown = false
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields("VARCHAR VARCHAR2 NVARCHAR2 CHAR NCHAR BINARY VARBINARY RAW UROWID DECIMAL NUMERIC NUMBER FLOAT DOUBLE REAL INT INTEGER BIGINT MEDIUMINT SMALLINT TINYINT BIT TIMESTAMP DATETIME TIME ENUM SET KEY CHECK VALUES PARTITION RANGE HASH LIST COLUMNS CURRENT_TIMESTAMP CURRENT_DATE CURRENT_TIME LOCALTIME LOCALTIMESTAMP AS") {
		allowed[name] = true
	}
	if start := strings.IndexByte(outside, '('); start >= 0 {
		outside = outside[start:]
	}
	for _, match := range definitionCall.FindAllStringSubmatchIndex(outside, -1) {
		if !allowed[outside[match[2]:match[3]]] && !definitionIndexName(outside[:match[0]]) {
			definition.permissionsKnown = false
		}
	}
	// 带引号的调用不能按内置类型放行；只有 KEY/INDEX 后的索引名称属于已识别结构。
	for _, match := range quotedDefinitionCall.FindAllStringIndex(outside, -1) {
		if !definitionIndexName(outside[:match[0]]) {
			definition.permissionsKnown = false
		}
	}
	return definition, true
}

func definitionIndexName(prefix string) bool {
	before := strings.TrimSpace(prefix)
	return strings.HasSuffix(before, " KEY") || strings.HasSuffix(before, " INDEX")
}

func definitionTarget(text, database string) (string, string, bool) {
	name, tail, ok := sqlIdentifier(text)
	if !ok {
		return "", "", false
	}
	rest := strings.TrimSpace(tail)
	if strings.HasPrefix(rest, ".") {
		if name != database {
			return "", "", false
		}
		return sqlIdentifier(strings.TrimSpace(rest[1:]))
	}
	return name, rest, true
}

// definitionPrelude 只跳过注释和官方导出常见会话设置，不接受自由 SET 表达式。
func definitionPrelude(input string) (string, bool) {
	text := input
	for i := 0; i < 32; i++ {
		if strings.HasPrefix(text, "--") {
			end := strings.IndexByte(text, '\n')
			if end < 0 {
				return "", false
			}
			text = strings.TrimSpace(text[end+1:])
			continue
		}
		if strings.HasPrefix(text, "/*") {
			if strings.HasPrefix(text, "/*!") {
				return "", false
			}
			end := strings.Index(text, "*/")
			if end < 0 {
				return "", false
			}
			text = strings.TrimSpace(text[end+2:])
			continue
		}
		if strings.HasPrefix(strings.ToUpper(text), "SET ") {
			end := strings.IndexByte(text, ';')
			if end < 0 {
				return "", false
			}
			setting := strings.ToUpper(strings.TrimSpace(text[4:end]))
			if !definitionSetting.MatchString(setting) {
				return "", false
			}
			text = strings.TrimSpace(text[end+1:])
			continue
		}
		break
	}
	return text, true
}

func createTableColumns(text, database, table string) ([]string, []string, bool) {
	upper := strings.ToUpper(text)
	if !strings.HasPrefix(upper, "CREATE TABLE ") {
		return nil, nil, false
	}
	text = strings.TrimSpace(text[13:])
	if strings.HasPrefix(strings.ToUpper(text), "IF NOT EXISTS ") {
		text = strings.TrimSpace(text[14:])
	}
	name, tail, ok := sqlIdentifier(text)
	if !ok {
		return nil, nil, false
	}
	text = strings.TrimSpace(tail)
	if strings.HasPrefix(text, ".") {
		if name != database {
			return nil, nil, false
		}
		name, tail, ok = sqlIdentifier(strings.TrimSpace(text[1:]))
		if !ok {
			return nil, nil, false
		}
		text = strings.TrimSpace(tail)
	}
	if name != table || !strings.HasPrefix(text, "(") {
		return nil, nil, false
	}
	var definitions []string
	depth, start := 1, 1
	var quote byte
	closed := false
	for i := 1; i < len(text); i++ {
		ch := text[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				if i+1 < len(text) && text[i+1] == quote {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		if ch == '(' {
			depth++
		}
		if ch == ')' {
			depth--
			if depth == 0 {
				definitions = append(definitions, strings.TrimSpace(text[start:i]))
				rest := outsideLiterals(text[i+1:])
				// 文件中后续语句可能再次改变或删除该表；不能只采用前一条 CREATE 的结构。
				if end := strings.IndexByte(rest, ';'); end >= 0 && strings.TrimSpace(rest[end+1:]) != "" {
					return nil, nil, false
				}
				if strings.Contains(rest, "ALTER TABLE") || strings.Contains(rest, "CREATE TABLE") {
					return nil, nil, false
				}
				closed = true
				break
			}
		}
		if ch == ',' && depth == 1 {
			definitions = append(definitions, strings.TrimSpace(text[start:i]))
			start = i + 1
			if len(definitions) > 4096 {
				return nil, nil, false
			}
		}
	}
	if !closed {
		return nil, nil, false
	}
	var columns, required []string
	seen := map[string]bool{}
	mayOmit := map[string]bool{}
	primary := map[string]bool{}
	for _, definition := range definitions {
		upper = strings.ToUpper(definition)
		if strings.HasPrefix(upper, "CONSTRAINT ") {
			_, rest, valid := sqlIdentifier(strings.TrimSpace(definition[11:]))
			if !valid {
				return nil, nil, false
			}
			definition = strings.TrimSpace(rest)
			upper = strings.ToUpper(definition)
		}
		if strings.HasPrefix(upper, "PRIMARY KEY") {
			open := strings.IndexByte(definition, '(')
			end := strings.LastIndexByte(definition, ')')
			if open < 0 || end < open {
				return nil, nil, false
			}
			for _, part := range strings.Split(definition[open+1:end], ",") {
				column, rest, valid := sqlIdentifier(strings.TrimSpace(part))
				if !valid || strings.TrimSpace(rest) != "" {
					return nil, nil, false
				}
				primary[column] = true
			}
			continue
		}
		skip := false
		for _, keyword := range []string{"KEY ", "UNIQUE ", "CONSTRAINT ", "INDEX ", "FOREIGN ", "CHECK ", "FULLTEXT ", "SPATIAL "} {
			if strings.HasPrefix(upper, keyword) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		column, attributes, valid := sqlIdentifier(definition)
		if !valid || seen[column] || strings.TrimSpace(attributes) == "" {
			return nil, nil, false
		}
		seen[column] = true
		attributes = outsideLiterals(attributes)
		if generatedColumn.MatchString(attributes) {
			continue
		}
		columns = append(columns, column)
		if len(columns) > 4096 {
			return nil, nil, false
		}
		optional := strings.Contains(attributes, " DEFAULT ") || strings.Contains(attributes, " AUTO_INCREMENT ")
		mayOmit[column] = optional
		if !optional && (strings.Contains(attributes, " NOT NULL") || strings.Contains(attributes, " PRIMARY KEY")) {
			required = append(required, column)
			primary[column] = true
		}
	}
	for _, column := range columns {
		if primary[column] && !mayOmit[column] {
			found := false
			for _, name := range required {
				if name == column {
					found = true
				}
			}
			if !found {
				required = append(required, column)
			}
		}
	}
	return columns, required, len(columns) > 0
}

func outsideLiterals(value string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				if i+1 < len(value) && value[i+1] == quote {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			out.WriteByte(' ')
			continue
		}
		out.WriteByte(ch)
	}
	return " " + strings.ToUpper(strings.Join(strings.Fields(out.String()), " ")) + " "
}
