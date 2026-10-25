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
	"regexp"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/deployment"
	"ob-data-orch/internal/logquery"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/store"
)

// Test日志中心实际登录会话与CSRF 使用临时 SQLite、受控分段和合成登录验证读取、下载及同会话撤权。
func Test日志中心实际登录会话与CSRF(t *testing.T) {
	database, path := openSyntheticStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := database.EnsureAuthSubject(ctx, store.AuthSubject{SubjectID: deployment.SubjectID, ExternalSubject: deployment.SubjectID, DisplayName: "合成管理员", AccountStatus: "ACTIVE", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := database.BootstrapPermissionOwner(ctx, deployment.SubjectID, now); err != nil {
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
	logs, err := logstream.NewPersistentStore(filepath.Join(t.TempDir(), "logs"), database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logs.Close() })
	query, err := logquery.New(database, logs)
	if err != nil {
		t.Fatal(err)
	}
	api := controlplane.NewHandlerWithDependencies(buildinfo.Info{}, controlplane.Dependencies{Identity: browser, Authorizer: database, LogQuery: query, CSRF: browser})
	web := t.TempDir()
	if err = os.WriteFile(filepath.Join(web, "index.html"), []byte(`<meta name="ob-data-orch-csrf-token" content="local-mvp-csrf-v1">`), 0600); err != nil {
		t.Fatal(err)
	}
	handler, err := browser.Handler(api, web, web, web)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://orch.example.test:8080"+path, strings.NewReader(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if method == "POST" {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", "https://orch.example.test:8080")
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	queryPath := "/api/v1/logs/tasks?from=2026-10-25T00:00:00Z&to=2026-10-25T01:00:00Z"
	if r := request("GET", queryPath, "", nil, ""); r.Code != 401 {
		t.Fatal("匿名读取被允许")
	}
	form := url.Values{"username": {"admin"}, "password": {"synthetic-password-1234"}}
	login := httptest.NewRequest("POST", "https://orch.example.test:8080/login", strings.NewReader(form.Encode()))
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, login)
	if out.Code != 303 || len(out.Result().Cookies()) != 1 {
		t.Fatal("合成登录失败")
	}
	cookie := out.Result().Cookies()[0]
	response := request("GET", queryPath, "", cookie, "")
	var page struct{ Item logquery.Page }
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Item.Availability != "NOT_COLLECTED" {
		t.Fatal("实际授权查询失败", response.Code)
	}
	html := request("GET", "/logs", "", cookie, "")
	matches := regexp.MustCompile(`name="ob-data-orch-csrf-token" content="([^"]+)"`).FindStringSubmatch(html.Body.String())
	if len(matches) != 2 || matches[1] == "local-mvp-csrf-v1" {
		t.Fatal("未注入实际会话 CSRF")
	}
	body, _ := json.Marshal(map[string]any{"token": page.Item.SnapshotToken, "fullTask": false})
	if r := request("POST", "/api/v1/logs/tasks:download", string(body), cookie, ""); r.Code != 401 {
		t.Fatal("缺失 CSRF 未拒绝", r.Code)
	}
	if r := request("POST", "/api/v1/logs/tasks:download", string(body), cookie, matches[1]); r.Code != 200 || r.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatal("当前会话下载失败", r.Code)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`DELETE FROM subject_grants WHERE subject_id=? AND grant_code IN ('ROLE_BASIC_USER','ROLE_OPERATOR')`, deployment.SubjectID); err != nil {
		t.Fatal(err)
	}
	if r := request("GET", queryPath, "", cookie, ""); r.Code != 404 {
		t.Fatal("同一会话撤权未立即生效", r.Code)
	}
	var auditCount int
	if err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action='TASK_LOG_DOWNLOAD'`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("下载未留审计", err)
	}
}
