package main

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentexecution"
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
