// Package agentexecution 只编排已冻结的 OBDUMPER_EXPORT 信封。
// 它不接收浏览器参数、任意命令、任意 SQL 或任意文件操作。
package agentexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/agentexec"
	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/outputpath"
)

var (
	ErrInvalidConfiguration = errors.New("执行 Worker 配置无效")
	ErrExecutionRejected    = errors.New("执行任务信封被拒绝")
	ErrExecutionFailed      = errors.New("执行任务未完成")
)

// Protocol 收窄 Worker 可调用的机器协议能力，避免执行器获得任意 HTTP 调用入口。
type Protocol interface {
	ClaimNextExecution(context.Context, agentwire.ExecutionClaimNext) (agentwire.ExecutionGrant, bool, error)
	AcknowledgeExecutionLease(context.Context, agentwire.ExecutionLeaseAcknowledgement) error
	RenewExecutionLease(context.Context, agentwire.ExecutionLeaseRenewal) error
	PollExecutionControl(context.Context, agentwire.ExecutionControlPoll) (agentwire.ExecutionControl, error)
	ResolveExecutionDatabaseConnection(context.Context, agentwire.ExecutionSecretSlotRequest) (agentwire.DatabaseConnectionSlot, error)
	AppendExecutionEvent(context.Context, agentwire.ExecutionEvent) error
	AppendExecutionLog(context.Context, agentwire.ExecutionLogBatch) error
	AppendExecutionLogGap(context.Context, agentwire.ExecutionLogGap) error
}

// Runtime 是已在 Agent 本机关联状态中固化并复核的启动配置。
// 路径绝不从任务信封、浏览器或环境变量直接读取。
type Runtime struct {
	TargetPlatform   string
	JavaPath         string
	ToolHome         string
	WorkspaceRoot    string
	LogQueueRoot     string
	LogQueueObserver LogQueueObserver
	Environment      []string
	AllowedRoots     []string
}

// LogQueuePosition 是已完成第一层脱敏和密封的日志批次位置。
// 它刻意不携带记录正文、任务配置、命令、路径或任何秘密，只能用于 Agent 本地的受控观测。
type LogQueuePosition struct {
	StreamID    string
	SourceEpoch int64
	FirstSeq    int64
	LastSeq     int64
	BatchDigest string
}

// LogQueueObserver 仅观察已密封批次进入、持久化和确认后的三个固定时点。
// 实现不得通过该接口打开远程控制、读取原始日志或改变控制面确认后才删除批次的规则。
type LogQueueObserver interface {
	BeforeEnqueue(context.Context, LogQueuePosition) error
	AfterEnqueue(context.Context, LogQueuePosition) error
	AfterConfirmation(context.Context, LogQueuePosition) error
}

// Outcome 是一次领取后的无秘密执行结果投影。
type Outcome struct {
	TaskID            string
	ExecutionID       string
	Succeeded         bool
	FileCount         uint64
	TotalBytes        uint64
	CheckpointPresent bool
}

// Worker 串行执行当前 Agent 最多一条 OBDUMPER_EXPORT。
// 执行前只接受固定参数集合；任一校验失败都上报 START_REJECTED，绝不尝试替代命令或路径。
type Worker struct {
	Protocol Protocol
	Runtime  Runtime
	Clock    func() time.Time
	BootID   string

	mu      sync.Mutex
	running bool
}

