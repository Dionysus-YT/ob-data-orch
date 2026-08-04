package config

import (
	"errors"
	"strconv"
	"strings"
)

const AgentExportPreflightEnvironmentVariable = "OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT"

var ErrAgentExportPreflightConfiguration = errors.New("Agent 导出预检查开关无效")

// LoadAgentExportPreflightEnabled 读取本机对固定 EXPORT_PREFLIGHT 的显式授权开关。
// 缺失或 false 时 Agent 不领取预检查；开启后仍只运行固定 JDBC、对象元数据、工具、路径和空间检查，绝不启动 OBDUMPER。
func LoadAgentExportPreflightEnabled(lookupEnv func(string) (string, bool)) (bool, error) {
	if lookupEnv == nil {
		return false, ErrAgentExportPreflightConfiguration
	}
	raw, exists := lookupEnv(AgentExportPreflightEnvironmentVariable)
	if !exists || strings.TrimSpace(raw) == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, ErrAgentExportPreflightConfiguration
	}
	return enabled, nil
}
