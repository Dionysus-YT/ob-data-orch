package config

import (
	"errors"
	"strconv"
	"strings"
)

const AgentJDBCConnectionTestEnvironmentVariable = "OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST"

var ErrAgentJDBCConnectionTestConfiguration = errors.New("Agent JDBC 连接测试开关无效")

// LoadAgentJDBCConnectionTestEnabled 读取本机对固定 JDBC 连接测试的显式授权开关。
// 缺失或 false 都保持关闭；该开关不开放 OBDUMPER、任意命令或其他真实执行能力。
func LoadAgentJDBCConnectionTestEnabled(lookupEnv func(string) (string, bool)) (bool, error) {
	if lookupEnv == nil {
		return false, ErrAgentJDBCConnectionTestConfiguration
	}
	raw, exists := lookupEnv(AgentJDBCConnectionTestEnvironmentVariable)
	if !exists || strings.TrimSpace(raw) == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, ErrAgentJDBCConnectionTestConfiguration
	}
	return enabled, nil
}