// RunNext 领取、确认、解析唯一槽位、写入任务级安全材料、直接启动 Java，并上报事件与日志。
// 无工作时 found=false；任务已经领取后任何错误都不会自动重新领取或再次启动。
func (w *Worker) RunNext(ctx context.Context) (Outcome, bool, error) {
	if !w.enter() {
		return Outcome{}, false, ErrInvalidConfiguration
	}
	defer w.leave()
	if !w.validConfiguration() {
		return Outcome{}, false, ErrInvalidConfiguration
	}
	var queue *agentlogqueue.Queue
	if w.Runtime.LogQueueRoot != "" {
		var err error
		queue, err = agentlogqueue.Open(w.Runtime.LogQueueRoot, agentlogqueue.DefaultMaxBytes)
		if err != nil || flushQueuedLogs(ctx, queue, w.Protocol, w.BootID, "") != nil {
			return Outcome{}, false, ErrExecutionRejected
		}
	}
	grant, found, err := w.Protocol.ClaimNextExecution(ctx, agentwire.ExecutionClaimNext{BootID: w.BootID, SentAt: w.now()})
	if err != nil {
		return Outcome{}, false, err
	}
	if !found {
		return Outcome{}, false, nil
	}
	if !validGrant(grant) || !w.now().Before(grant.ExpiresAt) {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	actualArgvDigest, err := agentwire.ArgvDigest(grant.Argv)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	if err := w.Protocol.AcknowledgeExecutionLease(ctx, agentwire.ExecutionLeaseAcknowledgement{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, SentAt: w.now()}); err != nil {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
	}
	control, err := w.Protocol.PollExecutionControl(ctx, agentwire.ExecutionControlPoll{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, SentAt: w.now()})
	if err != nil {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
	}
	if control.CancelRequested {
		if err := w.appendEvent(ctx, grant, 3, "PROCESS_CANCELLED", map[string]any{"noProcess": true, "terminationCode": "NOT_STARTED", "treeObserved": true}); err != nil {
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
		}
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, nil
	}
	executionPaths, ok := executionPathsFromArgv(w.Runtime.TargetPlatform, grant.Argv)
	if !ok || (!executionPaths.StorageOutput && !withinAllowedRoots(w.Runtime.TargetPlatform, executionPaths.OutputPath, w.Runtime.AllowedRoots)) ||
		(executionPaths.LogPath != "" && !withinAllowedRoots(w.Runtime.TargetPlatform, executionPaths.LogPath, w.Runtime.AllowedRoots)) ||
		(executionPaths.TmpPath != "" && !withinAllowedRoots(w.Runtime.TargetPlatform, executionPaths.TmpPath, w.Runtime.AllowedRoots)) {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	var localOutputPath string
	if !executionPaths.StorageOutput {
		localOutputPath, ok = outputpath.LocalFilesystemPath(w.Runtime.TargetPlatform, executionPaths.OutputPath)
		if !ok {
			_ = w.appendStartRejected(ctx, grant)
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
		}
	}
	workspace, err := credential.CreateWorkspace(w.Runtime.WorkspaceRoot, grant.ExecutionID)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	cleanup := func() { _, _ = agentexec.CleanupSynthetic(workspace, true) }
	defer cleanup()
	slot, err := w.Protocol.ResolveExecutionDatabaseConnection(ctx, agentwire.ExecutionSecretSlotRequest{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, SentAt: w.now()})
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	defer slot.Destroy()
	material, err := credential.GenerateSecurityMaterial(slot.Password)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	defer material.Destroy()
	paths, err := workspace.WriteSecurityMaterial(&material)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	// EX-I6 存储凭据槽位（2026-08-14）：对象存储任务在 execution 私有目录生成 core-site.xml，
	// 通过 HADOOP_CONF_DIR 环境变量短时注入；密钥绝不进入 argv、日志或长期环境。
	toolEnvironment := append([]string(nil), w.Runtime.Environment...)
	if slot.StorageCredential != nil {
		storageContent, err := credential.GenerateStorageConfiguration(slot.StorageCredential.Provider, slot.StorageCredential.AccessKey, slot.StorageCredential.SecretKey)
		if err != nil {
			_ = w.appendStartRejected(ctx, grant)
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
		}
		defer credential.Zero(storageContent)
		confDir, err := workspace.WriteStorageConfiguration(storageContent)
		if err != nil {
			_ = w.appendStartRejected(ctx, grant)
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
		}
		toolEnvironment = append(toolEnvironment, "HADOOP_CONF_DIR="+confDir)
	}
	javaDigest, err := digestFile(w.Runtime.JavaPath)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	policy, err := executionLogPolicy(slot.Password, slot.StorageCredential)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	defer policy.Destroy()
	logs := newLogUploader(ctx, w.Protocol, w.BootID, grant, queue, w.Runtime.LogQueueObserver)
	process, err := agentexec.StartDirectJava(ctx, workspace, agentexec.DirectJavaLaunch{
		Intent: agentexec.StartIntent{ExecutionID: grant.ExecutionID, TaskID: grant.TaskID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, CreatedAt: w.now()},
		BootID: w.BootID, JavaPath: w.Runtime.JavaPath, JavaSHA256: javaDigest, ToolHome: w.Runtime.ToolHome,
		SecurityConfiguration: paths.SecurityConfiguration, BusinessArguments: append([]string(nil), grant.Argv...), Environment: toolEnvironment,
		Output: agentexec.DirectJavaOutput{Policy: policy, Stdout: agentexec.DirectJavaLogStream{StreamID: grant.ExecutionID, SourceEpoch: 1, ParserVersion: "direct-java-v1"}, Stderr: agentexec.DirectJavaLogStream{StreamID: grant.ExecutionID, SourceEpoch: 2, ParserVersion: "direct-java-v1"}, Sink: logs.append},
	})
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	// 进程已经启动后，必须先等待其退出再清理任务级安全文件；即使事件网络上报失败也不能提前删除仍被 Java 使用的文件。
	startedErr := w.appendEvent(ctx, grant, 3, "PROCESS_STARTED", map[string]any{"pid": process.Identity().PID, "startedAt": process.Identity().StartedAt.Format(time.RFC3339Nano), "executableDigest": process.Identity().ExecutableDigest, "bootId": process.Identity().BootID, "argvDigest": actualArgvDigest})
	exited, exitCode, waitErr, cancelRequested, treeObserved := w.waitWithLeaseRenewal(ctx, grant, process)
	if startedErr != nil {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, startedErr
	}
	if !exited {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionFailed
	}
	nextSequence := int64(4)
	if cancelRequested {
		terminationCode := "CANCEL_FAILED"
		if treeObserved {
			terminationCode = "PROCESS_TREE_TERMINATED"
		}
		if err := w.appendEvent(ctx, grant, nextSequence, "PROCESS_CANCELLED", map[string]any{"noProcess": false, "terminationCode": terminationCode, "treeObserved": treeObserved}); err != nil {
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
		}
		nextSequence++
	}
	if err := w.appendEvent(ctx, grant, nextSequence, "PROCESS_EXITED", map[string]any{"exitCode": exitCode}); err != nil {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
	}
	nextSequence++
	if cancelRequested {
		if err := w.appendEvent(ctx, grant, nextSequence, "TOOL_TERMINAL_OBSERVED", map[string]any{"terminal": "FAILED", "errorCode": agentwire.ExecutionErrorCancelledByRequest}); err != nil {
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
		}
		nextSequence++
		fileCount, totalBytes, files, _ := executionOutputFacts(executionPaths, localOutputPath)
		fileEvidence := make([]map[string]any, 0, len(files))
		for _, file := range files {
			fileEvidence = append(fileEvidence, map[string]any{"path": file.Path, "size": file.Size})
		}
		checkpoint := executionCheckpointPresent(executionPaths, localOutputPath)
		if err := w.appendEvent(ctx, grant, nextSequence, "RESULT_FACTS_OBSERVED", map[string]any{
			"result": "FAILED", "fileCount": fileCount, "totalBytes": totalBytes, "files": fileEvidence,
			"checkpointPresent": checkpoint, "errorCode": agentwire.ExecutionErrorCancelledByRequest,
		}); err != nil {
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID, CheckpointPresent: checkpoint}, true, ErrExecutionFailed
		}
		if !treeObserved {
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID, CheckpointPresent: checkpoint}, true, ErrExecutionFailed
		}
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID, CheckpointPresent: checkpoint}, true, nil
	}
	if waitErr != nil || exitCode != 0 {
		errorCode := agentwire.ExecutionErrorProcessExitNonZero
		if waitErr != nil {
			errorCode = agentwire.ExecutionErrorProcessWaitFailed
		}
		if err := w.appendEvent(ctx, grant, 5, "TOOL_TERMINAL_OBSERVED", map[string]any{"terminal": "FAILED", "errorCode": errorCode}); err != nil {
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
		}
		// EX-I8：失败路径同样上报结果事实与 dump.ckpt 存在性（作为失败终态后的迟到事实，
		// 由控制面按同一租约与连续序号接受）；这是失败任务检查点继续资格的唯一起点事实。
		fileCount, totalBytes, files, _ := executionOutputFacts(executionPaths, localOutputPath)
		fileEvidence := make([]map[string]any, 0, len(files))
		for _, file := range files {
			fileEvidence = append(fileEvidence, map[string]any{"path": file.Path, "size": file.Size})
		}
		checkpoint := executionCheckpointPresent(executionPaths, localOutputPath)
		if err := w.appendEvent(ctx, grant, 6, "RESULT_FACTS_OBSERVED", map[string]any{
			"result": "FAILED", "fileCount": fileCount, "totalBytes": totalBytes, "files": fileEvidence,
			"checkpointPresent": checkpoint, "errorCode": errorCode,
		}); err != nil {
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID, CheckpointPresent: checkpoint}, true, ErrExecutionFailed
		}
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID, CheckpointPresent: checkpoint}, true, ErrExecutionFailed
	}
	if err := w.appendEvent(ctx, grant, 5, "TOOL_TERMINAL_OBSERVED", map[string]any{"terminal": "SUCCEEDED"}); err != nil {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
	}
	fileCount, totalBytes, files, outputOK := executionOutputFacts(executionPaths, localOutputPath)
	result := "FAILED"
	if outputOK && (executionPaths.StorageOutput || fileCount > 0 && totalBytes > 0) {
		result = "VERIFIED"
	}
	fileEvidence := make([]map[string]any, 0, len(files))
	for _, file := range files {
		fileEvidence = append(fileEvidence, map[string]any{"path": file.Path, "size": file.Size})
	}
	// EX-I8 结果与失败事实：结果事实与 dump.ckpt 存在性合并为同一事件上报。
	// 结果事实事件会触发任务终态转换与租约释放，检查点事实必须随它一起到达；
	// 失败任务的检查点继续资格由控制面依据该事实与冻结快照共同判定。
	checkpoint := executionCheckpointPresent(executionPaths, localOutputPath)
	resultEvidence := map[string]any{
		"result": result, "fileCount": fileCount, "totalBytes": totalBytes, "files": fileEvidence,
		"checkpointPresent": checkpoint,
	}
	if result == "FAILED" {
		resultEvidence["errorCode"] = agentwire.ExecutionErrorResultVerification
	}
	if err := w.appendEvent(ctx, grant, 6, "RESULT_FACTS_OBSERVED", resultEvidence); err != nil {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, err
	}
	return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID, Succeeded: result == "VERIFIED", FileCount: fileCount, TotalBytes: totalBytes, CheckpointPresent: checkpoint}, true, nil
}

