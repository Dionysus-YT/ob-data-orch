package agentexec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/logstream"
)

func Test直接Java启动输入只接受私有安全配置与已校验Java(t *testing.T) {
	workspace, configuration := directJavaWorkspace(t)
	javaPath, toolHome, digest := directJavaFiles(t)
	launch := DirectJavaLaunch{
		Intent:                validStartIntent(),
		BootID:                "boot-1",
		JavaPath:              javaPath,
		JavaSHA256:            digest,
		ToolHome:              toolHome,
		SecurityConfiguration: configuration,
		JVMOptions:            []string{"-Xms64m", "-XX:+UseG1GC"},
		BusinessArguments:     []string{"--host", "127.0.0.1", "--csv"},
		Environment:           []string{"PATH=C:\\Windows\\System32", "SystemRoot=C:\\Windows"},
		Output:                directJavaOutput(t),
	}
	if err := validateDirectJavaLaunch(workspace, launch); err != nil {
		t.Fatalf("validateDirectJavaLaunch() 错误 = %v", err)
	}
	launch.SecurityConfiguration = filepath.Join(t.TempDir(), "security.properties")
	if err := validateDirectJavaLaunch(workspace, launch); err != ErrDirectJavaLaunchInvalid {
		t.Fatalf("外部安全配置错误 = %v", err)
	}
}

func Test直接Java启动拒绝密码参数与环境注入(t *testing.T) {
	workspace, configuration := directJavaWorkspace(t)
	javaPath, toolHome, digest := directJavaFiles(t)
	launch := DirectJavaLaunch{
		Intent:                validStartIntent(),
		BootID:                "boot-1",
		JavaPath:              javaPath,
		JavaSHA256:            digest,
		ToolHome:              toolHome,
		SecurityConfiguration: configuration,
		BusinessArguments:     []string{"--password", "synthetic-secret-not-allowed"},
		Environment:           []string{"PATH=C:\\Windows\\System32"},
		Output:                directJavaOutput(t),
	}
	if err := validateDirectJavaLaunch(workspace, launch); err != ErrDirectJavaLaunchInvalid {
		t.Fatalf("密码参数错误 = %v", err)
	}
	launch.BusinessArguments = []string{"--csv"}
	launch.Environment = []string{"PATH=C:\\Windows\\System32", "JAVA_TOOL_OPTIONS=-Dunsafe=true"}
	if err := validateDirectJavaLaunch(workspace, launch); err != ErrDirectJavaLaunchInvalid {
		t.Fatalf("环境注入错误 = %v", err)
	}
	launch.Environment = []string{"PATH=C:\\Windows\\System32"}
	launch.Output.Policy = logstream.Policy{Version: "synthetic-v1", Secrets: []string{"synthetic-string-secret"}}
	if err := validateDirectJavaLaunch(workspace, launch); err != ErrDirectJavaLaunchInvalid {
		t.Fatalf("字符串秘密上下文错误 = %v", err)
	}
}

func Test直接Java管道在字节边界脱敏且不暴露原文(t *testing.T) {
	secret := []byte("synthetic-cross-boundary-secret")
	policy, err := logstream.NewBytePolicy("agent-v1", [][]byte{secret}, nil)
	credential.Zero(secret)
	if err != nil {
		t.Fatalf("NewBytePolicy() 错误 = %v", err)
	}
	defer policy.Destroy()
	reader := &chunkReader{chunks: [][]byte{[]byte("password=synthetic-cross-"), []byte("boundary-secret\nnext line\n")}}
	var records []logstream.Record
	err = collectDirectJavaStream(io.NopCloser(reader), logstream.SourceStdout, DirectJavaLogStream{StreamID: "stdout-1", SourceEpoch: 1, ParserVersion: "raw-pipe-v1"}, policy, func(record logstream.Record) error {
		records = append(records, record)
		return nil
	})
	if err != nil || len(records) != 2 {
		t.Fatalf("collectDirectJavaStream() = %#v, %v", records, err)
	}
	serialized := records[0].Message + "\n" + records[1].Message
	if strings.Contains(serialized, "synthetic-cross-boundary-secret") || !strings.Contains(serialized, logstream.Mask) {
		t.Fatalf("接收记录泄露原文: %q", serialized)
	}
}

