// Package logstream contains the pure, synthetic-data log pipeline shared by
// the future Agent and control-plane adapters. It never reads a tool log,
// starts a process, or accepts raw data for persistence.
package logstream

import (
	"errors"
	"time"
)

const (
	Mask           = "******"
	MaxRecordBytes = 256 * 1024
)

var (
	ErrInvalidInput   = errors.New("invalid log stream input")
	ErrRecordTooLarge = errors.New("log record exceeds safe limit")
	ErrPolicyRejected = errors.New("log redaction policy rejected record")
	ErrBatchGap       = errors.New("log batch has a sequence gap")
	ErrBatchConflict  = errors.New("log batch conflicts with accepted range")
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

type GapNotice struct {
	StreamID    string
	SourceEpoch int64
	FirstSeq    int64
	LastSeq     int64
	ReasonCode  string
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
