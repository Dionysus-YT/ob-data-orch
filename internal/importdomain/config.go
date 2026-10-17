// Package importdomain 独立管理普通导入的配置、条件与参数映射，不依赖导出或旁路领域。
package importdomain

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/inputpath"
)

const (
	// ConfigVersion 是普通导入独立的非敏感配置版本。
	ConfigVersion = "normal-import-v1"
	// MetadataVersion 是普通导入独立且冻结的参数目录版本。
	MetadataVersion = "obloader-4.3.5-normal-v1"
	// CapabilityVersion 只表示本地三格式普通导入，不授予真实执行资格。
	CapabilityVersion = "import-odp-local-v1"
	// TaskType 隔离普通导入与导出、旁路的冻结任务语义。
	TaskType = "OBLOADER_NORMAL_IMPORT"
	// ToolVersion 是当前配置绑定的官方目标版本。
	ToolVersion = "4.3.5-RELEASE"
)

// Config 是唯一可持久化的普通导入配置；不含凭据、命令或客户端提供的运行事实。
type Config struct {
	DataSourceID  string            `json:"dataSourceId"`
	NodeID        string            `json:"nodeId"`
	Database      string            `json:"database"`
	FilePath      string            `json:"filePath"`
	FileSuffix    string            `json:"fileSuffix,omitempty"`
	FileEncoding  string            `json:"fileEncoding,omitempty"`
	FileRegex     string            `json:"fileRegex,omitempty"`
	LogPath       string            `json:"logPath,omitempty"`
	Content       string            `json:"content"`
	Format        string            `json:"format,omitempty"`
	Scope         string            `json:"scope"`
	Objects       map[string]string `json:"objects,omitempty"`
	ExcludeTables string            `json:"excludeTables,omitempty"`
	Parser        Parser            `json:"parser"`
	Mapping       *Mapping          `json:"mapping,omitempty"`
	Execution     Execution         `json:"execution"`
	CompatMysql   bool              `json:"compatMysql,omitempty"`
	ReplaceData   bool              `json:"replaceData,omitempty"`
	ReplaceObject bool              `json:"replaceObject,omitempty"`
	Cleanup       string            `json:"cleanup"`
	TaskName      string            `json:"taskName,omitempty"`
}

// Parser 只承载当前活动格式，不能持久化隐藏格式或未经核对的解析开关。
type Parser struct {
	ColumnSeparator  string `json:"columnSeparator,omitempty"`
	ColumnQuote      string `json:"columnQuote,omitempty"`
	ColumnSplitter   string `json:"columnSplitter,omitempty"`
	LineSeparator    string `json:"lineSeparator,omitempty"`
	EscapeCharacter  string `json:"escapeCharacter,omitempty"`
	SkipHeader       bool   `json:"skipHeader,omitempty"`
	SkipFooter       bool   `json:"skipFooter,omitempty"`
	IgnoreEscape     bool   `json:"ignoreEscape,omitempty"`
	TrailDelimiter   bool   `json:"trailDelimiter,omitempty"`
	WithTrim         bool   `json:"withTrim,omitempty"`
	NullString       string `json:"nullString,omitempty"`
	EmptyString      string `json:"emptyString,omitempty"`
	Compress         bool   `json:"compress,omitempty"`
	CompressionAlgo  string `json:"compressionAlgo,omitempty"`
	CompressionLevel string `json:"compressionLevel,omitempty"`
	DateFormat       string `json:"dateFormat,omitempty"`
	DefaultDate      string `json:"defaultDate,omitempty"`
}

// Mapping 的映射方式严格单选，显式包含与排除名单由平台规则择一。
type Mapping struct {
	Kind             string `json:"kind"`
	CtlPath          string `json:"ctlPath,omitempty"`
	IncludeColumns   string `json:"includeColumns,omitempty"`
	ExcludeColumns   string `json:"excludeColumns,omitempty"`
	ExcludeDataTypes string `json:"excludeDataTypes,omitempty"`
}

