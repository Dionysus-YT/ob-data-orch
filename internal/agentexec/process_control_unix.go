//go:build !windows

package agentexec

import (
	"os"
	"os/exec"
	"syscall"
)

// processTreeController 在非 Windows 平台把受控 Java 放入独立进程组。
type processTreeController struct {
	pid int
}

func newProcessTreeController() *processTreeController { return &processTreeController{} }

func (c *processTreeController) prepare(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func (c *processTreeController) attach(process *os.Process) error {
	if process == nil || process.Pid < 1 {
		return ErrDirectJavaStartFailed
	}
	c.pid = process.Pid
	return nil
}

func (c *processTreeController) cancel() error {
	if c == nil || c.pid < 1 {
		return ErrDirectJavaStartFailed
	}
	return syscall.Kill(-c.pid, syscall.SIGKILL)
}

func (c *processTreeController) close() error { return nil }
