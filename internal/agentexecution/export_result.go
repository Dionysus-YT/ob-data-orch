package agentexecution

import (
	"context"
	"regexp"
	"strings"
	"sync"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/logstream"
)

var exportExitPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] System exit 0\s*$`)

// 导出不能从进程退出码推造工具终态；只接受固定版本、可靠 stdout 的完整成功行。
type exportTerminal struct {
	mu             sync.Mutex
	exited, failed bool
}

func (t *exportTerminal) observe(record logstream.Record) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if record.Kind != logstream.RecordLog || record.IntegrityCode != "" && record.IntegrityCode != "COMPLETE" ||
		record.ParserVersion != "direct-java-v1" || record.SourceKind != logstream.SourceStdout && record.SourceKind != logstream.SourceStderr {
		t.failed = true
		return
	}
	matched := exportExitPattern.MatchString(record.Message) && record.SourceKind == logstream.SourceStdout && record.SourceEpoch == 1 && record.SourceSeq > 0
	if matched {
		t.exited = true
	}
	lower := strings.ToLower(record.Message)
	if strings.Contains(lower, "[error]") || strings.Contains(lower, "[fatal]") || strings.Contains(lower, "system exit ") && !matched {
		t.failed = true
	}
}

func (t *exportTerminal) verified() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exited && !t.failed
}

// exportFileRequirements 把冻结参数转换为最低结果资格，不把日志、清单或检查点算作数据。
type exportFileRequirements struct {
	ddl                    bool
	extension, compression string
	emptyData              bool
	hasDDL, hasData        bool
}

func exportRequirements(argv []string) exportFileRequirements {
	r := exportFileRequirements{}
	compressed, codec := false, "zstd"
	for index, value := range argv {
		switch value {
		case "--ddl":
			r.ddl = true
		case "--csv":
			r.extension, r.emptyData = ".csv", true
		case "--cut":
			r.extension, r.emptyData = ".dat", true
		case "--sql":
			r.extension, r.emptyData = ".sql", true
		case "--pos":
			r.extension = ".dat"
		case "--par":
			r.extension = ".parquet"
		case "--orc":
			r.extension = ".orc"
		case "--avro":
			r.extension = ".avro"
		case "--compress":
			compressed = true
		case "--compression-algo":
			if index+1 < len(argv) {
				codec = argv[index+1]
			}
		}
	}
	if compressed {
		r.compression = map[string]string{"zstd": ".zst", "zlib": ".zz", "gzip": ".gz", "snappy": ".sz"}[codec]
		if r.compression == "" {
			r.extension = ""
		}
		r.emptyData = false
	}
	return r
}

func (r *exportFileRequirements) observe(file outputFileFact) {
	name := strings.ToLower(file.Path)
	if strings.HasSuffix(name, "-schema.sql") {
		r.hasDDL = r.hasDDL || file.Size > 0
		return
	}
	if r.extension != "" && strings.HasSuffix(name, r.extension+r.compression) && (file.Size > 0 || r.emptyData) {
		r.hasData = true
	}
}

func (r exportFileRequirements) verified() bool {
	return (r.ddl || r.extension != "") && (!r.ddl || r.hasDDL) && (r.extension == "" || r.hasData)
}

// localOutputVerified 仅接受与所选内容和格式匹配的真实结果文件。
// CSV/CUT/SQL 数据允许合法空集合；结构文件、压缩文件或结构化格式仍要求非零字节。
func localOutputVerified(argv []string, count, bytes uint64, files []outputFileFact) bool {
	r := exportRequirements(argv)
	for _, file := range files {
		r.observe(file)
	}
	return count > 0 && r.verified()
}

func (w *Worker) finishExport(ctx context.Context, grant agentwire.ExecutionGrant, sequence int64, exitCode int, waitErr error, cancelled, treeObserved bool, paths executionPaths, localPath string, terminal *exportTerminal, logs *logUploader) (Outcome, bool, error) {
	outcome := Outcome{TaskID: grant.TaskID, ExecutionID: grant.ExecutionID}
	_ = logs.finishBestEffort(ctx)
	complete := logs.collected()
	toolOK := !cancelled && waitErr == nil && exitCode == 0 && complete && terminal.verified()
	state, code := "FAILED", agentwire.ExecutionErrorToolFailed
	if toolOK {
		state = "SUCCEEDED"
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
	terminalEvidence := map[string]any{"terminal": state}
	if !toolOK {
		terminalEvidence["errorCode"] = code
	}
	if err := w.appendEvent(ctx, grant, sequence, "TOOL_TERMINAL_OBSERVED", terminalEvidence); err != nil {
		return outcome, true, err
	}
	var count, bytes uint64
	var files []outputFileFact
	outputOK := true
	requirements := exportRequirements(grant.Argv)
	if !paths.StorageOutput {
		count, bytes, files, outputOK = scanOutputFacts(localPath, requirements.observe)
	}
	verified := toolOK && outputOK && (paths.StorageOutput || requirements.verified())
	result := "FAILED"
	if verified {
		result = "VERIFIED"
	} else if toolOK {
		code = agentwire.ExecutionErrorResultVerification
	}
	fileEvidence := make([]map[string]any, 0, len(files))
	for _, file := range files {
		fileEvidence = append(fileEvidence, map[string]any{"path": file.Path, "size": file.Size})
	}
	outcome.Succeeded, outcome.FileCount, outcome.TotalBytes = verified, count, bytes
	outcome.CheckpointPresent = executionCheckpointPresent(paths, localPath)
	facts := map[string]any{"result": result, "fileCount": count, "totalBytes": bytes, "files": fileEvidence, "checkpointPresent": outcome.CheckpointPresent}
	if !verified {
		facts["errorCode"] = code
	}
	if err := w.appendEvent(ctx, grant, sequence+1, "RESULT_FACTS_OBSERVED", facts); err != nil {
		return outcome, true, err
	}
	if !verified && !(cancelled && treeObserved) {
		return outcome, true, ErrExecutionFailed
	}
	return outcome, true, nil
}
