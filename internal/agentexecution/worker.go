// Package agentexecution 编排已冻结的官方工具信封；各任务类型分别复核业务与结果边界。
// 它不接收浏览器参数、任意命令、任意 SQL 或任意文件操作。
package agentexecution

import (
	"bytes"
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
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/nodefacts"
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
	TargetPlatform string
	JavaPath       string
	ToolHome       string
	WorkspaceRoot  string
	LogQueueRoot   string
	// SharedLogQueue 由 Agent 装配层持有；并行 Worker 共用容量缓存与文件锁。
	SharedLogQueue   *agentlogqueue.Queue
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

// Worker 每个实例最多执行一条显式类型的冻结任务；并行实例由装配层和控制面预算约束。
// 执行前只接受固定参数集合；任一校验失败都上报 START_REJECTED，绝不尝试替代命令或路径。
type Worker struct {
	// TaskType 缺省只领取导出；普通导入由装配层显式传入独立能力。
	TaskType string
	Protocol Protocol
	Runtime  Runtime
	Clock    func() time.Time
	BootID   string

	mu      sync.Mutex
	running bool
	active  *nodefacts.ActiveExecution
	// 包内合成验证可注入离散时钟信号；正式装配始终使用固定 30 秒续租周期。
	leaseRenewalTicks <-chan time.Time
}

// RunNext 领取、确认、解析唯一槽位、写入任务级安全材料、直接启动 Java，并上报事件与日志。
// 无工作时 found=false；任务已经领取后任何错误都不会自动重新领取或再次启动。
func (w *Worker) RunNext(ctx context.Context) (outcome Outcome, found bool, runErr error) {
	if !w.enter() {
		return Outcome{}, false, ErrInvalidConfiguration
	}
	defer w.leave()
	if !w.validConfiguration() {
		return Outcome{}, false, ErrInvalidConfiguration
	}
	queue := w.Runtime.SharedLogQueue
	if queue == nil {
		var err error
		root := w.Runtime.LogQueueRoot
		if root == "" {
			root = filepath.Join(w.Runtime.WorkspaceRoot, "log-queue")
		}
		queue, err = agentlogqueue.Open(root, agentlogqueue.DefaultMaxBytes)
		if err != nil {
			return Outcome{}, false, ErrExecutionRejected
		}
	}
	// 尾部补传独立于新任务准入；残留护栏不能阻止已有可靠队列重放。
	if flushQueuedLogs(ctx, queue, w.Protocol, w.BootID, "") != nil {
		return Outcome{}, false, ErrExecutionRejected
	}
	if err := credential.CheckWorkspaceRoot(w.Runtime.WorkspaceRoot); err != nil {
		return Outcome{}, false, err
	}
	grant, found, err := w.Protocol.ClaimNextExecution(ctx, agentwire.ExecutionClaimNext{TaskType: w.TaskType, BootID: w.BootID, SentAt: w.now()})
	if err != nil {
		return Outcome{}, false, err
	}
	if !found {
		return Outcome{}, false, nil
	}
	if !validGrant(grant) || !w.now().Before(grant.ExpiresAt) || executionTaskType(w.TaskType) != executionTaskType(grant.TaskType) {
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	w.mu.Lock()
	w.active = &nodefacts.ActiveExecution{ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch}
	w.mu.Unlock()
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
	isImport := grant.TaskType == importdomain.TaskType
	isDirect := grant.TaskType == directload.TaskType
	if isImport {
		executionPaths, ok = w.normalImportPaths(ctx, grant)
	} else if isDirect {
		executionPaths, ok = w.directLoadPaths(ctx, grant)
	}
	if !ok || (!isImport && !isDirect && !executionPaths.StorageOutput && !withinAllowedRoots(w.Runtime.TargetPlatform, executionPaths.OutputPath, w.Runtime.AllowedRoots)) ||
		(!isImport && !isDirect && executionPaths.LogPath != "" && !withinAllowedRoots(w.Runtime.TargetPlatform, executionPaths.LogPath, w.Runtime.AllowedRoots)) ||
		(executionPaths.TmpPath != "" && !withinAllowedRoots(w.Runtime.TargetPlatform, executionPaths.TmpPath, w.Runtime.AllowedRoots)) {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	var localOutputPath string
	if !isImport && !isDirect && !executionPaths.StorageOutput {
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
	cleanupReady := true
	processStopped := true
	var materialCleanupErr error
	defer func() {
		cleanupErr := materialCleanupErr
		if cleanupErr != nil {
			workspace.Isolate()
		} else if processStopped {
			_, cleanupErr = agentexec.CleanupStoppedExecution(workspace, cleanupReady)
		} else {
			workspace.Isolate()
			cleanupErr = agentexec.ErrCleanupNotReady
		}
		if cleanupErr != nil {
			outcome.Succeeded = false
			runErr = errors.Join(runErr, cleanupErr)
		}
	}()
	slot, err := w.Protocol.ResolveExecutionDatabaseConnection(ctx, agentwire.ExecutionSecretSlotRequest{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, SentAt: w.now()})
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	defer slot.Destroy()
	if !validExecutionSysSlot(grant.Argv, slot) || (isImport || isDirect) && slot.StorageCredential != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	material, err := credential.GenerateSecurityMaterialWithSys(slot.Password, slot.SysPassword)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	defer material.Destroy()
	paths, err := workspace.WriteSecurityMaterial(&material)
	material.Destroy()
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
		credential.Zero(storageContent)
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
	sysUser := []byte(executionSysUserArgument(grant.Argv))
	defer credential.Zero(sysUser)
	policy, err := executionLogPolicy(slot.Password, slot.StorageCredential, slot.SysPassword, slot.SysUsername, sysUser)
	if err != nil {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	defer policy.Destroy()
	logs := newLogUploader(ctx, w.Protocol, w.BootID, grant, queue, w.Runtime.LogQueueObserver)
	logs.start()
	defer logs.stop()
	tool, parser := "", "direct-java-v1"
	stdout, stderr := agentexec.DirectJavaLogStream{StreamID: grant.ExecutionID, SourceEpoch: 1, ParserVersion: parser}, agentexec.DirectJavaLogStream{StreamID: grant.ExecutionID, SourceEpoch: 2, ParserVersion: parser}
	terminal := &normalImportTerminal{}
	directTerminal := &directLoadTerminal{}
	exportResult := &exportTerminal{}
	sink := func(record logstream.Record) error { exportResult.observe(record); return logs.append(record) }
	if isImport {
		// 解析只接收第一层脱敏后的固定工具管道；日志缺口或未知完成标记不能解锁成功。
		tool, parser = "OBLOADER", "obloader-4.3.5-v1"
		stdout.SourceKind, stderr.SourceKind = logstream.SourceLoaderStdout, logstream.SourceLoaderStderr
		stdout.ParserVersion, stderr.ParserVersion = parser, parser
		sink = func(record logstream.Record) error { terminal.observe(record); return logs.append(record) }
		if _, valid := w.normalImportPaths(ctx, grant); !valid || !w.now().Before(grant.ExpiresAt) {
			_ = w.appendStartRejected(ctx, grant)
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
		}
	}
	if isDirect {
		tool, parser = "OBLOADER_DIRECT_LOAD", directLoadParserVersion
		stdout.SourceKind, stderr.SourceKind = logstream.SourceLoaderStdout, logstream.SourceLoaderStderr
		stdout.ParserVersion, stderr.ParserVersion = parser, parser
		directTerminal.table = grant.DirectLoad.Config.Table
		sink = func(record logstream.Record) error { directTerminal.observe(record); return logs.append(record) }
		// 文件冻结在解析秘密前和启动前分别复核，不能将预检查后的改写默认为原输入。
		if _, valid := w.directLoadPaths(ctx, grant); !valid || !w.now().Before(grant.ExpiresAt) {
			_ = w.appendStartRejected(ctx, grant)
			return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
		}
	}
	if !w.now().Before(grant.ExpiresAt) {
		_ = w.appendStartRejected(ctx, grant)
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	process, err := agentexec.StartDirectJava(ctx, workspace, agentexec.DirectJavaLaunch{
		Tool: tool, LeaseExpiresAt: grant.ExpiresAt, Clock: w.now,
		Intent: agentexec.StartIntent{ExecutionID: grant.ExecutionID, TaskID: grant.TaskID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, CreatedAt: w.now()},
		BootID: w.BootID, JavaPath: w.Runtime.JavaPath, JavaSHA256: javaDigest, ToolHome: w.Runtime.ToolHome,
		SecurityConfiguration: paths.SecurityConfiguration, BusinessArguments: append([]string(nil), grant.Argv...), Environment: toolEnvironment,
		Output: agentexec.DirectJavaOutput{Policy: policy, Stdout: stdout, Stderr: stderr, Sink: sink},
	})
	// 启动器已克隆采集策略；父层不再需要秘密缓冲，避免网络事件等待延长其生命周期。
	slot.Destroy()
	policy.Destroy()
	if err != nil {
		// 已创建而监管未成立的进程不能被当作无进程；保留分配，等待租约核对。
		if !errors.Is(err, agentexec.ErrDirectJavaProcessUncertain) {
			_ = w.appendStartRejected(ctx, grant)
		} else {
			cleanupReady = false
			processStopped = false
		}
		return Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}, true, ErrExecutionRejected
	}
	cleanupReady = false
	processStopped = false
	stopRenewal := w.maintainExecutionLease(ctx, grant)
	defer stopRenewal()
	// 进程已经启动后，必须先等待其退出再清理任务级安全文件；即使事件网络上报失败也不能提前删除仍被 Java 使用的文件。
	startedResult := make(chan error, 1)
	go func() {
		startedResult <- w.appendEvent(ctx, grant, 3, "PROCESS_STARTED", map[string]any{"pid": process.Identity().PID, "startedAt": process.Identity().StartedAt.Format(time.RFC3339Nano), "executableDigest": process.Identity().ExecutableDigest, "bootId": process.Identity().BootID, "argvDigest": actualArgvDigest})
	}()
	exited, exitCode, waitErr, cancelRequested, treeObserved := w.waitWithExecutionControl(ctx, grant, process)
	processStopped = exited && (!cancelRequested || treeObserved)
	// 已排空管道并确认停止后先清秘密，再做可能阻塞的退出/结果网络上报。
	if processStopped {
		if err := workspace.CleanupMaterials(); err != nil {
			materialCleanupErr = agentexec.ErrCleanupFailed
		}
	}
	// 进程监管和秘密清理不等待启动事件网络响应，但退出事件仍必须排在启动确认之后。
	startedErr := <-startedResult
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
	if isImport {
		outcome, found, runErr = w.finishNormalImport(ctx, grant, nextSequence, exitCode, waitErr, cancelRequested, treeObserved, terminal, logs)
		cleanupReady = runErr == nil || errors.Is(runErr, ErrExecutionFailed)
		return outcome, found, runErr
	}
	if isDirect {
		outcome, found, runErr = w.finishDirectLoad(ctx, grant, nextSequence, exitCode, waitErr, cancelRequested, directTerminal, logs)
		cleanupReady = runErr == nil || errors.Is(runErr, ErrExecutionFailed)
		return outcome, found, runErr
	}
	outcome, found, runErr = w.finishExport(ctx, grant, nextSequence, exitCode, waitErr, cancelRequested, treeObserved, executionPaths, localOutputPath, exportResult, logs)
	cleanupReady = runErr == nil || errors.Is(runErr, ErrExecutionFailed)
	return outcome, found, runErr
}

func (w *Worker) appendStartRejected(ctx context.Context, grant agentwire.ExecutionGrant) error {
	return w.appendEvent(ctx, grant, 3, "START_REJECTED", map[string]any{"noProcess": true, "errorCode": agentwire.ExecutionErrorStartRejected})
}

func (w *Worker) appendEvent(ctx context.Context, grant agentwire.ExecutionGrant, sequence int64, eventType string, evidence map[string]any) error {
	eventID, err := identifier.NewUUIDV4()
	if err != nil {
		return ErrExecutionRejected
	}
	w.mu.Lock()
	if w.active != nil && w.active.ExecutionID == grant.ExecutionID {
		w.active.LastEventSeq = max(w.active.LastEventSeq, sequence)
	}
	w.mu.Unlock()
	return w.Protocol.AppendExecutionEvent(ctx, agentwire.ExecutionEvent{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, EventID: eventID, EventSeq: sequence, EventType: eventType, Evidence: evidence, SentAt: w.now()})
}

// maintainExecutionLease 把同一租约的既有续期覆盖到进程、日志收尾和最后状态事件。
// 失败沿用下一周期重试；调用方取消后仍先回收已启动进程，Worker 返回时停止并收回续租协程。
func (w *Worker) maintainExecutionLease(ctx context.Context, grant agentwire.ExecutionGrant) func() {
	renewCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	ticks := w.leaseRenewalTicks
	var ticker *time.Ticker
	if ticks == nil {
		ticker = time.NewTicker(30 * time.Second)
		ticks = ticker.C
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if ticker != nil {
			defer ticker.Stop()
		}
		for {
			select {
			case <-renewCtx.Done():
				return
			case _, ok := <-ticks:
				if !ok || renewCtx.Err() != nil {
					return
				}
				_ = w.Protocol.RenewExecutionLease(renewCtx, agentwire.ExecutionLeaseRenewal{BootID: w.BootID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, EnvelopeDigest: grant.EnvelopeDigest, SentAt: w.now()})
			}
		}
	}()
	return func() { cancel(); <-done }
}

// waitWithExecutionControl 将租约续期移出进程等待，只保留退出与取消轮询。
// 无论网络状态如何都先回收已启动进程，防止清理安全材料时破坏运行中的 Java。
func (w *Worker) waitWithExecutionControl(ctx context.Context, grant agentwire.ExecutionGrant, process *agentexec.DirectJavaProcess) (bool, int, error, bool, bool) {
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
	pollTicker := time.NewTicker(2 * time.Second)
	defer pollTicker.Stop()
	cancelRequested := false
	treeObserved := false
	for {
		select {
		case result := <-completed:
			return result.exited, result.code, result.err, cancelRequested, treeObserved
		case <-pollTicker.C:
			// 旁路服务端提交阶段的取消语义尚未核验，不能借共享轮询强杀并承诺回滚。
			if grant.TaskType == directload.TaskType {
				continue
			}
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
	if w.TaskType != "" && w.TaskType != "OBDUMPER_EXPORT" && w.TaskType != importdomain.TaskType && w.TaskType != directload.TaskType {
		return false
	}
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
func (w *Worker) leave() { w.mu.Lock(); defer w.mu.Unlock(); w.running = false; w.active = nil }

// ActiveExecution 返回当前 Worker 的无秘密副本；完成、拒绝和异常返回都会清除观测。
// 它不改变租约或状态机，也不能证明控制面已接受本地事件。
func (w *Worker) ActiveExecution() (nodefacts.ActiveExecution, bool) {
	if w == nil {
		return nodefacts.ActiveExecution{}, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active == nil {
		return nodefacts.ActiveExecution{}, false
	}
	return *w.active, true
}

func validGrant(grant agentwire.ExecutionGrant) bool {
	// v11 有 79 个固定字段，按每个字段最多两个令牌保留受限容量。
	if grant.TaskID == "" || grant.ExecutionID == "" || grant.LeaseID == "" || grant.LeaseEpoch < 1 || len(grant.EnvelopeDigest) != 64 || len(grant.Argv) == 0 || len(grant.Argv) > 160 {
		return false
	}
	if grant.TaskType == importdomain.TaskType {
		return grant.DirectLoad == nil && grant.NormalImport != nil && grant.NormalImport.Manifest.Valid(grant.NormalImport.Config) && executionargv.ValidNormalImport(grant.Argv)
	}
	if grant.TaskType == directload.TaskType {
		if grant.NormalImport != nil || grant.DirectLoad == nil || len(grant.DirectLoad.DirectLoadConfigFingerprint) != 64 || strings.ToLower(grant.DirectLoad.DirectLoadConfigFingerprint) != grant.DirectLoad.DirectLoadConfigFingerprint || grant.DirectLoad.CompatibilityMode != "MYSQL" && grant.DirectLoad.CompatibilityMode != "ORACLE" {
			return false
		}
		if _, err := hex.DecodeString(grant.DirectLoad.DirectLoadConfigFingerprint); err != nil {
			return false
		}
		return directloadfiles.Valid(grant.DirectLoad.Manifest, grant.DirectLoad.Config) && executionargv.MatchesDirectLoad(grant.Argv, grant.DirectLoad.Config)
	}
	if grant.DirectLoad != nil || grant.NormalImport != nil || grant.TaskType != "" && grant.TaskType != "OBDUMPER_EXPORT" {
		return false
	}
	_, ok := executionPathArguments(grant.Argv)
	return ok
}

// validExecutionSysSlot 复核冻结命令与短时 sys 槽位成对，防止额外材料或缺失材料进入进程。
func validExecutionSysSlot(argv []string, slot agentwire.DatabaseConnectionSlot) bool {
	required, userCount := false, 0
	for _, token := range argv {
		if token == "--add-extra-message" {
			if required {
				return false
			}
			required = true
		}
		if token == "--sys-user" {
			userCount++
		}
	}
	if !required {
		return userCount == 0 && len(slot.SysUsername) == 0 && len(slot.SysPassword) == 0
	}
	if userCount != 1 || len(slot.SysUsername) == 0 || len(slot.SysPassword) == 0 {
		return false
	}
	username := slot.SysUsername
	if boundary := bytes.IndexByte(username, '@'); boundary >= 0 {
		username = username[:boundary]
	}
	return bytes.Equal([]byte(executionSysUserArgument(argv)), username)
}

func executionSysUserArgument(argv []string) string {
	for index, token := range argv {
		if token == "--sys-user" && index+1 < len(argv) {
			return argv[index+1]
		}
	}
	return ""
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
		if (paths.LogPath != "" && !outputpath.IsExportOutputPath(platform, paths.LogPath)) || (paths.TmpPath != "" && !outputpath.IsExportOutputPath(platform, paths.TmpPath)) {
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
	if !executionargv.Valid(argv) {
		return executionPaths{}, false
	}
	paths := executionPaths{}
	seen := map[string]bool{}
	for index := 0; index < len(argv); index++ {
		value := argv[index]
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
// 业务用户名沿用连接诊断语义；额外 sys 身份与密码都必须脱敏。
func executionLogPolicy(password []byte, storageCredential *agentwire.StorageCredentialSlot, additionalSecrets ...[]byte) (logstream.Policy, error) {
	secrets := [][]byte{password}
	for _, secret := range additionalSecrets {
		if len(secret) != 0 {
			secrets = append(secrets, secret)
		}
	}
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
	return scanOutputFacts(path, nil)
}

// scanOutputFacts 在有界清单之外继续检查每一个文件，结果资格不受前 100 项截断影响。
func scanOutputFacts(path string, observe func(outputFileFact)) (uint64, uint64, []outputFileFact, bool) {
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
		relative, err := filepath.Rel(path, current)
		if err != nil || relative == "" || len(relative) > maxOutputFilePath || strings.ContainsAny(relative, "\x00\r\n") {
			return errors.New("output file path is invalid")
		}
		fact := outputFileFact{Path: filepath.ToSlash(relative), Size: size}
		if observe != nil {
			observe(fact)
		}
		if len(files) < maxOutputFileFacts {
			files = append(files, fact)
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
