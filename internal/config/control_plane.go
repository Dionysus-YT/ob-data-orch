package config

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ControlPlaneListenEnvironmentVariable = "OB_DATA_ORCH_LISTEN"
	RootKeyPathEnvironmentVariable        = "OB_DATA_ORCH_ROOT_KEY_PATH"
)

type ControlPlane struct {
	ListenAddress string
	RootKeyPath   string
	RootKeyID     string
}

func LoadControlPlane(lookupEnv func(string) (string, bool)) (ControlPlane, error) {
	config := ControlPlane{ListenAddress: "127.0.0.1:8080", RootKeyID: "control-plane-root-v1"}
	if raw, exists := lookupEnv(ControlPlaneListenEnvironmentVariable); exists && strings.TrimSpace(raw) != "" {
		config.ListenAddress = strings.TrimSpace(raw)
	}
	if _, _, err := net.SplitHostPort(config.ListenAddress); err != nil {
		return ControlPlane{}, fmt.Errorf("invalid %s: %w", ControlPlaneListenEnvironmentVariable, err)
	}
	rootKeyPath, err := defaultRootKeyPath(lookupEnv)
	if err != nil {
		return ControlPlane{}, err
	}
	if raw, exists := lookupEnv(RootKeyPathEnvironmentVariable); exists && strings.TrimSpace(raw) != "" {
		rootKeyPath = strings.TrimSpace(raw)
	}
	if !filepath.IsAbs(rootKeyPath) {
		return ControlPlane{}, fmt.Errorf("%s must be an absolute path", RootKeyPathEnvironmentVariable)
	}
	config.RootKeyPath = filepath.Clean(rootKeyPath)
	return config, nil
}

// defaultRootKeyPath keeps the root key out of SQLite and its backup tree.
// Linux follows the confirmed service-account-home convention; Windows uses
// ProgramData unless an explicit absolute path overrides it.
func defaultRootKeyPath(lookupEnv func(string) (string, bool)) (string, error) {
	if runtime.GOOS == "windows" {
		if programData, exists := lookupEnv("ProgramData"); exists && strings.TrimSpace(programData) != "" {
			return filepath.Join(strings.TrimSpace(programData), "OB Data Orch", "security", "root-key.json"), nil
		}
		return "", errors.New("ProgramData is required for the default Windows root key path")
	}
	userHome, exists := lookupEnv("HOME")
	if !exists || strings.TrimSpace(userHome) == "" {
		return "", errors.New("HOME is required for the default Linux root key path")
	}
	return filepath.Join(strings.TrimSpace(userHome), "ob-data-orch", "security", "root-key.json"), nil
}
