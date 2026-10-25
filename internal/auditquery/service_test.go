package auditquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type testStore struct {
	events       []Event
	denied       bool
	fingerprint  string
	failAudit    bool
	failRows     bool
	revokeOnRead bool
	accesses     []Access
}

func (f *testStore) AuditQueryAccess(ctx context.Context, subject string) (bool, error) {
	_, err := f.AuditQueryScope(ctx, subject)
	return err == nil, err
}
func (f *testStore) AuditQueryScope(context.Context, string) (Scope, error) {
	if f.denied {
		return Scope{}, ErrDenied
	}
	return Scope{Fingerprint: f.fingerprint, Watermark: int64(len(f.events))}, nil
}
func (f *testStore) AuditQueryRows(_ context.Context, w Window) ([]Event, error) {
	if f.revokeOnRead {
		f.denied = true
	}
	if f.failRows {
		return nil, ErrUnavailable
	}
	var out []Event
	for n, e := range f.events {
		if int64(n+1) > w.Watermark || w.Before != "" && e.Key >= w.Before || w.EventID != "" && e.ID != w.EventID {
			continue
		}
		out = append(out, e)
	}
	// 合成端口刻意以同一排序返回，晚到事件只通过水位可见。
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Key > out[i].Key {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > w.Limit {
		out = out[:w.Limit]
	}
	return out, nil
}
func (f *testStore) AuditQueryAccessEvent(_ context.Context, a Access) error {
	if f.failAudit {
		return ErrUnavailable
	}
	f.accesses = append(f.accesses, a)
	return nil
}
func sampleEvent(n int) Event {
	return Event{Key: fmt.Sprintf("20260102030405000000000|event-%03d", n), ID: fmt.Sprintf("event-%03d", n), ActorType: "SUBJECT", ActorID: "synthetic-actor", Action: "DATA_SOURCE_UPDATED", ObjectType: "DATA_SOURCE", ObjectID: "source-1", Result: "SUCCEEDED", RequestID: fmt.Sprintf("request-%d", n), At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Diff: []byte(`{"changed":true,"revision":2,"password":"synthetic-audit-secret"}`), Bytes: 256}
}
func setup(t *testing.T) (*Service, *testStore, Filter) {
	t.Helper()
	f := &testStore{fingerprint: "one", events: []Event{sampleEvent(1), sampleEvent(2), sampleEvent(3)}}
	s, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC) }
	return s, f, Filter{From: s.now().Add(-time.Hour), To: s.now(), Limit: 2}
}

