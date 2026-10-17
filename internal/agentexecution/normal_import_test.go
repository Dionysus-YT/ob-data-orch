package agentexecution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/outputpath"
)

func normalImportTestInput(t *testing.T) (*importfiles.ExecutionInput, []string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "synthetic.csv"), []byte("1,synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := importdomain.Config{DataSourceID: "source", NodeID: "node", Database: "synthetic", FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE"}
	m, err := importfiles.Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic", "--database", c.Database, "--no-sys", "--file-path", root, "--all", "--csv"}
	return &importfiles.ExecutionInput{Config: c, Manifest: m}, argv
}

func Test普通导入冻结输入与参数逐项匹配(t *testing.T) {
	input, argv := normalImportTestInput(t)
	if !normalImportArgumentsMatch(argv, input.Config) {
		t.Fatal("有效冻结参数被拒绝")
	}
	for _, extra := range [][]string{{"--direct"}, {"--query-sql", "SELECT 1"}, {"--database", "other"}, {"--yes"}, {"--replace-data"}} {
		if normalImportArgumentsMatch(append(append([]string(nil), argv...), extra...), input.Config) {
			t.Fatal("越界参数被接受")
		}
	}
}

func Test普通导入启动前复核输入变化(t *testing.T) {
	input, argv := normalImportTestInput(t)
	worker := &Worker{Runtime: Runtime{TargetPlatform: outputpath.PlatformWindowsAMD64, AllowedRoots: []string{input.Config.FilePath}}}
	grant := agentwire.ExecutionGrant{NormalImport: input, Argv: argv, ExpiresAt: time.Now().Add(time.Minute)}
	if _, ok := worker.normalImportPaths(context.Background(), grant); !ok {
		t.Fatal("原生输入被拒绝")
	}
	if err := os.WriteFile(filepath.Join(input.Config.FilePath, "synthetic.csv"), []byte("2,changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := worker.normalImportPaths(context.Background(), grant); ok {
		t.Fatal("文件变化未阻断")
	}
}

func Test普通导入终态要求全部阶段与固定日志且拒绝缺口(t *testing.T) {
	c := importdomain.Config{Content: "DDL_AND_DATA", Format: "CSV"}
	base := []string{"Load schema finished. Total Elapsed: 1 s", "Load record finished. Total Elapsed: 2 s", "System exit 0"}
	for _, invalid := range []string{"", "missing-schema", "missing-data", "missing-exit", "failed", "gap", "fake-prefix"} {
		t.Run(invalid, func(t *testing.T) {
			terminal := &normalImportTerminal{}
			for i, value := range base {
				if invalid == []string{"missing-schema", "missing-data", "missing-exit"}[i] {
					continue
				}
				prefix := "2026-10-16 12:00:00 [INFO] "
				if invalid == "fake-prefix" {
					prefix = "user text "
				}
				terminal.observe(logstream.Record{Kind: logstream.RecordLog, Message: prefix + value})
			}
			if invalid == "failed" {
				terminal.observe(logstream.Record{Kind: logstream.RecordLog, Message: "2026-10-16 12:00:00 [ERROR] Load failed!"})
			}
			if invalid == "gap" {
				terminal.observe(logstream.Record{Kind: logstream.RecordGap})
			}
			if terminal.verified(c) != (invalid == "") {
				t.Fatal("完成证据判定错误")
			}
		})
	}
}

func Test普通导入结果没有输入文件计数且非零退出失败(t *testing.T) {
	for _, exit := range []int{0, 1} {
		input, _ := normalImportTestInput(t)
		protocol := &executionWorkerTestProtocol{}
		worker := &Worker{Protocol: protocol, Clock: time.Now, BootID: "boot"}
		grant := agentwire.ExecutionGrant{TaskID: "task", ExecutionID: "execution", LeaseID: "lease", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("a", 64), NormalImport: input}
		terminal := &normalImportTerminal{data: true, exited: true}
		logs := newLogUploader(context.Background(), protocol, "boot", grant, nil, nil)
		outcome, _, err := worker.finishNormalImport(context.Background(), grant, 5, exit, nil, false, false, terminal, logs)
		if outcome.Succeeded != (exit == 0) || (err == nil) != (exit == 0) {
			t.Fatal("退出码与结果判定错误")
		}
		if len(protocol.events) != 2 {
			t.Fatal("终态事实不完整")
		}
		facts := protocol.events[1].Evidence
		for _, key := range []string{"fileCount", "totalBytes", "files"} {
			if _, ok := facts[key]; ok {
				t.Fatal("输入被伪装成结果")
			}
		}
		if facts["normalImport"] != true || facts["checkpointPresent"] != false {
			t.Fatal("结果类型错误")
		}
	}
}
