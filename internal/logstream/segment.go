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
	return w.appendRecords(batch.Records)
}

// AppendGap 将一个已验证的逻辑缺口写为单条 JSONL 记录。
// 缺口可覆盖多个来源序号，正文不伪造为每条缺失日志；其真实范围只保存在批次索引中。
func (w *SegmentWriter) AppendGap(record Record) (int64, error) {
	if strings.TrimSpace(record.StreamID) == "" || record.SourceEpoch < 1 || record.SourceSeq < 1 || record.Kind != RecordGap || strings.TrimSpace(record.Message) == "" || strings.TrimSpace(record.PolicyVersion) == "" {
		return 0, ErrInvalidInput
	}
	return w.appendRecords([]Record{record})
}

func (w *SegmentWriter) appendRecords(records []Record) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, record := range records {
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

// Close 仅关闭活动段句柄，不把服务异常或测试结束伪造成正常封段。
// 下次打开时持久索引会继续约束可追加偏移，并在发现未登记尾部时先截断。
func (w *SegmentWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *SegmentWriter) Path() string { return w.path }

// Size 返回活动段当前已 fsync 的字节数，供封段前的索引一致性核对使用。
func (w *SegmentWriter) Size() (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, ErrInvalidInput
	}
	info, err := w.file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat active log segment: %w", err)
	}
	return info.Size(), nil
}
