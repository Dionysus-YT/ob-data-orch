package controlplane

import "ob-data-orch/internal/store"

func projectedTaskType(summary store.TaskSummary) string {
	if summary.TaskType != "" {
		return summary.TaskType
	}
	return "OBDUMPER_EXPORT"
}
