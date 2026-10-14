package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// NodeTaskSummary 是本人任务的完整聚合，不从分页列表或 Agent 槽位猜测。
// 状态核对单独计数，避免同时计入运行与等待。
type NodeTaskSummary struct {
	Scope       string `json:"scope,omitempty"`
	Running     int    `json:"running"`
	Waiting     int    `json:"waiting"`
	Reconciling int    `json:"reconciling"`
}

// ListOwnedNodeTaskSummaries 仅统计当前有效身份创建的任务，不泄露共享节点上其他账号的任务。
// 调用方还须裁剪节点读取范围；不存在任务的授权节点由调用方明确投影为零。
func (s *Store) ListOwnedNodeTaskSummaries(ctx context.Context, subjectID string) (map[string]NodeTaskSummary, error) {
	return s.ListNodeTaskSummaries(ctx, subjectID, false)
}

// ListNodeTaskSummaries 的全局读取标志只能由服务端可信能力投影提供，SQL 仍复验账号有效性。
func (s *Store) ListNodeTaskSummaries(ctx context.Context, subjectID string, globalRead bool) (map[string]NodeTaskSummary, error) {
	if s == nil || s.db == nil || strings.TrimSpace(subjectID) == "" {
		return nil, errors.New("node task summary subject is invalid")
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT t.node_id,
               SUM(CASE WHEN COALESCE(e.reconciliation_required,0) = 0 AND e.state IN ('STARTING','RUNNING','CANCELLING') THEN 1 ELSE 0 END),
               SUM(CASE WHEN e.execution_id IS NULL THEN 1 ELSE 0 END),
               SUM(CASE WHEN e.reconciliation_required = 1 THEN 1 ELSE 0 END)
        FROM tasks t
        JOIN auth_subjects subject ON subject.subject_id = ? AND subject.account_status = 'ACTIVE'
        LEFT JOIN task_executions e ON e.task_id = t.task_id
        WHERE t.creator_subject_id = ? OR ? = 1
        GROUP BY t.node_id`, subjectID, subjectID, globalRead)
	if err != nil {
		return nil, fmt.Errorf("read owned node task summary: %w", err)
	}
	defer rows.Close()
	summaries := make(map[string]NodeTaskSummary)
	for rows.Next() {
		var nodeID string
		var summary NodeTaskSummary
		if err := rows.Scan(&nodeID, &summary.Running, &summary.Waiting, &summary.Reconciling); err != nil {
			return nil, err
		}
		summaries[nodeID] = summary
	}
	return summaries, rows.Err()
}
