-- 分段文件仍是日志正文的唯一持久位置；这些列只补齐查询、完整性和策略版本所需的索引事实。
ALTER TABLE log_segments ADD COLUMN record_count INTEGER NOT NULL DEFAULT 0 CHECK (record_count >= 0);
ALTER TABLE log_batches ADD COLUMN policy_version TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE log_batches ADD COLUMN parser_version TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE log_batches ADD COLUMN record_count INTEGER NOT NULL DEFAULT 0 CHECK (record_count >= 0);

CREATE INDEX idx_log_batches_stream_received
    ON log_batches(stream_id, received_at, batch_id);
