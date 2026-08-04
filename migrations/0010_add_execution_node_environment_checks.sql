ALTER TABLE execution_nodes ADD COLUMN environment_check_id TEXT;
ALTER TABLE execution_nodes ADD COLUMN environment_check_status TEXT NOT NULL DEFAULT 'NOT_CHECKED' CHECK (
    environment_check_status IN ('NOT_CHECKED', 'PENDING', 'PASSED', 'FAILED')
);
ALTER TABLE execution_nodes ADD COLUMN environment_check_code TEXT CHECK (
    environment_check_code IS NULL
    OR environment_check_code IN ('TOOL_RUNTIME_READY', 'TOOL_RUNTIME_INVALID', 'TOOL_RUNTIME_UNAVAILABLE')
);
ALTER TABLE execution_nodes ADD COLUMN environment_check_facts_revision INTEGER;
ALTER TABLE execution_nodes ADD COLUMN environment_check_requested_at TEXT;
ALTER TABLE execution_nodes ADD COLUMN environment_check_completed_at TEXT;

CREATE INDEX idx_execution_nodes_environment_check
ON execution_nodes(node_id, environment_check_status);
