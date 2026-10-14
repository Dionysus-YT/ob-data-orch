-- 0027：任务展示事实在提交时冻结，供任务中心在数据源或节点删除后仍显示历史名称。
-- 只保存非秘密展示名称与环境；不保存连接信息、路径、凭据或命令。
-- 迁移前已有任务按迁移时的当前名称回填（DEC-050），对象已删除时名称为 NULL。

CREATE TABLE task_display_facts (
    task_id TEXT PRIMARY KEY REFERENCES tasks(task_id),
    data_source_name TEXT CHECK (data_source_name IS NULL OR length(data_source_name) > 0),
    environment TEXT CHECK (environment IS NULL OR environment IN ('DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION')),
    node_name TEXT CHECK (node_name IS NULL OR length(node_name) > 0),
    captured_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

INSERT INTO task_display_facts(task_id, data_source_name, environment, node_name, captured_at)
SELECT t.task_id, ds.display_name, ds.environment, n.display_name, t.submitted_at
FROM tasks t
LEFT JOIN data_sources ds ON ds.data_source_id = t.data_source_id
LEFT JOIN execution_nodes n ON n.node_id = t.node_id;

CREATE INDEX idx_task_display_facts_environment ON task_display_facts(environment);

CREATE TRIGGER task_display_facts_no_update
BEFORE UPDATE ON task_display_facts
BEGIN
    SELECT RAISE(ABORT, 'task display facts are immutable');
END;

CREATE TRIGGER task_display_facts_no_delete
BEFORE DELETE ON task_display_facts
BEGIN
    SELECT RAISE(ABORT, 'task display facts are immutable');
END;
