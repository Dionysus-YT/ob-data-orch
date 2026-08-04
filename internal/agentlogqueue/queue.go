// Package agentlogqueue 提供 Agent 本机已脱敏日志批次的最小持久队列。
package agentlogqueue

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/logstream"
)

const (
	DefaultMaxBytes         int64 = 1 << 30
	DefaultEvidenceMaxBytes int64 = 16 << 20
	DefaultGapMaxBytes      int64 = 16 << 20
	evidenceFileName              = "queue-evidence.jsonl"
)

var (
	ErrInvalidInput = errors.New("agent log queue input is invalid")
	ErrFull         = errors.New("agent log queue is full")
	ErrCorrupt      = errors.New("agent log queue is corrupt")
	ErrEvidenceFull = errors.New("agent log queue evidence is full")
	ErrGapFull      = errors.New("agent log gap queue is full")
)

// EvidenceType 表示本地可靠队列中一个可复核但不含正文的状态变化。
type EvidenceType string

const (
	EvidenceEnqueued              EvidenceType = "ENQUEUED"
	EvidenceUploadAttempt         EvidenceType = "UPLOAD_ATTEMPT"
	EvidenceRetryAttempt          EvidenceType = "RETRY_ATTEMPT"
	EvidenceRecoveryReplayAttempt EvidenceType = "RECOVERY_REPLAY_ATTEMPT"
	EvidenceUploadFailed          EvidenceType = "UPLOAD_FAILED"
	EvidenceControlPlaneConfirmed EvidenceType = "CONTROL_PLANE_CONFIRMED"
)

// Evidence 是 Agent 私有队列账本的一条安全证据。
// 它只保留已密封批次的位置与摘要，不能增加记录正文、任务配置、命令、路径或原始错误字段。
type Evidence struct {
	Type        EvidenceType `json:"type"`
	StreamID    string       `json:"streamId"`
	SourceEpoch int64        `json:"sourceEpoch"`
	FirstSeq    int64        `json:"firstSeq"`
	LastSeq     int64        `json:"lastSeq"`
	BatchDigest string       `json:"batchDigest"`
	OccurredAt  time.Time    `json:"occurredAt"`
}

// Entry 是等待控制面确认的已密封日志批次；不包含凭据、命令或原始字节。
type Entry struct {
	ExecutionID    string          `json:"executionId"`
	LeaseID        string          `json:"leaseId"`
	LeaseEpoch     int64           `json:"leaseEpoch"`
	EnvelopeDigest string          `json:"envelopeDigest"`
	Batch          logstream.Batch `json:"batch"`
}

// GapEntry 是等待控制面确认的无正文来源缺口。
// 它与普通日志批次分开计量，保证本体队列满时仍能持久化可审查的丢失范围。
type GapEntry struct {
	ExecutionID    string              `json:"executionId"`
	LeaseID        string              `json:"leaseId"`
	LeaseEpoch     int64               `json:"leaseEpoch"`
	EnvelopeDigest string              `json:"envelopeDigest"`
	Gap            logstream.GapNotice `json:"gap"`
}

// PendingItem 是待确认普通批次或缺口的有序投影。
// 同一来源按 epoch 与序号排序，使缺口先于之后的正常批次获得确认。
type PendingItem struct {
	// Entry 非空时表示普通已密封日志批次。
	Entry *Entry
	// Gap 非空时表示无正文来源缺口；Entry 与 Gap 恰有一个非空。
	Gap *GapEntry
	// Token 只能传回 Acknowledge，不能作为浏览器或远程文件访问标识。
	Token string
}

// Queue 将每个批次以独立私有文件 fsync 后保存，防止确认前的崩溃丢失。
type Queue struct {
	root             string
	maxBytes         int64
	evidenceMaxBytes int64
	gapMaxBytes      int64
	// usedBytes 与 gapUsedBytes 只缓存当前打开实例已观察到的队列文件大小。
	// 每次 Open 必须重新扫描，随后仅在 fsync 写入或确认删除成功后更新，避免 1 GiB 队列在每次入队时重复全量扫描。
	usedBytes    int64
	gapUsedBytes int64
	mu           sync.Mutex
}

