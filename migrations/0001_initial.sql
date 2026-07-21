CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY CHECK (version > 0),
    name TEXT NOT NULL CHECK (length(name) > 0),
    checksum TEXT NOT NULL CHECK (length(checksum) = 64),
    applied_at TEXT NOT NULL CHECK (length(applied_at) > 0)
) STRICT;

CREATE TABLE auth_subjects (
    subject_id TEXT PRIMARY KEY,
    external_subject TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    account_status TEXT NOT NULL CHECK (account_status IN ('ACTIVE', 'DISABLED', 'UNKNOWN')),
    directory_revision TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (length(subject_id) > 0 AND length(external_subject) > 0)
) STRICT, WITHOUT ROWID;

CREATE TABLE subject_grants (
    subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    grant_code TEXT NOT NULL CHECK (grant_code IN (
        'ROLE_BASIC_USER', 'ROLE_OPERATOR', 'ROLE_DATA_SOURCE_ADMIN',
        'ROLE_NODE_ADMIN', 'ROLE_SYSTEM_ADMIN', 'CAP_PRODUCTION_TASK',
        'CAP_EXPERT_CONFIG', 'CAP_SENSITIVE_COMMAND', 'CAP_AUDIT_LOG',
        'CAP_GLOBAL_SCHEDULING_LOG'
    )),
    granted_by TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    granted_at TEXT NOT NULL,
    PRIMARY KEY (subject_id, grant_code)
) STRICT, WITHOUT ROWID;

CREATE TABLE subject_object_scopes (
    subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    scope_type TEXT NOT NULL CHECK (scope_type IN (
        'DATA_SOURCE_USE', 'DATA_SOURCE_MANAGE', 'NODE_USE',
        'NODE_OPERATE', 'NODE_MANAGE', 'TASK_OPERATE_BY_DATA_SOURCE'
    )),
    object_id TEXT NOT NULL CHECK (length(object_id) > 0 AND object_id <> '*'),
    granted_by TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    granted_at TEXT NOT NULL,
    PRIMARY KEY (subject_id, scope_type, object_id)
) STRICT, WITHOUT ROWID;

CREATE TABLE data_sources (
    data_source_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    environment TEXT NOT NULL CHECK (environment IN ('DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION')),
    connection_kind TEXT NOT NULL CHECK (connection_kind IN ('OBSERVER_DIRECT', 'ODP', 'PUBLIC_CLOUD', 'LOGICAL_DATABASE')),
    compatibility_mode TEXT NOT NULL CHECK (compatibility_mode IN ('MYSQL', 'ORACLE', 'UNKNOWN')),
    host TEXT NOT NULL CHECK (length(host) > 0),
    port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    username TEXT NOT NULL CHECK (length(username) > 0),
    default_database TEXT,
    credential_id TEXT NOT NULL,
    current_credential_revision INTEGER NOT NULL CHECK (current_credential_revision > 0),
    state TEXT NOT NULL CHECK (state IN ('ENABLED', 'DISABLED', 'ARCHIVED')),
    revision INTEGER NOT NULL CHECK (revision > 0),
    last_test_status TEXT CHECK (last_test_status IS NULL OR last_test_status IN ('SUCCEEDED', 'FAILED', 'UNKNOWN')),
    last_tested_at TEXT,
    last_test_safe_summary_json TEXT CHECK (last_test_safe_summary_json IS NULL OR json_valid(last_test_safe_summary_json)),
    created_by TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE credential_revisions (
    credential_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    data_source_id TEXT NOT NULL REFERENCES data_sources(data_source_id),
    secret_type TEXT NOT NULL CHECK (secret_type = 'DATABASE_PASSWORD'),
    key_id TEXT NOT NULL CHECK (length(key_id) > 0),
    nonce BLOB NOT NULL CHECK (length(nonce) > 0),
    ciphertext BLOB NOT NULL CHECK (length(ciphertext) > 0),
    aad_json TEXT NOT NULL CHECK (json_valid(aad_json)),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'SUPERSEDED', 'REVOKED')),
    created_at TEXT NOT NULL,
    retired_at TEXT,
    PRIMARY KEY (credential_id, revision),
    UNIQUE (data_source_id, revision)
) STRICT, WITHOUT ROWID;

