package main

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentexecution"
	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/logstream"
)

// Test异步导出执行器串行调度验证后台执行期间不会并发领取任务。
func Test异步导出执行器串行调度(t *testing.T) {
	t.Parallel()
	delegate := &blockingExportRunner{
		started:  make(chan int, 4),
		release:  make(chan struct{}),
		finished: make(chan int, 4),
	}
	runner := &asynchronousExportExecutionRunner{delegate: delegate}

	started, err := runner.RunNext(context.Background())
	if err != nil || !started {
		t.Fatalf("首次调度 = started=%v err=%v", started, err)
	}
	if call := <-delegate.started; call != 1 {
		t.Fatalf("后台首次调用序号 = %d, want 1", call)
	}
	started, err = runner.RunNext(context.Background())
	if err != nil || started {
		t.Fatalf("执行中重复调度 = started=%v err=%v", started, err)
	}

	close(delegate.release)
	if call := <-delegate.finished; call != 1 {
		t.Fatalf("后台首次完成序号 = %d, want 1", call)
	}
	waitAsyncRunnerIdle(t, runner)

	started, err = runner.RunNext(context.Background())
	if err != nil || !started {
		t.Fatalf("后台完成后的再次调度 = started=%v err=%v", started, err)
	}
	if call := <-delegate.started; call != 2 {
		t.Fatalf("后台再次调用序号 = %d, want 2", call)
	}
	if call := <-delegate.finished; call != 2 {
		t.Fatalf("后台再次完成序号 = %d, want 2", call)
	}
}

// Test异步导出执行器拒绝无效依赖验证失败关闭不会启动后台任务。
func Test异步导出执行器拒绝无效依赖(t *testing.T) {
	t.Parallel()
	var runner *asynchronousExportExecutionRunner
	started, err := runner.RunNext(context.Background())
	if started || err == nil {
		t.Fatalf("空执行器结果 = started=%v err=%v", started, err)
	}
}

type blockingExportRunner struct {
	mu       sync.Mutex
	calls    int
	started  chan int
	release  chan struct{}
	finished chan int
}

func Test并行Worker共用日志队列与容量计量(t *testing.T) {
	runner := &configuredExportExecutionRunner{}
	root := t.TempDir()
	queues := make(chan *agentlogqueue.Queue, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			queue, err := runner.sharedQueue(root)
			if err != nil {
				t.Error(err)
				return
			}
			queues <- queue
		}()
	}
	wg.Wait()
	close(queues)
	var shared *agentlogqueue.Queue
	for queue := range queues {
		if shared != nil && queue != shared {
			t.Fatal("并行 Worker 使用了重复容量缓存")
		}
		shared = queue
	}
	if shared == nil {
		t.Fatal("共享队列未创建")
	}
	if _, err := runner.sharedQueue(t.TempDir()); err == nil {
		t.Fatal("目录漂移仍打开第二个队列")
	}
	batch, err := logstream.SealBatch(logstream.Batch{StreamID: "synthetic-stream", SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, PolicyVersion: "policy-v1", Records: []logstream.Record{{StreamID: "synthetic-stream", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 1, Kind: logstream.RecordLog, Message: "合成已脱敏记录", PolicyVersion: "policy-v1", ParserVersion: "test-v1", ReceivedAt: time.Now().UTC()}}})
	if err != nil {
		t.Fatal(err)
	}
	entry := agentlogqueue.Entry{ExecutionID: "execution-1", LeaseID: "lease-1", LeaseEpoch: 1, EnvelopeDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Batch: batch}
	content, _ := json.Marshal(entry)
	// 一个共享实例的锁还必须保证容量检查与写入原子；小上限夹具不用真实日志。
	limited, err := agentlogqueue.Open(t.TempDir(), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	second := entry
	second.ExecutionID = "execution-2"
	for _, value := range []agentlogqueue.Entry{entry, second} {
		wg.Add(1)
		go func(e agentlogqueue.Entry) { defer wg.Done(); results <- limited.Enqueue(e) }(value)
	}
	wg.Wait()
	close(results)
	accepted, full := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, agentlogqueue.ErrFull) {
			full++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || full != 1 {
		t.Fatal("并行入队突破共享容量", accepted, full)
	}
}

func Test资源上界限制并行Worker且取消后不再领取(t *testing.T) {
	delegate := &blockingExportRunner{started: make(chan int, 4), release: make(chan struct{}), finished: make(chan int, 4)}
	runner := &asynchronousExportExecutionRunner{delegate: delegate, capacity: func() int { return 2 }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 2; i++ {
		if found, err := runner.RunNext(ctx); err != nil || !found {
			t.Fatal("并行调度失败", err)
		}
		<-delegate.started
	}
	if found, err := runner.RunNext(ctx); err != nil || found || runner.ActiveWorkers() != 2 {
		t.Fatal("超过本机上界", err)
	}
	cancel()
	waitAsyncRunnerIdle(t, runner)
	if found, err := runner.RunNext(ctx); err != nil || found || runner.ActiveWorkers() != 0 {
		t.Fatal("取消后创建Worker", err)
	}
}

func (r *blockingExportRunner) RunNext(ctx context.Context) (agentexecution.Outcome, bool, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	r.started <- call
	select {
	case <-r.release:
		r.finished <- call
		return agentexecution.Outcome{}, true, nil
	case <-ctx.Done():
		return agentexecution.Outcome{}, true, ctx.Err()
	}
}

func waitAsyncRunnerIdle(t *testing.T, runner *asynchronousExportExecutionRunner) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runner.mu.Lock()
		idle := !runner.running
		runner.mu.Unlock()
		if idle {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("异步执行器在后台任务结束后仍保持运行状态")
}
