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

// DataSourceStateChange 只表达启用或禁用这一受控状态动作。
// 它不携带连接字段或凭据，因此不能被误用为通用数据源更新入口。
type DataSourceStateChange struct {
	DataSourceID   string
	ActorSubjectID string
	TargetState    string
	RequestID      string
	ChangedAt      time.Time
}

// DataSourceStateChangeResult 让重复状态动作返回当前事实，避免重复审计。
type DataSourceStateChangeResult struct {
	State    string
	Revision int64
	Replayed bool
}
