package agentexecution

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/inputpath"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/outputpath"
)

// normalImportPaths 复核原样输入与冻结清单；导入目录绝不按导出结果目录枚举或转写。
func (w *Worker) normalImportPaths(ctx context.Context, grant agentwire.ExecutionGrant) (executionPaths, bool) {
	input := grant.NormalImport
	if input == nil || !input.Manifest.Valid(input.Config) || !normalImportArgumentsMatch(grant.Argv, input.Config) || !inputpath.Valid(w.Runtime.TargetPlatform, input.Config.FilePath) {
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
	checkCtx, cancel := context.WithDeadline(ctx, grant.ExpiresAt)
	defer cancel()
	manifest, err := importfiles.Scan(checkCtx, input.Config, roots)
	if err != nil || manifest.Digest != input.Manifest.Digest {
		return executionPaths{}, false
	}
	if input.Config.LogPath != "" && !normalImportLogPathAllowed(w.Runtime.TargetPlatform, input.Config.LogPath, roots) {
		return executionPaths{}, false
	}
	return executionPaths{OutputPath: input.Config.FilePath, LogPath: input.Config.LogPath}, true
}

func normalImportArgumentsMatch(argv []string, config importdomain.Config) bool {
	values := make(map[string]string)
	flags := map[string]bool{}
	for _, field := range importdomain.Fields(config) {
		if field.Value.Kind == commandgen.ValueBoolean {
			flags[field.Name] = true
		}
	}
	flags["--no-sys"] = true
	if config.Cleanup == "TRUNCATE" || config.Cleanup == "DELETE" {
		flags["--yes"] = true
	}
	for i := 0; i < len(argv); i++ {
		name := argv[i]
		if _, exists := values[name]; exists {
			return false
		}
		if flags[name] {
			values[name] = "true"
			continue
		}
		if i+1 >= len(argv) {
			return false
		}
		i++
		values[name] = argv[i]
	}
	if values["--database"] != config.Database || values["--no-sys"] != "true" {
		return false
	}
	for _, name := range []string{"--host", "--port", "--user", "--database", "--no-sys"} {
		if values[name] == "" {
			return false
		}
		delete(values, name)
	}
	if config.Cleanup == "TRUNCATE" || config.Cleanup == "DELETE" {
		if values["--yes"] != "true" {
			return false
		}
		delete(values, "--yes")
	}
	for _, field := range importdomain.Fields(config) {
		expected := field.Value.String
		if field.Value.Kind == commandgen.ValueBoolean {
			expected = "true"
		}
		if field.Value.Kind == commandgen.ValueInteger {
			expected = strconv.FormatInt(field.Value.Integer, 10)
		}
		if values[field.Name] != expected {
			return false
		}
		delete(values, field.Name)
	}
	return len(values) == 0
}

func normalImportLogPathAllowed(platform, path string, roots []string) bool {
	if !inputpath.Valid(platform, path) {
		return false
	}
	for _, root := range roots {
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			continue
		}
		current := root
		for _, part := range strings.Split(relative, string(filepath.Separator)) {
			info, err := os.Lstat(current)
			if err != nil && !os.IsNotExist(err) || err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
				return false
			}
			current = filepath.Join(current, part)
		}
		info, err := os.Lstat(path)
		return os.IsNotExist(err) || err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
	}
	return false
}

// 完成标记来自本机固定 4.3.5 Obloader/AbstractCommandParser 日志模板。
// 结束行自身不证明成功：还需退出码、所选全部阶段、无失败/缺口及上传完成。
var normalImportFinishedPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] Load (schema|record) finished\. Total Elapsed: [^\r\n]+$`)
var normalImportExitPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] System exit 0\s*$`)

type normalImportTerminal struct {
	mu                           sync.Mutex
	schema, data, exited, failed bool
}

func (t *normalImportTerminal) observe(record logstream.Record) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if record.Kind != logstream.RecordLog {
		t.failed = true
		return
	}
	message := record.Message
	if parts := normalImportFinishedPattern.FindStringSubmatch(message); len(parts) == 2 {
		if parts[1] == "schema" {
			t.schema = true
		} else {
			t.data = true
		}
	}
	if normalImportExitPattern.MatchString(message) {
		t.exited = true
	}
	// 工具容错阈值可能使含失败的任务退出 0；本切片保守拒绝该成功结论。
	lower := strings.ToLower(message)
	if strings.Contains(lower, "load failed") || strings.Contains(lower, "load schema failed") || strings.Contains(lower, "exec failure") || strings.Contains(lower, "failed to load") || strings.Contains(lower, "system exit ") && !normalImportExitPattern.MatchString(message) {
		t.failed = true
	}
}

func (t *normalImportTerminal) verified(config importdomain.Config) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.failed && t.exited && (!config.HasDDL() || t.schema) && (!config.HasData() || t.data)
}

func (w *Worker) finishNormalImport(ctx context.Context, grant agentwire.ExecutionGrant, sequence int64, exitCode int, waitErr error, cancelled, treeObserved bool, terminal *normalImportTerminal, logs *logUploader) (Outcome, bool, error) {
	outcome := Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}
	complete := !logs.gapped
	if logs.queue != nil && flushQueuedLogs(ctx, logs.queue, w.Protocol, w.BootID, "") != nil {
		complete = false
	}
	verified := !cancelled && waitErr == nil && exitCode == 0 && complete && terminal.verified(grant.NormalImport.Config)
	state, result, code := "FAILED", "FAILED", agentwire.ExecutionErrorToolFailed
	if verified {
		state, result = "SUCCEEDED", "VERIFIED"
	}
	if cancelled {
		code = agentwire.ExecutionErrorCancelledByRequest
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
	// 输入数量和字节数不进入导入结果；load.ckpt 继续能力未接入，不能借用 dump.ckpt。
	facts := map[string]any{"result": result, "normalImport": true, "checkpointPresent": false}
	if !verified {
		facts["errorCode"] = code
	}
	if err := w.appendEvent(ctx, grant, sequence+1, "RESULT_FACTS_OBSERVED", facts); err != nil {
		return outcome, true, err
	}
	outcome.Succeeded = verified
	if !verified && !(cancelled && treeObserved) {
		return outcome, true, ErrExecutionFailed
	}
	return outcome, true, nil
}
