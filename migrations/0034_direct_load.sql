-- 旁路导入仅扩展平台类型与固定检查用途；保留既有行、默认值、索引和不可变触发器。

-- 全程保留外键开启，迁移器提交前核验整库引用。

PRAGMA defer_foreign_keys = ON;

CREATE TABLE export_drafts_replacement (
    draft_id TEXT PRIMARY KEY,
    owner_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    data_source_id TEXT NOT NULL,
    node_id TEXT REFERENCES "execution_node_references"(node_id),
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
), deleted_at TEXT, draft_kind TEXT NOT NULL DEFAULT 'OBDUMPER_EXPORT'
    CHECK (draft_kind IN ('OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT', 'OBLOADER_DIRECT_LOAD'))) STRICT, WITHOUT ROWID;

INSERT INTO export_drafts_replacement ("draft_id", "owner_subject_id", "data_source_id", "node_id", "revision", "tool_version", "metadata_version", "capability_version", "config_json", "config_fingerprint", "invalidation_json", "created_at", "updated_at", "config_version", "object_scope_json", "content_selection_json", "data_format_json", "output_config_json", "performance_config_json", "filter_config_json", "ddl_behavior_json", "source_task_id", "source_derivation", "deleted_at", "draft_kind") SELECT "draft_id", "owner_subject_id", "data_source_id", "node_id", "revision", "tool_version", "metadata_version", "capability_version", "config_json", "config_fingerprint", "invalidation_json", "created_at", "updated_at", "config_version", "object_scope_json", "content_selection_json", "data_format_json", "output_config_json", "performance_config_json", "filter_config_json", "ddl_behavior_json", "source_task_id", "source_derivation", "deleted_at", "draft_kind" FROM export_drafts;

DROP TABLE export_drafts;

ALTER TABLE export_drafts_replacement RENAME TO export_drafts;

CREATE INDEX idx_drafts_kind_owner ON export_drafts(draft_kind, owner_subject_id, updated_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX idx_export_drafts_active_owner_updated ON export_drafts(owner_subject_id, updated_at DESC, draft_id DESC) WHERE deleted_at IS NULL;

CREATE INDEX idx_export_drafts_owner_updated ON export_drafts(owner_subject_id, updated_at DESC);

CREATE TABLE precheck_runs_replacement (
    precheck_id TEXT PRIMARY KEY,
    draft_id TEXT NOT NULL REFERENCES export_drafts(draft_id),
    draft_revision INTEGER NOT NULL CHECK (draft_revision > 0),
    config_fingerprint TEXT NOT NULL CHECK (length(config_fingerprint) = 64),
    data_source_id TEXT NOT NULL,
    credential_id TEXT NOT NULL,
    credential_revision INTEGER NOT NULL CHECK (credential_revision > 0),
    node_id TEXT NOT NULL REFERENCES "execution_node_references"(node_id),
    agent_id TEXT REFERENCES agents(agent_id),
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'LEASED', 'SUCCEEDED', 'FAILED', 'EXPIRED', 'INVALIDATED')),
    lease_id TEXT,
    lease_epoch INTEGER CHECK (lease_epoch IS NULL OR lease_epoch > 0),
    lease_expires_at TEXT,
    result_json TEXT CHECK (result_json IS NULL OR json_valid(result_json)),
    integrity_status TEXT NOT NULL CHECK (integrity_status IN ('UNKNOWN', 'COMPLETE', 'INCOMPLETE')),
    valid_until TEXT,
    created_at TEXT NOT NULL,
    completed_at TEXT, node_facts_revision INTEGER NOT NULL DEFAULT 0 CHECK (node_facts_revision >= 0), binding_digest TEXT CHECK (binding_digest IS NULL OR length(binding_digest) = 64), binding_agent_id TEXT REFERENCES agents(agent_id), storage_credential_id TEXT, storage_credential_revision INTEGER CHECK (storage_credential_revision IS NULL OR storage_credential_revision > 0), sys_credential_id TEXT, sys_credential_revision INTEGER
    CHECK ((sys_credential_id IS NULL AND sys_credential_revision IS NULL)
        OR (sys_credential_id IS NOT NULL AND sys_credential_revision IS NOT NULL
            AND length(sys_credential_id) BETWEEN 1 AND 256 AND sys_credential_revision > 0)), check_kind TEXT NOT NULL DEFAULT 'EXPORT_PREFLIGHT'
    CHECK (check_kind IN ('EXPORT_PREFLIGHT', 'NORMAL_IMPORT_FILES', 'NORMAL_IMPORT_PREFLIGHT', 'DIRECT_LOAD_FILES', 'DIRECT_LOAD_PREFLIGHT')), input_manifest_json TEXT CHECK (input_manifest_json IS NULL OR json_valid(input_manifest_json)),
    direct_load_config_fingerprint TEXT CHECK (direct_load_config_fingerprint IS NULL OR (check_kind='DIRECT_LOAD_PREFLIGHT' AND length(direct_load_config_fingerprint)=64 AND direct_load_config_fingerprint NOT GLOB '*[^0-9a-f]*')),
    CHECK ((lease_id IS NULL AND lease_epoch IS NULL) OR (lease_id IS NOT NULL AND lease_epoch IS NOT NULL))
) STRICT, WITHOUT ROWID;

