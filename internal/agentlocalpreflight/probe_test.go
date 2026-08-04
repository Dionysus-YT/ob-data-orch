package agentlocalpreflight

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/outputpath"
)

func TestProbeRunsFixedLocalChecksWithSafeFacts(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatalf("创建输出目录错误 = %v", err)
	}
	probe := Probe{
		JDBC:                  jdbcStub{},
		Runtime:               RuntimeValidatorFunc(func(context.Context) error { return nil }),
		MinimumAvailableBytes: 64,
		AvailableBytes: func(path string) (uint64, error) {
			if path != output {
				t.Fatal("空间检查没有使用解析后的冻结输出目录")
			}
			return 64, nil
		},
	}
	report, err := agentpreflight.Run(context.Background(), testRequest(output, root), probe)
	if err != nil || !report.Succeeded {
		t.Fatalf("Run() = %#v, %v", report, err)
	}
	if entries, readErr := os.ReadDir(output); readErr != nil || len(entries) != 0 {
		t.Fatalf("输出路径检查遗留文件 = %#v, %v", entries, readErr)
	}
}

func TestProbeFailsOutputChecksWithoutEscapingAllowedRoot(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	for _, directory := range []string{allowed, outside} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("创建目录错误 = %v", err)
		}
	}
	probe := Probe{}
	result, err := probe.Probe(context.Background(), agentpreflight.CheckOutputPath, testRequest(outside, allowed))
	if err != nil || result.Status != agentpreflight.StatusFailed || result.EvidenceCode != "OUTPUT_PATH_NOT_WRITABLE" {
		t.Fatalf("越界输出目录结果 = %#v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(allowed, "existing.csv"), []byte("synthetic"), 0o600); err != nil {
		t.Fatalf("写入非空夹具错误 = %v", err)
	}
	result, err = probe.Probe(context.Background(), agentpreflight.CheckOutputEmpty, testRequest(allowed, allowed))
	if err != nil || result.Status != agentpreflight.StatusFailed || result.EvidenceCode != "OUTPUT_PATH_NOT_EMPTY" {
		t.Fatalf("非空输出目录结果 = %#v, %v", result, err)
	}
}

func TestProbeChecksLogDirectoryButCanSkipOnlyOutputEmptiness(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	output := filepath.Join(allowed, "output")
	logs := filepath.Join(allowed, "logs")
	if err := os.Mkdir(allowed, 0o700); err != nil {
		t.Fatalf("创建允许根目录错误 = %v", err)
	}
	for _, directory := range []string{output, logs} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("创建测试目录错误 = %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(output, "existing.csv"), []byte("synthetic"), 0o600); err != nil {
		t.Fatalf("写入非空输出夹具错误 = %v", err)
	}
	request := testRequest(output, allowed)
	if testPlatform() == commandgen.PlatformWindowsAMD64 {
		logs = "/" + strings.ReplaceAll(logs, "\\", "/")
	}
	request.LogPath = logs
	request.SkipCheckDir = true
	probe := Probe{}
	result, err := probe.Probe(context.Background(), agentpreflight.CheckOutputPath, request)
	if err != nil || result.Status != agentpreflight.StatusPassed || result.EvidenceCode != "OUTPUT_PATH_WRITABLE" {
		t.Fatalf("日志路径可写检查 = %#v, %v", result, err)
	}
	result, err = probe.Probe(context.Background(), agentpreflight.CheckOutputEmpty, request)
	if err != nil || result.Status != agentpreflight.StatusPassed || result.EvidenceCode != "OUTPUT_EMPTY_CHECK_SKIPPED" {
		t.Fatalf("跳过目录空性检查 = %#v, %v", result, err)
	}
	request.LogPath = filepath.Join(root, "outside")
	if testPlatform() == commandgen.PlatformWindowsAMD64 {
		request.LogPath = "/" + strings.ReplaceAll(request.LogPath, "\\", "/")
	}
	result, err = probe.Probe(context.Background(), agentpreflight.CheckOutputPath, request)
	if err != nil || result.Status != agentpreflight.StatusFailed || result.EvidenceCode != "OUTPUT_PATH_NOT_WRITABLE" {
		t.Fatalf("越界日志路径检查 = %#v, %v", result, err)
	}
}

