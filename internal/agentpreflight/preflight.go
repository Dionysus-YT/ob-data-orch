// Package agentpreflight 提供 G2 阶段固定 EXPORT_PREFLIGHT 的纯本地编排。
// 它不解析凭据、不连接网络、不读取节点文件，也不具备启动任何进程的入口。
package agentpreflight

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
)

var (
	ErrInvalidRequest        = errors.New("预检查请求无效")
	ErrUnsupportedCapability = errors.New("预检查能力不受支持")
	ErrActiveExecution       = errors.New("Agent 存在活动执行，不能领取预检查")
	ErrProbeFailed           = errors.New("预检查合成事实不可用")
)

var evidenceCodePattern = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)

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

var fixedChecks = []CheckID{
	CheckDatabaseConnectivity,
	CheckObjectAccess,
	CheckToolEnvironment,
	CheckOutputPath,
	CheckOutputEmpty,
	CheckAvailableSpace,
}

// Request 是不可变预检查租约在 Agent 本地的非敏感投影。
// 其中不包含密码、SQL、Shell 文本、工具 argv 或可执行文件路径。
type Request struct {
	Capability      Capability
	PrecheckID      string
	NodeID          string
	AgentID         string
	LeaseID         string
	LeaseEpoch      int64
	Binding         agentstate.PrecheckBinding
	TargetPlatform  commandgen.Platform
	OutputPath      string
	ActiveExecution bool
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

// Run 执行唯一的固定检查清单。任何无效、未知或未完成结果都失败关闭。
func Run(ctx context.Context, request Request, probe Probe) (Report, error) {
	if err := validate(request, probe); err != nil {
		return Report{}, err
	}
	report := Report{PrecheckID: request.PrecheckID, Succeeded: true, Results: make([]Result, 0, len(fixedChecks))}
	for _, check := range fixedChecks {
		result, err := probe.Probe(ctx, check, request)
		if err != nil {
			return Report{}, ErrProbeFailed
		}
		if result.Check != check || !validStatus(result.Status) || !evidenceCodePattern.MatchString(result.EvidenceCode) {
			return Report{}, ErrInvalidRequest
		}
		report.Results = append(report.Results, result)
		if result.Status != StatusPassed {
			report.Succeeded = false
		}
	}
	return report, nil
}

// FixedChecks 返回固定清单的副本，避免外部修改影响检查范围或顺序。
func FixedChecks() []CheckID {
	return append([]CheckID(nil), fixedChecks...)
}

func validate(request Request, probe Probe) error {
	if probe == nil || request.Capability != CapabilityExportPreflight {
		if request.Capability != CapabilityExportPreflight {
			return ErrUnsupportedCapability
		}
		return ErrInvalidRequest
	}
	if request.ActiveExecution {
		return ErrActiveExecution
	}
	if blank(request.PrecheckID, request.NodeID, request.AgentID, request.LeaseID, request.OutputPath, request.Binding.PrecheckID, request.Binding.NodeID, request.Binding.ConfigFingerprint) || request.LeaseEpoch < 1 || request.Binding.DraftRevision < 1 || request.Binding.CredentialRevision < 1 || request.Binding.NodeFactsVersion < 1 {
		return ErrInvalidRequest
	}
	if request.PrecheckID != request.Binding.PrecheckID || request.NodeID != request.Binding.NodeID || !supportedPlatform(request.TargetPlatform) || !validAbsolutePath(request.TargetPlatform, request.OutputPath) {
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

func validAbsolutePath(platform commandgen.Platform, value string) bool {
	if strings.ContainsRune(value, 0) {
		return false
	}
	switch platform {
	case commandgen.PlatformWindowsAMD64:
		return len(value) >= 3 && isASCIILetter(value[0]) && value[1] == ':' && value[2] == '\\'
	case commandgen.PlatformLinuxAMD64, commandgen.PlatformLinuxARM64:
		return strings.HasPrefix(value, "/")
	default:
		return false
	}
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
