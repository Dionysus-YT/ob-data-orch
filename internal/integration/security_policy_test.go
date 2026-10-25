package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/deployment"
	"ob-data-orch/internal/securitypolicy"
	"ob-data-orch/internal/store"
	"ob-data-orch/internal/toolcompatibility"
)

// Test安全与工具状态实际登录会话与持久授权 使用临时 SQLite、合成口令和合成 keyring，不接触现役认证或真实凭据。
func Test安全与工具状态实际登录会话与持久授权(t *testing.T) {
	database, databasePath := openSyntheticStore(t)
	now := time.Now().UTC()
	if err := database.EnsureAuthSubject(context.Background(), store.AuthSubject{SubjectID: deployment.SubjectID, ExternalSubject: deployment.SubjectID, DisplayName: "合成管理员", AccountStatus: "ACTIVE", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := database.BootstrapPermissionOwner(context.Background(), deployment.SubjectID, now); err != nil {
		t.Fatal(err)
	}
	settings, err := deployment.OpenSettings(t.TempDir(), strings.NewReader("https://orch.example.test:8080\nsynthetic-password-1234\n"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := deployment.NewBrowser(settings)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	keyring, err := credential.NewKeyring(map[string][]byte{"synthetic-key-v1": key})
	credential.Zero(key)
	if err != nil {
		t.Fatal(err)
	}
	status, err := securitypolicy.New(securitypolicy.Startup{LoadedAt: now, CredentialKeyID: "synthetic-key-v1"}, func() bool { return len(keyring.KeyIDs()) == 1 })
	if err != nil {
		t.Fatal(err)
	}
	tools, err := toolcompatibility.New(now)
	if err != nil {
		t.Fatal(err)
	}
	api := controlplane.NewHandlerWithDependencies(buildinfo.Info{}, controlplane.Dependencies{Identity: browser, Authorizer: database, SecurityPolicy: status, ToolCompatibility: tools})
	web := t.TempDir()
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte(`<meta name="ob-data-orch-csrf-token" content="local-mvp-csrf-v1">`), 0600); err != nil {
		t.Fatal(err)
	}
	handler, err := browser.Handler(api, web, web, web)
	if err != nil {
		t.Fatal(err)
	}
	read := func(path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "https://orch.example.test:8080"+path, nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v1/settings/security", "/api/v1/settings/tools"} {
		if read(path, nil).Code != http.StatusUnauthorized {
			t.Fatal("匿名读取未拒绝")
		}
	}
	form := url.Values{"username": {"admin"}, "password": {"synthetic-password-1234"}}
	r := httptest.NewRequest(http.MethodPost, "https://orch.example.test:8080/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || len(w.Result().Cookies()) != 1 {
		t.Fatal("合成认证未完成")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("会话保护缺失")
	}
	response := read("/api/v1/settings/security", cookie)
	var body struct {
		Item securitypolicy.Snapshot `json:"item"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Item.RootKeyStatus != "LOADED" || body.Item.CredentialKeyID != "synthetic-key-v1" {
		t.Fatal("认证及持久授权读取失败")
	}
	var toolBody struct {
		Item toolcompatibility.Snapshot `json:"item"`
	}
	response = read("/api/v1/settings/tools", cookie)
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &toolBody) != nil || len(toolBody.Item.Tools) != 2 || toolBody.Item.Tools[0].Modules[0].MetadataSelection != "TASK_CONFIGURATION" || len(toolBody.Item.Tools[1].Modules) != 2 {
		t.Fatal("实际目录未通过认证及持久授权返回")
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(databasePath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM subject_grants WHERE subject_id=? AND grant_code='ROLE_SYSTEM_ADMIN'`, deployment.SubjectID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/settings/security", "/api/v1/settings/tools"} {
		if response := read(path, cookie); response.Code != 404 || strings.Contains(response.Body.String(), "synthetic-key-v1") || strings.Contains(response.Body.String(), "obdumper-4.3.5") {
			t.Fatal("同一会话撤权后仍泄露安全或工具状态")
		}
	}
}
