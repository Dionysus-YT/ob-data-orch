package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/logstream"
)

func retentionFixture(t *testing.T, state string, finished *time.Time, reconciliation bool) (*Store, *logstream.PersistentStore, string, logstream.Batch) {
	t.Helper()
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	if err := database.SubmitTask(ctx, validTaskSubmission("retention-task")); err != nil {
		t.Fatal(err)
	}
	if err := database.ClaimTask(ctx, Claim{ExecutionID: "retention-execution", TaskID: "retention-task", NodeID: "node-1", AgentID: "agent-1", LeaseID: "retention-lease", LeaseEpoch: 1, IssuedAt: testTime, ExpiresAt: testTime.Add(time.Minute), EventID: "retention-event", RequestID: "retention-request"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "logs")
	persistent, err := logstream.NewPersistentStore(root, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persistent.Close() })
	batch, err := logstream.SealBatch(logstream.Batch{StreamID: "retention-source", SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, PolicyVersion: "synthetic-v1", Records: []logstream.Record{{StreamID: "retention-source", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 1, Kind: logstream.RecordLog, Message: "合成日志", PolicyVersion: "synthetic-v1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistent.Append(ctx, "retention-execution", batch); err != nil {
		t.Fatal(err)
	}
	var ended any
	if finished != nil {
		ended = utcText(*finished)
	}
	if _, err := database.db.Exec(`UPDATE task_executions SET state = ?, finished_at = ?, reconciliation_required = ? WHERE execution_id = 'retention-execution'`, state, ended, reconciliation); err != nil {
		t.Fatal(err)
	}
	return database, persistent, root, batch
}

func Test日志保留只清理已结束且无需核对的过期任务(t *testing.T) {
	now := testTime.AddDate(0, 0, 50)
	old, recent := now.AddDate(0, 0, -30), now.AddDate(0, 0, -29)
	for _, scenario := range []struct {
		state     string
		end       *time.Time
		reconcile bool
		cleanup   bool
	}{
		{"SUCCEEDED", &old, false, true}, {"FAILED", &old, false, true}, {"CANCELLED", &old, false, true},
		{"RUNNING", &old, false, false}, {"CANCELLING", &old, false, false}, {"STARTING", &old, false, false},
		{"FAILED", &old, true, false}, {"SUCCEEDED", nil, false, false}, {"SUCCEEDED", &recent, false, false},
	} {
		t.Run(scenario.state+timeValue(scenario.end)+boolLabel(scenario.reconcile), func(t *testing.T) {
			database, persistent, root, batch := retentionFixture(t, scenario.state, scenario.end, scenario.reconcile)
			if err := persistent.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			var auditBefore int
			var snapshotBefore string
			if err := database.db.QueryRow(`SELECT snapshot_json FROM tasks WHERE task_id = 'retention-task'`).Scan(&snapshotBefore); err != nil {
				t.Fatal(err)
			}
			if err := database.db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditBefore); err != nil {
				t.Fatal(err)
			}
			count, err := persistent.CleanupExpired(context.Background(), now, 30, 64)
			if err != nil {
				t.Fatal(err)
			}
			expired, err := database.TaskLogsExpired(context.Background(), "retention-task")
			if err != nil || expired != scenario.cleanup {
				t.Fatalf("过期事实=%v err=%v", expired, err)
			}
			if scenario.cleanup {
				if count != 1 {
					t.Fatalf("清理数量=%d", count)
				}
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 0 {
					t.Fatalf("剩余段数=%d err=%v", len(entries), err)
				}
				assertCount(t, database.db, `SELECT COUNT(*) FROM log_batches`, 0)
				assertCount(t, database.db, `SELECT COUNT(*) FROM log_segments`, 0)
				assertCount(t, database.db, `SELECT COUNT(*) FROM log_streams`, 0)
				if _, _, _, err := persistent.ReadPage(context.Background(), "retention-task", nil); !errors.Is(err, logstream.ErrRetentionExpired) {
					t.Fatalf("读取过期日志=%v", err)
				}
				if _, err := persistent.Append(context.Background(), "retention-execution", batch); !errors.Is(err, logstream.ErrRetentionExpired) {
					t.Fatalf("清理后重复上传=%v", err)
				}
			} else if count != 0 {
				t.Fatalf("受保护任务被清理=%d", count)
			}
			assertCount(t, database.db, `SELECT COUNT(*) FROM tasks`, 1)
			var snapshotAfter string
			if err := database.db.QueryRow(`SELECT snapshot_json FROM tasks WHERE task_id = 'retention-task'`).Scan(&snapshotAfter); err != nil || snapshotAfter != snapshotBefore {
				t.Fatalf("冻结配置被清理改写：%v", err)
			}
			assertCount(t, database.db, `SELECT COUNT(*) FROM task_executions`, 1)
			assertCount(t, database.db, `SELECT COUNT(*) FROM audit_events`, auditBefore)
		})
	}
}

func timeValue(value *time.Time) string {
	if value == nil {
		return "无结束时间"
	}
	return value.Format("20060102")
}
func boolLabel(value bool) string {
	if value {
		return "待核对"
	}
	return ""
}

func Test日志意外缺失不能登记为正常过期(t *testing.T) {
	old := testTime
	database, persistent, root, _ := retentionFixture(t, "SUCCEEDED", &old, false)
	if err := persistent.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 1 {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, files[0].Name())); err != nil {
		t.Fatal(err)
	}
	if _, err := persistent.CleanupExpired(context.Background(), testTime.AddDate(0, 0, 40), 30, 64); !errors.Is(err, logstream.ErrStorageCorrupt) {
		t.Fatalf("未知缺失=%v", err)
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM log_segment_cleanup`, 0)
	assertCount(t, database.db, `SELECT COUNT(*) FROM log_segments`, 1)
}

func Test日志清理在文件删除后崩溃可恢复且不改写历史(t *testing.T) {
	old := testTime
	database, persistent, root, _ := retentionFixture(t, "SUCCEEDED", &old, false)
	if err := persistent.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := testTime.AddDate(0, 0, 40)
	candidates, err := database.ListLogCleanup(context.Background(), now.AddDate(0, 0, -30), 64)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("候选=%v err=%v", candidates, err)
	}
	marked, err := database.BeginLogCleanup(context.Background(), candidates[0], now.AddDate(0, 0, -30), now)
	if err != nil || !marked {
		t.Fatalf("登记清理=%v err=%v", marked, err)
	}
	if err := os.Remove(filepath.Join(root, candidates[0].StorageKey)); err != nil {
		t.Fatal(err)
	}
	restarted, err := logstream.NewPersistentStore(root, database)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM log_segment_cleanup WHERE state = 'DELETED'`, 1)
	assertCount(t, database.db, `SELECT COUNT(*) FROM log_batches`, 0)
	if err := restarted.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`UPDATE log_segment_cleanup SET task_id = 'other'`); err == nil {
		t.Fatal("清理证据允许改写")
	}
	if _, err := database.db.Exec(`DELETE FROM log_segment_cleanup`); err == nil {
		t.Fatal("清理证据允许删除")
	}
}

