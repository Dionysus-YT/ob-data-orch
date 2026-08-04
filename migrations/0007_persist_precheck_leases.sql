ALTER TABLE precheck_runs ADD COLUMN node_facts_revision INTEGER NOT NULL DEFAULT 0 CHECK (node_facts_revision >= 0);
ALTER TABLE precheck_runs ADD COLUMN binding_digest TEXT CHECK (binding_digest IS NULL OR length(binding_digest) = 64);
ALTER TABLE precheck_runs ADD COLUMN binding_agent_id TEXT REFERENCES agents(agent_id);

CREATE TABLE agent_precheck_receipts (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    request_id TEXT NOT NULL CHECK (length(request_id) BETWEEN 1 AND 256),
    operation TEXT NOT NULL CHECK (operation IN ('CLAIM', 'ACKNOWLEDGE', 'COMPLETE')),
    request_digest TEXT NOT NULL CHECK (length(request_digest) = 64),
    precheck_id TEXT NOT NULL REFERENCES precheck_runs(precheck_id),
    lease_id TEXT NOT NULL CHECK (length(lease_id) BETWEEN 1 AND 256),
    lease_epoch INTEGER NOT NULL CHECK (lease_epoch > 0),
    binding_digest TEXT NOT NULL CHECK (length(binding_digest) = 64),
    status TEXT NOT NULL CHECK (status IN ('LEASED', 'ACKNOWLEDGED', 'SUCCEEDED', 'FAILED', 'EXPIRED')),
    expires_at TEXT NOT NULL,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (agent_id, request_id),
    CHECK (
        (operation = 'COMPLETE' AND completed_at IS NOT NULL AND status IN ('SUCCEEDED', 'FAILED', 'EXPIRED'))
        OR (operation = 'CLAIM' AND completed_at IS NULL AND status = 'LEASED')
        OR (operation = 'ACKNOWLEDGE' AND completed_at IS NULL AND status = 'ACKNOWLEDGED')
    )
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_agent_precheck_receipts_precheck ON agent_precheck_receipts(precheck_id, operation, created_at);
