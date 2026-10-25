package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/logquery"
	"ob-data-orch/internal/logstream"
)

func grantLogQueryRole(t *testing.T, s *Store, subject, role string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES(?,?, 'subject-1',?)`, subject, role, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
}
func realLogQuery(t *testing.T) (*Store, *logstream.PersistentStore, *logquery.Service, logquery.Filter, string, logstream.Batch) {
	t.Helper()
	s, p, root, batch := retentionFixture(t, "RUNNING", nil, false)
	grantLogQueryRole(t, s, "subject-1", "ROLE_BASIC_USER")
	service, err := logquery.New(s, p)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return s, p, service, logquery.Filter{From: now.Add(-time.Hour), To: now.Add(time.Hour), Limit: 1}, root, batch
}

// Test日志中心真实分段查询隔离与固定快照 使用临时 SQLite 和脱敏文件覆盖跨主体、数据源运维及撤权。
func Test日志中心真实分段查询隔离与固定快照(t *testing.T) {
	s, p, service, f, _, first := realLogQuery(t)
	ctx := context.Background()
	second, err := logstream.SealBatch(logstream.Batch{StreamID: first.StreamID, SourceEpoch: 1, FirstSeq: 2, LastSeq: 3, PreviousDigest: first.Digest, PolicyVersion: "synthetic-v1", Records: []logstream.Record{
		{StreamID: first.StreamID, SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 2, Kind: logstream.RecordLog, Message: "password=****** safe continuation", PolicyVersion: "synthetic-v1"},
		{StreamID: first.StreamID, SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 3, Kind: logstream.RecordLog, Message: "safe output", PolicyVersion: "synthetic-v1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Append(ctx, "retention-execution", second); err != nil {
		t.Fatal(err)
	}
	page, err := service.Query(ctx, "subject-1", f, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Sequence != 3 || page.NextCursor == "" {
		t.Fatal("持久查询失败", err)
	}
	if _, err = s.db.Exec(`INSERT INTO auth_subjects VALUES('log-other','log-other','合成其他用户','ACTIVE',NULL,?,?)`, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	grantLogQueryRole(t, s, "log-other", "ROLE_BASIC_USER")
	other, err := service.Query(ctx, "log-other", f, "")
	if err != nil || len(other.Items) != 0 || len(other.Sources) != 0 || other.Expired {
		t.Fatal("无权元数据泄露", err)
	}
	exact := f
	exact.TaskID = "retention-task"
	if _, err = service.Query(ctx, "log-other", exact, ""); !errors.Is(err, logquery.ErrDenied) {
		t.Fatal("可枚举无权任务", err)
	}
	exact.TaskID = "not-found"
	if _, err = service.Query(ctx, "log-other", exact, ""); !errors.Is(err, logquery.ErrDenied) {
		t.Fatal("无权与不存在有差异", err)
	}
	grantLogQueryRole(t, s, "log-other", "ROLE_OPERATOR")
	if _, err = s.db.Exec(`INSERT INTO subject_object_scopes(subject_id,scope_type,object_id,granted_by,granted_at) VALUES('log-other','TASK_OPERATE_BY_DATA_SOURCE','source-1','subject-1',?)`, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	operator, err := service.Query(ctx, "log-other", f, "")
	if err != nil || len(operator.Items) != 1 {
		t.Fatal("数据源运维读取未生效", err)
	}
	if _, err = s.db.Exec(`DELETE FROM subject_object_scopes WHERE subject_id='log-other'`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Query(ctx, "log-other", f, operator.NextCursor); !errors.Is(err, logquery.ErrCursor) {
		t.Fatal("撤权游标仍有效", err)
	}
	data, err := service.Download(ctx, "subject-1", page.SnapshotToken, "log-download-1", false)
	if err != nil || !strings.Contains(string(data), "password=******") {
		t.Fatal("持久脱敏下载失败", err)
	}
	var audit string
	if err = s.db.QueryRow(`SELECT safe_diff_json FROM audit_events WHERE action='TASK_LOG_DOWNLOAD'`).Scan(&audit); err != nil || strings.Contains(audit, "continuation") || strings.Contains(audit, "password=") {
		t.Fatal("正文进入审计", err)
	}
	if err = p.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	detail, err := service.ReadContext(ctx, "subject-1", page.Items[0].Token)
	if err != nil || len(detail.Items) != 3 || detail.Items[0].Sequence != 1 {
		t.Fatal("封存后的同源读取失败", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM log_query_positions`, 2)
}

// Test日志中心损坏与保留清理分开表达 验证过期索引级联、损坏失败关闭且不伪造零条成功。
func Test日志中心损坏与保留清理分开表达(t *testing.T) {
	t.Run("损坏", func(t *testing.T) {
		_, _, service, f, root, _ := realLogQuery(t)
		files, err := os.ReadDir(root)
		if err != nil || len(files) == 0 {
			t.Fatal(err)
		}
		path := filepath.Join(root, files[0].Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data[len(data)/2] = '!'
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = service.Query(context.Background(), "subject-1", f, ""); !errors.Is(err, logquery.ErrUnavailable) {
			t.Fatal("损坏被当作无匹配", err)
		}
	})
	t.Run("过期", func(t *testing.T) {
		now := time.Now().UTC()
		old := now.AddDate(0, 0, -31)
		s, p, _, _ := retentionFixture(t, "SUCCEEDED", &old, false)
		grantLogQueryRole(t, s, "subject-1", "ROLE_BASIC_USER")
		if err := p.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := p.CleanupExpired(context.Background(), now, 30, 64); err != nil {
			t.Fatal(err)
		}
		assertCount(t, s.db, `SELECT COUNT(*) FROM log_query_positions`, 0)
		service, err := logquery.New(s, p)
		if err != nil {
			t.Fatal(err)
		}
		page, err := service.Query(context.Background(), "subject-1", logquery.Filter{TaskID: "retention-task", From: now.Add(-time.Hour), To: now.Add(time.Hour), Limit: 200}, "")
		if err != nil || page.Availability != "EXPIRED" || !page.Expired || len(page.Items) != 0 {
			t.Fatal("过期未独立表达", err)
		}
	})
}

// Test日志查询纳秒排序键与过滤边界 验证无小数和一至九位小数的顺序，不依赖毫秒精度日期函数。
func Test日志查询纳秒排序键与过滤边界(t *testing.T) {
	s, _ := openTestStore(t)
	for _, value := range []string{"2026-10-25T00:00:00Z", "2026-10-25T00:00:00.1Z", "2026-10-25T00:00:00.100000001Z", "2026-10-25T00:00:00.999999999Z"} {
		at, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			t.Fatal(err)
		}
		var key string
		if err = s.db.QueryRow(`SELECT `+logQueryTimeKey+` FROM (SELECT ? AS received_at) b`, value).Scan(&key); err != nil || key != timeQueryKey(at) {
			t.Fatal("时间键不一致", value, key, err)
		}
	}
}
