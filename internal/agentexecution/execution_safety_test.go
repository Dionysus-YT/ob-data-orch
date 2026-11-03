package agentexecution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/outputpath"
)

// 合成进程仅运行本测试二进制，不运行 Java、数据库或官方工具。
func init() {
	mode := os.Getenv("OBDO_TEST_EXECUTION_SAFETY")
	if mode == "" {
		return
	}
	for _, arg := range os.Args[1:] {
		if arg == "-version" {
			fmt.Fprintln(os.Stderr, `java version "1.8.0_491"`)
			os.Exit(0)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if mode == "normal-success" {
		fmt.Println("2026-10-28 00:00:00 [INFO] Load record finished. Total Elapsed: 1 s")
		fmt.Println("2026-10-28 00:00:00 [INFO] System exit 0")
		fmt.Fprintln(os.Stderr, "2026-10-28 00:00:00 [WARN] synthetic diagnostic one")
		fmt.Fprintln(os.Stderr, "2026-10-28 00:00:00 [WARN] synthetic diagnostic two")
		os.Exit(0)
	}
	var path string
	for index, arg := range os.Args[1:] {
		if arg == "--file-path" && index+2 < len(os.Args) {
			path = os.Args[index+2]
		}
	}
	if !strings.HasPrefix(path, "s3://") {
		if len(path) > 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		name, data := "synthetic.csv", []byte("synthetic row")
		if strings.HasPrefix(mode, "checkpoint") {
			name = "dump.ckpt"
		}
		if mode == "empty-success" {
			data = nil
		}
		if err := os.WriteFile(filepath.Join(filepath.FromSlash(path), name), data, 0600); err != nil {
			os.Exit(71)
		}
	}
	if mode == "data-error-success" {
		fmt.Println("2026-10-28 00:00:00 [ERROR] synthetic export failure")
	}
	if strings.HasSuffix(mode, "-success") {
		fmt.Println("2026-10-28 00:00:00 [INFO] System exit 0")
	}
	os.Exit(0)
}

func testNativePlatform(path string) string {
	if filepath.VolumeName(path) != "" {
		return outputpath.PlatformWindowsAMD64
	}
	return outputpath.PlatformLinuxAMD64
}

func testExportPath(path string) string {
	value := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		return "/" + value
	}
	return value
}

type safetyExecutionProtocol struct {
	executionWorkerTestProtocol
	now                      *time.Time
	expire, storage, offline bool
	residualPath             string
	storageMaterialPath      string
}

func (p *safetyExecutionProtocol) AppendExecutionEvent(ctx context.Context, event agentwire.ExecutionEvent) error {
	if event.EventType == "PROCESS_EXITED" && p.storageMaterialPath != "" {
		if _, err := os.Stat(p.storageMaterialPath); !errors.Is(err, os.ErrNotExist) {
			return errors.New("退出网络上报前存储秘密未删除")
		}
	}
	if event.EventType == "RESULT_FACTS_OBSERVED" && p.residualPath != "" {
		if err := os.WriteFile(p.residualPath, []byte("synthetic recovery residual"), 0600); err != nil {
			return err
		}
	}
	return p.executionWorkerTestProtocol.AppendExecutionEvent(ctx, event)
}

func (p *safetyExecutionProtocol) ResolveExecutionDatabaseConnection(context.Context, agentwire.ExecutionSecretSlotRequest) (agentwire.DatabaseConnectionSlot, error) {
	p.secretResolutions++
	if p.expire {
		*p.now = p.grant.ExpiresAt
	}
	slot := agentwire.DatabaseConnectionSlot{Password: []byte("synthetic-execution-password")}
	if p.storage {
		slot.StorageCredential = &agentwire.StorageCredentialSlot{Provider: "S3", AccessKey: []byte("synthetic-access"), SecretKey: []byte("synthetic-secret")}
	}
	return slot, nil
}

func (p *safetyExecutionProtocol) AppendExecutionLog(ctx context.Context, _ agentwire.ExecutionLogBatch) error {
	if p.offline {
		return errors.New("合成日志网络不可达")
	}
	return ctx.Err()
}

func safetyWorker(t *testing.T, mode string) (*Worker, *safetyExecutionProtocol, string) {
	t.Helper()
	now := time.Now().UTC()
	root, output, home := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "lib"), 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := &safetyExecutionProtocol{now: &now}
	p.grant = agentwire.ExecutionGrant{TaskID: "synthetic-task", ExecutionID: "synthetic-execution", LeaseID: "synthetic-lease", LeaseEpoch: 1, ExpiresAt: now.Add(time.Minute), EnvelopeDigest: strings.Repeat("a", 64), Argv: []string{"--file-path", testExportPath(output), "--csv"}}
	w := &Worker{Protocol: p, BootID: "synthetic-boot", Clock: func() time.Time { return now }, Runtime: Runtime{TargetPlatform: testNativePlatform(output), JavaPath: executable, ToolHome: home, WorkspaceRoot: root, Environment: []string{"SystemRoot=" + os.Getenv("SystemRoot"), "OBDO_TEST_EXECUTION_SAFETY=" + mode}, AllowedRoots: []string{output}}}
	return w, p, output
}

