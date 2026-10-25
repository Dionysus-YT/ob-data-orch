-- 未配置的覆盖保持空值，部署心跳基线、既有启动及 Agent 轮询行为不被静默替换。
CREATE TABLE platform_scheduling_timing_policy (
    policy_id TEXT PRIMARY KEY CHECK (policy_id = 'execution-timing'),
    heartbeat_seconds INTEGER CHECK (heartbeat_seconds BETWEEN 60 AND 3600),
    startup_seconds INTEGER CHECK (startup_seconds BETWEEN 15 AND 3600),
    polling_seconds INTEGER CHECK (polling_seconds BETWEEN 1 AND 60),
    revision INTEGER NOT NULL CHECK (revision > 0 AND revision < 9007199254740991),
    modified_by TEXT REFERENCES auth_subjects(subject_id),
    modified_at TEXT,
    CHECK ((heartbeat_seconds IS NULL AND startup_seconds IS NULL AND polling_seconds IS NULL AND modified_by IS NULL AND modified_at IS NULL)
        OR ((heartbeat_seconds IS NOT NULL OR startup_seconds IS NOT NULL OR polling_seconds IS NOT NULL) AND modified_by IS NOT NULL AND modified_at IS NOT NULL))
) STRICT, WITHOUT ROWID;
INSERT INTO platform_scheduling_timing_policy(policy_id,revision) VALUES ('execution-timing',1);

-- 启动期限只在新领取时冻结，历史空期限不回填；时间策略修改不改写已有执行。
ALTER TABLE task_executions ADD COLUMN startup_deadline TEXT;
CREATE TRIGGER task_execution_startup_deadline_immutable
BEFORE UPDATE OF startup_deadline ON task_executions
WHEN NEW.startup_deadline IS NOT OLD.startup_deadline
BEGIN
    SELECT RAISE(ABORT, 'execution startup deadline is immutable');
END;

-- 三类任务分别记录检查时点，避免 Agent 类型轮转与平台间隔形成固定相位饥饿。
CREATE TABLE agent_execution_poll_checks (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    task_type TEXT NOT NULL CHECK (task_type IN ('OBDUMPER_EXPORT','OBLOADER_NORMAL_IMPORT','OBLOADER_DIRECT_LOAD')),
    last_checked_at TEXT NOT NULL,
    PRIMARY KEY (agent_id,task_type)
) STRICT, WITHOUT ROWID;
