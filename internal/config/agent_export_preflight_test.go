package config

import "testing"

func TestLoadAgentExportPreflightEnabled(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		lookup  func(string) (string, bool)
		enabled bool
		wantErr bool
	}{
		{name: "缺失保持关闭", lookup: func(string) (string, bool) { return "", false }},
		{name: "显式开启", lookup: func(string) (string, bool) { return "true", true }, enabled: true},
		{name: "格式无效", lookup: func(string) (string, bool) { return "not-a-bool", true }, wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			enabled, err := LoadAgentExportPreflightEnabled(testCase.lookup)
			if (err != nil) != testCase.wantErr || enabled != testCase.enabled {
				t.Fatalf("LoadAgentExportPreflightEnabled() = %t, %v", enabled, err)
			}
		})
	}
}