func (w *Worker) appendStartRejected(ctx context.Context, grant agentwire.ExecutionGrant) error {
	return w.appendEvent(ctx, grant, 3, "START_REJECTED", map[string]any{"noProcess": true, "errorCode": agentwire.ExecutionErrorStartRejected})
}

func (w *Worker) appendEvent(ctx context.Context, grant agentwire.ExecutionGrant, sequence int64, eventType string, evidence map[string]any) error {
	eventID, err := identifier.NewUUIDV4()
	if err != nil {
		return ErrExecutionRejected
	}
	return w.Protocol.AppendExecutionEvent(ctx, agentwire.ExecutionEvent{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, EventID: eventID, EventSeq: sequence, EventType: eventType, Evidence: evidence, SentAt: w.now()})
}

// waitWithLeaseRenewal 在受控进程存活期间定期续期，避免长导出因初始短租约到期而失去结果归属。
// 续期失败会在下一周期重试；无论网络状态如何都先回收已启动进程，防止清理安全材料时破坏运行中的 Java。
func (w *Worker) waitWithLeaseRenewal(ctx context.Context, grant agentwire.ExecutionGrant, process *agentexec.DirectJavaProcess) (bool, int, error, bool, bool) {
	type waitResult struct {
		exited bool
		code   int
		err    error
	}
	completed := make(chan waitResult, 1)
	go func() {
		exited, code, err := process.Wait()
		completed <- waitResult{exited: exited, code: code, err: err}
	}()
	renewTicker := time.NewTicker(30 * time.Second)
	defer renewTicker.Stop()
	pollTicker := time.NewTicker(2 * time.Second)
	defer pollTicker.Stop()
	cancelRequested := false
	treeObserved := false
	for {
		select {
		case result := <-completed:
			return result.exited, result.code, result.err, cancelRequested, treeObserved
		case <-renewTicker.C:
			_ = w.Protocol.RenewExecutionLease(ctx, agentwire.ExecutionLeaseRenewal{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, SentAt: w.now()})
		case <-pollTicker.C:
			if cancelRequested {
				continue
			}
			control, err := w.Protocol.PollExecutionControl(ctx, agentwire.ExecutionControlPoll{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, SentAt: w.now()})
			if err != nil || !control.CancelRequested {
				continue
			}
			cancelRequested = true
			if process.Cancel() == nil {
				treeObserved = true
			}
		case <-ctx.Done():
			// exec.CommandContext 已绑定同一上下文；仍继续等待，直到子进程已被回收。
			ctx = context.Background()
		}
	}
}

