package agentlogqueue

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/logstream"
)

func Test队列重开后保留已确认前批次并在确认后删除(t *testing.T) {
	root := t.TempDir()
	queue, err := Open(root, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	entry := queueTestEntry(t, "首条已脱敏日志")
	if err := queue.Enqueue(entry); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	reopened, err := Open(root, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("重新 Open() error = %v", err)
	}
	entries, tokens, err := reopened.Pending()
	if err != nil || len(entries) != 1 || len(tokens) != 1 || entries[0].Batch.Digest != entry.Batch.Digest {
		t.Fatalf("Pending() entries=%#v tokens=%#v error=%v", entries, tokens, err)
	}
	if err := reopened.Acknowledge(tokens[0]); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}
	entries, tokens, err = reopened.Pending()
	if err != nil || len(entries) != 0 || len(tokens) != 0 {
		t.Fatalf("确认后 Pending() entries=%#v tokens=%#v error=%v", entries, tokens, err)
	}
}

func Test队列容量不足失败关闭(t *testing.T) {
	queue, err := Open(t.TempDir(), 1)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := queue.Enqueue(queueTestEntry(t, "容量受限日志")); !errors.Is(err, ErrFull) {
		t.Fatalf("Enqueue() error = %v, want ErrFull", err)
	}
}

func Test队列累计容量达到上限时保留先前批次(t *testing.T) {
	first := queueTestEntryAt(t, "首个受限批次", 1)
	second := queueTestEntryAt(t, "第二个受限批次", 2)
	firstContent, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("编码首个队列批次: %v", err)
	}
	secondContent, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("编码第二个队列批次: %v", err)
	}
	queue, err := Open(t.TempDir(), int64(len(firstContent)+len(secondContent)-1))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := queue.Enqueue(first); err != nil {
		t.Fatalf("首个 Enqueue() error = %v", err)
	}
	if err := queue.Enqueue(second); !errors.Is(err, ErrFull) {
		t.Fatalf("第二个 Enqueue() error = %v, want ErrFull", err)
	}
	entries, _, err := queue.Pending()
	if err != nil || len(entries) != 1 || entries[0].Batch.Digest != first.Batch.Digest {
		t.Fatalf("队列上限后不应覆盖已确认前批次: entries=%#v error=%v", entries, err)
	}
}

func Test队列容量计量在确认后释放并在重开时重算(t *testing.T) {
	first := queueTestEntryAt(t, "首个计量批次", 1)
	second := queueTestEntryAt(t, "第二个计量批次", 2)
	firstContent, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("编码首个计量批次: %v", err)
	}
	secondContent, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("编码第二个计量批次: %v", err)
	}
	root := t.TempDir()
	queue, err := Open(root, int64(len(firstContent)+len(secondContent)-1))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := queue.Enqueue(first); err != nil {
		t.Fatalf("首个计量批次入队失败: %v", err)
	}
	if queue.usedBytes != int64(len(firstContent)) {
		t.Fatalf("普通队列缓存字节数=%d，期望=%d", queue.usedBytes, len(firstContent))
	}
	if err := queue.Acknowledge(entryName(first)); err != nil {
		t.Fatalf("确认首个计量批次失败: %v", err)
	}
	if queue.usedBytes != 0 {
		t.Fatalf("确认后普通队列缓存字节数=%d，期望=0", queue.usedBytes)
	}
	if err := queue.Enqueue(second); err != nil {
		t.Fatalf("确认释放容量后第二个批次入队失败: %v", err)
	}
	reopened, err := Open(root, int64(len(firstContent)+len(secondContent)-1))
	if err != nil {
		t.Fatalf("重开计量队列失败: %v", err)
	}
	if reopened.usedBytes != int64(len(secondContent)) {
		t.Fatalf("重开后普通队列缓存字节数=%d，期望=%d", reopened.usedBytes, len(secondContent))
	}
}

