// Package logquery 提供有界任务日志快照查询，不接触 Agent 文件、原始正文或任务执行状态。
package logquery

import (
	"context"
	"errors"
	"time"

	"ob-data-orch/internal/logstream"
)

var (
	ErrInvalid     = errors.New("日志查询条件无效")
	ErrDenied      = errors.New("日志查询对象不可访问")
	ErrCursor      = errors.New("日志查询快照已失效，请重新查询")
	ErrLimit       = errors.New("日志下载超过限制，请缩小条件")
	ErrUnavailable = errors.New("日志暂时无法读取")
)

// Filter 只允许时间、精确任务与已接入通道，以及脱敏文本的字面包含。
type Filter struct {
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	TaskID  string    `json:"taskId"`
	Source  string    `json:"source"`
	Keyword string    `json:"keyword"`
	Limit   int       `json:"limit"`
}

// Scope 在当前授权内描述实际保留事实，绝不返回无权对象数量。
type Scope struct {
	Fingerprint string
	Watermark   int64
	Retained    bool
	Expired     bool
	Sources     []string
}

// Window 由已认证加密游标生成，不能直接接收浏览器提供的分段或文件定位。
type Window struct {
	SubjectID   string
	Fingerprint string
	Filter      Filter
	Watermark   int64
	BeforeKey   string
	Include     bool
	StreamID    string
	MinSeq      int64
	MaxSeq      int64
	AllRetained bool
}

// Candidate 是已在 SQLite 中按 TASK_READ 裁剪的内部索引，绝不直接序列化到响应。
type Candidate struct {
	Key    string
	TaskID string
	Batch  logstream.IndexedBatch
}

// Index 是授权、索引和下载审计的短事务边界，不存储全文或接受文件路径。
type Index interface {
	LogQueryScope(context.Context, string, Filter) (Scope, error)
	LogQueryBatches(context.Context, Window) ([]Candidate, error)
	AuditLogQuery(context.Context, Audit) error
}

// Reader 只读取受控索引对应的已登记正文。
type Reader interface {
	ReadIndexedBatch(context.Context, string, logstream.IndexedBatch) ([]logstream.Record, error)
}

// Audit 仅保留范围摘要，关键字使用摘要，不记录日志正文或用户原始关键字。
type Audit struct {
	SubjectID   string
	RequestID   string
	Action      string
	Result      string
	Filter      Filter
	Count       int
	At          time.Time
	Fingerprint string
}

// Item 保留实际接收时间、通道与来源序号；当前采集未提供可靠级别或来源时间。
type Item struct {
	Token         string    `json:"token"`
	TaskID        string    `json:"taskId"`
	Source        string    `json:"source"`
	StreamID      string    `json:"streamId"`
	Epoch         int64     `json:"epoch"`
	Sequence      int64     `json:"sequence"`
	Kind          string    `json:"kind"`
	Message       string    `json:"message"`
	IntegrityCode string    `json:"integrityCode"`
	ReceivedAt    time.Time `json:"receivedAt"`
}

// Page 是固定快照的一页；数量只表示实际返回量，不伪造总数或完整性。
type Page struct {
	Items          []Item    `json:"items"`
	NextCursor     string    `json:"nextCursor"`
	PageCursor     string    `json:"pageCursor"`
	SnapshotToken  string    `json:"snapshotToken"`
	SnapshotAt     time.Time `json:"snapshotAt"`
	Sources        []string  `json:"sources"`
	Availability   string    `json:"availability"`
	Expired        bool      `json:"expired"`
	Incomplete     bool      `json:"incomplete"`
	Truncated      bool      `json:"truncated"`
	ScannedRecords int       `json:"scannedRecords"`
	ScannedBytes   int64     `json:"scannedBytes"`
}

// Context 是同一来源和同一 epoch 的前后各至多 50 条，不受关键字筛选约束。
type Context struct {
	Items            []Item    `json:"items"`
	SelectedSequence int64     `json:"selectedSequence"`
	SnapshotAt       time.Time `json:"snapshotAt"`
	Incomplete       bool      `json:"incomplete"`
}
