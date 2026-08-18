// Package agentlocalpreflight 提供固定 EXPORT_PREFLIGHT 的本机非数据库检查。
// 它只读取控制面冻结的本机运行时、输出目录和空间事实，不接受命令、SQL、自由路径或工具启动请求。
package agentlocalpreflight

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/jdbcprobe"
	"ob-data-orch/internal/outputpath"
)

var (
	// ErrToolRuntimeInvalid 表示受控本机 Java 或 OBDUMPER 运行时布局不符合固定要求。
	ErrToolRuntimeInvalid = errors.New("受控工具运行时无效")
	// ErrToolRuntimeUnavailable 表示本机暂时无法确认受控工具运行时。
	ErrToolRuntimeUnavailable = errors.New("受控工具运行时不可用")
)

// RuntimeValidator 只验证 Agent 本机已配置的固定 Java 与 OBDUMPER 运行时。
// 它不接收 Agent 协议、浏览器或环境变量中的远端路径、命令、主类或秘密。
type RuntimeValidator interface {
	Validate(context.Context) error
}

// RuntimeValidatorFunc 让合成测试可以注入受控运行时事实，不能扩展 Probe 的检查清单。
type RuntimeValidatorFunc func(context.Context) error

// Validate 执行注入的受控运行时校验函数。
func (f RuntimeValidatorFunc) Validate(ctx context.Context) error {
	if f == nil {
		return ErrToolRuntimeUnavailable
	}
	return f(ctx)
}

// ToolRuntimeValidator 验证固定 Java、Connector/J 和目标平台 OBDUMPER 启动文件的本机布局。
// 它不启动 Java 或 OBDUMPER；包来源与摘要准入仍由 WI-01 的受控交付记录负责，不能仅凭本机文件存在性通过。
type ToolRuntimeValidator struct {
	JavaPath       string
	ToolHome       string
	Environment    []string
	TargetPlatform commandgen.Platform
}

// Validate 复核受控工具布局，不把任何路径或底层错误文本向预检查结果传播。
func (v ToolRuntimeValidator) Validate(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrToolRuntimeUnavailable
	}
	if v.TargetPlatform == commandgen.PlatformWindowsAMD64 && !windowsHadoopRuntimeReady(v.Environment) {
		return ErrToolRuntimeInvalid
	}
	if _, err := jdbcprobe.DiscoverRuntime(v.JavaPath, v.ToolHome, v.Environment); err != nil {
		return ErrToolRuntimeInvalid
	}
	launcher, ok := obdumperLauncher(v.ToolHome, v.TargetPlatform)
	if !ok {
		return ErrToolRuntimeInvalid
	}
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() {
		return ErrToolRuntimeInvalid
	}
	if v.TargetPlatform == commandgen.PlatformLinuxAMD64 || v.TargetPlatform == commandgen.PlatformLinuxARM64 {
		if info.Mode().Perm()&0o111 == 0 {
			return ErrToolRuntimeInvalid
		}
	}
	return nil
}

