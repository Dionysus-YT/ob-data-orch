package agentexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ob-data-orch/internal/credential"
)

// DirectLoadToolSHA256 对应已静态核验提交语义的官方 4.3.5 JAR；未知包不能沿用该解析器结论。
const DirectLoadToolSHA256 = "b43f88caafd2c62462cfdeff38a0fd7088b0195c5d064074f977bc0b363fec06"

// windowsDirectLoadReplicaArguments 复用官方固定属性，但让已核验主 JAR 优先提供旁路类。
// 去掉当前目录，防止工具工作目录内同名 class 或其他依赖 JAR 抢先覆盖已核验的提交实现。
func windowsDirectLoadReplicaArguments(workspace credential.Workspace, toolHome, security string, update int) ([]string, error) {
	if !workspace.OwnsSecurityConfiguration(security) {
		return nil, ErrDirectJavaLaunchInvalid
	}
	profile, err := windowsOBLoaderReplicaArguments(workspace, toolHome, security, update)
	if err != nil {
		return nil, err
	}
	configuration, err := directLoadLogConfiguration(workspace)
	if err != nil {
		return nil, err
	}
	found := false
	for i, option := range profile {
		if strings.HasPrefix(option, "-Dlog4j.configurationFile=") {
			profile[i] = "-Dlog4j.configurationFile=file:///" + strings.TrimPrefix(filepath.ToSlash(configuration), "/")
		}
		if option == "-classpath" && i+1 < len(profile) {
			// SOFA 只接受类路径资源名；最后加入受控 runtime 以读取固定 XML，不提供工作目录。
			profile[i+1] = filepath.Join(toolHome, "lib", "ob-loader-dumper-4.3.5-RELEASE.jar") + ";" + filepath.Join(toolHome, "lib", "*") + ";" + workspace.RuntimeDirectory()
			found = true
		}
	}
	if found {
		// 工具会覆盖全局 logging.path；Bolt 使用官方 SOFA 配置入口改为同一固定 stdout 配置。
		// 其他 SDK 的独立路径仍固定到私有目录，不使用用户文件目录。
		return append([]string{"-Dlogging.path=" + workspace.RawLogDirectory(), "-Dlogging.path.oceanbase-table-client=" + workspace.RawLogDirectory(), "-Dlogging.config.com.alipay.remoting=direct-load-log4j2.xml"}, profile...), nil
	}
	return nil, ErrDirectJavaLaunchInvalid
}

// directLoadLogConfiguration 固定仅输出到受控 stdout；官方 Routing 的 task.workspace 会把日志写回输入目录。
// 配置不接收任务内容或外部路径，已有运行时资产必须逐字节相同且不能为链接。
func directLoadLogConfiguration(workspace credential.Workspace) (string, error) {
	const content = `<?xml version="1.0" encoding="UTF-8"?><Configuration status="ERROR"><Appenders><Console name="ControlledOutput" target="SYSTEM_OUT"><PatternLayout pattern="%d{yyyy-MM-dd HH:mm:ss} [%p] %m%n"/></Console></Appenders><Loggers><Root level="info"><AppenderRef ref="ControlledOutput"/></Root></Loggers></Configuration>`
	path := filepath.Join(workspace.RuntimeDirectory(), "direct-load-log4j2.xml")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != int64(len(content)) {
			return "", ErrDirectJavaLaunchInvalid
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(body, []byte(content)) {
			return "", ErrDirectJavaLaunchInvalid
		}
		return path, nil
	}
	if err != nil {
		return "", ErrDirectJavaLaunchInvalid
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return "", ErrDirectJavaLaunchInvalid
	}
	return path, nil
}

// DirectLoadToolVersion 只读固化工具目录中的固定 JAR，不运行工具或依赖文件名猜测版本。
// 只接受已核验摘要；文件替换、链接、读取取消与预算超限均保持版本未知。
func DirectLoadToolVersion(ctx context.Context, toolHome string) (string, error) {
	digest, err := directLoadToolDigest(ctx, toolHome)
	if err != nil {
		return "", err
	}
	if digest != DirectLoadToolSHA256 {
		return "", ErrVersionOutputInvalid
	}
	return "4.3.5-RELEASE", nil
}

// directLoadToolDigest 有界读取固定路径，分别核对打开句柄及路径身份，拒绝祖先链接和并发替换。
func directLoadToolDigest(ctx context.Context, toolHome string) (string, error) {
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(toolHome) {
		return "", ErrVersionProbeUnavailable
	}
	path := filepath.Join(toolHome, "lib", "ob-loader-dumper-4.3.5-RELEASE.jar")
	for current := filepath.Dir(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", ErrVersionProbeUnavailable
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > 64<<20 {
		return "", ErrVersionProbeUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return "", ErrVersionProbeUnavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		return "", ErrVersionOutputInvalid
	}
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	readBytes := int64(0)
	for {
		if ctx.Err() != nil {
			return "", ErrVersionProbeUnavailable
		}
		n, err := file.Read(buffer)
		readBytes += int64(n)
		if readBytes > 64<<20 {
			return "", ErrVersionOutputInvalid
		}
		_, _ = hash.Write(buffer[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", ErrVersionProbeUnavailable
		}
	}
	after, err := os.Lstat(path)
	if err != nil || ctx.Err() != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) || before.Size() != after.Size() || before.Size() != readBytes || !before.ModTime().Equal(after.ModTime()) {
		return "", ErrVersionOutputInvalid
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
