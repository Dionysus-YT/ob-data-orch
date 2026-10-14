-- 节点配置可永久删除，历史外键只引用不可变标识，不复制名称、路径或运行配置。
-- 迁移器提交前检查全部外键，任何悬空引用都回滚。
PRAGMA defer_foreign_keys = ON;

ALTER TABLE execution_nodes RENAME TO execution_node_references;

CREATE TABLE execution_nodes (
    node_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    platform TEXT NOT NULL CHECK (platform IN ('WINDOWS_AMD64', 'LINUX_AMD64', 'LINUX_ARM64')),
    management_state TEXT NOT NULL CHECK (management_state IN ('ENABLED', 'DISABLED', 'MAINTENANCE', 'ARCHIVED')),
    allowed_roots_json TEXT NOT NULL CHECK (json_valid(allowed_roots_json)),
    tool_config_ref TEXT,
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_by TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    environment_check_id TEXT,
    environment_check_status TEXT NOT NULL DEFAULT 'NOT_CHECKED' CHECK (environment_check_status IN ('NOT_CHECKED', 'PENDING', 'PASSED', 'FAILED')),
    environment_check_code TEXT CHECK (environment_check_code IS NULL OR environment_check_code IN ('TOOL_RUNTIME_READY', 'TOOL_RUNTIME_INVALID', 'TOOL_RUNTIME_UNAVAILABLE')),
    environment_check_facts_revision INTEGER,
    environment_check_requested_at TEXT,
    environment_check_completed_at TEXT,
    tool_home TEXT NOT NULL DEFAULT '',
    java_path TEXT NOT NULL DEFAULT '',
    execution_budget_policy_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(execution_budget_policy_json))
) STRICT, WITHOUT ROWID;

INSERT INTO execution_nodes SELECT * FROM execution_node_references;

CREATE TABLE execution_node_references_replacement (
    node_id TEXT PRIMARY KEY
) STRICT, WITHOUT ROWID;
INSERT INTO execution_node_references_replacement SELECT node_id FROM execution_node_references;
DROP TABLE execution_node_references;
ALTER TABLE execution_node_references_replacement RENAME TO execution_node_references;

CREATE INDEX idx_execution_nodes_environment_check ON execution_nodes(node_id, environment_check_status);

-- 标识永久保留并禁止复用，迟到创建不能复活已删除的同一节点。
CREATE TRIGGER execution_node_references_no_update BEFORE UPDATE ON execution_node_references
BEGIN SELECT RAISE(ABORT, 'execution node reference is immutable'); END;
CREATE TRIGGER execution_node_references_no_delete BEFORE DELETE ON execution_node_references
BEGIN SELECT RAISE(ABORT, 'execution node reference is immutable'); END;
CREATE TRIGGER execution_nodes_no_reused_identity BEFORE INSERT ON execution_nodes
WHEN EXISTS (SELECT 1 FROM execution_node_references WHERE node_id = NEW.node_id)
BEGIN SELECT RAISE(ABORT, 'execution node identity cannot be reused'); END;
CREATE TRIGGER execution_nodes_create_identity AFTER INSERT ON execution_nodes
BEGIN INSERT INTO execution_node_references(node_id) VALUES (NEW.node_id); END;
CREATE TRIGGER execution_nodes_identity_no_update BEFORE UPDATE OF node_id ON execution_nodes
WHEN NEW.node_id != OLD.node_id
BEGIN SELECT RAISE(ABORT, 'execution node identity is immutable'); END;

-- 0024 的两张预算证据表补齐现行 STRICT 基线，保留全部记录与不可变约束。
CREATE TABLE execution_resource_allocations_replacement (
    execution_id TEXT PRIMARY KEY REFERENCES task_executions(execution_id),
    heap_bytes INTEGER NOT NULL CHECK(heap_bytes > 0),
    budget_bytes INTEGER NOT NULL CHECK(budget_bytes >= heap_bytes),
    allocated_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;
INSERT INTO execution_resource_allocations_replacement SELECT * FROM execution_resource_allocations;
DROP TABLE execution_resource_allocations;
ALTER TABLE execution_resource_allocations_replacement RENAME TO execution_resource_allocations;
CREATE TRIGGER execution_resource_allocations_no_update BEFORE UPDATE ON execution_resource_allocations BEGIN SELECT RAISE(ABORT, 'execution resource allocation is immutable'); END;
CREATE TRIGGER execution_resource_allocations_no_delete BEFORE DELETE ON execution_resource_allocations BEGIN SELECT RAISE(ABORT, 'execution resource allocation is immutable'); END;

CREATE TABLE execution_claim_receipts_replacement (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    request_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    grant_json TEXT NOT NULL CHECK(json_valid(grant_json)),
    PRIMARY KEY(agent_id, request_id)
) STRICT, WITHOUT ROWID;
INSERT INTO execution_claim_receipts_replacement SELECT * FROM execution_claim_receipts;
DROP TABLE execution_claim_receipts;
ALTER TABLE execution_claim_receipts_replacement RENAME TO execution_claim_receipts;
CREATE TRIGGER execution_claim_receipts_no_update BEFORE UPDATE ON execution_claim_receipts BEGIN SELECT RAISE(ABORT, 'execution claim receipt is immutable'); END;
CREATE TRIGGER execution_claim_receipts_no_delete BEFORE DELETE ON execution_claim_receipts BEGIN SELECT RAISE(ABORT, 'execution claim receipt is immutable'); END;
