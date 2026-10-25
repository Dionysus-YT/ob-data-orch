package store

import (
	"context"
	"database/sql"
	"time"

	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/schedulingpolicy"
)

// ReconcileSchedulingTimeouts 只附加待核对并保留原状态、租约和预算；审计失败整批回滚。
func (s *Store) ReconcileSchedulingTimeouts(ctx context.Context, now time.Time, heartbeatBaseline time.Duration) (int, error) {
	if now.IsZero() {
		return 0, schedulingpolicy.ErrInvalid
	}
	count := 0
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		raw, err := readSchedulingTiming(ctx, tx)
		if err != nil {
			return err
		}
		policy, err := schedulingpolicy.ResolveTiming(raw, heartbeatBaseline)
		if err != nil {
			return err
		}
		cutoff := utcText(now.Add(-time.Duration(policy.HeartbeatTimeoutSeconds) * time.Second))
		rows, err := tx.QueryContext(ctx, `SELECT e.execution_id,
            CASE WHEN e.state='STARTING' AND e.startup_deadline IS NOT NULL AND e.startup_deadline<=? THEN 'STARTUP_TIMEOUT' ELSE 'HEARTBEAT_TIMEOUT' END
            FROM task_executions e JOIN agents a ON a.agent_id=e.agent_id
            WHERE e.state IN ('STARTING','RUNNING','CANCELLING') AND e.reconciliation_required=0 AND `+budgetStillHeld+`
            AND ((e.state='STARTING' AND e.startup_deadline IS NOT NULL AND e.startup_deadline<=?) OR a.last_heartbeat_at<?)
            ORDER BY e.execution_id LIMIT 64`, utcText(now), utcText(now), cutoff)
		if err != nil {
			return err
		}
		type expired struct{ id, reason string }
		var items []expired
		for rows.Next() {
			var item expired
			if err = rows.Scan(&item.id, &item.reason); err != nil {
				rows.Close()
				return err
			}
			items = append(items, item)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		for _, item := range items {
			if _, err = tx.ExecContext(ctx, `UPDATE task_executions SET reconciliation_required=1,revision=revision+1,updated_at=? WHERE execution_id=? AND reconciliation_required=0`, utcText(now), item.id); err != nil {
				return err
			}
			id, err := identifier.NewUUIDV4()
			if err != nil {
				return err
			}
			// 固定原因不包含进程参数、节点路径或任何秘密；不根据超时合成终态。
			if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SYSTEM','control-plane','TASK_SCHEDULING_RECONCILIATION_REQUIRED','TASK_EXECUTION',?,'SUCCEEDED','scheduling-monitor',?,?)`, id, item.id, `{"reasonCode":"`+item.reason+`"}`, utcText(now)); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
