package nodefacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ob-data-orch/internal/outputpath"
)

// HostInventory 是固定本机系统接口取得的机器摘要，不包含用户、进程列表或网络探测结果。
type HostInventory struct {
	Hostname      string   `json:"hostname,omitempty"`
	Addresses     []string `json:"addresses"`
	OSName        string   `json:"osName,omitempty"`
	OSVersion     string   `json:"osVersion,omitempty"`
	CPUModel      string   `json:"cpuModel,omitempty"`
	LogicalCPUs   int      `json:"logicalCpus"`
	PhysicalCores *int     `json:"physicalCores,omitempty"`
}

// InspectionItem 是固定清单的一项证据；路径只指向本机固化配置，错误只允许稳定码。
type InspectionItem struct {
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Code       string `json:"code"`
	Required   bool   `json:"required"`
	Path       string `json:"path,omitempty"`
	Version    string `json:"version,omitempty"`
	Exists     *bool  `json:"exists,omitempty"`
	Readable   *bool  `json:"readable,omitempty"`
	Writable   *bool  `json:"writable,omitempty"`
	Executable *bool  `json:"executable,omitempty"`
}

// DirectLoadConfiguration 只保留 4.3.5 已核对的三个数值键，其他内容不能进入协议。
// Fingerprint 只绑定这个安全投影，不对可能包含秘密的整个文件计算公开指纹。
type DirectLoadConfiguration struct {
	Status              string `json:"status"`
	TaskTimeoutMS       *int64 `json:"taskTimeoutMs,omitempty"`
	HeartbeatTimeoutMS  *int64 `json:"heartbeatTimeoutMs,omitempty"`
	HeartbeatIntervalMS *int64 `json:"heartbeatIntervalMs,omitempty"`
	HasUnrecognizedKeys bool   `json:"hasUnrecognizedKeys"`
	Fingerprint         string `json:"fingerprint,omitempty"`
}

// RuntimeInspection 是一次固定检查的有界完整清单，不能作为任意远程操作请求。
type RuntimeInspection struct {
	CheckID                    string                   `json:"checkId"`
	RuntimeConfigurationDigest string                   `json:"runtimeConfigurationDigest"`
	ObservedAt                 time.Time                `json:"observedAt"`
	VersionProbeAllowed        bool                     `json:"versionProbeAllowed"`
	Items                      []InspectionItem         `json:"items"`
	DirectLoad                 *DirectLoadConfiguration `json:"directLoad"`
}

// InspectionKinds 是当前通用节点检查唯一固定清单，不接受客户端增加项目。
var inspectionKinds = [...]string{"JAVA_RUNTIME", "OBDUMPER_RUNTIME", "OBLOADER_RUNTIME", "CONNECTOR", "WORKSPACE", "LOG_QUEUE", "TOOL_LOG", "DIRECT_LOAD_CONFIG", "RESOURCES", "RUNTIME_LAYOUT"}
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var supportedJavaVersionPattern = regexp.MustCompile(`^1\.8\.0_[0-9]+(?:[-+][0-9A-Za-z._+\-]+)?$`)

// InspectionKinds 返回固定清单的值副本，调用方不能改写协议允许集合。
func InspectionKinds() [10]string { return inspectionKinds }

// SupportedRuntimeVersion 固定当前 Java 8 与 OB Loader/Dumper 4.3.5 的版本边界。
// 发送端与控制面使用相同规则，自报 PASSED 不能扩大工具支持范围。
func SupportedRuntimeVersion(kind, version string) bool {
	if !ValidVersion(version) {
		return false
	}
	if kind == "JAVA_RUNTIME" {
		return supportedJavaVersionPattern.MatchString(version)
	}
	return (kind == "OBDUMPER_RUNTIME" || kind == "OBLOADER_RUNTIME") && (version == "4.3.5" || version == "4.3.5-RELEASE")
}

// ValidDiagnosticText 限制系统原生元信息为无控制字符的短文本。
func ValidDiagnosticText(value string, maximum int) bool {
	if len(value) > maximum || !utf8.ValidString(value) || value != strings.TrimSpace(value) {
		return false
	}
	for _, char := range value {
		if char < 32 || char == 127 {
			return false
		}
	}
	return true
}

// ValidateHost 在发送和持久化前拒绝非法地址、重复地址与不可靠计数。
func ValidateHost(host *HostInventory) error {
	if host == nil {
		return nil
	}
	invalid := errors.New("node host inventory is invalid")
	if !ValidDiagnosticText(host.Hostname, 253) || !ValidDiagnosticText(host.OSName, 128) || !ValidDiagnosticText(host.OSVersion, 128) || !ValidDiagnosticText(host.CPUModel, 256) || host.LogicalCPUs < 1 || host.LogicalCPUs > 65536 || host.Addresses == nil || len(host.Addresses) > 32 {
		return invalid
	}
	if host.PhysicalCores != nil && (*host.PhysicalCores < 1 || *host.PhysicalCores > 65536) {
		return invalid
	}
	seen := make(map[string]bool)
	for _, address := range host.Addresses {
		ip, err := netip.ParseAddr(address)
		if err != nil || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsLoopback() || seen[address] {
			return invalid
		}
		seen[address] = true
	}
	return nil
}

