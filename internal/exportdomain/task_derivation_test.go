package exportdomain

import "testing"

func Test终态派生规则与核对失败关闭(t *testing.T) {
	for _, state := range []string{"WAITING_SCHEDULE", "STARTING", "RUNNING", "CANCELLING", "SUCCEEDED", "FAILED", "CANCELLED", "UNKNOWN"} {
		for _, kind := range []string{"REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH", "UNKNOWN"} {
			want := kind != "UNKNOWN" && (state == "FAILED" || state == "CANCELLED" || state == "SUCCEEDED" && kind == "REBUILD_FROM_CONFIG")
			if CanDeriveTaskDraft(state, false, kind) != want || CanDeriveTaskDraft(state, true, kind) {
				t.Fatal("终态或核对边界被扩大", state, kind)
			}
		}
	}
}