// Open 打开 Agent 身份状态目录下的队列根；根路径只能来自本机受保护配置。
func Open(root string, maxBytes int64) (*Queue, error) {
	if strings.TrimSpace(root) == "" || strings.ContainsRune(root, 0) || maxBytes < 1 {
		return nil, ErrInvalidInput
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	queue := &Queue{root: filepath.Clean(abs), maxBytes: maxBytes, evidenceMaxBytes: DefaultEvidenceMaxBytes, gapMaxBytes: DefaultGapMaxBytes}
	usedBytes, err := queue.sizeLocked()
	if err != nil {
		return nil, err
	}
	gapUsedBytes, err := queue.gapSizeLocked()
	if err != nil {
		return nil, err
	}
	queue.usedBytes, queue.gapUsedBytes = usedBytes, gapUsedBytes
	return queue, nil
}

// Enqueue 在返回前完成文件同步；同一批次重复写入保持幂等。
func (q *Queue) Enqueue(entry Entry) error {
	if q == nil || !validEntry(entry) {
		return ErrInvalidInput
	}
	sealed, err := logstream.SealBatch(entry.Batch)
	if err != nil || sealed.Digest != entry.Batch.Digest {
		return ErrInvalidInput
	}
	entry.Batch = sealed
	content, err := json.Marshal(entry)
	if err != nil {
		return ErrInvalidInput
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if int64(len(content)) > q.maxBytes {
		return ErrFull
	}
	if q.usedBytes+int64(len(content)) > q.maxBytes {
		return ErrFull
	}
	path := filepath.Join(q.root, entryName(entry))
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, content) {
			return nil
		}
		return ErrCorrupt
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := q.writeEntryLocked(path, content); err != nil {
		return err
	}
	q.usedBytes += int64(len(content))
	return nil
}

// EnqueueGap 在普通批次队列满时持久化无正文缺口，直到控制面确认后才可删除。
// 缺口存储有独立固定上限，避免无限元数据占用磁盘而又不会覆盖已有普通批次。
func (q *Queue) EnqueueGap(entry GapEntry) error {
	if q == nil || !validGapEntry(entry) {
		return ErrInvalidInput
	}
	content, err := json.Marshal(entry)
	if err != nil {
		return ErrInvalidInput
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if int64(len(content)) > q.gapMaxBytes {
		return ErrGapFull
	}
	if q.gapUsedBytes+int64(len(content)) > q.gapMaxBytes {
		return ErrGapFull
	}
	path := filepath.Join(q.root, gapEntryName(entry))
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, content) {
			return nil
		}
		return ErrCorrupt
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := q.writeEntryLocked(path, content); err != nil {
		return err
	}
	q.gapUsedBytes += int64(len(content))
	return nil
}

// Pending 按稳定文件名顺序返回尚未确认的批次及其确认令牌。
func (q *Queue) Pending() ([]Entry, []string, error) {
	if q == nil {
		return nil, nil, ErrInvalidInput
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	files, err := filepath.Glob(filepath.Join(q.root, "*.json"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(files)
	entries, tokens := make([]Entry, 0, len(files)), make([]string, 0, len(files))
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, nil, err
		}
		var entry Entry
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&entry) != nil || !validEntry(entry) || entryName(entry) != filepath.Base(file) {
			return nil, nil, ErrCorrupt
		}
		entries, tokens = append(entries, entry), append(tokens, filepath.Base(file))
	}
	return entries, tokens, nil
}

// PendingItems 返回普通批次与缺口的稳定待确认顺序。
// 跨来源顺序只用于重放稳定性；同一来源始终先确认较小序号，禁止让后续批次越过缺口。
func (q *Queue) PendingItems() ([]PendingItem, error) {
	if q == nil {
		return nil, ErrInvalidInput
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	items := make([]PendingItem, 0)
	batchFiles, err := filepath.Glob(filepath.Join(q.root, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, file := range batchFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var entry Entry
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&entry) != nil || !validEntry(entry) || entryName(entry) != filepath.Base(file) {
			return nil, ErrCorrupt
		}
		items = append(items, PendingItem{Entry: &entry, Token: filepath.Base(file)})
	}
	gapFiles, err := filepath.Glob(filepath.Join(q.root, "*.gap"))
	if err != nil {
		return nil, err
	}
	for _, file := range gapFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var entry GapEntry
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&entry) != nil || !validGapEntry(entry) || gapEntryName(entry) != filepath.Base(file) {
			return nil, ErrCorrupt
		}
		items = append(items, PendingItem{Gap: &entry, Token: filepath.Base(file)})
	}
	sort.Slice(items, func(left, right int) bool { return pendingItemBefore(items[left], items[right]) })
	return items, nil
}