func (w *Worker) validConfiguration() bool {
	if w.Protocol == nil || w.Clock == nil || w.BootID == "" || filepath.IsAbs(w.Runtime.JavaPath) == false || filepath.IsAbs(w.Runtime.ToolHome) == false || filepath.IsAbs(w.Runtime.WorkspaceRoot) == false || len(w.Runtime.Environment) == 0 || len(w.Runtime.AllowedRoots) == 0 {
		return false
	}
	for _, root := range w.Runtime.AllowedRoots {
		if !outputpath.IsAllowedRootPath(w.Runtime.TargetPlatform, root) {
			return false
		}
	}
	return true
}
func (w *Worker) now() time.Time { return w.Clock().UTC() }
func (w *Worker) enter() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return false
	}
	w.running = true
	return true
}
func (w *Worker) leave() { w.mu.Lock(); defer w.mu.Unlock(); w.running = false }

func validGrant(grant agentwire.ExecutionGrant) bool {
	if grant.TaskID == "" || grant.ExecutionID == "" || grant.LeaseID == "" || grant.LeaseEpoch < 1 || len(grant.EnvelopeDigest) != 64 || len(grant.Argv) == 0 || len(grant.Argv) > 32 {
		return false
	}
	_, ok := executionPathArguments(grant.Argv)
	return ok
}

