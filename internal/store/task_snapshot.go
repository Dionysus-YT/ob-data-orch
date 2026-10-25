package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// TaskSnapshotField 是冻结配置的字段白名单投影；历史快照未记录输入来源，不能猜测用户或工具默认。
type TaskSnapshotField struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// TaskSnapshotGroup 按各任务自身的向导步骤组织字段，不携带原始快照、秘密或凭据标识。
type TaskSnapshotGroup struct {
	Key    string              `json:"key"`
	Label  string              `json:"label"`
	Fields []TaskSnapshotField `json:"fields"`
}

// GetAuthorizedTaskSnapshotGroups 在任务读取范围内投影提交时配置，路径只对任务创建者展示。
func (s *Store) GetAuthorizedTaskSnapshotGroups(ctx context.Context, taskID, subjectID string, globalRead bool) ([]TaskSnapshotGroup, error) {
	predicate, args, err := buildTaskListPredicate(TaskListQuery{TaskID: taskID, SubjectID: subjectID, GlobalRead: globalRead})
	if err != nil || taskID == "" {
		return nil, errors.New("task snapshot scope is invalid")
	}
	var raw, owner, precheckID, nodeID, sourceID, taskKind string
	err = s.db.QueryRowContext(ctx, `SELECT t.snapshot_json, t.creator_subject_id, t.precheck_id, t.node_id, t.data_source_id,t.task_type `+taskListFrom+` WHERE `+predicate, args...).Scan(&raw, &owner, &precheckID, &nodeID, &sourceID, &taskKind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDataSourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if taskKind == "OBLOADER_DIRECT_LOAD" {
		return directLoadTaskSnapshotGroups(raw, owner == subjectID, precheckID)
	}
	if taskKind == "OBLOADER_NORMAL_IMPORT" {
		return normalImportTaskSnapshotGroups(raw, owner == subjectID, precheckID)
	}
	var frozen struct {
		Database string        `json:"database"`
		Table    string        `json:"table"`
		Format   string        `json:"format"`
		FilePath string        `json:"filePath"`
		Config   *ExportConfig `json:"config"`
	}
	if err := json.Unmarshal([]byte(raw), &frozen); err != nil {
		return nil, errors.New("task snapshot is unavailable")
	}
	cfg := frozen.Config
	if cfg == nil {
		cfg = &ExportConfig{ObjectScope: ObjectScope{Database: frozen.Database, ScopeKind: "SPECIFIED"}, DataFormat: DataFormat{FormatKind: frozen.Format}, OutputConfig: OutputConfig{OutputKind: "LOCAL", FilePath: frozen.FilePath}}
	}
	groups := []TaskSnapshotGroup{{Key: "source", Label: "1 · 数据源与执行节点"}, {Key: "objects", Label: "2 · 导出对象与内容"}, {Key: "format", Label: "3 · 数据格式"}, {Key: "output", Label: "4 · 输出与性能"}, {Key: "review", Label: "5 · 预检查与提交"}}
	add := func(group int, key, label string, value any) {
		text := "—"
		switch v := value.(type) {
		case string:
			if v != "" {
				text = v
			}
		case bool:
			if v {
				text = "是"
			} else {
				text = "否"
			}
		default:
			bytes, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				return
			}
			if string(bytes) != "null" && string(bytes) != "[]" {
				text = string(bytes)
			}
		}
		groups[group].Fields = append(groups[group].Fields, TaskSnapshotField{Key: key, Label: label, Value: text, Source: "UNKNOWN"})
	}
	add(0, "dataSourceId", "数据源", sourceID)
	add(0, "nodeId", "执行节点", nodeID)
	// 只从类型化领域配置选择明确字段；未知 JSON 键和存储凭据引用不进入响应。
	sections := []struct {
		group  int
		value  any
		fields []string
	}{
		{1, cfg.ObjectScope, []string{"database:数据库", "scopeKind:范围", "objectTypes:对象类型", "expressions:对象表达式", "excludeTables:排除表"}},
		{1, cfg.ContentSelection, []string{"contentKind:导出内容"}},
		{1, cfg.FilterConfig, []string{"querySql:SELECT 查询", "queryResultLimit:查询行数上限", "where:WHERE 条件", "partition:分区", "includeColumnNames:包含列", "excludeColumnNames:排除列", "excludeDataTypes:排除数据类型", "excludeVirtualColumns:排除虚拟列", "enableHiddenPk:隐藏主键", "flashbackScn:闪回 SCN", "flashbackTimestamp:闪回时间", "snapshot:快照", "weakRead:弱读"}},
		{1, cfg.DDLBehavior, []string{"dropObject:删除对象语句", "addExtraMessage:附加元信息", "retainSchema:保留 Schema", "compactSchema:精简 Schema", "sequencePolicy:序列策略"}},
		{2, cfg.DataFormat, []string{"formatKind:数据格式"}},
		{2, cfg.DataFormat.CsvOptions, []string{"skipHeader:跳过表头", "columnSeparator:列分隔符", "columnQuote:引用符", "columnQuoteMode:引用模式", "escapeCharacter:转义符", "lineSeparator:行分隔符", "nullString:NULL 替换", "fileEncoding:编码", "withTrim:去除空格", "columnSplitter:列分隔字符串"}},
		{2, cfg.DataFormat.CutOptions, []string{"trailDelimiter:尾部分隔符", "removeNewline:去除换行"}},
		{2, cfg.DataFormat.TimestampFormats, []string{"dateValueFormat:日期格式", "timeValueFormat:时间格式", "dateTimeValueFormat:日期时间格式", "timestampValueFormat:时间戳格式", "timestampTzValueFormat:带时区时间戳格式", "timestampLtzValueFormat:本地时区时间戳格式", "nlsDateFormat:NLS 日期格式", "nlsTimestampFormat:NLS 时间戳格式", "nlsTimestampTzFormat:NLS 带时区时间戳格式", "preserveZeroDatetime:保留零日期"}},
		{3, cfg.OutputConfig, []string{"outputKind:输出类型", "filePath:输出位置", "logPath:工具日志位置", "skipCheckDir:跳过目录检查", "noNestedDir:不创建嵌套目录", "maxFileSize:最大文件大小", "retainEmptyFiles:保留空文件", "compress:压缩", "compressionAlgo:压缩算法", "compressionLevel:压缩等级", "controlFilePath:控制文件位置", "tmpPath:临时目录"}},
		{3, cfg.PerformanceConfig, []string{"thread:线程数", "pageSize:分页大小", "parallelMacro:宏块并行", "fetchSize:抓取大小", "jvmMemory:JVM 内存", "retry:检查点继续", "blockSize:文件拆分大小"}},
	}
	for _, section := range sections {
		encoded, err := json.Marshal(section.value)
		if err != nil {
			return nil, err
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &values); err != nil {
			return nil, err
		}
		for _, definition := range section.fields {
			var key, label string
			for i, ch := range definition {
				if ch == ':' {
					key, label = definition[:i], definition[i+1:]
					break
				}
			}
			rawValue, exists := values[key]
			if !exists {
				continue
			}
			if key == "filePath" || key == "logPath" || key == "controlFilePath" || key == "tmpPath" {
				if owner != subjectID {
					add(section.group, key, label, "仅创建者可见")
					continue
				}
			}
			var value any
			// 大 SCN 和文件大小保持冻结整数精度，不先转换为 float64。
			decoder := json.NewDecoder(bytes.NewReader(rawValue))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				return nil, err
			}
			add(section.group, key, label, value)
		}
	}
	if frozen.Table != "" {
		add(1, "table", "导出表", frozen.Table)
	}
	if cfg.OutputConfig.StorageCredential != nil {
		add(3, "storageCredential", "存储凭据", "已脱敏")
	}
	add(4, "precheckId", "预检查", precheckID)
	return groups, nil
}
