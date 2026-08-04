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

func TestWindowsOBDumper启动配置复刻官方脚本并保留任务私有材料(t *testing.T) {
	workspace, configuration := directJavaWorkspace(t)
	toolHome := filepath.Join(t.TempDir(), "ob-loader-dumper-4.3.5-RELEASE")
	arguments, err := windowsOBDumperReplicaArguments(workspace, toolHome, configuration, 300)
	if err != nil {
		t.Fatalf("windowsOBDumperReplicaArguments() = %v", err)
	}
	log4jConfiguration := "file:///" + strings.TrimPrefix(filepath.ToSlash(filepath.Join(toolHome, "conf", "log4j2.xml")), "/")
	want := []string{
		"-server",
		"-Xms4G",
		"-Xmx4G",
		"-Xss512K",
		"-XX:MetaspaceSize=128M",
		"-XX:MaxMetaspaceSize=128M",
		"-XX:+UseG1GC",
		"-XX:CICompilerCount=4",
		"-XX:ParallelGCThreads=4",
		"-Xnoclassgc",
		"-XX:MaxGCPauseMillis=50",
		"-XX:+HeapDumpOnOutOfMemoryError",
		"-XX:HeapDumpPath=" + workspace.RawLogDirectory(),
		"-Dsun.stdout.encoding=UTF-8",
		"-Dsun.stderr.encoding=UTF-8",
		"-Dsecurity.configurationFile=" + configuration,
		"-Dpicocli.usage.width=180",
		"-Denable.parallel.write=false",
		"-Dskip.tableName.check=false",
		"-Dupload.buffer.type=disk",
		"-Dupload.buffer.size=67108864",
		"-Dupload.active.blocks=2",
		"-Dupload.disable.chunked.encoding=false",
		"-DsqlMonitor.enabled=true",
		"-DsqlMonitor.slowSql.threshold=3000",
		"-Denable.table.index=true",
		"-Denable.table.comment=true",
		"-Denable.table.column.comment=true",
		"-Dtool.base.dir=" + filepath.ToSlash(toolHome),
		"-Dobproxy.configurationFile=" + filepath.ToSlash(filepath.Join(toolHome, "conf", "secure.crt")),
		"-Dsession.configurationFile=" + filepath.ToSlash(filepath.Join(toolHome, "conf", "session.config.json")),
		"-Ddecrypt.configurationFile=" + filepath.ToSlash(filepath.Join(toolHome, "conf", "decrypt.properties")),
		"-Dlog4j.output=" + workspace.RawLogDirectory(),
		"-Dlog4j2.formatMsgNoLookups=true",
		"-Dlog4j.configurationFile=" + log4jConfiguration,
		"-Dhadoop.home.dir=" + filepath.ToSlash(filepath.Join(toolHome, "ext", "windows", "hadoop")),
		"-classpath",
		".;" + filepath.Join(toolHome, "lib", "*"),
		obdumperMainClass,
	}
	if len(arguments) != len(want) {
		t.Fatalf("启动参数数量 = %d, want %d: %s", len(arguments), len(want), strings.Join(arguments, "\n"))
	}
	for index := range want {
		if arguments[index] != want[index] {
			t.Fatalf("启动参数[%d] = %q, want %q", index, arguments[index], want[index])
		}
	}
}

func TestWindowsOBDumper启动配置保留Java8旧版本CMS分支(t *testing.T) {
	workspace, configuration := directJavaWorkspace(t)
	arguments, err := windowsOBDumperReplicaArguments(workspace, filepath.Join(t.TempDir(), "tool"), configuration, 291)
	if err != nil {
		t.Fatalf("windowsOBDumperReplicaArguments() = %v", err)
	}
	joined := strings.Join(arguments, "\n")
	if !strings.Contains(joined, "-XX:+UseConcMarkSweepGC") || strings.Contains(joined, "-XX:+UseG1GC") {
		t.Fatalf("Java 8 update 291 的 GC 参数 = %s", joined)
	}
}

func Test解析Java8Update仅接受官方脚本支持的版本文本(t *testing.T) {
	for _, testCase := range []struct {
		name string
		text string
		want int
		ok   bool
	}{
		{name: "oracle", text: "java version \"1.8.0_301\"", want: 301, ok: true},
		{name: "openjdk", text: "openjdk version \"1.8.0_292\"", want: 292, ok: true},
		{name: "unsupported", text: "openjdk version \"17.0.1\"", ok: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.ok {
				got, err := parseJava8Update([]byte(testCase.text))
				if err != nil || got != testCase.want {
					t.Fatalf("parseJava8Update() = %d, %v; want %d, nil", got, err, testCase.want)
				}
				return
			}
			if _, err := parseJava8Update([]byte(testCase.text)); err == nil {
				t.Fatal("不支持的 Java 版本被接受")
			}
		})
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
