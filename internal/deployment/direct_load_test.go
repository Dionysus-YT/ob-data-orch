package deployment

import "testing"

func Test旁路安装配置默认关闭且要求完整独立开关(t *testing.T) {
	base := testSettings(t)
	if base.DirectLoadProbeEnabled || base.DirectLoadExecutionEnabled {
		t.Fatal("首次安装默认开启旁路")
	}
	for _, scenario := range []struct{ real, probe, execute, valid bool }{{false, false, false, true}, {true, false, false, true}, {false, true, false, false}, {false, false, true, false}, {true, false, true, false}, {true, true, false, true}, {true, true, true, true}} {
		c := base
		c.RealExecutionEnabled, c.DirectLoadProbeEnabled, c.DirectLoadExecutionEnabled = scenario.real, scenario.probe, scenario.execute
		if (validateSettings(c) == nil) != scenario.valid {
			t.Fatal("旁路安装开关未失败关闭")
		}
	}
}
