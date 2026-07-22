// Package store implements the small, explicit SQLite write boundaries used by
// the first vertical slice. It deliberately does not expose a generic CRUD or
// a generic JSON persistence API.
package store

import (
	"errors"
	"time"
)

var (
	ErrRevisionConflict    = errors.New("stored revision no longer matches")
	ErrPrecheckInvalid     = errors.New("precheck is not valid for task submission")
	ErrAlreadyClaimed      = errors.New("task already has an execution")
	ErrClaimIneligible     = errors.New("task is not eligible for this agent")
	ErrEventRejected       = errors.New("execution event violates its lease or sequence")
	ErrDataSourceNotFound  = errors.New("data source does not exist")
	ErrIdempotencyConflict = errors.New("idempotency key was reused with different request")
)

type DraftUpdate struct {
	DraftID           string
	ExpectedRevision  int64
	ConfigJSON        string
	ConfigFingerprint string
	InvalidationJSON  string
	UpdatedAt         time.Time
}

type TaskSubmission struct {
	TaskID                 string
	CreatorSubjectID       string
	AuditActorID           string
	DataSourceID           string
	NodeID                 string
	PrecheckID             string
	CredentialID           string
	CredentialRevision     int64
	ConfigFingerprint      string
	ToolVersion            string
	MetadataVersion        string
	CapabilityVersion      string
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

// TaskSummary 是任务详情接口可返回的冻结非敏感投影。
// 它刻意不包含凭据引用、完整配置快照或未脱敏命令参数。
type TaskSummary struct {
	TaskID                 string
	CreatorSubjectID       string
	DataSourceID           string
	NodeID                 string
	PrecheckID             string
	ConfigFingerprint      string
	ToolVersion            string
	MetadataVersion        string
	CapabilityVersion      string
	PlannedCommandRedacted string
	State                  string
	ExecutionID            string
	SubmittedAt            time.Time
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

// DataSourceSummary is the non-sensitive projection available to API list and
// detail handlers. It intentionally excludes credential ciphertext, nonce,
// plaintext, and any field from which a password length can be inferred.
type DataSourceSummary struct {
	DataSourceID            string
	DisplayName             string
	Environment             string
	ConnectionKind          string
	CompatibilityMode       string
	Host                    string
	Port                    int
	Username                string
	DefaultDatabase         string
	State                   string
	Revision                int64
	CredentialRevision      int64
	LastTestStatus          string
	LastTestedAt            *time.Time
	LastTestSafeSummaryJSON string
	UpdatedAt               time.Time
}

// DataSourceCreate persists a new source and an already encrypted password
// envelope. Plaintext is deliberately not representable by this input type.
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
	Username          string
	DefaultDatabase   string
	KeyID             string
	Nonce             []byte
	Ciphertext        []byte
	RequestID         string
	IdempotencyKey    string
	RequestDigest     string
	CreatedAt         time.Time
}

// DataSourceCreateResult lets an HTTP adapter return the original resource for
// a safe idempotent retry without inserting a second credential revision.
type DataSourceCreateResult struct {
	DataSourceID string
	Replayed     bool
}

// DataSourceStateChange 只表达启用、禁用或归档这一受控状态动作。
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
	Username          string
	DefaultDatabase   string
	Password          *EncryptedDataSourcePassword
	RequestID         string
	UpdatedAt         time.Time
}

// DataSourceUpdateResult 只返回浏览器可安全得知的版本变化。
type DataSourceUpdateResult struct {
	Revision           int64
	CredentialRevision int64
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
	ConfigJSON        string
	ConfigFingerprint string
	InvalidationJSON  string
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	Status             string
	IntegrityStatus    string
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

// PrecheckCompletion 是经 Agent 租约校验后的结构化完成事实。
// Store 不接受租约本身；租约校验属于 Agent 状态协调器的职责。
type PrecheckCompletion struct {
	PrecheckID      string
	Succeeded       bool
	IntegrityStatus string
	ResultJSON      string
	CompletedAt     time.Time
}