func TestProbeAllowsObdumperToCreateMissingOutputAndLogDirectories(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "generated", "output")
	logs := filepath.Join(root, "generated", "logs")
	request := testRequest(output, root)
	if testPlatform() == commandgen.PlatformWindowsAMD64 {
		logs = "/" + strings.ReplaceAll(logs, "\\", "/")
	}
	request.LogPath = logs
	probe := Probe{
		MinimumAvailableBytes: 64,
		AvailableBytes: func(path string) (uint64, error) {
			if path != root {
				t.Fatalf("缺失目录的空间检查没有使用最近可用父目录：%q", path)
			}
			return 64, nil
		},
	}
	result, err := probe.Probe(context.Background(), agentpreflight.CheckOutputPath, request)
	if err != nil || result.Status != agentpreflight.StatusPassed || result.EvidenceCode != "OUTPUT_PATH_WRITABLE" {
		t.Fatalf("可由工具创建的目录可写检查 = %#v, %v", result, err)
	}
	result, err = probe.Probe(context.Background(), agentpreflight.CheckOutputEmpty, request)
	if err != nil || result.Status != agentpreflight.StatusPassed || result.EvidenceCode != "OUTPUT_PATH_EMPTY" {
		t.Fatalf("可由工具创建的目录空性检查 = %#v, %v", result, err)
	}
	result, err = probe.Probe(context.Background(), agentpreflight.CheckAvailableSpace, request)
	if err != nil || result.Status != agentpreflight.StatusPassed || result.EvidenceCode != "OUTPUT_SPACE_SUFFICIENT" {
		t.Fatalf("可由工具创建的目录空间检查 = %#v, %v", result, err)
	}
	for _, directory := range []string{output, logs} {
		localPath := directory
		if testPlatform() == commandgen.PlatformWindowsAMD64 {
			var ok bool
			localPath, ok = outputpath.LocalFilesystemPath(string(testPlatform()), directory)
			if !ok {
				t.Fatalf("日志路径转换失败：%q", directory)
			}
		}
		if _, statErr := os.Stat(localPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("预检查不应创建用户目录：%q, %v", localPath, statErr)
		}
	}
}

func TestProbeFailsClosedWhenRuntimeOrSpaceCannotBeConfirmed(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatalf("创建输出目录错误 = %v", err)
	}
	request := testRequest(output, root)
	for _, test := range []struct {
		name     string
		probe    Probe
		check    agentpreflight.CheckID
		status   agentpreflight.Status
		evidence string
	}{
		{name: "运行时无效", probe: Probe{Runtime: RuntimeValidatorFunc(func(context.Context) error { return ErrToolRuntimeInvalid })}, check: agentpreflight.CheckToolEnvironment, status: agentpreflight.StatusFailed, evidence: "TOOL_RUNTIME_INVALID"},
		{name: "运行时不可用", probe: Probe{Runtime: RuntimeValidatorFunc(func(context.Context) error { return errors.New("synthetic unavailable") })}, check: agentpreflight.CheckToolEnvironment, status: agentpreflight.StatusUnknown, evidence: "TOOL_RUNTIME_UNAVAILABLE"},
		{name: "空间阈值缺失", probe: Probe{AvailableBytes: func(string) (uint64, error) { return 1024, nil }}, check: agentpreflight.CheckAvailableSpace, status: agentpreflight.StatusUnknown, evidence: "OUTPUT_SPACE_UNAVAILABLE"},
		{name: "空间不足", probe: Probe{MinimumAvailableBytes: 128, AvailableBytes: func(string) (uint64, error) { return 127, nil }}, check: agentpreflight.CheckAvailableSpace, status: agentpreflight.StatusFailed, evidence: "OUTPUT_SPACE_INSUFFICIENT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.probe.Probe(context.Background(), test.check, request)
			if err != nil || result.Status != test.status || result.EvidenceCode != test.evidence {
				t.Fatalf("Probe() = %#v, %v", result, err)
			}
		})
	}
}

