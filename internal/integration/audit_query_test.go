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

	"ob-data-orch/internal/auditquery"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/deployment"
	"ob-data-orch/internal/store"
)

// Test审计实际会话CSRF与独立能力 使用合成口令、临时 SQLite 与真实浏览器会话验证完整传输边界。
func Test审计实际会话CSRF与独立能力(t *testing.T) {
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
	query, err := auditquery.New(database)
	if err != nil {
		t.Fatal(err)
	}
	api := controlplane.NewHandlerWithDependencies(buildinfo.Info{}, controlplane.Dependencies{Identity: browser, Authorizer: database, AuditQuery: query, CSRF: browser})
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
	f := url.Values{"from": {now.Add(-time.Minute).Format(time.RFC3339Nano)}, "to": {now.Add(time.Minute).Format(time.RFC3339Nano)}}
	queryPath := "/api/v1/logs/audit?" + f.Encode()
	if r := request("GET", queryPath, "", nil, ""); r.Code != 401 {
		t.Fatal("匿名审计读取", r.Code)
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
	if r := request("GET", queryPath, "", cookie, ""); r.Code != 404 {
		t.Fatal("系统管理员隐含获得审计", r.Code)
	}
	if r := request("GET", "/api/v1/logs/audit-access", "", cookie, ""); r.Code != 200 || !strings.Contains(r.Body.String(), `"allowed":false`) {
		t.Fatal("入口资格扩大权限", r.Code)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// 仅在隔离夹具预置显式授权，不修改现役主体，也不绕过自身只读的产品接口。
	if _, err = db.Exec(`INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES(?,'CAP_AUDIT_LOG',?,?)`, deployment.SubjectID, deployment.SubjectID, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	response := request("GET", queryPath, "", cookie, "")
	var page struct{ Item auditquery.Page }
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Item.Items) == 0 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("审计查询失败", response.Code)
	}
	for _, extra := range []string{"&from=2026-01-01T00:00:00Z", "&unknown=1", "&keyword=%zz", "&limit=0"} {
		if r := request("GET", queryPath+extra, "", cookie, ""); r.Code != 400 {
			t.Fatal("无效查询未拒绝", r.Code)
		}
	}
	detailPath := "/api/v1/logs/audit-detail?" + url.Values{"token": {page.Item.Items[0].Token}}.Encode()
	if r := request("GET", detailPath, "", cookie, ""); r.Code != 200 {
		t.Fatal("详情失败", r.Code)
	}
	html := request("GET", "/logs", "", cookie, "")
	matches := regexp.MustCompile(`name="ob-data-orch-csrf-token" content="([^"]+)"`).FindStringSubmatch(html.Body.String())
	if len(matches) != 2 || matches[1] == "local-mvp-csrf-v1" {
		t.Fatal("实际会话 CSRF 未注入")
	}
	body, _ := json.Marshal(map[string]string{"token": page.Item.SnapshotToken})
	if r := request("POST", "/api/v1/logs/audit:download", string(body), cookie, ""); r.Code != 401 {
		t.Fatal("未校验 CSRF", r.Code)
	}
	if r := request("POST", "/api/v1/logs/audit:download", string(body), cookie, matches[1]); r.Code != 200 || r.Header().Get("Content-Type") != "text/plain; charset=utf-8" || !strings.Contains(r.Body.String(), "已脱敏") {
		t.Fatal("下载失败", r.Code)
	}
	if r := request("POST", "/api/v1/logs/audit:download", `{"token":"x","path":"synthetic"}`, cookie, matches[1]); r.Code != 400 {
		t.Fatal("未知字段未拒绝", r.Code)
	}
	if _, err = db.Exec(`DELETE FROM subject_grants WHERE subject_id=? AND grant_code='CAP_AUDIT_LOG'`, deployment.SubjectID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{queryPath, detailPath} {
		if r := request("GET", path, "", cookie, ""); r.Code != 404 {
			t.Fatal("撤权后读取旧记录", r.Code)
		}
	}
	if r := request("POST", "/api/v1/logs/audit:download", string(body), cookie, matches[1]); r.Code != 404 {
		t.Fatal("撤权后下载旧快照", r.Code)
	}
	var n int
	if err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action IN ('AUDIT_LOG_QUERY','AUDIT_LOG_DETAIL','AUDIT_LOG_DOWNLOAD')`).Scan(&n); err != nil || n != 3 {
		t.Fatal("访问审计不完整", n, err)
	}
}
