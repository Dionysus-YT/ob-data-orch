package agentexec

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"ob-data-orch/internal/nodefacts"
)

var (
	// ErrVersionProbeUnavailable 表示固定版本进程未完成，不包含底层输出或路径。
	ErrVersionProbeUnavailable = errors.New("fixed version probe is unavailable")
	// ErrVersionProbeTimeout 表示固定版本进程达到时限并已回收。
	ErrVersionProbeTimeout = errors.New("fixed version probe timed out")
	// ErrVersionOutputInvalid 表示版本输出缺失、冲突或超出固定预算。
	ErrVersionOutputInvalid = errors.New("fixed version output is invalid")
)

// FixedVersionProber 只运行 Java -version 或固定 OBLOADER/OBDUMPER 主类 --version。
// 本机路径和环境必须来自固化配置；当前检查须获得控制面确认的固定复验许可，不区分人工或自动来源。
type FixedVersionProber struct {
	JavaPath      string
	ToolHome      string
	WorkspaceRoot string
	Environment   []string
}

// Probe 使用进程树监管、固定时限与私有临时日志目录，绝不调用 Shell 或接收任意参数。
func (p FixedVersionProber) Probe(ctx context.Context, kind string) (string, error) {
	if ctx == nil || !filepath.IsAbs(p.JavaPath) || !filepath.IsAbs(p.ToolHome) || !filepath.IsAbs(p.WorkspaceRoot) || len(p.Environment) == 0 {
		return "", ErrVersionProbeUnavailable
	}
	if kind != "JAVA_RUNTIME" && kind != "OBDUMPER_RUNTIME" && kind != "OBLOADER_RUNTIME" {
		return "", ErrVersionProbeUnavailable
	}
	directory, err := os.MkdirTemp(p.WorkspaceRoot, ".node-version-")
	if err != nil {
		return "", ErrVersionProbeUnavailable
	}
	defer os.RemoveAll(directory)
	arguments := []string{"-version"}
	if kind != "JAVA_RUNTIME" {
		mainClass := "com.oceanbase.tools.loaddump.cmd.Obdumper"
		if kind == "OBLOADER_RUNTIME" {
			mainClass = "com.oceanbase.tools.loaddump.cmd.Obloader"
		}
		arguments = []string{"-Xms16m", "-Xmx64m", "-Dlog4j.output=" + directory, "-Dlog4j2.formatMsgNoLookups=true", "-cp", filepath.Join(p.ToolHome, "lib", "*"), mainClass, "--version"}
	}
	probeContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	command := exec.CommandContext(probeContext, p.JavaPath, arguments...)
	command.Dir, command.Env = directory, append([]string(nil), p.Environment...)
	buffer := &versionBuffer{cancel: cancel}
	defer buffer.destroy()
	command.Stdout, command.Stderr = buffer, buffer
	command.WaitDelay = time.Second
	controller := newProcessTreeController()
	if controller == nil {
		return "", ErrVersionProbeUnavailable
	}
	defer controller.close()
	attached := make(chan struct{})
	// 在 Start 前绑定取消行为；等待 attach 完成再回收整棵树，避免并发改写 exec.Cmd.Cancel。
	command.Cancel = func() error {
		<-attached
		if err := controller.cancel(); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
	if controller.prepare(command) != nil || command.Start() != nil {
		close(attached)
		return "", ErrVersionProbeUnavailable
	}
	attachErr := controller.attach(command.Process)
	close(attached)
	if attachErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return "", ErrVersionProbeUnavailable
	}
	err = command.Wait()
	if buffer.overflow {
		return "", ErrVersionOutputInvalid
	}
	if probeContext.Err() != nil {
		return "", ErrVersionProbeTimeout
	}
	if err != nil {
		return "", ErrVersionProbeUnavailable
	}
	return parseRuntimeVersion(buffer.content, kind)
}

type versionBuffer struct {
	mu       sync.Mutex
	content  []byte
	overflow bool
	cancel   context.CancelFunc
}

func (b *versionBuffer) Write(content []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.content)+len(content) > 16<<10 {
		b.overflow = true
		b.cancel()
		return 0, io.ErrShortBuffer
	}
	b.content = append(b.content, content...)
	return len(content), nil
}
func (b *versionBuffer) destroy() {
	b.mu.Lock()
	defer b.mu.Unlock()
	zeroBytes(b.content)
	b.content = nil
}

var javaVersionPattern = regexp.MustCompile(`(?m)^(?:java|openjdk) version "([0-9][0-9A-Za-z._+\-]{0,63})"`)
var toolVersionPattern = regexp.MustCompile(`(?m)^\s*Version:\s*([0-9][0-9A-Za-z._+\-]{0,63})\s*$`)

func parseRuntimeVersion(content []byte, kind string) (string, error) {
	pattern := toolVersionPattern
	if kind == "JAVA_RUNTIME" {
		pattern = javaVersionPattern
	}
	matches := pattern.FindAllSubmatch(content, -1)
	if len(matches) != 1 || !nodefacts.ValidVersion(string(matches[0][1])) {
		return "", ErrVersionOutputInvalid
	}
	return string(matches[0][1]), nil
}