func Test直接Java管道接收失败仍排空来源(t *testing.T) {
	secret := []byte("synthetic-drain-secret")
	policy, err := logstream.NewBytePolicy("agent-v1", [][]byte{secret}, nil)
	credential.Zero(secret)
	if err != nil {
		t.Fatalf("NewBytePolicy() 错误 = %v", err)
	}
	defer policy.Destroy()
	reader := &chunkReader{chunks: [][]byte{[]byte("password=synthetic-drain-secret\n"), []byte("later safe line\n")}}
	err = collectDirectJavaStream(io.NopCloser(reader), logstream.SourceStderr, DirectJavaLogStream{StreamID: "stderr-1", SourceEpoch: 1, ParserVersion: "raw-pipe-v1"}, policy, func(logstream.Record) error {
		return errors.New("synthetic sink refusal")
	})
	if !errors.Is(err, ErrDirectJavaOutputFailed) || reader.remaining() != 0 {
		t.Fatalf("接收失败后的排空结果 = %v，剩余块数 = %d", err, reader.remaining())
	}
}

func directJavaWorkspace(t *testing.T) (credential.Workspace, string) {
	t.Helper()
	workspace := createWorkspace(t)
	t.Cleanup(func() { _ = workspace.Cleanup() })
	secret := []byte("synthetic-direct-java-secret")
	material, err := credential.GenerateSecurityMaterial(secret)
	credential.Zero(secret)
	if err != nil {
		t.Fatalf("GenerateSecurityMaterial() 错误 = %v", err)
	}
	defer material.Destroy()
	paths, err := workspace.WriteSecurityMaterial(&material)
	if err != nil {
		t.Fatalf("WriteSecurityMaterial() 错误 = %v", err)
	}
	return workspace, paths.SecurityConfiguration
}

func directJavaFiles(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	javaPath := filepath.Join(root, "java.exe")
	content := []byte("synthetic-java-binary")
	if err := os.WriteFile(javaPath, content, 0o600); err != nil {
		t.Fatalf("写入 Java 夹具错误 = %v", err)
	}
	toolHome := filepath.Join(root, "tool")
	if err := os.MkdirAll(filepath.Join(toolHome, "lib"), 0o700); err != nil {
		t.Fatalf("创建工具目录错误 = %v", err)
	}
	digest := sha256.Sum256(content)
	return javaPath, toolHome, hex.EncodeToString(digest[:])
}

func directJavaOutput(t *testing.T) DirectJavaOutput {
	t.Helper()
	secret := []byte("synthetic-output-secret")
	policy, err := logstream.NewBytePolicy("agent-v1", [][]byte{secret}, [][]byte{[]byte("synthetic-user@tenant")})
	credential.Zero(secret)
	if err != nil {
		t.Fatalf("NewBytePolicy() 错误 = %v", err)
	}
	t.Cleanup(policy.Destroy)
	return DirectJavaOutput{
		Policy: policy,
		Stdout: DirectJavaLogStream{StreamID: "stdout-1", SourceEpoch: 1, ParserVersion: "raw-pipe-v1"},
		Stderr: DirectJavaLogStream{StreamID: "stderr-1", SourceEpoch: 1, ParserVersion: "raw-pipe-v1"},
		Sink:   func(logstream.Record) error { return nil },
	}
}

type chunkReader struct {
	chunks [][]byte
}

func (r *chunkReader) Read(destination []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := r.chunks[0]
	r.chunks = r.chunks[1:]
	return copy(destination, chunk), nil
}

func (r *chunkReader) remaining() int {
	return len(r.chunks)
}

var _ io.Reader = (*chunkReader)(nil)
