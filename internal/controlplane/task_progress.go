package controlplane

import "ob-data-orch/internal/store"

// taskProgressEvidence 统一列表、概览和执行详情的证据级别；缺省兼容未接入的旧 Agent。
func taskProgressEvidence(progress *store.TaskProgress) (stage, amount string) {
	if progress == nil {
		return "UNAVAILABLE", "UNAVAILABLE"
	}
	if progress.Percent != nil {
		return "RELIABLE", "PERCENT"
	}
	return "RELIABLE", "UNAVAILABLE"
}
