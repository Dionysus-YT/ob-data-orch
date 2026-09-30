-- 0022：导出向导的有界对象候选查询复用节点侧 JDBC 短租约，不改变数据源基础连接测试事实。
ALTER TABLE data_source_connection_test_runs ADD COLUMN operation_kind TEXT NOT NULL DEFAULT 'CONNECTION_TEST'
    CHECK (operation_kind IN ('CONNECTION_TEST', 'EXPORT_OBJECT_CATALOG'));
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_database TEXT;
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_compatibility_mode TEXT
    CHECK (catalog_compatibility_mode IS NULL OR catalog_compatibility_mode IN ('MYSQL', 'ORACLE'));
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_object_type TEXT
    CHECK (catalog_object_type IS NULL OR catalog_object_type IN ('TABLE', 'VIEW'));
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_keyword TEXT;
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_objects_json TEXT
    CHECK (catalog_objects_json IS NULL OR json_valid(catalog_objects_json));
ALTER TABLE data_source_connection_test_runs ADD COLUMN catalog_truncated INTEGER NOT NULL DEFAULT 0
    CHECK (catalog_truncated IN (0, 1));
