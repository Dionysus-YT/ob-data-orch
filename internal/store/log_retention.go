package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ob-data-orch/internal/logstream"
)

// ListLogCleanup 返回已有清理意图及按任务结束时间到期的段；零截止时间只读取待恢复意图。
func (s *Store) ListLogCleanup(ctx context.Context, cutoff time.Time, limit int) ([]logstream.CleanupSegment, error) {
	if limit < 1 || limit > 128 {
		return nil, logstream.ErrInvalidInput
	}
	cutoffText := ""
	if !cutoff.IsZero() {
		cutoffText = utcText(cutoff)
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT seg.segment_id, e.task_id, seg.storage_key, seg.state, seg.byte_length,
               COALESCE(seg.content_digest, ''), COALESCE(c.state, '') = 'DELETING'
        FROM log_segments seg JOIN log_streams stream ON stream.stream_id = seg.stream_id
        JOIN task_executions e ON e.execution_id = stream.execution_id
        LEFT JOIN log_segment_cleanup c ON c.segment_id = seg.segment_id
        WHERE c.state = 'DELETING' OR (c.segment_id IS NULL AND ? <> ''
          AND e.state IN ('SUCCEEDED','FAILED','CANCELLED') AND e.reconciliation_required = 0
          AND e.finished_at IS NOT NULL AND julianday(e.finished_at) <= julianday(?))
        ORDER BY COALESCE(c.marked_at, e.finished_at), seg.segment_id LIMIT ?`, cutoffText, cutoffText, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]logstream.CleanupSegment, 0)
	for rows.Next() {
		var segment logstream.CleanupSegment
		if err := rows.Scan(&segment.SegmentID, &segment.TaskID, &segment.StorageKey, &segment.SegmentState, &segment.ByteLength, &segment.Digest, &segment.Pending); err != nil {
			return nil, err
		}
		result = append(result, segment)
	}
	return result, rows.Err()
}

// BeginLogCleanup 在短事务中复验终态、核对标识、结束时间和已核验的段事实，不能据占位 retain_until 删除。
func (s *Store) BeginLogCleanup(ctx context.Context, segment logstream.CleanupSegment, cutoff, now time.Time) (bool, error) {
	if cutoff.IsZero() || now.IsZero() {
		return false, logstream.ErrInvalidInput
	}
	marked := false
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
            INSERT INTO log_segment_cleanup(segment_id, task_id, storage_key, segment_state, byte_length, content_digest, state, marked_at)
            SELECT seg.segment_id, e.task_id, seg.storage_key, seg.state, seg.byte_length, seg.content_digest, 'DELETING', ?
            FROM log_segments seg JOIN log_streams stream ON stream.stream_id = seg.stream_id
            JOIN task_executions e ON e.execution_id = stream.execution_id
            WHERE seg.segment_id = ? AND e.task_id = ? AND seg.storage_key = ? AND seg.state = ? AND seg.byte_length = ?
              AND COALESCE(seg.content_digest, '') = ? AND e.state IN ('SUCCEEDED','FAILED','CANCELLED')
              AND e.reconciliation_required = 0 AND e.finished_at IS NOT NULL AND julianday(e.finished_at) <= julianday(?)
              AND NOT EXISTS (SELECT 1 FROM log_segment_cleanup c WHERE c.segment_id = seg.segment_id)`,
			utcText(now), segment.SegmentID, segment.TaskID, segment.StorageKey, segment.SegmentState, segment.ByteLength, segment.Digest, utcText(cutoff))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		marked = count == 1
		return err
	})
	return marked, err
}

// CompleteLogCleanup 只确认已提交 DELETING 的段；删除批次、段与空来源索引后保留任务级清理证据。
func (s *Store) CompleteLogCleanup(ctx context.Context, segmentID string, now time.Time) error {
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM log_segment_cleanup WHERE segment_id = ?`, segmentID).Scan(&state); err != nil {
			return err
		}
		if state == "DELETED" {
			return nil
		}
		if state != "DELETING" {
			return logstream.ErrStorageCorrupt
		}
		var streamID string
		if err := tx.QueryRowContext(ctx, `SELECT stream_id FROM log_segments WHERE segment_id = ?`, segmentID).Scan(&streamID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM log_batches WHERE segment_id = ?`, segmentID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM log_segments WHERE segment_id = ?`, segmentID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM log_streams WHERE stream_id = ? AND NOT EXISTS (SELECT 1 FROM log_segments WHERE stream_id = ?)`, streamID, streamID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE log_segment_cleanup SET state = 'DELETED', deleted_at = ? WHERE segment_id = ? AND state = 'DELETING'`, utcText(now), segmentID)
		return err
	})
}

// TaskLogsExpired 只读取已登记正常清理事实，不把未知缺失或损坏推断为过期。
func (s *Store) TaskLogsExpired(ctx context.Context, taskID string) (bool, error) {
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM log_segment_cleanup WHERE task_id = ? LIMIT 1`, taskID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return found == 1, err
}
