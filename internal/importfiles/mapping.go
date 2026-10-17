package importfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/importdomain"
)

// MappingTable 只保留映射所需的结构，不把文件内容送往 JDBC、控制面或日志。
type MappingTable struct {
	Name    string
	Columns int
	Names   []string
	HasData bool
	// HasDefinition 与 DropBeforeCreate 只表达当前清单中的同名定义事实，不改变工具选项。
	HasDefinition    bool
	DropBeforeCreate bool
	// TargetColumns 只来自同一冻结输入中可识别的同名 CREATE TABLE，不是任意文件 SQL。
	TargetColumns []string
	// RequiredColumns 是定义中没有默认值、自增或生成行为的必填列。
	RequiredColumns []string
}

// MappingPlan 是当前输入证据的有界结构投影；不确定语法保持未知，不据此证明全部数据行合法。
type MappingPlan struct {
	Tables   []MappingTable
	Complete bool
	// DDLComplete 只有全部定义均可核算基本操作权限时为真，未知定义不冒充 CREATE TABLE。
	DDLComplete bool
}

// InspectMapping 在相同允许根目录内重读当前清单，核对摘要后提取首条记录的列结构。
// 压缩、非 UTF-8、控制文件和不能解释的语法不冒充已验证；返回前清零文件前缀。
func InspectMapping(ctx context.Context, c importdomain.Config, manifest Manifest, roots []string) (MappingPlan, error) {
	plan := MappingPlan{Complete: true, DDLComplete: !c.HasDDL()}
	if ctx == nil || !manifest.Valid(c) {
		return plan, errors.New("映射输入绑定无效")
	}
	byName := map[string]MappingTable{}
	definitions := map[string]MappingTable{}
	if c.HasDDL() {
		plan.DDLComplete = !c.Parser.Compress
		for _, entry := range manifest.Files {
			if entry.Kind != "DDL" {
				continue
			}
			if c.Parser.Compress {
				definitions[entry.Object] = MappingTable{Name: entry.Object, HasDefinition: true}
				continue
			}
			prefix, whole, err := mappingPrefix(ctx, c.FilePath, entry, roots)
			if err != nil {
				return plan, err
			}
			definition, ok := inspectTableDefinition(string(prefix), c.Database, entry.Object)
			for i := range prefix {
				prefix[i] = 0
			}
			value := MappingTable{Name: entry.Object, HasDefinition: true}
			if ok && whole && validMappingNames(definition.columns) && validMappingNames(definition.required) {
				value.TargetColumns = definition.columns
				value.RequiredColumns = definition.required
				value.DropBeforeCreate = definition.dropBeforeCreate
			} else {
				ok = false
			}
			if !ok || !definition.permissionsKnown {
				plan.DDLComplete = false
			}
			if _, duplicate := definitions[entry.Object]; duplicate {
				plan.DDLComplete = false
				value.TargetColumns = nil
				value.RequiredColumns = nil
			}
			definitions[entry.Object] = value
		}
	}
	for _, entry := range manifest.Files {
		if entry.Kind != "DATA" {
			continue
		}
		table := MappingTable{Name: entry.Object, HasData: true}
		if c.Parser.Compress || c.FileEncoding != "" && !strings.EqualFold(c.FileEncoding, "UTF-8") && !strings.EqualFold(c.FileEncoding, "UTF8") || c.Mapping != nil && c.Mapping.Kind == "CONTROL" || c.Parser.SkipFooter {
			plan.Complete = false
		} else {
			prefix, whole, err := mappingPrefix(ctx, c.FilePath, entry, roots)
			if err != nil {
				return plan, err
			}
			structure := prefix
			if !whole {
				// 有界前缀可能恰好切断 UTF-8 字符；仅去掉末端残片，不把它当成首条完整记录的编码错误。
				for removed := 0; removed < utf8.UTFMax-1 && len(structure) > 0; removed++ {
					r, width := utf8.DecodeLastRune(structure)
					if r != utf8.RuneError || width != 1 {
						break
					}
					structure = structure[:len(structure)-1]
				}
			}
			if !utf8.Valid(structure) {
				plan.Complete = false
			} else {
				var ok bool
				if c.Format == "SQL" {
					table.Columns, table.Names, ok = insertColumns(string(structure), c.Database, entry.Object)
				} else {
					table.Columns, table.Names, ok = delimitedColumns(structure, whole, c)
				}
				if !ok || !validMappingNames(table.Names) {
					plan.Complete = false
					table.Columns, table.Names = 0, nil
				}
			}
			for index := range prefix {
				prefix[index] = 0
			}
		}
		if previous, exists := byName[table.Name]; exists && (previous.Columns != table.Columns || strings.Join(previous.Names, "\x00") != strings.Join(table.Names, "\x00")) {
			plan.Complete = false
		}
		byName[table.Name] = table
	}
	for name, definition := range definitions {
		table := byName[name]
		definition.HasData = table.HasData
		definition.Columns = table.Columns
		definition.Names = table.Names
		byName[name] = definition
	}
	for _, table := range byName {
		plan.Tables = append(plan.Tables, table)
	}
	sort.Slice(plan.Tables, func(i, j int) bool { return plan.Tables[i].Name < plan.Tables[j].Name })
	return plan, nil
}