// Test审计快照固定水位且访问审计不含正文 覆盖翻页、详情、下载、晚到、主体和令牌用途隔离。
func Test审计快照固定水位且访问审计不含正文(t *testing.T) {
	s, f, filter := setup(t)
	ctx := context.Background()
	page, err := s.Query(ctx, "reader", "query-1", filter, "")
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != "event-003" || page.NextCursor == "" {
		t.Fatal("首屏失败", err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "synthetic-audit-secret") || strings.Contains(string(raw), "password") || !page.Items[0].DetailsOmitted {
		t.Fatal("秘密差异进入页面")
	}
	f.events = append(f.events, sampleEvent(4))
	next, err := s.Query(ctx, "reader", "query-2", filter, page.NextCursor)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "event-001" {
		t.Fatal("晚到记录污染快照", err)
	}
	detail, err := s.Detail(ctx, "reader", "detail-1", page.Items[0].Token)
	if err != nil || detail.ID != page.Items[0].ID {
		t.Fatal("详情失败", err)
	}
	file, err := s.Download(ctx, "reader", "download-1", page.SnapshotToken)
	if err != nil || strings.Contains(string(file), "synthetic-audit-secret") || strings.Contains(string(file), "event-004") || !strings.Contains(string(file), "已脱敏") {
		t.Fatal("下载越过投影或水位", err)
	}
	for _, token := range []string{page.SnapshotToken, page.Items[0].Token, page.NextCursor + "x"} {
		if _, err := s.Query(ctx, "reader", "query-wrong", filter, token); !errors.Is(err, ErrCursor) {
			t.Fatal("用途或损坏未拒绝", err)
		}
	}
	if _, err := s.Detail(ctx, "other", "detail-other", page.Items[0].Token); !errors.Is(err, ErrCursor) {
		t.Fatal("跨主体读取", err)
	}
	filter.Keyword = "synthetic-audit-secret"
	empty, err := s.Query(ctx, "reader", "query-secret", filter, "")
	if err != nil || len(empty.Items) != 0 {
		t.Fatal("关键字检索了未知差异", err)
	}
	if len(f.accesses) != 5 {
		t.Fatal("查询、详情及下载未审计", len(f.accesses))
	}
}

// Test审计失败关闭与当前撤权 覆盖读失败后撤权、审计失败、授权变化、重启和到期。
func Test审计失败关闭与当前撤权(t *testing.T) {
	for _, mode := range []string{"audit", "read-revoke", "fingerprint", "restart", "expire", "invalid-projection"} {
		t.Run(mode, func(t *testing.T) {
			s, f, filter := setup(t)
			p, err := s.Query(context.Background(), "reader", "first", filter, "")
			if err != nil {
				t.Fatal(err)
			}
			expected := ErrUnavailable
			switch mode {
			case "audit":
				f.failAudit = true
			case "read-revoke":
				f.failRows = true
				f.revokeOnRead = true
				expected = ErrDenied
			case "fingerprint":
				f.fingerprint = "two"
				expected = ErrCursor
			case "restart":
				s, _ = New(f)
				expected = ErrCursor
			case "expire":
				old := s.now()
				s.now = func() time.Time { return old.Add(2 * time.Hour) }
				expected = ErrCursor
			case "invalid-projection":
				f.events[0].Diff = []byte(`{"revision":"synthetic-audit-secret"}`)
			}
			data, err := s.Download(context.Background(), "reader", "download", p.SnapshotToken)
			if !errors.Is(err, expected) || len(data) > 0 {
				t.Fatal("失败返回了审计内容", err)
			}
		})
	}
}

// Test无权请求不区分令牌形态 验证权限拒绝先于合法、过期或损坏令牌的差异。
func Test无权请求不区分令牌形态(t *testing.T) {
	s, f, filter := setup(t)
	p, err := s.Query(context.Background(), "reader", "first", filter, "")
	if err != nil {
		t.Fatal(err)
	}
	f.denied = true
	for _, token := range []string{p.SnapshotToken, p.Items[0].Token, "invalid-token", ""} {
		if data, err := s.Download(context.Background(), "reader", "download", token); !errors.Is(err, ErrDenied) || len(data) > 0 {
			t.Fatal("无权下载区分令牌形态", err)
		}
		if _, err := s.Detail(context.Background(), "reader", "detail", token); !errors.Is(err, ErrDenied) {
			t.Fatal("无权详情区分令牌形态", err)
		}
	}
}

// Test审计预算与输入边界 验证扫描上限明确截断、下载拒绝部分文件和字面过滤输入限制。
func Test审计预算与输入边界(t *testing.T) {
	s, f, filter := setup(t)
	f.events[2].Bytes = scanBytes + 1
	p, err := s.Query(context.Background(), "reader", "query", filter, "")
	if err != nil || !p.Truncated || p.NextCursor != "" {
		t.Fatal("预算未截断", err)
	}
	if data, err := s.Download(context.Background(), "reader", "download", p.SnapshotToken); !errors.Is(err, ErrLimit) || len(data) > 0 {
		t.Fatal("返回了部分下载", err)
	}
	for _, change := range []func(*Filter){func(f *Filter) { f.To = f.From.Add(25 * time.Hour) }, func(f *Filter) { f.ActorID = "*" }, func(f *Filter) { f.Action = "a' OR 1=1" }, func(f *Filter) { f.Keyword = "line\n" }, func(f *Filter) { f.ObjectType = "UNKNOWN" }, func(f *Filter) { f.Limit = 501 }} {
		bad := filter
		change(&bad)
		if _, err := s.Query(context.Background(), "reader", "invalid", bad, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("无效输入被允许", err)
		}
	}
}
