package deployment

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ob-data-orch/internal/credential"
)

// accountEntry 由现役认证适配器处理自己的账户，不开放主体选择或代改密码。
func (b *Browser) accountEntry(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/session":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			accountError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		} else {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]string{"username": "admin"})
		}
	case "/api/v1/session/password":
		b.changePassword(w, r)
	default:
		return false
	}
	return true
}

func (b *Browser) changePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		accountError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	if b.ValidateCSRF(r) != nil {
		accountError(w, http.StatusForbidden, "CSRF_INVALID")
		return
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/x-www-form-urlencoded" {
		accountError(w, http.StatusBadRequest, "PASSWORD_FIELDS_INVALID")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		accountError(w, http.StatusBadRequest, "PASSWORD_FIELDS_INVALID")
		return
	}
	current := []byte(r.PostForm.Get("currentPassword"))
	next := []byte(r.PostForm.Get("newPassword"))
	defer credential.Zero(current)
	defer credential.Zero(next)
	validFields := len(r.PostForm) == 2 && len(r.PostForm["currentPassword"]) == 1 && len(r.PostForm["newPassword"]) == 1
	r.PostForm = nil
	r.Form = nil
	if !validFields || len(current) == 0 || len(current) > 256 || len(next) < 8 || len(next) > 256 || bytes.Equal(current, next) {
		accountError(w, http.StatusBadRequest, "PASSWORD_FIELDS_INVALID")
		return
	}
	b.passwordMu.Lock()
	defer b.passwordMu.Unlock()
	// 等待其他轮换后必须重新核对会话，旧页面与并发请求不能继续修改新口令。
	if _, ok := b.session(r); !ok {
		accountError(w, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	b.mu.Lock()
	now := time.Now()
	if now.Sub(b.window) >= time.Minute {
		b.window, b.attempts = now, 0
	}
	b.attempts++
	allowed := b.attempts <= 12
	b.mu.Unlock()
	if !allowed {
		w.Header().Set("Retry-After", "60")
		accountError(w, http.StatusTooManyRequests, "PASSWORD_RATE_LIMITED")
		return
	}
	hash, err := passwordKey(current, b.settings.PasswordSalt)
	valid := err == nil && subtle.ConstantTimeCompare(hash, b.settings.PasswordHash) == 1
	credential.Zero(hash)
	if !valid {
		accountError(w, http.StatusBadRequest, "CURRENT_PASSWORD_INVALID")
		return
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		accountError(w, http.StatusServiceUnavailable, "PASSWORD_CHANGE_UNAVAILABLE")
		return
	}
	newHash, err := passwordKey(next, salt)
	if err != nil || !b.persistPassword(salt, newHash) {
		credential.Zero(newHash)
		accountError(w, http.StatusServiceUnavailable, "PASSWORD_CHANGE_UNAVAILABLE")
		return
	}
	// 磁盘提交成功后才切换内存凭据并撤销全部浏览器会话；不改变 Agent 身份。
	credential.Zero(b.settings.PasswordHash)
	b.settings.PasswordSalt, b.settings.PasswordHash = salt, newHash
	b.mu.Lock()
	clear(b.sessions)
	b.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

// persistPassword 保留配置的其他字段，只替换已核对版本的盐和派生值。
// 同目录临时文件先同步再替换；任何替换前失败都保留旧文件和旧会话，不输出文件错误。
func (b *Browser) persistPassword(salt, hash []byte) bool {
	path := b.settings.settingsPath
	if path == "" {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || credential.PreparePrivateDirectory(filepath.Dir(path)) != nil {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	content, readErr := io.ReadAll(io.LimitReader(file, 65537))
	closeErr := file.Close()
	defer credential.Zero(content)
	var persisted Settings
	var fields map[string]json.RawMessage
	if readErr != nil || closeErr != nil || len(content) > 65536 || json.Unmarshal(content, &persisted) != nil || validateSettings(persisted) != nil || !bytes.Equal(persisted.PasswordSalt, b.settings.PasswordSalt) || !bytes.Equal(persisted.PasswordHash, b.settings.PasswordHash) || json.Unmarshal(content, &fields) != nil {
		return false
	}
	fields["passwordSalt"], _ = json.Marshal(salt)
	fields["passwordHash"], _ = json.Marshal(hash)
	content, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return false
	}
	defer credential.Zero(content)
	temporary, err := os.CreateTemp(filepath.Dir(path), ".password-settings-*")
	if err != nil {
		return false
	}
	defer os.Remove(temporary.Name())
	_, writeErr := temporary.Write(content)
	syncErr := temporary.Sync()
	closeErr = temporary.Close()
	return writeErr == nil && syncErr == nil && closeErr == nil && os.Rename(temporary.Name(), path) == nil
}

func accountError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
}
