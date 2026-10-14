package store

import (
	"context"
	"errors"
	"time"
)

// TaskOperation 只含任务级动作事实，不含审计差异、日志正文、命令或机器身份材料。
type TaskOperation struct {
	ID         string    `json:"id"`
	ActorType  string    `json:"actorType"`
	ActorName  string    `json:"actorName"`
	Action     string    `json:"action"`
	Result     string    `json:"result"`
	OccurredAt time.Time `json:"occurredAt"`
}

// ListTaskOperations 在与列表相同的授权谓词内合并任务审计和执行事件，分页按时间与记录 ID 稳定排序。
func (s *Store) ListTaskOperations(ctx context.Context, query TaskListQuery, before time.Time, beforeID string) ([]TaskOperation, error) {
	if query.TaskID == "" || (beforeID == "") != before.IsZero() {
		return nil, errors.New("task operation scope is invalid")
	}
	predicate, args, err := buildTaskListPredicate(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
        WITH readable AS (SELECT t.task_id, e.execution_id `+taskListFrom+` WHERE `+predicate+`),
        records AS (
            SELECT 'audit:' || a.audit_event_id AS id, a.actor_type AS actor_type,
                   CASE WHEN a.actor_type = 'SUBJECT' THEN COALESCE(person.display_name, '未知用户') WHEN a.actor_type = 'AGENT' THEN 'Agent' ELSE '系统' END AS actor_name,
                   a.action AS action, a.result AS result, a.occurred_at AS occurred_at
            FROM readable r JOIN audit_events a ON (a.object_type='TASK' AND a.object_id=r.task_id) OR (a.object_type='TASK_EXECUTION' AND a.object_id=r.execution_id)
            LEFT JOIN auth_subjects person ON person.subject_id=a.actor_id
            WHERE a.action IN ('TASK_SUBMITTED','TASK_CLAIMED','TASK_CANCELLATION_REQUESTED','TASK_CANCELLATION_NOOP','TASK_CANCELLATION_TIMEOUT','TASK_RESUMED_FROM_CHECKPOINT','TASK_EXECUTION_LEASE_EXPIRED','TASK_EXECUTION_RECONCILIATION_REQUIRED','TASK_TEMPLATE_SAVED','TASK_DRAFT_DERIVED')
            UNION ALL
            SELECT 'event:' || event.event_id, 'AGENT', 'Agent', event.event_type, CASE WHEN event.accepted = 1 THEN 'RECORDED' ELSE 'DENIED' END, event.received_at
            FROM readable r JOIN execution_events event ON event.execution_id=r.execution_id
        )
        SELECT id, actor_type, actor_name, action, result, occurred_at FROM records
        WHERE ? = '' OR julianday(occurred_at) < julianday(?) OR (julianday(occurred_at) = julianday(?) AND id < ?)
        ORDER BY julianday(occurred_at) DESC, id DESC LIMIT 51`, append(args, beforeID, utcText(before), utcText(before), beforeID)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TaskOperation{}
	for rows.Next() {
		var item TaskOperation
		var instant string
		if err := rows.Scan(&item.ID, &item.ActorType, &item.ActorName, &item.Action, &item.Result, &instant); err != nil {
			return nil, err
		}
		if item.OccurredAt, err = parseTaskListTime(instant, true); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