// windowsHadoopRuntimeReady 只接受当前 Agent 本机配置的 HADOOP_HOME；任务、浏览器和控制面均不能影响该本地库加载路径。
func windowsHadoopRuntimeReady(environment []string) bool {
	hadoopHome := ""
	for _, value := range environment {
		key, candidate, found := strings.Cut(value, "=")
		if found && strings.EqualFold(key, "HADOOP_HOME") {
			hadoopHome = candidate
			break
		}
	}
	if hadoopHome == "" || strings.ContainsRune(hadoopHome, 0) || !filepath.IsAbs(hadoopHome) {
		return false
	}
	for _, fileName := range []string{"hadoop.dll", "winutils.exe"} {
		info, err := os.Stat(filepath.Join(hadoopHome, "bin", fileName))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// Probe 组合固定 JDBC、运行时、目录、空间与存储检查。
// 除 JDBC 的两项固定检查外，它不解析秘密、不启动进程，也不会把本地文件错误原文传出 Agent。
// EX-I6：StorageConnectivity/StorageAuth 只在对象存储输出任务上被编排器调用；
// 未装配或未授权时分别投影为受控 UNKNOWN，不把网络错误原文传给预检查结果。
type Probe struct {
	JDBC                  agentpreflight.Probe
	Runtime               RuntimeValidator
	MinimumAvailableBytes uint64
	AvailableBytes        func(string) (uint64, error)
	StorageConnectivity   StorageConnectivityProber
	StorageAuth           StorageAuthProber
	// StorageCredentials 只在明确启用凭据探测的 STORAGE_AUTH 检查中短时解析。
	// 默认失败关闭探测器不会调用它，避免尚未获授权的实现接触存储秘密。
	StorageCredentials agentpreflight.StorageCredentialResolver
}

// Probe 只接受既定检查。任何本机事实缺失、路径漂移或依赖异常都以稳定 UNKNOWN 结果失败关闭。
func (p Probe) Probe(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	if ctx == nil || ctx.Err() != nil {
		return unavailableResult(check), nil
	}
	switch check {
	case agentpreflight.CheckDatabaseConnectivity, agentpreflight.CheckObjectAccess:
		return p.jdbcResult(ctx, check, request)
	case agentpreflight.CheckToolEnvironment:
		return p.runtimeResult(ctx, check), nil
	case agentpreflight.CheckOutputPath:
		return p.outputPathResult(request), nil
	case agentpreflight.CheckOutputEmpty:
		return p.outputEmptyResult(request), nil
	case agentpreflight.CheckAvailableSpace:
		return p.availableSpaceResult(request), nil
	case agentpreflight.CheckStorageConnectivity:
		return p.storageConnectivityResult(ctx, request)
	case agentpreflight.CheckStorageAuth:
		return p.storageAuthResult(ctx, request)
	default:
		return agentpreflight.Result{}, agentpreflight.ErrInvalidRequest
	}
}

func (p Probe) jdbcResult(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	if p.JDBC == nil {
		return unavailableResult(check), nil
	}
	result, err := p.JDBC.Probe(ctx, check, request)
	if err != nil {
		return unavailableResult(check), nil
	}
	return result, nil
}

func (p Probe) runtimeResult(ctx context.Context, check agentpreflight.CheckID) agentpreflight.Result {
	if p.Runtime == nil {
		return unavailableResult(check)
	}
	err := p.Runtime.Validate(ctx)
	if err == nil {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "TOOL_RUNTIME_READY"}
	}
	if errors.Is(err, ErrToolRuntimeInvalid) {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusFailed, EvidenceCode: "TOOL_RUNTIME_INVALID"}
	}
	return unavailableResult(check)
}

func (p Probe) outputPathResult(request agentpreflight.Request) agentpreflight.Result {
	paths := []string{request.OutputPath}
	if request.LogPath != "" {
		paths = append(paths, request.LogPath)
	}
	for _, path := range paths {
		directory, err := secureDirectory(request, path)
		if err != nil {
			return agentpreflight.Result{Check: agentpreflight.CheckOutputPath, Status: agentpreflight.StatusFailed, EvidenceCode: "OUTPUT_PATH_NOT_WRITABLE"}
		}
		name, err := identifier.NewUUIDV4()
		if err != nil {
			return unavailableResult(agentpreflight.CheckOutputPath)
		}
		probeFile := filepath.Join(directory.probeDirectory, ".ob-data-orch-preflight-"+name)
		file, err := os.OpenFile(probeFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return agentpreflight.Result{Check: agentpreflight.CheckOutputPath, Status: agentpreflight.StatusFailed, EvidenceCode: "OUTPUT_PATH_NOT_WRITABLE"}
		}
		closeErr := file.Close()
		removeErr := os.Remove(probeFile)
		if closeErr != nil || removeErr != nil {
			return unavailableResult(agentpreflight.CheckOutputPath)
		}
	}
	return agentpreflight.Result{Check: agentpreflight.CheckOutputPath, Status: agentpreflight.StatusPassed, EvidenceCode: "OUTPUT_PATH_WRITABLE"}
}

