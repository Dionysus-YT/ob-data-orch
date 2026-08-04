package config

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadAgentRuntimeRequiresLocalAbsoluteInstallationPaths(t *testing.T) {
	javaPath, toolHome := agentRuntimePaths()
	config, err := LoadAgentRuntime(func(key string) (string, bool) {
		switch key {
		case AgentJavaPathEnvironmentVariable:
			return javaPath, true
		case AgentToolHomeEnvironmentVariable:
			return toolHome, true
		case "ProgramData":
			return `C:\ProgramData`, true
		case "SystemRoot":
			return `C:\Windows`, true
		case "HADOOP_HOME":
			return agentHadoopHome(), runtime.GOOS == "windows"
		case "HOME":
			return "/home/agent", true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatalf("LoadAgentRuntime() 错误 = %v", err)
	}
	if config.JavaPath != filepath.Clean(javaPath) || config.ToolHome != filepath.Clean(toolHome) || config.WorkspaceRoot == "" || len(config.Environment) == 0 {
		t.Fatalf("Agent 运行时配置不完整: %#v", config)
	}
	if !containsEnvironment(config.Environment, "JAVA_HOME="+filepath.Dir(filepath.Dir(config.JavaPath))) {
		t.Fatalf("子进程环境未传入与 JavaPath 一致的 JAVA_HOME: %#v", config.Environment)
	}
	if runtime.GOOS == "windows" && (!containsEnvironment(config.Environment, "HADOOP_HOME="+agentHadoopHome()) || !strings.Contains(environmentValue(config.Environment, "PATH"), filepath.Join(agentHadoopHome(), "bin"))) {
		t.Fatalf("Windows 子进程环境未传入受控 Hadoop 运行时: %#v", config.Environment)
	}
}

func TestLoadAgentRuntimeRejectsMissingOrRelativePath(t *testing.T) {
	for _, value := range []string{"", "relative/java"} {
		_, err := LoadAgentRuntime(func(key string) (string, bool) {
			switch key {
			case AgentJavaPathEnvironmentVariable:
				return value, true
			case AgentToolHomeEnvironmentVariable:
				return agentRuntimePathsSecond(), true
			case "ProgramData":
				return `C:\ProgramData`, true
			case "SystemRoot":
				return `C:\Windows`, true
			case "HOME":
				return "/home/agent", true
			default:
				return "", false
			}
		})
		if err == nil {
			t.Fatalf("非法 Java 路径 %q 被接受", value)
		}
	}
}

func TestLoadAgentRuntimeWindowsRejectsMissingHadoopHome(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 使用 Hadoop 本地库")
	}
	javaPath, toolHome := agentRuntimePaths()
	_, err := LoadAgentRuntime(func(key string) (string, bool) {
		switch key {
		case AgentJavaPathEnvironmentVariable:
			return javaPath, true
		case AgentToolHomeEnvironmentVariable:
			return toolHome, true
		case "ProgramData":
			return `C:\ProgramData`, true
		case "SystemRoot":
			return `C:\Windows`, true
		case "HOME":
			return "/home/agent", true
		default:
			return "", false
		}
	})
	if err == nil {
		t.Fatal("Windows Agent 接受了缺失 HADOOP_HOME 的运行时配置")
	}
}

func TestLoadAgentJDBCConnectionTestEnabled默认关闭且拒绝非法值(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		exists  bool
		enabled bool
		wantErr bool
	}{
		{name: "缺失", enabled: false},
		{name: "显式关闭", value: "false", exists: true, enabled: false},
		{name: "显式开启", value: "true", exists: true, enabled: true},
		{name: "非法值", value: "enabled", exists: true, wantErr: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			enabled, err := LoadAgentJDBCConnectionTestEnabled(func(key string) (string, bool) {
				if key != AgentJDBCConnectionTestEnvironmentVariable {
					return "", false
				}
				return testCase.value, testCase.exists
			})
			if (err != nil) != testCase.wantErr || enabled != testCase.enabled {
				t.Fatalf("LoadAgentJDBCConnectionTestEnabled() = %t, %v，期望 %t, 错误=%t", enabled, err, testCase.enabled, testCase.wantErr)
			}
		})
	}
}

func agentRuntimePaths() (string, string) {
	if runtime.GOOS == "windows" {
		return `C:\agent\java.exe`, `C:\agent\ob-loader-dumper`
	}
	return "/opt/agent/java", "/opt/agent/ob-loader-dumper"
}

func agentRuntimePathsSecond() string {
	_, toolHome := agentRuntimePaths()
	return toolHome
}

func agentHadoopHome() string {
	if runtime.GOOS == "windows" {
		return `C:\agent\hadoop`
	}
	return ""
}

func containsEnvironment(environment []string, expected string) bool {
	for _, value := range environment {
		if value == expected {
			return true
		}
	}
	return false
}

func environmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, value := range environment {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}
