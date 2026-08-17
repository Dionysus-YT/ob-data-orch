package agentexecution

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ob-data-orch/internal/agentwire"
)

// TestOutputFactsReportsBoundedFileList 验证结果事实枚举：数量/字节数与受限相对路径清单，
// 并拒绝超长文件名（失败关闭，不把未验证事实当作可靠结果）。
func TestOutputFactsReportsBoundedFileList(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(relative string, content []byte) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("data_1.csv", []byte("synthetic-row"))
	write("nested/data_2.csv", []byte("synthetic-row-2"))
	count, bytes, files, ok := outputFacts(root)
	if !ok || count != 2 || bytes != uint64(len("synthetic-row")+len("synthetic-row-2")) || len(files) != 2 {
		t.Fatalf("outputFacts() = %d, %d, %#v, %t", count, bytes, files, ok)
	}
	paths := map[string]uint64{}
	for _, file := range files {
		if strings.Contains(file.Path, "\\") {
			t.Fatalf("相对路径必须是斜杠形式: %q", file.Path)
		}
		paths[file.Path] = file.Size
	}
	if paths["data_1.csv"] != uint64(len("synthetic-row")) || paths["nested/data_2.csv"] != uint64(len("synthetic-row-2")) {
		t.Fatalf("结果文件清单 = %#v", paths)
	}
	// 含控制字符或超长路径的文件名必须失败关闭（Windows 文件系统本身禁止这类名称，
	// 该负例只在允许创建此类文件名的平台上执行；协议层的路径边界由 store 侧测试覆盖）。
	if runtime.GOOS != "windows" {
		write("data\nsecret.csv", []byte("synthetic"))
		if _, _, _, ok := outputFacts(root); ok {
			t.Fatal("含控制字符文件名被接受为可靠结果事实")
		}
	}
}

// TestCheckpointPresent 验证 dump.ckpt 存在性/可读性检查的失败关闭边界。
func TestCheckpointPresent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if checkpointPresent(root) {
		t.Fatal("不存在时不能报告检查点存在")
	}
	ckpt := filepath.Join(root, "dump.ckpt")
	if err := os.WriteFile(ckpt, []byte("synthetic-checkpoint"), 0o600); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	if !checkpointPresent(root) {
		t.Fatal("常规可读检查点文件必须报告存在")
	}
	// 目录形态的 dump.ckpt 不是有效检查点。
	if err := os.Remove(ckpt); err != nil {
		t.Fatalf("remove checkpoint: %v", err)
	}
	if err := os.Mkdir(ckpt, 0o700); err != nil {
		t.Fatalf("mkdir checkpoint: %v", err)
	}
	if checkpointPresent(root) {
		t.Fatal("目录形态的 dump.ckpt 被当作有效检查点")
	}
}

// TestExecutionPathsAcceptControlledStorageURI 验证对象存储输出不会再按 Agent 本地路径解析，
// 同时保留 URI、临时目录和查询参数的收窄边界。
func TestExecutionPathsAcceptControlledStorageURI(t *testing.T) {
	t.Parallel()
	paths, ok := executionPathsFromArgv("WINDOWS_AMD64", []string{
		"--file-path", "cos://synthetic-bucket/exports?endpoint=cos.example.com&storage-class=STANDARD",
		"--tmp-path", "/E:/tmp/ob-data-orch",
	})
	if !ok || !paths.StorageOutput || paths.OutputPath == "" || paths.TmpPath != "/E:/tmp/ob-data-orch" {
		t.Fatalf("executionPathsFromArgv() = %#v, %t", paths, ok)
	}
	negative := [][]string{
		{"--file-path", "ftp://synthetic-bucket/exports"},
		{"--file-path", "s3://synthetic-bucket/exports?access-key=synthetic"},
		{"--file-path", "oss://synthetic-bucket/exports", "--log-path", "/E:/tmp/logs"},
	}
	for _, argv := range negative {
		if paths, ok := executionPathsFromArgv("WINDOWS_AMD64", argv); ok {
			t.Fatalf("越界对象存储参数被接受: %#v", paths)
		}
	}
}

// TestExecutionLogPolicyRedactsStorageSecrets 验证数据库密码与对象存储两个秘密都进入
// execution 级字节脱敏策略，工具输出不得泄露槽位内容。
func TestExecutionLogPolicyRedactsStorageSecrets(t *testing.T) {
	t.Parallel()
	password := []byte("synthetic-db-password")
	storage := &agentwire.StorageCredentialSlot{
		Provider:  "OSS",
		AccessKey: []byte("synthetic-access-key"),
		SecretKey: []byte("synthetic-secret-key"),
	}
	policy, err := executionLogPolicy(password, storage)
	if err != nil {
		t.Fatalf("executionLogPolicy(): %v", err)
	}
	defer policy.Destroy()
	message := "password=synthetic-db-password access_key=synthetic-access-key secret=synthetic-secret-key"
	redacted, err := policy.Redact(message)
	if err != nil {
		t.Fatalf("Policy.Redact(): %v", err)
	}
	for _, secret := range []string{"synthetic-db-password", "synthetic-access-key", "synthetic-secret-key"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("脱敏结果仍含秘密 %q: %q", secret, redacted)
		}
	}
}

// TestStorageExecutionFactsRemainEvidenceLimited 验证远端清单能力未接入前，
// 对象存储成功仅投影空文件事实，且不会从本地临时目录伪造检查点。
func TestStorageExecutionFactsRemainEvidenceLimited(t *testing.T) {
	t.Parallel()
	paths := executionPaths{OutputPath: "s3://synthetic-bucket/exports", StorageOutput: true}
	count, bytes, files, ok := executionOutputFacts(paths, "")
	if !ok || count != 0 || bytes != 0 || len(files) != 0 {
		t.Fatalf("executionOutputFacts() = %d, %d, %#v, %t", count, bytes, files, ok)
	}
	if executionCheckpointPresent(paths, t.TempDir()) {
		t.Fatal("对象存储输出不能从本地临时目录报告检查点")
	}
}
