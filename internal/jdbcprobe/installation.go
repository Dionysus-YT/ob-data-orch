package jdbcprobe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

const connectorFileName = "oceanbase-client-2.4.14.jar"

var ErrInvalidInstallation = errors.New("JDBC 探针安装目录无效")

// DiscoverRuntime 根据 Agent 本地受控配置生成 JDBC 探针运行时清单。
// Connector/J 位置固定为 OBDUMPER 4.3.5 安装目录的 lib 子目录；该函数不接受远端请求传入的驱动或类路径。
func DiscoverRuntime(javaPath, toolHome string, environment []string) (Runtime, error) {
	if !absoluteRegularFile(javaPath) || !absoluteDirectory(toolHome) || len(environment) == 0 || forbiddenEnvironment(environment) {
		return Runtime{}, ErrInvalidInstallation
	}
	connectorPath := filepath.Join(toolHome, "lib", connectorFileName)
	if !absoluteRegularFile(connectorPath) {
		return Runtime{}, ErrInvalidInstallation
	}
	javaDigest, err := fileDigest(javaPath)
	if err != nil {
		return Runtime{}, ErrInvalidInstallation
	}
	connectorDigest, err := fileDigest(connectorPath)
	if err != nil {
		return Runtime{}, ErrInvalidInstallation
	}
	return Runtime{
		JavaPath:        filepath.Clean(javaPath),
		JavaSHA256:      javaDigest,
		ConnectorPath:   connectorPath,
		ConnectorSHA256: connectorDigest,
		Environment:     append([]string(nil), environment...),
	}, nil
}

func absoluteDirectory(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileDigest(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	defer zero(content)
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}