// executionPaths 是从冻结参数令牌中识别出的受控文件系统路径。
// 日志目录与导出目录均要在启动前核验，避免额外路径参数扩大 Agent 的写入范围。
type executionPaths struct {
	OutputPath    string
	LogPath       string
	TmpPath       string
	StorageOutput bool
}

func executionPathsFromArgv(platform string, argv []string) (executionPaths, bool) {
	paths, ok := executionPathArguments(argv)
	if !ok {
		return executionPaths{}, false
	}
	paths.StorageOutput = isControlledStorageOutputPath(paths.OutputPath)
	if paths.StorageOutput {
		if paths.LogPath != "" || (paths.TmpPath != "" && !outputpath.IsExportOutputPath(platform, paths.TmpPath)) {
			return executionPaths{}, false
		}
		return paths, true
	}
	if !outputpath.IsExportOutputPath(platform, paths.OutputPath) || (paths.LogPath != "" && !outputpath.IsExportOutputPath(platform, paths.LogPath)) || (paths.TmpPath != "" && !outputpath.IsExportOutputPath(platform, paths.TmpPath)) {
		return executionPaths{}, false
	}
	return paths, true
}

func outputPathFromArgv(platform string, argv []string) (string, bool) {
	paths, ok := executionPathsFromArgv(platform, argv)
	return paths.OutputPath, ok
}

func outputPathArgument(argv []string) (string, bool) {
	paths, ok := executionPathArguments(argv)
	return paths.OutputPath, ok
}

