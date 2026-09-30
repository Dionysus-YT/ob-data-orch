-- 0021：历史草稿、预检查、任务和连接测试保留来源标识，不再外键依赖可删除的数据源及凭据。

-- 迁移器在事务提交前检查全部外键，再解除重建表产生的延迟约束计数。

PRAGMA defer_foreign_keys = ON;

CREATE TABLE export_drafts_replacement (
    draft_id TEXT PRIMARY KEY,
    owner_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    data_source_id TEXT NOT NULL,
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
, config_version TEXT NOT NULL DEFAULT 'v5', object_scope_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(object_scope_json)), content_selection_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(content_selection_json)), data_format_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data_format_json)), output_config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output_config_json)), performance_config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(performance_config_json)), filter_config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(filter_config_json)), ddl_behavior_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(ddl_behavior_json)), source_task_id TEXT, source_derivation TEXT CHECK (
    source_derivation IS NULL OR source_derivation IN ('REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH')
)) STRICT, WITHOUT ROWID;

INSERT INTO export_drafts_replacement ("draft_id", "owner_subject_id", "data_source_id", "node_id", "revision", "tool_version", "metadata_version", "capability_version", "config_json", "config_fingerprint", "invalidation_json", "created_at", "updated_at", "config_version", "object_scope_json", "content_selection_json", "data_format_json", "output_config_json", "performance_config_json", "filter_config_json", "ddl_behavior_json", "source_task_id", "source_derivation") SELECT "draft_id", "owner_subject_id", "data_source_id", "node_id", "revision", "tool_version", "metadata_version", "capability_version", "config_json", "config_fingerprint", "invalidation_json", "created_at", "updated_at", "config_version", "object_scope_json", "content_selection_json", "data_format_json", "output_config_json", "performance_config_json", "filter_config_json", "ddl_behavior_json", "source_task_id", "source_derivation" FROM export_drafts;

DROP TABLE export_drafts;

ALTER TABLE export_drafts_replacement RENAME TO export_drafts;

CREATE INDEX idx_export_drafts_owner_updated ON export_drafts(owner_subject_id, updated_at DESC);

CREATE TABLE precheck_runs_replacement (
    precheck_id TEXT PRIMARY KEY,
    draft_id TEXT NOT NULL REFERENCES export_drafts(draft_id),
    draft_revision INTEGER NOT NULL CHECK (draft_revision > 0),
    config_fingerprint TEXT NOT NULL CHECK (length(config_fingerprint) = 64),
    data_source_id TEXT NOT NULL,
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
    completed_at TEXT, node_facts_revision INTEGER NOT NULL DEFAULT 0 CHECK (node_facts_revision >= 0), binding_digest TEXT CHECK (binding_digest IS NULL OR length(binding_digest) = 64), binding_agent_id TEXT REFERENCES agents(agent_id), storage_credential_id TEXT, storage_credential_revision INTEGER CHECK (storage_credential_revision IS NULL OR storage_credential_revision > 0),
    CHECK ((lease_id IS NULL AND lease_epoch IS NULL) OR (lease_id IS NOT NULL AND lease_epoch IS NOT NULL))
) STRICT, WITHOUT ROWID;

INSERT INTO precheck_runs_replacement ("precheck_id", "draft_id", "draft_revision", "config_fingerprint", "data_source_id", "credential_id", "credential_revision", "node_id", "agent_id", "status", "lease_id", "lease_epoch", "lease_expires_at", "result_json", "integrity_status", "valid_until", "created_at", "completed_at", "node_facts_revision", "binding_digest", "binding_agent_id", "storage_credential_id", "storage_credential_revision") SELECT "precheck_id", "draft_id", "draft_revision", "config_fingerprint", "data_source_id", "credential_id", "credential_revision", "node_id", "agent_id", "status", "lease_id", "lease_epoch", "lease_expires_at", "result_json", "integrity_status", "valid_until", "created_at", "completed_at", "node_facts_revision", "binding_digest", "binding_agent_id", "storage_credential_id", "storage_credential_revision" FROM precheck_runs;

DROP TABLE precheck_runs;

ALTER TABLE precheck_runs_replacement RENAME TO precheck_runs;

CREATE INDEX idx_precheck_draft_revision ON precheck_runs(draft_id, draft_revision);

CREATE INDEX idx_precheck_node_status_expiry ON precheck_runs(node_id, status, valid_until);

