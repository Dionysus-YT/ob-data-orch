-- 节点预算是调度配置；分配在领取事务冻结，不能用心跳的利用率或槽位替代。
ALTER TABLE execution_nodes ADD COLUMN execution_budget_policy_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(execution_budget_policy_json));

-- 旧 capacity 列属于已发布的单任务契约；新增字段表达真实并行上界与占用，不重建被外键引用的 Agent 表。
ALTER TABLE agents ADD COLUMN execution_worker_limit INTEGER NOT NULL DEFAULT 1 CHECK(execution_worker_limit BETWEEN 0 AND 1024);
ALTER TABLE agents ADD COLUMN execution_workers_active INTEGER NOT NULL DEFAULT 0 CHECK(execution_workers_active BETWEEN 0 AND 1024);
UPDATE agents SET execution_worker_limit=capacity_total, execution_workers_active=capacity_used;

CREATE TABLE execution_resource_allocations (
    execution_id TEXT PRIMARY KEY REFERENCES task_executions(execution_id),
    heap_bytes INTEGER NOT NULL CHECK(heap_bytes > 0),
    budget_bytes INTEGER NOT NULL CHECK(budget_bytes >= heap_bytes),
    allocated_at TEXT NOT NULL
);

-- 并行执行后，同一领取请求重放不能消耗第二笔预算或领取另一任务。
CREATE TABLE execution_claim_receipts (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    request_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    grant_json TEXT NOT NULL CHECK(json_valid(grant_json)),
    PRIMARY KEY(agent_id, request_id)
);

-- 释放由已接受事件派生，不能回写或删除冻结成本与领取回执。
CREATE TRIGGER execution_resource_allocations_no_update BEFORE UPDATE ON execution_resource_allocations BEGIN SELECT RAISE(ABORT, 'execution resource allocation is immutable'); END;
CREATE TRIGGER execution_resource_allocations_no_delete BEFORE DELETE ON execution_resource_allocations BEGIN SELECT RAISE(ABORT, 'execution resource allocation is immutable'); END;
CREATE TRIGGER execution_claim_receipts_no_update BEFORE UPDATE ON execution_claim_receipts BEGIN SELECT RAISE(ABORT, 'execution claim receipt is immutable'); END;
CREATE TRIGGER execution_claim_receipts_no_delete BEFORE DELETE ON execution_claim_receipts BEGIN SELECT RAISE(ABORT, 'execution claim receipt is immutable'); END;
