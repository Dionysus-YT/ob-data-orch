package deployment

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func accountFixture(t *testing.T) (*Browser, http.Handler, string) {
	t.Helper()
	directory := t.TempDir()
	settings, err := OpenSettings(directory, strings.NewReader("https://orch.example.test:8080\nsynthetic-password-1234\n"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBrowser(settings)
	if err != nil {
		t.Fatal(err)
	}
	b.sessions["synthetic-session"] = browserSession{csrf: "synthetic-csrf", expires: time.Now().Add(time.Hour)}
	b.sessions["synthetic-other"] = browserSession{csrf: "synthetic-other-csrf", expires: time.Now().Add(time.Hour)}
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("local-mvp-csrf-v1"), 0600); err != nil {
		t.Fatal(err)
	}
	handler, err := b.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("账户操作不得进入业务 API") }), directory, directory, directory)
	if err != nil {
		t.Fatal(err)
	}
	return b, handler, directory
}

func accountRequest(t *testing.T, handler http.Handler, method, path, body, token, origin string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://orch.example.test:8080"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", origin)
	r.Header.Set("X-CSRF-Token", token)
	if authenticated {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "synthetic-session"})
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), "synthetic-password") || strings.Contains(w.Body.String(), "synthetic-new") {
		t.Fatal("账户响应泄露秘密")
	}
	return w
}

func TestAccountSessionAndPasswordSecurityBoundary(t *testing.T) {
	_, handler, _ := accountFixture(t)
	fields := url.Values{"currentPassword": {"synthetic-password-1234"}, "newPassword": {"synthetic-new-password"}}.Encode()
	for _, tc := range []struct {
		name, method, path, body, csrf, origin string
		authenticated                          bool
		want                                   int
	}{
		{"匿名账户", "GET", "/api/v1/session", "", "", "", false, 401},
		{"机器凭据不授予账户", "POST", "/api/v1/session/password", fields, "synthetic-csrf", "", false, 401},
		{"匿名注销不重定向为成功", "POST", "/logout", "", "", "", false, 401},
		{"账户事实", "GET", "/api/v1/session", "", "", "", true, 200},
		{"账户方法", "POST", "/api/v1/session", "", "", "", true, 405},
		{"修改方法", "GET", "/api/v1/session/password", "", "synthetic-csrf", "", true, 405},
		{"缺少安全令牌", "POST", "/api/v1/session/password", fields, "", "", true, 403},
		{"其他会话令牌", "POST", "/api/v1/session/password", fields, "synthetic-other-csrf", "", true, 403},
		{"跨来源", "POST", "/api/v1/session/password", fields, "synthetic-csrf", "https://other.example.test", true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := accountRequest(t, handler, tc.method, tc.path, tc.body, tc.csrf, tc.origin, tc.authenticated)
			if w.Code != tc.want {
				t.Fatalf("状态 %d，预期 %d", w.Code, tc.want)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("账户响应必须禁止缓存")
			}
			if tc.want == 200 && strings.TrimSpace(w.Body.String()) != `{"username":"admin"}` {
				t.Fatal("账户事实必须为当前认证适配器的固定用户名")
			}
		})
	}
}

func TestBrowserPageInjectsAuthenticatedUsername(t *testing.T) {
	_, handler, directory := accountFixture(t)
	index := `<meta name="ob-data-orch-csrf-token" content="local-mvp-csrf-v1"><meta name="ob-data-orch-username" content="local-mvp-username-v1">`
	// Handler 已捕获启动时网页；共用渲染函数同样供开发代理调用。
	page := string(browserPage([]byte(index), "synthetic-csrf"))
	if !strings.Contains(page, `content="admin"`) || strings.Contains(page, "local-mvp-username-v1") || !strings.Contains(page, `content="synthetic-csrf"`) {
		t.Fatal("页面必须注入同一认证适配器的用户名和会话安全令牌")
	}
	if w := accountRequest(t, handler, "GET", "/api/v1/session", "", "", "", false); w.Code != 401 || strings.Contains(w.Body.String(), "admin") {
		t.Fatal("匿名账户查询不得泄露登录名")
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 2 {
		t.Fatal("只读账户查询不得新增身份或会话持久化文件")
	}
}

func TestPasswordValidationAndRateLimitPreserveOldCredentials(t *testing.T) {
	b, handler, directory := accountFixture(t)
	before, _ := os.ReadFile(filepath.Join(directory, "settings.json"))
	for _, tc := range []struct{ current, next string }{
		{"", "synthetic-new-password"}, {"wrong", "synthetic-new-password"},
		{"synthetic-password-1234", "short"}, {"synthetic-password-1234", "synthetic-password-1234"},
		{"synthetic-password-1234", strings.Repeat("中", 86)}, {"synthetic-password-1234", strings.Repeat("x", 257)},
	} {
		body := url.Values{"currentPassword": {tc.current}, "newPassword": {tc.next}}.Encode()
		if w := accountRequest(t, handler, "POST", "/api/v1/session/password", body, "synthetic-csrf", "", true); w.Code != 400 {
			t.Fatal("非法密码未被拒绝")
		}
	}
	for _, body := range []string{
		"currentPassword=x&currentPassword=y&newPassword=synthetic-new-password",
		"currentPassword=x&newPassword=synthetic-new-password&subjectId=other",
		strings.Repeat("x", 4097),
	} {
		if w := accountRequest(t, handler, "POST", "/api/v1/session/password", body, "synthetic-csrf", "", true); w.Code != 400 {
			t.Fatal("非法结构未被拒绝")
		}
	}
	for i := 0; i < 12; i++ {
		accountRequest(t, handler, "POST", "/api/v1/session/password", "currentPassword=wrong&newPassword=synthetic-new-password", "synthetic-csrf", "", true)
	}
	w := accountRequest(t, handler, "POST", "/api/v1/session/password", "currentPassword=wrong&newPassword=synthetic-new-password", "synthetic-csrf", "", true)
	if w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatal("旧口令猜测必须限流")
	}
	after, _ := os.ReadFile(filepath.Join(directory, "settings.json"))
	if !bytes.Equal(before, after) || len(b.sessions) != 2 {
		t.Fatal("失败不得改变凭据或撤销会话")
	}
}

