//go:build !queuevalidation

package main

import "ob-data-orch/internal/agentexecution"

// queueValidationObserver 在标准 Agent 构建中始终关闭本地故障演练屏障。
// 正式产物不能通过环境变量、任务参数或网络请求启用该测试路径。
func queueValidationObserver(string) agentexecution.LogQueueObserver {
	return nil
}
