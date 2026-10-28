-- 一次性授予证据独立于审计保留期限；升级不为任何主体增加权限。
ALTER TABLE subject_permission_versions ADD COLUMN audit_bootstrap_consumed INTEGER NOT NULL DEFAULT 0 CHECK (audit_bootstrap_consumed IN (0, 1));

-- 已有或曾经获得独立审计能力的初始身份不能借新入口重新授予。
UPDATE subject_permission_versions
SET audit_bootstrap_consumed = 1
WHERE deployment_owner = 1 AND (
    EXISTS (SELECT 1 FROM subject_grants g WHERE g.subject_id = subject_permission_versions.subject_id AND g.grant_code = 'CAP_AUDIT_LOG')
    OR EXISTS (
        SELECT 1 FROM audit_events a
        WHERE a.object_type = 'SUBJECT' AND a.object_id = subject_permission_versions.subject_id
          AND a.action = 'SUBJECT_PERMISSIONS_CHANGED' AND a.result = 'SUCCEEDED'
          AND (EXISTS (SELECT 1 FROM json_each(a.safe_diff_json, '$.before.capabilities') WHERE value = 'CAP_AUDIT_LOG')
            OR EXISTS (SELECT 1 FROM json_each(a.safe_diff_json, '$.after.capabilities') WHERE value = 'CAP_AUDIT_LOG'))
    )
);

-- 仅保护不可逆的使用证据，授予资格与业务确认仍由领域事务判断。
CREATE TRIGGER initial_audit_access_no_reset
BEFORE UPDATE OF audit_bootstrap_consumed ON subject_permission_versions
WHEN OLD.audit_bootstrap_consumed = 1 AND NEW.audit_bootstrap_consumed <> 1
BEGIN SELECT RAISE(ABORT, 'initial audit access evidence is immutable'); END;

CREATE TRIGGER initial_audit_access_no_delete
BEFORE DELETE ON subject_permission_versions
WHEN OLD.audit_bootstrap_consumed = 1
BEGIN SELECT RAISE(ABORT, 'initial audit access evidence is immutable'); END;
