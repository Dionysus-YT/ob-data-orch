-- 查询水位只索引已登记批次，不保存正文；AUTOINCREMENT 防止清理后重用旧快照位置。
CREATE TABLE log_query_positions (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id TEXT NOT NULL UNIQUE REFERENCES log_batches(batch_id) ON DELETE CASCADE
) STRICT;

INSERT INTO log_query_positions(batch_id)
SELECT batch_id FROM log_batches ORDER BY received_at, batch_id;
