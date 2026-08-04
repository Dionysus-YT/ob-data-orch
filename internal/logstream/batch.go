package logstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

// SealBatch 在第一层脱敏完成后计算规范内容摘要。
// 调用方只能上传该安全结构，原始字节不属于批次契约。
func SealBatch(batch Batch) (Batch, error) {
	if err := validateBatch(batch, false); err != nil {
		return Batch{}, err
	}
	copyBatch := batch
	copyBatch.Records = append([]Record(nil), batch.Records...)
	copyBatch.Digest = ""
	payload, err := json.Marshal(copyBatch)
	if err != nil {
		return Batch{}, fmt.Errorf("encode log batch: %w", err)
	}
	digest := sha256.Sum256(payload)
	copyBatch.Digest = hex.EncodeToString(digest[:])
	return copyBatch, nil
}

type BatchDecision string

const (
	BatchAccepted  BatchDecision = "ACCEPTED"
	BatchDuplicate BatchDecision = "DUPLICATE"
	BatchNeedGap   BatchDecision = "GAP_REQUIRED"
)

type BatchResult struct {
	Decision    BatchDecision
	ExpectedSeq int64
	LastDigest  string
}

type streamState struct {
	nextSeq    int64
	lastDigest string
	accepted   map[string]string
	gaps       map[string]struct{}
}

// BatchLedger 建模控制面的来源流序号。
// 它只保存内存状态且不依赖文件或 SQLite；持久化实现必须在事务中保持相同的重复和冲突判定。
type BatchLedger struct {
	mu      sync.Mutex
	streams map[string]*streamState
}

func NewBatchLedger() *BatchLedger { return &BatchLedger{streams: make(map[string]*streamState)} }

func (l *BatchLedger) Accept(batch Batch) (BatchResult, error) {
	sealed, err := SealBatch(batch)
	if err != nil {
		return BatchResult{}, err
	}
	if batch.Digest != "" && batch.Digest != sealed.Digest {
		return BatchResult{}, ErrBatchConflict
	}
	key := streamKey(sealed.StreamID, sealed.SourceEpoch)
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.state(key)
	rangeKey := fmt.Sprintf("%d-%d", sealed.FirstSeq, sealed.LastSeq)
	if digest, exists := state.accepted[rangeKey]; exists {
		if digest != sealed.Digest {
			return BatchResult{}, ErrBatchConflict
		}
		return BatchResult{Decision: BatchDuplicate, ExpectedSeq: state.nextSeq, LastDigest: state.lastDigest}, nil
	}
	if sealed.FirstSeq != state.nextSeq || sealed.PreviousDigest != state.lastDigest {
		return BatchResult{Decision: BatchNeedGap, ExpectedSeq: state.nextSeq, LastDigest: state.lastDigest}, ErrBatchGap
	}
	state.accepted[rangeKey] = sealed.Digest
	state.nextSeq = sealed.LastSeq + 1
	state.lastDigest = sealed.Digest
	return BatchResult{Decision: BatchAccepted, ExpectedSeq: state.nextSeq, LastDigest: state.lastDigest}, nil
}

// AcceptGap 只有在缺失范围从当前水位开始时才显式推进来源序号。
// 缺口是完整性事实，不能作为静默重试捷径；来源、策略和解析器身份缺失时必须失败关闭。
func (l *BatchLedger) AcceptGap(gap GapNotice) (BatchResult, error) {
	if !validGapNotice(gap) {
		return BatchResult{}, ErrInvalidInput
	}
	key := streamKey(gap.StreamID, gap.SourceEpoch)
	gapKey := fmt.Sprintf("%d-%d-%s", gap.FirstSeq, gap.LastSeq, gap.ReasonCode)
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.state(key)
	if _, exists := state.gaps[gapKey]; exists {
		return BatchResult{Decision: BatchDuplicate, ExpectedSeq: state.nextSeq, LastDigest: state.lastDigest}, nil
	}
	if gap.FirstSeq != state.nextSeq {
		return BatchResult{Decision: BatchNeedGap, ExpectedSeq: state.nextSeq, LastDigest: state.lastDigest}, ErrBatchGap
	}
	state.gaps[gapKey] = struct{}{}
	state.nextSeq = gap.LastSeq + 1
	return BatchResult{Decision: BatchAccepted, ExpectedSeq: state.nextSeq, LastDigest: state.lastDigest}, nil
}

func validGapNotice(gap GapNotice) bool {
	if strings.TrimSpace(gap.StreamID) == "" || gap.SourceEpoch <= 0 || gap.FirstSeq <= 0 || gap.LastSeq < gap.FirstSeq ||
		strings.TrimSpace(gap.PolicyVersion) == "" || strings.TrimSpace(gap.ParserVersion) == "" || !validGapReasonCode(gap.ReasonCode) {
		return false
	}
	switch gap.SourceKind {
	case SourceStdout, SourceStderr, SourceFile:
		return true
	default:
		return false
	}
}

// GapDigest 返回规范化缺口元数据的稳定摘要。
// 该摘要不包含任何日志正文，可用于 Agent 私有队列与控制面确认同一缺口事实。
func GapDigest(gap GapNotice) (string, error) {
	if !validGapNotice(gap) {
		return "", ErrInvalidInput
	}
	return gapDigest(gap), nil
}

func validGapReasonCode(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func (l *BatchLedger) state(key string) *streamState {
	if state, exists := l.streams[key]; exists {
		return state
	}
	state := &streamState{nextSeq: 1, accepted: make(map[string]string), gaps: make(map[string]struct{})}
	l.streams[key] = state
	return state
}

func validateBatch(batch Batch, requireDigest bool) error {
	if strings.TrimSpace(batch.StreamID) == "" || batch.SourceEpoch <= 0 || batch.FirstSeq <= 0 || batch.LastSeq < batch.FirstSeq || strings.TrimSpace(batch.PolicyVersion) == "" || len(batch.Records) == 0 {
		return ErrInvalidInput
	}
	if len(batch.Records) > MaxBatchRecords {
		return ErrBatchTooLarge
	}
	if requireDigest && strings.TrimSpace(batch.Digest) == "" {
		return ErrInvalidInput
	}
	if int64(len(batch.Records)) != batch.LastSeq-batch.FirstSeq+1 {
		return ErrInvalidInput
	}
	batchBytes := 0
	for index, record := range batch.Records {
		if record.StreamID != batch.StreamID || record.SourceEpoch != batch.SourceEpoch || record.SourceSeq != batch.FirstSeq+int64(index) || record.PolicyVersion != batch.PolicyVersion || record.Kind == "" {
			return ErrInvalidInput
		}
		if !utf8.ValidString(record.Message) {
			return ErrInvalidInput
		}
		if len(record.Message) > MaxRecordBytes {
			return ErrRecordTooLarge
		}
		batchBytes += len(record.Message)
		if batchBytes > MaxBatchBytes {
			return ErrBatchTooLarge
		}
	}
	return nil
}

func streamKey(streamID string, epoch int64) string { return streamID + "|" + fmt.Sprint(epoch) }
