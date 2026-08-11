-- 0016: 连接测试的 sys 凭据验证（--sys-user/--sys-password）
--
-- 参考 ODC：sys 租户账号有独立的连接验证。数据源配置了可选的 sys 凭据时，
-- 连接测试额外用 sys 身份（sysUser@sys#cluster）经固定 JDBC 探针验证一次认证；
-- sys 结果是独立的提示性事实，不阻断数据库启用门禁（sys 凭据为可选增强）。
--
-- 创建测试时冻结 sys 凭据引用（sys_credential_id/sys_credential_revision，可空）；
-- 完成后写入 sys 验证状态与证据码（可空）。两个 CHECK 只允许受控枚举与代码。

ALTER TABLE data_source_connection_test_runs ADD COLUMN sys_credential_id TEXT CHECK (sys_credential_id IS NULL OR length(sys_credential_id) BETWEEN 1 AND 256);
ALTER TABLE data_source_connection_test_runs ADD COLUMN sys_credential_revision INTEGER CHECK (sys_credential_revision IS NULL OR sys_credential_revision > 0);
ALTER TABLE data_source_connection_test_runs ADD COLUMN sys_verification_status TEXT CHECK (
    sys_verification_status IS NULL
    OR sys_verification_status IN ('NOT_CONFIGURED', 'SUCCEEDED', 'FAILED', 'UNKNOWN')
);
ALTER TABLE data_source_connection_test_runs ADD COLUMN sys_result_code TEXT CHECK (
    sys_result_code IS NULL
    OR (length(sys_result_code) BETWEEN 1 AND 128 AND sys_result_code NOT GLOB '*[^A-Z0-9_]*')
);
