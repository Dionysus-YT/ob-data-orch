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
	"sync"
	"time"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/logstream"
)

const obdumperMainClass = "com.oceanbase.tools.loaddump.cmd.Obdumper"

var (
	ErrDirectJavaLaunchInvalid = errors.New("直接 Java 启动请求无效")
	ErrDirectJavaStartFailed   = errors.New("直接 Java 进程启动失败")
	ErrDirectJavaWaitFailed    = errors.New("直接 Java 进程等待失败")
	ErrDirectJavaOutputFailed  = errors.New("直接 Java 输出采集失败")
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
	Output                DirectJavaOutput
}

// DirectJavaLogStream 是一个来源固定的工具管道流标识。
// streamId、epoch 和解析器版本必须由不可变任务信封与日志策略共同确定。
type DirectJavaLogStream struct {
	StreamID      string
	SourceEpoch   int64
	ParserVersion string
}

// DirectJavaOutput 定义直接 Java 进程的强制第一层脱敏出口。
// Sink 只能接收到已脱敏记录；实现必须快速入队，采集器会串行调用它以避免两个管道并发写入同一队列。
type DirectJavaOutput struct {
	Policy logstream.Policy
	Stdout DirectJavaLogStream
	Stderr DirectJavaLogStream
	Sink   func(logstream.Record) error
}

// DirectJavaProcess 只暴露受控进程身份和已启动的脱敏采集结果。
// 原始 stdout/stderr 永不向调用方暴露，避免上层遗漏第一层脱敏而直接持久化工具输出。
type DirectJavaProcess struct {
	command    *exec.Cmd
	identity   ProcessIdentity
	outputDone <-chan error
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
	outputDone := make(chan error, 1)
	policy := launch.Output.Policy.Clone()
	go collectDirectJavaOutput(stdout, stderr, launch.Output, policy, outputDone)
	return &DirectJavaProcess{
		command: command,
		identity: ProcessIdentity{
			PID:              command.Process.Pid,
			StartedAt:        time.Now().UTC(),
			ExecutableDigest: launch.JavaSHA256,
			BootID:           launch.BootID,
		},
		outputDone: outputDone,
	}, nil
}

// Identity 返回用于本地恢复和状态事件的最小无秘密进程身份。
func (p *DirectJavaProcess) Identity() ProcessIdentity {
	if p == nil {
		return ProcessIdentity{}
	}
	return p.identity
}

// Wait 只报告进程是否退出与操作系统退出码，不将退出码解释为工具成功。
// 它先等待两个管道排空，再回收进程，避免 Wait 过早关闭管道导致末尾日志丢失。
// OBDUMPER 终态还必须由脱敏日志和结果文件事实共同核对。
func (p *DirectJavaProcess) Wait() (exited bool, exitCode int, err error) {
	if p == nil || p.command == nil || p.outputDone == nil {
		return false, 0, ErrDirectJavaWaitFailed
	}
	outputErr := <-p.outputDone
	err = p.command.Wait()
	if err == nil {
		if outputErr != nil {
			return true, 0, ErrDirectJavaOutputFailed
		}
		return true, 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if outputErr != nil {
			return true, exitError.ExitCode(), ErrDirectJavaOutputFailed
		}
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
	if len(launch.Environment) == 0 || hasForbiddenEnvironment(launch.Environment) || !validDirectJavaOutput(launch.Output) {
		return ErrDirectJavaLaunchInvalid
	}
	return validateStartIntent(launch.Intent)
}

func validDirectJavaOutput(output DirectJavaOutput) bool {
	if output.Sink == nil || len(output.Policy.Secrets) != 0 || len(output.Policy.Identifiers) != 0 || len(output.Policy.BytesSecrets) == 0 {
		return false
	}
	for _, stream := range []DirectJavaLogStream{output.Stdout, output.Stderr} {
		if !opaqueIDPattern.MatchString(stream.StreamID) || stream.SourceEpoch < 1 || strings.TrimSpace(stream.ParserVersion) == "" {
			return false
		}
	}
	redacted, err := output.Policy.RedactBytes(nil)
	zeroBytes(redacted)
	return err == nil
}

func collectDirectJavaOutput(stdout, stderr io.ReadCloser, output DirectJavaOutput, policy logstream.Policy, done chan<- error) {
	defer policy.Destroy()
	defer close(done)
	var group sync.WaitGroup
	errors := make(chan error, 2)
	var sinkMu sync.Mutex
	serializedSink := func(record logstream.Record) error {
		sinkMu.Lock()
		defer sinkMu.Unlock()
		return output.Sink(record)
	}
	group.Add(2)
	go func() {
		defer group.Done()
		errors <- collectDirectJavaStream(stdout, logstream.SourceStdout, output.Stdout, policy, serializedSink)
	}()
	go func() {
		defer group.Done()
		errors <- collectDirectJavaStream(stderr, logstream.SourceStderr, output.Stderr, policy, serializedSink)
	}()
	group.Wait()
	close(errors)
	for result := range errors {
		if result != nil {
			done <- ErrDirectJavaOutputFailed
			return
		}
	}
	done <- nil
}

func collectDirectJavaStream(reader io.ReadCloser, sourceKind logstream.SourceKind, stream DirectJavaLogStream, policy logstream.Policy, sink func(logstream.Record) error) error {
	defer reader.Close()
	var reassembler logstream.Reassembler
	sequence := int64(0)
	var sinkFailed bool
	emit := func(kind logstream.RecordKind, message, integrityCode string) error {
		sequence++
		record := logstream.Record{StreamID: stream.StreamID, SourceKind: sourceKind, SourceEpoch: stream.SourceEpoch, SourceSeq: sequence, Kind: kind, Message: message, IntegrityCode: integrityCode, PolicyVersion: policy.Version, ParserVersion: stream.ParserVersion, ReceivedAt: time.Now().UTC()}
		if sinkFailed {
			return nil
		}
		if err := sink(record); err != nil {
			sinkFailed = true
			return ErrDirectJavaOutputFailed
		}
		return nil
	}
	consume := func(records [][]byte, truncated bool) error {
		var result error
		if truncated {
			if err := emit(logstream.RecordTruncated, "原始日志记录超过安全长度，已丢弃", "RECORD_TOO_LARGE"); err != nil {
				result = err
			}
		}
		for _, raw := range records {
			redacted, err := policy.RedactBytes(raw)
			zeroBytes(raw)
			if err != nil {
				result = ErrDirectJavaOutputFailed
				continue
			}
			message := string(redacted)
			zeroBytes(redacted)
			if err := emit(logstream.RecordLog, message, ""); err != nil {
				result = err
			}
		}
		return result
	}
	buffer := make([]byte, 32*1024)
	defer zeroBytes(buffer)
	var collectErr error
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			records, truncated := reassembler.PushBytes(buffer[:count])
			zeroBytes(buffer[:count])
			if consume(records, truncated) != nil {
				collectErr = ErrDirectJavaOutputFailed
			}
		}
		if errors.Is(err, io.EOF) {
			records, truncated := reassembler.FinishBytes()
			if consume(records, truncated) != nil {
				collectErr = ErrDirectJavaOutputFailed
			}
			return collectErr
		}
		if err != nil {
			return ErrDirectJavaOutputFailed
		}
	}
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
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