// Execution 保存显式覆盖；空字符串表示继承官方默认，不接受连接字符集或会话文件。
type Execution struct {
	MaxDiscards          string `json:"maxDiscards,omitempty"`
	MaxErrors            string `json:"maxErrors,omitempty"`
	Strict               string `json:"strict,omitempty"`
	Thread               string `json:"thread,omitempty"`
	RW                   string `json:"rw,omitempty"`
	Batch                string `json:"batch,omitempty"`
	BlockSize            string `json:"blockSize,omitempty"`
	MaxTPS               string `json:"maxTps,omitempty"`
	MaxWaitTimeout       string `json:"maxWaitTimeout,omitempty"`
	Memory               string `json:"memory,omitempty"`
	NLSDateFormat        string `json:"nlsDateFormat,omitempty"`
	NLSTimestampFormat   string `json:"nlsTimestampFormat,omitempty"`
	NLSTimestampTZFormat string `json:"nlsTimestampTzFormat,omitempty"`
}

// Issue 只返回固定字段和规则码，不回显原输入、路径或任何秘密。
type Issue struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

var objectTypes = []string{"table", "table-group", "view", "trigger", "obj-user", "role", "sequence", "synonym", "type", "type-body", "package", "package-body", "function", "procedure"}
var oracleTypes = []string{"role", "sequence", "synonym", "type", "type-body", "package", "package-body"}
var integerPattern = regexp.MustCompile(`^-?[0-9]+$`)
var memoryPattern = regexp.MustCompile(`^[1-9][0-9]*[mMgG]$`)

// Decode 对客户端与持久化数据使用同一严格结构，拒绝未知字段和尾随 JSON。
func Decode(raw string) (Config, error) {
	var config Config
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 128<<10 || decoder.Decode(&config) != nil {
		return Config{}, errors.New("普通导入配置无效")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("普通导入配置无效")
	}
	return config, nil
}

