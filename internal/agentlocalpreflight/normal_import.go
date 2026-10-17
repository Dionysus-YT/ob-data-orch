package agentlocalpreflight

import (
	"context"
	"os"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/jdbcprobe"
	"ob-data-orch/internal/outputpath"
	"ob-data-orch/internal/precheckcontract"
)

// ImportRuntimeValidator 只检查本机固定 OBLOADER 布局，不能以 OBDUMPER 文件代替。
type ImportRuntimeValidator struct{ ToolRuntimeValidator }

// Validate 检查同版 Java、固定 JDBC 和 OBLOADER 启动文件，不启动工具或解析秘密。
func (v ImportRuntimeValidator) Validate(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrToolRuntimeUnavailable
	}
	if v.TargetPlatform == commandgen.PlatformWindowsAMD64 && !windowsHadoopRuntimeReady(v.Environment) {
		return ErrToolRuntimeInvalid
	}
	if _, err := jdbcprobe.DiscoverRuntime(v.JavaPath, v.ToolHome, v.Environment); err != nil {
		return ErrToolRuntimeInvalid
	}
	// 与节点环境检查复用唯一官方布局，Windows 的启动文件位于 bin/windows。
	launcher, ok := ToolLauncherPath(v.ToolHome, v.TargetPlatform, "obloader")
	if !ok {
		return ErrToolRuntimeInvalid
	}
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() || v.TargetPlatform != commandgen.PlatformWindowsAMD64 && info.Mode().Perm()&0o111 == 0 {
		return ErrToolRuntimeInvalid
	}
	return nil
}

// ScanImportInput 在 Agent 原生文件系统内检查冻结输入；控制面不能借此浏览任意目录。
func (p Probe) ScanImportInput(ctx context.Context, request agentpreflight.Request) (importfiles.Manifest, error) {
	if request.NormalImport == nil || !agentpreflight.ValidNormalImportContext(request.NormalImport, request.CheckKind, request.CompatibilityMode, request.TargetPlatform, request.AllowedRoots) {
		return importfiles.Manifest{}, agentpreflight.ErrInvalidRequest
	}
	roots := make([]string, 0, len(request.AllowedRoots))
	for _, root := range request.AllowedRoots {
		// 节点允许根目录保留历史协议表示，转换只发生在匹配平台的 Agent；用户输入路径原样使用。
		native, ok := outputpath.LocalFilesystemPath(string(request.TargetPlatform), root)
		if !ok {
			return importfiles.Manifest{}, agentpreflight.ErrInvalidRequest
		}
		roots = append(roots, native)
	}
	return importfiles.Scan(ctx, *request.NormalImport, roots)
}

func (p Probe) normalImportResult(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	if ctx == nil || ctx.Err() != nil {
		return unavailableResult(check), nil
	}
	switch check {
	case agentpreflight.CheckToolEnvironment:
		copy := p
		copy.Runtime = p.ImportRuntime
		return copy.runtimeResult(ctx, check), nil
	case agentpreflight.CheckNodeResource:
		// 节点额度足够也不能覆盖工具自身的最低堆要求；使用同一冻结资源字段，不另行探测机器。
		if request.ResourceRequirement != nil && request.NormalImport.DataHeapInsufficient(request.ResourceRequirement.HeapBytes) {
			return agentpreflight.Result{Check: check, Status: agentpreflight.StatusFailed, EvidenceCode: precheckcontract.EvidenceImportJVMHeapTooSmall}, nil
		}
		return nodeResourceResult(request), nil
	case agentpreflight.CheckDatabaseConnectivity:
		if p.ImportDatabase == nil {
			return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: precheckcontract.EvidenceNormalImportProbeNotConfigured}, nil
		}
		return p.ImportDatabase.Probe(ctx, check, request)
	case "WRITE_ACCESS", "INPUT_MAPPING":
		// 导出 SELECT 权限不能证明写入或列映射正确；专用固定探针未准入时必须保持 UNKNOWN。
		if p.ImportChecks != nil {
			return p.ImportChecks.Probe(ctx, check, request)
		}
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: precheckcontract.EvidenceNormalImportProbeNotConfigured}, nil
	default:
		return agentpreflight.Result{}, agentpreflight.ErrInvalidRequest
	}
}
