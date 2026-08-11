// Package store implements the small, explicit SQLite write boundaries used by
// the first vertical slice. It deliberately does not expose a generic CRUD or
// a generic JSON persistence API.
package store

import (
	"errors"
	"time"
)

var (
	ErrRevisionConflict                      = errors.New("stored revision no longer matches")
	ErrPrecheckInvalid                       = errors.New("precheck is not valid for task submission")
	ErrAlreadyClaimed                        = errors.New("task already has an execution")
	ErrClaimIneligible                       = errors.New("task is not eligible for this agent")
	ErrEventRejected                         = errors.New("execution event violates its lease or sequence")
	ErrDataSourceNotFound                    = errors.New("data source does not exist")
	ErrDataSourceConnectionTestRequired      = errors.New("data source requires a successful connection test")
	ErrDataSourceNameUnavailable             = errors.New("data source name is unavailable")
	ErrExecutionNodeNotFound                 = errors.New("execution node does not exist")
	ErrExecutionNodeNameUnavailable          = errors.New("execution node name is unavailable")
	ErrExecutionNodeEnvironmentCheckRequired = errors.New("execution node environment check is required")
	ErrExecutionNodeEnableRejected           = errors.New("execution node cannot be enabled")
	ErrExecutionNodeHasRunningTask           = errors.New("execution node has a running task")
	ErrEnrollmentRejected                    = errors.New("agent enrollment is rejected")
	ErrAgentAlreadyAssociated                = errors.New("execution node already has an active agent")
	ErrAgentAuthenticationFailed             = errors.New("agent authentication failed")
	ErrAgentHeartbeatRejected                = errors.New("agent heartbeat is rejected")
	ErrAgentHeartbeatConflict                = errors.New("agent heartbeat request conflicts with prior request")
	ErrPrecheckLeaseRejected                 = errors.New("precheck lease is rejected")
	ErrPrecheckLeaseExpired                  = errors.New("precheck lease has expired")
	ErrDataSourceConnectionTestInvalid       = errors.New("data source connection test is invalid")
	ErrDataSourceConnectionTestLeaseRejected = errors.New("data source connection test lease is rejected")
	ErrDataSourceConnectionTestLeaseExpired  = errors.New("data source connection test lease has expired")
	ErrIdempotencyConflict                   = errors.New("idempotency key was reused with different request")
)

