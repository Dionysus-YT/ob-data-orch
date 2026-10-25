-- 模板沿用现有持久化与事务，显式区分已接入的两类任务；历史模板仍为导出。
ALTER TABLE export_config_templates ADD COLUMN task_type TEXT NOT NULL DEFAULT 'OBDUMPER_EXPORT'
    CHECK (task_type IN ('OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT'));
CREATE INDEX idx_templates_owner_type ON export_config_templates(owner_subject_id, task_type);
CREATE TRIGGER config_templates_type_immutable
BEFORE UPDATE OF task_type ON export_config_templates
WHEN NEW.task_type <> OLD.task_type
BEGIN
    SELECT RAISE(ABORT, 'template type is immutable');
END;
