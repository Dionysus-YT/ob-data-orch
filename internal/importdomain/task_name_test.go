package importdomain

import (
	"strings"
	"testing"
	"time"
)

func Test名称只在提交时填写或按UTC规则自动生成(t *testing.T) {
	now := time.Date(2026, 10, 16, 4, 5, 6, 0, time.UTC)
	name, err := TaskName("", "synthetic_db", "CSV", "12345678-1234", now)
	if err != nil || name != "普通导入-synthetic_db-CSV-20261016T040506Z-12345678" {
		t.Fatal("自动名称规则不符合预期")
	}
	name, err = TaskName("  合成任务  ", "synthetic_db", "CSV", "id", now)
	if err != nil || name != "合成任务" {
		t.Fatal("自定义名称未保留")
	}
	for _, input := range []string{"synthetic\nname", strings.Repeat("名", 201)} {
		if _, err := TaskName(input, "synthetic_db", "CSV", "id", now); err == nil {
			t.Fatal("非法名称未拒绝")
		}
	}
	c := configured()
	c.TaskName = ""
	if _, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true); len(issues) != 0 {
		t.Fatal("未填写任务名仍阻断命令或预检查")
	}
}
