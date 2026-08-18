package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/logstream"
)

const testFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var testTime = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

func TestOpenConfiguresSchemaAndCreatesVerifiedBackup(t *testing.T) {
	store, databasePath := openTestStore(t)
	seedBaseFixture(t, store)
	if err := store.SubmitTask(context.Background(), validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}

	var foreignKeys, busyTimeout, synchronous int
	var journalMode string
	if err := store.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if err := store.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if err := store.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("read synchronous: %v", err)
	}
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if foreignKeys != 1 || busyTimeout != 5000 || synchronous != 2 || !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("unexpected pragmas foreign_keys=%d busy_timeout=%d synchronous=%d journal_mode=%s", foreignKeys, busyTimeout, synchronous, journalMode)
	}

	backupPath := filepath.Join(filepath.Dir(databasePath), "metadata-backup.db")
	if err := store.Backup(context.Background(), backupPath); err != nil {
		t.Fatalf("Backup(): %v", err)
	}
	if err := store.Backup(context.Background(), backupPath); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Backup() overwrite error = %v", err)
	}
	backup, err := sql.Open("sqlite", sqliteReadOnlyDSN(backupPath))
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = backup.Close() })
	var taskCount int
	if err := backup.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&taskCount); err != nil {
		t.Fatalf("count backup tasks: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("backup task count = %d, want 1", taskCount)
	}
	restored, err := Open(context.Background(), backupPath)
	if err != nil {
		t.Fatalf("open restored backup: %v", err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	assertCount(t, restored.db, "SELECT COUNT(*) FROM tasks", 1)
}

func Test持久任务日志使用分段文件和稳定游标分页(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	if err := database.SubmitTask(ctx, validTaskSubmission("task-log-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	if err := database.ClaimTask(ctx, Claim{
		ExecutionID: "execution-log-1", TaskID: "task-log-1", NodeID: "node-1", AgentID: "agent-1",
		LeaseID: "lease-log-1", LeaseEpoch: 1, IssuedAt: testTime, ExpiresAt: testTime.Add(time.Minute),
		EventID: "event-log-1", RequestID: "request-log-1",
	}); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	logRoot := filepath.Join(t.TempDir(), "logs")
	persistent, err := logstream.NewPersistentStore(logRoot, database)
	if err != nil {
		t.Fatalf("NewPersistentStore(): %v", err)
	}
	t.Cleanup(func() { _ = persistent.Close() })
	records := make([]logstream.Record, 0, 201)
	for sequence := int64(1); sequence <= 201; sequence++ {
		records = append(records, logstream.Record{
			StreamID: "stream-log-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: sequence,
			Kind: logstream.RecordLog, Message: "safe log", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1",
		})
	}
	batch, err := logstream.SealBatch(logstream.Batch{
		StreamID: "stream-log-1", SourceEpoch: 1, FirstSeq: 1, LastSeq: 201, PolicyVersion: "synthetic-v1", Records: records,
	})
	if err != nil {
		t.Fatalf("SealBatch(): %v", err)
	}
	if result, err := persistent.Append(ctx, "execution-log-1", batch); err != nil || result.Decision != logstream.BatchAccepted || result.ExpectedSeq != 202 {
		t.Fatalf("Append() = %#v, %v", result, err)
	}
	if result, err := persistent.Append(ctx, "execution-log-1", batch); err != nil || result.Decision != logstream.BatchDuplicate {
		t.Fatalf("Append(duplicate) = %#v, %v", result, err)
	}
	first, firstNext, last, err := persistent.ReadPage(ctx, "task-log-1", nil)
	if err != nil || len(first) != 200 || firstNext == nil || last == nil || first[0].SourceSeq != 1 || first[199].SourceSeq != 200 {
		t.Fatalf("ReadPage(first) records=%d next=%#v last=%#v err=%v", len(first), firstNext, last, err)
	}
	second, next, last, err := persistent.ReadPage(ctx, "task-log-1", firstNext)
	if err != nil || len(second) != 1 || second[0].SourceSeq != 201 || next != nil || last == nil {
		t.Fatalf("ReadPage(second) records=%#v next=%#v last=%#v err=%v", second, next, last, err)
	}
	streamRecord, streamCursor, found, err := persistent.ReadNextSince(ctx, "task-log-1", firstNext)
	if err != nil || !found || streamCursor == nil || streamRecord.SourceSeq != 201 {
		t.Fatalf("ReadNextSince() record=%#v cursor=%#v found=%v err=%v", streamRecord, streamCursor, found, err)
	}
	var indexedRecords int
	if err := database.db.QueryRowContext(ctx, `SELECT record_count FROM log_batches`).Scan(&indexedRecords); err != nil || indexedRecords != 201 {
		t.Fatalf("indexed record_count=%d err=%v", indexedRecords, err)
	}
	var plaintextCount int
	if err := database.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE sql LIKE '%safe log%'`).Scan(&plaintextCount); err != nil || plaintextCount != 0 {
		t.Fatalf("SQLite schema unexpectedly contains log body count=%d err=%v", plaintextCount, err)
	}
	var segmentID string
	if err := database.db.QueryRowContext(ctx, `SELECT segment_id FROM log_segments WHERE state = 'OPEN'`).Scan(&segmentID); err != nil {
		t.Fatalf("读取活动段标识失败: %v", err)
	}
	if err := persistent.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	segmentPath := filepath.Join(logRoot, segmentID+".open")
	segmentData, err := os.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("读取活动段用于篡改验证失败: %v", err)
	}
	modifiedData := bytes.Replace(segmentData, []byte("safe log"), []byte("safe loG"), 1)
	if bytes.Equal(modifiedData, segmentData) {
		t.Fatal("未能构造等长段内容篡改")
	}
	if err := os.WriteFile(segmentPath, modifiedData, 0o600); err != nil {
		t.Fatalf("写入等长段内容篡改失败: %v", err)
	}
	restarted, err := logstream.NewPersistentStore(logRoot, database)
	if err != nil {
		t.Fatalf("NewPersistentStore(restarted): %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	if err := restarted.Recover(ctx); !errors.Is(err, logstream.ErrStorageCorrupt) {
		t.Fatalf("Recover(等长篡改) error = %v", err)
	}
}

func Test持久日志缺口保留来源身份并保持正常摘要链(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	if err := database.SubmitTask(ctx, validTaskSubmission("task-log-gap-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	if err := database.ClaimTask(ctx, Claim{ExecutionID: "execution-log-gap-1", TaskID: "task-log-gap-1", NodeID: "node-1", AgentID: "agent-1", LeaseID: "lease-log-gap-1", LeaseEpoch: 1, IssuedAt: testTime, ExpiresAt: testTime.Add(time.Minute), EventID: "event-log-gap-1", RequestID: "request-log-gap-1"}); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	persistent, err := logstream.NewPersistentStore(filepath.Join(t.TempDir(), "logs"), database)
	if err != nil {
		t.Fatalf("NewPersistentStore(): %v", err)
	}
	t.Cleanup(func() { _ = persistent.Close() })
	first, err := logstream.SealBatch(logstream.Batch{
		StreamID: "stream-log-gap-1", SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, PolicyVersion: "synthetic-v1",
		Records: []logstream.Record{{StreamID: "stream-log-gap-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 1, Kind: logstream.RecordLog, Message: "safe before gap", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"}},
	})
	if err != nil {
		t.Fatalf("SealBatch(first): %v", err)
	}
	if result, err := persistent.Append(ctx, "execution-log-gap-1", first); err != nil || result.Decision != logstream.BatchAccepted {
		t.Fatalf("Append(first) = %#v, %v", result, err)
	}
	gap := logstream.GapNotice{StreamID: "stream-log-gap-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, FirstSeq: 2, LastSeq: 3, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"}
	if result, err := persistent.AppendGap(ctx, "execution-log-gap-1", gap); err != nil || result.Decision != logstream.BatchAccepted || result.ExpectedSeq != 4 || result.LastDigest != first.Digest {
		t.Fatalf("AppendGap() = %#v, %v", result, err)
	}
	afterGap, err := logstream.SealBatch(logstream.Batch{
		StreamID: "stream-log-gap-1", SourceEpoch: 1, FirstSeq: 4, LastSeq: 4, PreviousDigest: first.Digest, PolicyVersion: "synthetic-v1",
		Records: []logstream.Record{{StreamID: "stream-log-gap-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 4, Kind: logstream.RecordLog, Message: "safe after gap", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"}},
	})
	if err != nil {
		t.Fatalf("SealBatch(after gap): %v", err)
	}
	if result, err := persistent.Append(ctx, "execution-log-gap-1", afterGap); err != nil || result.Decision != logstream.BatchAccepted || result.LastDigest != afterGap.Digest {
		t.Fatalf("Append(after gap) = %#v, %v", result, err)
	}
	records, next, last, err := persistent.ReadPage(ctx, "task-log-gap-1", nil)
	if err != nil || next != nil || last == nil || len(records) != 3 || records[1].Kind != logstream.RecordGap || records[1].SourceKind != logstream.SourceStdout || records[1].IntegrityCode != "LOCAL_SPOOL_LIMIT" {
		t.Fatalf("ReadPage() records=%#v next=%#v last=%#v err=%v", records, next, last, err)
	}
	var integrityStatus, gapSummary string
	if err := database.db.QueryRowContext(ctx, `SELECT integrity_status FROM log_streams`).Scan(&integrityStatus); err != nil || integrityStatus != "GAPPED" {
		t.Fatalf("日志流完整性状态=%q err=%v", integrityStatus, err)
	}
	if err := database.db.QueryRowContext(ctx, `SELECT gap_summary_json FROM log_batches WHERE first_sequence = 2`).Scan(&gapSummary); err != nil || gapSummary != `{"reasonCode":"LOCAL_SPOOL_LIMIT"}` {
		t.Fatalf("缺口摘要=%q err=%v", gapSummary, err)
	}
}

func Test持久日志达到段阈值后封存并新建活动段(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	if err := database.SubmitTask(ctx, validTaskSubmission("task-log-seal-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	if err := database.ClaimTask(ctx, Claim{ExecutionID: "execution-log-seal-1", TaskID: "task-log-seal-1", NodeID: "node-1", AgentID: "agent-1", LeaseID: "lease-log-seal-1", LeaseEpoch: 1, IssuedAt: testTime, ExpiresAt: testTime.Add(time.Minute), EventID: "event-log-seal-1", RequestID: "request-log-seal-1"}); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	logRoot := filepath.Join(t.TempDir(), "logs")
	persistent, err := logstream.NewPersistentStore(logRoot, database)
	if err != nil {
		t.Fatalf("NewPersistentStore(): %v", err)
	}
	t.Cleanup(func() { _ = persistent.Close() })
	message := strings.Repeat("x", 48<<10)
	previousDigest := ""
	lastSequence := int64(0)
	for batchNumber := 0; batchNumber < 18; batchNumber++ {
		records := make([]logstream.Record, 0, 10)
		for index := 0; index < 10; index++ {
			sequence := lastSequence + int64(index) + 1
			records = append(records, logstream.Record{StreamID: "stream-log-seal-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: sequence, Kind: logstream.RecordLog, Message: message, PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"})
		}
		batch, err := logstream.SealBatch(logstream.Batch{StreamID: "stream-log-seal-1", SourceEpoch: 1, FirstSeq: lastSequence + 1, LastSeq: lastSequence + int64(len(records)), PreviousDigest: previousDigest, PolicyVersion: "synthetic-v1", Records: records})
		if err != nil {
			t.Fatalf("SealBatch(%d): %v", batchNumber, err)
		}
		if result, err := persistent.Append(ctx, "execution-log-seal-1", batch); err != nil || result.Decision != logstream.BatchAccepted {
			t.Fatalf("Append(%d) = %#v, %v", batchNumber, result, err)
		}
		previousDigest, lastSequence = batch.Digest, batch.LastSeq
	}
	var segmentID, storageKey, digest string
	var byteLength int64
	if err := database.db.QueryRowContext(ctx, `SELECT segment_id, storage_key, byte_length, content_digest FROM log_segments WHERE state = 'SEALED'`).Scan(&segmentID, &storageKey, &byteLength, &digest); err != nil || storageKey != segmentID+".jsonl" || byteLength < 8<<20 || len(digest) != 64 {
		t.Fatalf("封存段元数据 id=%q key=%q bytes=%d digest=%q err=%v", segmentID, storageKey, byteLength, digest, err)
	}
	if _, err := os.Stat(filepath.Join(logRoot, segmentID+".jsonl")); err != nil {
		t.Fatalf("封存段文件不可读: %v", err)
	}
	nextBatch, err := logstream.SealBatch(logstream.Batch{StreamID: "stream-log-seal-1", SourceEpoch: 1, FirstSeq: lastSequence + 1, LastSeq: lastSequence + 1, PreviousDigest: previousDigest, PolicyVersion: "synthetic-v1", Records: []logstream.Record{{StreamID: "stream-log-seal-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: lastSequence + 1, Kind: logstream.RecordLog, Message: "safe open segment", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"}}})
	if err != nil {
		t.Fatalf("SealBatch(next): %v", err)
	}
	if result, err := persistent.Append(ctx, "execution-log-seal-1", nextBatch); err != nil || result.Decision != logstream.BatchAccepted {
		t.Fatalf("Append(next) = %#v, %v", result, err)
	}
	var openSegments int
	if err := database.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM log_segments WHERE state = 'OPEN'`).Scan(&openSegments); err != nil || openSegments != 1 {
		t.Fatalf("活动段数量=%d err=%v", openSegments, err)
	}
	records, _, _, err := persistent.ReadPage(ctx, "task-log-seal-1", nil)
	if err != nil || len(records) != 181 || records[0].Message != message || records[len(records)-1].Message != "safe open segment" {
		t.Fatalf("ReadPage() records=%d err=%v", len(records), err)
	}
}

func Test持久日志重启后截断未登记尾部并继续追加(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	if err := database.SubmitTask(ctx, validTaskSubmission("task-log-recover-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	if err := database.ClaimTask(ctx, Claim{ExecutionID: "execution-log-recover-1", TaskID: "task-log-recover-1", NodeID: "node-1", AgentID: "agent-1", LeaseID: "lease-log-recover-1", LeaseEpoch: 1, IssuedAt: testTime, ExpiresAt: testTime.Add(time.Minute), EventID: "event-log-recover-1", RequestID: "request-log-recover-1"}); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	logRoot := filepath.Join(t.TempDir(), "logs")
	persistent, err := logstream.NewPersistentStore(logRoot, database)
	if err != nil {
		t.Fatalf("NewPersistentStore(): %v", err)
	}
	first, err := logstream.SealBatch(logstream.Batch{StreamID: "stream-log-recover-1", SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, PolicyVersion: "synthetic-v1", Records: []logstream.Record{{StreamID: "stream-log-recover-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 1, Kind: logstream.RecordLog, Message: "safe before restart", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"}}})
	if err != nil {
		t.Fatalf("SealBatch(first): %v", err)
	}
	if result, err := persistent.Append(ctx, "execution-log-recover-1", first); err != nil || result.Decision != logstream.BatchAccepted {
		t.Fatalf("Append(first) = %#v, %v", result, err)
	}
	var segmentID string
	var indexedLength int64
	if err := database.db.QueryRowContext(ctx, `SELECT segment_id, byte_length FROM log_segments WHERE state = 'OPEN'`).Scan(&segmentID, &indexedLength); err != nil {
		t.Fatalf("读取活动段索引失败: %v", err)
	}
	if err := persistent.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	file, err := os.OpenFile(filepath.Join(logRoot, segmentID+".open"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("打开模拟未登记尾部失败: %v", err)
	}
	if _, err := file.WriteString(`{"unsafe":true}` + "\n"); err != nil {
		_ = file.Close()
		t.Fatalf("写入模拟未登记尾部失败: %v", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		t.Fatalf("同步模拟未登记尾部失败: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("关闭模拟未登记尾部失败: %v", err)
	}
	restarted, err := logstream.NewPersistentStore(logRoot, database)
	if err != nil {
		t.Fatalf("NewPersistentStore(restarted): %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	if err := restarted.Recover(ctx); err != nil {
		t.Fatalf("Recover(): %v", err)
	}
	info, err := os.Stat(filepath.Join(logRoot, segmentID+".open"))
	if err != nil {
		t.Fatalf("读取恢复后活动段失败: %v", err)
	}
	if info.Size() != indexedLength {
		t.Fatalf("恢复后活动段大小=%d 索引=%d", info.Size(), indexedLength)
	}
	next, err := logstream.SealBatch(logstream.Batch{StreamID: "stream-log-recover-1", SourceEpoch: 1, FirstSeq: 2, LastSeq: 2, PreviousDigest: first.Digest, PolicyVersion: "synthetic-v1", Records: []logstream.Record{{StreamID: "stream-log-recover-1", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 2, Kind: logstream.RecordLog, Message: "safe after restart", PolicyVersion: "synthetic-v1", ParserVersion: "parser-v1"}}})
	if err != nil {
		t.Fatalf("SealBatch(next): %v", err)
	}
	if result, err := restarted.Append(ctx, "execution-log-recover-1", next); err != nil || result.Decision != logstream.BatchAccepted {
		t.Fatalf("Append(next) = %#v, %v", result, err)
	}
	records, _, _, err := restarted.ReadPage(ctx, "task-log-recover-1", nil)
	if err != nil || len(records) != 2 || records[0].Message != "safe before restart" || records[1].Message != "safe after restart" {
		t.Fatalf("ReadPage() records=%#v err=%v", records, err)
	}
	if err := restarted.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown(): %v", err)
	}
	var sealedSegments int
	if err := database.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM log_segments WHERE state = 'SEALED'`).Scan(&sealedSegments); err != nil || sealedSegments != 1 {
		t.Fatalf("正常关闭后的封存段数量=%d err=%v", sealedSegments, err)
	}
}

func TestListExecutionNodeSummariesOnlyReturnsEnabledNodes(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if _, err := store.db.ExecContext(context.Background(), `
        INSERT INTO execution_nodes(node_id, display_name, normalized_name, platform, management_state, allowed_roots_json, tool_config_ref, revision, created_by, created_at, updated_at)
        VALUES (?, ?, ?, 'LINUX_AMD64', 'DISABLED', '[]', NULL, 1, ?, ?, ?)
    `, "node-disabled", "Disabled Node", "disabled node", "subject-1", utcText(testTime), utcText(testTime)); err != nil {
		t.Fatalf("seed disabled node: %v", err)
	}

	summaries, err := store.ListExecutionNodeSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListExecutionNodeSummaries() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0] != (ExecutionNodeSummary{NodeID: "node-1", DisplayName: "Synthetic Node", Platform: "WINDOWS_AMD64"}) {
		t.Fatalf("unexpected node summaries: %#v", summaries)
	}
}

func TestExecutionNodeManagementCreatesDisabledNodeAndProtectsRevision(t *testing.T) {
	store, _ := openTestStore(t)
	if err := store.EnsureAuthSubject(context.Background(), AuthSubject{
		SubjectID: "subject-node", ExternalSubject: "external-node", DisplayName: "Node Admin", AccountStatus: "ACTIVE",
		CreatedAt: testTime, UpdatedAt: testTime,
	}); err != nil {
		t.Fatalf("EnsureAuthSubject() error = %v", err)
	}
	input := ExecutionNodeCreate{
		NodeID: "node-created", CreatorSubjectID: "subject-node", DisplayName: "Windows Export Node", NormalizedName: "windows export node",
		Platform: "WINDOWS_AMD64", AllowedRoots: []string{`E:\ob-data\exports`}, ToolHome: `E:\tools\ob-loader-dumper-4.3.5`, JavaPath: `C:\Java\bin\java.exe`, RequestID: "request-node-create-1",
		IdempotencyKey: "node-create-idempotency-key", RequestDigest: testFingerprint, CreatedAt: testTime,
	}
	created, err := store.CreateExecutionNode(context.Background(), input)
	if err != nil || created.NodeID != input.NodeID || created.Replayed {
		t.Fatalf("CreateExecutionNode() = %#v, %v", created, err)
	}
	replayed, err := store.CreateExecutionNode(context.Background(), input)
	if err != nil || !replayed.Replayed || replayed.NodeID != input.NodeID {
		t.Fatalf("replayed CreateExecutionNode() = %#v, %v", replayed, err)
	}
	node, err := store.GetExecutionNode(context.Background(), input.NodeID)
	if err != nil {
		t.Fatalf("GetExecutionNode() error = %v", err)
	}
	if node.ManagementState != "DISABLED" || node.Revision != 1 || len(node.AllowedRoots) != 1 || node.AllowedRoots[0] != `E:\ob-data\exports` {
		t.Fatalf("unexpected created node: %#v", node)
	}
	summaries, err := store.ListExecutionNodeSummaries(context.Background())
	if err != nil || len(summaries) != 0 {
		t.Fatalf("disabled node must not enter export candidates: %#v, %v", summaries, err)
	}
	revision, err := store.UpdateExecutionNode(context.Background(), ExecutionNodeUpdate{
		NodeID: input.NodeID, ActorSubjectID: "subject-node", ExpectedRevision: 1, DisplayName: "Windows Export Node Updated",
		NormalizedName: "windows export node updated", Platform: "WINDOWS_AMD64", AllowedRoots: []string{`E:\ob-data\exports`, `E:\ob-data\archive`}, ToolHome: `E:\tools\ob-loader-dumper-4.3.5`, JavaPath: `C:\Java\bin\java.exe`,
		RequestID: "request-node-update-1", UpdatedAt: testTime.Add(time.Minute),
	})
	if err != nil || revision != 2 {
		t.Fatalf("UpdateExecutionNode() = %d, %v", revision, err)
	}
	if _, err := store.UpdateExecutionNode(context.Background(), ExecutionNodeUpdate{
		NodeID: input.NodeID, ActorSubjectID: "subject-node", ExpectedRevision: 1, DisplayName: "Stale Node",
		NormalizedName: "stale node", Platform: "WINDOWS_AMD64", AllowedRoots: []string{`E:\ob-data\exports`}, ToolHome: `E:\tools\ob-loader-dumper-4.3.5`, JavaPath: `C:\Java\bin\java.exe`,
		RequestID: "request-node-update-stale", UpdatedAt: testTime.Add(2 * time.Minute),
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale UpdateExecutionNode() error = %v, want ErrRevisionConflict", err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action IN ('EXECUTION_NODE_CREATED', 'EXECUTION_NODE_UPDATED')", 2)
}

func TestDeleteOrArchiveExecutionNodePreservesReferencesAndDeletesUnusedNode(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	agentCredential := sha256.Sum256([]byte("synthetic-node-retirement-credential"))
	enrollmentDigest := sha256.Sum256([]byte("synthetic-active-enrollment"))
	if _, err := store.db.ExecContext(ctx, `UPDATE agents SET credential_digest = ? WHERE agent_id = 'agent-1'`, agentCredential[:]); err != nil {
		t.Fatalf("seed agent credential: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO agent_enrollment_tokens(enrollment_id, node_id, token_digest, status, expires_at, consumed_at, consumed_by_agent_id, created_by, created_at)
		VALUES ('enrollment-active-node-1', 'node-1', ?, 'ACTIVE', ?, NULL, NULL, 'subject-1', ?)
	`, enrollmentDigest[:], utcText(testTime.Add(time.Hour)), utcText(testTime)); err != nil {
		t.Fatalf("seed active enrollment: %v", err)
	}
	if _, err := store.AuthenticateAgent(ctx, agentCredential[:]); err != nil {
		t.Fatalf("AuthenticateAgent() before archival = %v", err)
	}

	archived, err := store.DeleteOrArchiveExecutionNode(ctx, ExecutionNodeDeletion{
		NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		RequestID: "request-archive-node", DeletedAt: testTime.Add(time.Minute),
	})
	if err != nil || archived.Outcome != "ARCHIVED" || archived.Revision != 2 || !archived.AgentAccessRevoked {
		t.Fatalf("DeleteOrArchiveExecutionNode(referenced) = %#v, %v", archived, err)
	}
	var managementState string
	if err := store.db.QueryRow(`SELECT management_state FROM execution_nodes WHERE node_id = 'node-1'`).Scan(&managementState); err != nil || managementState != "ARCHIVED" {
		t.Fatalf("referenced node state = %q, %v", managementState, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM agents WHERE node_id = 'node-1'", 1)
	var agentStatus, enrollmentStatus string
	if err := store.db.QueryRow(`SELECT status FROM agents WHERE agent_id = 'agent-1'`).Scan(&agentStatus); err != nil || agentStatus != "REVOKED" {
		t.Fatalf("archived node agent status = %q, %v", agentStatus, err)
	}
	if err := store.db.QueryRow(`SELECT status FROM agent_enrollment_tokens WHERE enrollment_id = 'enrollment-active-node-1'`).Scan(&enrollmentStatus); err != nil || enrollmentStatus != "REVOKED" {
		t.Fatalf("archived node enrollment status = %q, %v", enrollmentStatus, err)
	}
	if _, err := store.AuthenticateAgent(ctx, agentCredential[:]); !errors.Is(err, ErrAgentAuthenticationFailed) {
		t.Fatalf("AuthenticateAgent() after archival = %v, want ErrAgentAuthenticationFailed", err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXECUTION_NODE_ARCHIVED'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'AGENT_REVOKED'", 1)

	unused := ExecutionNodeCreate{
		NodeID: "node-unused", CreatorSubjectID: "subject-1", DisplayName: "Unused Node", NormalizedName: "unused node",
		Platform: "WINDOWS_AMD64", AllowedRoots: []string{`E:\unused`}, ToolHome: `E:\tools\ob-loader-dumper-4.3.5`, JavaPath: `C:\Java\bin\java.exe`,
		RequestID: "request-create-unused-node", IdempotencyKey: "unused-node-create-idempotency", RequestDigest: testFingerprint, CreatedAt: testTime.Add(2 * time.Minute),
	}
	if _, err := store.CreateExecutionNode(ctx, unused); err != nil {
		t.Fatalf("CreateExecutionNode(unused) = %v", err)
	}
	deleted, err := store.DeleteOrArchiveExecutionNode(ctx, ExecutionNodeDeletion{
		NodeID: unused.NodeID, ActorSubjectID: "subject-1", ExpectedRevision: 1,
		RequestID: "request-delete-unused-node", DeletedAt: testTime.Add(3 * time.Minute),
	})
	if err != nil || deleted.Outcome != "DELETED" || deleted.Revision != 0 {
		t.Fatalf("DeleteOrArchiveExecutionNode(unused) = %#v, %v", deleted, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM execution_nodes WHERE node_id = 'node-unused'", 0)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXECUTION_NODE_DELETED'", 1)
}

func TestDeleteOrArchiveExecutionNodeRejectsRunningTask(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if err := store.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask() = %v", err)
	}
	if err := store.ClaimTask(ctx, validClaim("execution-1", "lease-1", "event-claim-1", "request-claim-1")); err != nil {
		t.Fatalf("ClaimTask() = %v", err)
	}
	_, err := store.DeleteOrArchiveExecutionNode(ctx, ExecutionNodeDeletion{
		NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		RequestID: "request-archive-running-node", DeletedAt: testTime.Add(4 * time.Minute),
	})
	if !errors.Is(err, ErrExecutionNodeHasRunningTask) {
		t.Fatalf("DeleteOrArchiveExecutionNode(running) = %v, want ErrExecutionNodeHasRunningTask", err)
	}
	var managementState, agentStatus string
	if err := store.db.QueryRow(`SELECT management_state FROM execution_nodes WHERE node_id = 'node-1'`).Scan(&managementState); err != nil || managementState != "ENABLED" {
		t.Fatalf("running node state = %q, %v", managementState, err)
	}
	if err := store.db.QueryRow(`SELECT status FROM agents WHERE agent_id = 'agent-1'`).Scan(&agentStatus); err != nil || agentStatus != "ACTIVE" {
		t.Fatalf("running node agent status = %q, %v", agentStatus, err)
	}
}

func TestExecutionNodeManagementRejectsCrossPlatformAndDuplicateRoots(t *testing.T) {
	store, _ := openTestStore(t)
	invalid := ExecutionNodeCreate{
		NodeID: "node-invalid", CreatorSubjectID: "subject", DisplayName: "Invalid Node", NormalizedName: "invalid node",
		Platform: "LINUX_AMD64", AllowedRoots: []string{`E:\not-linux`}, RequestID: "request-invalid",
		IdempotencyKey: "node-invalid-idempotency-key", RequestDigest: testFingerprint, CreatedAt: testTime,
	}
	if _, err := store.CreateExecutionNode(context.Background(), invalid); err == nil {
		t.Fatal("CreateExecutionNode() accepted a Windows root for Linux")
	}
	if ValidateExecutionNodeConfiguration("WINDOWS_AMD64", []string{`E:\same`, `e:\same`}) {
		t.Fatal("ValidateExecutionNodeConfiguration() accepted duplicate Windows roots")
	}
	if !ValidateExecutionNodeConfiguration("LINUX_ARM64", []string{"/var/lib/ob-data-orch"}) {
		t.Fatal("ValidateExecutionNodeConfiguration() rejected a valid Linux absolute root")
	}
}

func TestAgentEnrollmentReplacesPriorIdentityAndStoresOnlyDigests(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	oldCredential := []byte("synthetic-old-machine-credential-value")
	oldCredentialDigest := sha256.Sum256(oldCredential)
	if _, err := store.db.ExecContext(ctx, `UPDATE agents SET credential_digest = ? WHERE agent_id = 'agent-1'`, oldCredentialDigest[:]); err != nil {
		t.Fatalf("seed prior agent credential: %v", err)
	}
	if _, err := store.AuthenticateAgent(ctx, oldCredentialDigest[:]); err != nil {
		t.Fatalf("AuthenticateAgent() before replacement error = %v", err)
	}

	enrollmentMaterial := []byte("synthetic-one-time-enrollment-material")
	enrollmentDigest := sha256.Sum256(enrollmentMaterial)
	issuedAt := testTime.Add(time.Minute)
	if err := store.IssueAgentEnrollment(ctx, AgentEnrollmentIssue{
		EnrollmentID: "enrollment-1", NodeID: "node-1", ActorID: "subject-1", RequestID: "request-enrollment-issue-1",
		TokenDigest: enrollmentDigest[:], CreatedAt: issuedAt, ExpiresAt: issuedAt.Add(10 * time.Minute),
	}); err != nil {
		t.Fatalf("IssueAgentEnrollment() error = %v", err)
	}
	if _, err := store.AuthenticateAgent(ctx, oldCredentialDigest[:]); !errors.Is(err, ErrAgentAuthenticationFailed) {
		t.Fatalf("replaced credential AuthenticateAgent() error = %v, want ErrAgentAuthenticationFailed", err)
	}
	var priorStatus string
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM agents WHERE agent_id = 'agent-1'`).Scan(&priorStatus); err != nil {
		t.Fatalf("read prior agent status: %v", err)
	}
	if priorStatus != "REPLACED" {
		t.Fatalf("prior agent status = %q, want REPLACED", priorStatus)
	}
	var persistedEnrollmentDigest []byte
	if err := store.db.QueryRowContext(ctx, `SELECT token_digest FROM agent_enrollment_tokens WHERE enrollment_id = 'enrollment-1'`).Scan(&persistedEnrollmentDigest); err != nil {
		t.Fatalf("read enrollment digest: %v", err)
	}
	if !bytes.Equal(persistedEnrollmentDigest, enrollmentDigest[:]) || bytes.Equal(persistedEnrollmentDigest, enrollmentMaterial) {
		t.Fatal("enrollment storage must contain only the one-way digest")
	}

	machineCredential := []byte("synthetic-new-machine-credential-value")
	machineCredentialDigest := sha256.Sum256(machineCredential)
	exchange := AgentEnrollmentExchange{
		EnrollmentID: "enrollment-1", NodeID: "node-1", AgentID: "agent-2", RequestID: "request-enrollment-exchange-1",
		ProtocolVersion: "agent-v1", EnrollmentMaterialDigest: enrollmentDigest[:], CredentialDigest: machineCredentialDigest[:], ExchangedAt: issuedAt.Add(time.Minute),
	}
	wrongMaterial := exchange
	wrongDigest := sha256.Sum256([]byte("synthetic-wrong-enrollment-material"))
	wrongMaterial.EnrollmentMaterialDigest = wrongDigest[:]
	if _, err := store.ExchangeAgentEnrollment(ctx, wrongMaterial); !errors.Is(err, ErrEnrollmentRejected) {
		t.Fatalf("wrong material ExchangeAgentEnrollment() error = %v, want ErrEnrollmentRejected", err)
	}
	result, err := store.ExchangeAgentEnrollment(ctx, exchange)
	if err != nil || result.Replayed || result.AgentID != "agent-2" || result.NodeID != "node-1" {
		t.Fatalf("ExchangeAgentEnrollment() = %#v, %v", result, err)
	}
	replayed, err := store.ExchangeAgentEnrollment(ctx, exchange)
	if err != nil || !replayed.Replayed {
		t.Fatalf("replayed ExchangeAgentEnrollment() = %#v, %v", replayed, err)
	}
	identity, err := store.AuthenticateAgent(ctx, machineCredentialDigest[:])
	if err != nil || identity.AgentID != "agent-2" || identity.NodeID != "node-1" {
		t.Fatalf("AuthenticateAgent() = %#v, %v", identity, err)
	}
	var persistedCredentialDigest []byte
	if err := store.db.QueryRowContext(ctx, `SELECT credential_digest FROM agents WHERE agent_id = 'agent-2'`).Scan(&persistedCredentialDigest); err != nil {
		t.Fatalf("read enrolled credential digest: %v", err)
	}
	if !bytes.Equal(persistedCredentialDigest, machineCredentialDigest[:]) || bytes.Equal(persistedCredentialDigest, machineCredential) {
		t.Fatal("agent storage must contain only the machine credential digest")
	}
	rows, err := store.db.QueryContext(ctx, `SELECT safe_diff_json FROM audit_events WHERE action IN ('AGENT_REPLACED', 'AGENT_ENROLLMENT_ISSUED', 'AGENT_ENROLLED')`)
	if err != nil {
		t.Fatalf("read enrollment audit: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var safeDiff string
		if err := rows.Scan(&safeDiff); err != nil {
			t.Fatalf("scan enrollment audit: %v", err)
		}
		if bytes.Contains([]byte(safeDiff), enrollmentMaterial) || bytes.Contains([]byte(safeDiff), machineCredential) {
			t.Fatal("agent enrollment material escaped into audit")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate enrollment audit: %v", err)
	}
}

func TestAgentEnrollmentRejectsExpiredMaterial(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	materialDigest := sha256.Sum256([]byte("synthetic-expired-enrollment-material"))
	issuedAt := testTime.Add(time.Minute)
	if err := store.IssueAgentEnrollment(context.Background(), AgentEnrollmentIssue{
		EnrollmentID: "enrollment-expired", NodeID: "node-1", ActorID: "subject-1", RequestID: "request-enrollment-expired-issue",
		TokenDigest: materialDigest[:], CreatedAt: issuedAt, ExpiresAt: issuedAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("IssueAgentEnrollment() error = %v", err)
	}
	credentialDigest := sha256.Sum256([]byte("synthetic-expired-machine-credential"))
	if _, err := store.ExchangeAgentEnrollment(context.Background(), AgentEnrollmentExchange{
		EnrollmentID: "enrollment-expired", NodeID: "node-1", AgentID: "agent-expired", RequestID: "request-enrollment-expired-exchange",
		ProtocolVersion: "agent-v1", EnrollmentMaterialDigest: materialDigest[:], CredentialDigest: credentialDigest[:], ExchangedAt: issuedAt.Add(2 * time.Minute),
	}); !errors.Is(err, ErrEnrollmentRejected) {
		t.Fatalf("expired ExchangeAgentEnrollment() error = %v, want ErrEnrollmentRejected", err)
	}
	var status string
	if err := store.db.QueryRow(`SELECT status FROM agent_enrollment_tokens WHERE enrollment_id = 'enrollment-expired'`).Scan(&status); err != nil {
		t.Fatalf("read expired enrollment status: %v", err)
	}
	if status != "EXPIRED" {
		t.Fatalf("expired enrollment status = %q, want EXPIRED", status)
	}
}

func TestAgentHeartbeatIsIdempotentAndProtectsCurrentFacts(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE agents SET facts_revision = 0 WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("reset heartbeat facts revision: %v", err)
	}
	machineCredentialDigest := sha256.Sum256([]byte("synthetic-heartbeat-machine-credential"))
	if _, err := store.db.ExecContext(ctx, `UPDATE agents SET credential_digest = ? WHERE agent_id = 'agent-1'`, machineCredentialDigest[:]); err != nil {
		t.Fatalf("seed heartbeat credential: %v", err)
	}
	identity, err := store.AuthenticateAgent(ctx, machineCredentialDigest[:])
	if err != nil || identity.AgentID != "agent-1" {
		t.Fatalf("AuthenticateAgent() = %#v, %v", identity, err)
	}
	observedAt := testTime.Add(5 * time.Minute)
	receivedAt := observedAt.Add(time.Second)
	cpu, memory := 18, 46
	input := AgentHeartbeat{
		AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "boot-heartbeat-1", RequestID: "heartbeat-request-1",
		ObservedAt: observedAt, ReceivedAt: receivedAt, CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: observedAt, CPUUsagePercent: &cpu, MemoryUsagePercent: &memory},
	}
	firstRevision, err := store.RecordAgentHeartbeat(ctx, input)
	if err != nil || firstRevision != 1 {
		t.Fatalf("RecordAgentHeartbeat() = %d, %v; want 1, nil", firstRevision, err)
	}
	replay := input
	replay.ReceivedAt = receivedAt.Add(time.Minute)
	replayedRevision, err := store.RecordAgentHeartbeat(ctx, replay)
	if err != nil || replayedRevision != firstRevision {
		t.Fatalf("replayed RecordAgentHeartbeat() = %d, %v; want %d, nil", replayedRevision, err, firstRevision)
	}
	var storedRevision int64
	var storedReceivedAt, storedRequestID, storedRequestDigest string
	if err := store.db.QueryRowContext(ctx, `
        SELECT facts_revision, last_heartbeat_at, last_heartbeat_request_id, last_heartbeat_request_digest
        FROM agents WHERE agent_id = 'agent-1'
    `).Scan(&storedRevision, &storedReceivedAt, &storedRequestID, &storedRequestDigest); err != nil {
		t.Fatalf("read stored heartbeat: %v", err)
	}
	if storedRevision != 1 || storedReceivedAt != utcText(receivedAt) || storedRequestID != input.RequestID || len(storedRequestDigest) != 64 {
		t.Fatalf("stored heartbeat = revision=%d receivedAt=%q requestId=%q digest=%q", storedRevision, storedReceivedAt, storedRequestID, storedRequestDigest)
	}
	conflict := input
	conflict.Facts.AgentVersion = "agent-test-v2"
	if _, err := store.RecordAgentHeartbeat(ctx, conflict); !errors.Is(err, ErrAgentHeartbeatConflict) {
		t.Fatalf("conflicting RecordAgentHeartbeat() error = %v, want ErrAgentHeartbeatConflict", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT facts_revision FROM agents WHERE agent_id = 'agent-1'`).Scan(&storedRevision); err != nil || storedRevision != 1 {
		t.Fatalf("conflicting heartbeat changed revision = %d, %v", storedRevision, err)
	}
	next := input
	next.RequestID = "heartbeat-request-2"
	next.ReceivedAt = receivedAt.Add(2 * time.Minute)
	next.ObservedAt = observedAt.Add(2 * time.Minute)
	next.Facts.ObservedAt = next.ObservedAt
	nextRevision, err := store.RecordAgentHeartbeat(ctx, next)
	if err != nil || nextRevision != 1 {
		t.Fatalf("仅采样时间变化的 RecordAgentHeartbeat() = %d, %v; want 1, nil", nextRevision, err)
	}
	node, err := store.GetExecutionNode(ctx, "node-1")
	if err != nil || node.Agent == nil || node.Agent.FactsRevision != 1 || node.Agent.EnvironmentFacts.AgentVersion != "agent-test-v1" {
		t.Fatalf("GetExecutionNode() after heartbeat = %#v, %v", node, err)
	}
	if err := store.db.QueryRowContext(ctx, `
        SELECT last_heartbeat_at, last_heartbeat_request_id, last_heartbeat_request_digest
        FROM agents WHERE agent_id = 'agent-1'
    `).Scan(&storedReceivedAt, &storedRequestID, &storedRequestDigest); err != nil {
		t.Fatalf("read repeated heartbeat receipt: %v", err)
	}
	if storedReceivedAt != utcText(next.ReceivedAt) || storedRequestID != next.RequestID || len(storedRequestDigest) != 64 {
		t.Fatalf("repeated heartbeat did not refresh receipt: at=%q id=%q digest=%q", storedReceivedAt, storedRequestID, storedRequestDigest)
	}
	resourceSample := next
	resourceSample.RequestID = "heartbeat-request-resource-sample"
	resourceSample.ReceivedAt = next.ReceivedAt.Add(time.Minute)
	resourceSample.ObservedAt = next.ObservedAt.Add(time.Minute)
	resourceSample.Facts.ObservedAt = resourceSample.ObservedAt
	resourceCPU, resourceMemory := 77, 55
	resourceSample.Facts.CPUUsagePercent = &resourceCPU
	resourceSample.Facts.MemoryUsagePercent = &resourceMemory
	resourceSample.Facts.DataRootUsages = []AgentDataRootUsage{{RootDigest: strings.Repeat("a", 64), TotalBytes: 100, AvailableBytes: 60}}
	resourceRevision, err := store.RecordAgentHeartbeat(ctx, resourceSample)
	if err != nil || resourceRevision != 1 {
		t.Fatalf("资源采样变化的 RecordAgentHeartbeat() = %d, %v; want 1, nil", resourceRevision, err)
	}
	node, err = store.GetExecutionNode(ctx, "node-1")
	if err != nil || node.Agent == nil || node.Agent.EnvironmentFacts.CPUUsagePercent == nil || *node.Agent.EnvironmentFacts.CPUUsagePercent != resourceCPU || len(node.Agent.EnvironmentFacts.DataRootUsages) != 1 {
		t.Fatalf("资源采样没有被持久化: %#v, %v", node, err)
	}
	changedFacts := resourceSample
	changedFacts.RequestID = "heartbeat-request-3"
	changedFacts.ReceivedAt = resourceSample.ReceivedAt.Add(time.Minute)
	changedFacts.Facts.AgentVersion = "agent-test-v2"
	changedRevision, err := store.RecordAgentHeartbeat(ctx, changedFacts)
	if err != nil || changedRevision != 2 {
		t.Fatalf("changed RecordAgentHeartbeat() = %d, %v; want 2, nil", changedRevision, err)
	}
	invalidCapacity := next
	invalidCapacity.RequestID = "heartbeat-invalid-capacity"
	invalidCapacity.CapacityTotal = 2
	if _, err := store.RecordAgentHeartbeat(ctx, invalidCapacity); err == nil {
		t.Fatal("RecordAgentHeartbeat() accepted invalid capacity")
	}
	wrongNode := next
	wrongNode.RequestID = "heartbeat-wrong-node"
	wrongNode.NodeID = "node-other"
	if _, err := store.RecordAgentHeartbeat(ctx, wrongNode); !errors.Is(err, ErrAgentHeartbeatRejected) {
		t.Fatalf("wrong-node RecordAgentHeartbeat() error = %v, want ErrAgentHeartbeatRejected", err)
	}
}

func TestAgentHeartbeatBootChangeMarksActiveExecutionForReconciliation(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if err := store.SubmitTask(ctx, validTaskSubmission("task-boot-change")); err != nil {
		t.Fatalf("提交合成任务: %v", err)
	}
	claim := validClaim("execution-boot-change", "lease-boot-change", "event-boot-change", "request-boot-change")
	claim.TaskID = "task-boot-change"
	if err := store.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("领取合成任务: %v", err)
	}

	observedAt := testTime.Add(4 * time.Minute)
	_, err := store.RecordAgentHeartbeat(ctx, AgentHeartbeat{
		AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "boot-2", RequestID: "heartbeat-boot-change",
		ObservedAt: observedAt, ReceivedAt: observedAt, CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: observedAt},
	})
	if err != nil {
		t.Fatalf("记录重启心跳: %v", err)
	}

	var state, leaseStatus string
	var reconciliationRequired, revision, auditCount int
	if err := store.db.QueryRowContext(ctx, `SELECT state, reconciliation_required, revision FROM task_executions WHERE execution_id = ?`, claim.ExecutionID).Scan(&state, &reconciliationRequired, &revision); err != nil {
		t.Fatalf("读取重启后的执行状态: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM execution_leases WHERE execution_id = ?`, claim.ExecutionID).Scan(&leaseStatus); err != nil {
		t.Fatalf("读取重启后的租约: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE action = 'TASK_EXECUTION_RECONCILIATION_REQUIRED' AND object_id = ?`, claim.ExecutionID).Scan(&auditCount); err != nil {
		t.Fatalf("读取重启核对审计: %v", err)
	}
	if state != "STARTING" || reconciliationRequired != 1 || revision != 2 || leaseStatus != "ISSUED" || auditCount != 1 {
		t.Fatalf("重启核对投影不安全: state=%q reconciliation=%d revision=%d lease=%q audit=%d", state, reconciliationRequired, revision, leaseStatus, auditCount)
	}

	second := AgentHeartbeat{
		AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "boot-2", RequestID: "heartbeat-same-boot",
		ObservedAt: observedAt.Add(time.Minute), ReceivedAt: observedAt.Add(time.Minute), CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: observedAt.Add(time.Minute)},
	}
	if _, err := store.RecordAgentHeartbeat(ctx, second); err != nil {
		t.Fatalf("记录同一启动标识心跳: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE action = 'TASK_EXECUTION_RECONCILIATION_REQUIRED' AND object_id = ?`, claim.ExecutionID).Scan(&auditCount); err != nil {
		t.Fatalf("读取同一启动标识审计: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("同一启动标识重复写入核对审计: %d", auditCount)
	}
}

func TestUpdateDraftUsesOptimisticRevisionAndRejectsSecretJSON(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)

	update := DraftUpdate{
		DraftID:           "draft-1",
		ExpectedRevision:  1,
		ConfigJSON:        `{"database":"synthetic_db","table":"synthetic_table"}`,
		ConfigVersion:     "v5",
		ConfigFingerprint: strings.Repeat("b", 64),
		InvalidationJSON:  `{}`,
		UpdatedAt:         testTime.Add(time.Minute),
	}
	revision, err := store.UpdateDraft(context.Background(), update)
	if err != nil || revision != 2 {
		t.Fatalf("UpdateDraft() = %d, %v; want 2, nil", revision, err)
	}
	if _, err := store.UpdateDraft(context.Background(), update); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale UpdateDraft() error = %v, want ErrRevisionConflict", err)
	}

	update.ExpectedRevision = 2
	update.ConfigJSON = `{"password":"synthetic-secret"}`
	if _, err := store.UpdateDraft(context.Background(), update); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret draft JSON error = %v", err)
	}
}

func TestSubmitTaskIsImmutableAllowsEqualFingerprintAndRollsBackAuditFailure(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()

	if err := store.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(task-1): %v", err)
	}
	second := validTaskSubmission("task-2")
	second.RequestID = "request-submit-2"
	if err := store.SubmitTask(ctx, second); err != nil {
		t.Fatalf("SubmitTask(task-2): %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET snapshot_json = '{}' WHERE task_id = 'task-1'`); err == nil {
		t.Fatal("immutable task update unexpectedly succeeded")
	}
	unsafeArgv := validTaskSubmission("task-unsafe")
	unsafeArgv.PlannedArgvJSON = `["--password","not-allowed"]`
	if err := store.SubmitTask(ctx, unsafeArgv); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("unsafe planned argv error = %v", err)
	}

	rollback := validTaskSubmission("task-rollback")
	rollback.RequestID = "request-rollback"
	if _, err := store.db.ExecContext(ctx, `
        INSERT INTO audit_events(
            audit_event_id, actor_type, actor_id, action, object_type, object_id,
            result, request_id, safe_diff_json, occurred_at
        ) VALUES (?, 'SYSTEM', 'test', 'TEST', 'TASK', ?, 'SUCCEEDED', ?, '{}', ?)
    `, auditID(rollback.TaskID, rollback.RequestID), rollback.TaskID, rollback.RequestID, utcText(testTime)); err != nil {
		t.Fatalf("seed conflicting audit event: %v", err)
	}
	if err := store.SubmitTask(ctx, rollback); err == nil {
		t.Fatal("SubmitTask() with conflicting audit event unexpectedly succeeded")
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM tasks", 2)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events", 3)
}

func TestSubmitTaskIdempotentDoesNotCreateSecondTask(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	input := validTaskSubmission("task-idempotent")
	input.RequestID = "request-task-idempotent"
	created, err := store.SubmitTaskIdempotent(context.Background(), input, "idempotency-task-submit-1", testFingerprint)
	if err != nil || created.TaskID != input.TaskID || created.Replayed {
		t.Fatalf("SubmitTaskIdempotent() = %#v, %v", created, err)
	}
	replayed, err := store.SubmitTaskIdempotent(context.Background(), input, "idempotency-task-submit-1", testFingerprint)
	if err != nil || !replayed.Replayed || replayed.TaskID != input.TaskID {
		t.Fatalf("replayed SubmitTaskIdempotent() = %#v, %v", replayed, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM tasks WHERE task_id = 'task-idempotent'", 1)
	conflict := input
	conflict.TaskID = "task-other"
	if _, err := store.SubmitTaskIdempotent(context.Background(), conflict, "idempotency-task-submit-1", strings.Repeat("b", 64)); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting SubmitTaskIdempotent() error = %v", err)
	}
}

func TestSubmitTaskRequiresCurrentPersistentPrecheckBinding(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Store)
	}{
		{
			name: "节点事实漂移",
			mutate: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE agents SET facts_revision = 2 WHERE agent_id = 'agent-1'`); err != nil {
					t.Fatalf("drift agent facts: %v", err)
				}
			},
		},
		{
			name: "绑定 Agent 已撤销",
			mutate: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE agents SET status = 'REVOKED' WHERE agent_id = 'agent-1'`); err != nil {
					t.Fatalf("revoke binding agent: %v", err)
				}
			},
		},
		{
			name: "历史预检查没有冻结绑定",
			mutate: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE precheck_runs SET node_facts_revision = 0, binding_digest = NULL, binding_agent_id = NULL WHERE precheck_id = 'precheck-1'`); err != nil {
					t.Fatalf("clear legacy precheck binding: %v", err)
				}
			},
		},
		{
			name: "绑定摘要被篡改",
			mutate: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE precheck_runs SET binding_digest = ? WHERE precheck_id = 'precheck-1'`, strings.Repeat("b", 64)); err != nil {
					t.Fatalf("tamper precheck binding digest: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := openTestStore(t)
			seedBaseFixture(t, store)
			test.mutate(store)
			if err := store.SubmitTask(context.Background(), validTaskSubmission("task-binding-"+strings.ReplaceAll(test.name, " ", "-"))); !errors.Is(err, ErrPrecheckInvalid) {
				t.Fatalf("SubmitTask() error = %v, want ErrPrecheckInvalid", err)
			}
			assertCount(t, store.db, "SELECT COUNT(*) FROM tasks", 0)
		})
	}
}

func TestGetTaskSummaryExcludesFrozenSensitiveFields(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if err := store.SubmitTask(context.Background(), validTaskSubmission("task-summary")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	summary, err := store.GetTaskSummary(context.Background(), "task-summary")
	if err != nil || summary.State != "WAITING_SCHEDULE" || summary.PlannedCommandRedacted == "" || summary.ExecutionID != "" {
		t.Fatalf("GetTaskSummary() = %#v, %v", summary, err)
	}
}

func TestListTaskSummariesScopesAndPaginatesWithoutSensitiveFields(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	for _, subject := range []AuthSubject{
		{SubjectID: "subject-delegated", ExternalSubject: "external-delegated", DisplayName: "Delegated", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime},
		{SubjectID: "subject-unscoped", ExternalSubject: "external-unscoped", DisplayName: "Unscoped", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime},
	} {
		if err := store.EnsureAuthSubject(ctx, subject); err != nil {
			t.Fatalf("EnsureAuthSubject(%s): %v", subject.SubjectID, err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO subject_object_scopes(subject_id, scope_type, object_id, granted_by, granted_at) VALUES (?, 'TASK_OPERATE_BY_DATA_SOURCE', ?, ?, ?)`, "subject-delegated", "source-1", "subject-1", utcText(testTime)); err != nil {
		t.Fatalf("insert delegated task scope: %v", err)
	}
	newer := validTaskSubmission("task-list-newer")
	newer.SnapshotJSON = `{"database":"synthetic_db","table":"newer_table"}`
	newer.SubmittedAt = testTime.Add(4 * time.Minute)
	newer.RequestID = "request-task-list-newer"
	if err := store.SubmitTask(ctx, newer); err != nil {
		t.Fatalf("SubmitTask(newer): %v", err)
	}
	older := validTaskSubmission("task-list-older")
	older.SnapshotJSON = `{"database":"synthetic_db","table":"older_table"}`
	older.SubmittedAt = testTime.Add(3 * time.Minute)
	older.RequestID = "request-task-list-older"
	if err := store.SubmitTask(ctx, older); err != nil {
		t.Fatalf("SubmitTask(older): %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
        INSERT INTO task_executions(execution_id, task_id, node_id, agent_id, state, revision,
            reconciliation_required, created_at, started_at, updated_at)
        VALUES (?, ?, ?, ?, 'RUNNING', 1, 1, ?, ?, ?)
    `, "execution-list-newer", newer.TaskID, newer.NodeID, "agent-1", utcText(newer.SubmittedAt), utcText(newer.SubmittedAt), utcText(newer.SubmittedAt.Add(time.Minute))); err != nil {
		t.Fatalf("insert task execution: %v", err)
	}

	page, err := store.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-delegated", Limit: 1})
	if err != nil || len(page) != 2 {
		t.Fatalf("ListTaskSummaries(first) = %#v, %v", page, err)
	}
	if page[0].TaskID != newer.TaskID || page[0].Table != "newer_table" || page[0].State != "RUNNING" || !page[0].ReconciliationRequired {
		t.Fatalf("unexpected first task projection: %#v", page[0])
	}
	if page[0].CreatorSubjectID != "subject-1" || page[0].TaskType != "OBDUMPER_EXPORT" {
		t.Fatalf("authorization metadata missing from internal projection: %#v", page[0])
	}
	next, err := store.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-delegated", BeforeSubmitted: page[0].SubmittedAt, BeforeTaskID: page[0].TaskID, Limit: 1})
	if err != nil || len(next) == 0 || next[0].TaskID != older.TaskID {
		t.Fatalf("ListTaskSummaries(next) = %#v, %v", next, err)
	}
	total, err := store.CountAuthorizedTaskSummaries(ctx, "subject-delegated")
	if err != nil || total != 2 {
		t.Fatalf("CountAuthorizedTaskSummaries(delegated) = %d, %v", total, err)
	}
	unscoped, err := store.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-unscoped", Limit: 10})
	if err != nil || len(unscoped) != 0 {
		t.Fatalf("unscoped task list = %#v, %v", unscoped, err)
	}
	total, err = store.CountAuthorizedTaskSummaries(ctx, "subject-unscoped")
	if err != nil || total != 0 {
		t.Fatalf("CountAuthorizedTaskSummaries(unscoped) = %d, %v", total, err)
	}
	if _, err := store.GetAuthorizedTaskSummary(ctx, newer.TaskID, "subject-unscoped"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("GetAuthorizedTaskSummary(unscoped) error = %v", err)
	}
	if summary, err := store.GetAuthorizedTaskSummary(ctx, newer.TaskID, "subject-delegated"); err != nil || summary.TaskID != newer.TaskID {
		t.Fatalf("GetAuthorizedTaskSummary(delegated) = %#v, %v", summary, err)
	}
}

func TestClaimTaskIsAtomicAndConcurrent(t *testing.T) {
	primary, databasePath := openTestStore(t)
	seedBaseFixture(t, primary)
	if err := primary.SubmitTask(context.Background(), validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	secondary, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	t.Cleanup(func() { _ = secondary.Close() })

	claims := []Claim{
		validClaim("execution-1", "lease-1", "event-claim-1", "request-claim-1"),
		validClaim("execution-2", "lease-2", "event-claim-2", "request-claim-2"),
	}
	stores := []*Store{primary, secondary}
	results := make(chan error, len(claims))
	var ready sync.WaitGroup
	ready.Add(len(claims))
	start := make(chan struct{})
	for index := range claims {
		go func(index int) {
			ready.Done()
			<-start
			results <- stores[index].ClaimTask(context.Background(), claims[index])
		}(index)
	}
	ready.Wait()
	close(start)

	var succeeded, alreadyClaimed int
	for range claims {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAlreadyClaimed):
			alreadyClaimed++
		default:
			t.Fatalf("ClaimTask() error = %v", err)
		}
	}
	if succeeded != 1 || alreadyClaimed != 1 {
		t.Fatalf("claim outcomes succeeded=%d alreadyClaimed=%d", succeeded, alreadyClaimed)
	}
	assertCount(t, primary.db, "SELECT COUNT(*) FROM task_executions", 1)
	assertCount(t, primary.db, "SELECT COUNT(*) FROM execution_leases", 1)
	assertCount(t, primary.db, "SELECT COUNT(*) FROM execution_events", 1)
	assertCount(t, primary.db, "SELECT COUNT(*) FROM audit_events", 2)
}

func TestRenewExecutionLeaseKeepsCurrentAgentAndEpoch(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if err := store.SubmitTask(context.Background(), validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	if err := store.ClaimTask(context.Background(), validClaim("execution-1", "lease-1", "event-claim-1", "request-claim-1")); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	renewal := LeaseRenewal{ExecutionID: "execution-1", LeaseID: "lease-1", LeaseEpoch: 1, AgentID: "agent-1", ExpiresAt: testTime.Add(10 * time.Minute)}
	if err := store.RenewExecutionLease(context.Background(), renewal); err != nil {
		t.Fatalf("RenewExecutionLease(): %v", err)
	}
	wrongAgent := renewal
	wrongAgent.AgentID = "agent-other"
	if err := store.RenewExecutionLease(context.Background(), wrongAgent); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("wrong-agent RenewExecutionLease() error = %v", err)
	}
}

func TestAuthorizeExecutionLogAppendAllowsOnlyMatchingTerminalLeaseReplay(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if err := store.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask() error = %v", err)
	}
	claim := validClaim("execution-log-replay", "lease-log-replay", "event-log-replay", "request-log-replay")
	if err := store.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	var argv []string
	if err := json.Unmarshal([]byte(validTaskSubmission("task-1").PlannedArgvJSON), &argv); err != nil {
		t.Fatalf("解析合成任务参数失败: %v", err)
	}
	digest, err := executionEnvelopeDigest("task-1", "node-1", testFingerprint, "4.3.5-RELEASE", "obdumper-4.3.5-slice-v3", "export-odp-single-table-csv-v1", argv)
	if err != nil {
		t.Fatalf("计算冻结信封摘要失败: %v", err)
	}
	now := testTime.Add(4 * time.Minute)
	if taskID, err := store.AuthorizeExecutionLogAppend(ctx, "agent-1", claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch, digest, now); err != nil || taskID != "task-1" {
		t.Fatalf("活动租约日志授权 = (%q, %v), want task-1 and nil", taskID, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE execution_leases SET status = 'RELEASED', released_at = ? WHERE execution_id = ? AND lease_id = ? AND lease_epoch = ?`, utcText(now), claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch); err != nil {
		t.Fatalf("释放合成租约失败: %v", err)
	}
	if _, err := store.AuthorizeExecutionLogAppend(ctx, "agent-1", claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch, digest, now); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("非终态已释放租约补传 error = %v, want ErrEventRejected", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE task_executions SET state = 'SUCCEEDED', finished_at = ? WHERE execution_id = ?`, utcText(now), claim.ExecutionID); err != nil {
		t.Fatalf("设置合成成功终态失败: %v", err)
	}
	if taskID, err := store.AuthorizeExecutionLogAppend(ctx, "agent-1", claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch, digest, now.Add(time.Hour)); err != nil || taskID != "task-1" {
		t.Fatalf("成功终态已释放租约补传授权 = (%q, %v), want task-1 and nil", taskID, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE task_executions SET state = 'FAILED' WHERE execution_id = ?`, claim.ExecutionID); err != nil {
		t.Fatalf("设置合成失败终态失败: %v", err)
	}
	if taskID, err := store.AuthorizeExecutionLogAppend(ctx, "agent-1", claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch, digest, now.Add(time.Hour)); err != nil || taskID != "task-1" {
		t.Fatalf("失败终态已释放租约补传授权 = (%q, %v), want task-1 and nil", taskID, err)
	}
	if _, err := store.AuthorizeExecutionLogAppend(ctx, "agent-1", claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch, strings.Repeat("b", 64), now); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("错误信封摘要 error = %v, want ErrEventRejected", err)
	}
	if _, err := store.AuthorizeExecutionLogAppend(ctx, "agent-other", claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch, digest, now); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("其他 Agent 的终态补传 error = %v, want ErrEventRejected", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE execution_leases SET status = 'EXPIRED' WHERE execution_id = ? AND lease_id = ? AND lease_epoch = ?`, claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch); err != nil {
		t.Fatalf("设置过期租约失败: %v", err)
	}
	if _, err := store.AuthorizeExecutionLogAppend(ctx, "agent-1", claim.ExecutionID, claim.LeaseID, claim.LeaseEpoch, digest, now); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("过期租约补传 error = %v, want ErrEventRejected", err)
	}
}

func TestAppendExecutionEventRejectsDuplicateAndWrongLease(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if err := store.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	claim := validClaim("execution-1", "lease-1", "event-claim-1", "request-claim-1")
	if err := store.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}

	event := ExecutionEvent{
		EventID:     "event-2",
		ExecutionID: claim.ExecutionID,
		LeaseID:     claim.LeaseID,
		LeaseEpoch:  claim.LeaseEpoch,
		EventSeq:    2,
		EventType:   "PROCESS_STARTED",
		PayloadJSON: `{"pid":42}`,
		ReceivedAt:  testTime.Add(3 * time.Minute),
	}
	if err := store.AppendExecutionEvent(ctx, event); err != nil {
		t.Fatalf("AppendExecutionEvent(): %v", err)
	}
	duplicate := event
	duplicate.EventID = "event-duplicate"
	if err := store.AppendExecutionEvent(ctx, duplicate); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("duplicate event error = %v, want ErrEventRejected", err)
	}
	wrongLease := event
	wrongLease.EventID = "event-wrong-lease"
	wrongLease.EventSeq = 3
	wrongLease.LeaseID = "lease-not-owned"
	if err := store.AppendExecutionEvent(ctx, wrongLease); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("wrong lease event error = %v, want ErrEventRejected", err)
	}
	secretPayload := event
	secretPayload.EventID = "event-secret"
	secretPayload.EventSeq = 3
	secretPayload.PayloadJSON = `{"password":"synthetic-secret"}`
	if err := store.AppendExecutionEvent(ctx, secretPayload); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret payload error = %v", err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM execution_events", 2)
}

