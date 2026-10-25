-- 候选顺序独立保存；升级保留现有顺序，不改变用户选择或历史任务。
CREATE TABLE platform_scheduling_candidate_policy (
    policy_id TEXT PRIMARY KEY CHECK (policy_id = 'candidate-order'),
    candidate_order TEXT NOT NULL CHECK (candidate_order IN ('EXISTING','ONLINE_FIRST')),
    revision INTEGER NOT NULL CHECK (revision > 0 AND revision < 9007199254740991),
    modified_by TEXT REFERENCES auth_subjects(subject_id),
    modified_at TEXT,
    CHECK ((revision = 1 AND candidate_order = 'EXISTING' AND modified_by IS NULL AND modified_at IS NULL)
        OR (revision > 1 AND modified_by IS NOT NULL AND modified_at IS NOT NULL))
) STRICT, WITHOUT ROWID;
INSERT INTO platform_scheduling_candidate_policy(policy_id,candidate_order,revision) VALUES ('candidate-order','EXISTING',1);
