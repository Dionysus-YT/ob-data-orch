package logstream

import (
	"strings"
	"testing"
)

func TestSysIdentityIsProtectedWithoutHidingBusinessIdentity(t *testing.T) {
	policy := Policy{Version: "synthetic-sys-v1"}
	input := "--sys-user synthetic_sys sys_user=other_sys SYS-USER:'quoted_sys' synthetic_sys@sys#synthetic_cluster business_user@tenant#cluster business_user@sysmetrics#cluster"
	output, err := policy.Redact(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"synthetic_sys", "other_sys", "quoted_sys", "synthetic_cluster"} {
		if strings.Contains(output, private) {
			t.Fatal("二次脱敏泄露 sys 身份")
		}
	}
	if !strings.Contains(output, "business_user@tenant#cluster") || !strings.Contains(output, "business_user@sysmetrics#cluster") {
		t.Fatal("普通诊断身份被错误遮蔽")
	}
	again, err := policy.Redact(output)
	if err != nil || again != output {
		t.Fatal("sys 脱敏不幂等")
	}
}