// DirectLoadFingerprint 对已确认数值投影生成确定性指纹，不包含原始配置、未知键或路径。
func DirectLoadFingerprint(configuration DirectLoadConfiguration) string {
	configuration.Fingerprint = ""
	content, _ := json.Marshal(configuration)
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

// ValidateInspection 复验清单、稳定码、路径和数值来源，结构缺失不能变成检查通过。
func ValidateInspection(inspection *RuntimeInspection) error {
	if inspection == nil {
		return nil
	}
	invalid := errors.New("node runtime inspection is invalid")
	if !idPattern.MatchString(inspection.CheckID) || !sha256Pattern.MatchString(inspection.RuntimeConfigurationDigest) || inspection.ObservedAt.IsZero() || len(inspection.Items) != len(inspectionKinds) || inspection.DirectLoad == nil {
		return invalid
	}
	seen := make(map[string]bool)
	for _, item := range inspection.Items {
		known := false
		for _, kind := range inspectionKinds {
			known = known || kind == item.Kind
		}
		if !known || seen[item.Kind] {
			return invalid
		}
		seen[item.Kind] = true
		if item.Status != "PASSED" && item.Status != "FAILED" && item.Status != "UNKNOWN" && item.Status != "NOT_APPLICABLE" {
			return invalid
		}
		switch item.Code {
		case "READY", "MISSING", "NOT_READABLE", "NOT_WRITABLE", "NOT_EXECUTABLE", "VERSION_UNSUPPORTED", "VERSION_UNAVAILABLE", "VERSION_PROBE_NOT_REQUESTED", "VERSION_PROBE_TIMEOUT", "VERSION_OUTPUT_INVALID", "CONFIG_INVALID", "NOT_CONFIGURED", "SAMPLING_UNAVAILABLE":
		default:
			return invalid
		}
		if item.Path != "" && (!ValidDiagnosticText(item.Path, 2048) || (!outputpath.IsAllowedRootPath("WINDOWS_AMD64", item.Path) && !outputpath.IsAllowedRootPath("LINUX_AMD64", item.Path))) {
			return invalid
		}
		if item.Version != "" && (!ValidVersion(item.Version) || !inspection.VersionProbeAllowed || (item.Kind != "JAVA_RUNTIME" && item.Kind != "OBDUMPER_RUNTIME" && item.Kind != "OBLOADER_RUNTIME")) {
			return invalid
		}
		if item.Status == "PASSED" {
			if item.Code != "READY" {
				return invalid
			}
			for _, fact := range []*bool{item.Exists, item.Readable, item.Writable, item.Executable} {
				if fact != nil && !*fact {
					return invalid
				}
			}
			if (item.Kind == "JAVA_RUNTIME" || item.Kind == "OBDUMPER_RUNTIME" || item.Kind == "OBLOADER_RUNTIME") && !SupportedRuntimeVersion(item.Kind, item.Version) {
				return invalid
			}
		}
		if item.Required != (item.Kind == "JAVA_RUNTIME" || item.Kind == "OBDUMPER_RUNTIME" || item.Kind == "CONNECTOR" || item.Kind == "WORKSPACE" || item.Kind == "LOG_QUEUE" || item.Kind == "RESOURCES" || item.Kind == "RUNTIME_LAYOUT") {
			return invalid
		}
	}
	config := inspection.DirectLoad
	if config.Status != "READ" && config.Status != "MISSING" && config.Status != "UNAVAILABLE" && config.Status != "INVALID" {
		return invalid
	}
	for _, value := range []*int64{config.TaskTimeoutMS, config.HeartbeatTimeoutMS, config.HeartbeatIntervalMS} {
		if value != nil && (*value < 0 || *value > 1<<53-1 || config.Status != "READ") {
			return invalid
		}
	}
	if config.Status == "READ" {
		if config.Fingerprint != DirectLoadFingerprint(*config) {
			return invalid
		}
	} else if config.Fingerprint != "" {
		return invalid
	}
	return nil
}

// InspectionResult 从必需项派生固定结论；可选 OBLOADER/Direct Load 不授予导入能力。
func InspectionResult(inspection *RuntimeInspection) (string, string) {
	if inspection == nil || ValidateInspection(inspection) != nil {
		return "FAILED", "TOOL_RUNTIME_UNAVAILABLE"
	}
	for _, item := range inspection.Items {
		if item.Required && item.Status == "FAILED" {
			return "FAILED", "TOOL_RUNTIME_INVALID"
		}
	}
	for _, item := range inspection.Items {
		if item.Required && item.Status != "PASSED" {
			return "FAILED", "TOOL_RUNTIME_UNAVAILABLE"
		}
	}
	return "PASSED", "TOOL_RUNTIME_READY"
}
