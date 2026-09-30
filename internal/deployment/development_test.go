package deployment

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevelopmentProxyPreservesAuthenticationAndCSRF(t *testing.T) {
	settings := testSettings(t)
	settings.PublicURL = "https://127.0.0.1:18443"
	settings.RealExecutionEnabled = true
	browser, _ := NewBrowser(settings)
	browser.sessions["synthetic-session"] = browserSession{csrf: "synthetic-csrf", expires: time.Now().Add(time.Hour)}
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("开发服务器收到身份材料")
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<meta content="local-mvp-csrf-v1"><script src="/@vite/client"></script>`)
	}))
	defer upstream.Close()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("local-mvp-csrf-v1"), 0600)
	handler, err := browser.DevelopmentHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), dir, dir, dir, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, origin  string
		authenticated bool
		want          int
	}{
		{"/", "", false, 303}, {"/@vite/client", "", false, 303}, {"/api/v1/test", "", false, 401},
		{"/", "https://other.example.test", true, 403}, {"/", settings.PublicURL, true, 200},
		{"/agent/v1/test", "", false, 204}, {"/api/v1/test", settings.PublicURL, true, 204},
	} {
		r := httptest.NewRequest("GET", settings.PublicURL+tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		if tc.authenticated {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "synthetic-session"})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if w.Code == 200 && (!strings.Contains(w.Body.String(), "synthetic-csrf") || strings.Contains(w.Body.String(), "local-mvp-csrf-v1")) {
			t.Fatal("未注入真实会话令牌")
		}
	}
	if calls != 1 {
		t.Fatalf("未授权请求或 API 到达开发前端: %d", calls)
	}
	upstream.Close()
	r := httptest.NewRequest("GET", settings.PublicURL+"/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "synthetic-session"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 502 {
		t.Fatal("Vite 退出未失败关闭")
	}
}

func TestDevelopmentProxyRejectsUnsafeConfiguration(t *testing.T) {
	settings := testSettings(t)
	for _, tc := range []struct {
		public, upstream string
		real             bool
	}{
		{"https://orch.example.test", "http://127.0.0.1:15173", false},
		{"https://127.0.0.1:18443", "http://example.test:15173", false},
		{"https://127.0.0.1:18443", "http://127.0.0.1:15173/path", false},
	} {
		settings.PublicURL = tc.public
		settings.RealExecutionEnabled = tc.real
		b, _ := NewBrowser(settings)
		if _, err := b.DevelopmentHandler(nil, "", "", "", tc.upstream); err == nil {
			t.Fatal("接受不安全开发配置")
		}
	}
}