CREATE TABLE execution_nodes (
    node_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    platform TEXT NOT NULL CHECK (platform IN ('WINDOWS_AMD64', 'LINUX_AMD64', 'LINUX_ARM64')),
    management_state TEXT NOT NULL CHECK (management_state IN ('ENABLED', 'DISABLED', 'MAINTENANCE', 'ARCHIVED')),
    allowed_roots_json TEXT NOT NULL CHECK (json_valid(allowed_roots_json)),
    tool_config_ref TEXT,
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_by TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE agent_enrollment_tokens (
    enrollment_id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES execution_nodes(node_id),
    token_digest BLOB NOT NULL UNIQUE CHECK (length(token_digest) > 0),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'CONSUMED', 'EXPIRED', 'REVOKED')),
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    consumed_by_agent_id TEXT,
    created_by TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    created_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE agents (
    agent_id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES execution_nodes(node_id),
    credential_digest BLOB NOT NULL CHECK (length(credential_digest) > 0),
    credential_revision INTEGER NOT NULL CHECK (credential_revision > 0),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'REVOKED', 'REPLACED')),
    protocol_version TEXT NOT NULL,
    boot_id TEXT,
    last_heartbeat_at TEXT,
    capacity_total INTEGER NOT NULL DEFAULT 1 CHECK (capacity_total = 1),
    capacity_used INTEGER NOT NULL DEFAULT 0 CHECK (capacity_used IN (0, 1)),
    facts_json TEXT CHECK (facts_json IS NULL OR json_valid(facts_json)),
    created_at TEXT NOT NULL,
    revoked_at TEXT,
    UNIQUE (agent_id, node_id)
) STRICT, WITHOUT ROWID;

CREATE UNIQUE INDEX uq_agents_active_node ON agents(node_id) WHERE status = 'ACTIVE';

CREATE TABLE export_drafts (
    draft_id TEXT PRIMARY KEY,
    owner_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    data_source_id TEXT NOT NULL REFERENCES data_sources(data_source_id),
    node_id TEXT REFERENCES execution_nodes(node_id),
    revision INTEGER NOT NULL CHECK (revision > 0),
    tool_version TEXT NOT NULL,
    metadata_version TEXT NOT NULL,
    capability_version TEXT NOT NULL,
    config_json TEXT NOT NULL CHECK (json_valid(config_json)),
    config_fingerprint TEXT CHECK (config_fingerprint IS NULL OR length(config_fingerprint) = 64),
    invalidation_json TEXT NOT NULL CHECK (json_valid(invalidation_json)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE precheck_runs (
    precheck_id TEXT PRIMARY KEY,
    draft_id TEXT NOT NULL REFERENCES export_drafts(draft_id),
    draft_revision INTEGER NOT NULL CHECK (draft_revision > 0),
    config_fingerprint TEXT NOT NULL CHECK (length(config_fingerprint) = 64),
    data_source_id TEXT NOT NULL REFERENCES data_sources(data_source_id),
    credential_id TEXT NOT NULL,
    credential_revision INTEGER NOT NULL CHECK (credential_revision > 0),
    node_id TEXT NOT NULL REFERENCES execution_nodes(node_id),
    agent_id TEXT REFERENCES agents(agent_id),
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'LEASED', 'SUCCEEDED', 'FAILED', 'EXPIRED', 'INVALIDATED')),
    lease_id TEXT,
    lease_epoch INTEGER CHECK (lease_epoch IS NULL OR lease_epoch > 0),
    lease_expires_at TEXT,
    result_json TEXT CHECK (result_json IS NULL OR json_valid(result_json)),
    integrity_status TEXT NOT NULL CHECK (integrity_status IN ('UNKNOWN', 'COMPLETE', 'INCOMPLETE')),
    valid_until TEXT,
    created_at TEXT NOT NULL,
    completed_at TEXT,
    FOREIGN KEY (credential_id, credential_revision) REFERENCES credential_revisions(credential_id, revision),
    CHECK ((lease_id IS NULL AND lease_epoch IS NULL) OR (lease_id IS NOT NULL AND lease_epoch IS NOT NULL))
) STRICT, WITHOUT ROWID;

CREATE TABLE tasks (
    task_id TEXT PRIMARY KEY,
    creator_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    data_source_id TEXT NOT NULL REFERENCES data_sources(data_source_id),
    node_id TEXT NOT NULL REFERENCES execution_nodes(node_id),
    precheck_id TEXT NOT NULL REFERENCES precheck_runs(precheck_id),
    credential_id TEXT NOT NULL,
    credential_revision INTEGER NOT NULL CHECK (credential_revision > 0),
    config_fingerprint TEXT NOT NULL CHECK (length(config_fingerprint) = 64),
    tool_version TEXT NOT NULL,
    metadata_version TEXT NOT NULL,
    capability_version TEXT NOT NULL,
    snapshot_json TEXT NOT NULL CHECK (json_valid(snapshot_json)),
    planned_argv_json TEXT NOT NULL CHECK (json_valid(planned_argv_json)),
    planned_command_redacted TEXT NOT NULL,
    submitted_at TEXT NOT NULL,
    FOREIGN KEY (credential_id, credential_revision) REFERENCES credential_revisions(credential_id, revision)
) STRICT, WITHOUT ROWID;

