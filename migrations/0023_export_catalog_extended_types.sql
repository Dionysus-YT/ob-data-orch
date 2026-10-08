-- 0023：扩展导出候选目录类型；保留 0022 的历史约束与现有记录。
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_extended_object_type TEXT
    CHECK (catalog_extended_object_type IS NULL OR catalog_extended_object_type IN ('FUNCTION', 'PROCEDURE', 'SEQUENCE'));
