package featuregate

import (
	"fmt"
	"strconv"
	"strings"
)

const RealExecutionEnvironmentVariable = "OB_DATA_ORCH_ENABLE_REAL_EXECUTION"

// LoadRealExecutionEnabled 读取真实执行的显式本机开关。
// 调用方还必须结合部署模式、平台和运行时证据施加更窄的准入条件，不能把此开关本身当作授权。
func LoadRealExecutionEnabled(lookupEnv func(string) (string, bool)) (bool, error) {
	if lookupEnv == nil {
		return false, fmt.Errorf("%s environment lookup is unavailable", RealExecutionEnvironmentVariable)
	}
	raw, exists := lookupEnv(RealExecutionEnvironmentVariable)
	if !exists || strings.TrimSpace(raw) == "" {
		return false, nil
	}

	enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", RealExecutionEnvironmentVariable, err)
	}
	return enabled, nil
}

// RequireRealExecutionDisabled 保留给仍处于 G1/G2 的启动入口和测试夹具。
// Windows 本机 MVP 的受控 G3 入口改由 LoadRealExecutionEnabled 配合额外约束处理。
func RequireRealExecutionDisabled(lookupEnv func(string) (string, bool)) error {
	enabled, err := LoadRealExecutionEnabled(lookupEnv)
	if err != nil {
		return err
	}
	if enabled {
		return fmt.Errorf("%s cannot be enabled at development gate G1", RealExecutionEnvironmentVariable)
	}
	return nil
}
