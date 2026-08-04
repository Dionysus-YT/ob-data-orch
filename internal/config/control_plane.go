package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ControlPlaneListenEnvironmentVariable = "OB_DATA_ORCH_LISTEN"
	RootKeyPathEnvironmentVariable        = "OB_DATA_ORCH_ROOT_KEY_PATH"
	TLSCertFileEnvironmentVariable        = "OB_DATA_ORCH_TLS_CERT_FILE"
	TLSKeyFileEnvironmentVariable         = "OB_DATA_ORCH_TLS_KEY_FILE"
)

// ControlPlane 表示控制面启动所需的非秘密配置。
type ControlPlane struct {
	// ListenAddress 是控制面对外监听的网络地址。
	ListenAddress string
	// RootKeyPath 是控制面根密钥在本机的绝对路径。
	RootKeyPath string
	// RootKeyID 是根密钥在密文元数据中的稳定标识。
	RootKeyID string
	// TLSCertFile 是 TLS 证书的绝对常规文件路径；为空时使用 HTTP。
	TLSCertFile string
	// TLSKeyFile 是 TLS 私钥的绝对常规文件路径；为空时使用 HTTP。
	TLSKeyFile string
}

// LoadControlPlane 从环境读取并验证控制面启动配置。
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
	tlsCertFile, tlsKeyFile, err := loadTLSFiles(lookupEnv)
	if err != nil {
		return ControlPlane{}, err
	}
	config.TLSCertFile = tlsCertFile
	config.TLSKeyFile = tlsKeyFile
	return config, nil
}

// loadTLSFiles 只接受成对配置的证书和私钥常规文件，避免启动时退化为不完整 TLS。
func loadTLSFiles(lookupEnv func(string) (string, bool)) (string, string, error) {
	rawCertFile, certConfigured := lookupEnv(TLSCertFileEnvironmentVariable)
	rawKeyFile, keyConfigured := lookupEnv(TLSKeyFileEnvironmentVariable)
	if !certConfigured && !keyConfigured {
		return "", "", nil
	}
	certFile := strings.TrimSpace(rawCertFile)
	keyFile := strings.TrimSpace(rawKeyFile)
	if certFile == "" || keyFile == "" {
		return "", "", fmt.Errorf("%s and %s must be configured together", TLSCertFileEnvironmentVariable, TLSKeyFileEnvironmentVariable)
	}
	cleanCertFile, err := absoluteRegularFile(certFile, TLSCertFileEnvironmentVariable)
	if err != nil {
		return "", "", err
	}
	cleanKeyFile, err := absoluteRegularFile(keyFile, TLSKeyFileEnvironmentVariable)
	if err != nil {
		return "", "", err
	}
	return cleanCertFile, cleanKeyFile, nil
}

// absoluteRegularFile 在不读取敏感内容的前提下验证 TLS 文件路径和类型。
func absoluteRegularFile(path, environmentVariable string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path", environmentVariable)
	}
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("%s must reference an existing regular file", environmentVariable)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s must reference a regular file", environmentVariable)
	}
	return cleanPath, nil
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
