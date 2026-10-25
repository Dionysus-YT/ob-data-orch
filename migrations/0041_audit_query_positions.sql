-- 仅保存追加次序，既有审计正文不复制；回填和触发器与业务审计共同提交。
CREATE TABLE audit_query_positions (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    audit_event_id TEXT NOT NULL UNIQUE REFERENCES audit_events(audit_event_id)
) STRICT;

INSERT INTO audit_query_positions(audit_event_id)
SELECT audit_event_id FROM audit_events ORDER BY occurred_at, audit_event_id;

CREATE TRIGGER audit_query_position_after_insert
AFTER INSERT ON audit_events
BEGIN
    INSERT INTO audit_query_positions(audit_event_id) VALUES(NEW.audit_event_id);
END;

-- 只为现有有界时间排序建立索引，避免每页重新排序全部审计；无全文或任意差异索引。
CREATE INDEX idx_audit_query_time ON audit_events(
    (substr(occurred_at,1,19)||substr((CASE WHEN substr(occurred_at,20,1)='.' THEN substr(occurred_at,21,length(occurred_at)-21) ELSE '' END)||'000000000',1,9)),
    audit_event_id
);
