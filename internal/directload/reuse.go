package directload

import (
	"encoding/json"
	"errors"
)

// ErrReuseIncompatible 表示旁路活动配置不能按当前引用原样复用。
var ErrReuseIncompatible = errors.New("旁路导入配置与当前绑定不兼容")

// CanDeriveTaskDraft 只允许已确认终态派生；成功任务仅可基于配置新建。
func CanDeriveTaskDraft(state string, reconciliationRequired bool, kind string) bool {
	if reconciliationRequired || kind != "REBUILD_FROM_CONFIG" && kind != "RERUN_FROM_SCRATCH" {
		return false
	}
	return state == "FAILED" || state == "CANCELLED" || state == "SUCCEEDED" && kind == "REBUILD_FROM_CONFIG"
}

// BindReusableConfig 只重新校验当前引用，不复制原预检查、提交确认或输入清单。
func BindReusableConfig(config Config, sourceID, nodeID, mode, platform string) (Config, error) {
	config.DataSourceID, config.NodeID = sourceID, nodeID
	active, issues := Normalize(config, mode, platform, false)
	before, _ := json.Marshal(config)
	after, _ := json.Marshal(active)
	if len(issues) != 0 || string(before) != string(after) {
		return Config{}, ErrReuseIncompatible
	}
	return active, nil
}