func Test导出执行必须同时具有固定成功终态和真实格式文件(t *testing.T) {
	for _, mode := range []string{"checkpoint", "checkpoint-success", "data", "data-error-success", "data-success", "empty-success"} {
		t.Run(mode, func(t *testing.T) {
			w, p, _ := safetyWorker(t, mode)
			out, found, err := w.RunNext(context.Background())
			want := mode == "data-success" || mode == "empty-success"
			if !found || out.Succeeded != want || (err == nil) != want {
				t.Fatalf("成功证据错误: result=%+v err=%v", out, err)
			}
			started := false
			for _, event := range p.events {
				if event.EventType == "PROCESS_STARTED" {
					started = true
				}
				if event.EventType == "RESULT_FACTS_OBSERVED" && (event.Evidence["result"] == "VERIFIED") != want {
					t.Fatal("结果事件误报成功")
				}
			}
			if !started {
				t.Fatal("负例没有执行合成工具，未覆盖结果验证")
			}
			if err := credential.CheckWorkspaceRoot(w.Runtime.WorkspaceRoot); err != nil {
				t.Fatal("已停止执行的材料未清理", err)
			}
		})
	}
}

func Test导出秘密准备期间租约到期不启动工具(t *testing.T) {
	w, p, path := safetyWorker(t, "data-success")
	p.expire = true
	out, found, err := w.RunNext(context.Background())
	if !found || out.Succeeded || !errors.Is(err, ErrExecutionRejected) {
		t.Fatal("过期租约未被拒绝", out, err)
	}
	if p.secretResolutions != 1 {
		t.Fatal("未覆盖秘密解析期间过期")
	}
	for _, event := range p.events {
		if event.EventType == "PROCESS_STARTED" {
			t.Fatal("已过期仍启动工具")
		}
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatal("拒绝后仍产生工具结果", err)
	}
}

func Test对象存储清理和断网尾部独立保留(t *testing.T) {
	w, p, tmp := safetyWorker(t, "storage-success")
	p.storage, p.offline = true, true
	p.storageMaterialPath = filepath.Join(w.Runtime.WorkspaceRoot, "executions", p.grant.ExecutionID, "security", "hadoop-conf", "core-site.xml")
	p.grant.Argv = []string{"--file-path", "s3://synthetic-bucket/exports", "--tmp-path", testExportPath(tmp), "--csv"}
	out, found, err := w.RunNext(context.Background())
	if err != nil || !found || !out.Succeeded || out.FileCount != 0 || out.TotalBytes != 0 {
		t.Fatal("固定成功和受控进程未保留远端有限证据口径", out, err)
	}
	if _, err := os.Stat(filepath.Join(w.Runtime.WorkspaceRoot, "executions", p.grant.ExecutionID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("仅日志断网仍保留了秘密工作区", err)
	}
	if err := credential.CheckWorkspaceRoot(w.Runtime.WorkspaceRoot); err != nil {
		t.Fatal("仅日志断网不应隔离节点", err)
	}
	queue, err := agentlogqueue.Open(filepath.Join(w.Runtime.WorkspaceRoot, "log-queue"), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	items, err := queue.PendingItems()
	if err != nil || len(items) == 0 {
		t.Fatal("未确认脱敏尾部被丢弃", err)
	}
	p.offline = false
	if err := flushQueuedLogs(context.Background(), queue, p, "synthetic-restart", ""); err != nil {
		t.Fatal("尾部不能重放", err)
	}
}

func Test残留阻断所有任务类型领取前且不解析秘密(t *testing.T) {
	for _, taskType := range []string{"", importdomain.TaskType, directload.TaskType} {
		w, p, _ := safetyWorker(t, "data-success")
		w.TaskType = taskType
		old, err := credential.CreateWorkspace(w.Runtime.WorkspaceRoot, "synthetic-old")
		if err != nil {
			t.Fatal(err)
		}
		old.Isolate()
		_, found, err := w.RunNext(context.Background())
		if found || !errors.Is(err, credential.ErrWorkspaceResidual) || p.acknowledgements != 0 {
			t.Fatal("残留未在领取前阻断", err)
		}
	}
}

func TestWorker不吞掉清理失败且下次领取被阻断(t *testing.T) {
	w, p, _ := safetyWorker(t, "data-success")
	p.residualPath = filepath.Join(w.Runtime.WorkspaceRoot, "executions", p.grant.ExecutionID, "evidence", "synthetic-residual")
	out, found, err := w.RunNext(context.Background())
	if !found || err == nil || out.Succeeded {
		t.Fatal("清理失败被静默忽略", out, err)
	}
	if _, found, err := w.RunNext(context.Background()); found || !errors.Is(err, credential.ErrWorkspaceResidual) {
		t.Fatal("残留没有阻断后续领取", err)
	}
}

func Test导出终态拒绝错误来源版本缺口和伪造前缀(t *testing.T) {
	base := logstream.Record{Kind: logstream.RecordLog, SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 1, ParserVersion: "direct-java-v1", IntegrityCode: "COMPLETE", Message: "2026-10-28 00:00:00 [INFO] System exit 0"}
	for _, invalid := range []string{"source", "version", "gap", "prefix", "nonzero", "integrity"} {
		r := base
		switch invalid {
		case "source":
			r.SourceKind = logstream.SourceStderr
		case "version":
			r.ParserVersion = "unknown"
		case "gap":
			r.Kind = logstream.RecordGap
		case "prefix":
			r.Message = "user data " + r.Message
		case "nonzero":
			r.Message = strings.ReplaceAll(r.Message, "exit 0", "exit 1")
		case "integrity":
			r.IntegrityCode = "TRUNCATED"
		}
		terminal := &exportTerminal{}
		terminal.observe(r)
		if terminal.verified() {
			t.Fatal("未知终态被接受", invalid)
		}
	}
}
