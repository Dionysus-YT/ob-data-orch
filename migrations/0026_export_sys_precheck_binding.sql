-- 冻结依赖 sys 权限的导出预检查引用；历史检查保持空引用，不改变历史结论。
ALTER TABLE precheck_runs ADD COLUMN sys_credential_id TEXT;
ALTER TABLE precheck_runs ADD COLUMN sys_credential_revision INTEGER
    CHECK ((sys_credential_id IS NULL AND sys_credential_revision IS NULL)
        OR (sys_credential_id IS NOT NULL AND sys_credential_revision IS NOT NULL
            AND length(sys_credential_id) BETWEEN 1 AND 256 AND sys_credential_revision > 0));
