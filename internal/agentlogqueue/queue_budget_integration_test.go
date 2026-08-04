package agentlogqueue

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/logstream"
)

const (
	queueBudgetValidationEnabledEnvironmentVariable = "OB_DATA_ORCH_RUN_1GIB_QUEUE_VALIDATION"
	queueBudgetValidationRootEnvironmentVariable    = "OB_DATA_ORCH_1GIB_QUEUE_VALIDATION_ROOT"
)

func Test可选验证默认队列一GiB物理上限(t *testing.T) {
	if os.Getenv(queueBudgetValidationEnabledEnvironmentVariable) != "true" {
		t.Skip("未显式开启 1 GiB 队列物理容量验证")
	}
	root := strings.TrimSpace(os.Getenv(queueBudgetValidationRootEnvironmentVariable))
	if root == "" {
		t.Fatal("已开启 1 GiB 队列物理容量验证但未指定隔离根目录")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("解析队列物理容量验证根目录失败: %v", err)
	}
	rootInfo, err := os.Lstat(absRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("队列物理容量验证根目录必须是已有的非链接目录")
	}
	validationDirectory, err := os.MkdirTemp(filepath.Clean(absRoot), "obdo-agent-log-queue-budget-")
	if err != nil {
		t.Fatalf("创建队列物理容量验证目录失败: %v", err)
	}
	relative, err := filepath.Rel(filepath.Clean(absRoot), validationDirectory)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatal("队列物理容量验证目录不在指定根目录内")
	}
	t.Logf("1 GiB 队列物理容量验证目录：%s", validationDirectory)
	t.Cleanup(func() {
		if err := os.RemoveAll(validationDirectory); err != nil {
			t.Errorf("清理本次创建的队列物理容量验证目录失败: %v", err)
		}
	})

	queue, err := Open(validationDirectory, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("打开默认上限队列失败: %v", err)
	}
	message := strings.Repeat("x", logstream.MaxRecordBytes)
	var accepted int64
	for firstSequence := int64(1); ; firstSequence += 2 {
		entry := queueBudgetEntry(t, message, firstSequence)
		err := queue.Enqueue(entry)
		if errors.Is(err, ErrFull) {
			break
		}
		if err != nil {
			t.Fatalf("第 %d 个物理容量验证批次入队失败: %v", firstSequence, err)
		}
		accepted++
		if accepted > 4096 {
			t.Fatal("默认队列上限未在合理批次数内拒绝后续批次")
		}
	}
	if accepted == 0 || queue.usedBytes <= 0 || queue.usedBytes > DefaultMaxBytes {
		t.Fatalf("默认队列物理容量计量不可信: accepted=%d usedBytes=%d", accepted, queue.usedBytes)
	}
	physicalBytes, err := queue.sizeLocked()
	if err != nil || physicalBytes != queue.usedBytes {
		t.Fatalf("默认队列物理文件大小与缓存计量不一致: physicalBytes=%d usedBytes=%d error=%v", physicalBytes, queue.usedBytes, err)
	}
	reopened, err := Open(validationDirectory, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("重开默认上限队列失败: %v", err)
	}
	if reopened.usedBytes != queue.usedBytes {
		t.Fatalf("重开默认队列后的容量计量不一致: reopened=%d current=%d", reopened.usedBytes, queue.usedBytes)
	}
	if err := reopened.Enqueue(queueBudgetEntry(t, message, accepted*2+1)); !errors.Is(err, ErrFull) {
		t.Fatalf("达到默认 1 GiB 上限后继续入队 error=%v，期望 ErrFull", err)
	}
	t.Logf("默认 1 GiB 队列物理验证通过：accepted=%d usedBytes=%d", accepted, queue.usedBytes)
}

func queueBudgetEntry(t *testing.T, message string, firstSequence int64) Entry {
	t.Helper()
	firstRecord := logstream.Record{
		StreamID: "queue-budget-stream", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: firstSequence,
		Kind: logstream.RecordLog, Message: message, PolicyVersion: "queue-budget-v1", ParserVersion: "queue-budget-v1", ReceivedAt: time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC),
	}
	secondRecord := firstRecord
	secondRecord.SourceSeq++
	batch, err := logstream.SealBatch(logstream.Batch{
		StreamID: firstRecord.StreamID, SourceEpoch: firstRecord.SourceEpoch, FirstSeq: firstSequence, LastSeq: firstSequence + 1, PolicyVersion: firstRecord.PolicyVersion, Records: []logstream.Record{firstRecord, secondRecord},
	})
	if err != nil {
		t.Fatalf("密封第 %d 个物理容量验证批次失败: %v", firstSequence, err)
	}
	return Entry{ExecutionID: "queue-budget-execution", LeaseID: "queue-budget-lease", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("c", 64), Batch: batch}
}