// Acknowledge 仅删除已由调用方得到控制面确认的队列文件。
func (q *Queue) Acknowledge(token string) error {
	if q == nil || filepath.Base(token) != token || (!strings.HasSuffix(token, ".json") && !strings.HasSuffix(token, ".gap")) {
		return ErrInvalidInput
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	path := filepath.Join(q.root, token)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	usedBytes := &q.usedBytes
	if strings.HasSuffix(token, ".gap") {
		usedBytes = &q.gapUsedBytes
	}
	if info.Size() > *usedBytes {
		return ErrCorrupt
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	*usedBytes -= info.Size()
	return nil
}

// RecordEvidence 以追加并 fsync 的方式保存无正文队列证据。
// 账本不可用或达到固定上限时返回错误，调用方必须保留待确认批次，不能把缺少证据伪装成已确认。
func (q *Queue) RecordEvidence(evidence Evidence) error {
	if q == nil || !validEvidence(evidence) {
		return ErrInvalidInput
	}
	content, err := json.Marshal(evidence)
	if err != nil {
		return ErrInvalidInput
	}
	content = append(content, '\n')
	q.mu.Lock()
	defer q.mu.Unlock()
	path := filepath.Join(q.root, evidenceFileName)
	if info, err := os.Stat(path); err == nil {
		if info.Size()+int64(len(content)) > q.evidenceMaxBytes {
			return ErrEvidenceFull
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if int64(len(content)) > q.evidenceMaxBytes {
			return ErrEvidenceFull
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		return err
	}
	return file.Sync()
}

// EvidenceSnapshot 返回已经 fsync 的安全队列证据，供 Agent 本地恢复核对和测试使用。
// 它不是浏览器、HTTP 或任意文件读取能力，无法返回任何日志正文。
func (q *Queue) EvidenceSnapshot() ([]Evidence, error) {
	if q == nil {
		return nil, ErrInvalidInput
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	file, err := os.Open(filepath.Join(q.root, evidenceFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	evidence := make([]Evidence, 0)
	for {
		var item Evidence
		err := decoder.Decode(&item)
		if errors.Is(err, io.EOF) {
			return evidence, nil
		}
		if err != nil || !validEvidence(item) {
			return nil, ErrCorrupt
		}
		evidence = append(evidence, item)
	}
}

func (q *Queue) sizeLocked() (int64, error) {
	files, err := filepath.Glob(filepath.Join(q.root, "*.json"))
	if err != nil {
		return 0, err
	}
	var total int64
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func (q *Queue) gapSizeLocked() (int64, error) {
	files, err := filepath.Glob(filepath.Join(q.root, "*.gap"))
	if err != nil {
		return 0, err
	}
	var total int64
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func (q *Queue) writeEntryLocked(path string, content []byte) error {
	temporary, err := os.CreateTemp(q.root, ".pending-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func validEntry(entry Entry) bool {
	if !safeID(entry.ExecutionID) || !safeID(entry.LeaseID) || entry.LeaseEpoch < 1 || len(entry.EnvelopeDigest) != 64 || !safeID(entry.Batch.StreamID) {
		return false
	}
	sealed, err := logstream.SealBatch(entry.Batch)
	return err == nil && entry.Batch.Digest == sealed.Digest
}

func validGapEntry(entry GapEntry) bool {
	if !safeID(entry.ExecutionID) || !safeID(entry.LeaseID) || entry.LeaseEpoch < 1 || len(entry.EnvelopeDigest) != 64 || entry.Gap.StreamID != entry.ExecutionID {
		return false
	}
	_, err := logstream.GapDigest(entry.Gap)
	return err == nil
}

func validEvidence(evidence Evidence) bool {
	if evidence.Type != EvidenceEnqueued && evidence.Type != EvidenceUploadAttempt && evidence.Type != EvidenceRetryAttempt && evidence.Type != EvidenceRecoveryReplayAttempt && evidence.Type != EvidenceUploadFailed && evidence.Type != EvidenceControlPlaneConfirmed {
		return false
	}
	if !safeID(evidence.StreamID) || evidence.SourceEpoch < 1 || evidence.FirstSeq < 1 || evidence.LastSeq < evidence.FirstSeq || evidence.OccurredAt.IsZero() || len(evidence.BatchDigest) != 64 {
		return false
	}
	_, err := hex.DecodeString(evidence.BatchDigest)
	return err == nil
}
func safeID(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\\/\r\n")
}
func entryName(entry Entry) string {
	return fmt.Sprintf("%s-%03d-%020d-%s.json", entry.ExecutionID, entry.Batch.SourceEpoch, entry.Batch.FirstSeq, entry.Batch.Digest)
}

func gapEntryName(entry GapEntry) string {
	identity := entry.ExecutionID + "|" + entry.Gap.StreamID + "|" + fmt.Sprint(entry.Gap.SourceEpoch) + "|" + fmt.Sprint(entry.Gap.FirstSeq) + "|" + entry.Gap.ReasonCode + "|" + entry.Gap.PolicyVersion + "|" + entry.Gap.ParserVersion
	digest := sha256.Sum256([]byte(identity))
	return "gap-" + hex.EncodeToString(digest[:]) + ".gap"
}

func pendingItemBefore(left, right PendingItem) bool {
	leftStream, leftEpoch, leftSequence := pendingItemPosition(left)
	rightStream, rightEpoch, rightSequence := pendingItemPosition(right)
	if leftStream != rightStream {
		return leftStream < rightStream
	}
	if leftEpoch != rightEpoch {
		return leftEpoch < rightEpoch
	}
	if leftSequence != rightSequence {
		return leftSequence < rightSequence
	}
	return left.Token < right.Token
}

func pendingItemPosition(item PendingItem) (string, int64, int64) {
	if item.Entry != nil {
		return item.Entry.Batch.StreamID, item.Entry.Batch.SourceEpoch, item.Entry.Batch.FirstSeq
	}
	return item.Gap.Gap.StreamID, item.Gap.Gap.SourceEpoch, item.Gap.Gap.FirstSeq
}