func mappingPrefix(ctx context.Context, input string, entry File, roots []string) ([]byte, bool, error) {
	root, relative, err := openBounded(input, roots)
	if err != nil {
		return nil, false, errors.New("映射输入路径不可用")
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, errors.New("映射输入事实变化")
	}
	name := relative
	if info.IsDir() {
		name = filepath.Join(relative, filepath.FromSlash(entry.Path))
	} else if filepath.Base(relative) != entry.Path {
		return nil, false, errors.New("映射输入清单不匹配")
	}
	info, err = root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != entry.Size || info.ModTime().UnixNano() != entry.Modified {
		return nil, false, errors.New("映射输入事实变化")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, false, errors.New("映射输入不可读")
	}
	defer file.Close()
	const limit = 256 * 1024
	prefix := make([]byte, 0, limit)
	buffer := make([]byte, 64*1024)
	defer func() {
		for i := range buffer {
			buffer[i] = 0
		}
	}()
	hash := sha256.New()
	for {
		if ctx.Err() != nil {
			for i := range prefix {
				prefix[i] = 0
			}
			return nil, false, ctx.Err()
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			hash.Write(buffer[:count])
			if len(prefix) < limit {
				keep := min(count, limit-len(prefix))
				prefix = append(prefix, buffer[:keep]...)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			for i := range prefix {
				prefix[i] = 0
			}
			return nil, false, errors.New("映射输入读取失败")
		}
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(info, after) || after.Size() != entry.Size || after.ModTime().UnixNano() != entry.Modified || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
		for i := range prefix {
			prefix[i] = 0
		}
		return nil, false, errors.New("映射输入摘要变化")
	}
	return prefix, int64(len(prefix)) == entry.Size, nil
}

func delimitedColumns(input []byte, whole bool, c importdomain.Config) (int, []string, bool) {
	data := bytes.TrimPrefix(input, []byte{0xef, 0xbb, 0xbf})
	separator, quote, escape := c.Parser.ColumnSeparator, c.Parser.ColumnQuote, c.Parser.EscapeCharacter
	if separator == "" {
		separator = ","
	}
	if quote == "" {
		quote = "'"
	}
	if escape == "" {
		escape = "\\"
	}
	if c.Format == "CUT" {
		separator = c.Parser.ColumnSplitter
		quote = ""
		if c.Parser.IgnoreEscape {
			escape = ""
		}
	}
	if separator == "" || c.Parser.LineSeparator != "" && !isStandardLineSeparator(c.Parser.LineSeparator) {
		return 0, nil, false
	}
	text := string(data)
	auto := c.Mapping != nil && c.Mapping.Kind == "AUTO"
	needed := 1
	if c.Parser.SkipHeader || auto {
		needed = 2
	}
	rows := [][]string{}
	fields := []string{}
	var field strings.Builder
	quoted := false
	ended := false
	for index := 0; index < len(text); {
		if escape != "" && strings.HasPrefix(text[index:], escape) {
			index += len(escape)
			if index >= len(text) {
				return 0, nil, false
			}
			field.WriteByte(text[index])
			index++
			continue
		}
		if quote != "" && strings.HasPrefix(text[index:], quote) {
			if quoted && strings.HasPrefix(text[index+len(quote):], quote) {
				field.WriteString(quote)
				index += 2 * len(quote)
				continue
			}
			if quoted || field.Len() == 0 {
				quoted = !quoted
				index += len(quote)
				continue
			}
		}
		if !quoted && strings.HasPrefix(text[index:], separator) {
			fields = append(fields, field.String())
			field.Reset()
			index += len(separator)
			continue
		}
		if !quoted && (text[index] == '\n' || text[index] == '\r') {
			fields = append(fields, field.String())
			field.Reset()
			rows = append(rows, fields)
			fields = nil
			ended = true
			if len(rows) >= needed {
				break
			}
			if text[index] == '\r' && index+1 < len(text) && text[index+1] == '\n' {
				index++
			}
			index++
			continue
		}
		ended = false
		field.WriteByte(text[index])
		index++
	}
	if !ended && whole && !quoted {
		fields = append(fields, field.String())
		rows = append(rows, fields)
	}
	if quoted || len(rows) == 0 {
		return 0, nil, false
	}
	dataIndex := 0
	if c.Parser.SkipHeader || auto {
		dataIndex = 1
	}
	if dataIndex >= len(rows) {
		return 0, nil, false
	}
	row := rows[dataIndex]
	if c.Format == "CUT" && c.Parser.TrailDelimiter && len(row) > 0 && row[len(row)-1] == "" {
		row = row[:len(row)-1]
	}
	if len(row) == 0 || len(row) > 4096 {
		return 0, nil, false
	}
	if auto {
		header := rows[0]
		if len(header) != len(row) {
			return 0, nil, false
		}
		for i := range header {
			if c.Parser.WithTrim {
				header[i] = strings.TrimSpace(header[i])
			}
		}
		return len(row), header, true
	}
	return len(row), nil, true
}

func isStandardLineSeparator(value string) bool {
	return value == "\n" || value == "\r" || value == "\r\n" || value == `\n` || value == `\r` || value == `\r\n`
}

// insertColumns 只识别首个 INSERT 的结构，不向数据库传递文件中的 SQL。
func insertColumns(input, database, table string) (int, []string, bool) {
	text := strings.TrimSpace(strings.TrimPrefix(input, "\ufeff"))
	for i := 0; i < 32; i++ {
		if strings.HasPrefix(text, "--") {
			end := strings.IndexByte(text, '\n')
			if end < 0 {
				return 0, nil, false
			}
			text = strings.TrimSpace(text[end+1:])
			continue
		}
		if strings.HasPrefix(text, "/*") {
			end := strings.Index(text, "*/")
			if end < 0 {
				return 0, nil, false
			}
			text = strings.TrimSpace(text[end+2:])
			continue
		}
		if strings.HasPrefix(strings.ToUpper(text), "SET ") {
			end := strings.IndexByte(text, ';')
			if end < 0 {
				return 0, nil, false
			}
			text = strings.TrimSpace(text[end+1:])
			continue
		}
		break
	}
	upper := strings.ToUpper(text)
	if !strings.HasPrefix(upper, "INSERT ") {
		return 0, nil, false
	}
	text = strings.TrimSpace(text[7:])
	if strings.HasPrefix(strings.ToUpper(text), "IGNORE ") {
		text = strings.TrimSpace(text[7:])
	}
	if !strings.HasPrefix(strings.ToUpper(text), "INTO ") {
		return 0, nil, false
	}
	text = strings.TrimSpace(text[5:])
	name, rest, ok := sqlIdentifier(text)
	if !ok {
		return 0, nil, false
	}
	text = strings.TrimSpace(rest)
	if strings.HasPrefix(text, ".") {
		if name != database {
			return 0, nil, false
		}
		name, rest, ok = sqlIdentifier(strings.TrimSpace(text[1:]))
		if !ok {
			return 0, nil, false
		}
		text = strings.TrimSpace(rest)
	}
	if name != table {
		return 0, nil, false
	}
	var names []string
	if strings.HasPrefix(text, "(") {
		text = strings.TrimSpace(text[1:])
		for len(names) < 4096 {
			column, tail, valid := sqlIdentifier(text)
			if !valid {
				return 0, nil, false
			}
			names = append(names, column)
			text = strings.TrimSpace(tail)
			if strings.HasPrefix(text, ")") {
				text = strings.TrimSpace(text[1:])
				break
			}
			if !strings.HasPrefix(text, ",") {
				return 0, nil, false
			}
			text = strings.TrimSpace(text[1:])
		}
	}
	if !strings.HasPrefix(strings.ToUpper(text), "VALUES") {
		return 0, nil, false
	}
	text = strings.TrimSpace(text[6:])
	if !strings.HasPrefix(text, "(") {
		return 0, nil, false
	}
	depth, count := 1, 1
	start := 1
	var quote byte
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
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '(' {
			depth++
		}
		if ch == ')' {
			depth--
			if depth == 0 {
				return count, names, strings.TrimSpace(text[start:i]) != "" && (len(names) == 0 || len(names) == count)
			}
		}
		if ch == ',' && depth == 1 {
			if strings.TrimSpace(text[start:i]) == "" {
				return 0, nil, false
			}
			start = i + 1
			count++
			if count > 4096 {
				return 0, nil, false
			}
		}
	}
	return 0, nil, false
}

// 文件头可能是数据而非合法标识符；仅使映射保持未知，不能破坏独立的连接与权限检查。
func validMappingNames(names []string) bool {
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || len(name) > 256 || name != strings.TrimSpace(name) || !utf8.ValidString(name) || strings.ContainsAny(name, "*,") || strings.ContainsFunc(name, unicode.IsControl) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return len(names) <= 4096
}

func sqlIdentifier(input string) (string, string, bool) {
	if input == "" {
		return "", "", false
	}
	if input[0] == '`' || input[0] == '"' {
		q := input[0]
		var name strings.Builder
		for i := 1; i < len(input); i++ {
			if input[i] == q {
				if i+1 < len(input) && input[i+1] == q {
					name.WriteByte(q)
					i++
					continue
				}
				return name.String(), input[i+1:], name.Len() > 0
			}
			name.WriteByte(input[i])
		}
		return "", "", false
	}
	i := 0
	for i < len(input) && input[i] != ' ' && input[i] != '\t' && input[i] != '\n' && input[i] != '\r' && input[i] != '.' && input[i] != '(' && input[i] != ')' && input[i] != ',' && input[i] != ';' {
		i++
	}
	return input[:i], input[i:], i > 0
}
