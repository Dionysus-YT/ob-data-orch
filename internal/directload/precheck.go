package directload

import (
	"regexp"
	"strconv"
	"strings"
)

// TableFacts 只接受固定探针的可信事实；nil 表示未知，不可由客户端配置补齐。
type TableFacts struct {
	Exists              *bool `json:"exists"`
	Empty               *bool `json:"empty"`
	HasBIT              *bool `json:"hasBIT"`
	HasBinary           *bool `json:"hasBinary"`
	HasVirtualGenerated *bool `json:"hasVirtualGenerated"`
	HasIndex            *bool `json:"hasIndex"`
	HasLOB              *bool `json:"hasLOB"`
	HasUniqueIndex      *bool `json:"hasUniqueIndex"`
}

// TableAssessment 区分阻断和警告，模式始终由目标表事实派生。
type TableAssessment struct {
	Mode     string  `json:"mode"`
	Issues   []Issue `json:"issues"`
	Warnings []Issue `json:"warnings"`
}

// AssessTable 对缺失、受限结构和替换条件失败关闭；唯一索引不被表达为可由 replace-data 解决。
func AssessTable(c Config, f TableFacts) TableAssessment {
	a := TableAssessment{Mode: "UNKNOWN", Issues: []Issue{}, Warnings: []Issue{}}
	for _, v := range []struct {
		name  string
		value *bool
	}{{"table", f.Exists}, {"empty", f.Empty}, {"bit", f.HasBIT}, {"binary", f.HasBinary}, {"virtualGenerated", f.HasVirtualGenerated}, {"index", f.HasIndex}, {"lob", f.HasLOB}, {"uniqueIndex", f.HasUniqueIndex}} {
		if v.value == nil {
			a.Issues = append(a.Issues, Issue{v.name, "TABLE_FACT_UNAVAILABLE"})
		}
	}
	if f.Exists != nil && !*f.Exists {
		a.Issues = append(a.Issues, Issue{"table", "TABLE_NOT_FOUND"})
	}
	if f.Exists != nil && *f.Exists && f.Empty != nil {
		if *f.Empty {
			a.Mode = "FULL"
		} else {
			a.Mode = "INCREMENTAL"
		}
	}
	for _, v := range []struct {
		name  string
		value *bool
	}{{"bit", f.HasBIT}, {"binary", f.HasBinary}, {"virtualGenerated", f.HasVirtualGenerated}} {
		if v.value != nil && *v.value {
			a.Issues = append(a.Issues, Issue{v.name, "STRUCTURE_UNSUPPORTED"})
		}
	}
	if c.ReplaceData && a.Mode == "INCREMENTAL" && (truth(f.HasIndex) || truth(f.HasLOB)) {
		a.Issues = append(a.Issues, Issue{"replaceData", "INCREMENTAL_REPLACE_UNSUPPORTED"})
	}
	if truth(f.HasUniqueIndex) {
		a.Warnings = append(a.Warnings, Issue{"uniqueIndex", "UNIQUE_CONFLICT_NOT_RESOLVED"})
	}
	if c.Thread != "" && c.Parallel != "" {
		thread, _ := strconv.ParseInt(c.Thread, 10, 64)
		parallel, _ := strconv.ParseInt(c.Parallel, 10, 64)
		if thread != parallel {
			a.Warnings = append(a.Warnings, Issue{"parallel", "CONCURRENCY_DIFFERS"})
		}
	}
	return a
}

// ValidateVersions 使用官方矩阵及已取得完整结果证据的固定组合，不把未验证的新版本外推为支持。
func ValidateVersions(scenario, toolVersion, observerVersion, odpVersion, compatibilityMode string) []Issue {
	var issues []Issue
	if toolVersion == "" {
		issues = append(issues, Issue{"toolVersion", "VERSION_UNAVAILABLE"})
	} else if toolVersion != ToolVersion {
		issues = append(issues, Issue{"toolVersion", "VERSION_UNSUPPORTED"})
	}
	observer, ok := parseVersion(observerVersion)
	if !ok {
		issues = append(issues, Issue{"observerVersion", "VERSION_UNAVAILABLE"})
	} else {
		allowed := false
		switch scenario {
		case "PRIVATE_ODP":
			allowed = atLeast(observer, [4]int{4, 2, 1, 0})
		case "OBSERVER_DIRECT":
			allowed = observer[0] == 4 && observer[1] == 2 && observer[2] == 1 && observer[3] >= 7 || atLeast(observer, [4]int{4, 2, 4, 0}) && less(observer, [4]int{4, 3, 0, 0}) || observer[0] == 4 && observer[1] == 3 && atLeast(observer, [4]int{4, 3, 0, 1})
			// 2026-10-22 固定 4.3.5 主包在两种兼容模式的 4.4.2.3 直连均已取得同表提交与独立目标结果。
			allowed = allowed || (compatibilityMode == "MYSQL" || compatibilityMode == "ORACLE") && observer == [4]int{4, 4, 2, 3}
		default:
			issues = append(issues, Issue{"scenario", "UNSUPPORTED_SCENARIO"})
		}
		if !allowed {
			code := "VERSION_UNSUPPORTED"
			if scenario == "OBSERVER_DIRECT" && atLeast(observer, [4]int{4, 4, 0, 0}) {
				code = "VERSION_UNVERIFIED"
			}
			issues = append(issues, Issue{"observerVersion", code})
		}
	}
	if scenario == "PRIVATE_ODP" {
		odp, ok := parseVersion(odpVersion)
		if !ok {
			issues = append(issues, Issue{"odpVersion", "VERSION_UNAVAILABLE"})
		} else if !atLeast(odp, [4]int{4, 3, 0, 0}) {
			issues = append(issues, Issue{"odpVersion", "VERSION_UNSUPPORTED"})
		}
	}
	return issues
}

var versionPattern = regexp.MustCompile(`^[vV]?[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?$`)

func parseVersion(value string) ([4]int, bool) {
	var version [4]int
	if !versionPattern.MatchString(value) {
		return version, false
	}
	for i, part := range strings.Split(strings.TrimLeft(value, "vV"), ".") {
		n, err := strconv.ParseInt(part, 10, 32)
		if err != nil {
			return version, false
		}
		version[i] = int(n)
	}
	return version, true
}
func atLeast(a, b [4]int) bool { return !less(a, b) }
func less(a, b [4]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
func truth(value *bool) bool { return value != nil && *value }
