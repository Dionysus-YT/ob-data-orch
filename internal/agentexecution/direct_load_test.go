package agentexecution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentenvironment"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/outputpath"
)

func directLoadTerminalRecord(message string) logstream.Record {
	return logstream.Record{Kind: logstream.RecordLog, SourceKind: logstream.SourceLoaderStdout, IntegrityCode: "COMPLETE", ParserVersion: directLoadParserVersion, Message: "2026-10-19 12:00:00 [INFO] " + message}
}

func Test旁路终态接受真实采集器完整日志并继续拒绝缺口(t *testing.T) {
	for _, integrity := range []string{"", "COMPLETE", "UNKNOWN"} {
		terminal := &directLoadTerminal{table: "synthetic"}
		commit := directLoadTerminalRecord(`Load task on table "synthetic" is committed successfully! Elapsed: 1 s`)
		exit := directLoadTerminalRecord("System exit 0")
		commit.IntegrityCode, exit.IntegrityCode = integrity, integrity
		terminal.observe(commit)
		terminal.observe(exit)
		if terminal.verified() != (integrity != "UNKNOWN") {
			t.Fatal("真实日志完整性语义不符", integrity)
		}
	}
}

func Test旁路提交事实必须来自专用完整管道且严格匹配表(t *testing.T) {
	commit := directLoadTerminalRecord(`Load task on table "synthetic" is committed successfully! Elapsed: 1 s`)
	exit := directLoadTerminalRecord("System exit 0")
	for _, scenario := range []string{"valid", "normal-finished", "batch-written", "no-exit", "exit-first", "foreign-table", "stderr", "parser", "source", "integrity", "gap", "truncated", "error", "skip", "nonzero", "fake-prefix"} {
		t.Run(scenario, func(t *testing.T) {
			terminal := &directLoadTerminal{table: "synthetic"}
			first, last := commit, exit
			switch scenario {
			case "normal-finished":
				first = directLoadTerminalRecord("Load record finished. Total Elapsed: 1 s")
			case "batch-written":
				first = directLoadTerminalRecord("Insert 1 rows success")
			case "exit-first":
				first, last = exit, commit
			case "foreign-table":
				first.Message = strings.ReplaceAll(first.Message, `"synthetic"`, `"foreign"`)
			case "stderr":
				first.SourceKind = logstream.SourceLoaderStderr
			case "parser":
				first.ParserVersion = "obloader-4.3.5-v1"
			case "source":
				first.SourceKind = logstream.SourceStdout
			case "integrity":
				first.IntegrityCode = "UNKNOWN"
			case "fake-prefix":
				first.Message = "user data " + first.Message
			}
			terminal.observe(first)
			if scenario != "no-exit" {
				terminal.observe(last)
			}
			switch scenario {
			case "gap":
				terminal.observe(logstream.Record{Kind: logstream.RecordGap})
			case "truncated":
				terminal.observe(logstream.Record{Kind: logstream.RecordTruncated})
			case "error":
				terminal.observe(directLoadTerminalRecord("[ERROR] Failed to load directly"))
			case "skip":
				terminal.observe(directLoadTerminalRecord("Commit phase is skipped since no data has been written."))
			case "nonzero":
				terminal.observe(directLoadTerminalRecord("System exit 1"))
			}
			if terminal.verified() != (scenario == "valid") {
				t.Fatal("旁路终态证据判定错误")
			}
		})
	}
}

func Test旁路合法表名中的失败词不能覆盖固定提交事实(t *testing.T) {
	for _, table := range []string{"failed_orders", "failure_records"} {
		terminal := &directLoadTerminal{table: table}
		terminal.observe(directLoadTerminalRecord(`Load task on table "` + table + `" is committed successfully! Elapsed: 1 s`))
		terminal.observe(directLoadTerminalRecord("System exit 0"))
		if !terminal.verified() {
			t.Fatal("合法表名被裸词误判为失败")
		}
	}
	terminal := &directLoadTerminal{table: "synthetic"}
	terminal.observe(logstream.Record{Kind: logstream.RecordLog, SourceKind: logstream.SourceLoaderStdout, IntegrityCode: "COMPLETE", ParserVersion: directLoadParserVersion, Message: "2026-10-19 12:00:00 [WARN] Failed to load directly, tableName=synthetic"})
	terminal.observe(directLoadTerminalRecord(`Load task on table "synthetic" is committed successfully! Elapsed: 1 s`))
	terminal.observe(directLoadTerminalRecord("System exit 0"))
	if terminal.verified() {
		t.Fatal("固定旁路写入失败被后续成功行覆盖")
	}
}