func executionPathArguments(argv []string) (executionPaths, bool) {
	paths := executionPaths{}
	seen := map[string]bool{}
	for index := 0; index < len(argv); index++ {
		value := argv[index]
		if strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") || value == "--password" || strings.HasPrefix(value, "--password=") || strings.HasPrefix(value, "-p") {
			return executionPaths{}, false
		}
		if value != "--file-path" && value != "--log-path" && value != "--tmp-path" {
			continue
		}
		if index+1 >= len(argv) || strings.ContainsRune(argv[index+1], 0) || strings.ContainsAny(argv[index+1], "\r\n") || strings.TrimSpace(argv[index+1]) != argv[index+1] {
			return executionPaths{}, false
		}
		key := "file"
		if value == "--log-path" {
			key = "log"
		} else if value == "--tmp-path" {
			key = "tmp"
		}
		if seen[key] {
			return executionPaths{}, false
		}
		seen[key] = true
		if key == "file" {
			paths.OutputPath = argv[index+1]
		} else if key == "log" {
			paths.LogPath = argv[index+1]
		} else {
			paths.TmpPath = argv[index+1]
		}
		index++
	}
	return paths, paths.OutputPath != ""
}

// isControlledStorageOutputPath 只接受控制面已冻结的四种对象存储 URI 形态。
// URI 参数沿用草稿契约，不在 Agent 侧扩展新的目标或秘密传递方式。
func isControlledStorageOutputPath(value string) bool {
	if len(value) == 0 || len(value) > 4096 || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/") {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "oss", "s3", "cos", "obs":
	default:
		return false
	}
	for key := range parsed.Query() {
		if key != "endpoint" && key != "region" && key != "storage-class" {
			return false
		}
	}
	return true
}

// executionLogPolicy 保存当前 execution 的全部秘密值用于 Agent 第一层日志脱敏。
// 用户名是命令身份的一部分，应保留在工具日志中以便人工诊断连接对象。
func executionLogPolicy(password []byte, storageCredential *agentwire.StorageCredentialSlot) (logstream.Policy, error) {
	secrets := [][]byte{password}
	if storageCredential != nil {
		secrets = append(secrets, storageCredential.AccessKey, storageCredential.SecretKey)
	}
	return logstream.NewBytePolicy("execution-redaction-v1", secrets, nil)
}

func withinAllowedRoots(platform, path string, roots []string) bool {
	for _, root := range roots {
		if outputpath.WithinAllowedRoot(platform, path, root) {
			return true
		}
	}
	return false
}

func digestFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	defer credential.Zero(content)
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}

// outputFileFact 是执行结果的单个文件事实：只含输出目录内的相对路径与字节数，不含内容或校验和。
type outputFileFact struct {
	Path string
	Size uint64
}

// maxOutputFileFacts 限制随结果事件上报的文件清单长度，避免异常输出目录撑大事件载荷。
const maxOutputFileFacts = 100

// maxOutputFilePath 限制单个结果文件相对路径长度；含控制字符或越界时整个结果事实失败关闭。
const maxOutputFilePath = 512

// outputFacts 只读枚举输出目录内的事实：常规文件数量、总字节数与受限相对路径清单。
// 符号链接、读取失败或超限都返回 ok=false，不把未验证事实当作可靠结果。
func outputFacts(path string) (uint64, uint64, []outputFileFact, bool) {
	var count, bytes uint64
	var files []outputFileFact
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("output contains symbolic link")
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if count == ^uint64(0) {
			return errors.New("output file count overflow")
		}
		count++
		if info.Size() < 0 {
			return errors.New("invalid output size")
		}
		size := uint64(info.Size())
		if ^uint64(0)-bytes < size {
			return errors.New("output byte count overflow")
		}
		bytes += size
		if len(files) < maxOutputFileFacts {
			relative, err := filepath.Rel(path, current)
			if err != nil || relative == "" || len(relative) > maxOutputFilePath || strings.ContainsAny(relative, "\x00\r\n") {
				return errors.New("output file path is invalid")
			}
			files = append(files, outputFileFact{Path: filepath.ToSlash(relative), Size: size})
		}
		return nil
	})
	return count, bytes, files, err == nil
}

