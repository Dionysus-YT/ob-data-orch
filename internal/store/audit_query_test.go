package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/auditquery"
)

func addAuditFixture(t *testing.T, s *Store, id, kind, object string) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT','subject-1','DATA_SOURCE_UPDATED',?,?,'SUCCEEDED',?,'{"changed":true,"password":"synthetic-audit-secret"}',?)`, id, kind, object, "request-"+id, utcText(testTime))
	if err != nil {
		t.Fatal(err)
	}
}

// Test审计显式授予风险确认与撤销 验证管理员不隐含授权、新增确认及同会话撤权。
func Test审计显式授予风险确认与撤销(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	if allowed, err := s.AuditQueryAccess(ctx, "subject-1"); err != nil || allowed {
		t.Fatal("管理员被隐含授予审计", err)
	}
	c := permissionChange()
	c.Assignment.Capabilities = []accesscontrol.Capability{accesscontrol.AuditLog}
	if _, err := s.ChangeSubjectPermissions(ctx, c); !errors.Is(err, ErrPermissionConfirmation) {
		t.Fatal("新增审计能力不需确认", err)
	}
	c.Confirmed = true
	c.RequestID = "grant-audit"
	c.IdempotencyKey = "grant-audit"
	if _, err := s.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.AuditQueryAccess(ctx, c.SubjectID); err != nil || !allowed {
		t.Fatal("独立审计授权未生效", err)
	}
	addAuditFixture(t, s, "audit-visible", "DATA_SOURCE", "source-1")
	addAuditFixture(t, s, "audit-hidden", "DATA_SOURCE", "unscoped-source")
	addAuditFixture(t, s, "audit-node-hidden", "EXECUTION_NODE", "node-1")
	addAuditFixture(t, s, "audit-platform", "PLATFORM_SETTING", "execution-timing")
	service, _ := auditquery.New(s)
	f := auditquery.Filter{From: testTime.Add(-time.Minute), To: testTime.Add(time.Hour), Limit: 500}
	p, err := service.Query(ctx, c.SubjectID, "query-audit", f, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	for _, hidden := range []string{"audit-hidden", "unscoped-source", "audit-node-hidden", "synthetic-audit-secret"} {
		if strings.Contains(string(raw), hidden) {
			t.Fatal("无权对象或秘密进入投影")
		}
	}
	if !strings.Contains(string(raw), "audit-visible") || !strings.Contains(string(raw), "audit-platform") {
		t.Fatal("当前对象或平台审计缺失")
	}
	// 无权对象与不存在对象的精确查询使用相同空投影，不透露存在性。
	f.ObjectID = "unscoped-source"
	a, err := service.Query(ctx, c.SubjectID, "query-hidden", f, "")
	if err != nil || len(a.Items) != 0 {
		t.Fatal("发现无权对象", err)
	}
	f.ObjectID = "missing-source"
	b, err := service.Query(ctx, c.SubjectID, "query-missing", f, "")
	if err != nil || len(b.Items) != 0 || a.ScannedRecords != b.ScannedRecords {
		t.Fatal("对象存在性差异", err)
	}
	c.ExpectedRevision = 2
	c.Assignment.Capabilities = nil
	c.RequestID = "revoke-audit"
	c.IdempotencyKey = "revoke-audit"
	if _, err = s.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	if data, err := service.Download(ctx, c.SubjectID, "download-revoked", p.SnapshotToken); !errors.Is(err, auditquery.ErrDenied) || len(data) > 0 {
		t.Fatal("旧快照绕过撤权", err)
	}
	if _, err = service.Detail(ctx, c.SubjectID, "detail-revoked", p.Items[0].Token); !errors.Is(err, auditquery.ErrDenied) {
		t.Fatal("旧记录绕过撤权", err)
	}
}

// Test审计水位覆盖晚到与审计失败 验证触发器追加与业务同事务，晚到记录不进入固定水位。
func Test审计水位覆盖晚到与审计失败(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	c := permissionChange()
	c.Assignment.Capabilities = []accesscontrol.Capability{accesscontrol.AuditLog}
	c.Confirmed = true
	if _, err := s.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	addAuditFixture(t, s, "audit-z", "PLATFORM_SETTING", "execution-timing")
	addAuditFixture(t, s, "audit-y", "PLATFORM_SETTING", "execution-timing")
	service, _ := auditquery.New(s)
	f := auditquery.Filter{From: testTime.Add(-time.Minute), To: testTime.Add(time.Hour), Action: "DATA_SOURCE_UPDATED", Limit: 1}
	page, err := service.Query(ctx, c.SubjectID, "watermark-first", f, "")
	if err != nil || page.NextCursor == "" {
		t.Fatal(err)
	}
	addAuditFixture(t, s, "audit-x-late", "PLATFORM_SETTING", "execution-timing")
	next, err := service.Query(ctx, c.SubjectID, "watermark-next", f, page.NextCursor)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "audit-y" {
		t.Fatal("晚到污染分页", err)
	}
	var missing int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM audit_events a LEFT JOIN audit_query_positions pos ON pos.audit_event_id=a.audit_event_id WHERE pos.sequence IS NULL`).Scan(&missing); err != nil || missing != 0 {
		t.Fatal("审计水位遗漏事件", err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER synthetic_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='AUDIT_LOG_DOWNLOAD' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if data, err := service.Download(ctx, c.SubjectID, "download-audit-fail", page.SnapshotToken); err == nil || len(data) > 0 {
		t.Fatal("审计失败仍返回下载")
	}
	var raw string
	if err = s.db.QueryRow(`SELECT safe_diff_json FROM audit_events WHERE action='AUDIT_LOG_QUERY' LIMIT 1`).Scan(&raw); err != nil || strings.Contains(raw, "synthetic-audit-secret") {
		t.Fatal("访问审计复制正文", err)
	}
}

// Test审计扫描按UTF8字节计费 防止可信主体或请求标识包含中文时少计扫描预算。
func Test审计扫描按UTF8字节计费(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	c := permissionChange()
	c.Assignment.Capabilities = []accesscontrol.Capability{accesscontrol.AuditLog}
	c.Confirmed = true
	if _, err := s.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	const id, actor, action, object, request, diff = "unicode-audit", "合成主体", "LOG_RETENTION_CHANGED", "合成配置", "合成请求", `{"changed":true}`
	if _, err := s.db.Exec(`INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SYSTEM',?,?,'PLATFORM_SETTING',?,'SUCCEEDED',?,?,?)`, id, actor, action, object, request, diff, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	scope, err := s.AuditQueryScope(ctx, c.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.AuditQueryRows(ctx, auditquery.Window{Subject: c.SubjectID, Fingerprint: scope.Fingerprint, Watermark: scope.Watermark, Filter: auditquery.Filter{From: testTime.Add(-time.Minute), To: testTime.Add(time.Minute)}, EventID: id, Limit: 1})
	want := int64(len(id) + len(actor) + len(action) + len(object) + len(request) + len(diff) + 100)
	if err != nil || len(rows) != 1 || rows[0].Bytes != want {
		t.Fatal("UTF-8 扫描字节计数错误", err)
	}
}
