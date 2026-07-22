package config

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	AgentJavaPathEnvironmentVariable      = "OB_DATA_ORCH_AGENT_JAVA_PATH"
	AgentToolHomeEnvironmentVariable      = "OB_DATA_ORCH_AGENT_TOOL_HOME"
	AgentWorkspaceRootEnvironmentVariable = "OB_DATA_ORCH_AGENT_WORKSPACE_ROOT"
)

var ErrAgentRuntimeConfiguration = errors.New("Agent 运行时配置无效")

// AgentRuntime 保存 Agent 启动时由本机管理员提供的受控安装路径。
// 这些路径只能来自本地配置，不能由浏览器、控制面请求、任务信封或预检查请求覆盖。
type AgentRuntime struct {
	JavaPath      string
	ToolHome      string
	WorkspaceRoot string
	Environment   []string
}

// LoadAgentRuntime 读取固定 JDBC 探针和 OBDUMPER 启动所需的本机路径配置。
// 路径必须为绝对路径；文件存在性和摘要由 Agent 启动后的运行时发现阶段复验。
func LoadAgentRuntime(lookupEnv func(string) (string, bool)) (AgentRuntime, error) {
	if lookupEnv == nil {
		return AgentRuntime{}, ErrAgentRuntimeConfiguration
	}
	javaPath, ok := requiredAbsolutePath(lookupEnv, AgentJavaPathEnvironmentVariable)
	if !ok {
		return AgentRuntime{}, ErrAgentRuntimeConfiguration
	}
	toolHome, ok := requiredAbsolutePath(lookupEnv, AgentToolHomeEnvironmentVariable)
	if !ok {
		return AgentRuntime{}, ErrAgentRuntimeConfiguration
	}
	workspaceRoot, ok := optionalAbsolutePath(lookupEnv, AgentWorkspaceRootEnvironmentVariable)
	if !ok {
		return AgentRuntime{}, ErrAgentRuntimeConfiguration
	}
	if workspaceRoot == "" {
		var err error
		workspaceRoot, err = defaultAgentWorkspaceRoot(lookupEnv)
		if err != nil {
			return AgentRuntime{}, ErrAgentRuntimeConfiguration
		}
	}
	environment, err := agentJavaEnvironment(lookupEnv, javaPath)
	if err != nil {
		return AgentRuntime{}, ErrAgentRuntimeConfiguration
	}
	return AgentRuntime{JavaPath: javaPath, ToolHome: toolHome, WorkspaceRoot: workspaceRoot, Environment: environment}, nil
}

func requiredAbsolutePath(lookupEnv func(string) (string, bool), key string) (string, bool) {
	path, exists := lookupEnv(key)
	if !exists {
		return "", false
	}
	return normalizeAbsolutePath(path)
}

func optionalAbsolutePath(lookupEnv func(string) (string, bool), key string) (string, bool) {
	path, exists := lookupEnv(key)
	if !exists || strings.TrimSpace(path) == "" {
		return "", true
	}
	return normalizeAbsolutePath(path)
}

func normalizeAbsolutePath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return "", false
	}
	return filepath.Clean(path), true
}

func agentJavaEnvironment(lookupEnv func(string) (string, bool), javaPath string) ([]string, error) {
	if runtime.GOOS == "windows" {
		systemRoot, exists := lookupEnv("SystemRoot")
		if !exists || strings.TrimSpace(systemRoot) == "" || strings.ContainsRune(systemRoot, 0) {
			return nil, ErrAgentRuntimeConfiguration
		}
		windowsRoot := strings.TrimSpace(systemRoot)
		return []string{
			"SystemRoot=" + windowsRoot,
			"WINDIR=" + windowsRoot,
			"PATH=" + filepath.Dir(javaPath) + ";" + filepath.Join(windowsRoot, "System32") + ";" + windowsRoot,
		}, nil
	}
	userHome, exists := lookupEnv("HOME")
	if !exists || strings.TrimSpace(userHome) == "" || strings.ContainsRune(userHome, 0) {
		return nil, ErrAgentRuntimeConfiguration
	}
	return []string{
		"HOME=" + strings.TrimSpace(userHome),
		"LANG=C.UTF-8",
		"PATH=/usr/bin:/bin",
	}, nil
}

func defaultAgentWorkspaceRoot(lookupEnv func(string) (string, bool)) (string, error) {
	if runtime.GOOS == "windows" {
		programData, exists := lookupEnv("ProgramData")
		if !exists || strings.TrimSpace(programData) == "" {
			return "", ErrAgentRuntimeConfiguration
		}
		return filepath.Join(strings.TrimSpace(programData), "OB Data Orch", "agent-workspace"), nil
	}
	userHome, exists := lookupEnv("HOME")
	if !exists || strings.TrimSpace(userHome) == "" {
		return "", ErrAgentRuntimeConfiguration
	}
	return filepath.Join(strings.TrimSpace(userHome), "ob-data-orch", "agent-workspace"), nil
}
