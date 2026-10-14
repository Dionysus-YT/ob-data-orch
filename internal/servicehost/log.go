package servicehost

import (
	"io"
	"log/slog"
	"ob-data-orch/internal/credential"
	"os"
	"path/filepath"
)

// LogOutput 为后台服务保留固定诊断文件，不记录输入密码、注册码或请求正文。
func LogOutput(directory string) (io.Writer, func(), error) {
	return openLogOutput(directory, os.Stdout)
}

// openLogOutput 把控制台作为可替换输出端口，持久日志的权限、追加及错误边界保持一致。
func openLogOutput(directory string, console io.Writer) (io.Writer, func(), error) {
	if err := credential.PreparePrivateDirectory(directory); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, "service.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, err
	}
	output := &serviceLogOutput{file: file, console: console}
	slog.SetDefault(slog.New(slog.NewJSONHandler(output, nil)))
	return output, func() { _ = file.Close() }, nil
}

// serviceLogOutput 以持久文件为主输出，控制台仅作为交互启动时的可选副本。
type serviceLogOutput struct {
	file    io.Writer
	console io.Writer
}

// Write 先保留文件写入结果；后台服务的控制台句柄不可用时仍须完整记录日志。
func (output *serviceLogOutput) Write(content []byte) (int, error) {
	n, err := output.file.Write(content)
	if err != nil {
		return n, err
	}
	if n != len(content) {
		return n, io.ErrShortWrite
	}
	if output.console != nil {
		// 可选控制台失败不覆盖持久日志成功，文件错误则仍向调用方传播。
		_, _ = output.console.Write(content)
	}
	return n, nil
}
