package deployment

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestDevelopmentViteHMR 使用真实 Vite 和临时 TLS 服务验证源码响应及 WebSocket 转发；不启动业务工具。
func TestDevelopmentViteHMR(t *testing.T) {
	if os.Getenv("OBDO_TEST_VITE") != "1" {
		t.Skip("通过 OBDO_TEST_VITE=1 显式运行已安装前端依赖的隔离联调")
	}
	web, _ := filepath.Abs("../../web")
	fixture, err := os.CreateTemp(web, "dev-hmr-*.js")
	if err != nil {
		t.Fatal(err)
	}
	fixture.Close()
	defer os.Remove(fixture.Name())
	if err := os.WriteFile(fixture.Name(), []byte("export const value = 'before'; if (import.meta.hot) import.meta.hot.accept();"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	process := exec.Command("node", filepath.Join(web, "node_modules/vite/bin/vite.js"), "--port", fmt.Sprint(port))
	process.Dir = web
	logPath := filepath.Join(t.TempDir(), "vite.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	process.Stdout = logFile
	process.Stderr = logFile
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { process.Process.Kill(); process.Wait() }()
	upstream := fmt.Sprintf("http://127.0.0.1:%d", port)
	ready := false
	client := &http.Client{Timeout: time.Second}
	for i := 0; i < 160; i++ {
		response, err := client.Get(upstream)
		if err == nil {
			response.Body.Close()
			ready = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !ready {
		content, _ := os.ReadFile(logPath)
		t.Fatalf("Vite 未启动: %s", content)
	}
	settings := testSettings(t)
	settings.PublicURL = "https://127.0.0.1:18443"
	b, _ := NewBrowser(settings)
	b.sessions["synthetic-session"] = browserSession{csrf: "synthetic-csrf", expires: time.Now().Add(time.Hour)}
	handler, err := b.DevelopmentHandler(http.NotFoundHandler(), web, t.TempDir(), t.TempDir(), upstream)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	fetch := func(route string) string {
		r, _ := http.NewRequest("GET", server.URL+route, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "synthetic-session"})
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 {
			t.Fatalf("%s: %d", route, response.StatusCode)
		}
		return string(body)
	}
	index := fetch("/")
	if !strings.Contains(index, "/@vite/client") || !strings.Contains(index, "synthetic-csrf") {
		t.Fatal("未取得 Vite 页面和会话令牌")
	}
	if !strings.Contains(fetch("/src/main.ts"), "createApp") {
		t.Fatal("未取得转换后的源码")
	}
	clientCode := fetch("/@vite/client")
	fetch("/" + filepath.Base(fixture.Name()))
	token := regexp.MustCompile(`const wsToken = "([^"]+)"`).FindStringSubmatch(clientCode)
	if len(token) != 2 {
		t.Fatal("未取得 HMR 握手令牌")
	}
	connection, err := tls.Dial("tcp", strings.TrimPrefix(server.URL, "https://"), server.Client().Transport.(*http.Transport).TLSClientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(connection, "GET /?token=%s HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nCookie: %s=synthetic-session\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: c3ludGhldGljLWhhbmRzaA==\r\nSec-WebSocket-Protocol: vite-hmr\r\n\r\n", token[1], strings.TrimPrefix(server.URL, "https://"), settings.PublicURL, sessionCookie)
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 101 {
		t.Fatalf("HMR 升级失败: %d", response.StatusCode)
	}
	// Vite 首帧必须确认为 connected，不能仅凭 HTTP 101 声称通道可用。
	frame := make([]byte, 2)
	if _, err := io.ReadFull(reader, frame); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, int(frame[1]&127))
	if _, err := io.ReadFull(reader, payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "connected") {
		t.Fatal("HMR 未确认连接")
	}
	if err := os.WriteFile(fixture.Name(), []byte("export const value = 'after'; if (import.meta.hot) import.meta.hot.accept();"), 0600); err != nil {
		t.Fatal(err)
	}
	updated := false
	for attempt := 0; attempt < 10; attempt++ {
		if _, err := io.ReadFull(reader, frame); err != nil {
			t.Fatal(err)
		}
		length := int(frame[1] & 127)
		if length == 126 {
			size := make([]byte, 2)
			io.ReadFull(reader, size)
			length = int(size[0])*256 + int(size[1])
		}
		if length >= 65536 {
			t.Fatal("更新帧过大")
		}
		payload = make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), `"type":"update"`) && strings.Contains(string(payload), filepath.Base(fixture.Name())) {
			updated = true
			break
		}
	}
	if !updated {
		t.Fatal("未收到夹具 HMR 更新通知")
	}
	if !strings.Contains(fetch("/"+filepath.Base(fixture.Name())), "after") {
		t.Fatal("源码更新未生效")
	}
}
