-- 清理账本只保存受控段标识和清理证据，不保存日志正文、执行节点文件或审计内容。
CREATE TABLE log_segment_cleanup (
    segment_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    storage_key TEXT NOT NULL,
    segment_state TEXT NOT NULL CHECK (segment_state IN ('OPEN', 'SEALED')),
    byte_length INTEGER NOT NULL CHECK (byte_length >= 0),
    content_digest TEXT,
    state TEXT NOT NULL CHECK (state IN ('DELETING', 'DELETED')),
    marked_at TEXT NOT NULL,
    deleted_at TEXT,
    CHECK ((state = 'DELETING' AND deleted_at IS NULL) OR (state = 'DELETED' AND deleted_at IS NOT NULL))
) STRICT, WITHOUT ROWID;
CREATE INDEX idx_log_segment_cleanup_task ON log_segment_cleanup(task_id, state);

-- 封段内容继续不可改写；只允许先登记保留清理意图后删除其索引。
DROP TRIGGER sealed_log_segments_no_delete;
CREATE TRIGGER sealed_log_segments_no_delete
BEFORE DELETE ON log_segments
WHEN OLD.state = 'SEALED' AND NOT EXISTS (
    SELECT 1 FROM log_segment_cleanup c
    WHERE c.segment_id = OLD.segment_id AND c.state = 'DELETING'
      AND c.storage_key = OLD.storage_key AND c.byte_length = OLD.byte_length
      AND c.content_digest IS OLD.content_digest
)
BEGIN
    SELECT RAISE(ABORT, 'sealed log segments require retention cleanup');
END;

CREATE TRIGGER log_segment_cleanup_no_delete
BEFORE DELETE ON log_segment_cleanup
BEGIN
    SELECT RAISE(ABORT, 'log cleanup evidence is immutable');
END;
CREATE TRIGGER log_segment_cleanup_immutable
BEFORE UPDATE ON log_segment_cleanup
WHEN OLD.state <> 'DELETING' OR NEW.state <> 'DELETED'
 OR NEW.segment_id <> OLD.segment_id OR NEW.task_id <> OLD.task_id
 OR NEW.storage_key <> OLD.storage_key OR NEW.segment_state <> OLD.segment_state
 OR NEW.byte_length <> OLD.byte_length OR NEW.content_digest IS NOT OLD.content_digest
 OR NEW.marked_at <> OLD.marked_at
BEGIN
    SELECT RAISE(ABORT, 'log cleanup evidence is immutable');
END;