func TestListDataSourceSummariesExcludesCredentialMaterial(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO data_sources(
                data_source_id, display_name, normalized_name, environment, connection_kind,
                compatibility_mode, host, port, cluster_name, tenant_name, username, default_database,
                credential_id, current_credential_revision, state, revision, last_test_status,
                last_tested_at, last_test_safe_summary_json, created_by, created_at, updated_at
            ) VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', ?, 2882, 'synthetic-cluster', 'synthetic-tenant', ?, ?, ?, 1, 'ARCHIVED', 1, NULL, NULL, NULL, ?, ?, ?)
    `, "source-archived", "Archived", "archived", "127.0.0.2", "synthetic_user", "synthetic_db", "credential-archived", "subject-1", utcText(testTime), utcText(testTime)); err != nil {
		t.Fatalf("seed archived data source: %v", err)
	}
	summaries, err := store.ListDataSourceSummaries(ctx)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("ListDataSourceSummaries() = %#v, %v", summaries, err)
	}
	summary := summaries[0]
	if summary.DataSourceID != "source-1" || summary.CredentialRevision != 1 || summary.UpdatedAt.IsZero() {
		t.Fatalf("unexpected data source summary: %#v", summary)
	}
	detail, err := store.GetDataSourceSummary(ctx, "source-1")
	if err != nil || detail.DataSourceID != summary.DataSourceID {
		t.Fatalf("GetDataSourceSummary(source-1) = %#v, %v", detail, err)
	}
	if _, err := store.GetDataSourceSummary(ctx, "source-archived"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("GetDataSourceSummary(source-archived) error = %v", err)
	}
	serialized, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, forbidden := range []string{"key-1", "nonce", "ciphertext", "synthetic-secret"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("summary contains credential material %q: %s", forbidden, serialized)
		}
	}
}

func TestCreateDataSourceAtomicallyPersistsEncryptedCredentialAuditAndIdempotency(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	input := DataSourceCreate{
		DataSourceID: "source-create", CredentialID: "credential-create", CreatorSubjectID: "subject-1",
		DisplayName: "Created Source", NormalizedName: "created-source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.3", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", DefaultDatabase: "synthetic_db", KeyID: "key-create",
		Nonce: []byte{1, 2, 3}, Ciphertext: []byte{4, 5, 6}, RequestID: "request-create-1",
		IdempotencyKey: "idempotency-create-1", RequestDigest: testFingerprint, CreatedAt: testTime,
	}
	nonODP := input
	nonODP.DataSourceID, nonODP.CredentialID = "source-non-odp", "credential-non-odp"
	nonODP.ConnectionKind, nonODP.RequestID, nonODP.IdempotencyKey = "OBSERVER_DIRECT", "request-non-odp", "idempotency-non-odp"
	if _, err := store.CreateDataSource(context.Background(), nonODP); err == nil {
		t.Fatal("CreateDataSource() must reject a non-ODP connection kind")
	}
	created, err := store.CreateDataSource(context.Background(), input)
	if err != nil || created.DataSourceID != input.DataSourceID || created.Replayed {
		t.Fatalf("CreateDataSource() = %#v, %v", created, err)
	}
	nameUnavailable := input
	nameUnavailable.DataSourceID, nameUnavailable.CredentialID = "source-name-unavailable", "credential-name-unavailable"
	nameUnavailable.RequestID, nameUnavailable.IdempotencyKey = "request-name-unavailable", "idempotency-name-unavailable"
	if _, err := store.CreateDataSource(context.Background(), nameUnavailable); !errors.Is(err, ErrDataSourceNameUnavailable) {
		t.Fatalf("same normalized name CreateDataSource() error = %v, want ErrDataSourceNameUnavailable", err)
	}
	replay, err := store.CreateDataSource(context.Background(), input)
	if err != nil || !replay.Replayed || replay.DataSourceID != input.DataSourceID {
		t.Fatalf("replayed CreateDataSource() = %#v, %v", replay, err)
	}
	conflict := input
	conflict.RequestDigest = strings.Repeat("b", 64)
	if _, err := store.CreateDataSource(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting CreateDataSource() error = %v", err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM data_sources", 2)
	var createdState string
	if err := store.db.QueryRow(`SELECT state FROM data_sources WHERE data_source_id = 'source-create'`).Scan(&createdState); err != nil {
		t.Fatalf("read created data source state: %v", err)
	}
	if createdState != "DISABLED" {
		t.Fatalf("created data source state=%q, want DISABLED", createdState)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM credential_revisions WHERE credential_id = 'credential-create'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_CREATED'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM request_idempotency WHERE operation = 'CREATE_DATA_SOURCE'", 1)
}

func TestChangeDataSourceStateIsAtomicAndIdempotent(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	input := DataSourceStateChange{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", TargetState: "DISABLED",
		ExpectedRevision: 1, RequestID: "request-disable-1", ChangedAt: testTime.Add(time.Minute),
	}
	changed, err := store.ChangeDataSourceState(context.Background(), input)
	if err != nil || changed.State != "DISABLED" || changed.Revision != 2 || changed.Replayed {
		t.Fatalf("ChangeDataSourceState() = %#v, %v", changed, err)
	}
	replayed, err := store.ChangeDataSourceState(context.Background(), input)
	if err != nil || !replayed.Replayed || replayed.Revision != 2 {
		t.Fatalf("replayed ChangeDataSourceState() = %#v, %v", replayed, err)
	}
	var state string
	var revision int64
	if err := store.db.QueryRow("SELECT state, revision FROM data_sources WHERE data_source_id = 'source-1'").Scan(&state, &revision); err != nil {
		t.Fatalf("read changed data source: %v", err)
	}
	if state != "DISABLED" || revision != 2 {
		t.Fatalf("stored state=%s revision=%d", state, revision)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_DISABLED'", 1)
	missing := input
	missing.DataSourceID = "source-missing"
	missing.RequestID = "request-disable-missing"
	if _, err := store.ChangeDataSourceState(context.Background(), missing); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("missing ChangeDataSourceState() error = %v", err)
	}
	stale := input
	stale.TargetState = "ENABLED"
	stale.RequestID = "request-enable-stale"
	if _, err := store.ChangeDataSourceState(context.Background(), stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale ChangeDataSourceState() error = %v", err)
	}
	blockedEnable := input
	blockedEnable.TargetState, blockedEnable.ExpectedRevision, blockedEnable.RequestID = "ENABLED", 2, "request-enable-without-test"
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = NULL, last_tested_at = NULL, last_test_safe_summary_json = NULL, last_test_source = NULL WHERE data_source_id = 'source-1'`); err != nil {
		t.Fatalf("clear connection test: %v", err)
	}
	if _, err := store.ChangeDataSourceState(context.Background(), blockedEnable); !errors.Is(err, ErrDataSourceConnectionTestRequired) {
		t.Fatalf("unverified ChangeDataSourceState() error = %v, want ErrDataSourceConnectionTestRequired", err)
	}
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ? WHERE data_source_id = 'source-1'`, utcText(testTime.Add(2*time.Minute))); err != nil {
		t.Fatalf("seed successful connection test: %v", err)
	}
	if _, err := store.ChangeDataSourceState(context.Background(), blockedEnable); !errors.Is(err, ErrDataSourceConnectionTestRequired) {
		t.Fatalf("legacy-source ChangeDataSourceState() error = %v, want ErrDataSourceConnectionTestRequired", err)
	}
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_source = 'AGENT_JDBC' WHERE data_source_id = 'source-1'`); err != nil {
		t.Fatalf("seed JDBC connection test source: %v", err)
	}
	enabled, err := store.ChangeDataSourceState(context.Background(), blockedEnable)
	if err != nil || enabled.State != "ENABLED" || enabled.Revision != 3 {
		t.Fatalf("verified ChangeDataSourceState() = %#v, %v", enabled, err)
	}
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = NULL, last_tested_at = NULL, last_test_source = NULL WHERE data_source_id = 'source-1'`); err != nil {
		t.Fatalf("seed invalid legacy enabled state: %v", err)
	}
	if _, err := store.ChangeDataSourceState(context.Background(), blockedEnable); !errors.Is(err, ErrDataSourceConnectionTestRequired) {
		t.Fatalf("enabled source without current test ChangeDataSourceState() error = %v, want ErrDataSourceConnectionTestRequired", err)
	}
	archive := input
	archive.TargetState, archive.ExpectedRevision, archive.RequestID = "ARCHIVED", 3, "request-archive-via-state"
	if _, err := store.ChangeDataSourceState(context.Background(), archive); err == nil {
		t.Fatal("ChangeDataSourceState() must reject ARCHIVED; deletion must choose the archival outcome")
	}
}

func TestDeleteOrArchiveDataSourcePreservesHistoryAndReleasesUnusedName(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()

	archived, err := store.DeleteOrArchiveDataSource(ctx, DataSourceDeletion{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		RequestID: "request-archive-referenced", DeletedAt: testTime.Add(time.Minute),
	})
	if err != nil || archived.Outcome != "ARCHIVED" || archived.Revision != 2 {
		t.Fatalf("DeleteOrArchiveDataSource(referenced) = %#v, %v", archived, err)
	}
	var state string
	if err := store.db.QueryRow(`SELECT state FROM data_sources WHERE data_source_id = 'source-1'`).Scan(&state); err != nil || state != "ARCHIVED" {
		t.Fatalf("referenced source state = %q, %v", state, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM credential_revisions WHERE data_source_id = 'source-1'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_ARCHIVED'", 1)

	conflictingName := DataSourceCreate{
		DataSourceID: "source-name-conflict", CredentialID: "credential-name-conflict", CreatorSubjectID: "subject-1",
		DisplayName: "Synthetic Source", NormalizedName: "synthetic source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.7", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", DefaultDatabase: "synthetic_db", KeyID: "key-name-conflict",
		Nonce: []byte{1, 2, 3}, Ciphertext: []byte{4, 5, 6}, RequestID: "request-name-conflict",
		IdempotencyKey: "idempotency-name-conflict", RequestDigest: testFingerprint, CreatedAt: testTime.Add(2 * time.Minute),
	}
	if _, err := store.CreateDataSource(ctx, conflictingName); !errors.Is(err, ErrDataSourceNameUnavailable) {
		t.Fatalf("CreateDataSource(referenced archived name) error = %v, want ErrDataSourceNameUnavailable", err)
	}

	unused := conflictingName
	unused.DataSourceID, unused.CredentialID = "source-unused", "credential-unused"
	unused.DisplayName, unused.NormalizedName = "Unused Source", "unused source"
	unused.RequestID, unused.IdempotencyKey, unused.KeyID = "request-unused", "idempotency-unused", "key-unused"
	created, err := store.CreateDataSource(ctx, unused)
	if err != nil || created.DataSourceID != unused.DataSourceID {
		t.Fatalf("CreateDataSource(unused) = %#v, %v", created, err)
	}
	deleted, err := store.DeleteOrArchiveDataSource(ctx, DataSourceDeletion{
		DataSourceID: unused.DataSourceID, ActorSubjectID: "subject-1", ExpectedRevision: 1,
		RequestID: "request-delete-unused", DeletedAt: testTime.Add(3 * time.Minute),
	})
	if err != nil || deleted.Outcome != "DELETED" || deleted.Revision != 0 {
		t.Fatalf("DeleteOrArchiveDataSource(unused) = %#v, %v", deleted, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM data_sources WHERE data_source_id = 'source-unused'", 0)
	assertCount(t, store.db, "SELECT COUNT(*) FROM credential_revisions WHERE data_source_id = 'source-unused'", 0)
	assertCount(t, store.db, "SELECT COUNT(*) FROM request_idempotency WHERE resource_id = 'source-unused'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_DELETED'", 1)
	replayed, err := store.CreateDataSource(ctx, unused)
	if err != nil || !replayed.Replayed || replayed.DataSourceID != unused.DataSourceID {
		t.Fatalf("CreateDataSource(deleted replay) = %#v, %v", replayed, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM data_sources WHERE data_source_id = 'source-unused'", 0)

	recreated := unused
	recreated.DataSourceID, recreated.CredentialID = "source-unused-recreated", "credential-unused-recreated"
	recreated.RequestID, recreated.IdempotencyKey, recreated.KeyID = "request-unused-recreated", "idempotency-unused-recreated", "key-unused-recreated"
	recreated.CreatedAt = testTime.Add(4 * time.Minute)
	if _, err := store.CreateDataSource(ctx, recreated); err != nil {
		t.Fatalf("CreateDataSource(reused deleted name) error = %v", err)
	}
}

func TestUpdateDataSourceAtomicallyRotatesOptionalCredential(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	base := DataSourceUpdate{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		DisplayName: "Updated Source", NormalizedName: "updated source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.8", Port: 2882,
		ClusterName: "updated-cluster", TenantName: "updated-tenant", Username: "updated_user", DefaultDatabase: "updated_db", RequestID: "request-update-1", UpdatedAt: testTime.Add(time.Minute),
	}
	updated, err := store.UpdateDataSource(context.Background(), base)
	if err != nil || updated.Revision != 2 || updated.CredentialRevision != 1 {
		t.Fatalf("UpdateDataSource() = %#v, %v", updated, err)
	}
	rotation := base
	rotation.ExpectedRevision = 2
	rotation.RequestID = "request-rotate-1"
	rotation.UpdatedAt = testTime.Add(2 * time.Minute)
	rotation.Password = &EncryptedDataSourcePassword{CredentialID: "credential-1", Revision: 2, KeyID: "key-2", Nonce: []byte{7, 8, 9}, Ciphertext: []byte{10, 11, 12}}
	rotated, err := store.UpdateDataSource(context.Background(), rotation)
	if err != nil || rotated.Revision != 3 || rotated.CredentialRevision != 2 {
		t.Fatalf("rotated UpdateDataSource() = %#v, %v", rotated, err)
	}
	var state string
	if err := store.db.QueryRow("SELECT status FROM credential_revisions WHERE credential_id = 'credential-1' AND revision = 1").Scan(&state); err != nil {
		t.Fatalf("read retired credential: %v", err)
	}
	if state != "SUPERSEDED" {
		t.Fatalf("prior credential status=%q, want SUPERSEDED", state)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM credential_revisions WHERE credential_id = 'credential-1'", 2)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_UPDATED'", 2)
	stale := base
	stale.RequestID = "request-update-stale"
	if _, err := store.UpdateDataSource(context.Background(), stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale UpdateDataSource() error = %v", err)
	}
}

func TestUpdateDataSourceInvalidatesOnlyChangedConnectionTestFacts(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	testedAt := testTime.Add(-time.Hour)
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ?, last_test_safe_summary_json = '{}', last_test_source = 'AGENT_JDBC' WHERE data_source_id = 'source-1'`, utcText(testedAt)); err != nil {
		t.Fatalf("seed connection test: %v", err)
	}
	base := DataSourceUpdate{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		DisplayName: "Renamed Source", NormalizedName: "renamed source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic_user", DefaultDatabase: "synthetic_db",
		RequestID: "request-rename-1", UpdatedAt: testTime.Add(time.Minute),
	}
	renamed, err := store.UpdateDataSource(context.Background(), base)
	if err != nil || renamed.ConnectionTestInvalidated {
		t.Fatalf("renamed UpdateDataSource() = %#v, %v", renamed, err)
	}
	preserved, err := store.GetDataSourceSummary(context.Background(), "source-1")
	if err != nil || preserved.LastTestStatus != "SUCCEEDED" || preserved.LastTestedAt == nil || preserved.LastTestSource != "AGENT_JDBC" {
		t.Fatalf("display-only update must retain connection test: %#v, %v", preserved, err)
	}
	changed := base
	changed.ExpectedRevision, changed.RequestID, changed.UpdatedAt, changed.Host = 2, "request-host-2", testTime.Add(2*time.Minute), "127.0.0.9"
	updated, err := store.UpdateDataSource(context.Background(), changed)
	if err != nil || !updated.ConnectionTestInvalidated {
		t.Fatalf("connection update UpdateDataSource() = %#v, %v", updated, err)
	}
	invalidated, err := store.GetDataSourceSummary(context.Background(), "source-1")
	if err != nil || invalidated.State != "DISABLED" || invalidated.LastTestStatus != "" || invalidated.LastTestedAt != nil || invalidated.LastTestSafeSummaryJSON != "" || invalidated.LastTestSource != "" {
		t.Fatalf("connection update must disable the source and clear prior test facts: %#v, %v", invalidated, err)
	}
	if _, err := store.ChangeDataSourceState(context.Background(), DataSourceStateChange{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", TargetState: "ENABLED",
		ExpectedRevision: 3, RequestID: "request-enable-invalidated-source", ChangedAt: testTime.Add(2 * time.Minute),
	}); !errors.Is(err, ErrDataSourceConnectionTestRequired) {
		t.Fatalf("invalidated source enable error = %v, want ErrDataSourceConnectionTestRequired", err)
	}
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ?, last_test_safe_summary_json = '{}', last_test_source = 'AGENT_JDBC' WHERE data_source_id = 'source-1'`, utcText(testedAt)); err != nil {
		t.Fatalf("seed connection test before password rotation: %v", err)
	}
	passwordRotation := changed
	passwordRotation.ExpectedRevision, passwordRotation.RequestID, passwordRotation.UpdatedAt = 3, "request-password-3", testTime.Add(3*time.Minute)
	passwordRotation.Password = &EncryptedDataSourcePassword{CredentialID: "credential-1", Revision: 2, KeyID: "key-2", Nonce: []byte{7, 8, 9}, Ciphertext: []byte{10, 11, 12}}
	rotated, err := store.UpdateDataSource(context.Background(), passwordRotation)
	if err != nil || !rotated.ConnectionTestInvalidated {
		t.Fatalf("password update UpdateDataSource() = %#v, %v", rotated, err)
	}
	invalidatedByPassword, err := store.GetDataSourceSummary(context.Background(), "source-1")
	if err != nil || invalidatedByPassword.LastTestStatus != "" || invalidatedByPassword.LastTestedAt != nil || invalidatedByPassword.LastTestSafeSummaryJSON != "" || invalidatedByPassword.LastTestSource != "" {
		t.Fatalf("password update must clear prior test facts: %#v, %v", invalidatedByPassword, err)
	}
}

func TestRevokeLocalSyntheticDataSourceTestFactsIsScopedAndIdempotent(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ?, last_test_safe_summary_json = '{}', last_test_source = NULL WHERE data_source_id = 'source-1'`, utcText(testTime)); err != nil {
		t.Fatalf("seed enabled synthetic test fact: %v", err)
	}
	for _, fixture := range []struct {
		id, name, state, status, source string
	}{
		{id: "source-disabled", name: "Disabled Source", state: "DISABLED", status: "FAILED"},
		{id: "source-archived", name: "Archived Source", state: "ARCHIVED", status: "SUCCEEDED", source: "AGENT_JDBC"},
		{id: "source-real", name: "Real Source", state: "ENABLED", status: "SUCCEEDED", source: "AGENT_JDBC"},
	} {
		var source any
		if fixture.source != "" {
			source = fixture.source
		}
		if _, err := store.db.Exec(`
            INSERT INTO data_sources(
                data_source_id, display_name, normalized_name, environment, connection_kind,
                compatibility_mode, host, port, cluster_name, tenant_name, username, default_database,
                credential_id, current_credential_revision, state, revision, last_test_status,
				last_tested_at, last_test_safe_summary_json, last_test_source, created_by, created_at, updated_at
            ) VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', '127.0.0.1', 2881, 'synthetic-cluster', 'synthetic-tenant',
                      'synthetic-user', 'synthetic-db', ?, 1, ?, 1, ?, ?, '{}', ?, 'subject-1', ?, ?)
        `, fixture.id, fixture.name, strings.ToLower(fixture.name), "credential-"+fixture.id, fixture.state, fixture.status,
			utcText(testTime), source, utcText(testTime), utcText(testTime)); err != nil {
			t.Fatalf("seed %s source: %v", fixture.id, err)
		}
	}

	revoked, err := store.RevokeLocalSyntheticDataSourceTestFacts(context.Background(), testTime.Add(time.Minute))
	if err != nil || revoked != 2 {
		t.Fatalf("RevokeLocalSyntheticDataSourceTestFacts() = %d, %v; want 2, nil", revoked, err)
	}
	for _, dataSourceID := range []string{"source-1", "source-disabled"} {
		var state, status, testedAt, summary string
		var revision int64
		if err := store.db.QueryRow(`
            SELECT state, revision, COALESCE(last_test_status, ''), COALESCE(last_tested_at, ''), COALESCE(last_test_safe_summary_json, '')
            FROM data_sources WHERE data_source_id = ?
        `, dataSourceID).Scan(&state, &revision, &status, &testedAt, &summary); err != nil {
			t.Fatalf("read revoked source %s: %v", dataSourceID, err)
		}
		if state != "DISABLED" || revision != 2 || status != "" || testedAt != "" || summary != "" {
			t.Fatalf("revoked source %s = state=%q revision=%d status=%q testedAt=%q summary=%q", dataSourceID, state, revision, status, testedAt, summary)
		}
	}
	var archivedState, archivedStatus string
	if err := store.db.QueryRow(`SELECT state, COALESCE(last_test_status, '') FROM data_sources WHERE data_source_id = 'source-archived'`).Scan(&archivedState, &archivedStatus); err != nil {
		t.Fatalf("read archived source: %v", err)
	}
	if archivedState != "ARCHIVED" || archivedStatus != "SUCCEEDED" {
		t.Fatalf("archived source changed unexpectedly: state=%q status=%q", archivedState, archivedStatus)
	}
	var realState, realStatus, realSource string
	if err := store.db.QueryRow(`SELECT state, COALESCE(last_test_status, ''), COALESCE(last_test_source, '') FROM data_sources WHERE data_source_id = 'source-real'`).Scan(&realState, &realStatus, &realSource); err != nil {
		t.Fatalf("read real JDBC source: %v", err)
	}
	if realState != "ENABLED" || realStatus != "SUCCEEDED" || realSource != "AGENT_JDBC" {
		t.Fatalf("real JDBC source changed unexpectedly: state=%q status=%q source=%q", realState, realStatus, realSource)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_LOCAL_SYNTHETIC_TEST_REVOKED'", 2)
	revokedAgain, err := store.RevokeLocalSyntheticDataSourceTestFacts(context.Background(), testTime.Add(2*time.Minute))
	if err != nil || revokedAgain != 0 {
		t.Fatalf("second RevokeLocalSyntheticDataSourceTestFacts() = %d, %v; want 0, nil", revokedAgain, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_LOCAL_SYNTHETIC_TEST_REVOKED'", 2)
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'FAILED', last_tested_at = ?, last_test_safe_summary_json = '{}', last_test_source = NULL WHERE data_source_id = 'source-1'`, utcText(testTime.Add(3*time.Minute))); err != nil {
		t.Fatalf("恢复合成测试事实: %v", err)
	}
	revokedAfterRetest, err := store.RevokeLocalSyntheticDataSourceTestFacts(context.Background(), testTime.Add(4*time.Minute))
	if err != nil || revokedAfterRetest != 1 {
		t.Fatalf("重新出现合成测试后的 RevokeLocalSyntheticDataSourceTestFacts() = %d, %v; want 1, nil", revokedAfterRetest, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_LOCAL_SYNTHETIC_TEST_REVOKED'", 3)
}

func TestCreateExportDraftBindsEnabledSourceAndNodeWithIdempotency(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ?, last_test_source = 'AGENT_JDBC' WHERE data_source_id = 'source-1'`, utcText(testTime)); err != nil {
		t.Fatalf("seed successful connection test: %v", err)
	}
	input := ExportDraftCreate{
		ExportDraft: ExportDraft{DraftID: "draft-create", OwnerSubjectID: "subject-1", DataSourceID: "source-1", NodeID: "node-1", ToolVersion: "4.3.5-RELEASE", MetadataVersion: "obdumper-4.3.5-slice-v3", CapabilityVersion: "export-odp-single-table-csv-v1", ConfigVersion: "v5", ConfigJSON: `{"database":"synthetic_db","table":"synthetic_table","format":"CSV"}`, ConfigFingerprint: testFingerprint, InvalidationJSON: `{}`, CreatedAt: testTime, UpdatedAt: testTime},
		RequestID:   "request-draft-create-1", IdempotencyKey: "idempotency-draft-create-1", RequestDigest: testFingerprint,
	}
	created, err := store.CreateExportDraft(context.Background(), input)
	if err != nil || created.DraftID != input.DraftID || created.Replayed {
		t.Fatalf("CreateExportDraft() = %#v, %v", created, err)
	}
	replayed, err := store.CreateExportDraft(context.Background(), input)
	if err != nil || !replayed.Replayed || replayed.DraftID != input.DraftID {
		t.Fatalf("replayed CreateExportDraft() = %#v, %v", replayed, err)
	}
	draft, err := store.GetExportDraft(context.Background(), input.DraftID)
	if err != nil || draft.Revision != 1 || draft.ConfigJSON != input.ConfigJSON {
		t.Fatalf("GetExportDraft() = %#v, %v", draft, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM export_drafts WHERE draft_id = 'draft-create'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXPORT_DRAFT_CREATED'", 1)
	conflict := input
	conflict.RequestDigest = strings.Repeat("b", 64)
	if _, err := store.CreateExportDraft(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting CreateExportDraft() error = %v", err)
	}
}

func TestCreateExportDraftRejectsEnabledSourceWithoutSuccessfulConnectionTest(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = NULL, last_tested_at = NULL, last_test_safe_summary_json = NULL WHERE data_source_id = 'source-1'`); err != nil {
		t.Fatalf("clear connection test: %v", err)
	}
	input := ExportDraftCreate{
		ExportDraft: ExportDraft{DraftID: "draft-unverified", OwnerSubjectID: "subject-1", DataSourceID: "source-1", NodeID: "node-1", ToolVersion: "4.3.5-RELEASE", MetadataVersion: "obdumper-4.3.5-slice-v3", CapabilityVersion: "export-odp-single-table-csv-v1", ConfigVersion: "v5", ConfigJSON: `{"database":"synthetic_db","table":"synthetic_table","format":"CSV"}`, ConfigFingerprint: testFingerprint, InvalidationJSON: `{}`, CreatedAt: testTime, UpdatedAt: testTime},
		RequestID:   "request-draft-unverified", IdempotencyKey: "idempotency-draft-unverified", RequestDigest: testFingerprint,
	}
	if _, err := store.CreateExportDraft(context.Background(), input); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("CreateExportDraft() error = %v, want ErrDataSourceNotFound", err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM export_drafts WHERE draft_id = 'draft-unverified'", 0)
}

// TestExportDraftV6RoundTripAndLegacyCompatibility 验证 v6 草稿结构化列往返、
// 存量 v5 草稿默认版本读取，以及 v2 快照的任务投影兼容。
func TestExportDraftV6RoundTripAndLegacyCompatibility(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()

	input := ExportDraftCreate{
		ExportDraft: ExportDraft{
			DraftID: "draft-v6", OwnerSubjectID: "subject-1", DataSourceID: "source-1", NodeID: "node-1",
			ToolVersion: "4.3.5-RELEASE", MetadataVersion: "obdumper-4.3.5-slice-v5", CapabilityVersion: "export-odp-single-table-csv-v1",
			ConfigVersion:     "v6",
			ConfigJSON:        `{"configVersion":"v6","database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/tmp/output","config":{"dataFormat":{"formatKind":"CSV"}}}`,
			ConfigFingerprint: testFingerprint, InvalidationJSON: `{}`,
			ObjectScopeJSON: `{"scopeKind":"SPECIFIED"}`, ContentSelectionJSON: `{"contentKind":"DATA_ONLY"}`,
			DataFormatJSON: `{"formatKind":"CSV"}`, OutputConfigJSON: `{"outputKind":"LOCAL","filePath":"/E:/tmp/output"}`,
			PerformanceConfigJSON: `{}`, FilterConfigJSON: `{}`, DDLBehaviorJSON: `{}`,
			CreatedAt: testTime, UpdatedAt: testTime,
		},
		RequestID: "request-draft-v6", IdempotencyKey: "idempotency-draft-v6", RequestDigest: testFingerprint,
	}
	if _, err := store.CreateExportDraft(ctx, input); err != nil {
		t.Fatalf("v6 CreateExportDraft() error = %v", err)
	}
	draft, err := store.GetExportDraft(ctx, "draft-v6")
	if err != nil {
		t.Fatalf("v6 GetExportDraft() error = %v", err)
	}
	if draft.ConfigVersion != "v6" || draft.ObjectScopeJSON != `{"scopeKind":"SPECIFIED"}` || draft.DataFormatJSON != `{"formatKind":"CSV"}` || draft.OutputConfigJSON != `{"outputKind":"LOCAL","filePath":"/E:/tmp/output"}` || draft.DDLBehaviorJSON != `{}` {
		t.Fatalf("v6 draft round trip mismatch: %#v", draft)
	}

	// 存量草稿未写入新列时必须读出默认 v5 版本与空结构化列。
	legacy, err := store.GetExportDraft(ctx, "draft-1")
	if err != nil {
		t.Fatalf("legacy GetExportDraft() error = %v", err)
	}
	if legacy.ConfigVersion != "v5" || legacy.ObjectScopeJSON != "{}" || legacy.DDLBehaviorJSON != "{}" {
		t.Fatalf("legacy draft version mismatch: %#v", legacy)
	}

	// v2 快照保留扁平投影键，任务摘要投影必须继续可读。
	submission := validTaskSubmission("task-v2")
	submission.SnapshotVersion = "v2"
	submission.SnapshotJSON = input.ConfigJSON
	if err := store.SubmitTask(ctx, submission); err != nil {
		t.Fatalf("v2 SubmitTask() error = %v", err)
	}
	summary, err := store.GetTaskSummary(ctx, "task-v2")
	if err != nil {
		t.Fatalf("v2 GetTaskSummary() error = %v", err)
	}
	if summary.SnapshotVersion != "v2" || summary.Database != "synthetic_db" || summary.Table != "synthetic_table" || summary.Format != "CSV" {
		t.Fatalf("v2 task summary projection mismatch: %#v", summary)
	}

	// 未知快照版本必须失败关闭。
	invalid := validTaskSubmission("task-invalid-snapshot")
	invalid.SnapshotVersion = "v9"
	if err := store.SubmitTask(ctx, invalid); err == nil {
		t.Fatalf("invalid snapshot version accepted")
	}

	// 多对象 DDL+CSV 的 v2 快照投影：$.table 存逗号连接清单，$.format 存 DDL_CSV。
	multiObject := validTaskSubmission("task-v2-multi")
	multiObject.SnapshotVersion = "v2"
	multiObject.SnapshotJSON = `{"configVersion":"v6","database":"synthetic_db","scopeKind":"SPECIFIED","table":"table_one,table_two","contentKind":"DDL_AND_DATA","format":"DDL_CSV","filePath":"/E:/tmp/output","config":{}}`
	if err := store.SubmitTask(ctx, multiObject); err != nil {
		t.Fatalf("v2 multi-object SubmitTask() error = %v", err)
	}
	multiSummary, err := store.GetTaskSummary(ctx, "task-v2-multi")
	if err != nil {
		t.Fatalf("v2 multi-object GetTaskSummary() error = %v", err)
	}
	if multiSummary.SnapshotVersion != "v2" || multiSummary.Table != "table_one,table_two" || multiSummary.Format != "DDL_CSV" {
		t.Fatalf("v2 multi-object projection mismatch: %#v", multiSummary)
	}
}

func TestCreatePrecheckFreezesDraftBindingAndIdempotency(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = NULL, last_tested_at = NULL, last_test_safe_summary_json = NULL WHERE data_source_id = 'source-1'`); err != nil {
		t.Fatalf("clear connection test: %v", err)
	}
	input := PrecheckCreate{PrecheckRun: PrecheckRun{PrecheckID: "precheck-create", DraftID: "draft-1", DraftRevision: 1, ConfigFingerprint: testFingerprint, DataSourceID: "source-1", CredentialID: "credential-1", CredentialRevision: 1, NodeID: "node-1", CreatedAt: testTime, ValidUntil: testTime.Add(5 * time.Minute)}, CreatorSubjectID: "subject-1", RequestID: "request-precheck-create-1", IdempotencyKey: "idempotency-precheck-create-1", RequestDigest: testFingerprint}
	if _, err := store.CreatePrecheck(context.Background(), input); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatalf("CreatePrecheck() without successful connection test error = %v, want ErrPrecheckInvalid", err)
	}
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ?, last_test_source = 'AGENT_JDBC' WHERE data_source_id = 'source-1'`, utcText(testTime)); err != nil {
		t.Fatalf("seed successful connection test: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE agents SET facts_revision = 0 WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("clear synthetic agent facts revision: %v", err)
	}
	if _, err := store.CreatePrecheck(context.Background(), input); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatalf("CreatePrecheck() without current agent facts error = %v, want ErrPrecheckInvalid", err)
	}
	if _, err := store.db.Exec(`UPDATE agents SET status = 'REVOKED' WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("revoke synthetic agent: %v", err)
	}
	if _, err := store.CreatePrecheck(context.Background(), input); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatalf("CreatePrecheck() without active agent error = %v, want ErrPrecheckInvalid", err)
	}
	if _, err := store.db.Exec(`UPDATE agents SET facts_revision = 1 WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("seed current agent facts revision: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE agents SET status = 'ACTIVE' WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("restore synthetic agent: %v", err)
	}
	created, err := store.CreatePrecheck(context.Background(), input)
	if err != nil || created.PrecheckID != input.PrecheckID || created.Replayed {
		t.Fatalf("CreatePrecheck() = %#v, %v", created, err)
	}
	replayed, err := store.CreatePrecheck(context.Background(), input)
	if err != nil || !replayed.Replayed {
		t.Fatalf("replayed CreatePrecheck() = %#v, %v", replayed, err)
	}
	run, err := store.GetPrecheckRun(context.Background(), input.PrecheckID)
	if err != nil || run.Status != "PENDING" || run.CredentialID != "credential-1" || run.CredentialRevision != 1 || run.NodeFactsRevision != 1 || len(run.BindingDigest) != 64 {
		t.Fatalf("GetPrecheckRun() = %#v, %v", run, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXPORT_PRECHECK_CREATED'", 1)
}

func TestClaimNextPrecheckSelectsOnlyCurrentFrozenAgentBinding(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	first := createClaimablePrecheck(t, store, "precheck-candidate-first")
	second := createClaimablePrecheck(t, store, "precheck-candidate-second")
	now := testTime.Add(time.Minute)

	grant, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-next-first", RequestID: "precheck-next-first",
		RequestDigest: strings.Repeat("4", 64), LeaseTTL: time.Minute, Now: now,
	})
	if err != nil || !found || grant.PrecheckID != first.PrecheckID || grant.LeaseID != "lease-next-first" {
		t.Fatalf("ClaimNextPrecheck() = %#v, %t, %v", grant, found, err)
	}
	replayed, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-next-replayed", RequestID: "precheck-next-first",
		RequestDigest: strings.Repeat("4", 64), LeaseTTL: time.Minute, Now: now,
	})
	if err != nil || !found || !replayed.Replayed || replayed.PrecheckID != first.PrecheckID || replayed.LeaseID != grant.LeaseID {
		t.Fatalf("replayed ClaimNextPrecheck() = %#v, %t, %v", replayed, found, err)
	}
	if _, _, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-next-conflict", RequestID: "precheck-next-first",
		RequestDigest: strings.Repeat("b", 64), LeaseTTL: time.Minute, Now: now,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting ClaimNextPrecheck() error = %v, want ErrIdempotencyConflict", err)
	}
	if _, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-2", NodeID: "node-1", LeaseID: "lease-next-foreign-agent", RequestID: "precheck-next-foreign-agent",
		RequestDigest: strings.Repeat("5", 64), LeaseTTL: time.Minute, Now: now,
	}); err != nil || found {
		t.Fatalf("错误 Agent ClaimNextPrecheck() found = %t, error = %v", found, err)
	}
	if _, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-1", NodeID: "node-2", LeaseID: "lease-next-foreign-node", RequestID: "precheck-next-foreign-node",
		RequestDigest: strings.Repeat("6", 64), LeaseTTL: time.Minute, Now: now,
	}); err != nil || found {
		t.Fatalf("错误节点 ClaimNextPrecheck() found = %t, error = %v", found, err)
	}
	if _, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-next-busy", RequestID: "precheck-next-busy",
		RequestDigest: strings.Repeat("7", 64), LeaseTTL: time.Minute, Now: now,
	}); !errors.Is(err, ErrPrecheckLeaseRejected) || found {
		t.Fatalf("已占用 Agent ClaimNextPrecheck() found = %t, error = %v", found, err)
	}
	grant, found, err = store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-next-second", RequestID: "precheck-next-second",
		RequestDigest: strings.Repeat("a", 64), LeaseTTL: time.Minute, Now: grant.ExpiresAt.Add(time.Second),
	})
	if err != nil || !found || grant.PrecheckID != second.PrecheckID {
		t.Fatalf("租约过期后的 ClaimNextPrecheck() = %#v, %t, %v", grant, found, err)
	}
}

func TestClaimNextPrecheck收口过期执行租约并释放节点容量(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-after-expired-execution")
	if err := store.SubmitTask(context.Background(), validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask() error = %v", err)
	}
	claim := validClaim("execution-expired-lease", "lease-expired-lease", "event-expired-lease", "request-expired-lease")
	if err := store.ClaimTask(context.Background(), claim); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	now := claim.ExpiresAt.Add(time.Second)
	grant, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-after-expired-execution", RequestID: "precheck-after-expired-execution",
		RequestDigest: strings.Repeat("c", 64), LeaseTTL: time.Minute, Now: now,
	})
	if err != nil || !found || grant.PrecheckID != run.PrecheckID {
		t.Fatalf("ClaimNextPrecheck() = %#v, %t, %v", grant, found, err)
	}
	var leaseStatus, executionState string
	var reconciliationRequired int
	if err := store.db.QueryRow(`SELECT status FROM execution_leases WHERE execution_id = ?`, claim.ExecutionID).Scan(&leaseStatus); err != nil {
		t.Fatalf("读取过期租约: %v", err)
	}
	if err := store.db.QueryRow(`SELECT state, reconciliation_required FROM task_executions WHERE execution_id = ?`, claim.ExecutionID).Scan(&executionState, &reconciliationRequired); err != nil {
		t.Fatalf("读取过期执行: %v", err)
	}
	if leaseStatus != "EXPIRED" || executionState != "FAILED" || reconciliationRequired != 1 {
		t.Fatalf("过期执行未被安全收口: lease=%s state=%s reconciliation=%d", leaseStatus, executionState, reconciliationRequired)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'TASK_EXECUTION_LEASE_EXPIRED' AND object_id = 'execution-expired-lease'", 1)
}

func TestPersistentPrecheckLeaseRequiresAcknowledgementAndReplays(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-persistent")
	now := testTime.Add(time.Minute)
	claim := PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-persistent",
		RequestID: "precheck-claim-persistent", RequestDigest: strings.Repeat("b", 64), LeaseTTL: 2 * time.Minute, Now: now,
	}
	grant, err := store.ClaimPrecheck(context.Background(), claim)
	if err != nil || grant.LeaseEpoch != 1 || grant.Binding.BindingDigest != run.BindingDigest || grant.ExpiresAt != now.Add(2*time.Minute) {
		t.Fatalf("ClaimPrecheck() = %#v, %v", grant, err)
	}
	replayedGrant, err := store.ClaimPrecheck(context.Background(), claim)
	if err != nil || !replayedGrant.Replayed || replayedGrant.LeaseID != grant.LeaseID || replayedGrant.ExpiresAt != grant.ExpiresAt {
		t.Fatalf("replayed ClaimPrecheck() = %#v, %v", replayedGrant, err)
	}
	claimConflict := claim
	claimConflict.RequestDigest = strings.Repeat("c", 64)
	if _, err := store.ClaimPrecheck(context.Background(), claimConflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting ClaimPrecheck() error = %v, want ErrIdempotencyConflict", err)
	}

	completion := AgentPrecheckCompletion{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-complete-persistent", RequestDigest: strings.Repeat("d", 64),
		Results: passedPrecheckResults(), Now: now.Add(time.Minute),
	}
	if _, err := store.CompleteAgentPrecheck(context.Background(), completion); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("CompleteAgentPrecheck() without acknowledgement error = %v, want ErrPrecheckLeaseRejected", err)
	}
	acknowledgement := PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-ack-persistent", RequestDigest: strings.Repeat("e", 64), Now: now.Add(30 * time.Second),
	}
	acknowledgedGrant, err := store.AcknowledgePrecheck(context.Background(), acknowledgement)
	if err != nil || acknowledgedGrant.LeaseID != grant.LeaseID || acknowledgedGrant.Replayed {
		t.Fatalf("AcknowledgePrecheck() = %#v, %v", acknowledgedGrant, err)
	}
	replayedAck, err := store.AcknowledgePrecheck(context.Background(), acknowledgement)
	if err != nil || !replayedAck.Replayed || replayedAck.LeaseEpoch != grant.LeaseEpoch {
		t.Fatalf("replayed AcknowledgePrecheck() = %#v, %v", replayedAck, err)
	}
	crossOperation := acknowledgement
	crossOperation.RequestID = claim.RequestID
	if _, err := store.AcknowledgePrecheck(context.Background(), crossOperation); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("cross-operation request ID error = %v, want ErrIdempotencyConflict", err)
	}

	completed, err := store.CompleteAgentPrecheck(context.Background(), completion)
	if err != nil || completed.Status != "SUCCEEDED" || completed.IntegrityStatus != "COMPLETE" || completed.Replayed {
		t.Fatalf("CompleteAgentPrecheck() = %#v, %v", completed, err)
	}
	replayedCompletion, err := store.CompleteAgentPrecheck(context.Background(), completion)
	if err != nil || !replayedCompletion.Replayed || replayedCompletion.Status != "SUCCEEDED" {
		t.Fatalf("replayed CompleteAgentPrecheck() = %#v, %v", replayedCompletion, err)
	}
	stored, err := store.GetPrecheckRun(context.Background(), run.PrecheckID)
	if err != nil || stored.Status != "SUCCEEDED" || stored.IntegrityStatus != "COMPLETE" {
		t.Fatalf("GetPrecheckRun() after persistent completion = %#v, %v", stored, err)
	}
	if len(stored.Results) != len(fixedPrecheckChecks) || stored.Results[0].Check != fixedPrecheckChecks[0] || stored.Results[0].Status != "PASSED" {
		t.Fatalf("GetPrecheckRun() 未返回固定安全结果: %#v", stored.Results)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM agent_precheck_receipts WHERE precheck_id = 'precheck-persistent'", 3)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXPORT_PRECHECK_COMPLETED' AND object_id = 'precheck-persistent' AND actor_type = 'AGENT' AND actor_id = 'agent-1' AND result = 'SUCCEEDED'", 1)
}

func TestPersistentPrecheckLeaseRejectsSecondLeasedPrecheckForSameAgent(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	first := createClaimablePrecheck(t, store, "precheck-capacity-first")
	second := createClaimablePrecheck(t, store, "precheck-capacity-second")
	now := testTime.Add(time.Minute)

	if _, err := store.ClaimPrecheck(context.Background(), PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: first.PrecheckID, LeaseID: "lease-capacity-first",
		RequestID: "precheck-claim-capacity-first", RequestDigest: strings.Repeat("8", 64), LeaseTTL: 2 * time.Minute, Now: now,
	}); err != nil {
		t.Fatalf("ClaimPrecheck() first = %v", err)
	}
	if _, err := store.ClaimPrecheck(context.Background(), PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: second.PrecheckID, LeaseID: "lease-capacity-second",
		RequestID: "precheck-claim-capacity-second", RequestDigest: strings.Repeat("9", 64), LeaseTTL: 2 * time.Minute, Now: now,
	}); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("ClaimPrecheck() second error = %v, want ErrPrecheckLeaseRejected", err)
	}
	stored, err := store.GetPrecheckRun(context.Background(), second.PrecheckID)
	if err != nil || stored.Status != "PENDING" {
		t.Fatalf("second precheck after rejected claim = %#v, %v", stored, err)
	}
}

func TestPersistentPrecheckCompletionRollsBackWhenAuditCannotBeAppended(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-audit-rollback")
	now := testTime.Add(time.Minute)
	claim := PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-audit-rollback",
		RequestID: "precheck-claim-audit-rollback", RequestDigest: strings.Repeat("1", 64), LeaseTTL: 2 * time.Minute, Now: now,
	}
	grant, err := store.ClaimPrecheck(context.Background(), claim)
	if err != nil {
		t.Fatalf("ClaimPrecheck() = %v", err)
	}
	if _, err := store.AcknowledgePrecheck(context.Background(), PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-ack-audit-rollback", RequestDigest: strings.Repeat("2", 64), Now: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("AcknowledgePrecheck() = %v", err)
	}
	completion := AgentPrecheckCompletion{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-complete-audit-rollback", RequestDigest: strings.Repeat("3", 64),
		Results: passedPrecheckResults(), Now: now.Add(time.Minute),
	}
	if _, err := store.db.ExecContext(context.Background(), `
        INSERT INTO audit_events(
            audit_event_id, actor_type, actor_id, action, object_type, object_id,
            result, request_id, safe_diff_json, occurred_at
        ) VALUES (?, 'SYSTEM', 'test', 'TEST', 'PRECHECK', ?, 'SUCCEEDED', ?, '{}', ?)
    `, auditID(completion.PrecheckID, completion.RequestID), completion.PrecheckID, completion.RequestID, utcText(testTime)); err != nil {
		t.Fatalf("seed conflicting precheck audit: %v", err)
	}
	if _, err := store.CompleteAgentPrecheck(context.Background(), completion); err == nil {
		t.Fatal("CompleteAgentPrecheck() unexpectedly succeeded with conflicting audit")
	}
	stored, err := store.GetPrecheckRun(context.Background(), run.PrecheckID)
	if err != nil || stored.Status != "LEASED" || stored.IntegrityStatus != "UNKNOWN" {
		t.Fatalf("precheck after failed completion = %#v, %v", stored, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM agent_precheck_receipts WHERE precheck_id = 'precheck-audit-rollback' AND operation = 'COMPLETE'", 0)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXPORT_PRECHECK_COMPLETED' AND object_id = 'precheck-audit-rollback'", 0)
}

func TestAgentPrecheckCompletionRejectsUnboundedEvidenceCode(t *testing.T) {
	results := passedPrecheckResults()
	results[3].EvidenceCode = "E_WORKSPACE_SECRET_ABC"
	input := AgentPrecheckCompletion{
		AgentID: "agent-1", PrecheckID: "precheck-1", LeaseID: "lease-1", LeaseEpoch: 1,
		BindingDigest: strings.Repeat("a", 64), RequestID: "precheck-complete-unbounded-evidence", RequestDigest: strings.Repeat("b", 64),
		Results: results, Now: testTime,
	}
	if err := validateAgentPrecheckCompletion(input); err == nil {
		t.Fatal("validateAgentPrecheckCompletion() accepted unbounded evidence code")
	}
}

func TestPrecheckExpiryRollsBackWhenAuditCannotBeAppended(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-expiry-audit-rollback")
	now := run.ValidUntil.Add(time.Minute)
	requestID := "precheck-expiry-" + run.PrecheckID
	if _, err := store.db.ExecContext(context.Background(), `
        INSERT INTO audit_events(
            audit_event_id, actor_type, actor_id, action, object_type, object_id,
            result, request_id, safe_diff_json, occurred_at
        ) VALUES (?, 'SYSTEM', 'test', 'TEST', 'PRECHECK', ?, 'SUCCEEDED', ?, '{}', ?)
    `, auditID(run.PrecheckID, requestID), run.PrecheckID, requestID, utcText(testTime)); err != nil {
		t.Fatalf("seed conflicting expiry audit: %v", err)
	}
	if _, err := store.ExpirePrechecks(context.Background(), now); err == nil {
		t.Fatal("ExpirePrechecks() unexpectedly succeeded with conflicting audit")
	}
	stored, err := store.GetPrecheckRun(context.Background(), run.PrecheckID)
	if err != nil || stored.Status != "PENDING" || stored.IntegrityStatus != "UNKNOWN" {
		t.Fatalf("precheck after failed expiry = %#v, %v", stored, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXPORT_PRECHECK_EXPIRED' AND object_id = 'precheck-expiry-audit-rollback'", 0)
}

func TestPersistentPrecheckLeaseExpiresWithoutRequeue(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-expired")
	now := testTime.Add(time.Minute)
	claim := PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-expired",
		RequestID: "precheck-claim-expired", RequestDigest: strings.Repeat("f", 64), LeaseTTL: time.Minute, Now: now,
	}
	grant, err := store.ClaimPrecheck(context.Background(), claim)
	if err != nil {
		t.Fatalf("ClaimPrecheck() = %v", err)
	}
	if _, err := store.AcknowledgePrecheck(context.Background(), PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-ack-expired", RequestDigest: strings.Repeat("0", 64), Now: now.Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("AcknowledgePrecheck() = %v", err)
	}
	completion := AgentPrecheckCompletion{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-complete-expired", RequestDigest: strings.Repeat("1", 64),
		Results: passedPrecheckResults(), Now: grant.ExpiresAt,
	}
	expiredResult, err := store.CompleteAgentPrecheck(context.Background(), completion)
	if !errors.Is(err, ErrPrecheckLeaseExpired) || expiredResult.Status != "EXPIRED" || expiredResult.IntegrityStatus != "INCOMPLETE" {
		t.Fatalf("expired CompleteAgentPrecheck() error = %v, want ErrPrecheckLeaseExpired", err)
	}
	stored, err := store.GetPrecheckRun(context.Background(), run.PrecheckID)
	if err != nil || stored.Status != "EXPIRED" || stored.IntegrityStatus != "INCOMPLETE" {
		t.Fatalf("expired precheck = %#v, %v", stored, err)
	}
	if expired, err := store.ExpirePrechecks(context.Background(), grant.ExpiresAt.Add(time.Minute)); err != nil || expired != 0 {
		t.Fatalf("ExpirePrechecks() = %d, %v; want 0, nil", expired, err)
	}
	replayedExpiration, err := store.CompleteAgentPrecheck(context.Background(), completion)
	if !errors.Is(err, ErrPrecheckLeaseExpired) || !replayedExpiration.Replayed || replayedExpiration.Status != "EXPIRED" {
		t.Fatalf("replayed expired completion = %#v, %v", replayedExpiration, err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM agent_precheck_receipts WHERE precheck_id = 'precheck-expired' AND status = 'EXPIRED'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXPORT_PRECHECK_EXPIRED' AND object_id = 'precheck-expired' AND actor_type = 'SYSTEM' AND result = 'FAILED'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'EXPORT_PRECHECK_COMPLETION_EXPIRED' AND object_id = 'precheck-expired' AND actor_type = 'AGENT' AND actor_id = 'agent-1' AND result = 'FAILED'", 1)
	claim.RequestID, claim.RequestDigest = "precheck-claim-expired-next", strings.Repeat("2", 64)
	claim.Now = grant.ExpiresAt.Add(time.Minute)
	if _, err := store.ClaimPrecheck(context.Background(), claim); !errors.Is(err, ErrPrecheckLeaseExpired) {
		t.Fatalf("reclaim expired precheck error = %v, want ErrPrecheckLeaseExpired", err)
	}
}

func TestPersistentPrecheckLeaseRejectsFactDrift(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-fact-drift")
	now := testTime.Add(time.Minute)
	grant, err := store.ClaimPrecheck(context.Background(), PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-fact-drift",
		RequestID: "precheck-claim-fact-drift", RequestDigest: strings.Repeat("3", 64), LeaseTTL: 2 * time.Minute, Now: now,
	})
	if err != nil {
		t.Fatalf("ClaimPrecheck() = %v", err)
	}
	if _, err := store.db.Exec(`UPDATE agents SET facts_revision = 2 WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("drift agent facts: %v", err)
	}
	_, err = store.AcknowledgePrecheck(context.Background(), PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-ack-fact-drift", RequestDigest: strings.Repeat("4", 64), Now: now.Add(time.Minute),
	})
	if !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("fact-drift AcknowledgePrecheck() error = %v, want ErrPrecheckLeaseRejected", err)
	}
}

