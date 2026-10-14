package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeleteExecutionNode 原子删除节点配置并撤销机器访问，历史仅保留原标识与不可变证据。
// 运行、待核对或仍有未释放预算的执行阻断删除，不能因撤销身份失去进程报送。
func (s *Store) DeleteExecutionNode(ctx context.Context, input ExecutionNodeDeletion) (ExecutionNodeDeletionResult, error) {
	if err := validateExecutionNodeDeletion(input); err != nil {
		return ExecutionNodeDeletionResult{}, err
	}
	result := ExecutionNodeDeletionResult{}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		// 先取得 SQLite 写锁，再复验引用，避免跨 Store 的领取与删除交错。
		locked, err := tx.ExecContext(ctx, `UPDATE execution_nodes SET revision = revision WHERE node_id = ? AND revision = ?`, input.NodeID, input.ExpectedRevision)
		if err != nil {
			return fmt.Errorf("lock execution node deletion: %w", err)
		}
		count, err := locked.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			var revision int64
			err := tx.QueryRowContext(ctx, `SELECT revision FROM execution_nodes WHERE node_id = ?`, input.NodeID).Scan(&revision)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrExecutionNodeNotFound
			}
			if err != nil {
				return err
			}
			return ErrRevisionConflict
		}
		var unfinished int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_executions e WHERE e.node_id = ? AND
			(e.state IN ('STARTING','RUNNING','CANCELLING') OR e.reconciliation_required = 1 OR
			 (EXISTS (SELECT 1 FROM execution_resource_allocations a WHERE a.execution_id = e.execution_id) AND `+budgetStillHeld+`))`, input.NodeID).Scan(&unfinished); err != nil {
			return err
		}
		if unfinished != 0 {
			return ErrExecutionNodeHasRunningTask
		}
		revoked, err := revokeExecutionNodeAgentAccess(ctx, tx, input)
		if err != nil {
			return err
		}
		// 已完成的预检查与目录证据不改写；未完成探针失效，后续提交仍须找到现存节点。
		if _, err := tx.ExecContext(ctx, `UPDATE precheck_runs SET status = 'INVALIDATED', integrity_status = 'INCOMPLETE', completed_at = COALESCE(completed_at, ?)
			WHERE node_id = ? AND status IN ('PENDING','LEASED')`, utcText(input.DeletedAt), input.NodeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE data_source_connection_test_runs SET status = 'INVALIDATED', completed_at = COALESCE(completed_at, ?)
			WHERE node_id = ? AND status IN ('PENDING','LEASED')`, utcText(input.DeletedAt), input.NodeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM execution_nodes WHERE node_id = ? AND revision = ?`, input.NodeID, input.ExpectedRevision); err != nil {
			return fmt.Errorf("permanently delete execution node: %w", err)
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "EXECUTION_NODE_DELETED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.DeletedAt); err != nil {
			return err
		}
		result = ExecutionNodeDeletionResult{Outcome: "DELETED", AgentAccessRevoked: revoked}
		return nil
	})
	return result, err
}