CREATE TABLE tasks_replacement (
    task_id TEXT PRIMARY KEY,
    creator_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    data_source_id TEXT NOT NULL,
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
    submitted_at TEXT NOT NULL, snapshot_version TEXT NOT NULL DEFAULT 'v1', parent_task_id TEXT DEFAULT NULL REFERENCES tasks(task_id), derived_from_task_id TEXT DEFAULT NULL REFERENCES tasks(task_id), template_id TEXT DEFAULT NULL, storage_credential_id TEXT, storage_credential_revision INTEGER CHECK (storage_credential_revision IS NULL OR storage_credential_revision > 0), derivation_kind TEXT CHECK (
    derivation_kind IS NULL OR derivation_kind IN ('REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH', 'CHECKPOINT_RESUME')
)
) STRICT, WITHOUT ROWID;

INSERT INTO tasks_replacement ("task_id", "creator_subject_id", "data_source_id", "node_id", "precheck_id", "credential_id", "credential_revision", "config_fingerprint", "tool_version", "metadata_version", "capability_version", "snapshot_json", "planned_argv_json", "planned_command_redacted", "submitted_at", "snapshot_version", "parent_task_id", "derived_from_task_id", "template_id", "storage_credential_id", "storage_credential_revision", "derivation_kind") SELECT "task_id", "creator_subject_id", "data_source_id", "node_id", "precheck_id", "credential_id", "credential_revision", "config_fingerprint", "tool_version", "metadata_version", "capability_version", "snapshot_json", "planned_argv_json", "planned_command_redacted", "submitted_at", "snapshot_version", "parent_task_id", "derived_from_task_id", "template_id", "storage_credential_id", "storage_credential_revision", "derivation_kind" FROM tasks;

DROP TABLE tasks;

ALTER TABLE tasks_replacement RENAME TO tasks;

CREATE INDEX idx_tasks_creator_created ON tasks(creator_subject_id, submitted_at DESC);

CREATE INDEX idx_tasks_data_source_created ON tasks(data_source_id, submitted_at DESC);

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

CREATE INDEX idx_tasks_parent_task ON tasks(parent_task_id) WHERE parent_task_id IS NOT NULL;

CREATE INDEX idx_tasks_derived_from ON tasks(derived_from_task_id) WHERE derived_from_task_id IS NOT NULL;

CREATE INDEX idx_tasks_template ON tasks(template_id) WHERE template_id IS NOT NULL;

CREATE TABLE data_source_connection_test_runs_replacement (
    connection_test_id TEXT PRIMARY KEY CHECK (length(connection_test_id) BETWEEN 1 AND 256),
    data_source_id TEXT NOT NULL,
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
    valid_until TEXT, sys_credential_id TEXT CHECK (sys_credential_id IS NULL OR length(sys_credential_id) BETWEEN 1 AND 256), sys_credential_revision INTEGER CHECK (sys_credential_revision IS NULL OR sys_credential_revision > 0), sys_verification_status TEXT CHECK (
    sys_verification_status IS NULL
    OR sys_verification_status IN ('NOT_CONFIGURED', 'SUCCEEDED', 'FAILED', 'UNKNOWN')
), sys_result_code TEXT CHECK (
    sys_result_code IS NULL
    OR (length(sys_result_code) BETWEEN 1 AND 128 AND sys_result_code NOT GLOB '*[^A-Z0-9_]*')
),
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

INSERT INTO data_source_connection_test_runs_replacement ("connection_test_id", "data_source_id", "creator_subject_id", "connection_config_digest", "credential_id", "credential_revision", "node_id", "binding_agent_id", "node_facts_revision", "binding_digest", "status", "verification_source", "lease_id", "lease_epoch", "lease_expires_at", "result_code", "safe_summary_json", "created_at", "completed_at", "valid_until", "sys_credential_id", "sys_credential_revision", "sys_verification_status", "sys_result_code") SELECT "connection_test_id", "data_source_id", "creator_subject_id", "connection_config_digest", "credential_id", "credential_revision", "node_id", "binding_agent_id", "node_facts_revision", "binding_digest", "status", "verification_source", "lease_id", "lease_epoch", "lease_expires_at", "result_code", "safe_summary_json", "created_at", "completed_at", "valid_until", "sys_credential_id", "sys_credential_revision", "sys_verification_status", "sys_result_code" FROM data_source_connection_test_runs;

DROP TABLE data_source_connection_test_runs;

ALTER TABLE data_source_connection_test_runs_replacement RENAME TO data_source_connection_test_runs;

CREATE INDEX idx_data_source_connection_test_runs_node_status_created
ON data_source_connection_test_runs(node_id, status, created_at);

CREATE INDEX idx_data_source_connection_test_runs_source_created
ON data_source_connection_test_runs(data_source_id, created_at DESC);