// AuthSubject 是经认证系统确认后可被业务记录引用的最小身份投影。
// 它不包含会话、令牌、密码或任何可用于重新认证的材料。
type AuthSubject struct {
	SubjectID         string
	ExternalSubject   string
	DisplayName       string
	AccountStatus     string
	DirectoryRevision string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type DraftUpdate struct {
	DraftID           string
	ExpectedRevision  int64
	ConfigJSON        string
	ConfigFingerprint string
	InvalidationJSON  string
	UpdatedAt         time.Time
	// ConfigVersion 标记更新后草稿配置的版本（v5 或 v6）。
	ConfigVersion string
	// 结构化子配置 JSON，仅 v6 草稿写入非空值；v5 更新时重置为 '{}'。
	ObjectScopeJSON       string
	ContentSelectionJSON  string
	DataFormatJSON        string
	OutputConfigJSON      string
	PerformanceConfigJSON string
	FilterConfigJSON      string
	DDLBehaviorJSON       string
}

type TaskSubmission struct {
	TaskID             string
	CreatorSubjectID   string
	AuditActorID       string
	DataSourceID       string
	NodeID             string
	PrecheckID         string
	CredentialID       string
	CredentialRevision int64
	ConfigFingerprint  string
	ToolVersion        string
	MetadataVersion    string
	CapabilityVersion  string
	// SnapshotVersion 标记快照结构版本：v1 为原始扁平快照，v2 为泛化快照。
	SnapshotVersion        string
	SnapshotJSON           string
	PlannedArgvJSON        string
	PlannedCommandRedacted string
	RequestID              string
	SubmittedAt            time.Time
}

// TaskSubmissionResult 区分新建任务与同一幂等请求的安全重放。
type TaskSubmissionResult struct {
	TaskID   string
	Replayed bool
}

// TaskSummary 是任务详情各只读投影共用的冻结非敏感字段。
// HTTP 层必须按概览、快照、命令证据或执行事实再裁剪，不能把该内部结构整体返回。
type TaskSummary struct {
	TaskID            string
	CreatorSubjectID  string
	DataSourceID      string
	NodeID            string
	PrecheckID        string
	ConfigFingerprint string
	ToolVersion       string
	MetadataVersion   string
	CapabilityVersion string
	// SnapshotVersion 标记冻结快照的结构版本（v1 或 v2），只读投影不得修改它。
	SnapshotVersion        string
	Database               string
	Table                  string
	Format                 string
	PlannedCommandRedacted string
	State                  string
	ExecutionID            string
	ReconciliationRequired bool
	SubmittedAt            time.Time
	StartedAt              time.Time
	FinishedAt             time.Time
	UpdatedAt              time.Time
}

// TaskListQuery 是授权任务列表的固定游标查询。
// SubjectID 必须来自已认证浏览器身份；游标只改变排序位置，不能扩大对象范围。
type TaskListQuery struct {
	SubjectID       string
	BeforeSubmitted time.Time
	BeforeTaskID    string
	Limit           int
}

// TaskListItem 是任务中心列表可读取的最小安全投影。
// 它不包含任务配置、命令、凭据引用、日志正文、结果原文或错误原文。
type TaskListItem struct {
	TaskID                 string
	CreatorSubjectID       string
	DataSourceID           string
	NodeID                 string
	TaskType               string
	Database               string
	Table                  string
	State                  string
	ReconciliationRequired bool
	SubmittedAt            time.Time
	StartedAt              time.Time
	FinishedAt             time.Time
	UpdatedAt              time.Time
}

type Claim struct {
	ExecutionID string
	TaskID      string
	NodeID      string
	AgentID     string
	LeaseID     string
	LeaseEpoch  int64
	IssuedAt    time.Time
	ExpiresAt   time.Time
	EventID     string
	RequestID   string
}

// LeaseRenewal 是已认证 Agent 对现有领取租约的续期请求。
// 它不允许变更执行、节点、Agent 或 epoch，只能延长当前租约。
type LeaseRenewal struct {
	ExecutionID string
	LeaseID     string
	LeaseEpoch  int64
	AgentID     string
	ExpiresAt   time.Time
}

type ExecutionEvent struct {
	EventID     string
	ExecutionID string
	LeaseID     string
	LeaseEpoch  int64
	EventSeq    int64
	EventType   string
	PayloadJSON string
	ReceivedAt  time.Time
}

// ExecutionClaimNext 是受认证 Agent 领取本节点下一条冻结导出任务的最小输入。
// Agent 不能指定 taskId、参数、路径或凭据；这些事实均由控制面在短事务中重新读取。
type ExecutionClaimNext struct {
	AgentID             string
	NodeID              string
	ExecutionID         string
	LeaseID             string
	ScheduledEventID    string
	RequestID           string
	LeaseTTL            time.Duration
	HeartbeatFreshAfter time.Time
	Now                 time.Time
}

// ExecutionLeaseGrant 是控制面签发给已认证 Agent 的不可变任务信封安全投影。
// PlannedArgv 绝不包含密码；密码只能由后续受租约约束的唯一槽位提供。
type ExecutionLeaseGrant struct {
	TaskID            string
	ExecutionID       string
	LeaseID           string
	LeaseEpoch        int64
	ExpiresAt         time.Time
	EnvelopeDigest    string
	PlannedArgv       []string
	ConfigFingerprint string
	ToolVersion       string
	MetadataVersion   string
	CapabilityVersion string
}

// ExecutionSecretResolutionRequest 是正式导出在当前 execution 租约内解析数据库密码的受控输入。
// 不接受槽位名称以外的自由用途，也不允许浏览器或预检查租约复用该路径。
type ExecutionSecretResolutionRequest struct {
	AgentID        string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	RequestID      string
	RequestDigest  string
	Now            time.Time
}

// ExecutionSecretResolutionOutcome 记录控制面短时解密后的无秘密结果。
// 成功响应写出前必须复验 execution 租约、数据源和凭据状态，避免旧授权继续生效。
type ExecutionSecretResolutionOutcome struct {
	AgentID        string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	RequestID      string
	RequestDigest  string
	Succeeded      bool
	Now            time.Time
}

// EncryptedExecutionDatabaseConnection 是正式导出短时解密前的槽位材料。
// 它复用与预检查一致的销毁语义，但不允许用预检查的授权对象代替 execution 租约。
type EncryptedExecutionDatabaseConnection struct {
	Host         string
	Port         int
	Username     []byte
	DataSourceID string
	// OwnerSubjectID 与 NodeID 仅供控制面在解密前复验当前对象范围，绝不进入 Agent 响应。
	OwnerSubjectID string
	NodeID         string
	CredentialID   string
	Revision       int64
	KeyID          string
	Nonce          []byte
	Ciphertext     []byte
}

// Destroy 尽力清除执行槽位中的短时敏感字节。
func (c *EncryptedExecutionDatabaseConnection) Destroy() {
	if c == nil {
		return
	}
	for _, value := range [][]byte{c.Username, c.Nonce, c.Ciphertext} {
		for index := range value {
			value[index] = 0
		}
	}
	c.Username = nil
	c.Nonce = nil
	c.Ciphertext = nil
	c.OwnerSubjectID = ""
	c.NodeID = ""
}

// DataSourceSummary 是列表与详情 API 可返回的非敏感数据源投影。
// 它刻意排除凭据密文、nonce、明文及任何可推断密码长度的字段。
type DataSourceSummary struct {
	DataSourceID       string
	DisplayName        string
	Environment        string
	ConnectionKind     string
	CompatibilityMode  string
	Host               string
	Port               int
	ClusterName        string
	TenantName         string
	Username           string
	DefaultDatabase    string
	State              string
	Revision           int64
	CredentialRevision int64
	// SysUser 是可选的 sys 凭据账号（--sys-user）；SysCredentialRevision 大于 0 表示已配置 sys 凭据。
	// SysCredentialID 仅供受控写路径识别当前 sys 凭据版本，绝不进入浏览器响应投影。
	SysUser                 string
	SysCredentialID         string
	SysCredentialRevision   int64
	LastTestStatus          string
	LastTestedAt            *time.Time
	LastTestSafeSummaryJSON string
	LastTestSource          string
	UpdatedAt               time.Time
}

// ExecutionNodeSummary 是导出草稿选择节点时可返回的最小非敏感投影。
// 它不表示节点在线、路径可写或当前任务能够执行；这些事实只能由预检查确认。
type ExecutionNodeSummary struct {
	NodeID      string
	DisplayName string
	Platform    string
}

// ExecutionNode 是节点管理页面可读取的受控配置投影。
// Platform 与 AllowedRoots 是管理员声明的任务路由配置，不是 Agent 已验证的机器环境事实。
type ExecutionNode struct {
	NodeID          string
	DisplayName     string
	Platform        string
	ManagementState string
	AllowedRoots    []string
	// ToolHome 与 JavaPath 是节点管理员登记、首次 Agent 关联后由目标机器复核的本机运行时声明。
	// 它们不能由心跳、任务或后续控制面请求动态覆盖。
	ToolHome string
	JavaPath string
	// EnvironmentCheck 仅保存 Agent 固定本机运行时核验的安全结论；它不保存路径、命令、工具输出或原始错误。
	EnvironmentCheck ExecutionNodeEnvironmentCheck
	Agent            *ExecutionNodeAgent
	Revision         int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ExecutionNodeEnvironmentCheck 是当前节点固定环境检查的安全状态投影。
// 通过结论必须绑定当前 Agent 事实版本，Agent 重启或机器事实变化后会自动失效。
type ExecutionNodeEnvironmentCheck struct {
	CheckID       string
	Status        string
	Code          string
	FactsRevision int64
	RequestedAt   *time.Time
	CompletedAt   *time.Time
}

// ExecutionNodeAgent 是节点当前有效 Agent 的只读安全投影。
// 它不包含机器凭据、关联材料、主机地址、完整路径或任意 Agent 原始负载。
type ExecutionNodeAgent struct {
	AgentID          string
	ProtocolVersion  string
	BootID           string
	LastHeartbeatAt  *time.Time
	FactsRevision    int64
	EnvironmentFacts AgentEnvironmentFacts
	CapacityTotal    int
	CapacityUsed     int
}

// AgentEnvironmentFacts 是受认证 Agent 的受控环境摘要。
// 它只表达机器自报的运行时事实，不能替代工具、Java、目录和任务路径的固定环境检查。
type AgentEnvironmentFacts struct {
	OperatingSystem            string
	Architecture               string
	AgentVersion               string
	ObservedAt                 time.Time
	CPUUsagePercent            *int
	MemoryUsagePercent         *int
	RuntimeConfigurationDigest string
	DataRootUsages             []AgentDataRootUsage
}

// AgentDataRootUsage 是 Agent 对已固化导出目录的无路径空间采样。
// RootDigest 只能与当前节点声明配置匹配后才会映射为浏览器可见目录，避免 Agent 任意探测或泄露路径。
type AgentDataRootUsage struct {
	RootDigest     string
	TotalBytes     uint64
	AvailableBytes uint64
}

// AgentEnrollmentIssue 是节点管理员签发一次性关联材料时持久化的非敏感输入。
// 原始关联材料只在 HTTP 边界短时存在；仓储层只接收其单向摘要。
type AgentEnrollmentIssue struct {
	EnrollmentID string
	NodeID       string
	ActorID      string
	RequestID    string
	TokenDigest  []byte
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// AgentEnrollmentExchange 是 Agent 使用一次性材料关联机器身份的持久化输入。
// 两个摘要均由 HTTP 边界从短时字节缓冲生成，不能包含原始关联材料或机器凭据。
type AgentEnrollmentExchange struct {
	EnrollmentID             string
	NodeID                   string
	AgentID                  string
	RequestID                string
	ProtocolVersion          string
	EnrollmentMaterialDigest []byte
	CredentialDigest         []byte
	ExchangedAt              time.Time
}

// AgentEnrollmentResult 是关联结果的安全投影。
// Replayed 表示同一已消费材料和同一机器凭据的安全重试，没有再次消费关联材料。
type AgentEnrollmentResult struct {
	AgentID              string
	NodeID               string
	Replayed             bool
	RuntimeConfiguration AgentEnrollmentRuntimeConfiguration
}

// AgentEnrollmentRuntimeConfiguration 是首次关联时随受认证响应交给 Agent 的固定本机配置。
// 它来自节点管理员登记的声明，Agent 必须在本机复核后才能用于 JDBC 或 OBDUMPER，后续心跳不能覆盖它。
type AgentEnrollmentRuntimeConfiguration struct {
	Platform     string
	ToolHome     string
	JavaPath     string
	AllowedRoots []string
	Revision     int64
	Digest       string
}

// AgentIdentity 是通过机器凭据认证后的固定绑定关系。
// HTTP 适配器必须继续校验请求体中的 agentId 与 nodeId，不能只信任路径或请求体。
type AgentIdentity struct {
	AgentID         string
	NodeID          string
	ProtocolVersion string
}

// AgentHeartbeat 是 Agent 写入当前在线与环境事实的受控输入。
// 控制面接收时间由调用方传入，不能使用 Agent 的 sentAt 替代租约或在线判定时钟。
type AgentHeartbeat struct {
	AgentID         string
	NodeID          string
	ProtocolVersion string
	BootID          string
	RequestID       string
	ObservedAt      time.Time
	ReceivedAt      time.Time
	CapacityTotal   int
	CapacityUsed    int
	Facts           AgentEnvironmentFacts
}

// ExecutionNodeCreate 将节点配置、审计和幂等结果绑定在同一个短事务中。
// 新节点必须保持禁用，不能因浏览器写入而伪造 Agent 关联或环境检查结果。
type ExecutionNodeCreate struct {
	NodeID           string
	CreatorSubjectID string
	DisplayName      string
	NormalizedName   string
	Platform         string
	AllowedRoots     []string
	ToolHome         string
	JavaPath         string
	RequestID        string
	IdempotencyKey   string
	RequestDigest    string
	CreatedAt        time.Time
}

// ExecutionNodeCreateResult 区分首次写入和同一幂等请求的安全重放。
type ExecutionNodeCreateResult struct {
	NodeID   string
	Replayed bool
}

// ExecutionNodeUpdate 是版本保护的节点管理配置更新。
// 它不承载 Agent、工具、心跳、资源或环境检查字段，避免浏览器覆盖机器事实。
type ExecutionNodeUpdate struct {
	NodeID           string
	ActorSubjectID   string
	ExpectedRevision int64
	DisplayName      string
	NormalizedName   string
	Platform         string
	AllowedRoots     []string
	ToolHome         string
	JavaPath         string
	RequestID        string
	UpdatedAt        time.Time
}

// ExecutionNodeEnvironmentCheckRequest 由已授权节点管理员发起一次固定本机运行时核验。
// 浏览器不携带检查项、路径、命令或 Agent 身份，检查范围由 Agent 端固定实现。
type ExecutionNodeEnvironmentCheckRequest struct {
	NodeID           string
	ActorSubjectID   string
	ExpectedRevision int64
	CheckID          string
	RequestID        string
	RequestedAt      time.Time
}

// ExecutionNodeEnvironmentCheckRequestResult 表示已写入的固定检查请求版本。
type ExecutionNodeEnvironmentCheckRequestResult struct {
	CheckID  string
	Revision int64
}

// ExecutionNodeEnvironmentCheckRefresh 是控制面在当前 Agent 事实版本变化后自动重做固定运行时检查的输入。
// 它只由受认证心跳触发，保留节点原有的启用或禁用管理意图，不能替代管理员首次启用节点的显式操作。
type ExecutionNodeEnvironmentCheckRefresh struct {
	NodeID        string
	AgentID       string
	CheckID       string
	RequestID     string
	FactsRevision int64
	RequestedAt   time.Time
}

// PendingExecutionNodeEnvironmentCheck 是控制面仅向当前受认证 Agent 下发的固定检查标识。
// 它不包含路径、命令、秘密、连接参数或可扩展的检查清单。
type PendingExecutionNodeEnvironmentCheck struct {
	CheckID string
}

// AgentExecutionNodeEnvironmentCheckCompletion 是 Agent 对唯一固定运行时检查的安全回执。
// Code 只能是预先登记的稳定证据码，不能包含本机路径、命令输出或底层异常文本。
type AgentExecutionNodeEnvironmentCheckCompletion struct {
	NodeID        string
	AgentID       string
	CheckID       string
	FactsRevision int64
	Status        string
	Code          string
	RequestID     string
	CompletedAt   time.Time
}

// ExecutionNodeEnable 是节点管理员在当前固定环境检查通过后执行的受控状态转换。
// 存储层会再次校验当前 Agent、事实版本与心跳截止时间，不能仅信任页面投影。
type ExecutionNodeEnable struct {
	NodeID           string
	ActorSubjectID   string
	ExpectedRevision int64
	RequestID        string
	EnabledAt        time.Time
	OnlineAfter      time.Time
}

// ExecutionNodeDeletion 表达一次受版本保护的节点删除请求。
// 存储层依据 Agent、草稿、预检查、任务和连接测试等引用决定物理删除或归档，调用方不能预先指定结果。
type ExecutionNodeDeletion struct {
	NodeID           string
	ActorSubjectID   string
	ExpectedRevision int64
	RequestID        string
	DeletedAt        time.Time
}

// ExecutionNodeDeletionResult 只返回实际处置结果、归档后的新版本和机器身份撤销事实。
// 物理删除不再有可读取的节点版本，因此 Revision 为零；归档不会删除任务、预检查、连接测试或审计历史。
type ExecutionNodeDeletionResult struct {
	Outcome            string
	Revision           int64
	AgentAccessRevoked bool
}

// DataSourceCreate 持久化新数据源及已加密的密码信封。
// 该输入类型故意不能表达密码明文。
type DataSourceCreate struct {
	DataSourceID      string
	CredentialID      string
	CreatorSubjectID  string
	DisplayName       string
	NormalizedName    string
	Environment       string
	ConnectionKind    string
	CompatibilityMode string
	Host              string
	Port              int
	ClusterName       string
	TenantName        string
	Username          string
	DefaultDatabase   string
	KeyID             string
	Nonce             []byte
	Ciphertext        []byte
	// SysUser 与 SysPassword 是可选的 sys 凭据（参考 ODC 数据源高级设置）；
	// 两者要么同时提供（创建 SYS_PASSWORD 修订），要么同时为空（不配置 sys 凭据）。
	SysUser        string
	SysPassword    *EncryptedDataSourcePassword
	RequestID      string
	IdempotencyKey string
	RequestDigest  string
	CreatedAt      time.Time
}

// DataSourceCreateResult 让 HTTP 适配器在幂等重试时返回原始资源，
// 而不插入第二个凭据修订。
type DataSourceCreateResult struct {
	DataSourceID string
	Replayed     bool
}

// DataSourceStateChange 只表达启用或禁用这一受控状态动作。
// 它不携带连接字段或凭据，因此不能被误用为通用数据源更新入口。
type DataSourceStateChange struct {
	DataSourceID     string
	ActorSubjectID   string
	TargetState      string
	ExpectedRevision int64
	RequestID        string
	ChangedAt        time.Time
}

// DataSourceStateChangeResult 让重复状态动作返回当前事实，避免重复审计。
type DataSourceStateChangeResult struct {
	State    string
	Revision int64
	Replayed bool
}

// DataSourceDeletion 表达一次受版本保护的数据源删除请求。
// 仓储会依据历史引用决定实际物理删除或归档，调用方不能预先指定结果。
type DataSourceDeletion struct {
	DataSourceID     string
	ActorSubjectID   string
	ExpectedRevision int64
	RequestID        string
	DeletedAt        time.Time
}

// DataSourceDeletionResult 只返回实际处置结果和归档后的新版本。
// 物理删除不再有可读取的资源版本，因此 Revision 为零。
type DataSourceDeletionResult struct {
	Outcome  string
	Revision int64
}

// DataSourceCredentialReference 仅供受控写路径取得加密所需的版本引用。
// 它不是 API 响应模型，调用方不得将其序列化给浏览器。
type DataSourceCredentialReference struct {
	CredentialID string
	Revision     int64
}

// EncryptedDataSourcePassword 是一次密码轮换已完成的加密结果。
// 该类型刻意不能表达密码明文，避免仓储层成为明文秘密入口。
type EncryptedDataSourcePassword struct {
	CredentialID string
	Revision     int64
	KeyID        string
	Nonce        []byte
	Ciphertext   []byte
}

// DataSourceUpdate 是已合并字段与可选加密密码轮换的一次原子更新。
type DataSourceUpdate struct {
	DataSourceID      string
	ActorSubjectID    string
	ExpectedRevision  int64
	DisplayName       string
	NormalizedName    string
	Environment       string
	ConnectionKind    string
	CompatibilityMode string
	Host              string
	Port              int
	ClusterName       string
	TenantName        string
	Username          string
	DefaultDatabase   string
	Password          *EncryptedDataSourcePassword
	// SysUser 是合并后的 sys 账号值（空表示不配置）；SysPassword 非 nil 表示轮换/设置 sys 密码；
	// ClearSysCredential 为 true 时同时清除 sys 账号与 sys 凭据修订。
	SysUser            string
	SysPassword        *EncryptedDataSourcePassword
	ClearSysCredential bool
	RequestID          string
	UpdatedAt          time.Time
}

// DataSourceUpdateResult 返回安全的版本变化及旧连接测试结论是否失效。
// 连接测试失效时，仓储已在同一事务中将数据源降为 DISABLED。
type DataSourceUpdateResult struct {
	Revision                  int64
	CredentialRevision        int64
	ConnectionTestInvalidated bool
}

// ExportConfigTemplate 是从成功任务保存的导出配置模板。
// 可用于创建新草稿，复用已验证的配置结构。
type ExportConfigTemplate struct {
	TemplateID        string
	OwnerSubjectID    string
	DisplayName       string
	CapabilityVersion string
	ConfigJSON        string
	ConfigFingerprint string
	SourceTaskID      string
	Revision          int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ExportConfigTemplateCreate 将模板与幂等记录绑定在同一事务内。
type ExportConfigTemplateCreate struct {
	ExportConfigTemplate
	RequestID      string
	IdempotencyKey string
	RequestDigest  string
}

// ExportConfigTemplateCreateResult 为相同幂等请求提供稳定的模板标识。
type ExportConfigTemplateCreateResult struct {
	TemplateID string
	Replayed   bool
}

// ExportConfigTemplateUpdate 是版本保护的模板名称更新。
type ExportConfigTemplateUpdate struct {
	TemplateID       string
	ActorSubjectID   string
	ExpectedRevision int64
	DisplayName      string
	RequestID        string
	UpdatedAt        time.Time
}

// ExportConfig 是通用导出配置的结构化领域模型。
// 它不等同于前端表单 JSON 或 CLI argv，而是经过类型安全的独立抽象。
// json 标签与通用导出技术契约的 GeneralizedExportConfig 结构保持一致。
type ExportConfig struct {
	ObjectScope       ObjectScope       `json:"objectScope"`
	ContentSelection  ContentSelection  `json:"contentSelection"`
	DataFormat        DataFormat        `json:"dataFormat"`
	OutputConfig      OutputConfig      `json:"outputConfig"`
	PerformanceConfig PerformanceConfig `json:"performanceConfig"`
	FilterConfig      FilterConfig      `json:"filterConfig"`
	DDLBehavior       DDLBehavior       `json:"ddlBehavior"`
}

// ObjectScope 表达导出对象范围：全部对象或指定对象列表。
// Database 是对象所在数据库（--database），全部与指定范围均必填；
// 表达式的可选 schema 前缀只允许缺省或与 Database 一致，跨库未取证。
type ObjectScope struct {
	Database      string             `json:"database,omitempty"`
	ScopeKind     string             `json:"scopeKind"` // ALL | SPECIFIED
	ObjectTypes   []string           `json:"objectTypes"`
	Expressions   []ObjectExpression `json:"expressions"`
	ExcludeTables []string           `json:"excludeTables"`
}

// ObjectExpression 是单个对象表达式；Schema 为可选 schema（数据库）前缀，
// Name 为对象名称或通配表达式。RawInput 只允许由控制面按 schema.name 生成规范值，
// 浏览器提交的自由文本不得持久化，避免借此写入秘密或任意内容。
type ObjectExpression struct {
	Schema   string `json:"schema,omitempty"`
	Name     string `json:"name"`
	RawInput string `json:"rawInput,omitempty"`
}

// ContentSelection 表达导出内容类型。
type ContentSelection struct {
	ContentKind string `json:"contentKind"` // DDL_ONLY | DATA_ONLY | DDL_AND_DATA
}

// DataFormat 表达数据格式及专属序列化参数。
// CsvOptions 与 CutOptions 采用类型化结构而非自由 map，避免任意键值进入草稿与快照。
type DataFormat struct {
	FormatKind string     `json:"formatKind"` // CSV | CUT | SQL | POS | PARQUET | ORC | AVRO
	CsvOptions CsvOptions `json:"csvOptions,omitempty"`
	CutOptions CutOptions `json:"cutOptions,omitempty"`
}

// CsvOptions 是 EX-I3 已启用的 CSV 序列化参数集合；仅数据格式为 CSV 时参与活动，
// 其中的跨格式文本选项（转义字符、行分隔符、NULL 替换、文件编码、去除空格）
// 在 EX-I4 中随 FORMAT_IN 规则在 CUT/SQL 格式下同样活动；
// ColumnSplitter 为 CUT 专属列分隔字符串（POS 定版后解锁，2026-08-07）。
type CsvOptions struct {
	SkipHeader      bool   `json:"skipHeader,omitempty"`
	ColumnSeparator string `json:"columnSeparator,omitempty"`
	ColumnQuote     string `json:"columnQuote,omitempty"`
	ColumnQuoteMode string `json:"columnQuoteMode,omitempty"`
	EscapeCharacter string `json:"escapeCharacter,omitempty"`
	LineSeparator   string `json:"lineSeparator,omitempty"`
	NullString      string `json:"nullString,omitempty"`
	FileEncoding    string `json:"fileEncoding,omitempty"`
	WithTrim        bool   `json:"withTrim,omitempty"`
	ColumnSplitter  string `json:"columnSplitter,omitempty"`
}

// CutOptions 是 EX-I4 已启用的 CUT 序列化参数集合；仅数据格式为 CUT 时参与活动。
// RemoveNewline 会改变导出数据（高风险），二次确认机制由后续切片提供。
type CutOptions struct {
	TrailDelimiter bool `json:"trailDelimiter,omitempty"`
	RemoveNewline  bool `json:"removeNewline,omitempty"`
}

// OutputConfig 表达输出位置与文件布局。
// Compress 与 CompressionAlgo 是 EX-I3 启用的压缩选项；算法仅在启用压缩时有效。
// ControlFilePath 是 EX-I4 POS 定版（2026-08-07 实测）后的控制文件目录（--ctl-path），仅 POS 格式使用。
// TmpPath 是 EX-I6 对象存储（2026-08-07）的 Multipart 本地临时分块目录（--tmp-path）。
type OutputConfig struct {
	OutputKind       string `json:"outputKind"` // LOCAL | OSS | S3 | COS | OBS
	FilePath         string `json:"filePath"`
	LogPath          string `json:"logPath,omitempty"`
	SkipCheckDir     bool   `json:"skipCheckDir"`
	NoNestedDir      bool   `json:"noNestedDir"`
	MaxFileSize      *int64 `json:"maxFileSize,omitempty"`
	RetainEmptyFiles bool   `json:"retainEmptyFiles"`
	Compress         bool   `json:"compress,omitempty"`
	CompressionAlgo  string `json:"compressionAlgo,omitempty"`
	// CompressionLevel 是 EX-I7 压缩等级（2026-08-10）的 --compression-level：官方按算法分范围（zstd 1-22、zlib -1~9；gzip/snappy 不支持）。
	CompressionLevel *int64 `json:"compressionLevel,omitempty"`
	ControlFilePath  string `json:"controlFilePath,omitempty"`
	TmpPath          string `json:"tmpPath,omitempty"`
}

// PerformanceConfig 表达性能与资源参数。
type PerformanceConfig struct {
	Thread        *int   `json:"thread,omitempty"`
	PageSize      *int   `json:"pageSize,omitempty"`
	ParallelMacro *int   `json:"parallelMacro,omitempty"`
	FetchSize     *int   `json:"fetchSize,omitempty"`
	JvmMemory     string `json:"jvmMemory,omitempty"`
	Retry         bool   `json:"retry"`
	// BlockSize 是 EX-I7 文件拆分（2026-08-10）的 --block-size：数字（MB）或数字+MB/ROW 后缀，显式传值已受控实测。
	BlockSize string `json:"blockSize,omitempty"`
}

// FilterConfig 表达筛选与一致性参数。
type FilterConfig struct {
	QuerySql              string   `json:"querySql,omitempty"`
	Where                 string   `json:"where,omitempty"`
	Partition             string   `json:"partition,omitempty"`
	IncludeColumnNames    []string `json:"includeColumnNames,omitempty"`
	ExcludeColumnNames    []string `json:"excludeColumnNames,omitempty"`
	ExcludeDataTypes      []string `json:"excludeDataTypes,omitempty"`
	ExcludeVirtualColumns *bool    `json:"excludeVirtualColumns,omitempty"`
	EnableHiddenPk        *bool    `json:"enableHiddenPk,omitempty"`
	FlashbackScn          *int64   `json:"flashbackScn,omitempty"`
	FlashbackTimestamp    string   `json:"flashbackTimestamp,omitempty"`
	Snapshot              string   `json:"snapshot,omitempty"`
	WeakRead              *bool    `json:"weakRead,omitempty"`
}

// DDLBehavior 表达 DDL 行为参数。
type DDLBehavior struct {
	DropObject      *bool  `json:"dropObject,omitempty"`
	AddExtraMessage *bool  `json:"addExtraMessage,omitempty"`
	RetainSchema    *bool  `json:"retainSchema,omitempty"`
	CompactSchema   *bool  `json:"compactSchema,omitempty"`
	SequencePolicy  string `json:"sequencePolicy,omitempty"` // RESTART | PRESERVE
}

// SubmissionSnapshot 是泛化后的提交快照领域模型。
// 它替代当前 snapshot_json 中的硬编码结构，同时保持 v1 快照向后兼容。
type SubmissionSnapshot struct {
	ToolVersion       string
	MetadataVersion   string
	CapabilityVersion string
	DataSourceID      string
	NodeID            string
	PrecheckID        string
	ConfigFingerprint string
	ParentTaskID      string // 检查点继续
	DerivedFromTaskID string // 基于原配置新建
	TemplateID        string // 模板来源
}

// ExportDraft 是首条 CSV 导出链路可持久化的非敏感草稿投影。
// 它不保存密码、密文、秘密槽位解析结果或可执行进程信息。
type ExportDraft struct {
	DraftID           string
	OwnerSubjectID    string
	DataSourceID      string
	NodeID            string
	Revision          int64
	ToolVersion       string
	MetadataVersion   string
	CapabilityVersion string
	// ConfigVersion 标记 config_json 的结构版本：v5 扁平结构或 v6 泛化标准文档。
	ConfigVersion     string
	ConfigJSON        string
	ConfigFingerprint string
	InvalidationJSON  string
	// 结构化子配置 JSON，仅 v6 草稿持久化非空值；v5 草稿保持 '{}'。
	ObjectScopeJSON       string
	ContentSelectionJSON  string
	DataFormatJSON        string
	OutputConfigJSON      string
	PerformanceConfigJSON string
	FilterConfigJSON      string
	DDLBehaviorJSON       string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ExportDraftCreate 将草稿、审计与创建幂等记录绑定在同一事务内。
type ExportDraftCreate struct {
	ExportDraft
	RequestID      string
	IdempotencyKey string
	RequestDigest  string
}

// ExportDraftCreateResult 为相同幂等请求提供稳定的草稿标识。
type ExportDraftCreateResult struct {
	DraftID  string
	Replayed bool
}

// PrecheckRun 是提交前固定检查的非敏感绑定与状态投影。
type PrecheckRun struct {
	PrecheckID         string
	DraftID            string
	DraftRevision      int64
	ConfigFingerprint  string
	DataSourceID       string
	CredentialID       string
	CredentialRevision int64
	NodeID             string
	NodeFactsRevision  int64
	BindingAgentID     string
	BindingDigest      string
	Status             string
	IntegrityStatus    string
	Results            []PrecheckCheckResult
	ValidUntil         time.Time
	CreatedAt          time.Time
}

// PrecheckCreate 只创建固定 EXPORT_PREFLIGHT 绑定；它不含 Shell、SQL、路径浏览或秘密明文。
type PrecheckCreate struct {
	PrecheckRun
	CreatorSubjectID string
	RequestID        string
	IdempotencyKey   string
	RequestDigest    string
}

// PrecheckCreateResult 给出新建或幂等重放得到的预检查标识。
type PrecheckCreateResult struct {
	PrecheckID string
	Replayed   bool
}

// PrecheckBinding 是固定预检查领取、确认和完成共用的不可变非敏感绑定。
// 它只包含标识、版本和摘要，不能承载路径、SQL、命令或秘密原文。
type PrecheckBinding struct {
	PrecheckID         string
	DraftID            string
	DraftRevision      int64
	ConfigFingerprint  string
	DataSourceID       string
	CredentialID       string
	CredentialRevision int64
	NodeID             string
	NodeFactsRevision  int64
	BindingAgentID     string
	BindingDigest      string
	ValidUntil         time.Time
}

// PrecheckClaim 是受认证 Agent 对固定预检查短租约的领取请求。
// Now 必须来自控制面时钟，Agent 不能指定领取时间或篡改已冻结绑定。
type PrecheckClaim struct {
	AgentID       string
	NodeID        string
	PrecheckID    string
	LeaseID       string
	RequestID     string
	RequestDigest string
	LeaseTTL      time.Duration
	Now           time.Time
}

// PrecheckClaimNext 是受认证 Agent 请求服务端原子领取下一条固定预检查的输入。
// PrecheckID 不属于该模型；LeaseID 只能由控制面在收到请求后生成，不能由 Agent、命令行或本地环境指定。
type PrecheckClaimNext struct {
	AgentID       string
	NodeID        string
	LeaseID       string
	RequestID     string
	RequestDigest string
	LeaseTTL      time.Duration
	Now           time.Time
}

// PrecheckLeaseGrant 是持久化领取或确认重放后返回的短租约和冻结绑定。
type PrecheckLeaseGrant struct {
	PrecheckID string
	LeaseID    string
	LeaseEpoch int64
	ExpiresAt  time.Time
	Binding    PrecheckBinding
	// ExecutionContext 是由控制面从冻结草稿和节点声明中派生的固定本地检查上下文。
	// 它不承载 SQL、命令、秘密原文或浏览器可修改的运行参数。
	ExecutionContext PrecheckExecutionContext
	Replayed         bool
}

// PrecheckExecutionContext 是固定 EXPORT_PREFLIGHT 在 Agent 本地执行六项检查所需的最小非秘密输入。
// 所有字段都必须由当前冻结草稿和节点配置重新核验，Agent 不能用本地参数覆盖它们。
// Objects 是冻结对象清单：SPECIFIED 范围为名称列表，ALL 范围为空清单（按数据库级投影检查）。
type PrecheckExecutionContext struct {
	CompatibilityMode string
	Database          string
	Objects           []string
	ContentKind       string // DATA_ONLY | DDL_ONLY | DDL_AND_DATA
	OutputPath        string
	LogPath           string
	SkipCheckDir      bool
	TargetPlatform    string
	AllowedRoots      []string
}

// PrecheckSecretResolutionRequest 是 Agent 在已确认短租约内请求数据库连接槽位的受控输入。
// RequestDigest 只覆盖无秘密协议字段，用于同一请求的安全重放和异摘要冲突检测。
type PrecheckSecretResolutionRequest struct {
	AgentID       string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	RequestID     string
	RequestDigest string
	Now           time.Time
}

// EncryptedPrecheckDatabaseConnection 是短时解密前的数据库连接槽位材料。
// 它只在控制面内部传递已加密密码及冻结绑定的权限复核对象；调用方完成解密或失败处理后必须调用 Destroy。
type EncryptedPrecheckDatabaseConnection struct {
	Host string
	Port int
	// Username 是控制面按已登记字段短时组装的 username@tenant#cluster，不得持久化或记录日志。
	Username     []byte
	DataSourceID string
	// OwnerSubjectID 和 NodeID 只能用于解密前复验草稿所有者当前的数据源与节点权限，不能进入 Agent 响应或审计。
	OwnerSubjectID string
	NodeID         string
	CredentialID   string
	Revision       int64
	KeyID          string
	Nonce          []byte
	Ciphertext     []byte
}

// Destroy 尽力清除连接槽位中的敏感字节，避免它们在后续普通路径中继续存活。
func (c *EncryptedPrecheckDatabaseConnection) Destroy() {
	if c == nil {
		return
	}
	for _, value := range [][]byte{c.Username, c.Nonce, c.Ciphertext} {
		for index := range value {
			value[index] = 0
		}
	}
	c.Username = nil
	c.Nonce = nil
	c.Ciphertext = nil
	c.OwnerSubjectID = ""
	c.NodeID = ""
}

// PrecheckSecretResolutionOutcome 表示控制面在解密后记录的无秘密结果。
// 成功前会再次核验租约、绑定、数据源和凭据状态，避免旧授权在响应写出前继续生效。
type PrecheckSecretResolutionOutcome struct {
	AgentID       string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	RequestID     string
	RequestDigest string
	Succeeded     bool
	Now           time.Time
}

// PrecheckAcknowledgement 是 Agent 对已领取固定检查清单和租约的确认请求。
// 该输入仅允许引用冻结摘要，不能上传检查结果或任意运行参数。
type PrecheckAcknowledgement struct {
	AgentID       string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	RequestID     string
	RequestDigest string
	Now           time.Time
}

// PrecheckCompletion 是旧的合成协调器兼容完成事实。
// 正式持久化租约路径必须使用 AgentPrecheckCompletion，以免绕过租约校验。
type PrecheckCompletion struct {
	PrecheckID      string
	Succeeded       bool
	IntegrityStatus string
	ResultJSON      string
	CompletedAt     time.Time
}

// PrecheckCheckResult 是固定预检查清单中的单项非敏感结果。
// EvidenceCode 只能是稳定错误码，不能承载路径、SQL、命令、日志或秘密原文。
type PrecheckCheckResult struct {
	Check        string `json:"check"`
	Status       string `json:"status"`
	EvidenceCode string `json:"evidenceCode"`
}

// AgentPrecheckCompletion 是受认证 Agent 在当前短租约内提交的固定检查完成事实。
// 检查结果必须完整匹配固定清单；成功状态由 Store 根据每项结果推导，Agent 不能自行声明。
type AgentPrecheckCompletion struct {
	AgentID       string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	RequestID     string
	RequestDigest string
	Results       []PrecheckCheckResult
	Now           time.Time
}

// PrecheckCompletionResult 是持久化完成或同一请求重放后的安全状态投影。
type PrecheckCompletionResult struct {
	PrecheckID      string
	LeaseID         string
	LeaseEpoch      int64
	BindingDigest   string
	Status          string
	IntegrityStatus string
	CompletedAt     time.Time
	Replayed        bool
}

// DataSourceConnectionTestRun 是一次由指定执行节点 Agent 运行的基础连接测试安全投影。
// 它冻结连接配置、凭据修订、节点事实和 Agent 绑定，不包含密码、原始 JDBC 错误或连接字符串。
// Sys 字段是可选的 sys 凭据（--sys-user/--sys-password）验证事实，与数据库结果相互独立。
type DataSourceConnectionTestRun struct {
	ConnectionTestID       string
	DataSourceID           string
	CreatorSubjectID       string
	ConnectionConfigDigest string
	CredentialID           string
	CredentialRevision     int64
	NodeID                 string
	NodeFactsRevision      int64
	BindingAgentID         string
	BindingDigest          string
	Status                 string
	// VerificationSource 区分 G2 合成闭环与固定 JDBC 探针事实，防止合成结果被误作为真实验证。
	VerificationSource    string
	ResultCode            string
	SafeSummaryJSON       string
	SysCredentialID       string
	SysCredentialRevision int64
	SysVerificationStatus string
	SysResultCode         string
	ValidUntil            time.Time
	CreatedAt             time.Time
	CompletedAt           time.Time
}

// DataSourceConnectionTestCreate 只提交测试意图、选定节点和浏览器幂等信息。
// 连接摘要、凭据修订、Agent 和环境事实均由 Store 在同一短事务中从当前记录重新冻结。
type DataSourceConnectionTestCreate struct {
	ConnectionTestID           string
	DataSourceID               string
	CreatorSubjectID           string
	ExpectedDataSourceRevision int64
	NodeID                     string
	VerificationSource         string
	RequestID                  string
	IdempotencyKey             string
	RequestDigest              string
	CreatedAt                  time.Time
	HeartbeatFreshAfter        time.Time
	ValidUntil                 time.Time
}

// DataSourceConnectionTestCreateResult 区分新建测试与浏览器请求的安全重放。
type DataSourceConnectionTestCreateResult struct {
	ConnectionTestID string
	Replayed         bool
}

// DataSourceConnectionTestBinding 是领取、确认、秘密槽位解析和完成共用的不可变非敏感绑定。
// 它只表达受控数据源连接事实，不能承载 URL、SQL、命令、路径或秘密原文。
// SysCredentialID/SysCredentialRevision 是可选的 sys 凭据引用（大于 0 表示需要额外验证 sys 租户）。
type DataSourceConnectionTestBinding struct {
	ConnectionTestID       string
	DataSourceID           string
	ConnectionConfigDigest string
	CredentialID           string
	CredentialRevision     int64
	NodeID                 string
	NodeFactsRevision      int64
	BindingAgentID         string
	BindingDigest          string
	VerificationSource     string
	SysCredentialID        string
	SysCredentialRevision  int64
	ValidUntil             time.Time
}

// DataSourceConnectionTestClaimNext 是受认证 Agent 请求领取下一条匹配节点绑定测试的输入。
// 测试标识由控制面在事务内选择，LeaseID 由控制面生成后传入，Agent 不能指定二者。
type DataSourceConnectionTestClaimNext struct {
	AgentID       string
	NodeID        string
	LeaseID       string
	RequestID     string
	RequestDigest string
	LeaseTTL      time.Duration
	Now           time.Time
}

// DataSourceConnectionTestLeaseGrant 是已持久化短租约及冻结绑定的安全投影。
type DataSourceConnectionTestLeaseGrant struct {
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	ExpiresAt        time.Time
	Binding          DataSourceConnectionTestBinding
	Replayed         bool
}

// DataSourceConnectionTestAcknowledgement 只确认当前租约和绑定摘要。
// 它不接受 JDBC 结果、连接参数或秘密材料。
type DataSourceConnectionTestAcknowledgement struct {
	AgentID          string
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	BindingDigest    string
	RequestID        string
	RequestDigest    string
	Now              time.Time
}

// DataSourceConnectionTestSecretResolutionRequest 是 Agent 在已确认租约内请求唯一数据库连接槽位的受控输入。
// RequestDigest 只覆盖无秘密协议字段，用于同一请求的安全重放与异摘要冲突检测。
type DataSourceConnectionTestSecretResolutionRequest struct {
	AgentID          string
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	BindingDigest    string
	RequestID        string
	RequestDigest    string
	Now              time.Time
}

// EncryptedDataSourceConnectionTestDatabaseConnection 是控制面内部短时传递的加密连接材料。
// 调用方在解密完成或失败后必须调用 Destroy；此类型绝不表达密码明文。
type EncryptedDataSourceConnectionTestDatabaseConnection struct {
	Host string
	Port int
	// Username 是控制面按已登记字段短时组装的 username@tenant#cluster，不得持久化或记录日志。
	Username       []byte
	DataSourceID   string
	OwnerSubjectID string
	NodeID         string
	CredentialID   string
	Revision       int64
	KeyID          string
	Nonce          []byte
	Ciphertext     []byte
}

// Destroy 尽力清除短时连接槽位持有的敏感字节和授权上下文。
func (c *EncryptedDataSourceConnectionTestDatabaseConnection) Destroy() {
	if c == nil {
		return
	}
	for _, value := range [][]byte{c.Username, c.Nonce, c.Ciphertext} {
		for index := range value {
			value[index] = 0
		}
	}
	c.Username = nil
	c.Nonce = nil
	c.Ciphertext = nil
	c.OwnerSubjectID = ""
	c.NodeID = ""
}

// DataSourceConnectionTestSecretResolutionOutcome 记录短时解密结束时的无秘密结果。
// 成功写入前 Store 会重新校验租约、数据源、凭据、节点 Agent 与事实版本。
type DataSourceConnectionTestSecretResolutionOutcome struct {
	AgentID          string
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	BindingDigest    string
	RequestID        string
	RequestDigest    string
	Succeeded        bool
	Now              time.Time
}

// AgentDataSourceConnectionTestCompletion 是 Agent 在当前租约内提交的受控基础连接测试终态。
// 状态、证据码和来源均为固定枚举，不能传入 JDBC 原始异常、URL、用户名或其他自由文本。
// SysVerificationStatus/SysResultCode 只表达可选的 sys 凭据验证事实，与数据库结果相互独立。
type AgentDataSourceConnectionTestCompletion struct {
	AgentID               string
	ConnectionTestID      string
	LeaseID               string
	LeaseEpoch            int64
	BindingDigest         string
	RequestID             string
	RequestDigest         string
	Status                string
	EvidenceCode          string
	VerificationSource    string
	SysVerificationStatus string
	SysResultCode         string
	Now                   time.Time
}

// DataSourceConnectionTestCompletionResult 是完成或同一请求重放后可返回给受控协议层的安全状态投影。
type DataSourceConnectionTestCompletionResult struct {
	ConnectionTestID      string
	LeaseID               string
	LeaseEpoch            int64
	BindingDigest         string
	Status                string
	EvidenceCode          string
	VerificationSource    string
	SysVerificationStatus string
	SysResultCode         string
	CompletedAt           time.Time
	Replayed              bool
}
