// Package servicehost 将操作系统服务生命周期与业务入口隔离。
package servicehost

import (
	"context"
	"os"
	"os/signal"
)

func foreground(run func(context.Context) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, terminationSignal())
	defer stop()
	return run(ctx)
}