func Test日志读取与保留清理互斥(t *testing.T) {
	old := testTime
	_, persistent, _, _ := retentionFixture(t, "SUCCEEDED", &old, false)
	var waiting sync.WaitGroup
	waiting.Add(1)
	go func() {
		defer waiting.Done()
		for i := 0; i < 30; i++ {
			if _, _, _, err := persistent.ReadPage(context.Background(), "retention-task", nil); err != nil && !errors.Is(err, logstream.ErrRetentionExpired) {
				t.Errorf("并发读取=%v", err)
				return
			}
		}
	}()
	if _, err := persistent.CleanupExpired(context.Background(), testTime.AddDate(0, 0, 40), 30, 64); err != nil {
		t.Fatal(err)
	}
	waiting.Wait()
}

func Test日志清理拒绝被篡改段和越界键且不伪造过期(t *testing.T) {
	for _, scenario := range []string{"内容篡改", "越界键"} {
		t.Run(scenario, func(t *testing.T) {
			old := testTime
			database, persistent, root, _ := retentionFixture(t, "SUCCEEDED", &old, false)
			protected := filepath.Join(filepath.Dir(root), "protected-synthetic")
			if err := os.WriteFile(protected, []byte("保留此合成文件"), 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "内容篡改" {
				if err := persistent.Shutdown(context.Background()); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 1 {
					t.Fatal("段夹具错误", err)
				}
				path := filepath.Join(root, entries[0].Name())
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				content[0] = '!'
				if err := os.WriteFile(path, content, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := database.db.Exec(`UPDATE log_segments SET storage_key='../protected-synthetic'`); err == nil {
					t.Fatal("数据库允许越界日志键")
				}
				if _, err := database.db.Exec(`UPDATE log_segments SET storage_key='synthetic-unindexed.open'`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := persistent.CleanupExpired(context.Background(), testTime.AddDate(0, 0, 40), 30, 64); !errors.Is(err, logstream.ErrStorageCorrupt) {
				t.Fatal("损坏或越界段没有阻断", err)
			}
			expired, err := database.TaskLogsExpired(context.Background(), "retention-task")
			if err != nil || expired {
				t.Fatal("损坏被伪装成保留期清理", err)
			}
			content, err := os.ReadFile(protected)
			if err != nil || string(content) != "保留此合成文件" {
				t.Fatal("删除越过日志根")
			}
		})
	}
}
