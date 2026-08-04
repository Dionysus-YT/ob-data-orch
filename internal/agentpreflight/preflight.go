// Package agentpreflight 提供 G2 阶段固定 EXPORT_PREFLIGHT 的纯本地编排。
// 它不解析凭据、不连接网络、不读取节点文件，也不具备启动任何进程的入口。
package agentpreflight

import (
	"context"
	"errors"
	"strings"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/outputpath"
	"ob-data-orch/internal/precheckcontract"
)

var (
	ErrInvalidRequest        = errors.New("预检查请求无效")
	ErrUnsupportedCapability = errors.New("预检查能力不受支持")
	ErrActiveExecution       = errors.New("Agent 存在活动执行，不能领取预检查")
	ErrProbeFailed           = errors.New("预检查合成事实不可用")
)

// Capability 表示 Agent 可以领取的固定本地能力，不能由调用方扩展为任意操作。
type Capability string

const (
	// CapabilityExportPreflight 是唯一允许在本包运行的固定检查，不会启动工具。
	CapabilityExportPreflight Capability = "EXPORT_PREFLIGHT"
)

// CheckID 表示固定检查清单中的一个项目。调用方不能提交自定义检查名称。
type CheckID string

const (
	CheckDatabaseConnectivity CheckID = "DATABASE_CONNECTIVITY"
	CheckObjectAccess         CheckID = "OBJECT_ACCESS"
	CheckToolEnvironment      CheckID = "TOOL_ENVIRONMENT"
	CheckOutputPath           CheckID = "OUTPUT_PATH"
	CheckOutputEmpty          CheckID = "OUTPUT_EMPTY"
	CheckAvailableSpace       CheckID = "AVAILABLE_SPACE"
)

var fixedChecks = fixedChecksFromContract()

// localChecks 是无需数据库凭据即可确认的本机前置条件。
// 它们必须在数据库与对象检查前完成，避免本机运行时、输出路径或空间已失败时仍解析秘密槽位。
var localChecks = []CheckID{
	CheckToolEnvironment,
	CheckOutputPath,
	CheckOutputEmpty,
	CheckAvailableSpace,
}

// databaseChecks 依赖受租约约束的短时数据库槽位，只能在全部本机前置条件通过后执行。
var databaseChecks = []CheckID{
	CheckDatabaseConnectivity,
	CheckObjectAccess,
}

// Request 是不可变预检查租约在 Agent 本地的非敏感投影。
// 其中不包含密码、SQL、Shell 文本、工具 argv 或可执行文件路径。
type Request struct {
	Capability        Capability
	PrecheckID        string
	NodeID            string
	AgentID           string
	LeaseID           string
	LeaseEpoch        int64
	Binding           agentstate.PrecheckBinding
	CompatibilityMode string
	Database          string
	Table             string
	TargetPlatform    commandgen.Platform
	OutputPath        string
	LogPath           string
	SkipCheckDir      bool
	AllowedRoots      []string
	ActiveExecution   bool
}

// Status 是固定检查的安全结论；未知不能被提升为通过。
type Status string

const (
	StatusPassed  Status = "PASSED"
	StatusFailed  Status = "FAILED"
	StatusUnknown Status = "UNKNOWN"
)

// Result 是单项检查的无秘密结果，只允许稳定原因码作为证据摘要。
type Result struct {
	Check        CheckID
	Status       Status
	EvidenceCode string
}

// Report 保持固定清单的顺序和全部结果，供后续协议适配使用。
type Report struct {
	PrecheckID string
	Succeeded  bool
	Results    []Result
}

// Probe 由 G2 合成夹具实现，用于为固定检查提供合成机器事实。
// 它不接收命令、秘密或独立的自由路径，且本包不会调用任何操作系统进程 API。
type Probe interface {
	Probe(context.Context, CheckID, Request) (Result, error)
}

// Run 执行唯一的固定检查清单。报告始终按固定契约顺序序列化，
// 但会先完成无秘密本机检查；任一本机前置失败时，数据库与对象结果保持未知且绝不调用 Probe 解析槽位。
func Run(ctx context.Context, request Request, probe Probe) (Report, error) {
	if err := validate(request, probe); err != nil {
		return Report{}, err
	}
	results := make(map[CheckID]Result, len(fixedChecks))
	localSucceeded := true
	for _, check := range localChecks {
		result, err := probeResult(ctx, probe, check, request)
		if err != nil {
			return Report{}, err
		}
		results[check] = result
		if result.Status != StatusPassed {
			localSucceeded = false
		}
	}
	if localSucceeded {
		for _, check := range databaseChecks {
			result, err := probeResult(ctx, probe, check, request)
			if err != nil {
				return Report{}, err
			}
			results[check] = result
		}
	} else {
		results[CheckDatabaseConnectivity] = unavailableDatabaseResult()
		results[CheckObjectAccess] = unavailableObjectResult()
	}
	report := Report{PrecheckID: request.PrecheckID, Succeeded: true, Results: make([]Result, 0, len(fixedChecks))}
	for _, check := range fixedChecks {
		result := results[check]
		report.Results = append(report.Results, result)
		if result.Status != StatusPassed {
			report.Succeeded = false
		}
	}
	return report, nil
}

