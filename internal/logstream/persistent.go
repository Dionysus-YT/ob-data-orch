package logstream

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	persistedLogPageSize = 200
	maxSegmentBytes      = 8 << 20
)

// PersistentStore 将已双层脱敏的 JSONL 段与 SQLite 元数据索引按固定顺序组合。
// 它不认证调用方；浏览器和 Agent 的身份、租约及对象范围仍必须在控制面边界完成校验。
type PersistentStore struct {
	mu            sync.Mutex
	root          string
	index         BatchIndex
	writers       map[string]*SegmentWriter
	updates       chan struct{}
	needsRecovery bool
}

// NewPersistentStore 创建控制面本机分段日志存储。
// root 必须由组合根决定，不能接受浏览器、Agent 或任务配置提供的路径。
func NewPersistentStore(root string, index BatchIndex) (*PersistentStore, error) {
	if strings.TrimSpace(root) == "" || index == nil {
		return nil, ErrInvalidInput
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve log root: %w", err)
	}
	return &PersistentStore{
		root:    filepath.Clean(absRoot),
		index:   index,
		writers: make(map[string]*SegmentWriter),
		updates: make(chan struct{}, 1),
		// 新建实例必须先核对上一次进程留下的活动段，不能假设内存句柄仍存在。
		needsRecovery: true,
	}, nil
}

