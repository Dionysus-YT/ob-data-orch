package featuregate

import "testing"

func Test普通导入开关不继承导出且错误值拒绝(t *testing.T) {
	for _, scenario := range []struct {
		raw     string
		exists  bool
		want    bool
		invalid bool
	}{{}, {"true", true, true, false}, {"false", true, false, false}, {"invalid", true, false, true}} {
		got, err := LoadNormalImportExecutionEnabled(func(key string) (string, bool) {
			if key == RealExecutionEnvironmentVariable {
				return "true", true
			}
			return scenario.raw, scenario.exists
		})
		if got != scenario.want || (err != nil) != scenario.invalid {
			t.Fatal("开关边界错误")
		}
	}
}