// Normalize 返回独立的活动配置；不修改调用方输入，路径始终保持原样。
// complete=false 允许保存未填完的草稿，但类型、枚举与已填写活动值仍需合法。
func Normalize(input Config, mode, platform string, complete bool) (Config, []Issue) {
	c := input
	c.Objects = make(map[string]string)
	var issues []Issue
	add := func(field, code string) { issues = append(issues, Issue{field, code}) }
	requires := func(field, value string) {
		if complete && strings.TrimSpace(value) == "" {
			add(field, "REQUIRED")
		}
	}
	requires("dataSourceId", c.DataSourceID)
	requires("nodeId", c.NodeID)
	requires("database", c.Database)
	requires("filePath", c.FilePath)
	if !oneOf(mode, "MYSQL", "ORACLE") {
		add("dataSourceId", "UNSUPPORTED_MODE")
	}
	if c.Content != "" && !oneOf(c.Content, "DDL_ONLY", "DATA_ONLY", "DDL_AND_DATA") {
		add("content", "UNSUPPORTED_CONTENT")
	}
	requires("content", c.Content)
	data, ddl := c.HasData(), c.HasDDL()
	if c.Format != "" && !oneOf(c.Format, "CSV", "CUT", "SQL") {
		add("format", "UNSUPPORTED_FORMAT")
	}
	if data {
		requires("format", c.Format)
	} else {
		c.Format = ""
		c.Parser = Parser{}
		c.Mapping = nil
		c.ReplaceData = false
		c.Cleanup = "NONE"
	}
	if c.Scope != "" && !oneOf(c.Scope, "ALL", "SPECIFIC") {
		add("scope", "UNSUPPORTED_SCOPE")
	}
	requires("scope", c.Scope)
	for name, value := range input.Objects {
		if !oneOf(name, objectTypes...) {
			add("objects", "UNSUPPORTED_OBJECT")
			continue
		}
		if c.Scope != "SPECIFIC" || !ddl && name != "table" || mode != "ORACLE" && oneOf(name, oracleTypes...) {
			continue
		}
		if strings.TrimSpace(value) != "" {
			c.Objects[name] = value
		}
	}
	if complete && c.Scope == "SPECIFIC" && len(c.Objects) == 0 {
		add("objects", "REQUIRED")
	}
	if c.Scope != "ALL" && c.Objects["table"] == "" {
		c.ExcludeTables = ""
	}
	if !ddl {
		c.ReplaceObject = false
		c.CompatMysql = false
	}
	if mode != "MYSQL" {
		c.CompatMysql = false
		c.Parser.DateFormat = ""
		c.Parser.DefaultDate = ""
	}
	if !oneOf(c.Cleanup, "", "NONE", "TRUNCATE", "DELETE") {
		add("cleanup", "UNSUPPORTED_CLEANUP")
	}
	if c.Cleanup == "" {
		c.Cleanup = "NONE"
	}
	if !data {
		c.Execution = Execution{Thread: c.Execution.Thread, Memory: c.Execution.Memory, NLSDateFormat: c.Execution.NLSDateFormat, NLSTimestampFormat: c.Execution.NLSTimestampFormat, NLSTimestampTZFormat: c.Execution.NLSTimestampTZFormat}
	}
	if mode != "ORACLE" {
		c.Execution.NLSDateFormat = ""
		c.Execution.NLSTimestampFormat = ""
		c.Execution.NLSTimestampTZFormat = ""
	}
	if c.Format != "CSV" {
		c.Parser.ColumnQuote = ""
		c.Parser.ColumnSeparator = ""
	}
	if c.Format != "CUT" {
		c.Parser.ColumnSplitter = ""
		c.Parser.SkipFooter = false
		c.Parser.IgnoreEscape = false
		c.Parser.TrailDelimiter = false
	}
	if c.Format == "SQL" {
		c.Parser.EscapeCharacter = ""
		c.Parser.EmptyString = ""
		c.Parser.SkipHeader = false
		c.Parser.WithTrim = false
		c.Parser.DateFormat = ""
		c.Parser.DefaultDate = ""
	}
	if c.Parser.IgnoreEscape {
		c.Parser.EscapeCharacter = ""
	}
	if !c.Parser.Compress {
		c.Parser.CompressionAlgo = ""
		c.Parser.CompressionLevel = ""
	}
	if oneOf(c.Parser.CompressionAlgo, "gzip", "snappy") {
		c.Parser.CompressionLevel = ""
	}
	if c.Parser.Compress && !oneOf(c.Parser.CompressionAlgo, "", "zstd", "zlib", "gzip", "snappy") {
		add("compressionAlgo", "UNSUPPORTED_ALGORITHM")
	}
	if c.Parser.CompressionLevel != "" {
		min, max := int64(1), int64(22)
		if c.Parser.CompressionAlgo == "zlib" {
			min, max = -1, 9
		}
		if !integer(c.Parser.CompressionLevel, min, max) {
			add("compressionLevel", "OUT_OF_RANGE")
		}
	}
	for _, field := range []struct{ name, value string }{{"columnQuote", c.Parser.ColumnQuote}, {"escapeCharacter", c.Parser.EscapeCharacter}} {
		if field.value != "" && utf8.RuneCountInString(field.value) != 1 {
			add(field.name, "SINGLE_CHARACTER_REQUIRED")
		}
	}
	if data && c.Format != "SQL" && fallback(c.Parser.NullString, `\N`) == fallback(c.Parser.EmptyString, `\E`) {
		add("emptyString", "NULL_EMPTY_CONFLICT")
	}
	if c.Parser.DefaultDate != "" && c.Parser.DateFormat == "" {
		add("dateFormat", "REQUIRED")
	}
	if c.Mapping != nil {
		m := *c.Mapping
		c.Mapping = &m
		if !oneOf(m.Kind, "POSITION", "CONTROL", "AUTO", "EXPLICIT") {
			add("mapping", "UNSUPPORTED_MAPPING")
		}
		if m.Kind == "AUTO" && c.Format != "CSV" {
			add("mapping", "AUTOMATIC_MAPPING_UNSUPPORTED")
		}
		if m.Kind != "CONTROL" {
			c.Mapping.CtlPath = ""
		} else {
			requires("ctlPath", m.CtlPath)
		}
		if m.Kind != "EXPLICIT" {
			c.Mapping.IncludeColumns = ""
			c.Mapping.ExcludeColumns = ""
		} else {
			if m.IncludeColumns != "" && m.ExcludeColumns != "" {
				add("includeColumns", "COLUMN_LIST_CONFLICT")
			}
			if complete && strings.TrimSpace(m.IncludeColumns+m.ExcludeColumns) == "" {
				add("includeColumns", "REQUIRED")
			}
		}
	} else if data && complete {
		c.Mapping = &Mapping{Kind: "POSITION"}
	}
	for _, field := range []struct {
		name, value string
		min         int64
	}{{"maxDiscards", c.Execution.MaxDiscards, -1}, {"maxErrors", c.Execution.MaxErrors, -1}, {"thread", c.Execution.Thread, 1}, {"batch", c.Execution.Batch, 1}, {"blockSize", c.Execution.BlockSize, 1}, {"maxTps", c.Execution.MaxTPS, 1}, {"maxWaitTimeout", c.Execution.MaxWaitTimeout, 1}} {
		if field.value != "" && !integer(field.value, field.min, 9007199254740991) {
			add(field.name, "OUT_OF_RANGE")
		}
	}
	if c.Execution.RW != "" {
		value, err := strconv.ParseFloat(c.Execution.RW, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 || strings.ContainsAny(c.Execution.RW, "eE+-") {
			add("rw", "OUT_OF_RANGE")
		}
	}
	if !oneOf(c.Execution.Strict, "", "true", "false") {
		add("strict", "UNSUPPORTED_VALUE")
	}
	if c.Execution.Memory != "" && !memoryPattern.MatchString(c.Execution.Memory) {
		add("memory", "INVALID_MEMORY")
	}
	for _, field := range []struct{ name, value string }{{"filePath", c.FilePath}, {"logPath", c.LogPath}} {
		if field.value != "" && (complete || platform != "") && !inputpath.Valid(platform, field.value) {
			add(field.name, "ABSOLUTE_PATH_REQUIRED")
		}
	}
	if c.Mapping != nil && c.Mapping.CtlPath != "" && (complete || platform != "") && !inputpath.Valid(platform, c.Mapping.CtlPath) {
		add("ctlPath", "ABSOLUTE_PATH_REQUIRED")
	}
	// 活动标量统一限制大小与控制字符；正则和路径也不接受额外参数或秘密通道。
	raw, _ := json.Marshal(c)
	var object any
	_ = json.Unmarshal(raw, &object)
	var walk func(any, string)
	walk = func(value any, field string) {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				walk(item, key)
			}
		case string:
			if len(v) > 4096 || strings.ContainsFunc(v, unicode.IsControl) {
				add(field, "INVALID_TEXT")
			}
		}
	}
	walk(object, "config")
	return c, issues
}

// HasData 与 HasDDL 只按独立内容模型判断活动能力，不沿用导出资格。
func (c Config) HasData() bool { return oneOf(c.Content, "DATA_ONLY", "DDL_AND_DATA") }

// HasDDL 判断是否需要处理对象定义文件。
func (c Config) HasDDL() bool { return oneOf(c.Content, "DDL_ONLY", "DDL_AND_DATA") }

// HighRisk 指明必须绑定冻结指纹确认的写入或结构影响。
func (c Config) HighRisk() bool {
	return c.ReplaceData || c.ReplaceObject || oneOf(c.Cleanup, "TRUNCATE", "DELETE")
}
func oneOf(value string, values ...string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}
func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
func integer(value string, min, max int64) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return integerPattern.MatchString(value) && err == nil && n >= min && n <= max
}
