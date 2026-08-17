-- 0018: EX-I8 结果、失败恢复与复用——派生方式与来源草稿标记
--
-- tasks.parent_task_id 已由 0014 提供；本迁移只补充：
-- 1) tasks.derivation_kind：派生方式（基于原配置新建 / 从头重新执行 / 从检查点继续）；
-- 2) export_drafts.source_task_id + source_derivation：派生草稿的来源标记，
--    提交派生草稿时写入新任务的 parent_task_id/derivation_kind。
-- 派生任务不复制凭据明文、预检查结果、风险确认、日志或执行事实；检查点继续复用原预检查绑定
-- 并追加官方 --retry（由服务端派生路径固定构造，不经过草稿向导）。

ALTER TABLE tasks ADD COLUMN derivation_kind TEXT CHECK (
    derivation_kind IS NULL OR derivation_kind IN ('REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH', 'CHECKPOINT_RESUME')
);

ALTER TABLE export_drafts ADD COLUMN source_task_id TEXT;
ALTER TABLE export_drafts ADD COLUMN source_derivation TEXT CHECK (
    source_derivation IS NULL OR source_derivation IN ('REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH')
);
