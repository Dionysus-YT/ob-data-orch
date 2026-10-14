package agentlogqueue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func Test队列统计覆盖确认失败重启且不携带正文标识(t *testing.T) {
	root := t.TempDir()
	queue, err := Open(root, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	entry := queueTestEntry(t, "synthetic-private-log-content")
	if err := queue.Enqueue(entry); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueGap(queueTestGap(2)); err != nil {
		t.Fatal(err)
	}
	stats, err := queue.Stats()
	if err != nil || stats.PendingBatches != 1 || stats.PendingGaps != 1 || stats.PendingBytes <= 0 || stats.LastUploadResult != "UNKNOWN" {
		t.Fatal("持久入队统计不正确")
	}
	at := time.Now().UTC()
	evidence := Evidence{Type: EvidenceUploadFailed, StreamID: entry.Batch.StreamID, SourceEpoch: entry.Batch.SourceEpoch, FirstSeq: entry.Batch.FirstSeq, LastSeq: entry.Batch.LastSeq, BatchDigest: entry.Batch.Digest, OccurredAt: at}
	if err := queue.RecordEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	stats, _ = queue.Stats()
	if stats.LastUploadResult != "FAILED" || stats.PendingBatches != 1 {
		t.Fatal("失败上传不能清理或虚报确认")
	}
	evidence.Type, evidence.OccurredAt = EvidenceControlPlaneConfirmed, at.Add(time.Second)
	if err := queue.RecordEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	_, tokens, err := queue.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Acknowledge(tokens[0]); err != nil {
		t.Fatal(err)
	}
	items, err := queue.PendingItems()
	if err != nil || len(items) != 1 {
		t.Fatal("缺口统计丢失")
	}
	if err := queue.Acknowledge(items[0].Token); err != nil {
		t.Fatal(err)
	}
	stats, _ = queue.Stats()
	if stats.PendingBatches != 0 || stats.PendingBytes != 0 || stats.LastUploadResult != "SUCCEEDED" {
		t.Fatal("确认删除后没有释放统计")
	}
	encoded, _ := json.Marshal(stats)
	for _, forbidden := range []string{"synthetic-private-log-content", entry.ExecutionID, entry.Batch.StreamID, entry.LeaseID} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("统计携带正文或私有标识")
		}
	}
	reopened, err := Open(root, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	stats, _ = reopened.Stats()
	if stats.LastUploadResult != "UNKNOWN" || stats.LastUploadAt != nil {
		t.Fatal("重启沿用了另一进程的上传结果")
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Stats(); err == nil {
		t.Fatal("不可读队列不能报告零积压")
	}
}
