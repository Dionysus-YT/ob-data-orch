package store

import (
	"context"
	"database/sql"

	"ob-data-orch/internal/schedulingpolicy"
)

// schedulingHeldPredicate 与既有节点预算采用同一可靠释放证据，不按租约超时释放。
const schedulingHeldPredicate = `(e.state IN ('STARTING','RUNNING','CANCELLING') OR e.reconciliation_required=1
 OR EXISTS(SELECT 1 FROM execution_resource_allocations allocation WHERE allocation.execution_id=e.execution_id)) AND ` + budgetStillHeld

func schedulingUsage(ctx context.Context, reader sqlQueryReader) (held, running int, err error) {
	err = reader.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN e.state='RUNNING' THEN 1 ELSE 0 END),0) FROM task_executions e WHERE `+schedulingHeldPredicate).Scan(&held, &running)
	return
}

// schedulingAvailable 只能在领取的同一短写事务中用于准入，网页观察不能替代该检查。
func schedulingAvailable(ctx context.Context, reader sqlQueryReader, newlyInserted bool) (bool, error) {
	if _, err := readSchedulingTiming(ctx, reader); err != nil {
		return false, err
	}
	policy, err := readSchedulingPolicy(ctx, reader)
	if err != nil {
		return false, err
	}
	if policy.MaxConcurrentExecutions == nil {
		return true, nil
	}
	held, _, err := schedulingUsage(ctx, reader)
	if err != nil {
		return false, err
	}
	if newlyInserted {
		held--
	}
	return held < *policy.MaxConcurrentExecutions, nil
}

func enforceInsertedSchedulingCapacity(ctx context.Context, tx *sql.Tx) error {
	available, err := schedulingAvailable(ctx, tx, true)
	if err != nil {
		return err
	}
	if !available {
		return schedulingpolicy.ErrCapacity
	}
	return nil
}

// taskSchedulingWaitReason 只在已授权任务详情中投影固定原因，不暴露全局计数。
func (s *Store) taskSchedulingWaitReason(ctx context.Context) string {
	available, err := schedulingAvailable(ctx, s.db, false)
	if err != nil {
		return "SCHEDULING_POLICY_UNAVAILABLE"
	}
	if !available {
		return "PLATFORM_CONCURRENCY_LIMIT"
	}
	return ""
}