func TestPrecheckSecretResolutionCarriesFrozenOwnerAndBoundNode(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-secret-permission")
	now := testTime.Add(time.Minute)
	grant, err := store.ClaimPrecheck(context.Background(), PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-secret-permission",
		RequestID: "precheck-claim-secret-permission", RequestDigest: strings.Repeat("a", 64), LeaseTTL: 2 * time.Minute, Now: now,
	})
	if err != nil {
		t.Fatalf("ClaimPrecheck() = %v", err)
	}
	if _, err := store.AcknowledgePrecheck(context.Background(), PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-ack-secret-permission", RequestDigest: strings.Repeat("b", 64), Now: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("AcknowledgePrecheck() = %v", err)
	}
	input := PrecheckSecretResolutionRequest{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-secret-permission-request", RequestDigest: strings.Repeat("c", 64), Now: now.Add(2 * time.Second),
	}
	connection, err := store.ResolvePrecheckDatabaseConnection(context.Background(), input)
	if err != nil {
		t.Fatalf("ResolvePrecheckDatabaseConnection() = %v", err)
	}
	defer connection.Destroy()
	if !bytes.Equal(connection.Username, []byte("synthetic_user@synthetic-tenant#synthetic-cluster")) {
		t.Fatalf("precheck JDBC identity = %q", connection.Username)
	}
	if connection.OwnerSubjectID != "subject-1" || connection.NodeID != grant.Binding.NodeID || connection.DataSourceID != grant.Binding.DataSourceID {
		t.Fatalf("permission binding = %#v", struct {
			OwnerSubjectID string
			NodeID         string
			DataSourceID   string
		}{connection.OwnerSubjectID, connection.NodeID, connection.DataSourceID})
	}
	if err := store.FinishPrecheckSecretResolution(context.Background(), PrecheckSecretResolutionOutcome{
		AgentID: input.AgentID, PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
		BindingDigest: input.BindingDigest, RequestID: input.RequestID, RequestDigest: input.RequestDigest, Succeeded: false, Now: now.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("FinishPrecheckSecretResolution(false) = %v", err)
	}
}

