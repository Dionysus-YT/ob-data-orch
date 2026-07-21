package logstream

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReassemblerKeepsSourceRecordBoundaryAcrossChunks(t *testing.T) {
	var r Reassembler
	first, truncated := r.Push([]byte("first\r\nsecond "))
	if truncated || len(first) != 1 || first[0] != "first" {
		t.Fatalf("first Push() = %#v, truncated=%v", first, truncated)
	}
	second, truncated := r.Push([]byte("中文\nlast"))
	if truncated || len(second) != 1 || second[0] != "second 中文" {
		t.Fatalf("second Push() = %#v, truncated=%v", second, truncated)
	}
	last, truncated := r.Finish()
	if truncated || len(last) != 1 || last[0] != "last" {
		t.Fatalf("Finish() = %#v, truncated=%v", last, truncated)
	}
}

func TestSegmentWriterPersistsOnlySecondLayerRedactedJSONLAndRecoversTail(t *testing.T) {
	root := t.TempDir()
	policy := Policy{Version: "synthetic-v1", Secrets: []string{"synthetic-password"}}
	writer, err := OpenSegment(root, "segment-1", policy)
	if err != nil {
		t.Fatalf("OpenSegment() error = %v", err)
	}
	batch := syntheticBatch(1, 1, "")
	batch.Records[0].Message = "password=synthetic-password"
	batch, err = SealBatch(batch)
	if err != nil {
		t.Fatalf("SealBatch() error = %v", err)
	}
	offset, err := writer.Append(batch)
	if err != nil || offset <= 0 {
		t.Fatalf("Append() = %d, %v", offset, err)
	}
	if _, err := writer.file.WriteString("{\"unsafe\":true}\n"); err != nil {
		t.Fatalf("write simulated tail: %v", err)
	}
	if err := writer.file.Sync(); err != nil {
		t.Fatalf("fsync simulated tail: %v", err)
	}
	if err := writer.Recover(offset); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	digest, err := writer.CloseAndSeal()
	if err != nil || len(digest) != 64 {
		t.Fatalf("CloseAndSeal() = %q, %v", digest, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "segment-1.jsonl"))
	if err != nil {
		t.Fatalf("read sealed segment: %v", err)
	}
	if strings.Contains(string(data), "synthetic-password") || strings.Contains(string(data), "unsafe") || !strings.Contains(string(data), Mask) {
		t.Fatalf("unsafe segment text: %q", data)
	}
}

func TestSyntheticPipelineNeverPersistsSecretAcrossChunkAndBatchBoundaries(t *testing.T) {
	secret := "synthetic-cross-boundary-password"
	policy := Policy{Version: "synthetic-v1", Secrets: []string{secret}, Identifiers: []string{"synthetic_user@tenant"}}
	var reassembler Reassembler
	first, truncated := reassembler.Push([]byte("user=synthetic_user@tenant password=synthetic-cross-"))
	if truncated || len(first) != 0 {
		t.Fatalf("first source chunk = %#v, truncated=%v", first, truncated)
	}
	lines, truncated := reassembler.Push([]byte("boundary-password\nnext line\n"))
	if truncated || len(lines) != 2 {
		t.Fatalf("second source chunk = %#v, truncated=%v", lines, truncated)
	}
	records := make([]Record, 0, len(lines))
	for index, line := range lines {
		redacted, err := policy.Redact(line)
		if err != nil {
			t.Fatalf("agent Redact() error = %v", err)
		}
		records = append(records, Record{StreamID: "stream-pipeline", SourceKind: SourceStderr, SourceEpoch: 1, SourceSeq: int64(index + 1), Kind: RecordLog, Message: redacted, PolicyVersion: policy.Version})
	}
	batch, err := SealBatch(Batch{StreamID: "stream-pipeline", SourceEpoch: 1, FirstSeq: 1, LastSeq: 2, PolicyVersion: policy.Version, Records: records})
	if err != nil {
		t.Fatalf("SealBatch() error = %v", err)
	}
	ledger := NewBatchLedger()
	if result, err := ledger.Accept(batch); err != nil || result.Decision != BatchAccepted {
		t.Fatalf("Accept() = %#v, %v", result, err)
	}
	writer, err := OpenSegment(t.TempDir(), "pipeline", policy)
	if err != nil {
		t.Fatalf("OpenSegment() error = %v", err)
	}
	if _, err := writer.Append(batch); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if _, err := writer.CloseAndSeal(); err != nil {
		t.Fatalf("CloseAndSeal() error = %v", err)
	}
	data, err := os.ReadFile(writer.Path())
	if err != nil {
		t.Fatalf("read pipeline segment: %v", err)
	}
	for _, forbidden := range []string{secret, "synthetic_user@tenant"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("pipeline segment contains %q: %q", forbidden, data)
		}
	}
}

