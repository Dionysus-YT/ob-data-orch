package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"ob-data-orch/internal/executionbudget"
)

// NodeBudgetUsage 只包含节点共享预算，不承载其他账号的任务标识或内容。
type NodeBudgetUsage struct {
	UsedBytes     uint64
	ReservedBytes uint64
	Unknown       bool
}

// 已接受的进程退出、无进程启动拒绝或无进程取消是释放依据，租约/心跳超时不是。
const budgetStillHeld = `NOT EXISTS (
    SELECT 1 FROM execution_events released WHERE released.execution_id = e.execution_id AND released.accepted = 1
      AND (released.event_type IN ('PROCESS_EXITED', 'START_REJECTED') OR
           (released.event_type = 'PROCESS_CANCELLED' AND json_extract(released.payload_json, '$.noProcess') = 1 AND json_extract(released.payload_json, '$.treeObserved') = 1))
)`

func encodeBudgetPolicy(policy *executionbudget.Policy) (string, error) {
	if policy == nil {
		policy = &executionbudget.Policy{}
	}
	if policy.SystemReservedBytes > 1<<53-1 || policy.TaskOverheadBytes > 1<<53-1 {
		return "", errors.New("node budget policy is invalid")
	}
	encoded, err := json.Marshal(policy)
	return string(encoded), err
}

// ListNodeBudgetUsages 从持久执行分配聚合预算，待核对的运行分配退入预留而非释放。
// 升级前没有预算记录且没有进程结束证据的执行标为未知，阻止后续过量分配。
func (s *Store) ListNodeBudgetUsages(ctx context.Context) (map[string]NodeBudgetUsage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.node_id,
        SUM(CASE WHEN a.execution_id IS NOT NULL AND e.reconciliation_required = 0 AND EXISTS
            (SELECT 1 FROM execution_events started WHERE started.execution_id = e.execution_id AND started.event_type = 'PROCESS_STARTED' AND started.accepted = 1)
            THEN a.budget_bytes ELSE 0 END),
        SUM(CASE WHEN a.execution_id IS NOT NULL AND (e.reconciliation_required != 0 OR NOT EXISTS
            (SELECT 1 FROM execution_events started WHERE started.execution_id = e.execution_id AND started.event_type = 'PROCESS_STARTED' AND started.accepted = 1))
            THEN a.budget_bytes ELSE 0 END),
        SUM(CASE WHEN a.execution_id IS NULL THEN 1 ELSE 0 END)
        FROM task_executions e LEFT JOIN execution_resource_allocations a ON a.execution_id = e.execution_id
        WHERE (a.execution_id IS NOT NULL OR e.state IN ('STARTING','RUNNING','CANCELLING') OR e.reconciliation_required = 1) AND `+budgetStillHeld+`
        GROUP BY e.node_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]NodeBudgetUsage)
	for rows.Next() {
		var nodeID string
		var usage NodeBudgetUsage
		var unknown int
		if err := rows.Scan(&nodeID, &usage.UsedBytes, &usage.ReservedBytes, &unknown); err != nil {
			return nil, err
		}
		usage.Unknown = unknown != 0
		result[nodeID] = usage
	}
	return result, rows.Err()
}

// reserveExecutionBudgetTx 在领取事务内复验真实资源与所有账号的占用，返回不可变分配成本。
// 老 Agent 无物理内存事实时阻断新分配，不从旧槽位字段生成伪造额度。
func reserveExecutionBudgetTx(ctx context.Context, tx *sql.Tx, nodeID, agentID string, argv []string) (uint64, uint64, bool, error) {
	var factsJSON sql.NullString
	var policyJSON string
	var capacity int
	if err := tx.QueryRowContext(ctx, `SELECT a.facts_json, n.execution_budget_policy_json, a.execution_worker_limit FROM agents a JOIN execution_nodes n ON n.node_id = a.node_id WHERE a.agent_id = ? AND a.node_id = ? AND a.status = 'ACTIVE'`, agentID, nodeID).Scan(&factsJSON, &policyJSON, &capacity); err != nil {
		return 0, 0, false, err
	}
	var facts AgentEnvironmentFacts
	var err error
	if factsJSON.Valid {
		facts, err = decodeAgentEnvironmentFacts(factsJSON.String)
		if err != nil {
			return 0, 0, false, err
		}
	}
	var policy executionbudget.Policy
	if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
		return 0, 0, false, err
	}
	heap, err := executionbudget.EffectiveHeap(argv)
	if err != nil {
		return 0, 0, false, err
	}
	cost, err := policy.Cost(heap)
	if err != nil {
		return 0, 0, false, err
	}
	var held uint64
	var active, unknown int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(a.budget_bytes),0), COUNT(*), COALESCE(SUM(CASE WHEN a.execution_id IS NULL THEN 1 ELSE 0 END),0)
        FROM task_executions e LEFT JOIN execution_resource_allocations a ON a.execution_id = e.execution_id
        WHERE e.node_id = ? AND (a.execution_id IS NOT NULL OR e.state IN ('STARTING','RUNNING','CANCELLING') OR e.reconciliation_required = 1) AND `+budgetStillHeld, nodeID).Scan(&held, &active, &unknown)
	if err != nil {
		return 0, 0, false, err
	}
	if unknown > 0 || active >= capacity {
		return heap, cost, false, nil
	}
	if facts.MemoryTotalBytes == 0 {
		return heap, cost, false, nil
	}
	total, err := policy.Total(facts.MemoryTotalBytes)
	if err != nil || held > total || cost > total-held {
		return heap, cost, false, nil
	}
	// 可用内存只用于新任务准入，不与已占预算相加，避免将运行 RSS 再次计费。
	if facts.MemoryAvailableBytes < cost {
		return heap, cost, false, nil
	}
	return heap, cost, true, nil
}

// ensureExecutionChecksIdle 允许有预算的执行并行，但不与短时连接测试或预检查争用运行时。
func ensureExecutionChecksIdle(ctx context.Context, tx *sql.Tx, agentID string) error {
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT
        (SELECT COUNT(*) FROM precheck_runs WHERE agent_id = ? AND status = 'LEASED') +
        (SELECT COUNT(*) FROM data_source_connection_test_runs WHERE binding_agent_id = ? AND status = 'LEASED')`, agentID, agentID).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return ErrPrecheckLeaseRejected
	}
	return nil
}
