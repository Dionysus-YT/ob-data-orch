package agentexecution

import (
	"context"
	"errors"
	"sync"
	"time"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/logstream"
)

// logUploader 把采集与发送分离：采集只密封并 fsync，发送协程只读取可靠队列。
// 通知仅用于唤醒，不承载正文；通知合并、发送失败或进程退出均不能丢失已入队批次。
type logUploader struct {
	gapped   bool
	ctx      context.Context
	protocol Protocol
	bootID   string
	grant    agentwire.ExecutionGrant
	mu       sync.Mutex
	previous map[int64]string
	queued   map[string]bool
	queue    *agentlogqueue.Queue
	observer LogQueueObserver
	wake     chan struct{}
	done     chan struct{}
	cancel   context.CancelFunc
	failed   bool
	// 归属持续到收尾结束；通用重放不能偷确认仍由本发送器负责的条目。
	ownsQueue   bool
	releaseOnce sync.Once
}

const logUploadRetryInterval = time.Second
const logUploadBestEffortTimeout = 2 * time.Second

func newLogUploader(ctx context.Context, protocol Protocol, bootID string, grant agentwire.ExecutionGrant, queue *agentlogqueue.Queue, observer LogQueueObserver) *logUploader {
	return &logUploader{ctx: ctx, protocol: protocol, bootID: bootID, grant: grant, previous: make(map[int64]string), queued: make(map[string]bool), queue: queue, observer: observer, wake: make(chan struct{}, 1)}
}

func (u *logUploader) start() {
	if u.queue == nil {
		return
	}
	ctx, cancel := context.WithCancel(u.ctx)
	u.cancel, u.done = cancel, make(chan struct{})
	queuedLogPublishMu.Lock()
	queuedLogOwners[logQueueOwner{u.queue, u.grant.ExecutionID}]++
	u.ownsQueue = true
	queuedLogPublishMu.Unlock()
	go func() {
		defer close(u.done)
		ticker := time.NewTicker(logUploadRetryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-u.wake:
			case <-ticker.C:
			}
			if ctx.Err() != nil {
				return
			}
			_ = u.flush(ctx)
		}
	}()
}

// stop 取消正在等待的 HTTP 请求和重放锁，并等待发送协程结束；队列文件保留供补传。
func (u *logUploader) stop() {
	u.stopSender()
	u.releaseOnce.Do(func() {
		if u.ownsQueue {
			queuedLogPublishMu.Lock()
			key := logQueueOwner{u.queue, u.grant.ExecutionID}
			queuedLogOwners[key]--
			if queuedLogOwners[key] == 0 {
				delete(queuedLogOwners, key)
			}
			queuedLogPublishMu.Unlock()
		}
	})
}

func (u *logUploader) stopSender() {
	if u.cancel != nil {
		u.cancel()
		<-u.done
	}
}

// finish 在管道 EOF 后确认本 execution 的有限快照，不以任意总时长截断正常积压。
// 每次网络交换仍由现有协议请求期限约束；首个失败或调用方取消即保留剩余条目返回。
func (u *logUploader) finish(ctx context.Context) bool {
	u.stopSender()
	defer u.stop()
	complete := ctx.Err() == nil
	if u.queue != nil {
		complete = u.flush(ctx) == nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return complete && ctx.Err() == nil && !u.gapped && !u.failed
}

// finishBestEffort 仅供导出使用；其独立成功证据允许已持久化尾部在终态后补传。
// 普通导入和旁路不能使用这个时间预算替代完整上传确认。
func (u *logUploader) finishBestEffort(ctx context.Context) bool {
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), logUploadBestEffortTimeout)
	defer cancel()
	return u.finish(finalCtx)
}

// collected 表示采集证据完整且已经本地持久化；网络回执不改变这一事实。
func (u *logUploader) collected() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return !u.gapped && !u.failed
}

func (u *logUploader) notify() {
	select {
	case u.wake <- struct{}{}:
	default:
	}
}