func Test旁路结果拒绝仅退出成功且没有可恢复检查点(t *testing.T) {
	for _, scenario := range []string{"valid", "missing-commit", "exit", "wait", "gap", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			p := &executionWorkerTestProtocol{}
			w := &Worker{Protocol: p, Clock: time.Now, BootID: "synthetic-boot"}
			g := agentwire.ExecutionGrant{TaskID: "synthetic-task", TaskType: directload.TaskType, ExecutionID: "synthetic-execution", LeaseID: "synthetic-lease", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("a", 64)}
			terminal := &directLoadTerminal{table: "synthetic", committed: scenario != "missing-commit", exited: true}
			logs := newLogUploader(context.Background(), p, w.BootID, g, nil, nil)
			logs.gapped = scenario == "gap"
			exitCode := 0
			if scenario == "exit" {
				exitCode = 1
			}
			var waitErr error
			if scenario == "wait" {
				waitErr = errors.New("合成等待失败")
			}
			outcome, found, err := w.finishDirectLoad(context.Background(), g, 5, exitCode, waitErr, scenario == "cancel", terminal, logs)
			if !found || outcome.Succeeded != (scenario == "valid") || (err == nil) != (scenario == "valid") || outcome.CheckpointPresent || len(p.events) != 2 {
				t.Fatal("旁路结果未失败关闭")
			}
			facts := p.events[1].Evidence
			if facts["directLoad"] != true || facts["checkpointPresent"] != false || facts["serverCommitObserved"] != (scenario != "missing-commit") || p.events[0].EventSeq != 5 || p.events[1].EventSeq != 6 {
				t.Fatal("旁路事实类型或顺序错误")
			}
			want := 5
			if scenario == "valid" {
				want = 4
			}
			if len(facts) != want {
				t.Fatal("旁路不能包含输入文件计数或普通导入结果字段")
			}
		})
	}
}

func Test旁路启动前配置变化或缺失撤销预检查安全投影(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "conf"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "conf", "session.config.json")
	valid := `{"direct_path_load":{"task_timeout_ms":60000,"heartbeat_timeout_ms":6000,"heartbeat_interval_ms":3000}}`
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	fingerprint := agentenvironment.ReadDirectLoadConfiguration(root).Fingerprint
	if !directLoadRuntimeConfigurationMatches(root, fingerprint) {
		t.Fatal("相同安全投影被拒绝")
	}
	for _, body := range []string{strings.Replace(valid, "60000", "60001", 1), strings.Replace(valid, "60000", "0", 1), `{"direct_path_load":{}}`, strings.Replace(valid, `"task_timeout_ms":60000`, `"unknown":1,"task_timeout_ms":60000`, 1)} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if directLoadRuntimeConfigurationMatches(root, fingerprint) {
			t.Fatal("配置漂移或未知字段未阻断启动")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if directLoadRuntimeConfigurationMatches(root, fingerprint) || directLoadRuntimeConfigurationMatches(root, "") {
		t.Fatal("缺失配置或指纹未阻断启动")
	}
}

func Test旁路执行信封不被普通任务领取且未知工具在秘密前拒绝(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "synthetic.csv"), []byte("1,synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := directload.Config{DataSourceID: "source", NodeID: "node", Database: "synthetic_db", Scenario: "OBSERVER_DIRECT", RPCPort: 2882, Table: "synthetic", FilePath: root, Format: "CSV", Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	m, err := directloadfiles.Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	g := agentwire.ExecutionGrant{TaskID: "synthetic-task", TaskType: directload.TaskType, ExecutionID: "synthetic-execution", LeaseID: "synthetic-lease", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Minute), DirectLoad: &directloadfiles.ExecutionInput{CompatibilityMode: "MYSQL", Config: c, Manifest: m, DirectLoadConfigFingerprint: strings.Repeat("b", 64)}, Argv: []string{"--host", "127.0.0.1", "--port", "2881", "--user", "synthetic", "--tenant", "synthetic_tenant", "--database", c.Database, "--file-path", root, "--table", c.Table, "--csv", "--direct", "--rpc-port", "2882"}}
	if !validGrant(g) {
		t.Fatal("有效旁路信封被拒绝")
	}
	other := g
	other.NormalImport = &importfiles.ExecutionInput{}
	if validGrant(other) {
		t.Fatal("混合信封被接受")
	}
	platform := outputpath.PlatformLinuxAMD64
	if filepath.VolumeName(root) != "" {
		platform = outputpath.PlatformWindowsAMD64
	}
	for _, taskType := range []string{"", "OBDUMPER_EXPORT", importdomain.TaskType, directload.TaskType} {
		p := &executionWorkerTestProtocol{grant: g}
		w := &Worker{TaskType: taskType, Protocol: p, Clock: time.Now, BootID: "synthetic-boot", Runtime: Runtime{TargetPlatform: platform, JavaPath: filepath.Join(root, "java.exe"), ToolHome: root, WorkspaceRoot: filepath.Join(root, "workspace"), Environment: []string{"PATH=synthetic"}, AllowedRoots: []string{root}}}
		_, found, err := w.RunNext(context.Background())
		if !found || err == nil || p.secretResolutions != 0 {
			t.Fatal("类型或固定工具检查未在秘密前拒绝")
		}
		if taskType != directload.TaskType && p.acknowledgements != 0 {
			t.Fatal("普通 Worker 确认了旁路信封")
		}
	}
}