func TestPasswordCommitPersistsAndRevokesAllBrowserSessions(t *testing.T) {
	b, handler, directory := accountFixture(t)
	path := filepath.Join(directory, "settings.json")
	content, _ := os.ReadFile(path)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(content, &fields)
	fields["futureSetting"] = json.RawMessage(`{"keep":true}`)
	content, _ = json.Marshal(fields)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	body := url.Values{"currentPassword": {"synthetic-password-1234"}, "newPassword": {"synthetic-new-password"}}.Encode()
	w := accountRequest(t, handler, "POST", "/api/v1/session/password", body, "synthetic-csrf", "", true)
	if w.Code != 204 || len(b.sessions) != 0 {
		t.Fatal("成功必须清除全部浏览器会话")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("成功必须撤销安全 Cookie")
	}
	reopened, err := OpenSettings(directory, strings.NewReader(""), io.Discard)
	if err != nil || !bytes.Equal(reopened.PasswordHash, b.settings.PasswordHash) || !bytes.Equal(reopened.PasswordSalt, b.settings.PasswordSalt) {
		t.Fatal("重启必须使用已提交密码")
	}
	after, _ := os.ReadFile(path)
	if bytes.Contains(after, []byte("synthetic-password")) || bytes.Contains(after, []byte("synthetic-new")) || !bytes.Contains(after, []byte(`"futureSetting"`)) {
		t.Fatal("持久化须仅含派生值并保留其他设置")
	}
	for _, tc := range []struct {
		password string
		want     int
	}{{"synthetic-password-1234", 401}, {"synthetic-new-password", 303}} {
		r := httptest.NewRequest("POST", b.settings.PublicURL+"/login", strings.NewReader(url.Values{"username": {"admin"}, "password": {tc.password}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != tc.want {
			t.Fatal("新旧密码登录结果不符合提交事实")
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(directory, ".password-settings-*"))
	if len(leftovers) != 0 {
		t.Fatal("不得遗留密码派生配置临时文件")
	}
}

func TestPasswordPersistenceFailureAndConcurrentRequests(t *testing.T) {
	t.Run("持久配置损坏失败关闭", func(t *testing.T) {
		b, handler, directory := accountFixture(t)
		before := append([]byte(nil), b.settings.PasswordHash...)
		path := filepath.Join(directory, "settings.json")
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		w := accountRequest(t, handler, "POST", "/api/v1/session/password", "currentPassword=synthetic-password-1234&newPassword=synthetic-new-password", "synthetic-csrf", "", true)
		if w.Code != 503 || !bytes.Equal(before, b.settings.PasswordHash) || len(b.sessions) != 2 {
			t.Fatal("持久化失败不得提前切换内存或会话")
		}
		content, _ := os.ReadFile(path)
		if string(content) != "{}" {
			t.Fatal("不得静默覆盖失配或损坏配置")
		}
	})
	t.Run("并发轮换仅一次提交", func(t *testing.T) {
		_, handler, _ := accountFixture(t)
		var group sync.WaitGroup
		results := make(chan int, 2)
		for i := 0; i < 2; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				results <- accountRequest(t, handler, "POST", "/api/v1/session/password", "currentPassword=synthetic-password-1234&newPassword=synthetic-new-password", "synthetic-csrf", "", true).Code
			}()
		}
		group.Wait()
		close(results)
		counts := map[int]int{}
		for code := range results {
			counts[code]++
		}
		if counts[204] != 1 || counts[401] != 1 {
			t.Fatal("并发请求必须至多一次修改并拒绝旧会话")
		}
	})
}
