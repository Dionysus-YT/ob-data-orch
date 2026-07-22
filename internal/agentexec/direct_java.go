package agentexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ob-data-orch/internal/credential"
)

const obdumperMainClass = "com.oceanbase.tools.loaddump.cmd.Obdumper"

var (
	ErrDirectJavaLaunchInvalid = errors.New("直接 Java 启动请求无效")
	ErrDirectJavaStartFailed   = errors.New("直接 Java 进程启动失败")
	ErrDirectJavaWaitFailed    = errors.New("直接 Java 进程等待失败")
)

var jvmOptionPattern = regexp.MustCompile(`^-(X|XX:)[A-Za-z0-9._:=+\-]+$`)

// DirectJavaLaunch 是已由 Agent 配置和不可变任务信封共同固定的直接 Java 启动输入。
// 它不接收 Shell 文本、不继承父进程环境，也不允许密码进入业务参数或环境变量。
type DirectJavaLaunch struct {
	Intent                StartIntent
	BootID                string
	JavaPath              string
	JavaSHA256            string
	ToolHome              string
	SecurityConfiguration string
	JVMOptions            []string
	BusinessArguments     []string
	Environment           []string
}

// DirectJavaProcess 只暴露受控进程身份和两个尚未落盘的原始输出流。
// 调用方必须先完成双层脱敏，再允许将输出写入日志存储或返回给控制面。
type DirectJavaProcess struct {
	command  *exec.Cmd
	identity ProcessIdentity
	stdout   io.ReadCloser
	stderr   io.ReadCloser
}

// StartDirectJava 在写入不可覆盖的启动意图后直接启动包内 Java 主类。
// 该函数仅建立本地进程边界；任务租约、秘密解析、日志脱敏和终态投影必须由上层完成。
func StartDirectJava(ctx context.Context, workspace credential.Workspace, launch DirectJavaLaunch) (*DirectJavaProcess, error) {
	if err := validateDirectJavaLaunch(workspace, launch); err != nil {
		return nil, err
	}
	if err := WriteStartIntent(workspace, launch.Intent); err != nil {
		return nil, err
	}
	arguments := make([]string, 0, len(launch.JVMOptions)+len(launch.BusinessArguments)+5)
	arguments = append(arguments, launch.JVMOptions...)
	arguments = append(arguments,
		"-Dsecurity.configurationFile="+launch.SecurityConfiguration,
		"-Dlog4j.output="+workspace.RawLogDirectory(),
		"-classpath", filepath.Join(launch.ToolHome, "lib", "*"),
		obdumperMainClass,
	)
	arguments = append(arguments, launch.BusinessArguments...)
	command := exec.CommandContext(ctx, launch.JavaPath, arguments...)
	command.Dir = launch.ToolHome
	command.Env = append([]string(nil), launch.Environment...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, ErrDirectJavaStartFailed
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, ErrDirectJavaStartFailed
	}
	if err := command.Start(); err != nil {
		return nil, ErrDirectJavaStartFailed
	}
	return &DirectJavaProcess{
		command: command,
		identity: ProcessIdentity{
			PID:              command.Process.Pid,
			StartedAt:        time.Now().UTC(),
			ExecutableDigest: launch.JavaSHA256,
			BootID:           launch.BootID,
		},
		stdout: stdout,
		stderr: stderr,
	}, nil
}

// Identity 返回用于本地恢复和状态事件的最小无秘密进程身份。
func (p *DirectJavaProcess) Identity() ProcessIdentity {
	if p == nil {
		return ProcessIdentity{}
	}
	return p.identity
}

// Stdout 返回未落盘的工具标准输出流；上层必须先脱敏后再保存或上报。
func (p *DirectJavaProcess) Stdout() io.ReadCloser {
	if p == nil {
		return nil
	}
	return p.stdout
}

// Stderr 返回未落盘的工具错误输出流；上层必须先脱敏后再保存或上报。
func (p *DirectJavaProcess) Stderr() io.ReadCloser {
	if p == nil {
		return nil
	}
	return p.stderr
}

// Wait 只报告进程是否退出与操作系统退出码，不将退出码解释为工具成功。
// OBDUMPER 终态还必须由脱敏日志和结果文件事实共同核对。
func (p *DirectJavaProcess) Wait() (exited bool, exitCode int, err error) {
	if p == nil || p.command == nil {
		return false, 0, ErrDirectJavaWaitFailed
	}
	err = p.command.Wait()
	if err == nil {
		return true, 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return true, exitError.ExitCode(), nil
	}
	return false, 0, ErrDirectJavaWaitFailed
}

func validateDirectJavaLaunch(workspace credential.Workspace, launch DirectJavaLaunch) error {
	if !opaqueIDPattern.MatchString(launch.BootID) || !workspace.OwnsSecurityConfiguration(launch.SecurityConfiguration) || !isAbsoluteExistingFile(launch.JavaPath) || !isAbsoluteDirectory(launch.ToolHome) || !isDigestOfFile(launch.JavaPath, launch.JavaSHA256) {
		return ErrDirectJavaLaunchInvalid
	}
	if info, err := os.Stat(filepath.Join(launch.ToolHome, "lib")); err != nil || !info.IsDir() {
		return ErrDirectJavaLaunchInvalid
	}
	for _, option := range launch.JVMOptions {
		if !jvmOptionPattern.MatchString(option) {
			return ErrDirectJavaLaunchInvalid
		}
	}
	for _, argument := range launch.BusinessArguments {
		if strings.ContainsRune(argument, 0) || strings.ContainsAny(argument, "\r\n") || argument == "--password" || strings.HasPrefix(argument, "--password=") {
			return ErrDirectJavaLaunchInvalid
		}
	}
	if len(launch.Environment) == 0 || hasForbiddenEnvironment(launch.Environment) {
		return ErrDirectJavaLaunchInvalid
	}
	return validateStartIntent(launch.Intent)
}

func isAbsoluteExistingFile(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isAbsoluteDirectory(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isDigestOfFile(path, expected string) bool {
	if !digestPattern.MatchString(expected) {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(content)
	return strings.EqualFold(hex.EncodeToString(digest[:]), expected)
}

func hasForbiddenEnvironment(environment []string) bool {
	seen := make(map[string]struct{}, len(environment))
	for _, item := range environment {
		key, _, found := strings.Cut(item, "=")
		if !found || key == "" || strings.ContainsRune(item, 0) {
			return true
		}
		key = strings.ToUpper(key)
		if _, duplicate := seen[key]; duplicate || key == "JAVA_OPTS" || key == "JAVA_TOOL_OPTIONS" || key == "JDK_JAVA_OPTIONS" || key == "_JAVA_OPTIONS" || key == "CLASSPATH" || key == "LD_PRELOAD" {
			return true
		}
		seen[key] = struct{}{}
	}
	return false
}
