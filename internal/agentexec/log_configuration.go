package agentexec

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"ob-data-orch/internal/credential"
)

// controlledRuntimeDirectory 只接受当前工作区已建立的真实目录，不跟随替换链接或使用公共临时目录。
func controlledRuntimeDirectory(workspace credential.Workspace) (string, error) {
	directory := workspace.RuntimeDirectory()
	info, err := os.Lstat(directory)
	if !filepath.IsAbs(directory) || err != nil || !info.IsDir() {
		return "", ErrDirectJavaLaunchInvalid
	}
	return directory, nil
}

// controlledLogConfiguration 复用导出默认日志和旁路的固定管道出口，不修改共享工具配置。
func controlledLogConfiguration(workspace credential.Workspace, name string) (string, error) {
	if name != "export-log4j2.xml" && name != "direct-load-log4j2.xml" {
		return "", ErrDirectJavaLaunchInvalid
	}
	directory, err := controlledRuntimeDirectory(workspace)
	if err != nil {
		return "", err
	}
	const content = `<?xml version="1.0" encoding="UTF-8"?><Configuration status="ERROR"><Appenders><Console name="ControlledOutput" target="SYSTEM_OUT"><PatternLayout pattern="%d{yyyy-MM-dd HH:mm:ss} [%p] %m%n"/></Console></Appenders><Loggers><Root level="info"><AppenderRef ref="ControlledOutput"/></Root></Loggers></Configuration>`
	path := filepath.Join(directory, name)
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

// applyExportDefaultLogging 只在未显式填写 --log-path 时使用受控 stdout，避免 Routing 在空输出目录先建日志。
// 业务 argv 不变；用户明确填写日志路径时继续采用官方配置和原参数。
func applyExportDefaultLogging(workspace credential.Workspace, profile, business []string) ([]string, error) {
	for _, argument := range business {
		if argument == "--log-path" {
			return profile, nil
		}
	}
	for i, option := range profile {
		if !strings.HasPrefix(option, "-Dlog4j.configurationFile=") {
			continue
		}
		configuration, err := controlledLogConfiguration(workspace, "export-log4j2.xml")
		if err != nil {
			return nil, err
		}
		result := append([]string(nil), profile...)
		result[i] = "-Dlog4j.configurationFile=file:///" + strings.TrimPrefix(filepath.ToSlash(configuration), "/")
		return result, nil
	}
	return nil, ErrDirectJavaLaunchInvalid
}
