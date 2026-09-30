//go:build windows

package servicehost

import (
	"context"
	"golang.org/x/sys/windows/svc"
	"os"
)

// Run 在 Windows 服务管理器或交互终端中执行同一个入口。
func Run(name string, run func(context.Context) error) error {
	service, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !service {
		return foreground(run)
	}
	return svc.Run(name, serviceHandler{run: run})
}

type serviceHandler struct{ run func(context.Context) error }

func (h serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes <- svc.Status{State: svc.StartPending}
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-done; err != nil {
					return true, 1
				}
				return false, 0
			}
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		}
	}
}

func terminationSignal() os.Signal { return os.Interrupt }
