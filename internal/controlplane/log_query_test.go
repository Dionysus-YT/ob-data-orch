package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/logquery"
	"ob-data-orch/internal/logstream"
)

type logQueryPort struct {
	denied bool
	reads  int
	audits int
}

// LogQueryScope 返回合成授权指纹和快照水位。
func (p *logQueryPort) LogQueryScope(context.Context, string, logquery.Filter) (logquery.Scope, error) {
	if p.denied {
		return logquery.Scope{}, logquery.ErrDenied
	}
	return logquery.Scope{Fingerprint: "scope", Sources: []string{}}, nil
}

// LogQueryBatches 按测试窗口裁剪候选，不读取实际文件。
func (p *logQueryPort) LogQueryBatches(context.Context, logquery.Window) ([]logquery.Candidate, error) {
	return []logquery.Candidate{}, nil
}

// AuditLogQuery 记录合成输出审计并允许模拟审计故障。
func (p *logQueryPort) AuditLogQuery(context.Context, logquery.Audit) error { p.audits++; return nil }

// ReadIndexedBatch 提供合成记录并在读取边界触发故障或撤权。
func (p *logQueryPort) ReadIndexedBatch(context.Context, string, logstream.IndexedBatch) ([]logstream.Record, error) {
	p.reads++
	return nil, logquery.ErrUnavailable
}
func logQueryRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, req)
	return out
}

// Test日志中心HTTP隔离与严格字段 验证浏览器认证、领域授权、未知条件和下载 CSRF 失败关闭。
func Test日志中心HTTP隔离与严格字段(t *testing.T) {
	p := &logQueryPort{}
	service, err := logquery.New(p, p)
	if err != nil {
		t.Fatal(err)
	}
	deps := Dependencies{Identity: permissionProvider{"subject-1"}, LogQuery: service, CSRF: allowedCSRF{}}
	handler := NewHandlerWithDependencies(buildinfo.Info{}, deps)
	path := "/api/v1/logs/tasks?from=2026-10-25T00:00:00Z&to=2026-10-25T01:00:00Z"
	response := logQueryRequest(handler, "GET", path, "")
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"availability":"NOT_COLLECTED"`) {
		t.Fatal("初始持久状态错误", response.Code)
	}
	for _, suffix := range []string{"&path=synthetic", "&%zz", "&limit=0", "&limit=501", "&from=2026-10-25T00:00:00Z", "&source=AGENT"} {
		if r := logQueryRequest(handler, "GET", path+suffix, ""); r.Code != 400 {
			t.Fatal("非法查询被接受", r.Code)
		}
	}
	p.denied = true
	if r := logQueryRequest(handler, "GET", path, ""); r.Code != 404 || strings.Contains(r.Body.String(), "scope") {
		t.Fatal("无权状态被发现")
	}
	p.denied = false
	var page struct {
		Item struct {
			Token string `json:"snapshotToken"`
		}
	}
	if err = json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"token":"x"}`, `{"token":"x","fullTask":false,"path":"x"}`, `{"token":"x","fullTask":false} {}`} {
		if r := logQueryRequest(handler, "POST", "/api/v1/logs/tasks:download", body); r.Code != 400 {
			t.Fatal("未知下载字段被接受", r.Code)
		}
	}
	body := `{"token":"` + page.Item.Token + `","fullTask":false}`
	r := logQueryRequest(handler, "POST", "/api/v1/logs/tasks:download", body)
	if r.Code != 200 || r.Header().Get("Content-Type") != "text/plain; charset=utf-8" || !strings.Contains(r.Body.String(), "records=0") || p.audits != 1 {
		t.Fatal("下载类型或审计错误", r.Code, p.audits)
	}
	deps.CSRF = rejectedPermissionCSRF{}
	handler = NewHandlerWithDependencies(buildinfo.Info{}, deps)
	if r = logQueryRequest(handler, "POST", "/api/v1/logs/tasks:download", body); r.Code != 401 || p.audits != 1 {
		t.Fatal("CSRF 失败仍进入下载")
	}
	deps.Identity = nil
	handler = NewHandlerWithDependencies(buildinfo.Info{}, deps)
	if r = logQueryRequest(handler, "GET", path, ""); r.Code == 200 {
		t.Fatal("认证缺失放行")
	}
	if p.reads != 0 {
		t.Fatal("无对象查询读取了正文")
	}
}
