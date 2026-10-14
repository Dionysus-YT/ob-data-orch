package exportdomain

// CanDeriveTaskDraft 只允许可信终态复制配置；成功任务只新建可编辑草稿，失败或取消任务也可从头执行。
func CanDeriveTaskDraft(state string, reconciliationRequired bool, kind string) bool {
	if reconciliationRequired {
		return false
	}
	if kind != "REBUILD_FROM_CONFIG" && kind != "RERUN_FROM_SCRATCH" {
		return false
	}
	return state == "FAILED" || state == "CANCELLED" || state == "SUCCEEDED" && kind == "REBUILD_FROM_CONFIG"
}