// executionOutputFacts 按输出形态生成结果事实。
// 本地输出继续要求可枚举的非空文件事实；对象存储在 EX-V1 远端清单取证完成前没有本地结果目录，
// 由受控进程正常退出与固定工具成功终态确认结果，文件清单保持空值且不伪造远端对象。
func executionOutputFacts(paths executionPaths, localOutputPath string) (uint64, uint64, []outputFileFact, bool) {
	if paths.StorageOutput {
		return 0, 0, []outputFileFact{}, true
	}
	return outputFacts(localOutputPath)
}

// executionCheckpointPresent 只允许本地输出目录提供检查点事实；对象存储不把临时目录伪装为正式输出目录。
func executionCheckpointPresent(paths executionPaths, localOutputPath string) bool {
	return !paths.StorageOutput && checkpointPresent(localOutputPath)
}

// checkpointPresent 只做存在性/可读性检查：dump.ckpt 必须是输出目录下的常规可读文件。
// 不读取或解析检查点内容；继续资格的最终判定由控制面依据冻结快照完成。
func checkpointPresent(outputPath string) bool {
	if outputPath == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(outputPath, "dump.ckpt"))
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	file, err := os.Open(filepath.Join(outputPath, "dump.ckpt"))
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

type logUploader struct {
	ctx      context.Context
	protocol Protocol
	bootID   string
	grant    agentwire.ExecutionGrant
	mu       sync.Mutex
	previous map[int64]string
	queue    *agentlogqueue.Queue
	observer LogQueueObserver
}

func newLogUploader(ctx context.Context, protocol Protocol, bootID string, grant agentwire.ExecutionGrant, queue *agentlogqueue.Queue, observer LogQueueObserver) *logUploader {
	return &logUploader{ctx: ctx, protocol: protocol, bootID: bootID, grant: grant, previous: make(map[int64]string), queue: queue, observer: observer}
}
func (u *logUploader) append(record logstream.Record) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	batch := logstream.Batch{StreamID: record.StreamID, SourceEpoch: record.SourceEpoch, FirstSeq: record.SourceSeq, LastSeq: record.SourceSeq, PreviousDigest: u.previous[record.SourceEpoch], PolicyVersion: record.PolicyVersion, Records: []logstream.Record{record}}
	sealed, err := logstream.SealBatch(batch)
	if err != nil {
		return err
	}
	entry := agentlogqueue.Entry{ExecutionID: u.grant.ExecutionID, LeaseID: u.grant.LeaseID, LeaseEpoch: u.grant.LeaseEpoch, EnvelopeDigest: u.grant.EnvelopeDigest, Batch: sealed}
	if u.queue != nil {
		position := queuePosition(entry)
		if u.observer != nil {
			if err := u.observer.BeforeEnqueue(u.ctx, position); err != nil {
				return err
			}
		}
		if err := u.queue.Enqueue(entry); err != nil {
			if !errors.Is(err, agentlogqueue.ErrFull) {
				return err
			}
			if flushQueuedLogs(u.ctx, u.queue, u.protocol, u.bootID, "") == nil {
				if retryErr := u.queue.Enqueue(entry); retryErr == nil {
					err = nil
				} else {
					err = retryErr
				}
			}
			if err != nil {
				if !errors.Is(err, agentlogqueue.ErrFull) {
					return err
				}
				return u.enqueueLocalSpoolLimitGap(record)
			}
		}
		if err := recordQueueEvidence(u.queue, agentlogqueue.EvidenceEnqueued, entry); err != nil {
			return err
		}
		if u.observer != nil {
			if err := u.observer.AfterEnqueue(u.ctx, position); err != nil {
				return err
			}
		}
		if err := flushQueuedLogs(u.ctx, u.queue, u.protocol, u.bootID, entry.Batch.Digest); err == nil && u.observer != nil {
			if err := u.observer.AfterConfirmation(u.ctx, position); err != nil {
				return err
			}
		}
	} else if err := u.protocol.AppendExecutionLog(u.ctx, agentwire.ExecutionLogBatch{BootID: u.bootID, ExecutionID: entry.ExecutionID, LeaseID: entry.LeaseID, LeaseEpoch: entry.LeaseEpoch, EnvelopeDigest: entry.EnvelopeDigest, Batch: entry.Batch, SentAt: time.Now().UTC()}); err != nil {
		return err
	}
	u.previous[record.SourceEpoch] = sealed.Digest
	return nil
}

