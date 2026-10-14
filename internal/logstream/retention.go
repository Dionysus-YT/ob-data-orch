package logstream

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// DefaultRetentionDays 是 DEC-050 确认的普通任务日志默认保留天数。
const DefaultRetentionDays = 30

// ErrRetentionExpired 表示存在已登记的正常保留清理事实，不等同于文件损坏或采集缺口。
var ErrRetentionExpired = errors.New("task logs exceeded retention")

// CleanupSegment 是 SQLite 冻结的清理对象，只允许控制面日志根内的内部段名。
type CleanupSegment struct {
	SegmentID    string
	TaskID       string
	StorageKey   string
	SegmentState string
	ByteLength   int64
	Digest       string
	// Pending 表示此前已经提交 DELETING，重启后文件缺失可按已登记清理继续确认。
	Pending bool
}

// RetentionIndex 隔离短事务与文件删除；数据库适配层复验终态、结束时间和核对资格。
type RetentionIndex interface {
	ListLogCleanup(context.Context, time.Time, int) ([]CleanupSegment, error)
	BeginLogCleanup(context.Context, CleanupSegment, time.Time, time.Time) (bool, error)
	CompleteLogCleanup(context.Context, string, time.Time) error
	TaskLogsExpired(context.Context, string) (bool, error)
}

// RetentionDays 兼容旧部署配置的零值，其他值必须处于已确认的 7–365 天范围。
func RetentionDays(value int) (int, error) {
	if value == 0 {
		return DefaultRetentionDays, nil
	}
	if value < 7 || value > 365 {
		return 0, ErrInvalidInput
	}
	return value, nil
}

// CleanupExpired 分批清理已过保留期的终态日志，既不删除任务、审计和结果，也不访问 Agent 文件。
// 顺序固定为核验受控文件、提交 DELETING、删除文件、确认索引；读日志与删除互斥。
func (store *PersistentStore) CleanupExpired(ctx context.Context, now time.Time, days, limit int) (int, error) {
	if store == nil {
		return 0, ErrInvalidInput
	}
	days, err := RetentionDays(days)
	if err != nil || now.IsZero() || limit < 1 || limit > 128 {
		return 0, ErrInvalidInput
	}
	index, ok := store.index.(RetentionIndex)
	if !ok {
		return 0, ErrInvalidInput
	}
	store.readMu.Lock()
	defer store.readMu.Unlock()
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.needsRecovery {
		if err := store.recoverLocked(ctx); err != nil {
			return 0, err
		}
	}
	cutoff := now.UTC().AddDate(0, 0, -days)
	segments, err := index.ListLogCleanup(ctx, cutoff, limit)
	if err != nil {
		return 0, err
	}
	completed := 0
	var firstError error
	for _, segment := range segments {
		if err := ctx.Err(); err != nil {
			return completed, err
		}
		if err := store.cleanupSegmentLocked(ctx, index, segment, cutoff, now); err != nil {
			if firstError == nil {
				firstError = err
			}
		} else {
			completed++
		}
	}
	return completed, firstError
}

func (store *PersistentStore) cleanupSegmentLocked(ctx context.Context, index RetentionIndex, segment CleanupSegment, cutoff, now time.Time) error {
	if !validSegmentID(segment.SegmentID) || segment.TaskID == "" || segment.ByteLength < 0 {
		return ErrStorageCorrupt
	}
	suffix := ".open"
	if segment.SegmentState == "SEALED" {
		suffix = ".jsonl"
	} else if segment.SegmentState != "OPEN" {
		return ErrStorageCorrupt
	}
	if segment.StorageKey != segment.SegmentID+suffix {
		return ErrStorageCorrupt
	}
	if writer := store.writers[segment.SegmentID]; writer != nil {
		if err := writer.Close(); err != nil {
			return err
		}
		delete(store.writers, segment.SegmentID)
	}
	path := filepath.Join(store.root, segment.StorageKey)
	info, err := os.Lstat(path)
	if err != nil && (!os.IsNotExist(err) || !segment.Pending) {
		return ErrStorageCorrupt
	}
	if err == nil {
		// 不跟随符号链接或目录；仅删除与冻结大小、批次摘要及封段摘要一致的普通文件。
		if !info.Mode().IsRegular() || info.Size() != segment.ByteLength {
			return ErrStorageCorrupt
		}
		if err := store.verifyIndexedSegment(ctx, segment.SegmentID, segment.SegmentState); err != nil {
			return err
		}
		if segment.SegmentState == "SEALED" {
			digest, err := segmentDigest(path)
			if err != nil || digest != segment.Digest {
				return ErrStorageCorrupt
			}
		}
	}
	if !segment.Pending {
		marked, err := index.BeginLogCleanup(ctx, segment, cutoff, now.UTC())
		if err != nil {
			return err
		}
		if !marked {
			return nil
		}
	}
	// 只有 DELETING 已提交后，缺失文件才可能由本次或前次清理产生。
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return index.CompleteLogCleanup(ctx, segment.SegmentID, now.UTC())
}

func (store *PersistentStore) resumeCleanupLocked(ctx context.Context) error {
	index, ok := store.index.(RetentionIndex)
	if !ok {
		return nil
	}
	for {
		segments, err := index.ListLogCleanup(ctx, time.Time{}, 64)
		if err != nil {
			return err
		}
		if len(segments) == 0 {
			return nil
		}
		for _, segment := range segments {
			if err := store.cleanupSegmentLocked(ctx, index, segment, time.Time{}, time.Now().UTC()); err != nil {
				return err
			}
		}
	}
}

func (store *PersistentStore) retentionError(ctx context.Context, taskID string) error {
	index, ok := store.index.(RetentionIndex)
	if !ok {
		return nil
	}
	expired, err := index.TaskLogsExpired(ctx, taskID)
	if err != nil {
		return err
	}
	if expired {
		return ErrRetentionExpired
	}
	return nil
}

// RunRetention 由组合根管理生命周期，启动与每小时分批清理，失败保留账本供下一轮或重启恢复。
func (store *PersistentStore) RunRetention(ctx context.Context, days int, report func(error)) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		for {
			count, err := store.CleanupExpired(ctx, time.Now().UTC(), days, 64)
			if err != nil {
				if ctx.Err() == nil && report != nil {
					report(err)
				}
				break
			}
			if count < 64 || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
