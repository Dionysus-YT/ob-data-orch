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
	// EX-I6 存储专用预检查（2026-08-14）：仅对象存储输出任务的两项存储层检查。
	// 真实网络/凭据探测需显式授权（EX-V1）；默认 UNKNOWN，提交门禁按结果失败关闭。
	CheckStorageConnectivity CheckID = "STORAGE_CONNECTIVITY"
	CheckStorageAuth         CheckID = "STORAGE_AUTH"
)

var fixedChecks = fixedChecksFromContract()

// storageCheckList 是对象存储输出任务的检查清单：
// 本机检查裁剪掉不适用 URI 输出的 OUTPUT_PATH/OUTPUT_EMPTY，空间检查转向 --tmp-path 卷，
// 最后追加两项存储层检查；顺序固定，任何一项非 PASSED 都使整体失败。
var storageCheckList = []CheckID{
	CheckDatabaseConnectivity,
	CheckObjectAccess,
	CheckToolEnvironment,
	CheckAvailableSpace,
	CheckStorageConnectivity,
	CheckStorageAuth,
}

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

// OutputKind 表示预检查绑定的输出类型；只接受本地与四种受控对象存储。
type OutputKind string

const (
	OutputKindLocal OutputKind = "LOCAL"
	OutputKindOSS   OutputKind = "OSS"
	OutputKindS3    OutputKind = "S3"
	OutputKindCOS   OutputKind = "COS"
	OutputKindOBS   OutputKind = "OBS"
)

// ValidOutputKind 只接受受控输出类型枚举。
func ValidOutputKind(kind OutputKind) bool {
	switch kind {
	case OutputKindLocal, OutputKindOSS, OutputKindS3, OutputKindCOS, OutputKindOBS:
		return true
	default:
		return false
	}
}

// IsStorageOutput 判断输出类型是否为对象存储。
func (kind OutputKind) IsStorageOutput() bool {
	return kind == OutputKindOSS || kind == OutputKindS3 || kind == OutputKindCOS || kind == OutputKindOBS
}

// StorageTarget 是对象存储输出任务的最小非秘密预检查输入。
// URI 是受控存储 URI（无密钥参数）；Endpoint 由控制面从 URI 参数解析（可能为空，空表示无法确定探测目标）；
// TmpPath 是 --tmp-path 本地临时分块目录（可空，空表示空间检查无法定位卷）。
type StorageTarget struct {
	Provider string
	URI      string
	Endpoint string
	TmpPath  string
}

// StorageCredential 是仅供 STORAGE_AUTH 在内存中使用的短时对象存储凭据。
// 它不进入预检查请求、报告、日志、状态文件或任务快照；调用方使用完毕必须调用 Destroy。
type StorageCredential struct {
	Provider  string
	AccessKey []byte
	SecretKey []byte
}

// Destroy 尽力清除短时对象存储凭据字节，避免其继续被后续 Agent 逻辑持有。
func (c *StorageCredential) Destroy() {
	if c == nil {
		return
	}
	for _, value := range [][]byte{c.AccessKey, c.SecretKey} {
		for index := range value {
			value[index] = 0
		}
	}
	c.AccessKey = nil
	c.SecretKey = nil
}

// StorageCredentialResolver 把对象存储凭据解析限制在当前预检查绑定的短租约内。
// 具体实现必须拒绝过期、绑定漂移或非 STORAGE_AUTH 调用，不能缓存或持久化明文。
type StorageCredentialResolver interface {
	ResolveStorageCredential(context.Context, agentstate.PrecheckBinding) (StorageCredential, error)
}

// Request 是不可变预检查租约在 Agent 本地的非敏感投影。
// 其中不包含密码、SQL、Shell 文本、工具 argv 或可执行文件路径。
// Objects 为冻结对象清单；ALL 范围为空清单，对象检查按数据库级投影执行。
// EX-I6：对象存储输出任务通过 StorageTarget 携带受控 URI 与临时目录；本地输出为 nil。
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
	Objects           []string
	ObjectTypes       []string
	ContentKind       string
	TargetPlatform    commandgen.Platform
	OutputPath        string
	LogPath           string
	SkipCheckDir      bool
	AllowedRoots      []string
	ActiveExecution   bool
	// EX-I6：输出类型与存储目标；本地输出保持 StorageTarget 为空。
	OutputKind    OutputKind
	StorageTarget *StorageTarget
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

