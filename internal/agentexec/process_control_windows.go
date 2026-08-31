//go:build windows

package agentexec

import (
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processTreeController 使用 Job Object 绑定整个 Java 子进程树。
type processTreeController struct {
	job windows.Handle
}

func newProcessTreeController() *processTreeController {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return nil
	}
	return &processTreeController{job: job}
}

func (c *processTreeController) prepare(*exec.Cmd) error { return nil }

func (c *processTreeController) attach(process *os.Process) error {
	if c == nil || c.job == 0 || process == nil {
		return ErrDirectJavaStartFailed
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(process.Pid))
	if err != nil {
		return ErrDirectJavaStartFailed
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(c.job, handle); err != nil {
		return ErrDirectJavaStartFailed
	}
	return nil
}

func (c *processTreeController) cancel() error {
	if c == nil || c.job == 0 {
		return ErrDirectJavaStartFailed
	}
	return windows.TerminateJobObject(c.job, 1)
}

func (c *processTreeController) close() error {
	if c == nil || c.job == 0 {
		return nil
	}
	err := windows.CloseHandle(c.job)
	c.job = 0
	return err
}
