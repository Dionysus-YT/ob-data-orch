package logquery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/logstream"
)

type queryEntry struct {
	Candidate
	sequence int64
	records  []logstream.Record
}
type queryFixture struct {
	entries     []queryEntry
	fingerprint string
	denied      bool
	reads       int
	audits      []Audit
	auditErr    error
	readErr     error
	onRead      func()
	expired     bool
}

// LogQueryScope 返回合成授权指纹和快照水位。
func (f *queryFixture) LogQueryScope(_ context.Context, subject string, filter Filter) (Scope, error) {
	if f.denied || subject != "subject-1" || filter.TaskID == "hidden" {
		return Scope{}, ErrDenied
	}
	return Scope{Fingerprint: f.fingerprint, Watermark: int64(len(f.entries)), Retained: len(f.entries) > 0, Expired: f.expired, Sources: []string{"OBDUMPER_STDOUT"}}, nil
}

// LogQueryBatches 按测试窗口裁剪候选，不读取实际文件。
func (f *queryFixture) LogQueryBatches(_ context.Context, w Window) ([]Candidate, error) {
	if f.denied {
		return nil, ErrDenied
	}
	if f.fingerprint != w.Fingerprint {
		return nil, ErrCursor
	}
	items := []Candidate{}
	for _, e := range f.entries {
		if e.sequence > w.Watermark || w.Filter.TaskID != "" && e.TaskID != w.Filter.TaskID {
			continue
		}
		if w.StreamID != "" {
			if e.Batch.StreamID != w.StreamID || e.Batch.LastSequence < w.MinSeq || e.Batch.FirstSequence > w.MaxSeq {
				continue
			}
		} else {
			if !w.AllRetained && (e.Batch.ReceivedAt.Before(w.Filter.From) || !e.Batch.ReceivedAt.Before(w.Filter.To)) || w.BeforeKey != "" && (e.Key > w.BeforeKey || e.Key == w.BeforeKey && !w.Include) {
				continue
			}
		}
		items = append(items, e.Candidate)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key > items[j].Key })
	limit := 64
	if w.StreamID != "" {
		limit = 102
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// ReadIndexedBatch 提供合成记录并在读取边界触发故障或撤权。
func (f *queryFixture) ReadIndexedBatch(_ context.Context, _ string, b logstream.IndexedBatch) ([]logstream.Record, error) {
	f.reads++
	if f.onRead != nil {
		f.onRead()
	}
	if f.readErr != nil {
		return nil, f.readErr
	}
	for _, e := range f.entries {
		if e.Batch.BatchID == b.BatchID {
			return e.records, nil
		}
	}
	return nil, ErrUnavailable
}

// AuditLogQuery 记录合成输出审计并允许模拟审计故障。
func (f *queryFixture) AuditLogQuery(_ context.Context, a Audit) error {
	f.audits = append(f.audits, a)
	return f.auditErr
}
func newQueryFixture(t *testing.T, count int) (*Service, *queryFixture, Filter) {
	t.Helper()
	f := &queryFixture{fingerprint: "scope-v1"}
	now := time.Now().UTC()
	records := make([]logstream.Record, count)
	for i := range records {
		records[i] = logstream.Record{StreamID: "source-a", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: int64(i + 1), Kind: logstream.RecordLog, Message: fmt.Sprintf("line-%06d", i+1), ReceivedAt: now}
	}
	if count > 0 {
		f.entries = []queryEntry{{Candidate: Candidate{Key: "0001", TaskID: "task-1", Batch: logstream.IndexedBatch{BatchID: "batch-a", StreamID: strings.Repeat("a", 64), FirstSequence: 1, LastSequence: int64(count), ReceivedAt: now}}, sequence: 1, records: records}}
	}
	s, err := New(f, f)
	if err != nil {
		t.Fatal(err)
	}
	return s, f, Filter{From: now.Add(-time.Hour), To: now.Add(time.Second), Limit: 2}
}

// Test日志查询固定水位并绑定主体筛选与权限 验证晚到批次、篡改游标、跨主体和撤权均不能插入旧页。
func Test日志查询固定水位并绑定主体筛选与权限(t *testing.T) {
	s, f, filter := newQueryFixture(t, 3)
	ctx := context.Background()
	page, err := s.Query(ctx, "subject-1", filter, "")
	if err != nil || len(page.Items) != 2 || page.Items[0].Sequence != 3 || page.NextCursor == "" {
		t.Fatal("首个快照错误", err)
	}
	e := f.entries[0]
	e.Key = "0000"
	e.Batch.BatchID = "late-batch"
	e.sequence = 2
	e.records = []logstream.Record{e.records[0]}
	e.records[0].Message = "late"
	f.entries = append(f.entries, e)
	next, err := s.Query(ctx, "subject-1", filter, page.NextCursor)
	if err != nil || len(next.Items) != 1 || next.Items[0].Sequence != 1 || next.NextCursor != "" {
		t.Fatal("晚到日志改变旧快照", err)
	}
	back, err := s.Query(ctx, "subject-1", filter, page.PageCursor)
	if err != nil || len(back.Items) != 2 || !back.SnapshotAt.Equal(page.SnapshotAt) {
		t.Fatal("返回旧页未保持水位", err)
	}
	before := f.reads
	for _, subject := range []string{"subject-2", "subject-1"} {
		value := page.NextCursor
		if subject == "subject-1" {
			value = "x" + value
		}
		if _, err = s.Query(ctx, subject, filter, value); !errors.Is(err, ErrCursor) {
			t.Fatal("非法游标未拒绝", err)
		}
	}
	changed := filter
	changed.Keyword = "other"
	if _, err = s.Query(ctx, "subject-1", changed, page.NextCursor); !errors.Is(err, ErrCursor) {
		t.Fatal("游标可跨筛选", err)
	}
	f.fingerprint = "scope-v2"
	if _, err = s.Query(ctx, "subject-1", filter, page.NextCursor); !errors.Is(err, ErrCursor) {
		t.Fatal("撤权未作废快照", err)
	}
	if f.reads != before {
		t.Fatal("失效游标读取了正文")
	}
}

// Test日志查询输入限制与授权先于正文 覆盖跨对象长范围、非法字段和隐藏对象的相同拒绝。
func Test日志查询输入限制与授权先于正文(t *testing.T) {
	s, f, filter := newQueryFixture(t, 3)
	for _, mutate := range []func(*Filter){func(v *Filter) { v.From = v.To }, func(v *Filter) { v.From = v.To.Add(-25 * time.Hour) }, func(v *Filter) { v.Limit = 501 }, func(v *Filter) { v.Keyword = "x\x00" }, func(v *Filter) { v.Source = "FILE" }, func(v *Filter) { v.TaskID = "../x" }} {
		v := filter
		mutate(&v)
		if _, err := s.Query(context.Background(), "subject-1", v, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("无效条件被接受", err)
		}
	}
	filter.TaskID = "hidden"
	if _, err := s.Query(context.Background(), "subject-1", filter, ""); !errors.Is(err, ErrDenied) {
		t.Fatal("隐藏对象错误", err)
	}
	if f.reads != 0 {
		t.Fatal("未授权前读取正文")
	}
}

// Test日志上下文保持同源且批量复制先审计 验证窗口正序、相邻通道隔离及审计失败关闭。
func Test日志上下文保持同源且批量复制先审计(t *testing.T) {
	s, f, filter := newQueryFixture(t, 120)
	filter.Keyword = "line-000060"
	page, err := s.Query(context.Background(), "subject-1", filter, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(err)
	}
	detail, err := s.ReadContext(context.Background(), "subject-1", page.Items[0].Token)
	if err != nil || len(detail.Items) != 101 || detail.Items[0].Sequence != 10 || detail.Items[100].Sequence != 110 {
		t.Fatal("上下文窗口错误", err)
	}
	text, err := s.CopyContext(context.Background(), "subject-1", page.Items[0].Token, "copy-1")
	if err != nil || !strings.Contains(text, "line-000010") || len(f.audits) != 1 || f.audits[0].Count != 101 {
		t.Fatal("复制未留痕", err)
	}
	f.auditErr = ErrUnavailable
	if text, err = s.CopyContext(context.Background(), "subject-1", page.Items[0].Token, "copy-2"); err == nil || text != "" {
		t.Fatal("审计失败仍释放正文")
	}
	f.denied = true
	if _, err = s.ReadContext(context.Background(), "subject-1", page.Items[0].Token); !errors.Is(err, ErrDenied) {
		t.Fatal("撤权仍可读上下文", err)
	}
}

// Test日志查询扫描预算与超限下载不可静默截取 覆盖累计记录、字节边界及下载失败审计。
func Test日志查询扫描预算与超限下载不可静默截取(t *testing.T) {
	ctx := context.Background()
	s, f, filter := newQueryFixture(t, 50001)
	filter.Keyword = "no-match"
	page, err := s.Query(ctx, "subject-1", filter, "")
	if err != nil || !page.Truncated || page.ScannedRecords != 50000 || page.NextCursor != "" {
		t.Fatal("记录预算失效", err)
	}
	s, f, filter = newQueryFixture(t, 400)
	for i := range f.entries[0].records {
		f.entries[0].records[i].Message = strings.Repeat("z", 200000)
	}
	filter.Keyword = "no-match"
	page, err = s.Query(ctx, "subject-1", filter, "")
	if err != nil || !page.Truncated || page.ScannedBytes > queryBytes || page.ScannedRecords >= 400 {
		t.Fatal("字节预算失效", err)
	}
	s, f, filter = newQueryFixture(t, 100001)
	filter.Limit = 1
	page, err = s.Query(ctx, "subject-1", filter, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.Download(ctx, "subject-1", page.SnapshotToken, "download-too-many", false)
	if !errors.Is(err, ErrLimit) || data != nil || len(f.audits) != 1 || f.audits[0].Result != "FAILED" {
		t.Fatal("下载超限未失败关闭", err)
	}
}

// Test日志查询脱敏下载与响应前撤权失败关闭 验证过滤下载、全文保留模式、损坏错误遮蔽和会话过期。
func Test日志查询脱敏下载与响应前撤权失败关闭(t *testing.T) {
	s, f, filter := newQueryFixture(t, 4)
	ctx := context.Background()
	f.entries[0].records[1].Message = "safe password=******\ncontinuation"
	filter.TaskID = "task-1"
	filter.Keyword = "safe"
	page, err := s.Query(ctx, "subject-1", filter, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.Download(ctx, "subject-1", page.SnapshotToken, "download-1", false)
	if err != nil || !strings.Contains(string(data), "records=1") || !strings.Contains(string(data), "\n  continuation") || strings.Contains(string(data), "line-000004") {
		t.Fatal("筛选下载错误", err)
	}
	data, err = s.Download(ctx, "subject-1", page.SnapshotToken, "download-full", true)
	if err != nil || !strings.Contains(string(data), "records=4") {
		t.Fatal("完整保留任务下载错误", err)
	}
	f.readErr = errors.New("synthetic-private-path")
	if _, err = s.Query(ctx, "subject-1", filter, ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal("存储错误未遮蔽", err)
	}
	f.readErr = nil
	f.onRead = func() { f.denied = true }
	if _, err = s.Query(ctx, "subject-1", filter, ""); !errors.Is(err, ErrDenied) {
		t.Fatal("响应前未复验授权", err)
	}
	f.onRead = nil
	f.denied = false
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err = s.ReadContext(ctx, "subject-1", page.Items[0].Token); !errors.Is(err, ErrCursor) {
		t.Fatal("过期游标被接受", err)
	}
}
