package directload

import "testing"

func Test精确版本边界及未知失败关闭(t *testing.T) {
	for _, item := range []struct {
		scenario, observer, odp string
		allowed                 bool
	}{
		{"OBSERVER_DIRECT", "4.2.1.6", "", false}, {"OBSERVER_DIRECT", "4.2.1.7", "", true}, {"OBSERVER_DIRECT", "4.2.2.0", "", false}, {"OBSERVER_DIRECT", "4.2.3.9", "", false}, {"OBSERVER_DIRECT", "4.2.4.0", "", true}, {"OBSERVER_DIRECT", "4.3.0.0", "", false}, {"OBSERVER_DIRECT", "4.3.0.1", "", true}, {"OBSERVER_DIRECT", "4.3.5.0", "", true}, {"OBSERVER_DIRECT", "4.4.0.0", "", false},
		{"PRIVATE_ODP", "4.2.0.9", "4.3.0", false}, {"PRIVATE_ODP", "4.2.1.0", "4.2.9", false}, {"PRIVATE_ODP", "4.2.1.0", "4.3.0", true}, {"CLOUD_ODP", "4.3.5", "4.3.1", false}, {"CLOUD_ODP", "", "4.3.0", false}, {"PRIVATE_ODP", "4.3.5", "", false}, {"OBSERVER_DIRECT", "4.3", "", false}, {"OBSERVER_DIRECT", "4.3.5-synthetic", "", false},
	} {
		if got := len(ValidateVersions(item.scenario, ToolVersion, item.observer, item.odp, "MYSQL")) == 0; got != item.allowed {
			t.Fatalf("版本矩阵不符：%+v", item)
		}
	}
	if len(ValidateVersions("PRIVATE_ODP", "4.3.6-RELEASE", "4.3.5", "4.3.0", "MYSQL")) == 0 {
		t.Fatal("非固定工具版本被接受")
	}
}

func Test实证直连版本仅限固定双模式组合(t *testing.T) {
	for _, mode := range []string{"MYSQL", "ORACLE"} {
		if issues := ValidateVersions("OBSERVER_DIRECT", ToolVersion, "4.4.2.3", "", mode); len(issues) != 0 {
			t.Fatal("实证组合未放行", issues)
		}
	}
	for _, item := range []struct{ version, mode string }{{"4.4.2.3", "UNKNOWN"}, {"4.4.2.4", "ORACLE"}, {"4.4.2.4", "MYSQL"}, {"4.4.3", "MYSQL"}, {"5.0.0", "MYSQL"}} {
		issues := ValidateVersions("OBSERVER_DIRECT", ToolVersion, item.version, "", item.mode)
		if len(issues) != 1 || issues[0].Code != "VERSION_UNVERIFIED" {
			t.Fatal("未验证组合被外推或误判不支持", item)
		}
	}
}

func knownTable() TableFacts {
	yes, no := true, false
	return TableFacts{Exists: &yes, Empty: &yes, HasBIT: &no, HasBinary: &no, HasVirtualGenerated: &no, HasIndex: &no, HasLOB: &no, HasUniqueIndex: &no}
}

func Test表结构空状态替换与未知事实(t *testing.T) {
	c := configured()
	assessment := AssessTable(c, knownTable())
	if assessment.Mode != "FULL" || len(assessment.Issues) != 0 {
		t.Fatal("合法全量表被拒绝", assessment)
	}
	if a := AssessTable(c, TableFacts{}); a.Mode != "UNKNOWN" || len(a.Issues) != 8 {
		t.Fatal("未知被当作通过", a)
	}
	yes, no := true, false
	for _, change := range []func(*TableFacts){func(f *TableFacts) { f.HasBIT = &yes }, func(f *TableFacts) { f.HasBinary = &yes }, func(f *TableFacts) { f.HasVirtualGenerated = &yes }, func(f *TableFacts) { f.Exists = &no }, func(f *TableFacts) { f.HasLOB = nil }} {
		f := knownTable()
		change(&f)
		if len(AssessTable(c, f).Issues) == 0 {
			t.Fatal("结构或缺失事实未阻断")
		}
	}
	c.ReplaceData = true
	f := knownTable()
	f.Empty = &no
	f.HasIndex = &yes
	if len(AssessTable(c, f).Issues) == 0 {
		t.Fatal("增量索引替换被接受")
	}
	f.HasIndex = &no
	f.HasLOB = &yes
	if len(AssessTable(c, f).Issues) == 0 {
		t.Fatal("增量LOB替换被接受")
	}
	f.HasLOB = &no
	if a := AssessTable(c, f); len(a.Issues) != 0 || a.Mode != "INCREMENTAL" {
		t.Fatal("无索引LOB增量替换被拒绝")
	}
	f.HasUniqueIndex = &yes
	c.Thread = "1"
	c.Parallel = "2"
	a := AssessTable(c, f)
	if len(a.Issues) != 0 || len(a.Warnings) != 2 {
		t.Fatal("唯一索引与并发未正确警告", a)
	}
	// 清空意图不改写探针读到的表空事实，不能用未来写入行为绕过增量限制。
	c.TruncateTable = true
	f.HasIndex = &yes
	if a := AssessTable(c, f); a.Mode != "INCREMENTAL" || len(a.Issues) == 0 {
		t.Fatal("清空意图覆盖了真实表事实")
	}
}
