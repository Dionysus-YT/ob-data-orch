package directloadfiles

import (
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importfiles"
)

// ExecutionInput 只携带旁路冻结配置和单表输入清单，不包含连接秘密或客户端声称的结果。
type ExecutionInput struct {
	// DirectLoadConfigFingerprint 冻结固定工具配置三键安全投影，启动前必须重新读取并匹配。
	DirectLoadConfigFingerprint string `json:"directLoadConfigFingerprint"`
	// CompatibilityMode 来自当前授权数据源事实，执行节点不得自行推测。
	CompatibilityMode string               `json:"compatibilityMode"`
	Config            directload.Config    `json:"config"`
	Manifest          importfiles.Manifest `json:"manifest"`
}
