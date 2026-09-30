package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"ob-data-orch/internal/deployment"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeWithoutSettingsDoesNotExposeLocalIdentity(t *testing.T) {
	directory := t.TempDir()
	os.MkdirAll(filepath.Join(directory, "data"), 0700)
	os.WriteFile(filepath.Join(directory, "data", "settings.json"), []byte("{}"), 0600)
	err := serve(context.Background(), directory, filepath.Join(directory, "data"))
	if err == nil {
		t.Fatal("缺少首次配置不能启动匿名控制面")
	}
}

func TestDevelopmentRestartPreservesDataAndTrust(t *testing.T) {
	directory := t.TempDir()
	data := filepath.Join(directory, "data")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	settings, err := deployment.OpenSettings(data, strings.NewReader(fmt.Sprintf("https://127.0.0.1:%d\nsynthetic-development-password\n", port)), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	// 开启真实能力仅验证服务组装，测试没有数据源、任务或外部端点，不触发探测或工具。
	settings.RealExecutionEnabled = true
	content, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "settings.json"), content, 0600); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(directory, "web"), 0700)
	os.WriteFile(filepath.Join(directory, "web/index.html"), []byte("local-mvp-csrf-v1"), 0600)
	var trust string
	for cycle := 0; cycle < 2; cycle++ {
		os.Remove(filepath.Join(directory, "dev.stop"))
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- serveWithDevelopment(ctx, directory, data, "http://127.0.0.1:15173") }()
		ready := false
		for attempt := 0; attempt < 400; attempt++ {
			if deployment.CheckHealth(data) == nil {
				ready = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !ready {
			cancel()
			t.Fatal("开发控制面未健康启动")
		}
		ca, err := os.ReadFile(filepath.Join(data, "control-plane-ca.pem"))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if cycle == 0 {
			trust = string(ca)
		} else if trust != string(ca) {
			cancel()
			t.Fatal("重启改变信任根")
		}
		os.WriteFile(filepath.Join(directory, "dev.stop"), nil, 0600)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(20 * time.Second):
			cancel()
			t.Fatal("开发进程未正常停止")
		}
		cancel()
		if _, err := os.Stat(filepath.Join(data, "local-mvp.db")); err != nil {
			t.Fatal("开发数据库丢失")
		}
	}
}
