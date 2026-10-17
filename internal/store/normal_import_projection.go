package store

import (
	"encoding/json"
	"errors"
	"strconv"
)

func normalImportTaskSnapshotGroups(raw string, owner bool, precheckID string) ([]TaskSnapshotGroup, error) {
	snapshot, ok := normalImportSnapshot(raw)
	if !ok {
		return nil, errors.New("普通导入冻结配置不可用")
	}
	c := snapshot.Config
	groups := []TaskSnapshotGroup{{Key: "source", Label: "1 · 选择数据源"}, {Key: "input", Label: "2 · 导入文件与格式"}, {Key: "objects", Label: "3 · 对象与映射"}, {Key: "execution", Label: "4 · 执行参数"}, {Key: "review", Label: "5 · 预检查与命令"}}
	add := func(group int, key, label, value string) {
		if value != "" {
			groups[group].Fields = append(groups[group].Fields, TaskSnapshotField{Key: key, Label: label, Value: value, Source: "UNKNOWN"})
		}
	}
	path := func(group int, key, label, value string) {
		if value != "" {
			if !owner {
				value = "已隐藏"
			}
			add(group, key, label, value)
		}
	}
	encoded := func(value any) string {
		raw, _ := json.Marshal(value)
		if string(raw) == "{}" || string(raw) == "null" {
			return ""
		}
		return string(raw)
	}
	add(0, "dataSourceId", "目标数据源", c.DataSourceID)
	add(0, "database", "目标数据库", c.Database)
	add(1, "nodeId", "执行节点", c.NodeID)
	add(1, "content", "导入内容", c.Content)
	add(1, "format", "数据格式", c.Format)
	path(1, "filePath", "输入路径", c.FilePath)
	path(1, "logPath", "日志目录", c.LogPath)
	add(1, "fileSuffix", "文件后缀", c.FileSuffix)
	add(1, "fileEncoding", "文件编码", c.FileEncoding)
	add(1, "fileRegex", "文件正则", c.FileRegex)
	add(1, "parser", "活动解析参数", encoded(c.Parser))
	add(2, "scope", "对象范围", c.Scope)
	add(2, "objects", "指定对象", encoded(c.Objects))
	add(2, "excludeTables", "排除表", c.ExcludeTables)
	if c.Mapping != nil {
		add(2, "mapping", "列映射方式", c.Mapping.Kind)
		path(2, "ctlPath", "控制文件目录", c.Mapping.CtlPath)
		add(2, "includeColumns", "包含列", c.Mapping.IncludeColumns)
		add(2, "excludeColumns", "排除列", c.Mapping.ExcludeColumns)
		add(2, "excludeDataTypes", "排除类型", c.Mapping.ExcludeDataTypes)
	}
	add(3, "execution", "显式执行参数", encoded(c.Execution))
	add(3, "cleanup", "导入前处理", c.Cleanup)
	for _, field := range []struct {
		key, label string
		enabled    bool
	}{{"replaceData", "替换重复数据", c.ReplaceData}, {"replaceObject", "替换已有对象", c.ReplaceObject}, {"compatMysql", "MySQL 定义兼容", c.CompatMysql}} {
		if field.enabled {
			add(3, field.key, field.label, "是")
		}
	}
	name := snapshot.TaskName
	if name == "" {
		name = c.TaskName
	}
	add(4, "taskName", "任务名称", name)
	add(4, "precheckId", "完整预检查", precheckID)
	// 输入集合只作为提交时证据，不能放进结果摘要冒充数据库已导入行数或输出文件。
	add(4, "inputFileCount", "预检查输入文件数", strconv.Itoa(snapshot.InputManifest.FileCount))
	add(4, "inputBytes", "预检查输入字节数", strconv.FormatInt(snapshot.InputManifest.TotalBytes, 10))
	return groups, nil
}