// probeResult 统一复核单项本机事实，防止执行顺序变化后放宽既有结果边界。
func probeResult(ctx context.Context, probe Probe, check CheckID, request Request) (Result, error) {
	result, err := probe.Probe(ctx, check, request)
	if err != nil {
		return Result{}, ErrProbeFailed
	}
	if result.Check != check || !validStatus(result.Status) || !precheckcontract.ValidResult(string(result.Check), string(result.Status), result.EvidenceCode) {
		return Result{}, ErrInvalidRequest
	}
	return result, nil
}

// unavailableDatabaseResult 记录本机前置失败时未尝试数据库连接的安全事实。
func unavailableDatabaseResult() Result {
	return Result{Check: CheckDatabaseConnectivity, Status: StatusUnknown, EvidenceCode: "DATABASE_CONNECTION_UNAVAILABLE"}
}

// unavailableObjectResult 记录本机前置失败时未尝试对象读取的安全事实。
func unavailableObjectResult() Result {
	return Result{Check: CheckObjectAccess, Status: StatusUnknown, EvidenceCode: "OBJECT_ACCESS_UNAVAILABLE"}
}

// ValidateRequest 只复核固定预检查的非敏感本地投影。
// Worker 在 acknowledge 前调用它，只拒绝结构、绑定或平台边界错误；
// 输出目录是否位于允许根目录必须由本机 Probe 解析链接后回传固定失败结果，不能让租约自然过期。
func ValidateRequest(request Request) error {
	return validateRequest(request)
}

// ValidateReport 复核即将跨越 Agent 协议边界的固定检查报告。
// 它拒绝缺项、乱序、自定义检查、无效证据码及伪造的整体成功结论，避免 HTTP 适配器把报告退化为任意 JSON。
func ValidateReport(report Report) error {
	if strings.TrimSpace(report.PrecheckID) == "" || len(report.Results) != len(fixedChecks) {
		return ErrInvalidRequest
	}
	succeeded := true
	for index, check := range fixedChecks {
		result := report.Results[index]
		if result.Check != check || !validStatus(result.Status) || !precheckcontract.ValidResult(string(result.Check), string(result.Status), result.EvidenceCode) {
			return ErrInvalidRequest
		}
		if result.Status != StatusPassed {
			succeeded = false
		}
	}
	if report.Succeeded != succeeded {
		return ErrInvalidRequest
	}
	return nil
}

// FixedChecks 返回固定清单的副本，避免外部修改影响检查范围或顺序。
func FixedChecks() []CheckID {
	return append([]CheckID(nil), fixedChecks...)
}

func fixedChecksFromContract() []CheckID {
	checks := precheckcontract.FixedChecks()
	result := make([]CheckID, 0, len(checks))
	for _, check := range checks {
		result = append(result, CheckID(check))
	}
	return result
}

func validate(request Request, probe Probe) error {
	if probe == nil {
		return ErrInvalidRequest
	}
	return validateRequest(request)
}

func validateRequest(request Request) error {
	if request.Capability != CapabilityExportPreflight {
		return ErrUnsupportedCapability
	}
	if request.ActiveExecution {
		return ErrActiveExecution
	}
	if blank(request.PrecheckID, request.NodeID, request.AgentID, request.LeaseID, request.CompatibilityMode, request.Database, request.Table, request.OutputPath, request.Binding.PrecheckID, request.Binding.NodeID, request.Binding.ConfigFingerprint) || request.LeaseEpoch < 1 || request.Binding.DraftRevision < 1 || request.Binding.CredentialRevision < 1 || request.Binding.NodeFactsVersion < 1 {
		return ErrInvalidRequest
	}
	if request.CompatibilityMode != "MYSQL" && request.CompatibilityMode != "ORACLE" {
		return ErrInvalidRequest
	}
	if request.PrecheckID != request.Binding.PrecheckID || request.NodeID != request.Binding.NodeID || !supportedPlatform(request.TargetPlatform) || !validExportOutputPath(request.TargetPlatform, request.OutputPath) || (request.LogPath != "" && !validExportOutputPath(request.TargetPlatform, request.LogPath)) || !validAllowedRoots(request.TargetPlatform, request.AllowedRoots) {
		return ErrInvalidRequest
	}
	return nil
}

func blank(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func validStatus(status Status) bool {
	return status == StatusPassed || status == StatusFailed || status == StatusUnknown
}

func supportedPlatform(platform commandgen.Platform) bool {
	return platform == commandgen.PlatformWindowsAMD64 || platform == commandgen.PlatformLinuxAMD64 || platform == commandgen.PlatformLinuxARM64
}

func validExportOutputPath(platform commandgen.Platform, value string) bool {
	return outputpath.IsExportOutputPath(string(platform), value)
}

func validAllowedRoots(platform commandgen.Platform, roots []string) bool {
	if len(roots) == 0 || len(roots) > 32 {
		return false
	}
	for _, root := range roots {
		if !outputpath.IsAllowedRootPath(string(platform), root) {
			return false
		}
	}
	return true
}
