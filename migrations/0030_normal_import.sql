-- 历史草稿物理表扩展平台类型标记；导入与导出各自拥有独立配置和 API 授权谓词。
ALTER TABLE export_drafts ADD COLUMN draft_kind TEXT NOT NULL DEFAULT 'OBDUMPER_EXPORT'
    CHECK (draft_kind IN ('OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT'));
CREATE INDEX idx_drafts_kind_owner ON export_drafts(draft_kind, owner_subject_id, updated_at DESC) WHERE deleted_at IS NULL;
-- 文件发现与完整导入预检查共用租约，但用途不可互相替代。
ALTER TABLE precheck_runs ADD COLUMN check_kind TEXT NOT NULL DEFAULT 'EXPORT_PREFLIGHT'
    CHECK (check_kind IN ('EXPORT_PREFLIGHT', 'NORMAL_IMPORT_FILES', 'NORMAL_IMPORT_PREFLIGHT'));
ALTER TABLE precheck_runs ADD COLUMN input_manifest_json TEXT CHECK (input_manifest_json IS NULL OR json_valid(input_manifest_json));
-- 任务类型属于冻结事实，历史任务仍固定为导出。
ALTER TABLE tasks ADD COLUMN task_type TEXT NOT NULL DEFAULT 'OBDUMPER_EXPORT'
    CHECK (task_type IN ('OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT'));
