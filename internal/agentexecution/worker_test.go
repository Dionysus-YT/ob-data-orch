package agentexecution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/logstream"
)

func Test日志上传器在入队前后只暴露安全位置(t *testing.T) {
	t.Parallel()
	queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("打开队列失败: %v", err)
	}
	protocol := &queueTestProtocol{}
	var observed []LogQueuePosition
	observer := queueTestObserver{
		before: func(_ context.Context, position LogQueuePosition) error {
			if entries, _, err := queue.Pending(); err != nil || len(entries) != 0 {
				t.Fatalf("入队前不应存在待确认批次: entries=%d err=%v", len(entries), err)
			}
			observed = append(observed, position)
			return nil
		},
		after: func(_ context.Context, position LogQueuePosition) error {
			entries, _, err := queue.Pending()
			if err != nil || len(entries) != 1 {
				t.Fatalf("入队后必须先持久化待确认批次: entries=%d err=%v", len(entries), err)
			}
			observed = append(observed, position)
			return nil
		},
		confirmed: func(_ context.Context, position LogQueuePosition) error {
			if entries, _, err := queue.Pending(); err != nil || len(entries) != 0 {
				t.Fatalf("确认后不应保留待确认批次: entries=%d err=%v", len(entries), err)
			}
			observed = append(observed, position)
			return nil
		},
	}
	uploader := newLogUploader(context.Background(), protocol, "boot-test", queueTestGrant(), queue, observer)
	if err := uploader.append(queueTestRecord()); err != nil {
		t.Fatalf("追加日志失败: %v", err)
	}
	if len(observed) != 3 {
		t.Fatalf("观测次数错误: got %d want 3", len(observed))
	}
	for _, position := range observed {
		if position.StreamID != "stream-test" || position.SourceEpoch != 1 || position.FirstSeq != 1 || position.LastSeq != 1 || len(position.BatchDigest) != 64 {
			t.Fatalf("观测位置不完整或不安全: %+v", position)
		}
	}
	if len(protocol.batches) != 1 {
		t.Fatalf("控制面确认批次数错误: got %d want 1", len(protocol.batches))
	}
	if entries, _, err := queue.Pending(); err != nil || len(entries) != 0 {
		t.Fatalf("确认后队列必须清空: entries=%d err=%v", len(entries), err)
	}
}

func Test日志上传器在入队后屏障失败时保留待确认批次(t *testing.T) {
	t.Parallel()
	queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("打开队列失败: %v", err)
	}
	protocol := &queueTestProtocol{}
	observer := queueTestObserver{after: func(context.Context, LogQueuePosition) error {
		return errors.New("演练屏障停止回传")
	}}
	uploader := newLogUploader(context.Background(), protocol, "boot-test", queueTestGrant(), queue, observer)
	if err := uploader.append(queueTestRecord()); err == nil {
		t.Fatal("入队后屏障失败必须返回错误")
	}
	entries, _, err := queue.Pending()
	if err != nil || len(entries) != 1 {
		t.Fatalf("入队后的失败不能删除待确认批次: entries=%d err=%v", len(entries), err)
	}
	if len(protocol.batches) != 0 {
		t.Fatalf("屏障未释放前不能发起控制面回传: got %d", len(protocol.batches))
	}
}

func Test日志上传器记录失败重试和确认且不删除待确认批次(t *testing.T) {
	t.Parallel()
	queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("打开队列失败: %v", err)
	}
	protocol := &queueTestProtocol{appendErrors: []error{errors.New("合成控制面暂不可达")}}
	uploader := newLogUploader(context.Background(), protocol, "boot-test", queueTestGrant(), queue, nil)
	if err := uploader.append(queueTestRecord()); err != nil {
		t.Fatalf("网络暂不可达不能阻断日志接收: %v", err)
	}
	entries, _, err := queue.Pending()
	if err != nil || len(entries) != 1 {
		t.Fatalf("上传失败后必须保留待确认批次: entries=%d err=%v", len(entries), err)
	}
	if err := flushQueuedLogs(context.Background(), queue, protocol, "boot-restarted", ""); err != nil {
		t.Fatalf("重启补传失败: %v", err)
	}
	entries, _, err = queue.Pending()
	if err != nil || len(entries) != 0 {
		t.Fatalf("控制面确认后队列必须清空: entries=%d err=%v", len(entries), err)
	}
	evidence, err := queue.EvidenceSnapshot()
	if err != nil {
		t.Fatalf("读取队列证据失败: %v", err)
	}
	want := []agentlogqueue.EvidenceType{
		agentlogqueue.EvidenceEnqueued,
		agentlogqueue.EvidenceUploadAttempt,
		agentlogqueue.EvidenceUploadFailed,
		agentlogqueue.EvidenceRecoveryReplayAttempt,
		agentlogqueue.EvidenceControlPlaneConfirmed,
	}
	if len(evidence) != len(want) {
		t.Fatalf("队列证据数量错误: got %d want %d", len(evidence), len(want))
	}
	for index, expected := range want {
		if evidence[index].Type != expected || evidence[index].BatchDigest == "" || evidence[index].StreamID != "stream-test" {
			t.Fatalf("队列证据[%d]错误: %#v", index, evidence[index])
		}
	}
}

