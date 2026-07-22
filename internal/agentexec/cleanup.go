package agentexec

import (
	"errors"

	"ob-data-orch/internal/credential"
)

var (
	ErrCleanupNotReady = errors.New("终态证据未完成，不能清理执行材料")
	ErrCleanupFailed   = errors.New("执行材料清理失败，目录必须隔离")
)

// CleanupResult 是安全材料清理的无路径摘要，供节点阻断和审计使用。
type CleanupResult struct {
	Cleaned  bool
	Residual bool
}

// CleanupSynthetic 只在终态和结果证据已完成后清理 execution 私有材料。
// 任意删除失败都保留目录并返回隔离结论，不能通过新建 workspace 掩盖残留。
func CleanupSynthetic(workspace credential.Workspace, evidenceComplete bool) (CleanupResult, error) {
	if !evidenceComplete {
		return CleanupResult{}, ErrCleanupNotReady
	}
	if err := RemoveStartIntent(workspace); err != nil {
		return CleanupResult{Residual: true}, ErrCleanupFailed
	}
	if err := workspace.Cleanup(); err != nil {
		return CleanupResult{Residual: true}, ErrCleanupFailed
	}
	return CleanupResult{Cleaned: true}, nil
}
