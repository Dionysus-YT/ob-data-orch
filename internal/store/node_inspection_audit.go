package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// insertNodeCheckRequestAudit 为本次检查保存当前机器持续授权的快照，旧检查本身不授予未来权限。
func insertNodeCheckRequestAudit(ctx context.Context, tx *sql.Tx, input ExecutionNodeEnvironmentCheckRequest, agentID string, probeAllowed bool) error {
	content, err := json.Marshal(struct {
		ObjectID            string `json:"objectId"`
		CheckID             string `json:"checkId"`
		AgentID             string `json:"agentId"`
		VersionProbeAllowed bool   `json:"versionProbeAllowed"`
	}{input.NodeID, input.CheckID, agentID, probeAllowed})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at)
		VALUES(?,'SUBJECT',?,'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED','EXECUTION_NODE',?,'SUCCEEDED',?,?,?)`, auditID(input.NodeID, input.RequestID), input.ActorSubjectID, input.NodeID, input.RequestID, string(content), utcText(input.RequestedAt))
	return err
}

// insertNodeCheckRefreshAudit 记录自动检查来源和当前身份的固定版本查询许可，不携带自由命令或数据任务授权。
func insertNodeCheckRefreshAudit(ctx context.Context, tx *sql.Tx, input ExecutionNodeEnvironmentCheckRefresh, probeAllowed bool) error {
	content, err := json.Marshal(struct {
		ObjectID            string `json:"objectId"`
		CheckID             string `json:"checkId"`
		AgentID             string `json:"agentId"`
		VersionProbeAllowed bool   `json:"versionProbeAllowed"`
	}{input.NodeID, input.CheckID, input.AgentID, probeAllowed})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at)
		VALUES(?,'AGENT',?,'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED','EXECUTION_NODE',?,'SUCCEEDED',?,?,?)`, auditID(input.NodeID, input.RequestID), input.AgentID, input.NodeID, input.RequestID, string(content), utcText(input.RequestedAt))
	return err
}

// insertNodeEnrollmentInspectionAudit 记录签发关联材料时授予的固定本机复验范围，不记录一次性材料。
func insertNodeEnrollmentInspectionAudit(ctx context.Context, tx *sql.Tx, input AgentEnrollmentIssue) error {
	content, err := json.Marshal(struct {
		ObjectID                        string `json:"objectId"`
		EnvironmentInspectionAuthorized bool   `json:"environmentInspectionAuthorized"`
	}{input.NodeID, input.EnvironmentInspectionAuthorized})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at)
		VALUES(?,'SUBJECT',?,'AGENT_ENROLLMENT_ISSUED','EXECUTION_NODE',?,'SUCCEEDED',?,?,?)`, auditID(input.NodeID, input.RequestID), input.ActorID, input.NodeID, input.RequestID, string(content), utcText(input.CreatedAt))
	return err
}