// enqueueLocalSpoolLimitGap 在正文批次无法持久化时保存同一来源的无正文缺口。
// 缺口成功入队后继续排空工具来源；若连缺口都不能持久化则返回错误，由上层以失败关闭处理。
func (u *logUploader) enqueueLocalSpoolLimitGap(record logstream.Record) error {
	return u.queue.EnqueueGap(agentlogqueue.GapEntry{
		ExecutionID: u.grant.ExecutionID, LeaseID: u.grant.LeaseID, LeaseEpoch: u.grant.LeaseEpoch, EnvelopeDigest: u.grant.EnvelopeDigest,
		Gap: logstream.GapNotice{
			StreamID: record.StreamID, SourceKind: record.SourceKind, SourceEpoch: record.SourceEpoch,
			FirstSeq: record.SourceSeq, LastSeq: record.SourceSeq, ReasonCode: "LOCAL_SPOOL_LIMIT",
			PolicyVersion: record.PolicyVersion, ParserVersion: record.ParserVersion,
		},
	})
}

func queuePosition(entry agentlogqueue.Entry) LogQueuePosition {
	return LogQueuePosition{
		StreamID:    entry.Batch.StreamID,
		SourceEpoch: entry.Batch.SourceEpoch,
		FirstSeq:    entry.Batch.FirstSeq,
		LastSeq:     entry.Batch.LastSeq,
		BatchDigest: entry.Batch.Digest,
	}
}

func flushQueuedLogs(ctx context.Context, queue *agentlogqueue.Queue, protocol Protocol, bootID, newlyEnqueuedDigest string) error {
	items, err := queue.PendingItems()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Gap != nil {
			gap := item.Gap
			if err := protocol.AppendExecutionLogGap(ctx, agentwire.ExecutionLogGap{BootID: bootID, ExecutionID: gap.ExecutionID, LeaseID: gap.LeaseID, LeaseEpoch: gap.LeaseEpoch, EnvelopeDigest: gap.EnvelopeDigest, Gap: gap.Gap, SentAt: time.Now().UTC()}); err != nil {
				return err
			}
			if err := queue.Acknowledge(item.Token); err != nil {
				return err
			}
			continue
		}
		entry := item.Entry
		evidenceType := agentlogqueue.EvidenceRecoveryReplayAttempt
		if newlyEnqueuedDigest != "" && entry.Batch.Digest == newlyEnqueuedDigest {
			evidenceType = agentlogqueue.EvidenceUploadAttempt
		} else if newlyEnqueuedDigest != "" {
			evidenceType = agentlogqueue.EvidenceRetryAttempt
		}
		if err := recordQueueEvidence(queue, evidenceType, *entry); err != nil {
			return err
		}
		if err := protocol.AppendExecutionLog(ctx, agentwire.ExecutionLogBatch{BootID: bootID, ExecutionID: entry.ExecutionID, LeaseID: entry.LeaseID, LeaseEpoch: entry.LeaseEpoch, EnvelopeDigest: entry.EnvelopeDigest, Batch: entry.Batch, SentAt: time.Now().UTC()}); err != nil {
			if evidenceErr := recordQueueEvidence(queue, agentlogqueue.EvidenceUploadFailed, *entry); evidenceErr != nil {
				return evidenceErr
			}
			return err
		}
		if err := recordQueueEvidence(queue, agentlogqueue.EvidenceControlPlaneConfirmed, *entry); err != nil {
			return err
		}
		if err := queue.Acknowledge(item.Token); err != nil {
			return err
		}
	}
	return nil
}

func recordQueueEvidence(queue *agentlogqueue.Queue, evidenceType agentlogqueue.EvidenceType, entry agentlogqueue.Entry) error {
	return queue.RecordEvidence(agentlogqueue.Evidence{
		Type: evidenceType, StreamID: entry.Batch.StreamID, SourceEpoch: entry.Batch.SourceEpoch, FirstSeq: entry.Batch.FirstSeq,
		LastSeq: entry.Batch.LastSeq, BatchDigest: entry.Batch.Digest, OccurredAt: time.Now().UTC(),
	})
}
