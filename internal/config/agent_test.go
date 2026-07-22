package config

import (
	"path/filepath"
	"runtime"
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
