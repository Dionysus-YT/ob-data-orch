package featuregate

import (
	"fmt"
	"strconv"
	"strings"
)

const RealExecutionEnvironmentVariable = "OB_DATA_ORCH_ENABLE_REAL_EXECUTION"

// RequireRealExecutionDisabled makes the G1 boundary executable rather than
// relying on documentation alone. Real execution has no implementation in
// DEV-01 and any attempt to enable it must stop the process.
func RequireRealExecutionDisabled(lookupEnv func(string) (string, bool)) error {
	raw, exists := lookupEnv(RealExecutionEnvironmentVariable)
	if !exists || strings.TrimSpace(raw) == "" {
		return nil
	}

	enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s must be a boolean: %w", RealExecutionEnvironmentVariable, err)
	}
	if enabled {
		return fmt.Errorf("%s cannot be enabled at development gate G1", RealExecutionEnvironmentVariable)
	}
	return nil
}
