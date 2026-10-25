// Package directload 管理旁路导入独立配置与固定预检规则，不继承普通导入参数或执行资格。
package directload

import (
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/inputpath"
)

const (
	// ConfigVersion 标识旁路独立的非敏感配置格式。
	ConfigVersion = "direct-load-v1"
	// MetadataVersion 冻结旁路参数目录，不改变普通导入或导出快照。
	MetadataVersion = "obloader-4.3.5-direct-v1"
	// CapabilityVersion 只表示本地文件旁路切片，不授予真实执行资格。
	CapabilityVersion = "direct-load-local-v1"
	// TaskType 隔离旁路与普通导入的任务语义。
	TaskType = "OBLOADER_DIRECT_LOAD"
	// ToolVersion 是唯一允许的官方目标版本。
	ToolVersion = "4.3.5-RELEASE"
	// RPCSource 记录本轮唯一可追溯的 RPC 端口输入来源，不依据 SQL 端口推测。
	RPCSource = "MANUAL"
)

// Config 只保存用户可编辑的活动配置，不接收凭据、命令或客户端声称的运行事实。
type Config struct {
	DataSourceID    string        `json:"dataSourceId"`
	NodeID          string        `json:"nodeId"`
	Database        string        `json:"database"`
	Scenario        string        `json:"scenario"`
	RPCPort         int           `json:"rpcPort"`
	Table           string        `json:"table"`
	FilePath        string        `json:"filePath"`
	Format          string        `json:"format"`
	ExternalData    bool          `json:"externalData,omitempty"`
	ColumnSeparator string        `json:"columnSeparator,omitempty"`
	Thread          string        `json:"thread,omitempty"`
	Parallel        string        `json:"parallel,omitempty"`
	ReplaceData     bool          `json:"replaceData,omitempty"`
	TruncateTable   bool          `json:"truncateTable,omitempty"`
	Applicability   Applicability `json:"applicability"`
}

// Applicability 仅表示入口已阅读确认，不能代替绑定当前快照的最终风险确认或预检事实。
type Applicability struct {
	SingleTable      bool `json:"singleTable"`
	WholeTableCommit bool `json:"wholeTableCommit"`
	NoCheckpoint     bool `json:"noCheckpoint"`
	StructureLimits  bool `json:"structureLimits"`
}

// Issue 仅返回固定字段和规则码，避免回显原始输入、路径或秘密。
type Issue struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

// Decode 拒绝未知字段、重复键、空值、无效 UTF-8 和尾随 JSON，客户端与持久化配置采用同一规则。
func Decode(raw string) (Config, error) {
	invalid := errors.New("旁路导入配置无效")
	if len(raw) > 32<<10 || !utf8.ValidString(raw) || !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return Config{}, invalid
	}
	d := json.NewDecoder(strings.NewReader(raw))
	if strictValue(d, 0) != nil {
		return Config{}, invalid
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return Config{}, invalid
	}
	d = json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	var c Config
	if d.Decode(&c) != nil {
		return Config{}, invalid
	}
	return c, nil
}

// strictValue 在每个对象作用域拒绝重复键；深度上限限制恶意配置的解析开销。
func strictValue(d *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("配置层级无效")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t == nil {
		return errors.New("配置不接受空值")
	}
	if delim, ok := t.(json.Delim); ok {
		if delim != '{' {
			return errors.New("配置结构无效")
		}
		keys := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] || !knownConfigKey(depth, name) {
				return errors.New("配置键无效")
			}
			keys[name] = true
			if err := strictValue(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("配置对象无效")
		}
	}
	return nil
}

// knownConfigKey 使用契约的精确拼写，避免 encoding/json 的大小写兼容把别名键合并覆盖。
func knownConfigKey(depth int, name string) bool {
	if depth == 0 {
		return oneOf(name, "dataSourceId", "nodeId", "database", "scenario", "rpcPort", "table", "filePath", "format", "externalData", "columnSeparator", "thread", "parallel", "replaceData", "truncateTable", "applicability")
	}
	return depth == 1 && oneOf(name, "singleTable", "wholeTableCommit", "noCheckpoint", "structureLimits")
}