func Test日志上传器在连续不可达后保留批次并重放(t *testing.T) {
	t.Parallel()
	queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("打开队列失败: %v", err)
	}
	protocol := &queueTestProtocol{appendErrors: []error{
		errors.New("第一次合成不可达"), errors.New("第二次合成不可达"), errors.New("第三次合成不可达"),
	}}
	uploader := newLogUploader(context.Background(), protocol, "boot-test", queueTestGrant(), queue, nil)
	if err := uploader.append(queueTestRecord()); err != nil {
		t.Fatalf("首次不可达不能阻断日志接收: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := flushQueuedLogs(context.Background(), queue, protocol, "boot-test", ""); err == nil {
			t.Fatalf("第 %d 次持续不可达意外确认批次", attempt+2)
		}
		entries, _, pendingErr := queue.Pending()
		if pendingErr != nil || len(entries) != 1 {
			t.Fatalf("第 %d 次不可达后队列不应清空: entries=%d err=%v", attempt+2, len(entries), pendingErr)
		}
	}
	if err := flushQueuedLogs(context.Background(), queue, protocol, "boot-restarted", ""); err != nil {
		t.Fatalf("恢复后的重放失败: %v", err)
	}
	entries, _, err := queue.Pending()
	if err != nil || len(entries) != 0 {
		t.Fatalf("恢复确认后队列必须清空: entries=%d err=%v", len(entries), err)
	}
	evidence, err := queue.EvidenceSnapshot()
	if err != nil {
		t.Fatalf("读取连续不可达队列证据失败: %v", err)
	}
	want := []agentlogqueue.EvidenceType{
		agentlogqueue.EvidenceEnqueued,
		agentlogqueue.EvidenceUploadAttempt,
		agentlogqueue.EvidenceUploadFailed,
		agentlogqueue.EvidenceRecoveryReplayAttempt,
		agentlogqueue.EvidenceUploadFailed,
		agentlogqueue.EvidenceRecoveryReplayAttempt,
		agentlogqueue.EvidenceUploadFailed,
		agentlogqueue.EvidenceRecoveryReplayAttempt,
		agentlogqueue.EvidenceControlPlaneConfirmed,
	}
	if len(evidence) != len(want) {
		t.Fatalf("连续不可达队列证据数量错误: got %d want %d", len(evidence), len(want))
	}
	for index, expected := range want {
		if evidence[index].Type != expected || evidence[index].BatchDigest == "" {
			t.Fatalf("连续不可达队列证据[%d]错误: %#v", index, evidence[index])
		}
	}
}

type queueTestObserver struct {
	before    func(context.Context, LogQueuePosition) error
	after     func(context.Context, LogQueuePosition) error
	confirmed func(context.Context, LogQueuePosition) error
}

func (o queueTestObserver) BeforeEnqueue(ctx context.Context, position LogQueuePosition) error {
	if o.before == nil {
		return nil
	}
	return o.before(ctx, position)
}

func (o queueTestObserver) AfterEnqueue(ctx context.Context, position LogQueuePosition) error {
	if o.after == nil {
		return nil
	}
	return o.after(ctx, position)
}

func (o queueTestObserver) AfterConfirmation(ctx context.Context, position LogQueuePosition) error {
	if o.confirmed == nil {
		return nil
	}
	return o.confirmed(ctx, position)
}

type queueTestProtocol struct {
	batches      []agentwire.ExecutionLogBatch
	appendErrors []error
}

func (p *queueTestProtocol) ClaimNextExecution(context.Context, agentwire.ExecutionClaimNext) (agentwire.ExecutionGrant, bool, error) {
	return agentwire.ExecutionGrant{}, false, nil
}

func (p *queueTestProtocol) AcknowledgeExecutionLease(context.Context, agentwire.ExecutionLeaseAcknowledgement) error {
	return nil
}

func (p *queueTestProtocol) RenewExecutionLease(context.Context, agentwire.ExecutionLeaseRenewal) error {
	return nil
}

func (p *queueTestProtocol) ResolveExecutionDatabaseConnection(context.Context, agentwire.ExecutionSecretSlotRequest) (agentwire.DatabaseConnectionSlot, error) {
	return agentwire.DatabaseConnectionSlot{}, nil
}

func (p *queueTestProtocol) AppendExecutionEvent(context.Context, agentwire.ExecutionEvent) error {
	return nil
}

func (p *queueTestProtocol) AppendExecutionLog(_ context.Context, batch agentwire.ExecutionLogBatch) error {
	p.batches = append(p.batches, batch)
	if len(p.appendErrors) != 0 {
		err := p.appendErrors[0]
		p.appendErrors = p.appendErrors[1:]
		return err
	}
	return nil
}

func (p *queueTestProtocol) AppendExecutionLogGap(context.Context, agentwire.ExecutionLogGap) error {
	return nil
}

func queueTestGrant() agentwire.ExecutionGrant {
	return agentwire.ExecutionGrant{
		ExecutionID:    "execution-test",
		LeaseID:        "lease-test",
		LeaseEpoch:     1,
		EnvelopeDigest: strings.Repeat("a", 64),
	}
}

func queueTestRecord() logstream.Record {
	return logstream.Record{
		StreamID:      "stream-test",
		SourceKind:    logstream.SourceStdout,
		SourceEpoch:   1,
		SourceSeq:     1,
		Kind:          logstream.RecordLog,
		Message:       "已脱敏测试记录",
		PolicyVersion: "test-v1",
		ParserVersion: "test-parser-v1",
	}
}