func (u *logUploader) append(record logstream.Record) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.queue == nil {
		return ErrInvalidConfiguration
	}
	record.Progress = logstream.ParseProgress(record)
	batch := logstream.Batch{StreamID: record.StreamID, SourceEpoch: record.SourceEpoch, FirstSeq: record.SourceSeq, LastSeq: record.SourceSeq, PreviousDigest: u.previous[record.SourceEpoch], PolicyVersion: record.PolicyVersion, Records: []logstream.Record{record}}
	sealed, err := logstream.SealBatch(batch)
	if err != nil {
		u.failed = true
		return err
	}
	entry := agentlogqueue.Entry{ExecutionID: u.grant.ExecutionID, LeaseID: u.grant.LeaseID, LeaseEpoch: u.grant.LeaseEpoch, EnvelopeDigest: u.grant.EnvelopeDigest, Batch: sealed}
	position := queuePosition(entry)
	// 入队证据和本地观测屏障先于发送可见；该锁不跨网络调用持有。
	queuedLogPublishMu.Lock()
	defer queuedLogPublishMu.Unlock()
	if u.observer != nil {
		if err := u.observer.BeforeEnqueue(u.ctx, position); err != nil {
			u.failed = true
			return err
		}
	}
	if err := u.queue.Enqueue(entry); err != nil {
		if !errors.Is(err, agentlogqueue.ErrFull) {
			u.failed = true
			return err
		}
		return u.enqueueLocalSpoolLimitGap(record)
	}
	if err := recordQueueEvidence(u.queue, agentlogqueue.EvidenceEnqueued, entry); err != nil {
		u.failed = true
		return err
	}
	u.previous[record.SourceEpoch] = sealed.Digest
	u.queued[sealed.Digest] = false
	if u.observer != nil {
		if err := u.observer.AfterEnqueue(u.ctx, position); err != nil {
			u.failed = true
			return err
		}
	}
	u.notify()
	return nil
}

// enqueueLocalSpoolLimitGap 不等待网络腾出容量；独立缺口容量耗尽时失败关闭并继续排空管道。
func (u *logUploader) enqueueLocalSpoolLimitGap(record logstream.Record) error {
	u.gapped = true
	err := u.queue.EnqueueGap(agentlogqueue.GapEntry{
		ExecutionID: u.grant.ExecutionID, LeaseID: u.grant.LeaseID, LeaseEpoch: u.grant.LeaseEpoch, EnvelopeDigest: u.grant.EnvelopeDigest,
		Gap: logstream.GapNotice{StreamID: record.StreamID, SourceKind: record.SourceKind, SourceEpoch: record.SourceEpoch, FirstSeq: record.SourceSeq, LastSeq: record.SourceSeq, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: record.PolicyVersion, ParserVersion: record.ParserVersion},
	})
	if err != nil {
		u.failed = true
	}
	u.notify()
	return err
}

func (u *logUploader) flush(ctx context.Context) error {
	u.mu.Lock()
	failed := u.failed
	u.mu.Unlock()
	if failed {
		return ErrExecutionFailed
	}
	return replayQueuedLogs(ctx, u.queue, u.protocol, u.bootID, u.grant.ExecutionID, func(entry agentlogqueue.Entry) agentlogqueue.EvidenceType {
		u.mu.Lock()
		defer u.mu.Unlock()
		attempted, exists := u.queued[entry.Batch.Digest]
		if !exists {
			return agentlogqueue.EvidenceRecoveryReplayAttempt
		}
		u.queued[entry.Batch.Digest] = true
		if attempted {
			return agentlogqueue.EvidenceRetryAttempt
		}
		return agentlogqueue.EvidenceUploadAttempt
	}, func(entry agentlogqueue.Entry) error {
		u.mu.Lock()
		_, exists := u.queued[entry.Batch.Digest]
		delete(u.queued, entry.Batch.Digest)
		u.mu.Unlock()
		if exists && u.observer != nil {
			if err := u.observer.AfterConfirmation(ctx, queuePosition(entry)); err != nil {
				u.mu.Lock()
				u.failed = true
				u.mu.Unlock()
				return err
			}
		}
		return nil
	})
}

func queuePosition(entry agentlogqueue.Entry) LogQueuePosition {
	return LogQueuePosition{
		StreamID:    entry.Batch.StreamID,
		SourceEpoch: entry.Batch.SourceEpoch,
		FirstSeq:    entry.Batch.FirstSeq,
		LastSeq:     entry.Batch.LastSeq,
		BatchDigest: entry.Batch.Digest,
	}
}

