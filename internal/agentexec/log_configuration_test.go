package agentexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ob-data-orch/internal/credential"
)

func Test默认导出配置复用且显式路径不写运行资产(t *testing.T) {
	workspace, security := directJavaWorkspace(t)
	_, home, _ := directJavaFiles(t)
	profile, err := windowsOBDumperReplicaArguments(workspace, home, security, 491)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyExportDefaultLogging(workspace, profile, []string{"--log-path", filepath.Join(t.TempDir(), "logs")}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace.RuntimeDirectory(), "export-log4j2.xml")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("显式日志路径仍写了默认资产")
	}
	for range 2 {
		if _, err := applyExportDefaultLogging(workspace, profile, []string{"--csv"}); err != nil {
			t.Fatal("相同的受控配置无法复用", err)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(body), "Routing") || strings.Contains(string(body), "task.workspace") || !strings.Contains(string(body), "SYSTEM_OUT") {
		t.Fatal("默认日志仍采用输出目录路由")
	}
}

func Test默认导出日志篡改和目录替换阻断(t *testing.T) {
	for _, kind := range []string{"同长度篡改", "配置变目录", "运行目录变文件"} {
		t.Run(kind, func(t *testing.T) {
			workspace, security := directJavaWorkspace(t)
			_, home, _ := directJavaFiles(t)
			profile, err := windowsOBDumperReplicaArguments(workspace, home, security, 491)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := applyExportDefaultLogging(workspace, profile, []string{"--csv"}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(workspace.RuntimeDirectory(), "export-log4j2.xml")
			switch kind {
			case "同长度篡改":
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				body[len(body)-2] ^= 1
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			case "配置变目录":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "运行目录变文件":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(workspace.RuntimeDirectory()); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(workspace.RuntimeDirectory(), []byte("synthetic-invalid-directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := applyExportDefaultLogging(workspace, profile, []string{"--csv"}); err == nil {
				t.Fatal("篡改或目录替换仍可进入启动")
			}
		})
	}
}

func Test三类工具临时目录唯一绑定当前工作区(t *testing.T) {
	workspace, security := directJavaWorkspace(t)
	_, home, _ := directJavaFiles(t)
	for _, build := range []func(credential.Workspace, string, string, int) ([]string, error){windowsOBDumperReplicaArguments, windowsOBLoaderReplicaArguments, windowsDirectLoadReplicaArguments} {
		profile, err := build(workspace, home, security, 491)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, argument := range profile {
			if strings.HasPrefix(argument, "-Djava.io.tmpdir=") {
				count++
				if argument != "-Djava.io.tmpdir="+workspace.RuntimeDirectory() {
					t.Fatal("临时目录串到其他位置")
				}
			}
		}
		if count != 1 {
			t.Fatal("临时目录缺失或重复")
		}
	}
}
