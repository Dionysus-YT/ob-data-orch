package deployment

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLoginNavigationFeedbackIsConsumedOnce 验证原生表单失败落在 GET，刷新不重提、不保留输入或重复错误。
func TestLoginNavigationFeedbackIsConsumedOnce(t *testing.T) {
	settings := testSettings(t)
	cases := []struct {
		name     string
		feedback loginFeedback
		origin   string
		form     string
		prepare  func(*Browser)
	}{
		{name: "口令错误", feedback: loginInvalidCredentials},
		{name: "跨来源拒绝", feedback: loginInvalidOrigin, origin: "https://other.example.test"},
		{name: "跨站导航拒绝", feedback: loginInvalidOrigin},
		{name: "格式错误", feedback: loginInvalidInput, form: "username=%ZZ"},
		{name: "频率限制", feedback: loginRateLimited, prepare: func(b *Browser) { b.attempts, b.window = 12, time.Now() }},
		{name: "会话容量限制", feedback: loginSessionsFull, form: url.Values{"username": {"admin"}, "password": {"synthetic-password-1234"}}.Encode(), prepare: func(b *Browser) {
			for index := range 128 {
				b.sessions[strconv.Itoa(index)] = browserSession{expires: time.Now().Add(time.Hour)}
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			browser, err := NewBrowser(settings)
			if err != nil {
				t.Fatal(err)
			}
			if test.prepare != nil {
				test.prepare(browser)
			}
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(`local-mvp-csrf-v1`), 0600); err != nil {
				t.Fatal(err)
			}
			handler, err := browser.Handler(http.NotFoundHandler(), directory, directory, directory)
			if err != nil {
				t.Fatal(err)
			}
			form := test.form
			if form == "" {
				form = url.Values{"username": {"synthetic-submitted-user"}, "password": {"synthetic-submitted-secret"}}.Encode()
			}
			origin := test.origin
			if origin == "" {
				origin = settings.PublicURL
			}
			request := httptest.NewRequest(http.MethodPost, settings.PublicURL+"/login", strings.NewReader(form))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Accept", "text/html")
			request.Header.Set("Origin", origin)
			if test.name == "跨站导航拒绝" {
				request.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
				t.Fatalf("失败导航没有转为固定 GET：%d", response.Code)
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Name != loginFeedbackCookie || cookies[0].Value != string(test.feedback) || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" || cookies[0].Domain != "" || cookies[0].MaxAge != 60 {
				t.Fatal("登录反馈不是固定、短时且受保护的独立 Cookie")
			}
			attempts := browser.attempts
			request = httptest.NewRequest(http.MethodGet, settings.PublicURL+"/login", nil)
			request.AddCookie(cookies[0])
			if _, err := browser.AuthenticateBrowser(request); err == nil {
				t.Fatal("反馈 Cookie 不得授予浏览器身份")
			}
			if _, err := browser.AuthenticateAgent(request); err == nil || browser.ValidateCSRF(request) == nil {
				t.Fatal("反馈 Cookie 不得代替机器身份或 CSRF")
			}
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			status, message := test.feedback.details()
			body := response.Body.String()
			if response.Code != status || !strings.Contains(body, message) || !strings.Contains(body, `role="alert"`) {
				t.Fatalf("GET 未消费固定失败反馈：%d", response.Code)
			}
			if strings.Contains(body, "synthetic-submitted-user") || strings.Contains(body, "synthetic-submitted-secret") || strings.Contains(body, "synthetic-password-1234") || strings.Contains(body, `value="admin"`) {
				t.Fatal("GET 不得回填账户或口令")
			}
			cleared := response.Result().Cookies()
			if len(cleared) != 1 || cleared[0].Name != loginFeedbackCookie || cleared[0].MaxAge >= 0 || !cleared[0].Secure || !cleared[0].HttpOnly {
				t.Fatal("GET 必须立即清除一次性反馈")
			}
			if status == http.StatusTooManyRequests && response.Header().Get("Retry-After") != "60" {
				t.Fatal("限流落地页须保留重试间隔")
			}
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, settings.PublicURL+"/login", nil))
			if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `role="alert"`) || browser.attempts != attempts {
				t.Fatal("刷新须为空白 GET，不重复反馈或增加认证尝试")
			}
			if status == http.StatusTooManyRequests {
				response = httptest.NewRecorder()
				writeLoginFailure(response, httptest.NewRequest(http.MethodPost, settings.PublicURL+"/login", nil), loginRateLimited)
				if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" || len(response.Result().Cookies()) != 0 {
					t.Fatal("原始客户端限流状态不得被导航反馈替代")
				}
			}
		})
	}
}

// TestUnknownLoginFeedbackFailsClosed 验证外部未知值不能形成原始错误回显或任何身份。
func TestUnknownLoginFeedbackFailsClosed(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://orch.example.test/login", nil)
	request.AddCookie(&http.Cookie{Name: loginFeedbackCookie, Value: "untrusted-feedback"})
	response := httptest.NewRecorder()
	status, message := consumeLoginFeedback(response, request)
	if status != http.StatusOK || message != "" || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("未知反馈须清除且不展示")
	}
	response = httptest.NewRecorder()
	writeLoginFailure(response, request, loginFeedback("untrusted-feedback"))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "untrusted-feedback") {
		t.Fatal("内部未知反馈必须失败关闭")
	}
}

// TestSuccessfulLoginClearsStaleFeedback 验证成功仍只靠正式会话认证，残留反馈被撤销。
func TestSuccessfulLoginClearsStaleFeedback(t *testing.T) {
	settings := testSettings(t)
	browser, err := NewBrowser(settings)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"username": {"admin"}, "password": {"synthetic-password-1234"}}
	request := httptest.NewRequest(http.MethodPost, settings.PublicURL+"/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", settings.PublicURL)
	request.AddCookie(&http.Cookie{Name: loginFeedbackCookie, Value: string(loginInvalidCredentials)})
	response := httptest.NewRecorder()
	browser.login(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Fatal("成功登录导航已改变")
	}
	var session, cleared bool
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == loginFeedbackCookie {
			cleared = cookie.MaxAge < 0
		}
		if cookie.Name == sessionCookie {
			session = true
			request = httptest.NewRequest(http.MethodGet, settings.PublicURL+"/", nil)
			request.AddCookie(cookie)
			if _, err := browser.AuthenticateBrowser(request); err != nil {
				t.Fatal("正式会话未建立")
			}
			if _, err := browser.AuthenticateAgent(request); err == nil {
				t.Fatal("正式浏览器会话不得代替机器身份")
			}
		}
	}
	if !session || !cleared {
		t.Fatal("成功登录须建立正式会话并清除残留反馈")
	}
}