func Test队列缺口容量计量在确认后释放(t *testing.T) {
	gap := queueTestGap(1)
	content, err := json.Marshal(gap)
	if err != nil {
		t.Fatalf("编码计量缺口失败: %v", err)
	}
	queue, err := Open(t.TempDir(), DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	queue.gapMaxBytes = int64(len(content))
	if err := queue.EnqueueGap(gap); err != nil {
		t.Fatalf("计量缺口入队失败: %v", err)
	}
	if queue.gapUsedBytes != int64(len(content)) {
		t.Fatalf("缺口队列缓存字节数=%d，期望=%d", queue.gapUsedBytes, len(content))
	}
	if err := queue.Acknowledge(gapEntryName(gap)); err != nil {
		t.Fatalf("确认计量缺口失败: %v", err)
	}
	if queue.gapUsedBytes != 0 {
		t.Fatalf("确认后缺口队列缓存字节数=%d，期望=0", queue.gapUsedBytes)
	}
}

func Test队列缺口只保存元数据且在同源后续批次前重放(t *testing.T) {
	queue, err := Open(t.TempDir(), DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	first := queueTestEntryAt(t, "首批日志", 1)
	third := queueTestEntryAt(t, "后续日志", 3)
	third.Batch.PreviousDigest = first.Batch.Digest
	third.Batch, err = logstream.SealBatch(third.Batch)
	if err != nil {
		t.Fatalf("密封后续批次失败: %v", err)
	}
	if err := queue.Enqueue(first); err != nil {
		t.Fatalf("首批 Enqueue() error = %v", err)
	}
	if err := queue.EnqueueGap(queueTestGap(2)); err != nil {
		t.Fatalf("EnqueueGap() error = %v", err)
	}
	if err := queue.Enqueue(third); err != nil {
		t.Fatalf("后续 Enqueue() error = %v", err)
	}
	items, err := queue.PendingItems()
	if err != nil || len(items) != 3 || items[0].Entry == nil || items[0].Entry.Batch.FirstSeq != 1 || items[1].Gap == nil || items[1].Gap.Gap.FirstSeq != 2 || items[2].Entry == nil || items[2].Entry.Batch.FirstSeq != 3 {
		t.Fatalf("PendingItems() items=%#v error=%v", items, err)
	}
	content, err := os.ReadFile(filepath.Join(queue.root, items[1].Token))
	if err != nil {
		t.Fatalf("读取缺口文件失败: %v", err)
	}
	if strings.Contains(string(content), "首批日志") || strings.Contains(string(content), "后续日志") || strings.Contains(string(content), "message") {
		t.Fatalf("缺口文件泄露正文: %s", content)
	}
}

func Test队列缺口达到独立上限时失败关闭(t *testing.T) {
	queue, err := Open(t.TempDir(), DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	queue.gapMaxBytes = 1
	if err := queue.EnqueueGap(queueTestGap(1)); !errors.Is(err, ErrGapFull) {
		t.Fatalf("EnqueueGap() error = %v, want ErrGapFull", err)
	}
	items, err := queue.PendingItems()
	if err != nil || len(items) != 0 {
		t.Fatalf("缺口上限失败后 PendingItems() items=%#v error=%v", items, err)
	}
}

func Test队列重复批次保持幂等(t *testing.T) {
	queue, err := Open(t.TempDir(), DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	entry := queueTestEntry(t, "重复批次日志")
	if err := queue.Enqueue(entry); err != nil {
		t.Fatalf("首次 Enqueue() error = %v", err)
	}
	if err := queue.Enqueue(entry); err != nil {
		t.Fatalf("重复 Enqueue() error = %v", err)
	}
	entries, _, err := queue.Pending()
	if err != nil || len(entries) != 1 {
		t.Fatalf("Pending() entries=%#v error=%v", entries, err)
	}
}

func Test队列证据只保存批次位置且可重开读取(t *testing.T) {
	root := t.TempDir()
	queue, err := Open(root, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	entry := queueTestEntry(t, "不应写入队列证据的正文")
	entry.ExecutionID = "execution-opaque"
	if err := queue.RecordEvidence(evidenceForEntry(EvidenceEnqueued, entry)); err != nil {
		t.Fatalf("RecordEvidence() error = %v", err)
	}
	reopened, err := Open(root, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("重新 Open() error = %v", err)
	}
	evidence, err := reopened.EvidenceSnapshot()
	if err != nil || len(evidence) != 1 {
		t.Fatalf("EvidenceSnapshot() evidence=%#v error=%v", evidence, err)
	}
	if evidence[0].Type != EvidenceEnqueued || evidence[0].StreamID != entry.Batch.StreamID || evidence[0].BatchDigest != entry.Batch.Digest {
		t.Fatalf("队列证据不完整: %#v", evidence[0])
	}
	raw, err := os.ReadFile(filepath.Join(root, evidenceFileName))
	if err != nil {
		t.Fatalf("读取队列证据失败: %v", err)
	}
	if strings.Contains(string(raw), entry.Batch.Records[0].Message) || strings.Contains(string(raw), entry.ExecutionID) || strings.Contains(string(raw), entry.LeaseID) {
		t.Fatalf("队列证据泄露正文或租约身份: %s", raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("证据 JSON 无效: %v", err)
	}
	for _, forbidden := range []string{"message", "argv", "path", "executionId", "leaseId", "error"} {
		if _, found := fields[forbidden]; found {
			t.Fatalf("队列证据包含禁止字段 %q", forbidden)
		}
	}
}

func Test队列证据达到上限时失败关闭(t *testing.T) {
	queue, err := Open(t.TempDir(), DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	queue.evidenceMaxBytes = 1
	if err := queue.RecordEvidence(evidenceForEntry(EvidenceEnqueued, queueTestEntry(t, "容量测试"))); !errors.Is(err, ErrEvidenceFull) {
		t.Fatalf("RecordEvidence() error = %v, want ErrEvidenceFull", err)
	}
}

func Test队列证据拒绝包含正文的篡改行(t *testing.T) {
	root := t.TempDir()
	queue, err := Open(root, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	entry := queueTestEntry(t, "篡改夹具")
	if err := queue.RecordEvidence(evidenceForEntry(EvidenceEnqueued, entry)); err != nil {
		t.Fatalf("RecordEvidence() error = %v", err)
	}
	unsafe := `{"type":"ENQUEUED","streamId":"stream-test","sourceEpoch":1,"firstSeq":1,"lastSeq":1,"batchDigest":"` + strings.Repeat("a", 64) + `","occurredAt":"2026-08-03T00:00:00Z","message":"不得接受"}` + "\n"
	file, err := os.OpenFile(filepath.Join(root, evidenceFileName), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("打开队列证据失败: %v", err)
	}
	if _, err := file.WriteString(unsafe); err != nil {
		_ = file.Close()
		t.Fatalf("写入篡改行失败: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("关闭篡改文件失败: %v", err)
	}
	if _, err := queue.EvidenceSnapshot(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("EvidenceSnapshot() error = %v, want ErrCorrupt", err)
	}
}

func evidenceForEntry(evidenceType EvidenceType, entry Entry) Evidence {
	return Evidence{Type: evidenceType, StreamID: entry.Batch.StreamID, SourceEpoch: entry.Batch.SourceEpoch, FirstSeq: entry.Batch.FirstSeq, LastSeq: entry.Batch.LastSeq, BatchDigest: entry.Batch.Digest, OccurredAt: time.Date(2026, time.August, 3, 0, 0, 0, 0, time.UTC)}
}

func queueTestEntry(t *testing.T, message string) Entry {
	return queueTestEntryAt(t, message, 1)
}

func queueTestEntryAt(t *testing.T, message string, sequence int64) Entry {
	t.Helper()
	batch, err := logstream.SealBatch(logstream.Batch{
		StreamID: "execution-test", SourceEpoch: 1, FirstSeq: sequence, LastSeq: sequence, PolicyVersion: "policy-v1",
		Records: []logstream.Record{{StreamID: "execution-test", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: sequence, Kind: logstream.RecordLog, Message: message, PolicyVersion: "policy-v1", ParserVersion: "test-v1", ReceivedAt: time.Date(2026, time.August, 3, 0, 0, 0, 0, time.UTC)}},
	})
	if err != nil {
		t.Fatalf("SealBatch() error = %v", err)
	}
	return Entry{ExecutionID: "execution-test", LeaseID: "lease-test", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("a", 64), Batch: batch}
}

func queueTestGap(sequence int64) GapEntry {
	return GapEntry{
		ExecutionID: "execution-test", LeaseID: "lease-test", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("a", 64),
		Gap: logstream.GapNotice{
			StreamID: "execution-test", SourceKind: logstream.SourceStdout, SourceEpoch: 1,
			FirstSeq: sequence, LastSeq: sequence, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "policy-v1", ParserVersion: "test-v1",
		},
	}
}