// Run 执行按输出类型确定的固定检查清单。报告始终按契约顺序序列化，
// 但会先完成无秘密本机检查；任一本机前置失败时，数据库与存储结果保持未知且绝不调用 Probe 解析槽位。
func Run(ctx context.Context, request Request, probe Probe) (Report, error) {
	if err := validate(request, probe); err != nil {
		return Report{}, err
	}
	checks := ChecksForOutputKind(request.OutputKind)
	results := make(map[CheckID]Result, len(checks))
	localSucceeded := true
	for _, check := range localCheckList(request.OutputKind) {
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
		if request.OutputKind.IsStorageOutput() {
			// 存储探测只能建立在数据库连接和对象访问均已通过的事实上。
			// 连接或对象检查未通过时，不能再连接端点、更不能解析存储凭据。
			if results[CheckDatabaseConnectivity].Status != StatusPassed || results[CheckObjectAccess].Status != StatusPassed {
				for _, check := range storageCheckOrder() {
					results[check] = unavailableStorageResult(check)
				}
			} else {
				connectivity, err := probeResult(ctx, probe, CheckStorageConnectivity, request)
				if err != nil {
					return Report{}, err
				}
				results[CheckStorageConnectivity] = connectivity
				if connectivity.Status != StatusPassed {
					results[CheckStorageAuth] = unavailableStorageResult(CheckStorageAuth)
				} else {
					auth, err := probeResult(ctx, probe, CheckStorageAuth, request)
					if err != nil {
						return Report{}, err
					}
					results[CheckStorageAuth] = auth
				}
			}
		}
	} else {
		results[CheckDatabaseConnectivity] = unavailableDatabaseResult()
		results[CheckObjectAccess] = unavailableObjectResult()
		if request.OutputKind.IsStorageOutput() {
			for _, check := range storageCheckOrder() {
				results[check] = unavailableStorageResult(check)
			}
		}
	}
	report := Report{PrecheckID: request.PrecheckID, Succeeded: true, Results: make([]Result, 0, len(checks))}
	for _, check := range checks {
		result := results[check]
		report.Results = append(report.Results, result)
		if result.Status != StatusPassed {
			report.Succeeded = false
		}
	}
	return report, nil
}

// localCheckList 按输出类型返回本机前置清单：
// 对象存储输出不适用本地 OUTPUT_PATH/OUTPUT_EMPTY，空间检查转向 --tmp-path 卷。
func localCheckList(kind OutputKind) []CheckID {
	if kind.IsStorageOutput() {
		return []CheckID{CheckToolEnvironment, CheckAvailableSpace}
	}
	return localChecks
}

