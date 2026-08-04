// Package logstream 提供 Agent 与控制面共享的日志记录、脱敏和批次纯核心。
// 本包不自行读取工具日志、不启动进程，也不接受未完成脱敏的内容持久化。
package logstream

import (
	"context"
	"errors"
	"time"
)

const (
	Mask = "******"
	// MaxRecordBytes 限制单条脱敏正文，超限内容不能写入控制面。
	MaxRecordBytes = 256 * 1024
	// MaxBatchRecords 与 MaxBatchBytes 同时限制单次上传，避免段读取无法受控地放大。
	MaxBatchRecords = 500
	MaxBatchBytes   = 512 * 1024
)

var (
	ErrInvalidInput   = errors.New("invalid log stream input")
	ErrRecordTooLarge = errors.New("log record exceeds safe limit")
	ErrBatchTooLarge  = errors.New("log batch exceeds safe limit")
	ErrPolicyRejected = errors.New("log redaction policy rejected record")
	ErrBatchGap       = errors.New("log batch has a sequence gap")
	ErrBatchConflict  = errors.New("log batch conflicts with accepted range")
	ErrStorageCorrupt = errors.New("persisted log storage is inconsistent")
)

type SourceKind string

const (
	SourceStdout SourceKind = "OBDUMPER_STDOUT"
	SourceStderr SourceKind = "OBDUMPER_STDERR"
	SourceFile   SourceKind = "OBDUMPER_FILE"
)

type RecordKind string

const (
	RecordLog       RecordKind = "LOG"
	RecordGap       RecordKind = "GAP"
	RecordTruncated RecordKind = "TRUNCATED"
)

type Record struct {
	StreamID      string
	SourceKind    SourceKind
	SourceEpoch   int64
	SourceSeq     int64
	Kind          RecordKind
	Message       string
	IntegrityCode string
	PolicyVersion string
	ParserVersion string
	ReceivedAt    time.Time
}

// GapNotice 表示无法补传的连续来源序号范围。
// 来源、策略和解析器身份必须显式携带，控制面不得根据缺口内容猜测来源。
type GapNotice struct {
	StreamID      string
	SourceKind    SourceKind
	SourceEpoch   int64
	FirstSeq      int64
	LastSeq       int64
	ReasonCode    string
	PolicyVersion string
	ParserVersion string
}

type Batch struct {
	StreamID       string
	SourceEpoch    int64
	FirstSeq       int64
	LastSeq        int64
	PreviousDigest string
	Digest         string
	PolicyVersion  string
	Records        []Record
}

// BatchPosition 是已持久化批次的稳定内部排序位置。
// 它只在服务端和不透明游标中使用，不能作为浏览器 API 字段或 SQLite rowid 的替代品直接暴露。
type BatchPosition struct {
	ReceivedAt time.Time
	BatchID    string
}

// PageCursor 记录固定快照内最后可靠的逻辑记录位置。
// Snapshot 将同一页链固定在写入水位，Position 与 RecordOffset 仅用于服务端续读分段文件。
type PageCursor struct {
	Snapshot     BatchPosition
	Position     BatchPosition
	RecordOffset int
}

// IndexedBatch 是 SQLite 索引定位 JSONL 批次所需的非正文元数据。
// Message 始终留在分段文件，不能加入本结构或 SQLite 索引实现。
type IndexedBatch struct {
	BatchID            string
	ExecutionID        string
	StreamID           string
	SourceKind         string
	SourceRef          string
	SourceEpoch        int64
	FirstSequence      int64
	LastSequence       int64
	PreviousDigest     string
	Digest             string
	PolicyVersion      string
	ParserVersion      string
	SegmentID          string
	SegmentOrdinal     int64
	SegmentStorageKey  string
	SegmentState       string
	SegmentByteLength  int64
	SegmentOffsetStart int64
	SegmentOffsetEnd   int64
	RecordCount        int
	// GapReasonCode 仅保存稳定原因码；空值表示普通日志批次。
	GapReasonCode string
	ReceivedAt    time.Time
}

// IndexedSegment 是恢复、封段和读取所需的非正文元数据。
// StorageKey 只能是内部段标识，调用方不得把本机绝对路径写入该字段。
type IndexedSegment struct {
	SegmentID  string
	StorageKey string
	ByteLength int64
	State      string
}

// AppendPlan 是文件追加前从 SQLite 取得的活动段和序号事实。
// 先 fsync 文件再登记 Batch，调用方若登记失败必须截断到 SegmentByteLength。
type AppendPlan struct {
	Decision          BatchDecision
	ExpectedSequence  int64
	LastDigest        string
	SegmentID         string
	SegmentOrdinal    int64
	SegmentByteLength int64
}

// BatchIndex 由 SQLite 适配层实现，隔离分段文件与业务元数据访问。
// 它只接收范围、摘要、偏移和版本，不接收日志正文、凭据或用户路径。
type BatchIndex interface {
	PrepareLogAppend(context.Context, IndexedBatch) (AppendPlan, error)
	CommitLogAppend(context.Context, IndexedBatch) (BatchResult, error)
	LatestTaskLogPosition(context.Context, string) (BatchPosition, bool, error)
	NextTaskLogBatch(context.Context, string, BatchPosition, BatchPosition, bool) (IndexedBatch, bool, error)
	ListLogSegmentBatches(context.Context, string) ([]IndexedBatch, error)
	ListOpenLogSegments(context.Context) ([]IndexedSegment, error)
	SealLogSegment(context.Context, IndexedSegment, string, time.Time) error
}
