package servicehost

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

type logWriterFunc func([]byte) (int, error)

// Write 将测试用例提供的写入行为适配为日志输出端口。
func (write logWriterFunc) Write(content []byte) (int, error) { return write(content) }

func TestServiceLogSurvivesUnavailableConsole(t *testing.T) {
	var file bytes.Buffer
	output := &serviceLogOutput{file: &file, console: logWriterFunc(func([]byte) (int, error) {
		return 0, errors.New("合成控制台句柄不可用")
	})}
	record := []byte("synthetic startup failure\n")
	n, err := output.Write(record)
	if err != nil || n != len(record) || !bytes.Equal(file.Bytes(), record) {
		t.Fatalf("控制台不可用时未完整保留服务日志：写入 %d 字节，错误 %v", n, err)
	}
}

func TestServiceLogPreservesRequiredFileFailures(t *testing.T) {
	fileError := errors.New("合成日志文件写入失败")
	for _, testCase := range []struct {
		name string
		file io.Writer
		want error
	}{
		{name: "文件写入失败", file: logWriterFunc(func([]byte) (int, error) { return 0, fileError }), want: fileError},
		{name: "文件短写", file: logWriterFunc(func([]byte) (int, error) { return 1, nil }), want: io.ErrShortWrite},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var console bytes.Buffer
			output := &serviceLogOutput{file: testCase.file, console: &console}
			if _, err := output.Write([]byte("synthetic record")); !errors.Is(err, testCase.want) {
				t.Fatalf("未保留必需文件写入错误：%v", err)
			}
			if console.Len() != 0 {
				t.Fatal("文件失败后不应只输出控制台并造成日志已保留的假象")
			}
		})
	}
}

func TestServiceLogCopiesAvailableConsole(t *testing.T) {
	var file, console bytes.Buffer
	output := &serviceLogOutput{file: &file, console: &console}
	record := []byte("synthetic service record")
	if _, err := output.Write(record); err != nil || !bytes.Equal(file.Bytes(), record) || !bytes.Equal(console.Bytes(), record) {
		t.Fatal("可用控制台未收到已保存的日志副本")
	}
	output.console = nil
	if _, err := output.Write(record); err != nil {
		t.Fatalf("无控制台时日志写入失败：%v", err)
	}
}

func TestLogOutputAppendsPrivateServiceFileWithUnavailableConsole(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "service.log")
	prior := []byte("synthetic prior record\n")
	if err := os.WriteFile(path, prior, 0600); err != nil {
		t.Fatal(err)
	}
	previousLogger := slog.Default()
	defer slog.SetDefault(previousLogger)
	output, closeLog, err := openLogOutput(directory, logWriterFunc(func([]byte) (int, error) {
		return 0, errors.New("合成后台服务没有控制台")
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	record := []byte("synthetic appended record\n")
	if _, err := output.Write(record); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(content, append(prior, record...)) {
		t.Fatal("服务日志未追加到原文件")
	}
}
