package store

import (
	"context"
	"errors"
)

// NodeExecutionBinding 是节点诊断读取所需的授权与当前租约绑定，不含任务内容。
type NodeExecutionBinding struct {
	TaskID      string
	ExecutionID string
	LeaseID     string
	LeaseEpoch  int64
}

// ListAuthorizedNodeExecutionBindings 将 Agent 自报执行与数据库权威租约及当前任务读取范围核对。
// globalRead 只能由可信全局角色与能力共同提供；节点管理权限不能授权其他账号的执行标识。
func (s *Store) ListAuthorizedNodeExecutionBindings(ctx context.Context, nodeID, agentID, subjectID string, globalRead bool) ([]NodeExecutionBinding, error) {
	if s == nil || s.db == nil || nodeID == "" || agentID == "" || subjectID == "" {
		return nil, errors.New("node execution observation scope is invalid")
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT t.task_id, e.execution_id, l.lease_id, l.lease_epoch
        FROM task_executions e
        JOIN tasks t ON t.task_id = e.task_id
        JOIN execution_leases l ON l.execution_id = e.execution_id AND l.agent_id = e.agent_id
        JOIN agents a ON a.agent_id = e.agent_id AND a.node_id = e.node_id AND a.status = 'ACTIVE'
        JOIN auth_subjects subject ON subject.subject_id = ? AND subject.account_status = 'ACTIVE'
        WHERE e.node_id = ? AND e.agent_id = ? AND e.state IN ('STARTING','RUNNING','CANCELLING')
          AND l.lease_epoch = (SELECT MAX(current.lease_epoch) FROM execution_leases current WHERE current.execution_id = e.execution_id)
          AND l.status != 'RELEASED'
          AND ((? = 1 AND `+managedGlobalTaskRead()+`) OR (t.creator_subject_id = ? AND `+managedTaskOwnerRead("t")+`) OR EXISTS (
              SELECT 1 FROM subject_object_scopes task_scope WHERE task_scope.subject_id = ?
                AND task_scope.scope_type = 'TASK_OPERATE_BY_DATA_SOURCE' AND `+managedTaskOperatorRead("task_scope")+` AND task_scope.object_id = t.data_source_id))
        ORDER BY e.execution_id
    `, subjectID, nodeID, agentID, globalRead, subjectID, subjectID, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := make([]NodeExecutionBinding, 0)
	for rows.Next() {
		var binding NodeExecutionBinding
		if err := rows.Scan(&binding.TaskID, &binding.ExecutionID, &binding.LeaseID, &binding.LeaseEpoch); err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}
