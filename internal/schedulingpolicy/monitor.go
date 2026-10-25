package schedulingpolicy

import (
	"context"
	"time"
)

// TimeoutReconciler 仅提供标记超时核对的固定能力，不启动或终止进程。
type TimeoutReconciler interface {
	ReconcileSchedulingTimeouts(context.Context, time.Time, time.Duration) (int, error)
}

// RunMonitor 使用固定控制面扫描周期，不把平台领取检查设置用于续租或后台安全核对。
func RunMonitor(ctx context.Context, reconciler TimeoutReconciler, baseline time.Duration, onError func(error)) {
	if reconciler == nil || ctx.Err() != nil {
		return
	}
	check := func() {
		checkContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := reconciler.ReconcileSchedulingTimeouts(checkContext, time.Now().UTC(), baseline); err != nil && ctx.Err() == nil && onError != nil {
			onError(err)
		}
	}
	check()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