func TestToolRuntimeValidatorChecksOnlyFixedLocalLayout(t *testing.T) {
	root := t.TempDir()
	javaPath := filepath.Join(root, "java")
	toolHome := filepath.Join(root, "tool")
	connector := filepath.Join(toolHome, "lib", "oceanbase-client-2.4.14.jar")
	launcher, ok := obdumperLauncher(toolHome, testPlatform())
	if !ok {
		t.Fatal("当前测试平台没有固定 OBDUMPER 启动文件")
	}
	for _, path := range []string{filepath.Dir(connector), filepath.Dir(launcher)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("创建受控运行时目录错误 = %v", err)
		}
	}
	for _, path := range []string{javaPath, connector, launcher} {
		if err := os.WriteFile(path, []byte("synthetic"), 0o700); err != nil {
			t.Fatalf("创建受控运行时文件错误 = %v", err)
		}
	}
	if runtime.GOOS == "linux" {
		if err := os.Chmod(launcher, 0o700); err != nil {
			t.Fatalf("设置启动文件权限错误 = %v", err)
		}
	}
	environment := []string{"PATH=" + filepath.Dir(javaPath)}
	if runtime.GOOS == "windows" {
		hadoopHome := filepath.Join(root, "hadoop")
		for _, path := range []string{filepath.Join(hadoopHome, "bin", "hadoop.dll"), filepath.Join(hadoopHome, "bin", "winutils.exe")} {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("创建 Hadoop 运行时目录错误 = %v", err)
			}
			if err := os.WriteFile(path, []byte("synthetic"), 0o700); err != nil {
				t.Fatalf("创建 Hadoop 运行时文件错误 = %v", err)
			}
		}
		environment = append(environment, "HADOOP_HOME="+hadoopHome)
	}
	validator := ToolRuntimeValidator{JavaPath: javaPath, ToolHome: toolHome, Environment: environment, TargetPlatform: testPlatform()}
	if err := validator.Validate(context.Background()); err != nil {
		t.Fatalf("受控运行时校验错误 = %v", err)
	}
	if err := os.Remove(launcher); err != nil {
		t.Fatalf("移除启动文件错误 = %v", err)
	}
	if err := validator.Validate(context.Background()); !errors.Is(err, ErrToolRuntimeInvalid) {
		t.Fatalf("缺失启动文件错误 = %v", err)
	}
}

type jdbcStub struct{}

func (jdbcStub) Probe(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
	switch check {
	case agentpreflight.CheckDatabaseConnectivity:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "DATABASE_CONNECTED"}, nil
	case agentpreflight.CheckObjectAccess:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "OBJECT_ACCESSIBLE"}, nil
	default:
		return agentpreflight.Result{}, errors.New("unexpected JDBC check")
	}
}

func testRequest(output, root string) agentpreflight.Request {
	if testPlatform() == commandgen.PlatformWindowsAMD64 {
		output = "/" + strings.ReplaceAll(output, "\\", "/")
	}
	return agentpreflight.Request{
		Capability:        agentpreflight.CapabilityExportPreflight,
		PrecheckID:        "precheck-1",
		NodeID:            "node-1",
		AgentID:           "agent-1",
		LeaseID:           "lease-1",
		LeaseEpoch:        1,
		CompatibilityMode: "MYSQL",
		Database:          "synthetic_db",
		Table:             "synthetic_table",
		TargetPlatform:    testPlatform(),
		OutputPath:        output,
		AllowedRoots:      []string{root},
		Binding: agentstate.PrecheckBinding{
			PrecheckID:         "precheck-1",
			NodeID:             "node-1",
			DraftRevision:      1,
			ConfigFingerprint:  "synthetic-fingerprint",
			CredentialRevision: 1,
			NodeFactsVersion:   1,
		},
	}
}

func testPlatform() commandgen.Platform {
	if runtime.GOOS == "windows" {
		return commandgen.PlatformWindowsAMD64
	}
	if runtime.GOARCH == "arm64" {
		return commandgen.PlatformLinuxARM64
	}
	return commandgen.PlatformLinuxAMD64
}
