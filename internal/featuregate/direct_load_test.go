package featuregate

import "testing"

func Test旁路探针与执行开关分别显式开启(t *testing.T) {
	for _, load := range []func(func(string) (string, bool)) (bool, error){LoadDirectLoadProbeEnabled, LoadDirectLoadExecutionEnabled} {
		for _, raw := range []string{"", "false", "true", "invalid"} {
			enabled, err := load(func(key string) (string, bool) {
				if key == RealExecutionEnvironmentVariable || key == NormalImportEnvironmentVariable {
					return "true", true
				}
				return raw, raw != ""
			})
			if enabled != (raw == "true") || (err != nil) != (raw == "invalid") {
				t.Fatal("旁路开关继承或失败关闭错误")
			}
		}
		if _, err := load(nil); err == nil {
			t.Fatal("环境读取器缺失被接受")
		}
	}
}
