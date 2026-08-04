package config

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLoadAgentControlPlaneAcceptsStrictHTTPSConfiguration(t *testing.T) {
	stateDirectory, caFile := agentWirePaths()
	config, err := LoadAgentControlPlane(func(key string) (string, bool) {
		switch key {
		case AgentControlPlaneURLEnvironmentVariable:
			return "https://control.example.test/ob-data-orch", true
		case AgentControlPlaneCAFileEnvironmentVariable:
			return caFile, true
		case AgentStateDirectoryEnvironmentVariable:
			return stateDirectory, true
		case AgentHeartbeatIntervalEnvironmentVariable:
			return "45s", true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatalf("LoadAgentControlPlane() error = %v", err)
	}
	if config.URL != "https://control.example.test/ob-data-orch" || config.CAFile != filepath.Clean(caFile) || config.StateDirectory != filepath.Clean(stateDirectory) || config.HeartbeatInterval != 45*time.Second {
		t.Fatalf("Agent 通信配置不符合预期: %#v", config)
	}
}

func TestLoadAgentControlPlaneRejectsUnsafeEndpoint(t *testing.T) {
	stateDirectory, _ := agentWirePaths()
	for _, endpoint := range []string{
		"http://control.example.test",
		"https://user@control.example.test",
		"https://control.example.test/?credential=value",
		"https://control.example.test/#fragment",
		"https:///missing-host",
	} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := LoadAgentControlPlane(func(key string) (string, bool) {
				switch key {
				case AgentControlPlaneURLEnvironmentVariable:
					return endpoint, true
				case AgentStateDirectoryEnvironmentVariable:
					return stateDirectory, true
				default:
					return "", false
				}
			})
			if err == nil {
				t.Fatal("不安全控制面端点被接受")
			}
		})
	}
}

func TestLoadAgentStateDirectoryUsesPlatformPrivateDefault(t *testing.T) {
	directory, err := LoadAgentStateDirectory(func(key string) (string, bool) {
		switch key {
		case "ProgramData":
			return `C:\ProgramData`, true
		case "HOME":
			return "/home/agent", true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatalf("LoadAgentStateDirectory() error = %v", err)
	}
	if runtime.GOOS == "windows" {
		if directory != filepath.Join(`C:\ProgramData`, "OB Data Orch", "agent-security") {
			t.Fatalf("Windows 默认状态目录 = %q", directory)
		}
		return
	}
	if directory != "/home/agent/ob-data-orch/security/agent" {
		t.Fatalf("Linux 默认状态目录 = %q", directory)
	}
}

func TestLoadAgentControlPlaneRejectsUnsafeHeartbeatInterval(t *testing.T) {
	stateDirectory, _ := agentWirePaths()
	for _, interval := range []string{"4s", "301s", "bad"} {
		t.Run(interval, func(t *testing.T) {
			_, err := LoadAgentControlPlane(func(key string) (string, bool) {
				switch key {
				case AgentControlPlaneURLEnvironmentVariable:
					return "https://control.example.test", true
				case AgentStateDirectoryEnvironmentVariable:
					return stateDirectory, true
				case AgentHeartbeatIntervalEnvironmentVariable:
					return interval, true
				default:
					return "", false
				}
			})
			if err == nil {
				t.Fatal("非法心跳间隔被接受")
			}
		})
	}
}

func agentWirePaths() (string, string) {
	if runtime.GOOS == "windows" {
		return `C:\agent\security`, `C:\agent\ca.pem`
	}
	return "/var/lib/ob-data-orch/agent-security", "/etc/ob-data-orch/ca.pem"
}
