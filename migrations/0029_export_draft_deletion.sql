-- 草稿删除保留历史预检查和审计引用；领域事务控制资格与失效，列表排除删除标记。
ALTER TABLE export_drafts ADD COLUMN deleted_at TEXT;
CREATE INDEX idx_export_drafts_active_owner_updated ON export_drafts(owner_subject_id, updated_at DESC, draft_id DESC) WHERE deleted_at IS NULL;