func (p Probe) outputEmptyResult(request agentpreflight.Request) agentpreflight.Result {
	if request.SkipCheckDir {
		return agentpreflight.Result{Check: agentpreflight.CheckOutputEmpty, Status: agentpreflight.StatusPassed, EvidenceCode: "OUTPUT_EMPTY_CHECK_SKIPPED"}
	}
	directory, err := secureOutputDirectory(request)
	if err != nil {
		return unavailableResult(agentpreflight.CheckOutputEmpty)
	}
	// OBDUMPER 会在实际启动时创建尚不存在的导出目录；该目录在创建前必然为空，预检查不抢先创建它。
	if !directory.exists {
		return agentpreflight.Result{Check: agentpreflight.CheckOutputEmpty, Status: agentpreflight.StatusPassed, EvidenceCode: "OUTPUT_PATH_EMPTY"}
	}
	entries, err := os.ReadDir(directory.targetDirectory)
	if err != nil {
		return unavailableResult(agentpreflight.CheckOutputEmpty)
	}
	if len(entries) != 0 {
		return agentpreflight.Result{Check: agentpreflight.CheckOutputEmpty, Status: agentpreflight.StatusFailed, EvidenceCode: "OUTPUT_PATH_NOT_EMPTY"}
	}
	return agentpreflight.Result{Check: agentpreflight.CheckOutputEmpty, Status: agentpreflight.StatusPassed, EvidenceCode: "OUTPUT_PATH_EMPTY"}
}

func (p Probe) availableSpaceResult(request agentpreflight.Request) agentpreflight.Result {
	if p.MinimumAvailableBytes == 0 || p.AvailableBytes == nil {
		return unavailableResult(agentpreflight.CheckAvailableSpace)
	}
	// EX-I6：对象存储输出没有本地导出目录，空间检查转向 --tmp-path 所在卷；
	// 未指定 tmp-path 时无法定位卷，回报 UNKNOWN 失败关闭。
	var probePath string
	if request.OutputKind.IsStorageOutput() {
		if request.StorageTarget == nil || request.StorageTarget.TmpPath == "" {
			return unavailableResult(agentpreflight.CheckAvailableSpace)
		}
		directory, err := secureDirectory(request, request.StorageTarget.TmpPath)
		if err != nil {
			return unavailableResult(agentpreflight.CheckAvailableSpace)
		}
		probePath = directory.probeDirectory
	} else {
		directory, err := secureOutputDirectory(request)
		if err != nil {
			return unavailableResult(agentpreflight.CheckAvailableSpace)
		}
		probePath = directory.probeDirectory
	}
	available, err := p.AvailableBytes(probePath)
	if err != nil {
		return unavailableResult(agentpreflight.CheckAvailableSpace)
	}
	if available < p.MinimumAvailableBytes {
		return agentpreflight.Result{Check: agentpreflight.CheckAvailableSpace, Status: agentpreflight.StatusFailed, EvidenceCode: "OUTPUT_SPACE_INSUFFICIENT"}
	}
	return agentpreflight.Result{Check: agentpreflight.CheckAvailableSpace, Status: agentpreflight.StatusPassed, EvidenceCode: "OUTPUT_SPACE_SUFFICIENT"}
}

func secureOutputDirectory(request agentpreflight.Request) (directoryResolution, error) {
	return secureDirectory(request, request.OutputPath)
}

// directoryResolution 描述已校验的目标目录及可用于探针的现有目录。
// 目标目录不存在时，probeDirectory 是其最近的既有父目录；预检查绝不创建用户目录。
type directoryResolution struct {
	targetDirectory string
	probeDirectory  string
	exists          bool
}