CREATE TABLE task_executions (
    execution_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL UNIQUE REFERENCES tasks(task_id),
    node_id TEXT NOT NULL REFERENCES execution_nodes(node_id),
    agent_id TEXT REFERENCES agents(agent_id),
    state TEXT NOT NULL CHECK (state IN ('STARTING', 'RUNNING', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED')),
    revision INTEGER NOT NULL CHECK (revision > 0),
    reconciliation_required INTEGER NOT NULL DEFAULT 0 CHECK (reconciliation_required IN (0, 1)),
    process_evidence_json TEXT CHECK (process_evidence_json IS NULL OR json_valid(process_evidence_json)),
    result_summary_json TEXT CHECK (result_summary_json IS NULL OR json_valid(result_summary_json)),
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE execution_leases (
    lease_id TEXT PRIMARY KEY,
    execution_id TEXT NOT NULL REFERENCES task_executions(execution_id),
    lease_epoch INTEGER NOT NULL CHECK (lease_epoch > 0),
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    status TEXT NOT NULL CHECK (status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE', 'EXPIRED', 'RELEASED', 'REVOKED')),
    issued_at TEXT NOT NULL,
    acknowledged_at TEXT,
    expires_at TEXT NOT NULL,
    released_at TEXT,
    UNIQUE (execution_id, lease_epoch),
    UNIQUE (execution_id, lease_id, lease_epoch)
) STRICT, WITHOUT ROWID;

CREATE TABLE execution_events (
    event_id TEXT PRIMARY KEY,
    execution_id TEXT NOT NULL,
    lease_id TEXT NOT NULL,
    lease_epoch INTEGER NOT NULL CHECK (lease_epoch > 0),
    event_seq INTEGER NOT NULL CHECK (event_seq > 0),
    event_type TEXT NOT NULL CHECK (length(event_type) > 0),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    received_at TEXT NOT NULL,
    accepted INTEGER NOT NULL CHECK (accepted IN (0, 1)),
    rejection_code TEXT,
    UNIQUE (execution_id, event_seq),
    FOREIGN KEY (execution_id, lease_id, lease_epoch)
        REFERENCES execution_leases(execution_id, lease_id, lease_epoch)
) STRICT, WITHOUT ROWID;

CREATE TABLE request_idempotency (
    subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    operation TEXT NOT NULL CHECK (length(operation) > 0),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) > 0),
    request_digest TEXT NOT NULL CHECK (length(request_digest) = 64),
    result_status INTEGER NOT NULL CHECK (result_status BETWEEN 100 AND 599),
    resource_kind TEXT,
    resource_id TEXT,
    response_json TEXT CHECK (response_json IS NULL OR json_valid(response_json)),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (subject_id, operation, idempotency_key)
) STRICT, WITHOUT ROWID;

CREATE TABLE audit_events (
    audit_event_id TEXT PRIMARY KEY,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('SUBJECT', 'AGENT', 'SYSTEM')),
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK (length(action) > 0),
    object_type TEXT NOT NULL CHECK (length(object_type) > 0),
    object_id TEXT,
    result TEXT NOT NULL CHECK (result IN ('SUCCEEDED', 'FAILED', 'DENIED')),
    request_id TEXT NOT NULL,
    safe_diff_json TEXT NOT NULL CHECK (json_valid(safe_diff_json)),
    occurred_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE log_streams (
    stream_id TEXT PRIMARY KEY,
    execution_id TEXT NOT NULL REFERENCES task_executions(execution_id),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('STDOUT', 'STDERR', 'TOOL_FILE', 'AGENT_EVENT')),
    source_ref TEXT NOT NULL,
    stream_epoch INTEGER NOT NULL CHECK (stream_epoch > 0),
    expected_sequence INTEGER NOT NULL CHECK (expected_sequence > 0),
    collection_status TEXT NOT NULL CHECK (collection_status IN ('OPEN', 'CLOSED', 'FAILED')),
    integrity_status TEXT NOT NULL CHECK (integrity_status IN ('COMPLETE', 'GAPPED', 'UNKNOWN')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (execution_id, source_kind, source_ref, stream_epoch)
) STRICT, WITHOUT ROWID;

CREATE TABLE log_segments (
    segment_id TEXT PRIMARY KEY,
    stream_id TEXT NOT NULL REFERENCES log_streams(stream_id),
    segment_ordinal INTEGER NOT NULL CHECK (segment_ordinal >= 0),
    storage_key TEXT NOT NULL CHECK (
        length(storage_key) > 0
        AND substr(storage_key, 1, 1) <> '/'
        AND instr(storage_key, ':') = 0
        AND instr(storage_key, '..') = 0
    ),
    first_sequence INTEGER NOT NULL CHECK (first_sequence > 0),
    last_sequence INTEGER CHECK (last_sequence IS NULL OR last_sequence >= first_sequence),
    byte_length INTEGER NOT NULL DEFAULT 0 CHECK (byte_length >= 0),
    content_digest TEXT CHECK (content_digest IS NULL OR length(content_digest) = 64),
    state TEXT NOT NULL CHECK (state IN ('OPEN', 'SEALED')),
    retain_until TEXT NOT NULL,
    created_at TEXT NOT NULL,
    sealed_at TEXT,
    UNIQUE (stream_id, segment_ordinal),
    CHECK ((state = 'OPEN' AND sealed_at IS NULL) OR (state = 'SEALED' AND sealed_at IS NOT NULL))
) STRICT, WITHOUT ROWID;

CREATE TABLE log_batches (
    batch_id TEXT PRIMARY KEY,
    stream_id TEXT NOT NULL REFERENCES log_streams(stream_id),
    stream_epoch INTEGER NOT NULL CHECK (stream_epoch > 0),
    first_sequence INTEGER NOT NULL CHECK (first_sequence > 0),
    last_sequence INTEGER NOT NULL CHECK (last_sequence >= first_sequence),
    batch_digest TEXT NOT NULL CHECK (length(batch_digest) = 64),
    segment_id TEXT NOT NULL REFERENCES log_segments(segment_id),
    segment_offset_start INTEGER NOT NULL CHECK (segment_offset_start >= 0),
    segment_offset_end INTEGER NOT NULL CHECK (segment_offset_end >= segment_offset_start),
    gap_summary_json TEXT CHECK (gap_summary_json IS NULL OR json_valid(gap_summary_json)),
    received_at TEXT NOT NULL,
    UNIQUE (stream_id, stream_epoch, first_sequence)
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_data_sources_list ON data_sources(state, environment, normalized_name);
CREATE INDEX idx_export_drafts_owner_updated ON export_drafts(owner_subject_id, updated_at DESC);
CREATE INDEX idx_precheck_draft_revision ON precheck_runs(draft_id, draft_revision);
CREATE INDEX idx_precheck_node_status_expiry ON precheck_runs(node_id, status, valid_until);
CREATE INDEX idx_tasks_creator_created ON tasks(creator_subject_id, submitted_at DESC);
CREATE INDEX idx_tasks_data_source_created ON tasks(data_source_id, submitted_at DESC);
CREATE INDEX idx_task_executions_node_state ON task_executions(node_id, state);
CREATE INDEX idx_execution_leases_expiry ON execution_leases(status, expires_at);
CREATE INDEX idx_execution_events_sequence ON execution_events(execution_id, event_seq);
CREATE INDEX idx_request_idempotency_expiry ON request_idempotency(expires_at);
CREATE INDEX idx_audit_actor_time ON audit_events(actor_type, actor_id, occurred_at DESC);
CREATE INDEX idx_audit_object_time ON audit_events(object_type, object_id, occurred_at DESC);
CREATE INDEX idx_log_streams_execution ON log_streams(execution_id, source_kind, stream_epoch);
CREATE INDEX idx_log_segments_retention ON log_segments(state, retain_until);

CREATE TRIGGER schema_migrations_no_update
BEFORE UPDATE ON schema_migrations
BEGIN
    SELECT RAISE(ABORT, 'migration history is immutable');
END;

CREATE TRIGGER schema_migrations_no_delete
BEFORE DELETE ON schema_migrations
BEGIN
    SELECT RAISE(ABORT, 'migration history is immutable');
END;

CREATE TRIGGER tasks_no_update
BEFORE UPDATE ON tasks
BEGIN
    SELECT RAISE(ABORT, 'tasks are immutable');
END;

CREATE TRIGGER tasks_no_delete
BEFORE DELETE ON tasks
BEGIN
    SELECT RAISE(ABORT, 'tasks are immutable');
END;

CREATE TRIGGER execution_events_no_update
BEFORE UPDATE ON execution_events
BEGIN
    SELECT RAISE(ABORT, 'execution events are append-only');
END;

CREATE TRIGGER execution_events_no_delete
BEFORE DELETE ON execution_events
BEGIN
    SELECT RAISE(ABORT, 'execution events are append-only');
END;

CREATE TRIGGER audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit events are append-only');
END;

CREATE TRIGGER audit_events_no_delete
BEFORE DELETE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit events are append-only');
END;

CREATE TRIGGER sealed_log_segments_no_update
BEFORE UPDATE ON log_segments
WHEN OLD.state = 'SEALED'
BEGIN
    SELECT RAISE(ABORT, 'sealed log segments are immutable');
END;

CREATE TRIGGER sealed_log_segments_no_delete
BEFORE DELETE ON log_segments
WHEN OLD.state = 'SEALED'
BEGIN
    SELECT RAISE(ABORT, 'sealed log segments are immutable');
END;
