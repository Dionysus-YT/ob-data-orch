package deployment

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identity"
)

const sessionCookie = "__Host-ob_data_orch_session"

type browserSession struct {
	csrf    string
	expires time.Time
}

// Browser 提供单管理员登录与独立的浏览器会话，不接受机器凭据或来源 IP 作为用户身份。
type Browser struct {
	settings Settings
	mu       sync.Mutex
	sessions map[string]browserSession
	attempts int
	window   time.Time
}

// NewBrowser 创建固定管理员的认证边界，重启后浏览器重新登录，Agent 身份不受影响。
func NewBrowser(settings Settings) (*Browser, error) {
	if err := validateSettings(settings); err != nil {
		return nil, err
	}
	return &Browser{settings: settings, sessions: make(map[string]browserSession)}, nil
}

func (b *Browser) session(r *http.Request) (browserSession, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return browserSession{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	session, ok := b.sessions[cookie.Value]
	if ok && !time.Now().Before(session.expires) {
		delete(b.sessions, cookie.Value)
		ok = false
	}
	return session, ok
}

// AuthenticateBrowser 只接受本服务通过登录建立的有效会话。
func (b *Browser) AuthenticateBrowser(r *http.Request) (identity.Principal, error) {
	if _, ok := b.session(r); !ok {
		return identity.Principal{}, identity.ErrUnauthenticated
	}
	return identity.Principal{Type: identity.BrowserPrincipal, ID: SubjectID}, nil
}

// AuthenticateAgent 明确拒绝以浏览器认证边界代替机器协议认证。
func (b *Browser) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthenticated
}

// Authorize 将初始管理员显式绑定到现有业务范围，不接受客户端声明角色。
func (b *Browser) Authorize(_ context.Context, p identity.Principal, scope identity.Scope, object string) error {
	if p.Type != identity.BrowserPrincipal || p.ID != SubjectID || object == "" {
		return identity.ErrDenied
	}
	switch scope {
	case identity.ScopeDataSourceRead, identity.ScopeDataSourceUse, identity.ScopeDataSourceWrite, identity.ScopeNodeUse, identity.ScopeNodeManage, identity.ScopeTaskRead, identity.ScopeTaskReadAll:
		return nil
	}
	return identity.ErrDenied
}

// AuthorizeRole 为可信固定管理员授予全局、数据源和节点职责，客户端不能自授角色。
func (b *Browser) AuthorizeRole(_ context.Context, p identity.Principal, role identity.Role) error {
	if p.Type == identity.BrowserPrincipal && p.ID == SubjectID && (role == identity.RoleDataSourceAdmin || role == identity.RoleNodeAdmin || role == identity.RoleGlobalAdmin) {
		return nil
	}
	return identity.ErrDenied
}

// ValidateCSRF 将写请求令牌绑定到当前会话，同时拒绝跨来源请求。
func (b *Browser) ValidateCSRF(r *http.Request) error {
	session, ok := b.session(r)
	if !ok || !b.sameOrigin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.csrf)) != 1 {
		return errors.New("请求安全令牌无效")
	}
	return nil
}

func (b *Browser) sameOrigin(r *http.Request) bool {
	return (r.Header.Get("Origin") == "" || strings.TrimSuffix(r.Header.Get("Origin"), "/") == strings.TrimSuffix(b.settings.PublicURL, "/")) && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

// Handler 将登录、受认证静态网页及下载与业务 API 组成同一个 HTTPS 服务。
func (b *Browser) Handler(api http.Handler, webDirectory, packageDirectory, dataDirectory string) (http.Handler, error) {
	return b.handler(api, webDirectory, packageDirectory, dataDirectory, nil)
}

func (b *Browser) handler(api http.Handler, webDirectory, packageDirectory, dataDirectory string, development http.Handler) (http.Handler, error) {
	index, err := os.ReadFile(filepath.Join(webDirectory, "index.html"))
	if err != nil {
		return nil, errors.New("网页资源缺失，请使用完整安装包")
	}
	if !strings.Contains(string(index), "local-mvp-csrf-v1") {
		return nil, errors.New("网页安全令牌占位缺失，请重新构建安装包")
	}
	static := http.FileServer(http.Dir(webDirectory))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/agent/v1/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			api.ServeHTTP(w, r)
			return
		}
		if development == nil && strings.HasPrefix(r.URL.Path, "/assets/") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			static.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/login" {
			b.login(w, r)
			return
		}
		session, ok := b.session(r)
		if !ok {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "UNAUTHENTICATED", "message": "请登录"}})
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		if r.URL.Path == "/logout" {
			if r.Method != http.MethodPost || b.ValidateCSRF(r) != nil {
				http.Error(w, "请求无效", http.StatusForbidden)
				return
			}
			cookie, _ := r.Cookie(sessionCookie)
			b.mu.Lock()
			delete(b.sessions, cookie.Value)
			b.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/ob-data-orch-agent-") {
			b.agentPackage(w, r, packageDirectory, dataDirectory)
			return
		}
		if development != nil {
			if !b.sameOrigin(r) {
				http.Error(w, "开发资源来源无效", http.StatusForbidden)
				return
			}
			development.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			static.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(strings.ReplaceAll(string(index), "local-mvp-csrf-v1", session.csrf)))
		}
	}), nil
}

// login 处理固定管理员登录；失败分支保留原状态码，并在同一登录页给出固定中文提示，便于直接重试。
func (b *Browser) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if !b.sameOrigin(r) {
			renderLogin(w, http.StatusForbidden, "请求来源无效，请从平台地址重新打开登录页。", "")
			return
		}
		b.mu.Lock()
		now := time.Now()
		if now.Sub(b.window) >= time.Minute {
			b.window = now
			b.attempts = 0
		}
		b.attempts++
		allowed := b.attempts <= 12
		b.mu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", "60")
			renderLogin(w, http.StatusTooManyRequests, "尝试过于频繁，请 1 分钟后再试。", "")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.ParseForm() != nil {
			renderLogin(w, http.StatusBadRequest, "登录信息无效，请重新输入。", "")
			return
		}
		username := r.PostForm.Get("username")
		password := []byte(r.PostForm.Get("password"))
		r.PostForm.Del("password")
		r.Form.Del("password")
		hash, err := passwordKey(password, b.settings.PasswordSalt)
		credential.Zero(password)
		valid := err == nil && subtle.ConstantTimeCompare(hash, b.settings.PasswordHash) == 1 && username == "admin"
		credential.Zero(hash)
		if valid {
			token, csrf := randomToken(), randomToken()
			if token == "" || csrf == "" {
				renderLogin(w, http.StatusServiceUnavailable, "登录暂时不可用，请稍后重试。", username)
				return
			}
			b.mu.Lock()
			for key, session := range b.sessions {
				if !now.Before(session.expires) {
					delete(b.sessions, key)
				}
			}
			if len(b.sessions) >= 128 {
				b.mu.Unlock()
				renderLogin(w, http.StatusServiceUnavailable, "当前有效会话过多，请稍后重试。", username)
				return
			}
			b.sessions[token] = browserSession{csrf: csrf, expires: now.Add(12 * time.Hour)}
			b.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: 43200, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		renderLogin(w, http.StatusUnauthorized, "账户或密码不正确", username)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	renderLogin(w, http.StatusOK, "", "")
}

func randomToken() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