// 多个隔离 Worker 共享私有可靠队列，重放与确认必须串行，防止双重删除或回执错位。
var queuedLogReplayGate = make(chan struct{}, 1)
var queuedLogPublishMu sync.Mutex

type logQueueOwner struct {
	queue       *agentlogqueue.Queue
	executionID string
}

var queuedLogOwners = make(map[logQueueOwner]int)

func flushQueuedLogs(ctx context.Context, queue *agentlogqueue.Queue, protocol Protocol, bootID, newlyEnqueuedDigest string) error {
	return replayQueuedLogs(ctx, queue, protocol, bootID, "", func(entry agentlogqueue.Entry) agentlogqueue.EvidenceType {
		if newlyEnqueuedDigest == entry.Batch.Digest {
			return agentlogqueue.EvidenceUploadAttempt
		}
		if newlyEnqueuedDigest != "" {
			return agentlogqueue.EvidenceRetryAttempt
		}
		return agentlogqueue.EvidenceRecoveryReplayAttempt
	}, nil)
}

func replayQueuedLogs(ctx context.Context, queue *agentlogqueue.Queue, protocol Protocol, bootID, owner string, evidenceType func(agentlogqueue.Entry) agentlogqueue.EvidenceType, confirmed func(agentlogqueue.Entry) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case queuedLogReplayGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-queuedLogReplayGate }()
	queuedLogPublishMu.Lock()
	items, err := queue.PendingItems()
	selected := items[:0]
	for _, item := range items {
		id := ""
		if item.Entry != nil {
			id = item.Entry.ExecutionID
		} else if item.Gap != nil {
			id = item.Gap.ExecutionID
		}
		if owner != "" && id != owner || owner == "" && queuedLogOwners[logQueueOwner{queue, id}] > 0 {
			continue
		}
		selected = append(selected, item)
	}
	items = selected
	queuedLogPublishMu.Unlock()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.Gap != nil {
			gap := item.Gap
			if err := protocol.AppendExecutionLogGap(ctx, agentwire.ExecutionLogGap{BootID: bootID, ExecutionID: gap.ExecutionID, LeaseID: gap.LeaseID, LeaseEpoch: gap.LeaseEpoch, EnvelopeDigest: gap.EnvelopeDigest, Gap: gap.Gap, SentAt: time.Now().UTC()}); err != nil {
				return err
			}
			if err := queue.Acknowledge(item.Token); err != nil {
				return err
			}
			continue
		}
		entry := item.Entry
		if err := recordQueueEvidence(queue, evidenceType(*entry), *entry); err != nil {
			return err
		}
		if err := protocol.AppendExecutionLog(ctx, agentwire.ExecutionLogBatch{BootID: bootID, ExecutionID: entry.ExecutionID, LeaseID: entry.LeaseID, LeaseEpoch: entry.LeaseEpoch, EnvelopeDigest: entry.EnvelopeDigest, Batch: entry.Batch, SentAt: time.Now().UTC()}); err != nil {
			if evidenceErr := recordQueueEvidence(queue, agentlogqueue.EvidenceUploadFailed, *entry); evidenceErr != nil {
				return evidenceErr
			}
			return err
		}
		if err := recordQueueEvidence(queue, agentlogqueue.EvidenceControlPlaneConfirmed, *entry); err != nil {
			return err
		}
		if err := queue.Acknowledge(item.Token); err != nil {
			return err
		}
		if confirmed != nil {
			if err := confirmed(*entry); err != nil {
				return err
			}
		}

	}
	return nil
}

func recordQueueEvidence(queue *agentlogqueue.Queue, evidenceType agentlogqueue.EvidenceType, entry agentlogqueue.Entry) error {
	return queue.RecordEvidence(agentlogqueue.Evidence{
		Type: evidenceType, StreamID: entry.Batch.StreamID, SourceEpoch: entry.Batch.SourceEpoch, FirstSeq: entry.Batch.FirstSeq,
		LastSeq: entry.Batch.LastSeq, BatchDigest: entry.Batch.Digest, OccurredAt: time.Now().UTC(),
	})
}
