package agentlocalpreflight

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/precheckcontract"
)

func Test普通导入Windows仅认可官方OBLOADER启动布局(t *testing.T) {
	root := t.TempDir()
	javaPath := filepath.Join(root, "java.exe")
	toolHome := filepath.Join(root, "tool")
	hadoopHome := filepath.Join(root, "hadoop")
	launcher := filepath.Join(toolHome, "bin", "windows", "obloader.bat")
	connector := filepath.Join(toolHome, "lib", "oceanbase-client-2.4.14.jar")
	writeFixture := func(name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal("无法创建合成运行时目录", err)
		}
		if err := os.WriteFile(name, []byte("synthetic"), 0o700); err != nil {
			t.Fatal("无法创建合成运行时文件", err)
		}
	}
	for _, name := range []string{javaPath, connector, launcher, filepath.Join(hadoopHome, "bin", "hadoop.dll"), filepath.Join(hadoopHome, "bin", "winutils.exe")} {
		writeFixture(name)
	}
	validator := ImportRuntimeValidator{ToolRuntimeValidator: ToolRuntimeValidator{
		JavaPath: javaPath, ToolHome: toolHome, TargetPlatform: commandgen.PlatformWindowsAMD64,
		Environment: []string{"PATH=" + filepath.Dir(javaPath), "HADOOP_HOME=" + hadoopHome},
	}}
	if err := validator.Validate(context.Background()); err != nil {
		t.Fatal("官方 Windows OBLOADER 布局被误判", err)
	}
	for _, alternate := range []string{filepath.Join(toolHome, "bin", "obloader.bat"), filepath.Join(toolHome, "bin", "windows", "obdumper.bat")} {
		writeFixture(alternate)
	}
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(context.Background()); !errors.Is(err, ErrToolRuntimeInvalid) {
		t.Fatal("错误层级或 OBDUMPER 不能替代 OBLOADER", err)
	}
	writeFixture(launcher)
	if err := os.Remove(connector); err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(context.Background()); !errors.Is(err, ErrToolRuntimeInvalid) {
		t.Fatal("缺少固定 JDBC 时未阻断", err)
	}
	writeFixture(connector)
	withoutHadoop := validator
	withoutHadoop.Environment = []string{"PATH=" + filepath.Dir(javaPath)}
	if err := withoutHadoop.Validate(context.Background()); !errors.Is(err, ErrToolRuntimeInvalid) {
		t.Fatal("缺少 Windows 本机依赖时未阻断", err)
	}
	if err := validator.Validate(nil); !errors.Is(err, ErrToolRuntimeUnavailable) {
		t.Fatal("缺少上下文时未失败关闭", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validator.Validate(ctx); !errors.Is(err, ErrToolRuntimeUnavailable) {
		t.Fatal("取消后仍检查运行时", err)
	}
}

func Test普通导入未装配检查不伪装连接或权限失败(t *testing.T) {
	probe := Probe{}
	request := agentpreflight.Request{NormalImport: &importdomain.Config{}}
	for _, check := range []agentpreflight.CheckID{agentpreflight.CheckDatabaseConnectivity, "WRITE_ACCESS", "INPUT_MAPPING"} {
		result, err := probe.Probe(context.Background(), check, request)
		if err != nil || result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != precheckcontract.EvidenceNormalImportProbeNotConfigured ||
			!precheckcontract.ValidResult(string(check), string(result.Status), result.EvidenceCode) || precheckcontract.ValidResult(string(check), "PASSED", result.EvidenceCode) {
			t.Fatal("未装配检查没有独立 UNKNOWN 证据或被接受为通过", err)
		}
	}
}

func Test普通导入数据最低堆与节点额度分别失败关闭(t *testing.T) {
	for _, item := range []struct {
		name, content, memory string
		memoryTotal           uint64
		status                agentpreflight.Status
		code                  string
	}{
		{"数据小堆", "DATA_ONLY", "512M", 16 << 30, agentpreflight.StatusFailed, precheckcontract.EvidenceImportJVMHeapTooSmall},
		{"结构和数据小堆", "DDL_AND_DATA", "699M", 16 << 30, agentpreflight.StatusFailed, precheckcontract.EvidenceImportJVMHeapTooSmall},
		{"工具下限", "DATA_ONLY", "700M", 16 << 30, agentpreflight.StatusPassed, "NODE_RESOURCE_READY"},
		{"足额堆", "DATA_ONLY", "1G", 16 << 30, agentpreflight.StatusPassed, "NODE_RESOURCE_READY"},
		{"官方默认", "DATA_ONLY", "", 16 << 30, agentpreflight.StatusPassed, "NODE_RESOURCE_READY"},
		{"仅结构", "DDL_ONLY", "512M", 16 << 30, agentpreflight.StatusPassed, "NODE_RESOURCE_READY"},
		{"节点额度不足", "DATA_ONLY", "4G", 4 << 30, agentpreflight.StatusFailed, "NODE_RESOURCE_INSUFFICIENT"},
		{"缺少节点事实", "DATA_ONLY", "1G", 0, agentpreflight.StatusUnknown, "NODE_RESOURCE_UNAVAILABLE"},
		{"无效堆", "DATA_ONLY", "invalid", 16 << 30, agentpreflight.StatusUnknown, "NODE_RESOURCE_UNAVAILABLE"},
	} {
		t.Run(item.name, func(t *testing.T) {
			requirement := executionbudget.Preflight(item.memory, item.memoryTotal, executionbudget.Policy{})
			request := agentpreflight.Request{NormalImport: &importdomain.Config{Content: item.content}, ResourceRequirement: &requirement}
			result, err := (Probe{}).Probe(context.Background(), agentpreflight.CheckNodeResource, request)
			if err != nil || result.Status != item.status || result.EvidenceCode != item.code || !precheckcontract.ValidResult(string(result.Check), string(result.Status), result.EvidenceCode) {
				t.Fatal("工具堆下限与节点额度没有按各自证据判定", result, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, ctx, context.Background()} {
		result, err := (Probe{}).Probe(ctx, agentpreflight.CheckNodeResource, agentpreflight.Request{NormalImport: &importdomain.Config{Content: "DATA_ONLY"}})
		if err != nil || result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != "NODE_RESOURCE_UNAVAILABLE" {
			t.Fatal("取消或缺失资源事实时没有保持未知", result, err)
		}
	}
}