// Normalize 返回活动配置副本；草稿可不完整，但所有已填写值仍须满足已确认能力边界。
// 路径按节点平台原样校验，未知事实不在此处推定为通过。
func Normalize(input Config, mode, platform string, complete bool) (Config, []Issue) {
	c := input
	var issues []Issue
	add := func(field, code string) { issues = append(issues, Issue{field, code}) }
	require := func(field, value string) {
		if complete && strings.TrimSpace(value) == "" {
			add(field, "REQUIRED")
		}
	}
	for _, f := range []struct{ name, value string }{{"dataSourceId", c.DataSourceID}, {"nodeId", c.NodeID}, {"database", c.Database}, {"scenario", c.Scenario}, {"table", c.Table}, {"filePath", c.FilePath}, {"format", c.Format}} {
		require(f.name, f.value)
		if len(f.value) > 4096 || strings.ContainsFunc(f.value, unicode.IsControl) || strings.HasPrefix(f.value, "-") {
			add(f.name, "INVALID_TEXT")
		}
	}
	if mode != "MYSQL" && mode != "ORACLE" {
		add("dataSourceId", "UNSUPPORTED_MODE")
	}
	if c.Scenario != "" && !oneOf(c.Scenario, "PRIVATE_ODP", "OBSERVER_DIRECT") {
		add("scenario", "UNSUPPORTED_SCENARIO")
	}
	if c.RPCPort < 0 || c.RPCPort > 65535 || complete && c.RPCPort == 0 {
		add("rpcPort", "OUT_OF_RANGE")
	}
	for _, f := range []struct{ name, value string }{{"database", c.Database}, {"table", c.Table}} {
		if f.value != "" && !identifier(f.value) {
			add(f.name, "SINGLE_IDENTIFIER_REQUIRED")
		}
	}
	if c.FilePath != "" && (platform != "" || complete) && !inputpath.Valid(platform, c.FilePath) {
		add("filePath", "ABSOLUTE_PATH_REQUIRED")
	}
	if c.Format != "" && c.Format != "CSV" {
		add("format", "UNSUPPORTED_FORMAT")
	}
	// 非 CSV 的共享解析字段尚无旁路适用证据，不进入活动配置或最终命令。
	if c.Format != "CSV" {
		c.ColumnSeparator = ""
		c.ExternalData = false
	}
	if len(c.ColumnSeparator) > 32 || strings.ContainsFunc(c.ColumnSeparator, unicode.IsControl) || strings.HasPrefix(c.ColumnSeparator, "-") {
		add("columnSeparator", "INVALID_TEXT")
	}
	for _, f := range []struct{ name, value string }{{"thread", c.Thread}, {"parallel", c.Parallel}} {
		if f.value != "" && !positiveInteger(f.value) {
			add(f.name, "OUT_OF_RANGE")
		}
	}
	if complete && !(c.Applicability.SingleTable && c.Applicability.WholeTableCommit && c.Applicability.NoCheckpoint && c.Applicability.StructureLimits) {
		add("applicability", "CONFIRMATION_REQUIRED")
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Field != issues[j].Field {
			return issues[i].Field < issues[j].Field
		}
		return issues[i].Code < issues[j].Code
	})
	return c, issues
}

// HighRisk 始终要求旁路最终确认；入口勾选不授予写入或清空权限。
func (c Config) HighRisk() bool { return true }

func identifier(value string) bool {
	if len(value) > 256 || value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' && r != '-' {
			return false
		}
	}
	return !strings.HasPrefix(value, "-")
}
func positiveInteger(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(value, 10, 32)
	return err == nil && n > 0
}
func oneOf(value string, values ...string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}
