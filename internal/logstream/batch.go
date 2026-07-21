package logstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// SealBatch calculates the canonical content digest after first-layer
// redaction. Callers may send this structure safely; raw bytes are not part of
// the batch contract.
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

// BatchLedger models control-plane stream sequencing. It is memory-only and
// has no file or SQLite dependency; later persistence must retain identical
// duplicate/conflict decisions transactionally.
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

// AcceptGap explicitly advances a source sequence only when the missing range
// starts at the current watermark. A Gap is an integrity fact, never a silent
// retry shortcut.
func (l *BatchLedger) AcceptGap(gap GapNotice) (BatchResult, error) {
	if strings.TrimSpace(gap.StreamID) == "" || gap.SourceEpoch <= 0 || gap.FirstSeq <= 0 || gap.LastSeq < gap.FirstSeq || strings.TrimSpace(gap.ReasonCode) == "" {
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
	if requireDigest && strings.TrimSpace(batch.Digest) == "" {
		return ErrInvalidInput
	}
	if int64(len(batch.Records)) != batch.LastSeq-batch.FirstSeq+1 {
		return ErrInvalidInput
	}
	for index, record := range batch.Records {
		if record.StreamID != batch.StreamID || record.SourceEpoch != batch.SourceEpoch || record.SourceSeq != batch.FirstSeq+int64(index) || record.PolicyVersion != batch.PolicyVersion || record.Kind == "" {
			return ErrInvalidInput
		}
	}
	return nil
}

func streamKey(streamID string, epoch int64) string { return streamID + "|" + fmt.Sprint(epoch) }
