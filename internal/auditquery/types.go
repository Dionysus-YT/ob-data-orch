// Package auditquery 提供结构化审计的有界只读投影；不开放原始差异 JSON 或业务详情。
package auditquery

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("审计查询条件无效")
	ErrDenied      = errors.New("审计记录不可访问")
	ErrCursor      = errors.New("审计快照已失效")
	ErrLimit       = errors.New("审计下载超过限制")
	ErrUnavailable = errors.New("审计查询暂不可用")
)

// Filter 的精确标识只作为绑定参数；关键字仅匹配安全投影。
type Filter struct {
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	ActorID    string    `json:"actorId"`
	ObjectType string    `json:"objectType"`
	ObjectID   string    `json:"objectId"`
	Action     string    `json:"action"`
	Result     string    `json:"result"`
	Keyword    string    `json:"keyword"`
	Limit      int       `json:"limit"`
}

// Scope 在当前显式审计能力下返回权限指纹与固定追加水位，不返回事件数量。
type Scope struct {
	Fingerprint string
	Watermark   int64
}

// Window 只来自通过认证的服务端游标；每个读取事务仍复验当前授权。
type Window struct {
	Subject     string
	Fingerprint string
	Filter      Filter
	Watermark   int64
	Before      string
	EventID     string
	Limit       int
}

// Event 只在内部传递已授权事件；Diff 不得直接序列化或参与关键字匹配。
type Event struct {
	Key        string
	ID         string
	ActorType  string
	ActorID    string
	Action     string
	ObjectType string
	ObjectID   string
	Result     string
	RequestID  string
	At         time.Time
	Diff       []byte
	Bytes      int64
}

// Access 保存访问审计所需的范围摘要，不保存关键字、正文、令牌或差异原文。
type Access struct {
	Subject     string
	Fingerprint string
	RequestID   string
	Action      string
	Result      string
	Filter      Filter
	Count       int
	At          time.Time
}

// Store 是当前授权、短事务读取和访问审计的边界。
type Store interface {
	AuditQueryAccess(context.Context, string) (bool, error)
	AuditQueryScope(context.Context, string) (Scope, error)
	AuditQueryRows(context.Context, Window) ([]Event, error)
	AuditQueryAccessEvent(context.Context, Access) error
}

// Field 只允许展示固定字段的布尔或整数事实，不承载任意字符串及嵌套结构。
type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Item 保留真实平台审计来源；没有采集级别或第二个时间时不伪造。
type Item struct {
	Token          string    `json:"token"`
	ID             string    `json:"id"`
	ActorType      string    `json:"actorType"`
	ActorID        string    `json:"actorId"`
	Action         string    `json:"action"`
	ObjectType     string    `json:"objectType"`
	ObjectID       string    `json:"objectId"`
	Result         string    `json:"result"`
	RequestID      string    `json:"requestId"`
	OccurredAt     time.Time `json:"occurredAt"`
	Fields         []Field   `json:"fields"`
	DetailsOmitted bool      `json:"detailsOmitted"`
}

// Page 不宣称全库总数；扫描预算跨分页累计，下载固定到同一水位。
type Page struct {
	Items          []Item    `json:"items"`
	NextCursor     string    `json:"nextCursor"`
	PageCursor     string    `json:"pageCursor"`
	SnapshotToken  string    `json:"snapshotToken"`
	SnapshotAt     time.Time `json:"snapshotAt"`
	ScannedRecords int       `json:"scannedRecords"`
	ScannedBytes   int64     `json:"scannedBytes"`
	Truncated      bool      `json:"truncated"`
}
