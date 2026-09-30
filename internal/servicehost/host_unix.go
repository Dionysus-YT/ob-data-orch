//go:build !windows

package servicehost

import (
	"context"
	"os"
	"syscall"
)

// Run 接受终端中断和 systemd 停止信号，复用同一生命周期。
func Run(_ string, run func(context.Context) error) error { return foreground(run) }
func terminationSignal() os.Signal                        { return syscall.SIGTERM }