INSERT INTO precheck_runs_replacement ("precheck_id", "draft_id", "draft_revision", "config_fingerprint", "data_source_id", "credential_id", "credential_revision", "node_id", "agent_id", "status", "lease_id", "lease_epoch", "lease_expires_at", "result_json", "integrity_status", "valid_until", "created_at", "completed_at", "node_facts_revision", "binding_digest", "binding_agent_id", "storage_credential_id", "storage_credential_revision", "sys_credential_id", "sys_credential_revision", "check_kind", "input_manifest_json") SELECT "precheck_id", "draft_id", "draft_revision", "config_fingerprint", "data_source_id", "credential_id", "credential_revision", "node_id", "agent_id", "status", "lease_id", "lease_epoch", "lease_expires_at", "result_json", "integrity_status", "valid_until", "created_at", "completed_at", "node_facts_revision", "binding_digest", "binding_agent_id", "storage_credential_id", "storage_credential_revision", "sys_credential_id", "sys_credential_revision", "check_kind", "input_manifest_json" FROM precheck_runs;

DROP TABLE precheck_runs;

ALTER TABLE precheck_runs_replacement RENAME TO precheck_runs;

CREATE INDEX idx_precheck_draft_revision ON precheck_runs(draft_id, draft_revision);

CREATE INDEX idx_precheck_node_status_expiry ON precheck_runs(node_id, status, valid_until);

CREATE TABLE tasks_replacement (
    task_id TEXT PRIMARY KEY,
    creator_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    data_source_id TEXT NOT NULL,
    node_id TEXT NOT NULL REFERENCES "execution_node_references"(node_id),
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
, task_type TEXT NOT NULL DEFAULT 'OBDUMPER_EXPORT'
    CHECK (task_type IN ('OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT', 'OBLOADER_DIRECT_LOAD'))) STRICT, WITHOUT ROWID;

INSERT INTO tasks_replacement ("task_id", "creator_subject_id", "data_source_id", "node_id", "precheck_id", "credential_id", "credential_revision", "config_fingerprint", "tool_version", "metadata_version", "capability_version", "snapshot_json", "planned_argv_json", "planned_command_redacted", "submitted_at", "snapshot_version", "parent_task_id", "derived_from_task_id", "template_id", "storage_credential_id", "storage_credential_revision", "derivation_kind", "task_type") SELECT "task_id", "creator_subject_id", "data_source_id", "node_id", "precheck_id", "credential_id", "credential_revision", "config_fingerprint", "tool_version", "metadata_version", "capability_version", "snapshot_json", "planned_argv_json", "planned_command_redacted", "submitted_at", "snapshot_version", "parent_task_id", "derived_from_task_id", "template_id", "storage_credential_id", "storage_credential_revision", "derivation_kind", "task_type" FROM tasks;

DROP TABLE tasks;

ALTER TABLE tasks_replacement RENAME TO tasks;

CREATE INDEX idx_tasks_creator_created ON tasks(creator_subject_id, submitted_at DESC);

CREATE INDEX idx_tasks_data_source_created ON tasks(data_source_id, submitted_at DESC);

CREATE INDEX idx_tasks_derived_from ON tasks(derived_from_task_id) WHERE derived_from_task_id IS NOT NULL;

CREATE INDEX idx_tasks_parent_task ON tasks(parent_task_id) WHERE parent_task_id IS NOT NULL;

CREATE INDEX idx_tasks_template ON tasks(template_id) WHERE template_id IS NOT NULL;

CREATE TRIGGER tasks_no_delete
BEFORE DELETE ON tasks
BEGIN
    SELECT RAISE(ABORT, 'tasks are immutable');
END;

CREATE TRIGGER tasks_no_update
BEFORE UPDATE ON tasks
BEGIN
    SELECT RAISE(ABORT, 'tasks are immutable');
END;
