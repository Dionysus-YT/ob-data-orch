-- 通用导出领域模型扩展：草稿结构化列、任务派生关系、配置模板表
-- 向后兼容：v5 草稿和 v1 快照保持可读，新列均携带默认值

-- 草稿表：新增结构化列 + 版本标识
ALTER TABLE export_drafts ADD COLUMN config_version TEXT NOT NULL DEFAULT 'v5';
ALTER TABLE export_drafts ADD COLUMN object_scope_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(object_scope_json));
ALTER TABLE export_drafts ADD COLUMN content_selection_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(content_selection_json));
ALTER TABLE export_drafts ADD COLUMN data_format_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data_format_json));
ALTER TABLE export_drafts ADD COLUMN output_config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output_config_json));
ALTER TABLE export_drafts ADD COLUMN performance_config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(performance_config_json));
ALTER TABLE export_drafts ADD COLUMN filter_config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(filter_config_json));
ALTER TABLE export_drafts ADD COLUMN ddl_behavior_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(ddl_behavior_json));

-- 任务表：快照泛化 + 派生关系
ALTER TABLE tasks ADD COLUMN snapshot_version TEXT NOT NULL DEFAULT 'v1';
ALTER TABLE tasks ADD COLUMN parent_task_id TEXT DEFAULT NULL REFERENCES tasks(task_id);
ALTER TABLE tasks ADD COLUMN derived_from_task_id TEXT DEFAULT NULL REFERENCES tasks(task_id);
ALTER TABLE tasks ADD COLUMN template_id TEXT DEFAULT NULL;

CREATE INDEX idx_tasks_parent_task ON tasks(parent_task_id) WHERE parent_task_id IS NOT NULL;
CREATE INDEX idx_tasks_derived_from ON tasks(derived_from_task_id) WHERE derived_from_task_id IS NOT NULL;
CREATE INDEX idx_tasks_template ON tasks(template_id) WHERE template_id IS NOT NULL;

-- 导出配置模板表（从成功任务保存，可创建新草稿）
CREATE TABLE IF NOT EXISTS export_config_templates (
    template_id        TEXT PRIMARY KEY CHECK (length(template_id) BETWEEN 1 AND 256),
    owner_subject_id   TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    display_name       TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 256),
    capability_version TEXT NOT NULL CHECK (length(capability_version) BETWEEN 1 AND 256),
    config_json        TEXT NOT NULL CHECK (json_valid(config_json)),
    config_fingerprint TEXT NOT NULL CHECK (length(config_fingerprint) = 64),
    source_task_id     TEXT REFERENCES tasks(task_id),
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    revision           INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_templates_owner ON export_config_templates(owner_subject_id);
CREATE INDEX idx_templates_capability ON export_config_templates(capability_version);
