package importdomain

import "testing"

func Test复用只允许可信终态且不开放检查点(t *testing.T) {
	for _, state := range []string{"WAITING_SCHEDULE", "RUNNING", "FAILED", "CANCELLED", "SUCCEEDED", "UNKNOWN"} {
		for _, kind := range []string{"REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH", "CHECKPOINT_RESUME"} {
			want := state == "FAILED" || state == "CANCELLED" || state == "SUCCEEDED" && kind == "REBUILD_FROM_CONFIG"
			want = want && kind != "CHECKPOINT_RESUME"
			if CanDeriveTaskDraft(state, false, kind) != want || CanDeriveTaskDraft(state, true, kind) {
				t.Fatalf("派生资格错误: %s %s", state, kind)
			}
		}
	}
}

func Test模板绑定拒绝跨模式静默清值与跨平台路径(t *testing.T) {
	config := Config{Database: "synthetic", FilePath: `E:\synthetic`, Content: "DDL_ONLY", Scope: "SPECIFIC", Objects: map[string]string{"sequence": "synthetic_seq"}, Cleanup: "NONE"}
	if _, err := BindReusableConfig(config, "source", "node", "MYSQL", "WINDOWS_AMD64"); err == nil {
		t.Fatal("Oracle 对象被静默删除")
	}
	bound, err := BindReusableConfig(config, "source", "node", "ORACLE", "WINDOWS_AMD64")
	if err != nil || bound.DataSourceID != "source" || bound.NodeID != "node" || bound.Objects["sequence"] == "" {
		t.Fatal("合法绑定未保留显式配置", err)
	}
	if _, err := BindReusableConfig(config, "source", "node", "ORACLE", "LINUX_AMD64"); err == nil {
		t.Fatal("跨平台路径未被拒绝")
	}
}
