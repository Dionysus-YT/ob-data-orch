package importdomain

import (
	"encoding/json"
	"errors"
)

// ErrReuseIncompatible 表示原活动配置无法按当前绑定原样使用。
var ErrReuseIncompatible = errors.New("普通导入配置与当前绑定不兼容")

// CanDeriveTaskDraft 只允许可信终态派生；成功任务只新建草稿，失败或取消后可从头复验。
func CanDeriveTaskDraft(state string, reconciliationRequired bool, kind string) bool {
	if reconciliationRequired || kind != "REBUILD_FROM_CONFIG" && kind != "RERUN_FROM_SCRATCH" {
		return false
	}
	return state == "FAILED" || state == "CANCELLED" || state == "SUCCEEDED" && kind == "REBUILD_FROM_CONFIG"
}

// BindReusableConfig 用当前绑定校验复用配置，不允许静默删除不适用对象或改写路径。
// 只产生草稿配置；预检查、风险确认和执行资格必须在新会话重新取得。
func BindReusableConfig(config Config, sourceID, nodeID, mode, platform string) (Config, error) {
	config.DataSourceID, config.NodeID, config.TaskName = sourceID, nodeID, ""
	normalized, issues := Normalize(config, mode, platform, false)
	before, _ := json.Marshal(config)
	after, _ := json.Marshal(normalized)
	if len(issues) != 0 || string(before) != string(after) {
		return Config{}, ErrReuseIncompatible
	}
	return normalized, nil
}
