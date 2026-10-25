-- 空覆盖保留既有部署配置；修改元信息与覆盖值同时存在，不能伪造当前生效事实。
CREATE TABLE ordinary_log_retention_policy (
    policy_id TEXT PRIMARY KEY CHECK (policy_id = 'ordinary-task-logs'),
    retention_days INTEGER CHECK (retention_days BETWEEN 7 AND 365),
    revision INTEGER NOT NULL CHECK (revision > 0 AND revision < 9007199254740991),
    modified_by TEXT REFERENCES auth_subjects(subject_id),
    modified_at TEXT,
    CHECK ((retention_days IS NULL AND modified_by IS NULL AND modified_at IS NULL)
        OR (retention_days IS NOT NULL AND modified_by IS NOT NULL AND modified_at IS NOT NULL))
) STRICT, WITHOUT ROWID;
INSERT INTO ordinary_log_retention_policy(policy_id,revision) VALUES ('ordinary-task-logs',1);