func TestBatchLedgerAcceptsDuplicateRejectsConflictAndRequiresExplicitGap(t *testing.T) {
	ledger := NewBatchLedger()
	first := syntheticBatch(1, 2, "")
	sealed, err := SealBatch(first)
	if err != nil {
		t.Fatalf("SealBatch() error = %v", err)
	}
	accepted, err := ledger.Accept(sealed)
	if err != nil || accepted.Decision != BatchAccepted || accepted.ExpectedSeq != 3 {
		t.Fatalf("Accept(first) = %#v, %v", accepted, err)
	}
	duplicate, err := ledger.Accept(sealed)
	if err != nil || duplicate.Decision != BatchDuplicate {
		t.Fatalf("Accept(duplicate) = %#v, %v", duplicate, err)
	}
	conflict := sealed
	conflict.Records[0].Message = "different-synthetic-message"
	conflict.Digest = ""
	if _, err := ledger.Accept(conflict); !errors.Is(err, ErrBatchConflict) {
		t.Fatalf("Accept(conflict) error = %v", err)
	}
	late := syntheticBatch(4, 4, accepted.LastDigest)
	if result, err := ledger.Accept(late); !errors.Is(err, ErrBatchGap) || result.ExpectedSeq != 3 {
		t.Fatalf("Accept(gap) = %#v, %v", result, err)
	}
	gap, err := ledger.AcceptGap(GapNotice{StreamID: "stream-1", SourceEpoch: 1, FirstSeq: 3, LastSeq: 3, ReasonCode: "LOCAL_SPOOL_LIMIT"})
	if err != nil || gap.ExpectedSeq != 4 {
		t.Fatalf("AcceptGap() = %#v, %v", gap, err)
	}
	late, err = SealBatch(late)
	if err != nil {
		t.Fatalf("SealBatch(late) error = %v", err)
	}
	if result, err := ledger.Accept(late); err != nil || result.Decision != BatchAccepted {
		t.Fatalf("Accept(after gap) = %#v, %v", result, err)
	}
}

func syntheticBatch(first, last int64, previous string) Batch {
	records := make([]Record, 0, last-first+1)
	for seq := first; seq <= last; seq++ {
		records = append(records, Record{StreamID: "stream-1", SourceKind: SourceStdout, SourceEpoch: 1, SourceSeq: seq, Kind: RecordLog, Message: "safe", PolicyVersion: "synthetic-v1"})
	}
	return Batch{StreamID: "stream-1", SourceEpoch: 1, FirstSeq: first, LastSeq: last, PreviousDigest: previous, PolicyVersion: "synthetic-v1", Records: records}
}

func TestReassemblerDropsOversizedRecordWithoutPersistingText(t *testing.T) {
	var r Reassembler
	_, truncated := r.Push([]byte(strings.Repeat("x", MaxRecordBytes+1)))
	if !truncated {
		t.Fatal("oversized tail was not marked truncated")
	}
	records, truncated := r.Push([]byte("still-secret\nnext\n"))
	if !truncated || len(records) != 1 || records[0] != "next" {
		t.Fatalf("drop recovery = %#v, truncated=%v", records, truncated)
	}
}

func TestPolicyIsIdempotentAndNeverRetainsSyntheticSecret(t *testing.T) {
	policy := Policy{Version: "synthetic-v1", Secrets: []string{"synthetic-password"}, Identifiers: []string{"synthetic_user@tenant"}}
	input := "password=synthetic-password user=synthetic_user@tenant token:abc123"
	once, err := policy.Redact(input)
	if err != nil {
		t.Fatalf("Redact() error = %v", err)
	}
	twice, err := policy.Redact(once)
	if err != nil || once != twice {
		t.Fatalf("idempotent Redact() = %q, %q, %v", once, twice, err)
	}
	for _, forbidden := range []string{"synthetic-password", "synthetic_user@tenant", "abc123"} {
		if strings.Contains(once, forbidden) {
			t.Fatalf("redacted text contains %q: %q", forbidden, once)
		}
	}
	if strings.Count(once, Mask) != 3 {
		t.Fatalf("mask count = %d, text=%q", strings.Count(once, Mask), once)
	}
}

func TestPolicyFailsClosedWithoutVersionOrWithEmptySecret(t *testing.T) {
	if _, err := (Policy{}).Redact("message"); err != ErrPolicyRejected {
		t.Fatalf("missing version error = %v", err)
	}
	if _, err := (Policy{Version: "v1", Secrets: []string{""}}).Redact("message"); err != ErrPolicyRejected {
		t.Fatalf("empty secret error = %v", err)
	}
}
