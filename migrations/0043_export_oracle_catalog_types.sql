-- Oracle 新分类使用独立受约束列，保留 0022/0023 历史类型约束和已有记录。
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_oracle_object_type TEXT
    CHECK (catalog_oracle_object_type IS NULL OR catalog_oracle_object_type IN ('PACKAGE', 'TRIGGER', 'SYNONYM', 'PUBLIC_SYNONYM', 'TYPE'));
