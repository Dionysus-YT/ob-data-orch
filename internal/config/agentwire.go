package config

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	AgentControlPlaneURLEnvironmentVariable    = "OB_DATA_ORCH_AGENT_CONTROL_PLANE_URL"
	AgentControlPlaneCAFileEnvironmentVariable = "OB_DATA_ORCH_AGENT_CONTROL_PLANE_CA_FILE"
	AgentStateDirectoryEnvironmentVariable     = "OB_DATA_ORCH_AGENT_STATE_DIRECTORY"
	AgentHeartbeatIntervalEnvironmentVariable  = "OB_DATA_ORCH_AGENT_HEARTBEAT_INTERVAL"
	defaultAgentHeartbeatInterval              = 30 * time.Second
)

var ErrAgentWireConfiguration = errors.New("Agent 通信配置无效")

// AgentControlPlane 保存首次关联时必须由本机管理员提供的控制面配置。
// 关联完成后 URL 与 CA 文件路径会被写入受保护身份状态，后续心跳不信任可被远端影响的输入。
type AgentControlPlane struct {
	URL               string
	CAFile            string
	StateDirectory    string
	HeartbeatInterval time.Duration
}

// LoadAgentControlPlane 读取首次关联所需的本机控制面配置。
// 仅接受 HTTPS 端点和本地绝对 CA 路径，禁止通过查询、片段或用户信息混入认证材料。
func LoadAgentControlPlane(lookupEnv func(string) (string, bool)) (AgentControlPlane, error) {
	if lookupEnv == nil {
		return AgentControlPlane{}, ErrAgentWireConfiguration
	}
	endpoint, ok := lookupEnv(AgentControlPlaneURLEnvironmentVariable)
	if !ok {
		return AgentControlPlane{}, ErrAgentWireConfiguration
	}
	normalizedEndpoint, ok := normalizeAgentControlPlaneURL(endpoint)
	if !ok {
		return AgentControlPlane{}, ErrAgentWireConfiguration
	}
	stateDirectory, err := LoadAgentStateDirectory(lookupEnv)
	if err != nil {
		return AgentControlPlane{}, err
	}
	caFile, ok := optionalAbsolutePath(lookupEnv, AgentControlPlaneCAFileEnvironmentVariable)
	if !ok {
		return AgentControlPlane{}, ErrAgentWireConfiguration
	}
	interval, err := LoadAgentHeartbeatInterval(lookupEnv)
	if err != nil {
		return AgentControlPlane{}, ErrAgentWireConfiguration
	}
	return AgentControlPlane{
		URL:               normalizedEndpoint,
		CAFile:            caFile,
		StateDirectory:    stateDirectory,
		HeartbeatInterval: interval,
	}, nil
}

// LoadAgentStateDirectory 解析 Agent 机器身份的受保护本地状态目录。
// 该目录独立于任务工作目录，避免清理某次任务时误删除机器身份。
func LoadAgentStateDirectory(lookupEnv func(string) (string, bool)) (string, error) {
	if lookupEnv == nil {
		return "", ErrAgentWireConfiguration
	}
	if configured, ok := optionalAbsolutePath(lookupEnv, AgentStateDirectoryEnvironmentVariable); !ok {
		return "", ErrAgentWireConfiguration
	} else if configured != "" {
		return configured, nil
	}
	if runtime.GOOS == "windows" {
		programData, exists := lookupEnv("ProgramData")
		if !exists || strings.TrimSpace(programData) == "" || strings.ContainsRune(programData, 0) {
			return "", ErrAgentWireConfiguration
		}
		result, ok := normalizeAbsolutePath(filepath.Join(strings.TrimSpace(programData), "OB Data Orch", "agent-security"))
		if !ok {
			return "", ErrAgentWireConfiguration
		}
		return result, nil
	}
	home, exists := lookupEnv("HOME")
	if !exists || strings.TrimSpace(home) == "" || strings.ContainsRune(home, 0) {
		return "", ErrAgentWireConfiguration
	}
	result, ok := normalizeAbsolutePath(filepath.Join(strings.TrimSpace(home), "ob-data-orch", "security", "agent"))
	if !ok {
		return "", ErrAgentWireConfiguration
	}
	return result, nil
}

func normalizeAgentControlPlaneURL(value string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if strings.ContainsRune(parsed.Host, 0) || strings.ContainsRune(parsed.Path, 0) {
		return "", false
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed.String(), true
}

// LoadAgentHeartbeatInterval 读取 Agent 的本机心跳周期。
// 周期不从控制面响应取得，避免远端输入改变本机网络行为或形成高频请求。
func LoadAgentHeartbeatInterval(lookupEnv func(string) (string, bool)) (time.Duration, error) {
	raw, exists := lookupEnv(AgentHeartbeatIntervalEnvironmentVariable)
	if !exists || strings.TrimSpace(raw) == "" {
		return defaultAgentHeartbeatInterval, nil
	}
	interval, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || interval < 5*time.Second || interval > 5*time.Minute {
		return 0, ErrAgentWireConfiguration
	}
	return interval, nil
}
