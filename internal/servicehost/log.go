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
	if err := credential.PreparePrivateDirectory(directory); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, "service.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, err
	}
	output := io.MultiWriter(os.Stdout, file)
	slog.SetDefault(slog.New(slog.NewJSONHandler(output, nil)))
	return output, func() { _ = file.Close() }, nil
}
