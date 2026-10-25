package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ob-data-orch/internal/schedulingpolicy"
)

// schedulingTimingForClaim 在领取短事务内使用最新覆盖，成功机器回执必须先于本方法重放。
func schedulingTimingForClaim(ctx context.Context, tx *sql.Tx, input *ExecutionClaimNext, kind string) (schedulingpolicy.TimingOverrides, bool, error) {
	raw, err := readSchedulingTiming(ctx, tx)
	if err != nil {
		return raw, false, err
	}
	if raw.HeartbeatSeconds != nil {
		input.HeartbeatFreshAfter = input.Now.Add(-time.Duration(*raw.HeartbeatSeconds) * time.Second)
	}
	if raw.PollingSeconds == nil {
		return raw, true, nil
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE agent_id=? AND node_id=? AND status='ACTIVE')`, input.AgentID, input.NodeID).Scan(&active); err != nil {
		return raw, false, err
	}
	if !active {
		return raw, false, ErrClaimIneligible
	}
	var stamp string
	err = tx.QueryRowContext(ctx, `SELECT last_checked_at FROM agent_execution_poll_checks WHERE agent_id=? AND task_type=?`, input.AgentID, kind).Scan(&stamp)
	if err == nil {
		last, parseErr := time.Parse(time.RFC3339Nano, stamp)
		if parseErr != nil {
			return raw, false, schedulingpolicy.ErrUnavailable
		}
		if input.Now.Before(last.Add(time.Duration(*raw.PollingSeconds) * time.Second)) {
			return raw, false, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return raw, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_execution_poll_checks(agent_id,task_type,last_checked_at) VALUES(?,?,?) ON CONFLICT(agent_id,task_type) DO UPDATE SET last_checked_at=excluded.last_checked_at`, input.AgentID, kind, utcText(input.Now))
	return raw, err == nil, err
}

// schedulingStartupDeadline 只用于执行首次插入；配置改变不得回填或缩短旧执行期限。
func schedulingStartupDeadline(raw schedulingpolicy.TimingOverrides, now time.Time) any {
	if raw.StartupSeconds == nil {
		return nil
	}
	return utcText(now.Add(time.Duration(*raw.StartupSeconds) * time.Second))
}
