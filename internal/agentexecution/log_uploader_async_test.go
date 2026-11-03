package agentexecution

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
)

type ownedLogProtocol struct {
	executionWorkerTestProtocol
	id        string
	confirmed chan string
	mu        sync.Mutex
	foreign   bool
}

func (p *ownedLogProtocol) AppendExecutionLog(_ context.Context, batch agentwire.ExecutionLogBatch) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if batch.ExecutionID != p.id {
		p.foreign = true
		return errors.New("发送器越权确认其他执行")
	}
	return nil
}

func Test两个发送器共享队列不偷确认且各自清除待确认位置(t *testing.T) {
	queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	var uploaders []*logUploader
	var protocols []*ownedLogProtocol
	for _, id := range []string{"execution-a", "execution-b"} {
		g := queueTestGrant()
		g.ExecutionID = id
		p := &ownedLogProtocol{id: id, confirmed: make(chan string, 4)}
		u := newLogUploader(context.Background(), p, "synthetic-boot", g, queue, queueTestObserver{confirmed: func(_ context.Context, position LogQueuePosition) error { p.confirmed <- position.StreamID; return nil }})
		u.start()
		u.stopSender()
		defer u.stop()
		for seq := int64(1); seq <= 2; seq++ {
			r := queueTestRecord()
			r.StreamID = id
			r.SourceSeq = seq
			if err := u.append(r); err != nil {
				t.Fatal(err)
			}
		}
		uploaders = append(uploaders, u)
		protocols = append(protocols, p)
	}
	// 活动发送器持有的条目不能被领取前的通用恢复吞掉。
	recovery := &queueTestProtocol{}
	if err := flushQueuedLogs(context.Background(), queue, recovery, "synthetic-restart", ""); err != nil {
		t.Fatal(err)
	}
	if len(recovery.batches) != 0 {
		t.Fatal("恢复路径偷确认活动执行")
	}
	if err := uploaders[0].flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	items, _, err := queue.Pending()
	if err != nil || len(items) != 2 || items[0].ExecutionID != "execution-b" {
		t.Fatal("发送器A消费了B的批次", err)
	}
	if err := uploaders[1].flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	for index, u := range uploaders {
		if len(u.queued) != 0 || protocols[index].foreign || len(protocols[index].confirmed) != 2 {
			t.Fatal("共享队列丢失归属回调或位置未释放")
		}
	}
}

func Test发送器关闭后未确认所有权交回重启补传(t *testing.T) {
	queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	p := &blockedLogProtocol{entered: make(chan struct{}, 1)}
	u := newLogUploader(context.Background(), p, "synthetic-boot", queueTestGrant(), queue, nil)
	u.start()
	u.stopSender()
	if err := u.append(queueTestRecord()); err != nil {
		t.Fatal(err)
	}
	u.stop()
	recovery := &queueTestProtocol{}
	if err := flushQueuedLogs(context.Background(), queue, recovery, "synthetic-restart", ""); err != nil || len(recovery.batches) != 1 {
		t.Fatal("关闭后所有权未交回补传", err)
	}
}

type blockedLogProtocol struct {
	executionWorkerTestProtocol
	entered chan struct{}
}

func (p *blockedLogProtocol) AppendExecutionLog(ctx context.Context, _ agentwire.ExecutionLogBatch) error {
	select {
	case p.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func Test异步日志网络阻塞时继续持久入队且关闭保留尾部可重放(t *testing.T) {
	root := t.TempDir()
	queue, err := agentlogqueue.Open(root, agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	protocol := &blockedLogProtocol{entered: make(chan struct{}, 1)}
	u := newLogUploader(context.Background(), protocol, "synthetic-boot", queueTestGrant(), queue, nil)
	u.start()
	defer u.stop()
	if err := u.append(queueTestRecord()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-protocol.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("未进入合成网络阻塞")
	}
	done := make(chan error, 1)
	go func() {
		for sequence := int64(2); sequence <= 4; sequence++ {
			r := queueTestRecord()
			r.SourceSeq = sequence
			if err := u.append(r); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("网络等待阻塞了后续持久入队")
	}
	entries, _, err := queue.Pending()
	if err != nil || len(entries) != 4 {
		t.Fatal("未确认记录没有完整持久化", len(entries), err)
	}
	for index := 1; index < len(entries); index++ {
		if entries[index].Batch.PreviousDigest != entries[index-1].Batch.Digest {
			t.Fatal("异步采集破坏摘要链")
		}
	}
	started := time.Now()
	if u.finishBestEffort(context.Background()) {
		t.Fatal("阻塞网络被当作完整确认")
	}
	if time.Since(started) > logUploadBestEffortTimeout+time.Second {
		t.Fatal("关闭超过有界收尾时间")
	}
	select {
	case <-u.done:
	default:
		t.Fatal("发送协程未关闭")
	}
	reopened, err := agentlogqueue.Open(root, agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	recovered := &queueTestProtocol{}
	if err := flushQueuedLogs(context.Background(), reopened, recovered, "synthetic-restart", ""); err != nil {
		t.Fatal(err)
	}
	if len(recovered.batches) != 4 {
		t.Fatal("重启补传丢失尾部")
	}
	for index, batch := range recovered.batches {
		if batch.Batch.Digest != entries[index].Batch.Digest {
			t.Fatal("重启补传改变批次摘要")
		}
	}
	if entries, _, err := reopened.Pending(); err != nil || len(entries) != 0 {
		t.Fatal("确认后仍有未释放批次", err)
	}
}

func Test日志满额不等待网络且缺口和已有正文一起保留(t *testing.T) {
	grant := queueTestGrant()
	record := queueTestRecord()
	record.StreamID = grant.ExecutionID
	first := 受控队列条目(t, grant, record, "")
	body, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := agentlogqueue.Open(t.TempDir(), int64(len(body)+8))
	if err != nil {
		t.Fatal(err)
	}
	p := &blockedLogProtocol{entered: make(chan struct{}, 1)}
	u := newLogUploader(context.Background(), p, "synthetic-boot", grant, queue, nil)
	u.start()
	defer u.stop()
	if err := u.append(record); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("未进入网络等待")
	}
	r := record
	r.SourceSeq = 2
	done := make(chan error, 1)
	go func() { done <- u.append(r) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("容量处理同步等待网络腾出空间")
	}
	u.stop()
	items, err := queue.PendingItems()
	if err != nil || len(items) != 2 || items[0].Entry == nil || items[1].Gap == nil || items[1].Gap.Gap.FirstSeq != 2 {
		t.Fatal("满额未保留原批次和缺口", err)
	}
	if u.collected() {
		t.Fatal("发生容量缺口后仍宣称采集完整")
	}
}

func Test日志缺少持久队列时不回退到同步网络(t *testing.T) {
	u := newLogUploader(context.Background(), &queueTestProtocol{}, "synthetic-boot", queueTestGrant(), nil, nil)
	if err := u.append(queueTestRecord()); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatal("无持久队列仍接收日志", err)
	}
}
