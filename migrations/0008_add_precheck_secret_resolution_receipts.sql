CREATE TABLE agent_precheck_secret_resolution_receipts (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    request_id TEXT NOT NULL CHECK (length(request_id) BETWEEN 1 AND 256),
    request_digest TEXT NOT NULL CHECK (length(request_digest) = 64),
    precheck_id TEXT NOT NULL REFERENCES precheck_runs(precheck_id),
    lease_id TEXT NOT NULL CHECK (length(lease_id) BETWEEN 1 AND 256),
    lease_epoch INTEGER NOT NULL CHECK (lease_epoch > 0),
    binding_digest TEXT NOT NULL CHECK (length(binding_digest) = 64),
    status TEXT NOT NULL CHECK (status IN ('AUTHORIZED', 'RESOLVED', 'FAILED')),
    created_at TEXT NOT NULL,
    completed_at TEXT,
    PRIMARY KEY (agent_id, request_id),
    CHECK (
        (status = 'AUTHORIZED' AND completed_at IS NULL)
        OR (status IN ('RESOLVED', 'FAILED') AND completed_at IS NOT NULL)
    )
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_agent_precheck_secret_resolution_precheck
ON agent_precheck_secret_resolution_receipts(precheck_id, created_at);
