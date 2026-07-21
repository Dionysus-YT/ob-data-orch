package config

import (
	"fmt"
	"net"
	"strings"
)

const ControlPlaneListenEnvironmentVariable = "OB_DATA_ORCH_LISTEN"

type ControlPlane struct {
	ListenAddress string
}

func LoadControlPlane(lookupEnv func(string) (string, bool)) (ControlPlane, error) {
	config := ControlPlane{ListenAddress: "127.0.0.1:8080"}
	if raw, exists := lookupEnv(ControlPlaneListenEnvironmentVariable); exists && strings.TrimSpace(raw) != "" {
		config.ListenAddress = strings.TrimSpace(raw)
	}
	if _, _, err := net.SplitHostPort(config.ListenAddress); err != nil {
		return ControlPlane{}, fmt.Errorf("invalid %s: %w", ControlPlaneListenEnvironmentVariable, err)
	}
	return config, nil
}