func TestPersistentPrecheckLeaseRejectsAgentWithActiveExecution(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	if err := store.SubmitTask(context.Background(), validTaskSubmission("task-precheck-capacity")); err != nil {
		t.Fatalf("SubmitTask() error = %v", err)
	}
	claim := validClaim("execution-precheck-capacity", "execution-lease-precheck-capacity", "event-precheck-capacity", "request-precheck-capacity")
	claim.TaskID = "task-precheck-capacity"
	if err := store.ClaimTask(context.Background(), claim); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}

	run := createClaimablePrecheck(t, store, "precheck-active-execution")
	_, err := store.ClaimPrecheck(context.Background(), PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "precheck-lease-active-execution",
		RequestID: "precheck-request-active-execution", RequestDigest: strings.Repeat("9", 64), LeaseTTL: time.Minute, Now: testTime.Add(time.Minute),
	})
	if !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("ClaimPrecheck() error = %v, want ErrPrecheckLeaseRejected", err)
	}
}

func TestPersistentPrecheckClaimIsAtomicAcrossStoreInstances(t *testing.T) {
	store, databasePath := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-concurrent")
	secondStore, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	t.Cleanup(func() { _ = secondStore.Close() })
	inputs := []PrecheckClaim{
		{AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-concurrent-1", RequestID: "precheck-claim-concurrent-1", RequestDigest: strings.Repeat("5", 64), LeaseTTL: time.Minute, Now: testTime.Add(time.Minute)},
		{AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-concurrent-2", RequestID: "precheck-claim-concurrent-2", RequestDigest: strings.Repeat("6", 64), LeaseTTL: time.Minute, Now: testTime.Add(time.Minute)},
	}
	type outcome struct{ err error }
	start := make(chan struct{})
	results := make(chan outcome, len(inputs))
	for index, input := range inputs {
		candidate := store
		if index == 1 {
			candidate = secondStore
		}
		go func(candidate *Store, input PrecheckClaim) {
			<-start
			_, err := candidate.ClaimPrecheck(context.Background(), input)
			results <- outcome{err: err}
		}(candidate, input)
	}
	close(start)
	successes := 0
	for range inputs {
		result := <-results
		if result.err == nil {
			successes++
			continue
		}
		if !errors.Is(result.err, ErrPrecheckLeaseRejected) {
			t.Fatalf("concurrent ClaimPrecheck() error = %v, want ErrPrecheckLeaseRejected", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent ClaimPrecheck() successes = %d, want 1", successes)
	}
}

func TestClaimNextPrecheckIsAtomicAcrossStoreInstancesAndPendingRecords(t *testing.T) {
	store, databasePath := openTestStore(t)
	seedBaseFixture(t, store)
	createClaimablePrecheck(t, store, "precheck-next-concurrent-first")
	createClaimablePrecheck(t, store, "precheck-next-concurrent-second")
	secondStore, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	t.Cleanup(func() { _ = secondStore.Close() })
	inputs := []PrecheckClaimNext{
		{AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-next-concurrent-1", RequestID: "precheck-next-concurrent-1", RequestDigest: strings.Repeat("c", 64), LeaseTTL: time.Minute, Now: testTime.Add(time.Minute)},
		{AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-next-concurrent-2", RequestID: "precheck-next-concurrent-2", RequestDigest: strings.Repeat("d", 64), LeaseTTL: time.Minute, Now: testTime.Add(time.Minute)},
	}
	type outcome struct {
		found bool
		err   error
	}
	start := make(chan struct{})
	results := make(chan outcome, len(inputs))
	for index, input := range inputs {
		candidate := store
		if index == 1 {
			candidate = secondStore
		}
		go func(candidate *Store, input PrecheckClaimNext) {
			<-start
			_, found, err := candidate.ClaimNextPrecheck(context.Background(), input)
			results <- outcome{found: found, err: err}
		}(candidate, input)
	}
	close(start)
	successes := 0
	for range inputs {
		result := <-results
		if result.err == nil && result.found {
			successes++
			continue
		}
		if !errors.Is(result.err, ErrPrecheckLeaseRejected) || result.found {
			t.Fatalf("concurrent ClaimNextPrecheck() found=%t error=%v, want false and ErrPrecheckLeaseRejected", result.found, result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent ClaimNextPrecheck() successes = %d, want 1", successes)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM precheck_runs WHERE agent_id = 'agent-1' AND status = 'LEASED'", 1)
}

func TestPersistentPrecheckReceiptsSurviveStoreReopen(t *testing.T) {
	store, databasePath := openTestStore(t)
	seedBaseFixture(t, store)
	run := createClaimablePrecheck(t, store, "precheck-reopen")
	now := testTime.Add(time.Minute)
	claim := PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-reopen",
		RequestID: "precheck-claim-reopen", RequestDigest: strings.Repeat("7", 64), LeaseTTL: 2 * time.Minute, Now: now,
	}
	grant, err := store.ClaimPrecheck(context.Background(), claim)
	if err != nil {
		t.Fatalf("ClaimPrecheck() = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}
	reopened, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	replayed, err := reopened.ClaimPrecheck(context.Background(), claim)
	if err != nil || !replayed.Replayed || replayed.LeaseID != grant.LeaseID {
		t.Fatalf("reopened ClaimPrecheck() = %#v, %v", replayed, err)
	}
	ack := PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-ack-reopen", RequestDigest: strings.Repeat("8", 64), Now: now.Add(time.Minute),
	}
	if _, err := reopened.AcknowledgePrecheck(context.Background(), ack); err != nil {
		t.Fatalf("AcknowledgePrecheck() = %v", err)
	}
	completion := AgentPrecheckCompletion{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-complete-reopen", RequestDigest: strings.Repeat("9", 64),
		Results: passedPrecheckResults(), Now: now.Add(time.Minute),
	}
	if _, err := reopened.CompleteAgentPrecheck(context.Background(), completion); err != nil {
		t.Fatalf("CompleteAgentPrecheck() = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened store: %v", err)
	}
	finalStore, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open final store: %v", err)
	}
	t.Cleanup(func() { _ = finalStore.Close() })
	replayedCompletion, err := finalStore.CompleteAgentPrecheck(context.Background(), completion)
	if err != nil || !replayedCompletion.Replayed || replayedCompletion.Status != "SUCCEEDED" {
		t.Fatalf("reopened completion replay = %#v, %v", replayedCompletion, err)
	}
}

func createClaimablePrecheck(t *testing.T, store *Store, precheckID string) PrecheckRun {
	t.Helper()
	if _, err := store.db.Exec(`UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ?, last_test_source = 'AGENT_JDBC' WHERE data_source_id = 'source-1'`, utcText(testTime)); err != nil {
		t.Fatalf("seed connection test: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE agents SET facts_revision = 1 WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("seed agent facts: %v", err)
	}
	input := PrecheckCreate{PrecheckRun: PrecheckRun{
		PrecheckID: precheckID, DraftID: "draft-1", DraftRevision: 1, ConfigFingerprint: testFingerprint,
		DataSourceID: "source-1", CredentialID: "credential-1", CredentialRevision: 1, NodeID: "node-1",
		CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute),
	}, CreatorSubjectID: "subject-1", RequestID: "request-" + precheckID, IdempotencyKey: "idempotency-" + precheckID, RequestDigest: strings.Repeat("a", 64)}
	if _, err := store.CreatePrecheck(context.Background(), input); err != nil {
		t.Fatalf("CreatePrecheck(%q) = %v", precheckID, err)
	}
	run, err := store.GetPrecheckRun(context.Background(), precheckID)
	if err != nil || run.NodeFactsRevision != 1 || len(run.BindingDigest) != 64 {
		t.Fatalf("created precheck = %#v, %v", run, err)
	}
	return run
}

func passedPrecheckResults() []PrecheckCheckResult {
	results := make([]PrecheckCheckResult, 0, len(fixedPrecheckChecks))
	for _, check := range fixedPrecheckChecks {
		results = append(results, PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: "SYNTHETIC_OK"})
	}
	return results
}

// passedStoragePrecheckResults 返回对象存储形态的六项 PASSED 结果（EX-I6 存储专用预检查）。
func passedStoragePrecheckResults() []PrecheckCheckResult {
	results := make([]PrecheckCheckResult, 0, len(storagePrecheckCheckList()))
	for _, check := range storagePrecheckCheckList() {
		evidence := "SYNTHETIC_OK"
		if check == "STORAGE_CONNECTIVITY" {
			evidence = "STORAGE_ENDPOINT_REACHABLE"
		}
		if check == "STORAGE_AUTH" {
			evidence = "STORAGE_CREDENTIAL_VERIFIED"
		}
		results = append(results, PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: evidence})
	}
	return results
}

// storageDraftConfigJSON 是合成 v6 对象存储草稿的持久化配置（扁平投影 + 嵌套标准文档）。
// filePath 为受控 OSS URI（无密钥参数），tmpPath 为节点本地临时分块目录。
const storageDraftConfigJSON = `{"configVersion":"v6","dataSourceId":"source-1","nodeId":"node-1","database":"synthetic_db","scopeKind":"SPECIFIED","table":"synthetic_table","contentKind":"DATA_ONLY","format":"CSV","filePath":"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com","logPath":"","skipCheckDir":false,"config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"OSS","filePath":"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com","tmpPath":"/E:/workespace/tmp"}}}`

// TestParsePrecheckExecutionContextStorageShape 验证 v6 对象存储草稿解析出受控存储目标段，
// 并拒绝 scheme 不符、URI 携带密钥参数、未知查询参数与缺 bucket 的失败关闭输入。
func TestParsePrecheckExecutionContextStorageShape(t *testing.T) {
	t.Parallel()
	context, ok := parsePrecheckExecutionContext("v6", storageDraftConfigJSON)
	if !ok {
		t.Fatal("存储草稿上下文解析失败")
	}
	if context.OutputKind != "OSS" || context.OutputPath != "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com" || context.LogPath != "" {
		t.Fatalf("存储上下文 = %#v", context)
	}
	if context.StorageTarget == nil || context.StorageTarget.Provider != "OSS" || context.StorageTarget.Endpoint != "oss-cn-hangzhou.aliyuncs.com" || context.StorageTarget.TmpPath != "/E:/workespace/tmp" {
		t.Fatalf("存储目标段 = %#v", context.StorageTarget)
	}
	base := func(outputKind, filePath string) string {
		return `{"configVersion":"v6","dataSourceId":"source-1","nodeId":"node-1","database":"synthetic_db","scopeKind":"SPECIFIED","table":"synthetic_table","contentKind":"DATA_ONLY","format":"CSV","filePath":` + strconv.Quote(filePath) + `,"logPath":"","skipCheckDir":false,"config":{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":` + strconv.Quote(outputKind) + `,"filePath":` + strconv.Quote(filePath) + `}}}`
	}
	positive := base("COS", "cos://bucket/path?endpoint=cos.example.com&storage-class=STANDARD")
	if context, ok := parsePrecheckExecutionContext("v6", positive); !ok || context.StorageTarget == nil || context.StorageTarget.Provider != "COS" || context.StorageTarget.Endpoint != "cos.example.com" {
		t.Fatalf("COS storage-class 上下文 = %#v, %t", context, ok)
	}
	negative := map[string]string{
		"scheme 与输出类型不符": base("OSS", "s3://bucket/path?endpoint=host"),
		"URI 携带密钥参数":     base("OSS", "oss://bucket/path?endpoint=host&access-key=AK"),
		"URI 携带未知参数":     base("S3", "s3://bucket/path?region=x&unknown=y"),
		"URI 缺少 bucket":  base("COS", "cos:///path?region=x"),
		"URI 含换行":        base("OSS", "oss://bucket/path\n?endpoint=host"),
	}
	for name, config := range negative {
		if context, ok := parsePrecheckExecutionContext("v6", config); ok {
			t.Fatalf("%s 被接受：%#v", name, context)
		}
	}
}

// TestCompleteAgentPrecheckEnforcesStorageShape 验证正式完成端点按冻结草稿的输出类型
// 复核检查清单：对象存储草稿提交本地六项形态被拒绝，存储六项形态被接受。
func TestCompleteAgentPrecheckEnforcesStorageShape(t *testing.T) {
	t.Parallel()
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	// 插入合成 v6 对象存储草稿（与 seedBaseFixture 的 draft-1 同口径但输出为受控 OSS URI）。
	if _, err := store.db.ExecContext(ctx, `
        INSERT INTO export_drafts(draft_id, owner_subject_id, data_source_id, node_id, revision, tool_version, metadata_version, capability_version, config_version, config_json, config_fingerprint, invalidation_json, created_at, updated_at)
        VALUES ('draft-storage-1', 'subject-1', 'source-1', 'node-1', 1, '4.3.5-RELEASE', 'obdumper-4.3.5-slice-v7', 'export-odp-full-csv-v1', 'v6', ?, ?, '{}', ?, ?)`,
		storageDraftConfigJSON, testFingerprint, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatalf("插入存储草稿失败: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE data_sources SET last_test_status = 'SUCCEEDED', last_tested_at = ?, last_test_source = 'AGENT_JDBC' WHERE data_source_id = 'source-1'`, utcText(testTime)); err != nil {
		t.Fatalf("seed connection test: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE agents SET facts_revision = 1 WHERE agent_id = 'agent-1'`); err != nil {
		t.Fatalf("seed agent facts: %v", err)
	}
	run := PrecheckRun{
		PrecheckID: "precheck-storage", DraftID: "draft-storage-1", DraftRevision: 1, ConfigFingerprint: testFingerprint,
		DataSourceID: "source-1", CredentialID: "credential-1", CredentialRevision: 1, NodeID: "node-1",
		CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute),
	}
	if _, err := store.CreatePrecheck(ctx, PrecheckCreate{
		PrecheckRun: run, CreatorSubjectID: "subject-1", RequestID: "request-precheck-storage",
		IdempotencyKey: "idempotency-precheck-storage", RequestDigest: strings.Repeat("a", 64),
	}); err != nil {
		t.Fatalf("CreatePrecheck(storage) = %v", err)
	}
	now := testTime.Add(time.Minute)
	grant, err := store.ClaimPrecheck(ctx, PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-storage",
		RequestID: "precheck-claim-storage", RequestDigest: strings.Repeat("b", 64), LeaseTTL: 2 * time.Minute, Now: now,
	})
	if err != nil || grant.ExecutionContext.OutputKind != "OSS" || grant.ExecutionContext.StorageTarget == nil || grant.ExecutionContext.StorageTarget.Endpoint != "oss-cn-hangzhou.aliyuncs.com" {
		t.Fatalf("ClaimPrecheck(storage) = %#v, %v", grant, err)
	}
	if _, err := store.AcknowledgePrecheck(ctx, PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "precheck-ack-storage", RequestDigest: strings.Repeat("c", 64), Now: now.Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("AcknowledgePrecheck(storage) = %v", err)
	}
	completion := func(requestID, digest string, results []PrecheckCheckResult) AgentPrecheckCompletion {
		return AgentPrecheckCompletion{
			AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
			BindingDigest: grant.Binding.BindingDigest, RequestID: requestID, RequestDigest: digest, Results: results, Now: now.Add(time.Minute),
		}
	}
	// 负例：本地六项形态（含 OUTPUT_PATH/OUTPUT_EMPTY）与存储草稿不匹配，必须失败关闭。
	if _, err := store.CompleteAgentPrecheck(ctx, completion("precheck-complete-storage-wrong", strings.Repeat("d", 64), passedPrecheckResults())); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("本地形态完成错误 = %v, want ErrPrecheckLeaseRejected", err)
	}
	// 正例：存储六项形态接受，结果按存储清单持久化。
	completed, err := store.CompleteAgentPrecheck(ctx, completion("precheck-complete-storage", strings.Repeat("e", 64), passedStoragePrecheckResults()))
	if err != nil || completed.Status != "SUCCEEDED" || completed.IntegrityStatus != "COMPLETE" {
		t.Fatalf("CompleteAgentPrecheck(storage) = %#v, %v", completed, err)
	}
	stored, err := store.GetPrecheckRun(ctx, run.PrecheckID)
	if err != nil || len(stored.Results) != len(storagePrecheckCheckList()) || stored.Results[4].Check != "STORAGE_CONNECTIVITY" || stored.Results[5].Check != "STORAGE_AUTH" {
		t.Fatalf("存储预检查结果 = %#v, %v", stored.Results, err)
	}
}

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metadata.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

// TestDeriveTaskFromCheckpoint 验证 EX-I8 检查点继续：失败任务 + dump.ckpt 事实满足时
// 派生新任务（parent/derivation + 追加 --retry），不满足条件或重复 --retry 时失败关闭。
func TestDeriveTaskFromCheckpoint(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if err := store.SubmitTask(ctx, validTaskSubmission("task-resume-source")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	claim := validClaim("execution-resume", "lease-resume", "event-resume-claim", "request-resume-claim")
	claim.TaskID = "task-resume-source"
	if err := store.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	appendEvent := func(seq int64, eventID, eventType, payload string) {
		t.Helper()
		if _, err := store.AppendAuthenticatedExecutionEvent(ctx, "agent-1", ExecutionEvent{
			EventID: eventID, ExecutionID: claim.ExecutionID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
			EventSeq: seq, EventType: eventType, PayloadJSON: payload, ReceivedAt: testTime.Add(3 * time.Minute).Add(time.Duration(seq) * 30 * time.Second),
		}); err != nil {
			t.Fatalf("append %s: %v", eventType, err)
		}
	}
	appendEvent(2, "event-resume-ack", "LEASE_ACKNOWLEDGED", `{}`)
	appendEvent(3, "event-resume-started", "PROCESS_STARTED", `{"pid":42,"startedAt":"2026-07-27T01:00:00Z","executableDigest":"`+strings.Repeat("a", 64)+`","bootId":"boot-1"}`)
	appendEvent(4, "event-resume-exited", "PROCESS_EXITED", `{"exitCode":1}`)
	appendEvent(5, "event-resume-terminal", "TOOL_TERMINAL_OBSERVED", `{"terminal":"FAILED"}`)
	appendEvent(6, "event-resume-facts", "RESULT_FACTS_OBSERVED", `{"result":"FAILED","fileCount":1,"totalBytes":10,"files":[{"path":"data_1.csv","size":10}],"checkpointPresent":true}`)
	// 失败终态只接受第一份迟到结果事实，防止不同事件覆盖任务摘要或检查点资格。
	if _, err := store.AppendAuthenticatedExecutionEvent(ctx, "agent-1", ExecutionEvent{
		EventID: "event-resume-facts-conflict", ExecutionID: claim.ExecutionID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
		EventSeq: 7, EventType: "RESULT_FACTS_OBSERVED", PayloadJSON: `{"result":"FAILED","fileCount":0,"totalBytes":0,"files":[],"checkpointPresent":false}`,
		ReceivedAt: testTime.Add(7 * time.Minute),
	}); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("second late result error = %v, want ErrEventRejected", err)
	}
	// 不满足条件的负例：来源任务不存在。
	if _, err := store.DeriveTaskFromCheckpoint(ctx, CheckpointResumeDerivation{
		TaskID: "task-resume-missing", CreatorSubjectID: "subject-1", AuditActorID: "subject-1", SourceTaskID: "task-none",
		RequestID: "request-resume-missing", IdempotencyKey: "idem-resume-missing", RequestDigest: strings.Repeat("a", 64), Now: testTime.Add(5 * time.Minute),
	}); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("missing source error = %v, want ErrDataSourceNotFound", err)
	}
	// 正例：派生继续任务，argv 追加 --retry，parent/derivation 落库。
	result, err := store.DeriveTaskFromCheckpoint(ctx, CheckpointResumeDerivation{
		TaskID: "task-resume-derived", CreatorSubjectID: "subject-1", AuditActorID: "subject-1", SourceTaskID: "task-resume-source",
		RequestID: "request-resume-1", IdempotencyKey: "idem-resume-1", RequestDigest: strings.Repeat("b", 64), Now: testTime.Add(5 * time.Minute),
	})
	if err != nil || result.TaskID != "task-resume-derived" || result.NodeID != "node-1" || result.Replayed {
		t.Fatalf("DeriveTaskFromCheckpoint() = %#v, %v", result, err)
	}
	var parent, derivation, argvJSON string
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(parent_task_id,''), COALESCE(derivation_kind,''), planned_argv_json FROM tasks WHERE task_id = 'task-resume-derived'`).Scan(&parent, &derivation, &argvJSON); err != nil {
		t.Fatalf("read derived task: %v", err)
	}
	if parent != "task-resume-source" || derivation != "CHECKPOINT_RESUME" {
		t.Fatalf("derived relation = %q %q", parent, derivation)
	}
	var argv []string
	if err := json.Unmarshal([]byte(argvJSON), &argv); err != nil || len(argv) < 2 || argv[len(argv)-1] != "--retry" {
		t.Fatalf("derived argv = %s, %v", argvJSON, err)
	}
	// 幂等重放返回同一任务。
	replayed, err := store.DeriveTaskFromCheckpoint(ctx, CheckpointResumeDerivation{
		TaskID: "task-resume-derived-2", CreatorSubjectID: "subject-1", AuditActorID: "subject-1", SourceTaskID: "task-resume-source",
		RequestID: "request-resume-1", IdempotencyKey: "idem-resume-1", RequestDigest: strings.Repeat("b", 64), Now: testTime.Add(5 * time.Minute),
	})
	if err != nil || !replayed.Replayed || replayed.TaskID != "task-resume-derived" || replayed.NodeID != "node-1" {
		t.Fatalf("replayed resume = %#v, %v", replayed, err)
	}
}

// TestDeriveTaskFromCheckpointRejectsResumeOfResumeAndMissingCheckpoint 验证失败关闭边界：
// 已带 --retry 的继续任务不能再次继续；无 dump.ckpt 事实的任务不能继续。
func TestDeriveTaskFromCheckpointRejectsResumeOfResumeAndMissingCheckpoint(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	// 来源任务为 WAITING_SCHEDULE（无执行事实）→ 不满足失败+检查点条件。
	if err := store.SubmitTask(ctx, validTaskSubmission("task-resume-pending")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	if _, err := store.DeriveTaskFromCheckpoint(ctx, CheckpointResumeDerivation{
		TaskID: "task-resume-invalid", CreatorSubjectID: "subject-1", AuditActorID: "subject-1", SourceTaskID: "task-resume-pending",
		RequestID: "request-resume-pending", IdempotencyKey: "idem-resume-pending", RequestDigest: strings.Repeat("c", 64), Now: testTime.Add(5 * time.Minute),
	}); !errors.Is(err, ErrCheckpointResumeUnavailable) {
		t.Fatalf("pending source error = %v, want ErrCheckpointResumeUnavailable", err)
	}
	// 来源任务 argv 已带 --retry（模拟历史继续任务）→ 不能链式继续。
	withRetry := validTaskSubmission("task-resume-already")
	withRetry.PlannedArgvJSON = `["--host","127.0.0.1","--port","2881","--user","synthetic_user@synthetic_tenant","--database","synthetic_db","--table","synthetic_table","--csv","--file-path","/E:/tmp/output","--retry"]`
	withRetry.ParentTaskID, withRetry.DerivationKind = "task-resume-pending", "CHECKPOINT_RESUME"
	if err := store.SubmitTask(ctx, withRetry); err != nil {
		t.Fatalf("SubmitTask(retry): %v", err)
	}
	claim := validClaim("execution-resume-2", "lease-resume-2", "event-resume-2-claim", "request-resume-2-claim")
	claim.TaskID = "task-resume-already"
	if err := store.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	appendEvent := func(seq int64, eventID, eventType, payload string) {
		t.Helper()
		if _, err := store.AppendAuthenticatedExecutionEvent(ctx, "agent-1", ExecutionEvent{
			EventID: eventID, ExecutionID: claim.ExecutionID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
			EventSeq: seq, EventType: eventType, PayloadJSON: payload, ReceivedAt: testTime.Add(3 * time.Minute).Add(time.Duration(seq) * 30 * time.Second),
		}); err != nil {
			t.Fatalf("append %s: %v", eventType, err)
		}
	}
	appendEvent(2, "event-resume2-ack", "LEASE_ACKNOWLEDGED", `{}`)
	appendEvent(3, "event-resume2-started", "PROCESS_STARTED", `{"pid":42,"startedAt":"2026-07-27T01:00:00Z","executableDigest":"`+strings.Repeat("a", 64)+`","bootId":"boot-1"}`)
	appendEvent(4, "event-resume2-exited", "PROCESS_EXITED", `{"exitCode":1}`)
	appendEvent(5, "event-resume2-terminal", "TOOL_TERMINAL_OBSERVED", `{"terminal":"FAILED"}`)
	appendEvent(6, "event-resume2-facts", "RESULT_FACTS_OBSERVED", `{"result":"FAILED","fileCount":1,"totalBytes":10,"files":[{"path":"data_1.csv","size":10}],"checkpointPresent":true}`)
	if _, err := store.DeriveTaskFromCheckpoint(ctx, CheckpointResumeDerivation{
		TaskID: "task-resume-chained", CreatorSubjectID: "subject-1", AuditActorID: "subject-1", SourceTaskID: "task-resume-already",
		RequestID: "request-resume-chained", IdempotencyKey: "idem-resume-chained", RequestDigest: strings.Repeat("d", 64), Now: testTime.Add(5 * time.Minute),
	}); !errors.Is(err, ErrCheckpointResumeUnavailable) {
		t.Fatalf("chained resume error = %v, want ErrCheckpointResumeUnavailable", err)
	}
}

// TestExecutionResultSummaryMerge 验证 EX-I8 结果事实与检查点事实合并进任务级结果摘要，
// 摘要只含受控字段并可经授权详情投影读取。
func TestExecutionResultSummaryMerge(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if err := store.SubmitTask(ctx, validTaskSubmission("task-result")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	claim := validClaim("execution-result", "lease-result", "event-result-claim", "request-result-claim")
	claim.TaskID = "task-result"
	if err := store.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}
	appendEvent := func(seq int64, eventID, eventType, payload string) {
		t.Helper()
		if _, err := store.AppendAuthenticatedExecutionEvent(ctx, "agent-1", ExecutionEvent{
			EventID: eventID, ExecutionID: claim.ExecutionID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
			EventSeq: seq, EventType: eventType, PayloadJSON: payload, ReceivedAt: testTime.Add(3 * time.Minute).Add(time.Duration(seq) * 30 * time.Second),
		}); err != nil {
			t.Fatalf("append %s: %v", eventType, err)
		}
	}
	appendEvent(2, "event-result-ack", "LEASE_ACKNOWLEDGED", `{}`)
	appendEvent(3, "event-result-started", "PROCESS_STARTED", `{"pid":42,"startedAt":"2026-07-27T01:00:00Z","executableDigest":"`+strings.Repeat("a", 64)+`","bootId":"boot-1"}`)
	appendEvent(4, "event-result-exited", "PROCESS_EXITED", `{"exitCode":0}`)
	appendEvent(5, "event-result-terminal", "TOOL_TERMINAL_OBSERVED", `{"terminal":"SUCCEEDED"}`)
	appendEvent(6, "event-result-facts", "RESULT_FACTS_OBSERVED", `{"result":"VERIFIED","fileCount":2,"totalBytes":120,"files":[{"path":"data_1.csv","size":100},{"path":"data_2.csv","size":20}],"checkpointPresent":true}`)
	summary, err := store.GetAuthorizedTaskSummary(ctx, "task-result", "subject-1")
	if err != nil {
		t.Fatalf("GetAuthorizedTaskSummary(): %v", err)
	}
	if summary.State != "SUCCEEDED" || summary.ResultSummary == nil {
		t.Fatalf("任务摘要 = %#v", summary)
	}
	if summary.ResultSummary.Result != "VERIFIED" || summary.ResultSummary.FileCount != 2 || summary.ResultSummary.TotalBytes != 120 || !summary.ResultSummary.CheckpointPresent || len(summary.ResultSummary.Files) != 2 {
		t.Fatalf("结果摘要 = %#v", summary.ResultSummary)
	}
	if summary.ResultSummary.Files[0].Path != "data_1.csv" || summary.ResultSummary.Files[0].Size != 100 {
		t.Fatalf("结果文件清单 = %#v", summary.ResultSummary.Files)
	}
}

// TestExecutionEventValidationResultAndCheckpoint 验证结果/检查点事件负载的失败关闭边界。
func TestExecutionEventValidationResultAndCheckpoint(t *testing.T) {
	t.Parallel()
	validFacts := `{"result":"VERIFIED","fileCount":1,"totalBytes":10,"files":[{"path":"data_1.csv","size":10}],"checkpointPresent":true}`
	if !validProjectedExecutionEvent(ExecutionEvent{EventType: "RESULT_FACTS_OBSERVED", PayloadJSON: validFacts}) {
		t.Fatal("受控结果事实被拒绝")
	}
	negative := map[string]struct {
		eventType string
		payload   string
	}{
		"文件路径为绝对路径":   {"RESULT_FACTS_OBSERVED", `{"result":"VERIFIED","fileCount":1,"totalBytes":10,"files":[{"path":"/E:/tmp/data.csv","size":10}],"checkpointPresent":false}`},
		"文件路径含回退段":    {"RESULT_FACTS_OBSERVED", `{"result":"VERIFIED","fileCount":1,"totalBytes":10,"files":[{"path":"../data.csv","size":10}],"checkpointPresent":false}`},
		"文件路径含换行":     {"RESULT_FACTS_OBSERVED", `{"result":"VERIFIED","fileCount":1,"totalBytes":10,"files":[{"path":"data\n.csv","size":10}],"checkpointPresent":false}`},
		"检查点字段非布尔":    {"RESULT_FACTS_OBSERVED", `{"result":"VERIFIED","fileCount":1,"totalBytes":10,"files":[],"checkpointPresent":"yes"}`},
		"结果事实缺少检查点字段": {"RESULT_FACTS_OBSERVED", `{"result":"VERIFIED","fileCount":1,"totalBytes":10,"files":[]}`},
		"结果事实含多余字段":   {"RESULT_FACTS_OBSERVED", `{"result":"VERIFIED","fileCount":1,"totalBytes":10,"files":[],"checkpointPresent":false,"extra":1}`},
	}
	for name, test := range negative {
		if validProjectedExecutionEvent(ExecutionEvent{EventType: test.eventType, PayloadJSON: test.payload}) {
			t.Fatalf("%s 被接受", name)
		}
	}
}

// TestStorageCredentialLifecycle 验证对象存储凭据（EX-I6 存储凭据槽位）的
// 创建/幂等重放/轮换/删除/越权负例全生命周期。
func TestStorageCredentialLifecycle(t *testing.T) {
	t.Parallel()
	s, _ := openTestStore(t)
	ctx := context.Background()
	subject := "subject-1"
	if err := s.EnsureAuthSubject(ctx, AuthSubject{SubjectID: subject, ExternalSubject: "external-1", DisplayName: "Synthetic User", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
		t.Fatalf("EnsureAuthSubject(): %v", err)
	}
	created, err := s.CreateStorageCredential(ctx, StorageCredentialCreate{
		StorageCredentialID: "storage-1", OwnerSubjectID: subject, DisplayName: "合成 OSS 凭据", Provider: "OSS",
		AccessKey: EncryptedStorageSecret{CredentialID: "storage-key-1", Revision: 1, KeyID: "key-1", Nonce: []byte{1}, Ciphertext: []byte{2}},
		SecretKey: EncryptedStorageSecret{CredentialID: "storage-secret-1", Revision: 1, KeyID: "key-1", Nonce: []byte{3}, Ciphertext: []byte{4}},
		RequestID: "req-1", IdempotencyKey: "idem-1", RequestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: testTime,
	})
	if err != nil || created.StorageCredentialID != "storage-1" || created.Replayed {
		t.Fatalf("CreateStorageCredential() = %#v, %v", created, err)
	}
	// 幂等重放必须返回同一资源且不重复插入。
	replayed, err := s.CreateStorageCredential(ctx, StorageCredentialCreate{
		StorageCredentialID: "storage-1b", OwnerSubjectID: subject, DisplayName: "合成 OSS 凭据", Provider: "OSS",
		AccessKey: EncryptedStorageSecret{CredentialID: "storage-key-1", Revision: 1, KeyID: "key-1", Nonce: []byte{1}, Ciphertext: []byte{2}},
		SecretKey: EncryptedStorageSecret{CredentialID: "storage-secret-1", Revision: 1, KeyID: "key-1", Nonce: []byte{3}, Ciphertext: []byte{4}},
		RequestID: "req-1", IdempotencyKey: "idem-1", RequestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: testTime,
	})
	if err != nil || !replayed.Replayed || replayed.StorageCredentialID != "storage-1" {
		t.Fatalf("replayed create = %#v, %v", replayed, err)
	}
	// 同幂等键异内容必须冲突，不能静默返回旧资源。
	if _, err := s.CreateStorageCredential(ctx, StorageCredentialCreate{
		StorageCredentialID: "storage-1c", OwnerSubjectID: subject, DisplayName: "另一份合成凭据", Provider: "S3",
		AccessKey: EncryptedStorageSecret{CredentialID: "storage-key-1x", Revision: 1, KeyID: "key-1", Nonce: []byte{11}, Ciphertext: []byte{12}},
		SecretKey: EncryptedStorageSecret{CredentialID: "storage-secret-1x", Revision: 1, KeyID: "key-1", Nonce: []byte{13}, Ciphertext: []byte{14}},
		RequestID: "req-1b", IdempotencyKey: "idem-1", RequestDigest: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", CreatedAt: testTime,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrIdempotencyConflict", err)
	}
	// 引用投影必须不含秘密且返回 provider/版本/所有者。
	reference, err := s.GetStorageCredentialReference(ctx, "storage-1")
	if err != nil || reference.Provider != "OSS" || reference.Revision != 1 || reference.OwnerSubjectID != subject {
		t.Fatalf("GetStorageCredentialReference() = %#v, %v", reference, err)
	}
	// 列表只投影安全字段。
	list, err := s.ListStorageCredentials(ctx, subject)
	if err != nil || len(list) != 1 || list[0].DisplayName != "合成 OSS 凭据" || list[0].CurrentRevision != 1 {
		t.Fatalf("ListStorageCredentials() = %#v, %v", list, err)
	}
	// 轮换：两个信封同时轮换，旧修订 SUPERSEDED，版本递增（ExpectedRevision 为当前版本乐观锁）。
	rotated, err := s.RotateStorageCredential(ctx, StorageCredentialRotate{
		StorageCredentialID: "storage-1", ActorSubjectID: subject, ExpectedRevision: 1,
		AccessKey: EncryptedStorageSecret{CredentialID: "storage-key-2", Revision: 2, KeyID: "key-1", Nonce: []byte{5}, Ciphertext: []byte{6}},
		SecretKey: EncryptedStorageSecret{CredentialID: "storage-secret-2", Revision: 2, KeyID: "key-1", Nonce: []byte{7}, Ciphertext: []byte{8}},
		RequestID: "req-2", IdempotencyKey: "idem-2", RequestDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", UpdatedAt: testTime,
	})
	if err != nil || rotated.DisplayName != "合成 OSS 凭据" || rotated.Provider != "OSS" || rotated.CurrentRevision != 2 || rotated.Revision != 2 {
		t.Fatalf("RotateStorageCredential() = %#v, %v", rotated, err)
	}
	var activeCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM storage_credential_revisions WHERE storage_credential_id = 'storage-1' AND status = 'ACTIVE'`).Scan(&activeCount); err != nil {
		t.Fatalf("count active revisions: %v", err)
	}
	if activeCount != 2 {
		t.Fatalf("active revision count = %d, want 2", activeCount)
	}
	var supersededCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM storage_credential_revisions WHERE storage_credential_id = 'storage-1' AND status = 'SUPERSEDED'`).Scan(&supersededCount); err != nil {
		t.Fatalf("count superseded revisions: %v", err)
	}
	if supersededCount != 2 {
		t.Fatalf("superseded revision count = %d, want 2", supersededCount)
	}
	// 越权负例：非所有者轮换与删除必须失败关闭。
	if _, err := s.RotateStorageCredential(ctx, StorageCredentialRotate{
		StorageCredentialID: "storage-1", ActorSubjectID: "subject-other", ExpectedRevision: 2,
		AccessKey: EncryptedStorageSecret{CredentialID: "storage-key-3", Revision: 3, KeyID: "key-1", Nonce: []byte{9}, Ciphertext: []byte{10}},
		SecretKey: EncryptedStorageSecret{CredentialID: "storage-secret-3", Revision: 3, KeyID: "key-1", Nonce: []byte{11}, Ciphertext: []byte{12}},
		RequestID: "req-3", IdempotencyKey: "idem-3", RequestDigest: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", UpdatedAt: testTime,
	}); !errors.Is(err, ErrStorageCredentialForbidden) {
		t.Fatalf("foreign rotate error = %v, want ErrStorageCredentialForbidden", err)
	}
	if err := s.DeleteStorageCredential(ctx, StorageCredentialDeletion{
		StorageCredentialID: "storage-1", ActorSubjectID: "subject-other", ExpectedRevision: 2, RequestID: "req-4", DeletedAt: testTime,
	}); !errors.Is(err, ErrStorageCredentialForbidden) {
		t.Fatalf("foreign delete error = %v, want ErrStorageCredentialForbidden", err)
	}
	// 版本冲突负例。
	if err := s.DeleteStorageCredential(ctx, StorageCredentialDeletion{
		StorageCredentialID: "storage-1", ActorSubjectID: subject, ExpectedRevision: 99, RequestID: "req-5", DeletedAt: testTime,
	}); !errors.Is(err, ErrStorageCredentialRevision) {
		t.Fatalf("stale delete error = %v, want ErrStorageCredentialRevision", err)
	}
	// 所有者删除：物理删除主表与全部信封。
	if err := s.DeleteStorageCredential(ctx, StorageCredentialDeletion{
		StorageCredentialID: "storage-1", ActorSubjectID: subject, ExpectedRevision: 2, RequestID: "req-6", DeletedAt: testTime,
	}); err != nil {
		t.Fatalf("DeleteStorageCredential() = %v", err)
	}
	if _, err := s.GetStorageCredentialReference(ctx, "storage-1"); !errors.Is(err, ErrStorageCredentialNotFound) {
		t.Fatalf("reference after delete error = %v, want ErrStorageCredentialNotFound", err)
	}
	var remaining int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM storage_credential_revisions WHERE storage_credential_id = 'storage-1'`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining revisions: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining revisions = %d, want 0", remaining)
	}
}

// TestResolveExecutionStorageCredentialReturnsEnvelopeIDs 验证执行槽位解析返回加密信封自身的
// credentialId（AAD 绑定项），控制面解密必须使用它们而不是 storageCredentialId；
// 本地输出任务不绑定凭据时必须返回空结构且不报错。
func TestResolveExecutionStorageCredentialReturnsEnvelopeIDs(t *testing.T) {
	t.Parallel()
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	if _, err := s.CreateStorageCredential(ctx, StorageCredentialCreate{
		StorageCredentialID: "storage-exec-1", OwnerSubjectID: "subject-1", DisplayName: "执行槽位 OSS 凭据", Provider: "OSS",
		AccessKey: EncryptedStorageSecret{CredentialID: "storage-envelope-access-1", Revision: 1, KeyID: "key-1", Nonce: []byte{21}, Ciphertext: []byte{22}},
		SecretKey: EncryptedStorageSecret{CredentialID: "storage-envelope-secret-1", Revision: 1, KeyID: "key-1", Nonce: []byte{23}, Ciphertext: []byte{24}},
		RequestID: "req-exec-1", IdempotencyKey: "idem-exec-1", RequestDigest: testFingerprint, CreatedAt: testTime,
	}); err != nil {
		t.Fatalf("CreateStorageCredential(): %v", err)
	}
	storageSubmission := validTaskSubmission("task-storage-1")
	storageSubmission.StorageCredentialID = "storage-exec-1"
	storageSubmission.StorageCredentialRevision = 1
	if err := s.SubmitTask(ctx, storageSubmission); err != nil {
		t.Fatalf("SubmitTask(storage): %v", err)
	}
	claim := validClaim("execution-storage-1", "lease-storage-1", "event-storage-1", "request-storage-1")
	claim.TaskID = "task-storage-1"
	if err := s.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("ClaimTask(storage): %v", err)
	}
	// 任务已冻结版本 1 后轮换到版本 2；旧修订虽已 SUPERSEDED，排队/已领取任务仍须可解析。
	if _, err := s.RotateStorageCredential(ctx, StorageCredentialRotate{
		StorageCredentialID: "storage-exec-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		AccessKey: EncryptedStorageSecret{CredentialID: "storage-envelope-access-2", Revision: 2, KeyID: "key-1", Nonce: []byte{25}, Ciphertext: []byte{26}},
		SecretKey: EncryptedStorageSecret{CredentialID: "storage-envelope-secret-2", Revision: 2, KeyID: "key-1", Nonce: []byte{27}, Ciphertext: []byte{28}},
		RequestID: "req-exec-rotate", IdempotencyKey: "idem-exec-rotate", RequestDigest: strings.Repeat("d", 64), UpdatedAt: testTime.Add(time.Minute),
	}); err != nil {
		t.Fatalf("RotateStorageCredential(): %v", err)
	}
	resolved, err := s.ResolveExecutionStorageCredential(ctx, ExecutionSecretResolutionRequest{
		AgentID: "agent-1", ExecutionID: "execution-storage-1", LeaseID: "lease-storage-1", LeaseEpoch: 1,
		EnvelopeDigest: strings.Repeat("a", 64), RequestID: "request-resolve-1", RequestDigest: strings.Repeat("b", 64),
		Now: testTime.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ResolveExecutionStorageCredential(): %v", err)
	}
	defer resolved.Destroy()
	if resolved.StorageCredentialID != "storage-exec-1" || resolved.Provider != "OSS" || resolved.Revision != 1 ||
		resolved.AccessKeyCredentialID != "storage-envelope-access-1" || resolved.SecretKeyCredentialID != "storage-envelope-secret-1" {
		t.Fatalf("ResolveExecutionStorageCredential() = %#v", resolved)
	}
	// 本地输出任务：空结构、无错误，调用方据此跳过存储注入。
	localSubmission := validTaskSubmission("task-local-1")
	if err := s.SubmitTask(ctx, localSubmission); err != nil {
		t.Fatalf("SubmitTask(local): %v", err)
	}
	localClaim := validClaim("execution-local-1", "lease-local-1", "event-local-1", "request-local-1")
	localClaim.TaskID = "task-local-1"
	if err := s.ClaimTask(ctx, localClaim); err != nil {
		t.Fatalf("ClaimTask(local): %v", err)
	}
	local, err := s.ResolveExecutionStorageCredential(ctx, ExecutionSecretResolutionRequest{
		AgentID: "agent-1", ExecutionID: "execution-local-1", LeaseID: "lease-local-1", LeaseEpoch: 1,
		EnvelopeDigest: strings.Repeat("a", 64), RequestID: "request-resolve-2", RequestDigest: strings.Repeat("c", 64),
		Now: testTime.Add(4 * time.Minute),
	})
	if err != nil || local.StorageCredentialID != "" {
		t.Fatalf("local ResolveExecutionStorageCredential() = %#v, %v", local, err)
	}
}

// TestPrecheckStorageCredentialFrozenAndResolved 验证 EX-V1 预检查存储凭据链路：
// 创建时冻结引用（写入 precheck_runs），已确认租约内解析返回加密信封（AAD 用信封自身 credentialId），
// 未绑定凭据与越权租约均失败关闭。
func TestPrecheckStorageCredentialFrozenAndResolved(t *testing.T) {
	t.Parallel()
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	now := testTime.Add(time.Minute)
	// 未绑定存储凭据的预检查：不得请求 STORAGE_CREDENTIAL 槽位。
	local := createClaimablePrecheck(t, s, "precheck-storage-local-1")
	localGrant, err := s.ClaimPrecheck(ctx, PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: "precheck-storage-local-1", LeaseID: "lease-precheck-local-1",
		RequestID: "claim-precheck-local-1", RequestDigest: strings.Repeat("c", 64), LeaseTTL: 2 * time.Minute, Now: now,
	})
	if err != nil {
		t.Fatalf("ClaimPrecheck(local) = %v", err)
	}
	if _, err := s.AcknowledgePrecheck(ctx, PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: local.PrecheckID, LeaseID: localGrant.LeaseID, LeaseEpoch: localGrant.LeaseEpoch,
		BindingDigest: localGrant.Binding.BindingDigest, RequestID: "ack-precheck-local-1", RequestDigest: strings.Repeat("d", 64), Now: now.Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("AcknowledgePrecheck(local) = %v", err)
	}
	_, err = s.ResolvePrecheckStorageCredential(ctx, PrecheckSecretResolutionRequest{
		AgentID: "agent-1", PrecheckID: local.PrecheckID, LeaseID: localGrant.LeaseID, LeaseEpoch: localGrant.LeaseEpoch,
		BindingDigest: localGrant.Binding.BindingDigest, RequestID: "resolve-precheck-local-1", RequestDigest: strings.Repeat("e", 64), Now: now.Add(time.Minute),
	})
	if !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("local ResolvePrecheckStorageCredential() error = %v, want ErrPrecheckLeaseRejected", err)
	}
	if _, err := s.CompleteAgentPrecheck(ctx, AgentPrecheckCompletion{
		AgentID: "agent-1", PrecheckID: local.PrecheckID, LeaseID: localGrant.LeaseID, LeaseEpoch: localGrant.LeaseEpoch,
		BindingDigest: localGrant.Binding.BindingDigest, RequestID: "complete-precheck-local-1", RequestDigest: strings.Repeat("0", 64),
		Results: passedPrecheckResults(), Now: now.Add(90 * time.Second),
	}); err != nil {
		t.Fatalf("CompleteAgentPrecheck(local) = %v", err)
	}
	if _, err := s.CreateStorageCredential(ctx, StorageCredentialCreate{
		StorageCredentialID: "storage-precheck-1", OwnerSubjectID: "subject-1", DisplayName: "预检查槽位 OSS 凭据", Provider: "OSS",
		AccessKey: EncryptedStorageSecret{CredentialID: "precheck-envelope-access-1", Revision: 1, KeyID: "key-1", Nonce: []byte{31}, Ciphertext: []byte{32}},
		SecretKey: EncryptedStorageSecret{CredentialID: "precheck-envelope-secret-1", Revision: 1, KeyID: "key-1", Nonce: []byte{33}, Ciphertext: []byte{34}},
		RequestID: "req-precheck-storage", IdempotencyKey: "idem-precheck-storage", RequestDigest: testFingerprint, CreatedAt: testTime,
	}); err != nil {
		t.Fatalf("CreateStorageCredential(): %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
        INSERT INTO export_drafts(draft_id, owner_subject_id, data_source_id, node_id, revision, tool_version, metadata_version, capability_version, config_version, config_json, config_fingerprint, invalidation_json, created_at, updated_at)
        VALUES ('draft-storage-secret-1', 'subject-1', 'source-1', 'node-1', 1, '4.3.5-RELEASE', 'obdumper-4.3.5-slice-v7', 'export-odp-full-csv-v1', 'v6', ?, ?, '{}', ?, ?)
    `, storageDraftConfigJSON, testFingerprint, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatalf("创建合成对象存储草稿: %v", err)
	}
	input := PrecheckCreate{PrecheckRun: PrecheckRun{
		PrecheckID: "precheck-storage-1", DraftID: "draft-storage-secret-1", DraftRevision: 1, ConfigFingerprint: testFingerprint,
		DataSourceID: "source-1", CredentialID: "credential-1", CredentialRevision: 1, NodeID: "node-1",
		CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute),
		// EX-V1：对象存储草稿预检查冻结存储凭据引用。
		StorageCredentialID: "storage-precheck-1", StorageCredentialRevision: 1,
	}, CreatorSubjectID: "subject-1", RequestID: "request-precheck-storage-1", IdempotencyKey: "idempotency-precheck-storage-1", RequestDigest: strings.Repeat("f", 64)}
	if _, err := s.CreatePrecheck(ctx, input); err != nil {
		t.Fatalf("CreatePrecheck(storage): %v", err)
	}
	run, err := s.GetPrecheckRun(ctx, "precheck-storage-1")
	if err != nil || run.StorageCredentialID != "storage-precheck-1" || run.StorageCredentialRevision != 1 {
		t.Fatalf("冻结的预检查引用 = %#v, %v", run, err)
	}
	grant, err := s.ClaimPrecheck(ctx, PrecheckClaim{
		AgentID: "agent-1", NodeID: "node-1", PrecheckID: "precheck-storage-1", LeaseID: "lease-precheck-storage-1",
		RequestID: "claim-precheck-storage-1", RequestDigest: strings.Repeat("0", 64), LeaseTTL: 2 * time.Minute, Now: now,
	})
	if err != nil {
		t.Fatalf("ClaimPrecheck(storage) = %v", err)
	}
	if _, err := s.AcknowledgePrecheck(ctx, PrecheckAcknowledgement{
		AgentID: "agent-1", PrecheckID: "precheck-storage-1", LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "ack-precheck-storage-1", RequestDigest: strings.Repeat("a", 64), Now: now.Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("AcknowledgePrecheck(storage) = %v", err)
	}
	resolved, err := s.ResolvePrecheckStorageCredential(ctx, PrecheckSecretResolutionRequest{
		AgentID: "agent-1", PrecheckID: "precheck-storage-1", LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "resolve-precheck-storage-1", RequestDigest: strings.Repeat("b", 64), Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ResolvePrecheckStorageCredential() = %v", err)
	}
	defer resolved.Destroy()
	if resolved.StorageCredentialID != "storage-precheck-1" || resolved.Provider != "OSS" || resolved.Revision != 1 ||
		resolved.OwnerSubjectID != "subject-1" || resolved.DataSourceID != "source-1" || resolved.NodeID != "node-1" ||
		resolved.AccessKeyCredentialID != "precheck-envelope-access-1" || resolved.SecretKeyCredentialID != "precheck-envelope-secret-1" {
		t.Fatalf("ResolvePrecheckStorageCredential() = %#v", resolved)
	}
	if err := s.FinishPrecheckSecretResolution(ctx, PrecheckSecretResolutionOutcome{
		AgentID: "agent-1", PrecheckID: "precheck-storage-1", LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "resolve-precheck-storage-1", RequestDigest: strings.Repeat("b", 64),
		Succeeded: true, Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("FinishPrecheckSecretResolution(storage) = %v", err)
	}
	// 越权租约（非本 Agent 领取）必须失败关闭。
	if _, err := s.ResolvePrecheckStorageCredential(ctx, PrecheckSecretResolutionRequest{
		AgentID: "agent-other", PrecheckID: "precheck-storage-1", LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "resolve-precheck-foreign-1", RequestDigest: strings.Repeat("f", 64), Now: now.Add(time.Minute),
	}); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatalf("foreign resolve error = %v, want ErrPrecheckLeaseRejected", err)
	}
}

func seedBaseFixture(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	precheckValidUntil := testTime.Add(10 * time.Minute)
	binding := PrecheckBinding{
		PrecheckID: "precheck-1", DraftID: "draft-1", DraftRevision: 1, ConfigFingerprint: testFingerprint,
		DataSourceID: "source-1", CredentialID: "credential-1", CredentialRevision: 1, NodeID: "node-1",
		NodeFactsRevision: 1, BindingAgentID: "agent-1", ValidUntil: precheckValidUntil,
	}
	bindingDigest, err := precheckBindingDigest(binding)
	if err != nil {
		t.Fatalf("计算合成预检查绑定摘要: %v", err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO auth_subjects VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, []any{"subject-1", "external-1", "Synthetic User", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO data_sources(
                data_source_id, display_name, normalized_name, environment, connection_kind,
                compatibility_mode, host, port, cluster_name, tenant_name, username, default_database,
                credential_id, current_credential_revision, state, revision, last_test_status,
                last_tested_at, last_test_safe_summary_json, last_test_source, created_by, created_at, updated_at
            ) VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', ?, 2881, 'synthetic-cluster', 'synthetic-tenant', ?, ?, ?, 1, 'ENABLED', 1, 'SUCCEEDED', ?, '{}', 'AGENT_JDBC', ?, ?, ?)`, []any{"source-1", "Synthetic Source", "synthetic source", "127.0.0.1", "synthetic_user", "synthetic_db", "credential-1", utcText(testTime), "subject-1", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO credential_revisions VALUES (?, 1, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)`, []any{"credential-1", "source-1", "key-1", []byte{1, 2, 3}, []byte{4, 5, 6}, utcText(testTime)}},
		{`INSERT INTO execution_nodes(node_id, display_name, normalized_name, platform, management_state, allowed_roots_json, tool_home, java_path, tool_config_ref, revision, created_by, created_at, updated_at) VALUES (?, ?, ?, 'WINDOWS_AMD64', 'ENABLED', '["E:\\tmp"]', 'E:\\tools\\ob-loader-dumper-4.3.5', 'C:\\Java\\bin\\java.exe', NULL, 1, ?, ?, ?)`, []any{"node-1", "Synthetic Node", "synthetic node", "subject-1", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO agents(
            agent_id, node_id, credential_digest, credential_revision, status,
            protocol_version, boot_id, last_heartbeat_at, capacity_total, capacity_used,
            facts_json, facts_revision, created_at, revoked_at
        ) VALUES (?, ?, ?, 1, 'ACTIVE', ?, ?, ?, 1, 0, NULL, 1, ?, NULL)`, []any{"agent-1", "node-1", []byte{7, 8, 9}, "agent-v1", "boot-1", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO export_drafts(draft_id, owner_subject_id, data_source_id, node_id, revision, tool_version, metadata_version, capability_version, config_json, config_fingerprint, invalidation_json, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?, ?, '{"database":"synthetic_db","table":"synthetic_table","format":"CSV","filePath":"/E:/tmp/output"}', ?, '{}', ?, ?)`, []any{"draft-1", "subject-1", "source-1", "node-1", "4.3.5-RELEASE", "obdumper-4.3.5-slice-v3", "export-odp-single-table-csv-v1", testFingerprint, utcText(testTime), utcText(testTime)}},
		{`INSERT INTO precheck_runs(
            precheck_id, draft_id, draft_revision, config_fingerprint, data_source_id,
            credential_id, credential_revision, node_id, agent_id, status, lease_id,
            lease_epoch, lease_expires_at, result_json, integrity_status, valid_until,
            created_at, completed_at, node_facts_revision, binding_digest, binding_agent_id
        ) VALUES (?, ?, 1, ?, ?, ?, 1, ?, ?, 'SUCCEEDED', NULL, NULL, NULL, '{}', 'COMPLETE', ?, ?, ?, 1, ?, ?)`, []any{"precheck-1", "draft-1", testFingerprint, "source-1", "credential-1", "node-1", "agent-1", utcText(precheckValidUntil), utcText(testTime), utcText(testTime), bindingDigest, "agent-1"}},
	}
	for index, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed statement %d: %v", index+1, err)
		}
	}
}

func validTaskSubmission(taskID string) TaskSubmission {
	return TaskSubmission{
		TaskID:                 taskID,
		CreatorSubjectID:       "subject-1",
		AuditActorID:           "subject-1",
		DataSourceID:           "source-1",
		NodeID:                 "node-1",
		PrecheckID:             "precheck-1",
		CredentialID:           "credential-1",
		CredentialRevision:     1,
		ConfigFingerprint:      testFingerprint,
		ToolVersion:            "4.3.5-RELEASE",
		MetadataVersion:        "obdumper-4.3.5-slice-v3",
		CapabilityVersion:      "export-odp-single-table-csv-v1",
		SnapshotVersion:        "v1",
		SnapshotJSON:           `{"credentialReference":{"credentialId":"credential-1","revision":1}}`,
		PlannedArgvJSON:        `["--host","127.0.0.1","--port","2881","--user","synthetic_user@synthetic_tenant","--database","synthetic_db","--table","synthetic_table","--csv","--file-path","/E:/tmp/output"]`,
		PlannedCommandRedacted: `obdumper --host 127.0.0.1 --port 2881 --user ****** --database synthetic_db --table synthetic_table --csv --file-path /E:/tmp/output`,
		RequestID:              "request-submit-1",
		SubmittedAt:            testTime.Add(2 * time.Minute),
	}
}

func validClaim(executionID, leaseID, eventID, requestID string) Claim {
	return Claim{
		ExecutionID: executionID,
		TaskID:      "task-1",
		NodeID:      "node-1",
		AgentID:     "agent-1",
		LeaseID:     leaseID,
		LeaseEpoch:  1,
		IssuedAt:    testTime.Add(3 * time.Minute),
		ExpiresAt:   testTime.Add(8 * time.Minute),
		EventID:     eventID,
		RequestID:   requestID,
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("count query %q = %d, want %d", query, got, want)
	}
}

// TestDataSourceSysCredentialLifecycle 验证可选 sys 凭据（参考 ODC 数据源高级设置）：
// 创建时成对持久化 SYS_PASSWORD 修订，轮换递增修订，清除时标记 REVOKED 并清空投影。
func TestDataSourceSysCredentialLifecycle(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	base := DataSourceCreate{
		DataSourceID: "source-sys", CredentialID: "credential-sys", CreatorSubjectID: "subject-1",
		DisplayName: "Sys Source", NormalizedName: "sys-source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.4", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", KeyID: "key-sys",
		Nonce: []byte{1}, Ciphertext: []byte{2}, RequestID: "request-sys-1",
		IdempotencyKey: "idempotency-sys-1", RequestDigest: testFingerprint, CreatedAt: testTime,
	}
	// 非成对必须拒绝：有密码信封但没有账号。
	invalid := base
	invalid.SysPassword = &EncryptedDataSourcePassword{CredentialID: "sys-credential-1", Revision: 1, KeyID: "key-sys", Nonce: []byte{9}, Ciphertext: []byte{8}}
	if _, err := store.CreateDataSource(ctx, invalid); err == nil {
		t.Fatal("CreateDataSource() must reject a sys password without a sys user")
	}
	// 成对创建。
	valid := base
	valid.SysUser = "root"
	valid.SysPassword = &EncryptedDataSourcePassword{CredentialID: "sys-credential-1", Revision: 1, KeyID: "key-sys", Nonce: []byte{9}, Ciphertext: []byte{8}}
	created, err := store.CreateDataSource(ctx, valid)
	if err != nil || created.DataSourceID != valid.DataSourceID {
		t.Fatalf("CreateDataSource(sys) = %#v, %v", created, err)
	}
	summary, err := store.GetDataSourceSummary(ctx, "source-sys")
	if err != nil || summary.SysUser != "root" || summary.SysCredentialRevision != 1 || summary.SysCredentialID != "sys-credential-1" {
		t.Fatalf("GetDataSourceSummary(sys) = %#v, %v", summary, err)
	}
	// 轮换：修订 1 -> 2，旧修订 SUPERSEDED。
	rotated, err := store.UpdateDataSource(ctx, DataSourceUpdate{
		DataSourceID: "source-sys", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		DisplayName: "Sys Source", NormalizedName: "sys-source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.4", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user",
		SysUser:     "root",
		SysPassword: &EncryptedDataSourcePassword{CredentialID: "sys-credential-1", Revision: 2, KeyID: "key-sys", Nonce: []byte{7}, Ciphertext: []byte{6}},
		RequestID:   "request-sys-2", UpdatedAt: testTime.Add(time.Minute),
	})
	if err != nil || rotated.Revision != 2 {
		t.Fatalf("UpdateDataSource(rotate sys) = %#v, %v", rotated, err)
	}
	afterRotation, err := store.GetDataSourceSummary(ctx, "source-sys")
	if err != nil || afterRotation.SysCredentialRevision != 2 {
		t.Fatalf("sys revision after rotation = %#v, %v", afterRotation, err)
	}
	// 清除：账号置空、修订标记 REVOKED。
	cleared, err := store.UpdateDataSource(ctx, DataSourceUpdate{
		DataSourceID: "source-sys", ActorSubjectID: "subject-1", ExpectedRevision: 2,
		DisplayName: "Sys Source", NormalizedName: "sys-source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.4", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user",
		ClearSysCredential: true, RequestID: "request-sys-3", UpdatedAt: testTime.Add(2 * time.Minute),
	})
	if err != nil || cleared.Revision != 3 {
		t.Fatalf("UpdateDataSource(clear sys) = %#v, %v", cleared, err)
	}
	afterClear, err := store.GetDataSourceSummary(ctx, "source-sys")
	if err != nil || afterClear.SysUser != "" || afterClear.SysCredentialRevision != 0 || afterClear.SysCredentialID != "" {
		t.Fatalf("sys projection after clear = %#v, %v", afterClear, err)
	}
	var revokedCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sys_credential_revisions WHERE data_source_id = 'source-sys' AND status = 'REVOKED'`).Scan(&revokedCount); err != nil || revokedCount != 1 {
		t.Fatalf("revoked sys revisions = %d, %v", revokedCount, err)
	}
	// 清除后重新配置使用新的凭据标识并从修订 1 开始，不能与历史已撤销修订冲突。
	reconfigured, err := store.UpdateDataSource(ctx, DataSourceUpdate{
		DataSourceID: "source-sys", ActorSubjectID: "subject-1", ExpectedRevision: 3,
		DisplayName: "Sys Source", NormalizedName: "sys-source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.4", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user",
		SysUser:     "root2",
		SysPassword: &EncryptedDataSourcePassword{CredentialID: "sys-credential-2", Revision: 1, KeyID: "key-sys", Nonce: []byte{5}, Ciphertext: []byte{4}},
		RequestID:   "request-sys-4", UpdatedAt: testTime.Add(3 * time.Minute),
	})
	if err != nil || reconfigured.Revision != 4 || !reconfigured.ConnectionTestInvalidated {
		t.Fatalf("UpdateDataSource(reconfigure sys) = %#v, %v", reconfigured, err)
	}
	afterReconfigure, err := store.GetDataSourceSummary(ctx, "source-sys")
	if err != nil || afterReconfigure.SysUser != "root2" || afterReconfigure.SysCredentialID != "sys-credential-2" || afterReconfigure.SysCredentialRevision != 1 {
		t.Fatalf("sys projection after reconfigure = %#v, %v", afterReconfigure, err)
	}
	// 清除与轮换同时发生必须拒绝。
	conflict := DataSourceUpdate{
		DataSourceID: "source-sys", ActorSubjectID: "subject-1", ExpectedRevision: 4,
		DisplayName: "Sys Source", NormalizedName: "sys-source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.4", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user",
		ClearSysCredential: true,
		SysPassword:        &EncryptedDataSourcePassword{CredentialID: "sys-credential-1", Revision: 1, KeyID: "key-sys", Nonce: []byte{5}, Ciphertext: []byte{4}},
		RequestID:          "request-sys-5", UpdatedAt: testTime.Add(4 * time.Minute),
	}
	if _, err := store.UpdateDataSource(ctx, conflict); err == nil {
		t.Fatal("UpdateDataSource() must reject clearing and rotating sys credential at once")
	}
}

// TestExportConfigTemplateLifecycle 验证 EX-I8 模板复用存储生命周期：
// 创建/幂等重放/列表/授权读取/改名/删除/越权与版本冲突负例。
func TestExportConfigTemplateLifecycle(t *testing.T) {
	t.Parallel()
	s, _ := openTestStore(t)
	ctx := context.Background()
	if err := s.EnsureAuthSubject(ctx, AuthSubject{SubjectID: "subject-1", ExternalSubject: "external-1", DisplayName: "Synthetic User", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
		t.Fatalf("EnsureAuthSubject(): %v", err)
	}
	created, err := s.CreateExportConfigTemplate(ctx, ExportConfigTemplateCreate{
		ExportConfigTemplate: ExportConfigTemplate{
			TemplateID: "template-1", OwnerSubjectID: "subject-1", DisplayName: "合成 CSV 模板",
			CapabilityVersion: "export-odp-full-csv-v1", ConfigJSON: `{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}`,
			ConfigFingerprint: testFingerprint, CreatedAt: testTime, UpdatedAt: testTime,
		},
		RequestID: "request-template-1", IdempotencyKey: "idem-template-1", RequestDigest: strings.Repeat("a", 64),
	})
	if err != nil || created.TemplateID != "template-1" || created.Replayed {
		t.Fatalf("CreateExportConfigTemplate() = %#v, %v", created, err)
	}
	replayed, err := s.CreateExportConfigTemplate(ctx, ExportConfigTemplateCreate{
		ExportConfigTemplate: ExportConfigTemplate{
			TemplateID: "template-1b", OwnerSubjectID: "subject-1", DisplayName: "合成 CSV 模板",
			CapabilityVersion: "export-odp-full-csv-v1", ConfigJSON: `{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/E:/tmp/out"}}`,
			ConfigFingerprint: testFingerprint, CreatedAt: testTime, UpdatedAt: testTime,
		},
		RequestID: "request-template-1", IdempotencyKey: "idem-template-1", RequestDigest: strings.Repeat("a", 64),
	})
	if err != nil || !replayed.Replayed || replayed.TemplateID != "template-1" {
		t.Fatalf("replayed template create = %#v, %v", replayed, err)
	}
	// 列表不返回配置 JSON；授权读取返回完整模板。
	list, err := s.ListExportConfigTemplates(ctx, "subject-1")
	if err != nil || len(list) != 1 || list[0].DisplayName != "合成 CSV 模板" || list[0].ConfigJSON != "" {
		t.Fatalf("ListExportConfigTemplates() = %#v, %v", list, err)
	}
	got, err := s.GetAuthorizedExportConfigTemplate(ctx, "template-1", "subject-1")
	if err != nil || got.ConfigJSON == "" {
		t.Fatalf("GetAuthorizedExportConfigTemplate() = %#v, %v", got, err)
	}
	// 越权与不存在同错误。
	if _, err := s.GetAuthorizedExportConfigTemplate(ctx, "template-1", "subject-other"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("foreign template read error = %v, want ErrDataSourceNotFound", err)
	}
	// 改名与版本冲突。
	revision, err := s.UpdateExportConfigTemplate(ctx, ExportConfigTemplateUpdate{
		TemplateID: "template-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		DisplayName: "改名后的模板", RequestID: "request-template-2", UpdatedAt: testTime,
	})
	if err != nil || revision != 2 {
		t.Fatalf("UpdateExportConfigTemplate() = %d, %v", revision, err)
	}
	if _, err := s.UpdateExportConfigTemplate(ctx, ExportConfigTemplateUpdate{
		TemplateID: "template-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		DisplayName: "再次改名", RequestID: "request-template-3", UpdatedAt: testTime,
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale rename error = %v, want ErrRevisionConflict", err)
	}
	// 越权删除失败关闭；所有者删除成功。
	if err := s.DeleteExportConfigTemplate(ctx, "template-1", "subject-other", 2, "request-template-4", testTime); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("foreign delete error = %v, want ErrDataSourceNotFound", err)
	}
	if err := s.DeleteExportConfigTemplate(ctx, "template-1", "subject-1", 2, "request-template-5", testTime); err != nil {
		t.Fatalf("DeleteExportConfigTemplate() = %v", err)
	}
	left, err := s.ListExportConfigTemplates(ctx, "subject-1")
	if err != nil || len(left) != 0 {
		t.Fatalf("list after delete = %#v, %v", left, err)
	}
}