// Append 按“索引核对 → 第二层脱敏 → 文件 fsync → SQLite 登记”的顺序保存一批日志。
// 任意登记失败都会把 .open 段恢复到已登记偏移，不能把未索引尾部当成可读取日志。
func (store *PersistentStore) Append(ctx context.Context, executionID string, batch Batch) (BatchResult, error) {
	if store == nil || strings.TrimSpace(executionID) == "" {
		return BatchResult{}, ErrInvalidInput
	}
	sealed, err := SealBatch(batch)
	if err != nil {
		return BatchResult{}, err
	}
	if len(sealed.StreamID) > 256 || strings.ContainsAny(sealed.StreamID, `/\\`) {
		return BatchResult{}, ErrInvalidInput
	}
	if batch.Digest != "" && batch.Digest != sealed.Digest {
		return BatchResult{}, ErrBatchConflict
	}
	for _, record := range sealed.Records {
		// 未携带秘密上下文时仍须拒绝显式敏感键值，避免把“第一层已脱敏”当作可信前提。
		redacted, err := (Policy{Version: sealed.PolicyVersion}).Redact(record.Message)
		if err != nil || redacted != record.Message {
			return BatchResult{}, ErrPolicyRejected
		}
	}
	sourceKind, err := persistedSourceKind(sealed)
	if err != nil {
		return BatchResult{}, err
	}
	now := time.Now().UTC()
	entry := IndexedBatch{
		BatchID:        internalID("log-batch", executionID, sealed.StreamID, fmt.Sprint(sealed.SourceEpoch), fmt.Sprint(sealed.FirstSeq), sealed.Digest),
		ExecutionID:    executionID,
		StreamID:       internalID("log-stream", executionID, sealed.StreamID, fmt.Sprint(sealed.SourceEpoch)),
		SourceKind:     sourceKind,
		SourceRef:      sealed.StreamID,
		SourceEpoch:    sealed.SourceEpoch,
		FirstSequence:  sealed.FirstSeq,
		LastSequence:   sealed.LastSeq,
		PreviousDigest: sealed.PreviousDigest,
		Digest:         sealed.Digest,
		PolicyVersion:  sealed.PolicyVersion,
		ParserVersion:  persistedParserVersion(sealed),
		RecordCount:    len(sealed.Records),
		ReceivedAt:     now,
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	return store.appendIndexedLocked(ctx, entry, Policy{Version: sealed.PolicyVersion}, func(writer *SegmentWriter) (int64, error) {
		return writer.Append(sealed)
	})
}

// AppendGap 将有完整来源身份的序号缺口保存为一个受控 JSONL 记录与范围索引。
// 缺口不继承或猜测 stdout/stderr 身份，后续普通批次仍以前一个正常批次摘要续接。
func (store *PersistentStore) AppendGap(ctx context.Context, executionID string, gap GapNotice) (BatchResult, error) {
	if store == nil || strings.TrimSpace(executionID) == "" || !validGapNotice(gap) || len(gap.StreamID) > 256 || strings.ContainsAny(gap.StreamID, `/\\`) {
		return BatchResult{}, ErrInvalidInput
	}
	sourceKind, err := persistedSourceKindFromRecord(gap.SourceKind)
	if err != nil {
		return BatchResult{}, err
	}
	now := time.Now().UTC()
	digest := gapDigest(gap)
	entry := IndexedBatch{
		BatchID:       internalID("log-gap", executionID, gap.StreamID, fmt.Sprint(gap.SourceEpoch), fmt.Sprint(gap.FirstSeq), fmt.Sprint(gap.LastSeq), gap.ReasonCode, digest),
		ExecutionID:   executionID,
		StreamID:      internalID("log-stream", executionID, gap.StreamID, fmt.Sprint(gap.SourceEpoch)),
		SourceKind:    sourceKind,
		SourceRef:     gap.StreamID,
		SourceEpoch:   gap.SourceEpoch,
		FirstSequence: gap.FirstSeq,
		LastSequence:  gap.LastSeq,
		Digest:        digest,
		PolicyVersion: gap.PolicyVersion,
		ParserVersion: gap.ParserVersion,
		RecordCount:   1,
		GapReasonCode: gap.ReasonCode,
		ReceivedAt:    now,
	}
	record := Record{
		StreamID: gap.StreamID, SourceKind: gap.SourceKind, SourceEpoch: gap.SourceEpoch, SourceSeq: gap.FirstSeq,
		Kind: RecordGap, Message: "日志序号存在缺口", IntegrityCode: gap.ReasonCode,
		PolicyVersion: gap.PolicyVersion, ParserVersion: gap.ParserVersion, ReceivedAt: now,
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	return store.appendIndexedLocked(ctx, entry, Policy{Version: gap.PolicyVersion}, func(writer *SegmentWriter) (int64, error) {
		return writer.AppendGap(record)
	})
}

func (store *PersistentStore) appendIndexedLocked(ctx context.Context, entry IndexedBatch, policy Policy, appendToWriter func(*SegmentWriter) (int64, error)) (BatchResult, error) {
	if store.needsRecovery {
		if err := store.recoverLocked(ctx); err != nil {
			return BatchResult{}, err
		}
	}
	plan, err := store.index.PrepareLogAppend(ctx, entry)
	if err != nil {
		return BatchResult{}, err
	}
	if plan.Decision == BatchDuplicate {
		return BatchResult{Decision: BatchDuplicate, ExpectedSeq: plan.ExpectedSequence, LastDigest: plan.LastDigest}, nil
	}
	if plan.Decision == BatchNeedGap {
		return BatchResult{Decision: BatchNeedGap, ExpectedSeq: plan.ExpectedSequence, LastDigest: plan.LastDigest}, ErrBatchGap
	}
	entry.SegmentID = plan.SegmentID
	entry.SegmentOrdinal = plan.SegmentOrdinal
	entry.SegmentByteLength = plan.SegmentByteLength
	writer, err := store.writer(plan, policy)
	if err != nil {
		return BatchResult{}, err
	}
	entry.SegmentOffsetStart = plan.SegmentByteLength
	entry.SegmentOffsetEnd, err = appendToWriter(writer)
	if err != nil {
		return BatchResult{}, err
	}
	result, err := store.index.CommitLogAppend(ctx, entry)
	if err != nil {
		if recoverErr := writer.Recover(plan.SegmentByteLength); recoverErr != nil {
			store.needsRecovery = true
			return BatchResult{}, fmt.Errorf("commit log index: %w; recover segment: %v", err, recoverErr)
		}
		return BatchResult{}, err
	}
	if result.Decision != BatchAccepted {
		if recoverErr := writer.Recover(plan.SegmentByteLength); recoverErr != nil {
			store.needsRecovery = true
			return BatchResult{}, fmt.Errorf("unexpected log index decision %s; recover segment: %v", result.Decision, recoverErr)
		}
		return BatchResult{}, ErrStorageCorrupt
	}
	store.notify()
	if entry.SegmentOffsetEnd >= maxSegmentBytes {
		if err := store.sealWriterLocked(ctx, entry.SegmentID, writer); err != nil {
			return BatchResult{}, err
		}
	}
	return result, nil
}

// ReadPage 从一个固定快照读取最多 200 条已登记记录。
// cursor 为 nil 时先固定当前水位；后续页不会混入新批次，保证阅读位置稳定。
func (store *PersistentStore) ReadPage(ctx context.Context, taskID string, cursor *PageCursor) ([]Record, *PageCursor, *PageCursor, error) {
	return store.readPageLimit(ctx, taskID, cursor, persistedLogPageSize)
}

func (store *PersistentStore) readPageLimit(ctx context.Context, taskID string, cursor *PageCursor, limit int) ([]Record, *PageCursor, *PageCursor, error) {
	if store == nil || strings.TrimSpace(taskID) == "" || limit < 1 || limit > persistedLogPageSize {
		return nil, nil, nil, ErrInvalidInput
	}
	if err := store.ensureRecovered(ctx); err != nil {
		return nil, nil, nil, err
	}
	working := PageCursor{}
	if cursor == nil {
		snapshot, found, err := store.index.LatestTaskLogPosition(ctx, taskID)
		if err != nil {
			return nil, nil, nil, err
		}
		if !found {
			return []Record{}, nil, nil, nil
		}
		working.Snapshot = snapshot
	} else {
		working = *cursor
		if working.Snapshot.BatchID == "" || working.Snapshot.ReceivedAt.IsZero() || working.RecordOffset < 0 {
			return nil, nil, nil, ErrInvalidInput
		}
	}

	items := make([]Record, 0, limit)
	last := PageCursor{Snapshot: working.Snapshot, Position: working.Position, RecordOffset: working.RecordOffset}
	for len(items) < limit {
		batch, found, err := store.index.NextTaskLogBatch(ctx, taskID, working.Snapshot, working.Position, true)
		if err != nil {
			return nil, nil, nil, err
		}
		if !found {
			break
		}
		records, err := store.readBatch(batch)
		if err != nil {
			return nil, nil, nil, err
		}
		start := 0
		if samePosition(batchPosition(batch), working.Position) {
			start = working.RecordOffset + 1
		}
		if start > len(records) {
			return nil, nil, nil, ErrStorageCorrupt
		}
		if start == len(records) {
			batch, found, err = store.index.NextTaskLogBatch(ctx, taskID, working.Snapshot, working.Position, false)
			if err != nil {
				return nil, nil, nil, err
			}
			if !found {
				break
			}
			records, err = store.readBatch(batch)
			if err != nil {
				return nil, nil, nil, err
			}
			start = 0
		}
		for index := start; index < len(records) && len(items) < limit; index++ {
			items = append(items, records[index])
			last = PageCursor{Snapshot: working.Snapshot, Position: batchPosition(batch), RecordOffset: index}
		}
		working.Position = batchPosition(batch)
		working.RecordOffset = len(records) - 1
		if len(records) == 0 {
			return nil, nil, nil, ErrStorageCorrupt
		}
	}
	if len(items) == 0 {
		return items, nil, nil, nil
	}
	hasMore, err := store.hasFollowingRecord(ctx, taskID, last)
	if err != nil {
		return nil, nil, nil, err
	}
	if !hasMore {
		return items, nil, &last, nil
	}
	// next 与 last 都是最后可靠位置；调用方继续同一固定快照时使用 next，
	// SSE 重连则把 last 作为 after 位置重新建立新的快照。
	return items, &last, &last, nil
}

// ReadSince 以最后可靠位置为起点建立新快照，供 SSE 连接和断线后的 REST 增量补读使用。
func (store *PersistentStore) ReadSince(ctx context.Context, taskID string, after *PageCursor) ([]Record, *PageCursor, error) {
	if store == nil || strings.TrimSpace(taskID) == "" {
		return nil, nil, ErrInvalidInput
	}
	if err := store.ensureRecovered(ctx); err != nil {
		return nil, nil, err
	}
	if after == nil {
		items, _, last, err := store.readPageLimit(ctx, taskID, nil, persistedLogPageSize)
		return items, last, err
	}
	snapshot, found, err := store.index.LatestTaskLogPosition(ctx, taskID)
	if err != nil || !found {
		return []Record{}, nil, err
	}
	if comparePosition(snapshot, after.Position) <= 0 {
		return []Record{}, after, nil
	}
	cursor := &PageCursor{Snapshot: snapshot, Position: after.Position, RecordOffset: after.RecordOffset}
	items, _, last, err := store.readPageLimit(ctx, taskID, cursor, persistedLogPageSize)
	return items, last, err
}

// ReadNextSince 以单条事件粒度推进最后可靠游标，避免 SSE 在半页断开时静默跨过未送达记录。
func (store *PersistentStore) ReadNextSince(ctx context.Context, taskID string, after *PageCursor) (Record, *PageCursor, bool, error) {
	if store == nil || strings.TrimSpace(taskID) == "" {
		return Record{}, after, false, ErrInvalidInput
	}
	if err := store.ensureRecovered(ctx); err != nil {
		return Record{}, after, false, err
	}
	snapshot, found, err := store.index.LatestTaskLogPosition(ctx, taskID)
	if err != nil || !found {
		return Record{}, after, false, err
	}
	var cursor *PageCursor
	if after == nil {
		cursor = &PageCursor{Snapshot: snapshot}
	} else {
		cursor = &PageCursor{Snapshot: snapshot, Position: after.Position, RecordOffset: after.RecordOffset}
	}
	items, _, last, err := store.readPageLimit(ctx, taskID, cursor, 1)
	if err != nil || len(items) == 0 || last == nil {
		return Record{}, after, false, err
	}
	return items[0], last, true, nil
}

func (store *PersistentStore) hasFollowingRecord(ctx context.Context, taskID string, cursor PageCursor) (bool, error) {
	batch, found, err := store.index.NextTaskLogBatch(ctx, taskID, cursor.Snapshot, cursor.Position, true)
	if err != nil || !found {
		return false, err
	}
	records, err := store.readBatch(batch)
	if err != nil {
		return false, err
	}
	if samePosition(batchPosition(batch), cursor.Position) && cursor.RecordOffset+1 < len(records) {
		return true, nil
	}
	batch, found, err = store.index.NextTaskLogBatch(ctx, taskID, cursor.Snapshot, cursor.Position, false)
	if err != nil || !found {
		return false, err
	}
	records, err = store.readBatch(batch)
	return len(records) > 0, err
}

// Subscribe 返回批次登记完成后的非阻塞通知。
// 通知只表示“可能有新已持久化记录”，订阅方必须重新按游标读取，不能把通知本身当成日志事实。
func (store *PersistentStore) Subscribe() <-chan struct{} {
	if store == nil {
		return nil
	}
	return store.updates
}

// Close 释放活动段文件句柄，供控制面关闭和合成测试清理使用。
// 它不封段；只有确认的正常收尾流程才可以把 .open 原子改名为不可变段。
func (store *PersistentStore) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	var first error
	for segmentID, writer := range store.writers {
		if err := writer.Close(); err != nil && first == nil {
			first = err
		}
		delete(store.writers, segmentID)
	}
	// Close 后若同一实例被复用，仍须按索引重新核对磁盘段，不能复用已经失效的句柄假设。
	store.needsRecovery = true
	return first
}

// Recover 在启动或不确定的封段失败后，仅按已登记的 SQLite 偏移恢复活动段。
// 未登记的尾部会被截断；发现段状态、长度、摘要或文件名不一致时返回错误而不猜测可读内容。
func (store *PersistentStore) Recover(ctx context.Context) error {
	if store == nil {
		return ErrInvalidInput
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.recoverLocked(ctx)
}

// Shutdown 将已登记的活动段原子封存，供控制面正常退出调用。
// 非正常中断不伪装为封段，下一次启动仍由 Recover 依据索引失败关闭地处理。
func (store *PersistentStore) Shutdown(ctx context.Context) error {
	if store == nil {
		return ErrInvalidInput
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.needsRecovery {
		if err := store.recoverLocked(ctx); err != nil {
			return err
		}
	}
	var first error
	for segmentID, writer := range store.writers {
		if err := store.sealWriterLocked(ctx, segmentID, writer); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (store *PersistentStore) ensureRecovered(ctx context.Context) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.needsRecovery {
		return nil
	}
	return store.recoverLocked(ctx)
}

func (store *PersistentStore) recoverLocked(ctx context.Context) error {
	for segmentID, writer := range store.writers {
		if err := writer.Close(); err != nil {
			return fmt.Errorf("close active segment before recovery: %w", err)
		}
		delete(store.writers, segmentID)
	}
	segments, err := store.index.ListOpenLogSegments(ctx)
	if err != nil {
		return err
	}
	for _, segment := range segments {
		if !validIndexedOpenSegment(segment) {
			return ErrStorageCorrupt
		}
		openPath := filepath.Join(store.root, segment.SegmentID+".open")
		sealedPath := filepath.Join(store.root, segment.SegmentID+".jsonl")
		openInfo, openErr := os.Stat(openPath)
		sealedInfo, sealedErr := os.Stat(sealedPath)
		if (openErr == nil && sealedErr == nil) || (openErr != nil && !os.IsNotExist(openErr)) || (sealedErr != nil && !os.IsNotExist(sealedErr)) {
			return ErrStorageCorrupt
		}
		switch {
		case sealedErr == nil:
			if sealedInfo.IsDir() || sealedInfo.Size() != segment.ByteLength {
				return ErrStorageCorrupt
			}
			digest, err := segmentDigest(sealedPath)
			if err != nil {
				return err
			}
			if err := store.verifyIndexedSegment(ctx, segment.SegmentID, "SEALED"); err != nil {
				return err
			}
			if err := store.index.SealLogSegment(ctx, segment, digest, time.Now().UTC()); err != nil {
				return err
			}
		case openErr == nil:
			if openInfo.IsDir() || openInfo.Size() < segment.ByteLength {
				return ErrStorageCorrupt
			}
			if openInfo.Size() > segment.ByteLength {
				if err := truncateOpenSegment(openPath, segment.ByteLength); err != nil {
					return err
				}
			}
			if err := store.verifyIndexedSegment(ctx, segment.SegmentID, "OPEN"); err != nil {
				return err
			}
		case segment.ByteLength != 0:
			return ErrStorageCorrupt
		}
	}
	store.needsRecovery = false
	return nil
}

func (store *PersistentStore) verifyIndexedSegment(ctx context.Context, segmentID, state string) error {
	batches, err := store.index.ListLogSegmentBatches(ctx, segmentID)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		batch.SegmentState = state
		switch state {
		case "OPEN":
			batch.SegmentStorageKey = segmentID + ".open"
		case "SEALED":
			batch.SegmentStorageKey = segmentID + ".jsonl"
		default:
			return ErrStorageCorrupt
		}
		if _, err := store.readBatch(batch); err != nil {
			return err
		}
	}
	return nil
}

func (store *PersistentStore) sealWriterLocked(ctx context.Context, segmentID string, writer *SegmentWriter) error {
	if writer == nil || !validSegmentID(segmentID) {
		return ErrStorageCorrupt
	}
	byteLength, err := writer.Size()
	if err != nil {
		store.needsRecovery = true
		return err
	}
	digest, err := writer.CloseAndSeal()
	delete(store.writers, segmentID)
	if err != nil {
		store.needsRecovery = true
		return err
	}
	segment := IndexedSegment{SegmentID: segmentID, StorageKey: segmentID + ".open", ByteLength: byteLength, State: "OPEN"}
	if err := store.index.SealLogSegment(ctx, segment, digest, time.Now().UTC()); err != nil {
		// 文件已经不可变但数据库尚未确认时，下一次恢复会重新核对摘要并完成状态转换。
		store.needsRecovery = true
		return err
	}
	return nil
}

func validIndexedOpenSegment(segment IndexedSegment) bool {
	return validSegmentID(segment.SegmentID) && segment.StorageKey == segment.SegmentID+".open" && segment.ByteLength >= 0 && segment.State == "OPEN"
}

func validSegmentID(value string) bool {
	if len(value) != 64 || strings.ContainsAny(value, `/\\`) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func truncateOpenSegment(path string, length int64) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open active segment for recovery: %w", err)
	}
	defer file.Close()
	if err := file.Truncate(length); err != nil {
		return fmt.Errorf("truncate unregistered log tail: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("fsync recovered log segment: %w", err)
	}
	return nil
}

func segmentDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open sealed log segment: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash sealed log segment: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (store *PersistentStore) writer(plan AppendPlan, policy Policy) (*SegmentWriter, error) {
	if plan.SegmentID == "" || plan.SegmentByteLength < 0 {
		return nil, ErrStorageCorrupt
	}
	if writer := store.writers[plan.SegmentID]; writer != nil {
		return writer, nil
	}
	path := filepath.Join(store.root, plan.SegmentID+".open")
	if info, err := os.Stat(path); err == nil {
		if info.Size() < plan.SegmentByteLength {
			return nil, ErrStorageCorrupt
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat log segment: %w", err)
	}
	writer, err := OpenSegment(store.root, plan.SegmentID, policy)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err != nil || info.Size() != plan.SegmentByteLength {
		if err == nil && info.Size() > plan.SegmentByteLength {
			err = writer.Recover(plan.SegmentByteLength)
		}
		if err != nil {
			return nil, fmt.Errorf("recover active log segment: %w", err)
		}
	}
	store.writers[plan.SegmentID] = writer
	return writer, nil
}

func (store *PersistentStore) readBatch(batch IndexedBatch) ([]Record, error) {
	if !validSegmentID(batch.SegmentID) || batch.SegmentOffsetStart < 0 || batch.SegmentOffsetEnd < batch.SegmentOffsetStart || batch.SegmentOffsetEnd-batch.SegmentOffsetStart > 1<<20 {
		return nil, ErrStorageCorrupt
	}
	var path string
	switch batch.SegmentState {
	case "OPEN":
		if batch.SegmentStorageKey != batch.SegmentID+".open" {
			return nil, ErrStorageCorrupt
		}
		path = filepath.Join(store.root, batch.SegmentID+".open")
	case "SEALED":
		if batch.SegmentStorageKey != batch.SegmentID+".jsonl" {
			return nil, ErrStorageCorrupt
		}
		path = filepath.Join(store.root, batch.SegmentID+".jsonl")
	default:
		return nil, ErrStorageCorrupt
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open indexed log segment: %w", err)
	}
	defer file.Close()
	reader := io.NewSectionReader(file, batch.SegmentOffsetStart, batch.SegmentOffsetEnd-batch.SegmentOffsetStart)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), MaxRecordBytes+1024)
	records := make([]Record, 0, batch.RecordCount)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, ErrStorageCorrupt
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil || len(records) != batch.RecordCount {
		return nil, ErrStorageCorrupt
	}
	if err := verifyIndexedBatch(batch, records); err != nil {
		return nil, err
	}
	policy := Policy{Version: batch.PolicyVersion}
	for index := range records {
		record := &records[index]
		message, err := policy.Redact(record.Message)
		if err != nil {
			return nil, err
		}
		record.Message = message
		if record.ReceivedAt.IsZero() {
			record.ReceivedAt = batch.ReceivedAt
		}
	}
	return records, nil
}

func verifyIndexedBatch(batch IndexedBatch, records []Record) error {
	if batch.GapReasonCode != "" {
		kind, err := storedSourceKind(batch.SourceKind)
		if err != nil || len(records) != 1 {
			return ErrStorageCorrupt
		}
		record := records[0]
		if record.StreamID != batch.SourceRef || record.SourceKind != kind || record.SourceEpoch != batch.SourceEpoch || record.SourceSeq != batch.FirstSequence ||
			record.Kind != RecordGap || record.Message != "日志序号存在缺口" || record.IntegrityCode != batch.GapReasonCode ||
			record.PolicyVersion != batch.PolicyVersion || record.ParserVersion != batch.ParserVersion || !record.ReceivedAt.Equal(batch.ReceivedAt) {
			return ErrStorageCorrupt
		}
		gap := GapNotice{StreamID: batch.SourceRef, SourceKind: kind, SourceEpoch: batch.SourceEpoch, FirstSeq: batch.FirstSequence, LastSeq: batch.LastSequence, ReasonCode: batch.GapReasonCode, PolicyVersion: batch.PolicyVersion, ParserVersion: batch.ParserVersion}
		if !validGapNotice(gap) || gapDigest(gap) != batch.Digest {
			return ErrStorageCorrupt
		}
		return nil
	}
	sealed, err := SealBatch(Batch{
		StreamID: batch.SourceRef, SourceEpoch: batch.SourceEpoch, FirstSeq: batch.FirstSequence, LastSeq: batch.LastSequence,
		PreviousDigest: batch.PreviousDigest, Digest: batch.Digest, PolicyVersion: batch.PolicyVersion, Records: records,
	})
	if err != nil || sealed.Digest != batch.Digest {
		return ErrStorageCorrupt
	}
	return nil
}

func storedSourceKind(value string) (SourceKind, error) {
	switch value {
	case "STDOUT":
		return SourceStdout, nil
	case "STDERR":
		return SourceStderr, nil
	case "TOOL_FILE":
		return SourceFile, nil
	default:
		return "", ErrStorageCorrupt
	}
}

func (store *PersistentStore) notify() {
	select {
	case store.updates <- struct{}{}:
	default:
	}
}

func persistedSourceKind(batch Batch) (string, error) {
	kind := SourceKind("")
	for _, record := range batch.Records {
		if record.SourceKind == "" {
			continue
		}
		if kind != "" && kind != record.SourceKind {
			return "", ErrInvalidInput
		}
		kind = record.SourceKind
	}
	if kind == "" {
		return "STDOUT", nil
	}
	return persistedSourceKindFromRecord(kind)
}

func persistedSourceKindFromRecord(kind SourceKind) (string, error) {
	switch kind {
	case SourceStdout:
		return "STDOUT", nil
	case SourceStderr:
		return "STDERR", nil
	case SourceFile:
		return "TOOL_FILE", nil
	default:
		return "", ErrInvalidInput
	}
}

func gapDigest(gap GapNotice) string {
	payload := struct {
		StreamID      string
		SourceKind    SourceKind
		SourceEpoch   int64
		FirstSeq      int64
		LastSeq       int64
		ReasonCode    string
		PolicyVersion string
		ParserVersion string
	}{
		StreamID: gap.StreamID, SourceKind: gap.SourceKind, SourceEpoch: gap.SourceEpoch,
		FirstSeq: gap.FirstSeq, LastSeq: gap.LastSeq, ReasonCode: gap.ReasonCode,
		PolicyVersion: gap.PolicyVersion, ParserVersion: gap.ParserVersion,
	}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func persistedParserVersion(batch Batch) string {
	for _, record := range batch.Records {
		if strings.TrimSpace(record.ParserVersion) != "" {
			return record.ParserVersion
		}
	}
	return "unparsed-v1"
}

func internalID(namespace string, values ...string) string {
	digest := sha256.Sum256([]byte(namespace + "|" + strings.Join(values, "|")))
	return hex.EncodeToString(digest[:])
}

func batchPosition(batch IndexedBatch) BatchPosition {
	return BatchPosition{ReceivedAt: batch.ReceivedAt.UTC(), BatchID: batch.BatchID}
}

func samePosition(left, right BatchPosition) bool {
	return left.BatchID != "" && left.BatchID == right.BatchID && left.ReceivedAt.Equal(right.ReceivedAt)
}

func comparePosition(left, right BatchPosition) int {
	if left.ReceivedAt.Before(right.ReceivedAt) {
		return -1
	}
	if left.ReceivedAt.After(right.ReceivedAt) {
		return 1
	}
	return strings.Compare(left.BatchID, right.BatchID)
}
