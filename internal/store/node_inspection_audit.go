package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// insertNodeCheckRequestAudit 把固定版本查询授权绑定唯一检查标识；自动刷新不能从旧请求继承。
func insertNodeCheckRequestAudit(ctx context.Context, tx *sql.Tx, input ExecutionNodeEnvironmentCheckRequest) error {
	content, err := json.Marshal(struct {
		ObjectID            string `json:"objectId"`
		CheckID             string `json:"checkId"`
		VersionProbeAllowed bool   `json:"versionProbeAllowed"`
	}{input.NodeID, input.CheckID, input.VersionProbeAllowed})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at)
		VALUES(?,'SUBJECT',?,'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED','EXECUTION_NODE',?,'SUCCEEDED',?,?,?)`, auditID(input.NodeID, input.RequestID), input.ActorSubjectID, input.NodeID, input.RequestID, string(content), utcText(input.RequestedAt))
	return err
}

// insertNodeCheckRefreshAudit 记录自动检查来源，明确不携带固定版本进程查询授权。
func insertNodeCheckRefreshAudit(ctx context.Context, tx *sql.Tx, input ExecutionNodeEnvironmentCheckRefresh) error {
	content, err := json.Marshal(struct {
		ObjectID            string `json:"objectId"`
		CheckID             string `json:"checkId"`
		VersionProbeAllowed bool   `json:"versionProbeAllowed"`
	}{input.NodeID, input.CheckID, false})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at)
		VALUES(?,'AGENT',?,'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED','EXECUTION_NODE',?,'SUCCEEDED',?,?,?)`, auditID(input.NodeID, input.RequestID), input.AgentID, input.NodeID, input.RequestID, string(content), utcText(input.RequestedAt))
	return err
}
