package agentenvironment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentexec"
	"ob-data-orch/internal/agentlocalpreflight"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/nodefacts"
)

type fixedVersionStub struct {
	calls   int
	version string
	err     error
}

func (v *fixedVersionStub) Probe(_ context.Context, kind string) (string, error) {
	v.calls++
	if v.err != nil {
		return "", v.err
	}
	if v.version != "" {
		return v.version, nil
	}
	if kind == "JAVA_RUNTIME" {
		return "1.8.0_491", nil
	}
	return "4.3.5-RELEASE", nil
}

func inspectionFixture(t *testing.T) (LocalInspector, *fixedVersionStub) {
	t.Helper()
	root := t.TempDir()
	work, queue, tool := filepath.Join(root, "workspace"), filepath.Join(root, "queue"), filepath.Join(root, "tool")
	for _, path := range []string{work, queue, filepath.Join(tool, "lib"), filepath.Join(tool, "bin"), filepath.Join(tool, "conf")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	extension := ""
	launcherDirectory := filepath.Join(tool, "bin")
	platform := "LINUX_AMD64"
	if filepath.VolumeName(root) != "" {
		platform = "WINDOWS_AMD64"
		extension = ".bat"
		launcherDirectory = filepath.Join(launcherDirectory, "windows")
	}
	if err := os.MkdirAll(launcherDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(root, "java")
	for _, path := range []string{java, filepath.Join(launcherDirectory, "obdumper"+extension), filepath.Join(launcherDirectory, "obloader"+extension), filepath.Join(tool, "lib", "oceanbase-client-2.4.14.jar")} {
		if err := os.WriteFile(path, []byte("synthetic fixture"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tool, "conf", "session.config.json"), []byte(`{"private":{"password":"synthetic-secret-never-project"},"direct_path_load":{"task_timeout_ms":"0","heartbeat_timeout_ms":60000000,"heartbeat_interval_ms":"30000000","unknown":"synthetic-private-value"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	versions := &fixedVersionStub{}
	return LocalInspector{Platform: platform, JavaPath: java, ToolHome: tool, WorkspaceRoot: work, LogQueueRoot: queue, ConfigurationDigest: strings.Repeat("a", 64), Versions: versions, ResourceReady: func() bool { return true }, Clock: time.Now}, versions
}

func Test完整采集区分文件与实际版本且未授权时不启动工具(t *testing.T) {
	inspector, versions := inspectionFixture(t)
	report, err := inspector.Inspect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	report.CheckID = "synthetic-check"
	if nodefacts.ValidateInspection(report) != nil || versions.calls != 0 || len(report.Items) != 10 {
		t.Fatal("未授权采集扩大了版本查询或缺少固定项")
	}
	if status, _ := nodefacts.InspectionResult(report); status != "FAILED" {
		t.Fatal("未验证实际版本却形成完整通过")
	}
	report, err = inspector.Inspect(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	report.CheckID = "synthetic-check"
	if nodefacts.ValidateInspection(report) != nil || versions.calls != 3 {
		t.Fatal("固定复验授权没有采集三个实际版本")
	}
	if status, _ := nodefacts.InspectionResult(report); status != "PASSED" {
		t.Fatal("完整的合成检查不能通过")
	}
	if report.DirectLoad.TaskTimeoutMS == nil || *report.DirectLoad.TaskTimeoutMS != 0 || report.DirectLoad.Fingerprint == "" || !report.DirectLoad.HasUnrecognizedKeys {
		t.Fatal("安全数值摘要遗漏真实零值或来源")
	}
	encoded, _ := json.Marshal(report)
	for _, forbidden := range []string{"synthetic-secret-never-project", "synthetic-private-value", "password"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("原始或未知配置进入响应")
		}
	}
	for _, item := range report.Items {
		if item.Kind == "WORKSPACE" && (item.Writable == nil || !*item.Writable) {
			t.Fatal("目录访问没有实际核验")
		}
	}
	entries, err := os.ReadDir(inspector.WorkspaceRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("目录访问测试留下临时文件")
	}
}

func Test逐项采集不能把错误层级启动文件当成平台布局通过(t *testing.T) {
	inspector, _ := inspectionFixture(t)
	launcher, ok := agentlocalpreflight.ToolLauncherPath(inspector.ToolHome, commandgen.Platform(inspector.Platform), "obdumper")
	if !ok {
		t.Fatal("合成平台布局无效")
	}
	wrong := filepath.Join(inspector.ToolHome, "bin", "obdumper.bat")
	if inspector.Platform != "WINDOWS_AMD64" {
		wrong = filepath.Join(inspector.ToolHome, "obdumper")
	}
	if err := os.Rename(launcher, wrong); err != nil {
		t.Fatal(err)
	}
	report, err := inspector.Inspect(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		if item.Kind == "OBDUMPER_RUNTIME" {
			if item.Path != launcher || item.Status != "FAILED" || item.Code != "MISSING" {
				t.Fatal("错误层级文件覆盖了正确平台路径缺失")
			}
			return
		}
	}
	t.Fatal("清单遗漏导出工具")
}

func Test未知冲突配置与版本异常有明确稳定事实(t *testing.T) {
	inspector, versions := inspectionFixture(t)
	versions.err = agentexec.ErrVersionProbeTimeout
	report, _ := inspector.Inspect(context.Background(), true)
	if report.Items[0].Code != "VERSION_PROBE_TIMEOUT" || report.Items[0].Version != "" {
		t.Fatal("超时被当作版本结果")
	}
	versions.err = nil
	versions.version = "17.0.1"
	report, _ = inspector.Inspect(context.Background(), true)
	if report.Items[0].Status != "FAILED" || report.Items[0].Code != "VERSION_UNSUPPORTED" {
		t.Fatal("错误 Java 版本通过")
	}
	path := filepath.Join(inspector.ToolHome, "conf", "session.config.json")
	for _, content := range []string{`{"direct_path_load":{"task_timeout_ms":1,"task_timeout_ms":2}}`, `{"direct_path_load":{"task_timeout_ms":"synthetic-secret"}}`, `{"direct_path_load":true}`, strings.Repeat("x", (64<<10)+1)} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if value := readDirectLoadConfiguration(path); value.Status != "INVALID" || value.Fingerprint != "" {
			t.Fatal("非法配置被展示为已读数值")
		}
	}
}

type inspectionProtocol struct {
	recordingProtocol
	report *nodefacts.RuntimeInspection
}

func (p *inspectionProtocol) CompleteExecutionNodeEnvironmentInspection(_ context.Context, _, _ string, _ int64, status, code string, _ time.Time, report *nodefacts.RuntimeInspection) error {
	p.status, p.code, p.report = status, code, report
	return nil
}

func Test逐项结果不能覆盖平台布局失败(t *testing.T) {
	inspector, _ := inspectionFixture(t)
	protocol := &inspectionProtocol{}
	worker := Worker{Protocol: protocol, Runtime: agentlocalpreflight.RuntimeValidatorFunc(func(context.Context) error { return agentlocalpreflight.ErrToolRuntimeInvalid }), Inspector: inspector, Clock: time.Now, BootID: "synthetic-boot"}
	if ran, err := worker.RunWithInspection(context.Background(), "synthetic-check", 1, true); !ran || err != nil {
		t.Fatal(err)
	}
	if protocol.status != "FAILED" || protocol.report == nil {
		t.Fatal("逐项采集覆盖了既有失败关闭边界")
	}
}
