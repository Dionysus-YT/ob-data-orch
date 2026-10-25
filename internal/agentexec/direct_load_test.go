package agentexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test旁路类加载以已核验主包优先并排除工作目录(t *testing.T) {
	workspace, security := directJavaWorkspace(t)
	_, home, _ := directJavaFiles(t)
	profile, err := windowsDirectLoadReplicaArguments(workspace, home, security, 491)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, option := range profile {
		if option != "-classpath" {
			continue
		}
		found = true
		parts := strings.Split(profile[i+1], ";")
		if len(parts) != 3 || parts[0] != filepath.Join(home, "lib", "ob-loader-dumper-4.3.5-RELEASE.jar") || parts[1] != filepath.Join(home, "lib", "*") || parts[2] != workspace.RuntimeDirectory() {
			t.Fatal("旁路类加载没有绑定固定主包")
		}
	}
	if !found || profile[len(profile)-1] != obloaderMainClass {
		t.Fatal("旁路固定主类缺失")
	}
}

func Test旁路版本不依赖文件名且拒绝未知包和取消(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "lib"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "lib", "ob-loader-dumper-4.3.5-RELEASE.jar")
	for _, body := range [][]byte{nil, []byte("synthetic-4.3.5-RELEASE")} {
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		if version, err := DirectLoadToolVersion(context.Background(), root); err == nil || version != "" {
			t.Fatal("未知或空包被文件名冒充版本")
		}
		if len(body) > 0 {
			expected := sha256.Sum256(body)
			if digest, err := directLoadToolDigest(context.Background(), root); err != nil || digest != hex.EncodeToString(expected[:]) {
				t.Fatal("固定路径摘要读取不一致", err)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, input := range []struct {
		ctx  context.Context
		home string
	}{{nil, root}, {ctx, root}, {context.Background(), "relative"}, {context.Background(), t.TempDir()}} {
		if version, err := DirectLoadToolVersion(input.ctx, input.home); err == nil || version != "" {
			t.Fatal("无效版本读取被接受")
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(64<<20 + 1)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if version, err := DirectLoadToolVersion(context.Background(), root); err == nil || version != "" {
		t.Fatal("超预算包未在读取前拒绝")
	}
}

func Test旁路日志配置仅走受控管道且SDK路径绑定私有目录(t *testing.T) {
	workspace, security := directJavaWorkspace(t)
	_, home, _ := directJavaFiles(t)
	profile, err := windowsDirectLoadReplicaArguments(workspace, home, security, 491)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(profile, "\n")
	if !strings.Contains(joined, "-Dlogging.config.com.alipay.remoting=direct-load-log4j2.xml") {
		t.Fatal("Bolt 未绑定固定受控日志配置")
	}
	for _, property := range []string{"-Dlogging.path=", "-Dlogging.path.oceanbase-table-client="} {
		if !strings.Contains(joined, property+workspace.RawLogDirectory()) {
			t.Fatal("SDK 日志未绑定私有目录")
		}
	}
	path := filepath.Join(workspace.RuntimeDirectory(), "direct-load-log4j2.xml")
	body, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(body), "Routing") || strings.Contains(string(body), "task.workspace") || !strings.Contains(string(body), "SYSTEM_OUT") {
		t.Fatal("旁路仍使用输入目录路由日志")
	}
	if err := os.WriteFile(path, []byte("synthetic-tampering"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := windowsDirectLoadReplicaArguments(workspace, home, security, 491); err == nil {
		t.Fatal("被替换的固定日志配置未阻断")
	}
}
