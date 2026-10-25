package schedulingpolicy

import (
	"context"
	"testing"
	"time"
)

type monitorFixture func(context.Context, time.Time, time.Duration) (int, error)

func (f monitorFixture) ReconcileSchedulingTimeouts(ctx context.Context, now time.Time, baseline time.Duration) (int, error) {
	return f(ctx, now, baseline)
}

func Test核对监视器有界上下文且取消后退出(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	calls := 0
	reported := 0
	fake := monitorFixture(func(checkContext context.Context, now time.Time, baseline time.Duration) (int, error) {
		calls++
		deadline, ok := checkContext.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second || now.IsZero() || now.Location() != time.UTC || baseline != 2*time.Minute {
			t.Error("监视器边界无效")
		}
		return 0, ErrUnavailable
	})
	go func() { defer close(done); RunMonitor(ctx, fake, 2*time.Minute, func(error) { reported++; cancel() }) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("监视器未响应取消")
	}
	if calls != 1 || reported != 1 {
		t.Fatalf("调用=%d 反馈=%d", calls, reported)
	}
}

func Test取消的监视器不再读取或发出失败反馈(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	RunMonitor(ctx, monitorFixture(func(context.Context, time.Time, time.Duration) (int, error) { calls++; return 0, ErrUnavailable }), 2*time.Minute, func(error) { calls++ })
	if calls != 0 {
		t.Fatal("取消后继续执行核对")
	}
}
