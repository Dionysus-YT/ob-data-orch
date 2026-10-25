package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/logstream"
)

func progressBatch(t *testing.T, stream string, seq int64, previous string, imported bool, amount string) logstream.Batch {
	t.Helper()
	record := logstream.Record{StreamID: stream, SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: seq, Kind: logstream.RecordLog, IntegrityCode: "COMPLETE", PolicyVersion: "synthetic-v1", ParserVersion: "direct-java-v1", Message: "2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 7 Running Tasks: 3 Progress: " + amount + "% ----------"}
	if imported {
		record.SourceKind, record.ParserVersion = logstream.SourceLoaderStdout, "obloader-4.3.5-v1"
	}
	record.Progress = logstream.ParseProgress(record)
	batch, err := logstream.SealBatch(logstream.Batch{StreamID: stream, SourceEpoch: 1, FirstSeq: seq, LastSeq: seq, PreviousDigest: previous, PolicyVersion: record.PolicyVersion, Records: []logstream.Record{record}})
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func Test进度批次同事务投影列表详情并隔离重复缺口重启和终态(t *testing.T) {
	for _, imported := range []bool{false, true} {
		name := "导出"
		if imported {
			name = "普通导入"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			database, logs, root, initial := retentionFixture(t, "RUNNING", nil, false)
			task, execution, stream, seq, previous := "retention-task", "retention-execution", "retention-source", int64(2), initial.Digest
			if imported {
				database, _ = reuseSourceFixture(t, "RUNNING")
				task, execution, stream, seq, previous = "reuse-task", "reuse-execution", "normal-progress", 1, ""
				root = filepath.Join(t.TempDir(), "import-logs")
				var err error
				logs, err = logstream.NewPersistentStore(root, database)
				if err != nil {
					t.Fatal(err)
				}
				defer logs.Close()
			}
			first := progressBatch(t, stream, seq, previous, imported, "63.25")
			if _, err := logs.Append(ctx, execution, first); err != nil {
				t.Fatal(err)
			}
			read := func() *TaskProgress {
				t.Helper()
				detail, err := database.GetAuthorizedTaskSummary(ctx, task, "subject-1")
				if err != nil || detail.Progress == nil || detail.State != "RUNNING" || detail.ResultSummary != nil {
					t.Fatal("进度修改了结果或未取得详情", detail.Progress, detail.State, err)
				}
				list, err := database.ListTaskSummaries(ctx, TaskListQuery{TaskID: task, SubjectID: "subject-1", Limit: 10})
				if err != nil || len(list) != 1 || list[0].Progress == nil || *list[0].Progress.Percent != *detail.Progress.Percent || list[0].Progress.Incomplete != detail.Progress.Incomplete {
					t.Fatal("列表与详情进度不一致", err)
				}
				return detail.Progress
			}
			firstProjection := read()
			if *firstProjection.Percent != 63.25 || *firstProjection.CompletedTasks != 7 {
				t.Fatal("工具进度口径被改写")
			}
			if _, err := database.GetAuthorizedTaskSummary(ctx, task, "foreign"); !errors.Is(err, ErrDataSourceNotFound) {
				t.Fatal("进度泄露到范围外", err)
			}
			if result, err := logs.Append(ctx, execution, first); err != nil || result.Decision != logstream.BatchDuplicate || !read().UpdatedAt.Equal(firstProjection.UpdatedAt) {
				t.Fatal("重放刷新了进度时间", err)
			}
			future := progressBatch(t, stream, seq+2, first.Digest, imported, "100.00")
			if _, err := logs.Append(ctx, execution, future); !errors.Is(err, logstream.ErrBatchGap) || *read().Percent != 63.25 {
				t.Fatal("缺号日志推进了进度", err)
			}
			gapKind, parser := logstream.SourceStdout, "direct-java-v1"
			if imported {
				gapKind, parser = logstream.SourceLoaderStdout, "obloader-4.3.5-v1"
			}
			if _, err := logs.AppendGap(ctx, execution, logstream.GapNotice{StreamID: stream, SourceKind: gapKind, SourceEpoch: 1, FirstSeq: seq + 1, LastSeq: seq + 1, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "synthetic-v1", ParserVersion: parser}); err != nil || !read().Incomplete {
				t.Fatal("缺口没有保留并标记末次进度", err)
			}
			if _, err := logs.Append(ctx, execution, future); err != nil || read().Incomplete || *read().Percent != 100 {
				t.Fatal("完整累计进度不能恢复证据", err)
			}
			if err := logs.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := logstream.NewPersistentStore(root, database)
			if err != nil || restarted.Recover(ctx) != nil {
				t.Fatal("含进度批次不能恢复", err)
			}
			defer restarted.Close()
			if _, err := restarted.Append(ctx, execution, first); err != nil || *read().Percent != 100 {
				t.Fatal("重启后重复旧批次倒退进度", err)
			}
			if _, err := database.db.Exec(`UPDATE task_executions SET state='FAILED',finished_at=? WHERE execution_id=?`, utcText(testTime.Add(time.Minute)), execution); err != nil {
				t.Fatal(err)
			}
			late := progressBatch(t, stream, seq+3, future.Digest, imported, "10.00")
			if _, err := restarted.Append(ctx, execution, late); err != nil {
				t.Fatal("终态尾部日志没有补存", err)
			}
			end, err := database.GetAuthorizedTaskSummary(ctx, task, "subject-1")
			if err != nil || end.State != "FAILED" || *end.Progress.Percent != 100 {
				t.Fatal("终态补传改写了结果或末次进度", err)
			}
			if imported && end.TaskType != importdomain.TaskType {
				t.Fatal("导入进度丢失任务类型")
			}
		})
	}
}

