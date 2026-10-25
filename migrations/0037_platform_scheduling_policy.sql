-- 继承已有调度行为，不把未经确认的常量冒充部署并发上限。
CREATE TABLE platform_scheduling_policy (
    policy_id TEXT PRIMARY KEY CHECK (policy_id = 'platform-executions'),
    max_concurrent_executions INTEGER CHECK (max_concurrent_executions BETWEEN 1 AND 1024),
    revision INTEGER NOT NULL CHECK (revision > 0 AND revision < 9007199254740991),
    modified_by TEXT REFERENCES auth_subjects(subject_id),
    modified_at TEXT,
    CHECK ((max_concurrent_executions IS NULL AND modified_by IS NULL AND modified_at IS NULL)
        OR (max_concurrent_executions IS NOT NULL AND modified_by IS NOT NULL AND modified_at IS NOT NULL))
) STRICT, WITHOUT ROWID;
INSERT INTO platform_scheduling_policy(policy_id,revision) VALUES ('platform-executions',1);
