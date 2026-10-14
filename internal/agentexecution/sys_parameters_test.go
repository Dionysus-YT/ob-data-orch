package agentexecution

import (
	"strings"
	"testing"

	"ob-data-orch/internal/agentwire"
)

func TestExecutionSysSlotMustMatchFrozenParameters(t *testing.T) {
	argv := []string{"--add-extra-message", "--sys-user", "synthetic_sys"}
	slot := agentwire.DatabaseConnectionSlot{SysUsername: []byte("synthetic_sys@sys#synthetic_cluster"), SysPassword: []byte("synthetic_sys_password")}
	if !validExecutionSysSlot(argv, slot) {
		t.Fatal("合法 sys 绑定被拒绝")
	}
	for _, bad := range [][]string{{}, {"--add-extra-message"}, {"--sys-user", "synthetic_sys"}, {"--add-extra-message", "--sys-user", "other_sys"}, {"--add-extra-message", "--sys-user", "synthetic_sys", "--sys-user", "synthetic_sys"}} {
		if validExecutionSysSlot(bad, slot) {
			t.Fatal("不匹配或额外的 sys 槽位被接受")
		}
	}
	if validExecutionSysSlot(argv, agentwire.DatabaseConnectionSlot{}) {
		t.Fatal("缺少 sys 材料仍可启动")
	}
	if _, ok := executionPathArguments([]string{"--file-path", "/E:/tmp/synthetic", "--sys-password=synthetic"}); ok {
		t.Fatal("sys 密码进入进程参数")
	}
	policy, err := executionLogPolicy([]byte("synthetic_password"), nil, slot.SysUsername, slot.SysPassword, []byte("synthetic_sys"))
	if err != nil {
		t.Fatal(err)
	}
	defer policy.Destroy()
	result, err := policy.Redact("synthetic_sys synthetic_sys@sys#synthetic_cluster synthetic_sys_password")
	if err != nil || strings.Contains(result, "synthetic_sys") {
		t.Fatal("sys 日志身份或秘密未脱敏")
	}
}
