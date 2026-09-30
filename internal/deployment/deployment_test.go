package deployment

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSettings(t *testing.T) Settings {
	t.Helper()
	settings, err := OpenSettings(t.TempDir(), strings.NewReader("https://orch.example.test:8080\nsynthetic-password-1234\n"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestSettingsRestartPreservesIdentityAndRejectsCorruption(t *testing.T) {
	directory := t.TempDir()
	first, err := OpenSettings(directory, strings.NewReader("https://orch.example.test:8080\nsynthetic-password-1234\n"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenSettings(directory, strings.NewReader(""), io.Discard)
	if err != nil || !bytes.Equal(first.PasswordHash, second.PasswordHash) {
		t.Fatal("重启改变管理员身份")
	}
	content, _ := os.ReadFile(filepath.Join(directory, "settings.json"))
	if bytes.Contains(content, []byte("synthetic-password")) {
		t.Fatal("密码不得明文保存")
	}
	os.WriteFile(filepath.Join(directory, "settings.json"), []byte("{}"), 0600)
	if _, err := OpenSettings(directory, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("损坏配置不能自动重建")
	}
}

func TestBrowserLoginCSRFAndMachineIsolation(t *testing.T) {
	settings := testSettings(t)
	browser, err := NewBrowser(settings)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	os.WriteFile(filepath.Join(directory, "index.html"), []byte(`<meta name="ob-data-orch-csrf-token" content="local-mvp-csrf-v1">`), 0600)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	handler, err := browser.Handler(api, directory, directory, directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/data-sources", "/ob-data-orch-agent-windows-amd64.zip", "/"} {
		request := httptest.NewRequest("GET", settings.PublicURL+path, nil)
		request.RemoteAddr = "127.0.0.1:12345"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 401 && response.Code != 303 {
			t.Fatalf("匿名访问未被拒绝 %s: %d", path, response.Code)
		}
	}
	login := func(password, origin string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", settings.PublicURL+"/login", strings.NewReader(url.Values{"username": {"admin"}, "password": {password}}.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if login("wrong", settings.PublicURL).Code != 401 {
		t.Fatal("错误密码被接受")
	}
	if login("synthetic-password-1234", "https://other.example.test").Code != 403 {
		t.Fatal("跨来源登录被接受")
	}
	response := login("synthetic-password-1234", settings.PublicURL)
	if response.Code != 303 {
		t.Fatalf("登录失败 %d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("会话 Cookie 缺少保护")
	}
	request := httptest.NewRequest("POST", settings.PublicURL+"/api/v1/example", nil)
	request.AddCookie(cookies[0])
	request.Header.Set("Origin", settings.PublicURL)
	if _, err := browser.AuthenticateBrowser(request); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.AuthenticateAgent(request); err == nil {
		t.Fatal("浏览器身份不能成为机器身份")
	}
	if browser.ValidateCSRF(request) == nil {
		t.Fatal("缺少 CSRF 未拒绝")
	}
	session, _ := browser.session(request)
	request.Header.Set("X-CSRF-Token", session.csrf)
	if err := browser.ValidateCSRF(request); err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://other.example.test")
	if browser.ValidateCSRF(request) == nil {
		t.Fatal("跨来源写入未拒绝")
	}
	request = httptest.NewRequest("GET", settings.PublicURL+"/nodes", nil)
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), session.csrf) || strings.Contains(response.Body.String(), "local-mvp-csrf-v1") {
		t.Fatal("页面未注入会话令牌")
	}
	second, _ := NewBrowser(settings)
	if _, err := second.AuthenticateBrowser(request); err == nil {
		t.Fatal("重启后旧浏览器会话应失效")
	}
	request = httptest.NewRequest("GET", settings.PublicURL+"/agent/v1/test", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal("机器请求不应被浏览器登录拦截，须交由机器协议认证")
	}
}

func TestTLSStableTrustAndHostnameVerification(t *testing.T) {
	directory := t.TempDir()
	first, err := TLSConfig(directory, "https://orch.example.test:8080")
	if err != nil {
		t.Fatal(err)
	}
	second, err := TLSConfig(directory, "https://orch.example.test:8080")
	if err != nil {
		t.Fatal(err)
	}
	a, err := first.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Certificate[1], b.Certificate[1]) {
		t.Fatal("重启更换了信任根")
	}
	roots := x509.NewCertPool()
	ca, _ := x509.ParseCertificate(a.Certificate[1])
	roots.AddCert(ca)
	if _, err := b.Leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "orch.example.test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "other.example.test"}); err == nil {
		t.Fatal("错误主机名被接受")
	}
	os.Remove(filepath.Join(directory, "server-trust-key.json"))
	if _, err := TLSConfig(directory, "https://orch.example.test:8080"); err == nil {
		t.Fatal("缺失私钥不能自动替换已有信任")
	}
}
