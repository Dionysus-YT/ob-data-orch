ALTER TABLE data_sources ADD COLUMN last_test_source TEXT CHECK (
    last_test_source IS NULL
    OR last_test_source = 'AGENT_JDBC'
);

CREATE TABLE data_source_connection_test_runs (
    connection_test_id TEXT PRIMARY KEY CHECK (length(connection_test_id) BETWEEN 1 AND 256),
    data_source_id TEXT NOT NULL REFERENCES data_sources(data_source_id),
    creator_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    connection_config_digest TEXT NOT NULL CHECK (length(connection_config_digest) = 64),
    credential_id TEXT NOT NULL CHECK (length(credential_id) BETWEEN 1 AND 256),
    credential_revision INTEGER NOT NULL CHECK (credential_revision > 0),
    node_id TEXT NOT NULL REFERENCES execution_nodes(node_id),
    binding_agent_id TEXT NOT NULL CHECK (length(binding_agent_id) BETWEEN 1 AND 256),
    node_facts_revision INTEGER NOT NULL CHECK (node_facts_revision > 0),
    binding_digest TEXT NOT NULL CHECK (length(binding_digest) = 64),
    status TEXT NOT NULL CHECK (status IN (
        'PENDING', 'LEASED', 'SUCCEEDED', 'FAILED', 'UNKNOWN', 'EXPIRED', 'INVALIDATED'
    )),
    verification_source TEXT NOT NULL CHECK (verification_source IN ('G2_SYNTHETIC', 'AGENT_JDBC')),
    lease_id TEXT,
    lease_epoch INTEGER CHECK (lease_epoch IS NULL OR lease_epoch > 0),
    lease_expires_at TEXT,
    result_code TEXT CHECK (
        result_code IS NULL
        OR (length(result_code) BETWEEN 1 AND 128 AND result_code NOT GLOB '*[^A-Z0-9_]*')
    ),
    safe_summary_json TEXT CHECK (safe_summary_json IS NULL OR json_valid(safe_summary_json)),
    created_at TEXT NOT NULL,
    completed_at TEXT,
    valid_until TEXT,
    FOREIGN KEY (credential_id, credential_revision)
        REFERENCES credential_revisions(credential_id, revision),
    FOREIGN KEY (binding_agent_id, node_id)
        REFERENCES agents(agent_id, node_id),
    CHECK (
        (lease_id IS NULL AND lease_epoch IS NULL AND lease_expires_at IS NULL)
        OR (
            lease_id IS NOT NULL
            AND length(lease_id) BETWEEN 1 AND 256
            AND lease_epoch IS NOT NULL
            AND lease_expires_at IS NOT NULL
        )
    ),
    CHECK (
        (status IN ('PENDING', 'LEASED') AND completed_at IS NULL)
        OR (status IN ('SUCCEEDED', 'FAILED', 'UNKNOWN', 'EXPIRED', 'INVALIDATED') AND completed_at IS NOT NULL)
    )
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_data_source_connection_test_runs_node_status_created
ON data_source_connection_test_runs(node_id, status, created_at);

CREATE INDEX idx_data_source_connection_test_runs_source_created
ON data_source_connection_test_runs(data_source_id, created_at DESC);

CREATE TABLE agent_data_source_connection_test_receipts (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    request_id TEXT NOT NULL CHECK (length(request_id) BETWEEN 1 AND 256),
    operation TEXT NOT NULL CHECK (operation IN ('CLAIM', 'ACKNOWLEDGE', 'COMPLETE')),
    request_digest TEXT NOT NULL CHECK (length(request_digest) = 64),
    connection_test_id TEXT NOT NULL REFERENCES data_source_connection_test_runs(connection_test_id),
    lease_id TEXT NOT NULL CHECK (length(lease_id) BETWEEN 1 AND 256),
    lease_epoch INTEGER NOT NULL CHECK (lease_epoch > 0),
    binding_digest TEXT NOT NULL CHECK (length(binding_digest) = 64),
    status TEXT NOT NULL CHECK (status IN (
        'LEASED', 'ACKNOWLEDGED', 'SUCCEEDED', 'FAILED', 'UNKNOWN', 'EXPIRED', 'INVALIDATED'
    )),
    expires_at TEXT NOT NULL,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (agent_id, request_id),
    CHECK (
        (operation = 'CLAIM' AND status = 'LEASED' AND completed_at IS NULL)
        OR (operation = 'ACKNOWLEDGE' AND status = 'ACKNOWLEDGED' AND completed_at IS NULL)
        OR (
            operation = 'COMPLETE'
            AND status IN ('SUCCEEDED', 'FAILED', 'UNKNOWN', 'EXPIRED', 'INVALIDATED')
            AND completed_at IS NOT NULL
        )
    )
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_agent_data_source_connection_test_receipts_test
ON agent_data_source_connection_test_receipts(connection_test_id, operation, created_at);

CREATE TABLE agent_data_source_connection_test_secret_resolution_receipts (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    request_id TEXT NOT NULL CHECK (length(request_id) BETWEEN 1 AND 256),
    request_digest TEXT NOT NULL CHECK (length(request_digest) = 64),
    connection_test_id TEXT NOT NULL REFERENCES data_source_connection_test_runs(connection_test_id),
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

CREATE INDEX idx_agent_data_source_connection_test_secret_resolution_receipts_test
ON agent_data_source_connection_test_secret_resolution_receipts(connection_test_id, created_at);