// secureDirectory 以同一白名单和链接解析规则验证导出目录或用户指定的日志目录。
// 两类目录都必须属于节点允许根目录，不能因日志参数成为未受控文件写入入口。
// 对 OBDUMPER 将在实际执行中创建的目录，只在最近现有父目录验证可创建性，不改写传给工具的原始路径。
func secureDirectory(request agentpreflight.Request, path string) (directoryResolution, error) {
	if !validLocalPlatform(request.TargetPlatform) || path == "" || len(request.AllowedRoots) == 0 {
		return directoryResolution{}, errors.New("输出目录约束缺失")
	}
	localOutputPath, ok := outputpath.LocalFilesystemPath(string(request.TargetPlatform), path)
	if !ok {
		return directoryResolution{}, errors.New("输出目录格式无效")
	}
	localOutputPath = filepath.Clean(localOutputPath)
	if !targetPathWithinAllowedRoot(request, localOutputPath) {
		return directoryResolution{}, errors.New("输出目录不在允许根目录内")
	}
	if output, err := filepath.EvalSymlinks(localOutputPath); err == nil {
		info, statErr := os.Stat(output)
		if statErr != nil || !info.IsDir() {
			return directoryResolution{}, errors.New("输出目录不可用")
		}
		if !directoryWithinAllowedRoot(request, output) {
			return directoryResolution{}, errors.New("输出目录不在允许根目录内")
		}
		return directoryResolution{targetDirectory: output, probeDirectory: output, exists: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return directoryResolution{}, err
	}

	parent := filepath.Dir(localOutputPath)
	for {
		resolvedParent, err := filepath.EvalSymlinks(parent)
		if err == nil {
			info, statErr := os.Stat(resolvedParent)
			if statErr != nil || !info.IsDir() {
				return directoryResolution{}, errors.New("输出目录父目录不可用")
			}
			if !directoryWithinAllowedRoot(request, resolvedParent) {
				return directoryResolution{}, errors.New("输出目录不在允许根目录内")
			}
			return directoryResolution{targetDirectory: localOutputPath, probeDirectory: resolvedParent, exists: false}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return directoryResolution{}, err
		}
		parentDirectory := filepath.Dir(parent)
		if parentDirectory == parent {
			return directoryResolution{}, err
		}
		parent = parentDirectory
	}
}

// targetPathWithinAllowedRoot 先按规范路径验证不存在的目标目录没有越过白名单边界。
// 随后的 directoryWithinAllowedRoot 还会解析既有链接，避免链接把路径导向白名单之外。
func targetPathWithinAllowedRoot(request agentpreflight.Request, target string) bool {
	for _, root := range request.AllowedRoots {
		localRootPath, rootOK := outputpath.LocalFilesystemPath(string(request.TargetPlatform), root)
		if rootOK && pathWithinRoot(request.TargetPlatform, localRootPath, target) {
			return true
		}
	}
	return false
}

func directoryWithinAllowedRoot(request agentpreflight.Request, directory string) bool {
	for _, root := range request.AllowedRoots {
		localRootPath, rootOK := outputpath.LocalFilesystemPath(string(request.TargetPlatform), root)
		if !rootOK {
			continue
		}
		resolvedRoot, rootErr := filepath.EvalSymlinks(localRootPath)
		if rootErr != nil {
			continue
		}
		rootInfo, rootStatErr := os.Stat(resolvedRoot)
		if rootStatErr != nil || !rootInfo.IsDir() {
			continue
		}
		if pathWithinRoot(request.TargetPlatform, resolvedRoot, directory) {
			return true
		}
	}
	return false
}

func pathWithinRoot(platform commandgen.Platform, root, output string) bool {
	return outputpath.WithinAllowedRoot(string(platform), output, root)
}

func validLocalPlatform(platform commandgen.Platform) bool {
	return platform == commandgen.PlatformWindowsAMD64 || platform == commandgen.PlatformLinuxAMD64 || platform == commandgen.PlatformLinuxARM64
}

func obdumperLauncher(toolHome string, platform commandgen.Platform) (string, bool) {
	if !filepath.IsAbs(toolHome) {
		return "", false
	}
	switch platform {
	case commandgen.PlatformWindowsAMD64:
		return filepath.Join(toolHome, "bin", "windows", "obdumper.bat"), true
	case commandgen.PlatformLinuxAMD64, commandgen.PlatformLinuxARM64:
		return filepath.Join(toolHome, "bin", "obdumper"), true
	default:
		return "", false
	}
}

func unavailableResult(check agentpreflight.CheckID) agentpreflight.Result {
	switch check {
	case agentpreflight.CheckDatabaseConnectivity:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "DATABASE_CONNECTION_UNAVAILABLE"}
	case agentpreflight.CheckObjectAccess:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "OBJECT_ACCESS_UNAVAILABLE"}
	case agentpreflight.CheckToolEnvironment:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "TOOL_RUNTIME_UNAVAILABLE"}
	case agentpreflight.CheckOutputPath, agentpreflight.CheckOutputEmpty:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "OUTPUT_PATH_UNAVAILABLE"}
	case agentpreflight.CheckAvailableSpace:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "OUTPUT_SPACE_UNAVAILABLE"}
	case agentpreflight.CheckStorageConnectivity:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "STORAGE_CONNECTIVITY_UNAVAILABLE"}
	case agentpreflight.CheckStorageAuth:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "STORAGE_AUTH_UNAVAILABLE"}
	default:
		return agentpreflight.Result{}
	}
}
