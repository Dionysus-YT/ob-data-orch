package store

import (
	"errors"
	"strconv"

	"ob-data-orch/internal/directload"
)

func directLoadTaskSnapshotGroups(raw string, owner bool, precheckID string) ([]TaskSnapshotGroup, error) {
	snapshot, ok := directLoadSnapshot(raw)
	if !ok {
		return nil, errors.New("旁路导入冻结配置不可用")
	}
	c := snapshot.Config
	groups := []TaskSnapshotGroup{{Key: "applicability", Label: "1 · 适用条件"}, {Key: "connection", Label: "2 · 连接目标"}, {Key: "input", Label: "3 · 单表数据文件"}, {Key: "target", Label: "4 · 目标表与格式"}, {Key: "execution", Label: "5 · 执行参数"}, {Key: "review", Label: "6 · 预检查与命令"}}
	add := func(i int, key, label, value string) {
		if value != "" {
			groups[i].Fields = append(groups[i].Fields, TaskSnapshotField{Key: key, Label: label, Value: value, Source: "UNKNOWN"})
		}
	}
	add(0, "commit", "提交边界", "按表整体提交；失败只能从头执行")
	add(1, "dataSourceId", "数据源", c.DataSourceID)
	add(1, "database", "目标数据库", c.Database)
	add(1, "scenario", "连接场景", c.Scenario)
	add(1, "rpcPort", "RPC 端口", strconv.Itoa(c.RPCPort))
	add(1, "rpcPortSource", "RPC 来源", directload.RPCSource)
	add(2, "nodeId", "执行节点", c.NodeID)
	path := c.FilePath
	if !owner {
		path = "已隐藏"
	}
	add(2, "filePath", "输入路径", path)
	add(3, "table", "唯一目标表", c.Table)
	add(3, "format", "数据格式", c.Format)
	add(3, "columnSeparator", "CSV 列分隔符", c.ColumnSeparator)
	add(4, "direct", "旁路模式", "固定生成 --direct")
	add(4, "thread", "客户端连接池", c.Thread)
	add(4, "parallel", "服务端并行度", c.Parallel)
	for _, f := range []struct {
		key, label string
		value      bool
	}{{"externalData", "第三方文件", c.ExternalData}, {"replaceData", "替换重复数据", c.ReplaceData}, {"truncateTable", "导入前清空表", c.TruncateTable}} {
		if f.value {
			add(4, f.key, f.label, "是")
		}
	}
	add(5, "taskName", "任务名称", snapshot.TaskName)
	add(5, "precheckId", "完整预检查", precheckID)
	add(5, "inputFileCount", "预检查输入文件数", strconv.Itoa(snapshot.InputManifest.FileCount))
	add(5, "inputBytes", "预检查输入字节数", strconv.FormatInt(snapshot.InputManifest.TotalBytes, 10))
	return groups, nil
}