func Test进度索引写入失败时日志登记和摘要一起回滚(t *testing.T) {
	database, logs, _, initial := retentionFixture(t, "RUNNING", nil, false)
	if _, err := database.db.Exec(`CREATE TRIGGER synthetic_progress_failure BEFORE UPDATE OF progress_json ON task_executions BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	batch := progressBatch(t, "retention-source", 2, initial.Digest, false, "50.00")
	if _, err := logs.Append(context.Background(), "retention-execution", batch); err == nil {
		t.Fatal("没有验证故障回滚")
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM log_batches`, 1)
	assertCount(t, database.db, `SELECT COUNT(*) FROM task_executions WHERE progress_json IS NOT NULL`, 0)
	if _, err := database.db.Exec(`DROP TRIGGER synthetic_progress_failure`); err != nil {
		t.Fatal(err)
	}
	if _, err := logs.Append(context.Background(), "retention-execution", batch); err != nil {
		t.Fatal("回滚后不能重试同批次", err)
	}
}

func Test进度隔离跨流跨代跨工具且冲突和截断保留可信值(t *testing.T) {
	ctx := context.Background()
	database, logs, _, initial := retentionFixture(t, "RUNNING", nil, false)
	first := progressBatch(t, "retention-source", 2, initial.Digest, false, "63.25")
	if _, err := logs.Append(ctx, "retention-execution", first); err != nil {
		t.Fatal(err)
	}
	read := func(incomplete bool) *TaskProgress {
		t.Helper()
		detail, err := database.GetAuthorizedTaskSummary(ctx, "retention-task", "subject-1")
		if err != nil || detail.Progress == nil || detail.Progress.Percent == nil || *detail.Progress.Percent != 63.25 || detail.Progress.Incomplete != incomplete {
			t.Fatal("可信进度被替换或完整性错误", detail.Progress, err)
		}
		return detail.Progress
	}
	trusted := read(false)
	conflicting := progressBatch(t, "retention-source", 2, initial.Digest, false, "10.00")
	if _, err := logs.Append(ctx, "retention-execution", conflicting); !errors.Is(err, logstream.ErrBatchConflict) {
		t.Fatal("相同来源序号的异摘要批次未被拒绝", err)
	}
	read(false)
	foreign := []logstream.Batch{
		progressBatch(t, "other-progress-stream", 1, "", false, "99.00"),
		progressBatch(t, "retention-source", 1, "", false, "99.00"),
		progressBatch(t, "other-tool-stream", 1, "", true, "99.00"),
	}
	foreign[1].SourceEpoch, foreign[1].Records[0].SourceEpoch = 2, 2
	for _, batch := range foreign {
		sealed, err := logstream.SealBatch(batch)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := logs.Append(ctx, "retention-execution", sealed); err != nil {
			t.Fatal("独立来源的日志未被正常保存", err)
		}
		if !read(false).UpdatedAt.Equal(trusted.UpdatedAt) {
			t.Fatal("跨来源事实刷新了进度")
		}
	}
	truncated := progressBatch(t, "retention-source", 3, first.Digest, false, "90.00")
	truncated.Records[0].Kind, truncated.Records[0].IntegrityCode, truncated.Records[0].Progress = logstream.RecordTruncated, "TRUNCATED", nil
	truncated, err := logstream.SealBatch(truncated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logs.Append(ctx, "retention-execution", truncated); err != nil {
		t.Fatal(err)
	}
	if !read(true).UpdatedAt.Equal(trusted.UpdatedAt) {
		t.Fatal("截断改写了可信进度时间")
	}
}

func Test进度阶段无虚构数值且秘密拦截和取消后不推进摘要(t *testing.T) {
	ctx := context.Background()
	database, logs, _, initial := retentionFixture(t, "RUNNING", nil, false)
	batch := progressBatch(t, "retention-source", 2, initial.Digest, false, "50.00")
	batch.Records[0].Message = "2026-10-19 10:20:30 [INFO] Dump schema finished. Total Elapsed: 1 s"
	batch.Records[0].Progress = logstream.ParseProgress(batch.Records[0])
	batch, err := logstream.SealBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logs.Append(ctx, "retention-execution", batch); err != nil {
		t.Fatal(err)
	}
	read := func() *TaskProgress {
		t.Helper()
		detail, err := database.GetAuthorizedTaskSummary(ctx, "retention-task", "subject-1")
		if err != nil || detail.Progress == nil || detail.Progress.Phase != "EXPORT_SCHEMA_COMPLETE" || detail.Progress.Percent != nil || detail.Progress.CompletedTasks != nil || detail.Progress.RunningTasks != nil {
			t.Fatal("阶段事实虚构了数值", detail.Progress, err)
		}
		return detail.Progress
	}
	trusted := read()
	unsafe := progressBatch(t, "retention-source", 3, batch.Digest, false, "90.00")
	unsafe.Records[0].Message = "2026-10-19 10:20:30 [INFO] Dump record finished. Total Elapsed: token=synthetic-value"
	unsafe.Records[0].Progress = logstream.ParseProgress(unsafe.Records[0])
	unsafe, err = logstream.SealBatch(unsafe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logs.Append(ctx, "retention-execution", unsafe); !errors.Is(err, logstream.ErrPolicyRejected) {
		t.Fatal("结构化进度绕过了第二层秘密拦截", err)
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM log_batches`, 2)
	if !read().UpdatedAt.Equal(trusted.UpdatedAt) {
		t.Fatal("拒绝的正文推进了进度")
	}
	if _, err := database.db.Exec(`UPDATE task_executions SET state='CANCELLED',finished_at=? WHERE execution_id='retention-execution'`, utcText(testTime.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	late := progressBatch(t, "retention-source", 3, batch.Digest, false, "100.00")
	if _, err := logs.Append(ctx, "retention-execution", late); err != nil {
		t.Fatal("取消后的日志未补存", err)
	}
	if !read().UpdatedAt.Equal(trusted.UpdatedAt) {
		t.Fatal("取消后的日志改变了冻结进度")
	}
}

func Test无效固定摘要降级未知且核对中保留原进度(t *testing.T) {
	const raw = `{"version":"tool-progress-v1","streamId":"synthetic-stream","sourceEpoch":1,"sequence":1,"fact":{"phase":"EXPORT_DATA","bps":6325,"completed":7,"running":3},"updatedAt":"2026-10-19T00:00:00Z","incomplete":false}`
	if progress := projectTaskProgress(raw, "OBDUMPER_EXPORT", true); progress == nil || !progress.Incomplete || progress.Percent == nil || *progress.Percent != 63.25 {
		t.Fatal("核对中丢失了最后可信值", progress)
	}
	for _, invalid := range []string{
		"", "{}", strings.Replace(raw, "tool-progress-v1", "unknown", 1),
		strings.Replace(raw, `"sourceEpoch":1`, `"sourceEpoch":0`, 1),
		strings.Replace(raw, `"sequence":1`, `"sequence":0`, 1),
		strings.Replace(raw, "EXPORT_DATA", "IMPORT_DATA", 1),
		strings.Replace(raw, `"bps":6325`, `"bps":10001`, 1),
		strings.Replace(raw, `"completed":7`, `"completed":-1`, 1),
		strings.Replace(raw, `"running":3`, `"running":9007199254740992`, 1),
		strings.Replace(raw, "2026-10-19T00:00:00Z", "unknown", 1),
	} {
		if projectTaskProgress(invalid, "OBDUMPER_EXPORT", false) != nil {
			t.Fatal("无效固定摘要被认作可信进度")
		}
	}
	if projectTaskProgress(raw, "UNKNOWN", false) != nil || projectTaskProgress(raw, "OBLOADER_NORMAL_IMPORT", false) != nil {
		t.Fatal("摘要跨工具或未知任务类型生效")
	}
}