// storageCheckOrder 返回存储层检查的固定执行顺序（先网络后凭据）。
func storageCheckOrder() []CheckID {
	return []CheckID{CheckStorageConnectivity, CheckStorageAuth}
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

// unavailableStorageResult 记录前置失败或探测未授权时存储层检查的固定未知结论。
func unavailableStorageResult(check CheckID) Result {
	code := "STORAGE_CONNECTIVITY_UNAVAILABLE"
	if check == CheckStorageAuth {
		code = "STORAGE_AUTH_UNAVAILABLE"
	}
	return Result{Check: check, Status: StatusUnknown, EvidenceCode: code}
}

// ValidateRequest 只复核固定预检查的非敏感本地投影。
// Worker 在 acknowledge 前调用它，只拒绝结构、绑定或平台边界错误；
// 输出目录是否位于允许根目录必须由本机 Probe 解析链接后回传固定失败结果，不能让租约自然过期。
func ValidateRequest(request Request) error {
	return validateRequest(request)
}

// ChecksForOutputKind 返回该输出类型的固定检查清单副本；本地输出保持冻结六项。
func ChecksForOutputKind(kind OutputKind) []CheckID {
	if kind.IsStorageOutput() {
		return append([]CheckID(nil), storageCheckList...)
	}
	return append([]CheckID(nil), fixedChecks...)
}

// ValidateReportFor 按输出类型复核即将跨越 Agent 协议边界的检查报告。
// 它拒绝缺项、乱序、自定义检查、无效证据码及伪造的整体成功结论，避免 HTTP 适配器把报告退化为任意 JSON。
func ValidateReportFor(kind OutputKind, report Report) error {
	checks := ChecksForOutputKind(kind)
	if strings.TrimSpace(report.PrecheckID) == "" || len(report.Results) != len(checks) {
		return ErrInvalidRequest
	}
	succeeded := true
	for index, check := range checks {
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

// ValidateReport 保持冻结六项报告的校验语义（本地输出）。
func ValidateReport(report Report) error {
	return ValidateReportFor(OutputKindLocal, report)
}

// FixedChecks 返回冻结六项清单的副本，避免外部修改影响检查范围或顺序。
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
	// 兼容既有本地请求：缺省输出类型按 LOCAL 处理，不改变冻结六项语义。
	if request.OutputKind == "" {
		request.OutputKind = OutputKindLocal
	}
	if request.Capability != CapabilityExportPreflight {
		return ErrUnsupportedCapability
	}
	if request.ActiveExecution {
		return ErrActiveExecution
	}
	if blank(request.PrecheckID, request.NodeID, request.AgentID, request.LeaseID, request.CompatibilityMode, request.Database, request.ContentKind, request.OutputPath, request.Binding.PrecheckID, request.Binding.NodeID, request.Binding.ConfigFingerprint) || request.LeaseEpoch < 1 || request.Binding.DraftRevision < 1 || request.Binding.CredentialRevision < 1 || request.Binding.NodeFactsVersion < 1 {
		return ErrInvalidRequest
	}
	if request.CompatibilityMode != "MYSQL" && request.CompatibilityMode != "ORACLE" {
		return ErrInvalidRequest
	}
	if request.ContentKind != "DATA_ONLY" && request.ContentKind != "DDL_ONLY" && request.ContentKind != "DDL_AND_DATA" {
		return ErrInvalidRequest
	}
	// ALL 范围允许空对象清单；SPECIFIED 范围逐项验证冻结名称。
	for _, object := range request.Objects {
		if blank(object) || len(object) > 256 || strings.ContainsAny(object, "\x00\r\n") {
			return ErrInvalidRequest
		}
	}
	if len(request.ObjectTypes) != 0 && len(request.ObjectTypes) != len(request.Objects) {
		return ErrInvalidRequest
	}
	for _, objectType := range request.ObjectTypes {
		if objectType != "TABLE" && objectType != "VIEW" && objectType != "FUNCTION" && objectType != "PROCEDURE" && objectType != "SEQUENCE" {
			return ErrInvalidRequest
		}
		if objectType != "TABLE" && request.ContentKind == "DATA_ONLY" {
			return ErrInvalidRequest
		}
	}
	if request.PrecheckID != request.Binding.PrecheckID || request.NodeID != request.Binding.NodeID || !supportedPlatform(request.TargetPlatform) || !validAllowedRoots(request.TargetPlatform, request.AllowedRoots) {
		return ErrInvalidRequest
	}
	if !ValidOutputKind(request.OutputKind) {
		return ErrInvalidRequest
	}
	if request.OutputKind.IsStorageOutput() {
		// EX-I6：对象存储输出的路径字段承载受控 URI（非本地目录）；本地 OUTPUT_PATH 语义不适用。
		// StorageTarget.URI 必须与控制面下发的 OutputPath 完全一致，防止目标段被替换为另一条 URI。
		if request.StorageTarget == nil || request.StorageTarget.URI != request.OutputPath || !validStorageTarget(request, request.OutputKind, request.StorageTarget) || !validStorageURI(request.OutputPath) {
			return ErrInvalidRequest
		}
		// 对象存储任务不携带本地日志路径（向导只对本地输出收集日志路径）。
		if request.LogPath != "" {
			return ErrInvalidRequest
		}
	} else {
		if request.StorageTarget != nil || !validExportOutputPath(request.TargetPlatform, request.OutputPath) || (request.LogPath != "" && !validExportOutputPath(request.TargetPlatform, request.LogPath)) {
			return ErrInvalidRequest
		}
	}
	return nil
}

// validStorageTarget 只接受与输出类型一致的受控存储目标。
// Endpoint 可空（空表示无法确定探测目标，STORAGE_CONNECTIVITY 将回报 UNKNOWN）；
// TmpPath 非空时必须符合目标平台本机绝对路径形态。
func validStorageTarget(request Request, kind OutputKind, target *StorageTarget) bool {
	if target == nil || target.Provider != string(kind) || !validStorageURI(target.URI) {
		return false
	}
	if target.Endpoint != "" && !validStorageURI(target.Endpoint) {
		return false
	}
	if target.TmpPath != "" && !validExportOutputPath(request.TargetPlatform, target.TmpPath) {
		return false
	}
	return true
}

// validStorageURI 只做结构失败关闭：非空、长度上限且不含控制字符。
// 受控 URI 的 scheme/参数白名单由控制面在草稿创建时校验，Agent 不重新解释 URI 语义。
func validStorageURI(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\x00\r\n")
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
