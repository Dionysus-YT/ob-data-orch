-- 正式 execution 槽位解析需要跨控制面重启保持同一请求的授权与完成事实，
-- 因此不能复用预检查回执，也不能把明文或密文写入该表。
CREATE TABLE execution_secret_resolution_receipts (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    request_id TEXT NOT NULL,
    request_digest TEXT NOT NULL CHECK (length(request_digest) = 64),
    execution_id TEXT NOT NULL REFERENCES task_executions(execution_id),
    lease_id TEXT NOT NULL,
    lease_epoch INTEGER NOT NULL CHECK (lease_epoch > 0),
    envelope_digest TEXT NOT NULL CHECK (length(envelope_digest) = 64),
    status TEXT NOT NULL CHECK (status IN ('AUTHORIZED', 'RESOLVED', 'FAILED')),
    created_at TEXT NOT NULL,
    completed_at TEXT,
    PRIMARY KEY (agent_id, request_id),
    FOREIGN KEY (execution_id, lease_id, lease_epoch)
        REFERENCES execution_leases(execution_id, lease_id, lease_epoch)
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_execution_secret_resolution_receipts_execution
    ON execution_secret_resolution_receipts(execution_id, lease_id, lease_epoch);
