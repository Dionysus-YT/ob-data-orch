package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// NodeActivity 是已经持久化的节点事件/操作投影，不包含审计差异、账号标识或注册码。
type NodeActivity struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	Source     string    `json:"source"`
	Result     string    `json:"result"`
	OccurredAt time.Time `json:"occurredAt"`
}

// NodeActivitySummary 分开返回有界 Agent 事件和用户操作，检查来源绑定当前标识。
type NodeActivitySummary struct {
	Events      []NodeActivity `json:"events"`
	Operations  []NodeActivity `json:"operations"`
	CheckSource string         `json:"checkSource"`
}

// ReadNodeActivity 仅供已通过 NODE_MANAGE 对象校验的控制面使用，并复验账号有效。
// 白名单之外的审计和所有原始差异都不投影，避免把其它业务或敏感材料带到节点详情。
func (s *Store) ReadNodeActivity(ctx context.Context, nodeID, subjectID, checkID string) (NodeActivitySummary, error) {
	result := NodeActivitySummary{Events: make([]NodeActivity, 0), Operations: make([]NodeActivity, 0), CheckSource: "UNKNOWN"}
	rows, err := s.db.QueryContext(ctx, `WITH recent AS (SELECT audit_event_id,action,actor_type,result,occurred_at,
		ROW_NUMBER() OVER(PARTITION BY CASE WHEN actor_type='SUBJECT' THEN 'OPERATION' ELSE 'EVENT' END ORDER BY occurred_at DESC,audit_event_id DESC) AS category_row
		FROM audit_events WHERE object_type='EXECUTION_NODE' AND object_id=?
		AND EXISTS(SELECT 1 FROM auth_subjects WHERE subject_id=? AND account_status='ACTIVE')
		AND action IN ('EXECUTION_NODE_CREATED','EXECUTION_NODE_UPDATED','EXECUTION_NODE_ENABLED','EXECUTION_NODE_ARCHIVED','EXECUTION_NODE_DELETED','AGENT_ENROLLMENT_ISSUED','AGENT_ENROLLED','EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED','EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED','EXECUTION_NODE_ENVIRONMENT_CHECK_COMPLETED')
		) SELECT audit_event_id,action,actor_type,result,occurred_at FROM recent WHERE category_row<=5 ORDER BY occurred_at DESC,audit_event_id DESC`, nodeID, subjectID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item NodeActivity
		var occurred string
		if err := rows.Scan(&item.ID, &item.Action, &item.Source, &item.Result, &occurred); err != nil {
			return result, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return result, err
		}
		item.OccurredAt = parsed
		if item.Source == "SUBJECT" {
			if len(result.Operations) < 5 {
				result.Operations = append(result.Operations, item)
			}
		} else if len(result.Events) < 5 {
			result.Events = append(result.Events, item)
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	// 当前检查来源独立按标识读取，不能被近期大量编辑挤出事件/操作窗口。
	err = s.db.QueryRowContext(ctx, `SELECT actor_type FROM audit_events WHERE object_type='EXECUTION_NODE' AND object_id=?
		AND action IN ('EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED','EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED')
		AND json_extract(safe_diff_json,'$.checkId')=?
		AND EXISTS(SELECT 1 FROM auth_subjects WHERE subject_id=? AND account_status='ACTIVE')
		ORDER BY occurred_at DESC,audit_event_id DESC LIMIT 1`, nodeID, checkID, subjectID).Scan(&result.CheckSource)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return result, err
}
