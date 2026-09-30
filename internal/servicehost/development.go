package servicehost

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// DevelopmentContext 监听开发进程目录的固定停止标记，让热重启沿正常取消流程关闭资源。
// 只由显式开发入口调用，正式服务不接受文件控制。
func DevelopmentContext(parent context.Context, directory string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat(filepath.Join(directory, "dev.stop")); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}
