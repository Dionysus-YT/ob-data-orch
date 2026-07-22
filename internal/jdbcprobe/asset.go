package jdbcprobe

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"ob-data-orch/internal/credential"
)

const probeArtifactName = "ob-data-orch-jdbc-probe.jar"

// embeddedProbe 是随 Go Agent 一起发布的固定 JDBC 探针。
// 它仅由受控构建脚本生成，Agent 不从网络、当前目录或用户路径加载替代 JAR。
//
//go:embed assets/ob-data-orch-jdbc-probe.jar
var embeddedProbe []byte

// Install 将内嵌探针写入一次预检查专属的运行时目录并返回已核验的绝对路径与摘要。
// 工作区重复或资产被替换时失败关闭，避免不同任务共享可变探针文件。
func Install(workspace credential.Workspace) (string, string, error) {
	directory := workspace.RuntimeDirectory()
	if directory == "" {
		return "", "", ErrInvalidRuntime
	}
	path := filepath.Join(directory, probeArtifactName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", "", ErrProbeFailed
	}
	defer file.Close()
	if _, err := file.Write(embeddedProbe); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", "", ErrProbeFailed
	}
	if err := file.Sync(); err != nil {
		return "", "", ErrProbeFailed
	}
	digest := sha256.Sum256(embeddedProbe)
	return path, hex.EncodeToString(digest[:]), nil
}

// EmbeddedDigest 返回内嵌探针的 SHA-256，供 Agent 运行时与释放文件交叉核验。
func EmbeddedDigest() (string, error) {
	if len(embeddedProbe) == 0 {
		return "", errors.New("内嵌 JDBC 探针为空")
	}
	digest := sha256.Sum256(embeddedProbe)
	return hex.EncodeToString(digest[:]), nil
}
