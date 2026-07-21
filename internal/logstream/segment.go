package logstream

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SegmentWriter is the file half of the control-plane logging contract. Its
// registeredOffset represents the later SQLite batch-index transaction; an
// adapter must update that durable index only after Append reports success.
type SegmentWriter struct {
	mu               sync.Mutex
	path             string
	file             *os.File
	policy           Policy
	registeredOffset int64
}

func OpenSegment(root, segmentID string, policy Policy) (*SegmentWriter, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(segmentID) == "" || strings.ContainsAny(segmentID, `/\\`) {
		return nil, ErrInvalidInput
	}
	if _, err := policy.Redact(""); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create log segment root: %w", err)
	}
	path := filepath.Join(root, segmentID+".open")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log segment: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat log segment: %w", err)
	}
	return &SegmentWriter{path: path, file: file, policy: policy, registeredOffset: info.Size()}, nil
}

func (w *SegmentWriter) Append(batch Batch) (int64, error) {
	if err := validateBatch(batch, true); err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, record := range batch.Records {
		message, err := w.policy.Redact(record.Message)
		if err != nil {
			return 0, err
		}
		record.Message = message
		payload, err := json.Marshal(record)
		if err != nil {
			return 0, fmt.Errorf("encode redacted log record: %w", err)
		}
		if _, err := w.file.Write(append(payload, '\n')); err != nil {
			return 0, fmt.Errorf("append log segment: %w", err)
		}
	}
	if err := w.file.Sync(); err != nil {
		return 0, fmt.Errorf("fsync log segment: %w", err)
	}
	offset, err := w.file.Seek(0, 1)
	if err != nil {
		return 0, fmt.Errorf("read log segment offset: %w", err)
	}
	w.registeredOffset = offset
	return offset, nil
}

func (w *SegmentWriter) Recover(registeredOffset int64) error {
	if registeredOffset < 0 {
		return ErrInvalidInput
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	// Windows does not permit truncating an actively opened append handle. Close
	// and reopen the same private segment so recovery has identical semantics on
	// Windows and Linux.
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close log segment for recovery: %w", err)
	}
	if err := os.Truncate(w.path, registeredOffset); err != nil {
		return fmt.Errorf("truncate unregistered log tail: %w", err)
	}
	file, err := os.OpenFile(w.path, os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("reopen recovered log segment: %w", err)
	}
	w.file = file
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("fsync recovered log segment: %w", err)
	}
	w.registeredOffset = registeredOffset
	return nil
}

func (w *SegmentWriter) CloseAndSeal() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return "", ErrInvalidInput
	}
	if err := w.file.Sync(); err != nil {
		return "", err
	}
	if _, err := w.file.Seek(0, 0); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, bufio.NewReader(w.file)); err != nil {
		return "", fmt.Errorf("hash log segment: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return "", err
	}
	w.file = nil
	sealedPath := strings.TrimSuffix(w.path, ".open") + ".jsonl"
	if err := os.Rename(w.path, sealedPath); err != nil {
		return "", fmt.Errorf("seal log segment: %w", err)
	}
	w.path = sealedPath
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (w *SegmentWriter) Path() string { return w.path }
