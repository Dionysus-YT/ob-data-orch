package agentexecution

import (
	"context"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"

	"ob-data-orch/internal/agentenvironment"
	"ob-data-orch/internal/agentexec"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/outputpath"
)

const directLoadParserVersion = "obloader-direct-4.3.5-v1"

func executionTaskType(value string) string {
	if value == "" {
		return "OBDUMPER_EXPORT"
	}
	return value
}

// directLoadPaths 只复核旁路原样输入和单表清单，不创建导出结果目录或探测检查点。
func (w *Worker) directLoadPaths(ctx context.Context, grant agentwire.ExecutionGrant) (executionPaths, bool) {
	input := grant.DirectLoad
	if ctx == nil || input == nil || grant.NormalImport != nil || grant.TaskType != directload.TaskType || !directloadfiles.Valid(input.Manifest, input.Config) || !executionargv.MatchesDirectLoad(grant.Argv, input.Config) {
		return executionPaths{}, false
	}
	active, issues := directload.Normalize(input.Config, input.CompatibilityMode, w.Runtime.TargetPlatform, true)
	if len(issues) != 0 || active != input.Config {
		return executionPaths{}, false
	}
	if !directLoadRuntimeConfigurationMatches(w.Runtime.ToolHome, input.DirectLoadConfigFingerprint) {
		return executionPaths{}, false
	}
	if _, err := agentexec.DirectLoadToolVersion(ctx, w.Runtime.ToolHome); err != nil {
		return executionPaths{}, false
	}
	roots := make([]string, 0, len(w.Runtime.AllowedRoots))
	for _, root := range w.Runtime.AllowedRoots {
		native, ok := outputpath.LocalFilesystemPath(w.Runtime.TargetPlatform, root)
		if !ok {
			return executionPaths{}, false
		}
		roots = append(roots, native)
	}
	bounded, cancel := context.WithDeadline(ctx, grant.ExpiresAt)
	defer cancel()
	manifest, err := directloadfiles.Scan(bounded, input.Config, roots)
	if err != nil || manifest.Digest != input.Manifest.Digest {
		return executionPaths{}, false
	}
	return executionPaths{OutputPath: input.Config.FilePath}, true
}

// directLoadRuntimeConfigurationMatches 只比较已冻结的三个安全数值投影，不读取或公开整文件摘要。
func directLoadRuntimeConfigurationMatches(toolHome, expected string) bool {
	if len(expected) != 64 || strings.ToLower(expected) != expected {
		return false
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return false
	}
	c := agentenvironment.ReadDirectLoadConfiguration(toolHome)
	if c == nil || c.Status != "READ" || c.HasUnrecognizedKeys || c.Fingerprint != expected {
		return false
	}
	for _, value := range []*int64{c.TaskTimeoutMS, c.HeartbeatTimeoutMS, c.HeartbeatIntervalMS} {
		if value == nil || *value <= 0 || *value > 1<<53-1 {
			return false
		}
	}
	return true
}

// 固定 4.3.5 DirectPathConnection.commit 仅在 commitAsync 的 await 与 isSuccess 均成功后输出提交行。
// 该静态模板不代表 DLV 真实验收已通过；只能在独立准入的受控进程和完整管道中形成任务事实。
var directLoadCommitPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] Load task on table "([^"\r\n]+)" is committed successfully! Elapsed: [^\r\n]+$`)
var directLoadExitPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] System exit 0\s*$`)
var directLoadWriteFailurePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[WARN\] Failed to load directly, tableName=`)

type directLoadTerminal struct {
	mu                        sync.Mutex
	table                     string
	committed, exited, failed bool
}

func (t *directLoadTerminal) observe(record logstream.Record) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// 真实采集器的完整 LOG 使用空原因码，历史合成夹具也可携带 COMPLETE；其他值均为缺口或未知。
	if record.Kind != logstream.RecordLog || record.IntegrityCode != "" && record.IntegrityCode != "COMPLETE" || record.ParserVersion != directLoadParserVersion || record.SourceKind != logstream.SourceLoaderStdout && record.SourceKind != logstream.SourceLoaderStderr {
		t.failed = true
		return
	}
	message := record.Message
	lower := strings.ToLower(message)
	// 提交跳过、记录写入失败和任何错误都不能被后续通用 finished/exit 0 覆盖。
	if strings.Contains(message, "[ERROR]") || strings.Contains(message, "[FATAL]") || directLoadWriteFailurePattern.MatchString(message) || strings.Contains(lower, "commit phase is skipped") || strings.Contains(lower, "system exit ") && !directLoadExitPattern.MatchString(message) {
		t.failed = true
	}
	if match := directLoadCommitPattern.FindStringSubmatch(message); len(match) == 2 {
		if record.SourceKind != logstream.SourceLoaderStdout || match[1] != t.table || t.exited {
			t.failed = true
			return
		}
		t.committed = true
	}
	if directLoadExitPattern.MatchString(message) {
		if record.SourceKind != logstream.SourceLoaderStdout || !t.committed {
			t.failed = true
		}
		t.exited = true
	}
}

func (t *directLoadTerminal) verified() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.table != "" && t.committed && t.exited && !t.failed
}

// commitObserved 保留同表提交行的实际观测；后续退出或证据失败不能被解释成服务端未提交。
func (t *directLoadTerminal) commitObserved() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.table != "" && t.committed
}

func (w *Worker) finishDirectLoad(ctx context.Context, grant agentwire.ExecutionGrant, sequence int64, exitCode int, waitErr error, cancelled bool, terminal *directLoadTerminal, logs *logUploader) (Outcome, bool, error) {
	outcome := Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}
	complete := !logs.gapped
	if logs.queue != nil && flushQueuedLogs(ctx, logs.queue, w.Protocol, w.BootID, "") != nil {
		complete = false
	}
	verified := !cancelled && waitErr == nil && exitCode == 0 && complete && terminal.verified()
	state, result, code := "FAILED", "FAILED", agentwire.ExecutionErrorResultVerification
	if verified {
		state, result = "SUCCEEDED", "VERIFIED"
	} else if waitErr != nil {
		code = agentwire.ExecutionErrorProcessWaitFailed
	} else if exitCode != 0 {
		code = agentwire.ExecutionErrorProcessExitNonZero
	} else if !complete {
		code = "EVIDENCE_UNAVAILABLE"
	}
	evidence := map[string]any{"terminal": state}
	if !verified {
		evidence["errorCode"] = code
	}
	if err := w.appendEvent(ctx, grant, sequence, "TOOL_TERMINAL_OBSERVED", evidence); err != nil {
		return outcome, true, err
	}
	// 输入文件不等于导入结果，旁路不产生可继续的检查点或导出文件清单。
	facts := map[string]any{"result": result, "directLoad": true, "serverCommitObserved": terminal.commitObserved(), "checkpointPresent": false}
	if !verified {
		facts["errorCode"] = code
	}
	if err := w.appendEvent(ctx, grant, sequence+1, "RESULT_FACTS_OBSERVED", facts); err != nil {
		return outcome, true, err
	}
	outcome.Succeeded = verified
	if !verified {
		return outcome, true, ErrExecutionFailed
	}
	return outcome, true, nil
}
