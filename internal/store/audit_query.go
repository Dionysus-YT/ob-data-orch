package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/auditquery"
	"ob-data-orch/internal/identifier"
)

const auditTimeKey = `(substr(a.occurred_at,1,19)||substr((CASE WHEN substr(a.occurred_at,20,1)='.' THEN substr(a.occurred_at,21,length(a.occurred_at)-21) ELSE '' END)||'000000000',1,9))`
const auditRowKey = `(` + auditTimeKey + `||'|'||a.audit_event_id)`

func auditProfile(ctx context.Context, tx *sql.Tx, subject string) (accesscontrol.Profile, string, error) {
	p, err := permissionProfile(ctx, tx, subject)
	if errors.Is(err, ErrPermissionDenied) {
		return p, "", auditquery.ErrDenied
	}
	if err != nil {
		return p, "", err
	}
	if p.Subject.AccountStatus != "ACTIVE" || !p.Assignment.HasCapability(accesscontrol.AuditLog) {
		return p, "", auditquery.ErrDenied
	}
	raw, err := json.Marshal(struct {
		Revision   int64
		Initial    bool
		Assignment accesscontrol.Assignment
	}{p.Revision, p.InitialAdministrator, accesscontrol.Canonical(p.Assignment)})
	if err != nil {
		return p, "", err
	}
	sum := sha256.Sum256(raw)
	return p, hex.EncodeToString(sum[:]), nil
}

// AuditQueryAccess 不因普通系统管理员、初始身份或已有登录会话隐含授予审计能力。
func (s *Store) AuditQueryAccess(ctx context.Context, subject string) (bool, error) {
	_, err := s.AuditQueryScope(ctx, subject)
	if errors.Is(err, auditquery.ErrDenied) {
		return false, nil
	}
	return err == nil, err
}

// AuditQueryScope 在短读事务中取得当前授权指纹和追加水位，不读取正文或对象候选。
func (s *Store) AuditQueryScope(ctx context.Context, subject string) (auditquery.Scope, error) {
	out := auditquery.Scope{}
	if s == nil || s.db == nil {
		return out, auditquery.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	_, out.Fingerprint, err = auditProfile(ctx, tx, subject)
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM audit_query_positions`).Scan(&out.Watermark)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// auditPredicate 先裁剪业务对象，再允许读取事件差异。平台事件的安全投影属于显式审计职责，
// 不由该能力派生业务详情、数据源使用、节点操作或任务读取资格；未知事件域默认排除。
func auditPredicate(p accesscontrol.Profile) (string, []any, error) {
	subject := p.Subject.ID
	v := p.Assignment
	task, args, err := buildTaskListPredicate(TaskListQuery{SubjectID: subject, GlobalRead: initialGlobalTaskRead(p)})
	if err != nil {
		return "", nil, err
	}
	parts := []string{`a.object_type IN ('SUBJECT','PLATFORM_SETTING','STORAGE_CREDENTIAL','LOG_QUERY')`,
		`EXISTS(SELECT 1 FROM tasks t WHERE (` + task + `) AND ((a.object_type='TASK' AND t.task_id=a.object_id) OR (a.object_type='TASK_EXECUTION' AND EXISTS(SELECT 1 FROM task_executions execution WHERE execution.task_id=t.task_id AND execution.execution_id=a.object_id))))`}
	addIDs := func(kind string, ids []string) {
		if len(ids) == 0 {
			return
		}
		parts = append(parts, kind+` IN (`+placeholders(len(ids))+`)`)
		for _, id := range ids {
			args = append(args, id)
		}
	}
	var sources, nodes []string
	for _, scope := range v.Scopes {
		switch scope.Type {
		case accesscontrol.DataSourceManage:
			if v.HasScope(scope.Type, scope.ObjectID) {
				sources = append(sources, scope.ObjectID)
			}
		case accesscontrol.DataSourceUse:
			// 生产资格失效时使用范围不能借审计继续发现生产对象；管理范围另按职责处理。
			if v.HasScope(scope.Type, scope.ObjectID) && v.HasCapability(accesscontrol.ProductionTask) {
				sources = append(sources, scope.ObjectID)
			}
			if v.HasScope(scope.Type, scope.ObjectID) && !v.HasCapability(accesscontrol.ProductionTask) {
				parts = append(parts, `(a.object_type='DATA_SOURCE' AND a.object_id=? AND EXISTS(SELECT 1 FROM data_sources source WHERE source.data_source_id=a.object_id AND source.environment NOT IN ('PROD','PRODUCTION')))`)
				args = append(args, scope.ObjectID)
			}
		case accesscontrol.NodeManage, accesscontrol.NodeOperate:
			if v.HasScope(scope.Type, scope.ObjectID) {
				nodes = append(nodes, scope.ObjectID)
			}
		}
	}
	addIDs(`(CASE WHEN a.object_type='DATA_SOURCE' THEN a.object_id ELSE NULL END)`, sources)
	addIDs(`(CASE WHEN a.object_type='EXECUTION_NODE' THEN a.object_id WHEN a.object_type='AGENT' THEN (SELECT agent.node_id FROM agents agent WHERE agent.agent_id=a.object_id) ELSE NULL END)`, nodes)
	if v.HasRole(accesscontrol.BasicUser) {
		parts = append(parts, `(a.object_type='EXPORT_DRAFT' AND EXISTS(SELECT 1 FROM export_drafts draft WHERE draft.draft_id=a.object_id AND draft.owner_subject_id=?))`)
		args = append(args, subject)
	}
	return "(" + strings.Join(parts, " OR ") + ")", args, nil
}

// AuditQueryRows 每批限 128 行；权限指纹变化、非 ACTIVE 和空能力一律停止旧快照。
// 扫描预算以 UTF-8 字节计费，不能用 SQLite 字符数少计中文标识。
// 超长差异仅取长度，避免将未知正文读入响应路径；不按原始差异做搜索、计数或候选。
func (s *Store) AuditQueryRows(ctx context.Context, w auditquery.Window) ([]auditquery.Event, error) {
	if s == nil || s.db == nil {
		return nil, auditquery.ErrUnavailable
	}
	if w.Limit < 1 || w.Limit > 128 {
		return nil, auditquery.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	p, fingerprint, err := auditProfile(ctx, tx, w.Subject)
	if err != nil {
		return nil, err
	}
	if fingerprint != w.Fingerprint {
		return nil, auditquery.ErrCursor
	}
	predicate, args, err := auditPredicate(p)
	if err != nil {
		return nil, err
	}
	predicate += ` AND pos.sequence<=? AND ` + auditTimeKey + `>=? AND ` + auditTimeKey + `<?`
	args = append(args, w.Watermark, timeQueryKey(w.Filter.From), timeQueryKey(w.Filter.To))
	for _, f := range []struct{ col, value string }{{"actor_id", w.Filter.ActorID}, {"object_type", w.Filter.ObjectType}, {"object_id", w.Filter.ObjectID}, {"action", w.Filter.Action}, {"result", w.Filter.Result}} {
		if f.value != "" {
			predicate += ` AND a.` + f.col + `=?`
			args = append(args, f.value)
		}
	}
	if w.Before != "" {
		predicate += ` AND ` + auditRowKey + `<?`
		args = append(args, w.Before)
	}
	if w.EventID != "" {
		predicate += ` AND a.audit_event_id=?`
		args = append(args, w.EventID)
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+auditRowKey+`,a.audit_event_id,a.actor_type,a.actor_id,a.action,a.object_type,COALESCE(a.object_id,''),a.result,a.request_id,a.occurred_at,
 CASE WHEN length(CAST(a.safe_diff_json AS BLOB))<=16384 THEN a.safe_diff_json ELSE '' END,
 length(CAST(a.safe_diff_json AS BLOB))+length(CAST(a.audit_event_id AS BLOB))+length(CAST(a.actor_id AS BLOB))+length(CAST(a.action AS BLOB))+length(CAST(COALESCE(a.object_id,'') AS BLOB))+length(CAST(a.request_id AS BLOB))+100
 FROM audit_events a JOIN audit_query_positions pos ON pos.audit_event_id=a.audit_event_id WHERE `+predicate+` ORDER BY `+auditRowKey+` DESC LIMIT ?`, append(args, w.Limit)...)
	if err != nil {
		return nil, err
	}
	out := []auditquery.Event{}
	for rows.Next() {
		var e auditquery.Event
		var at, diff string
		if err = rows.Scan(&e.Key, &e.ID, &e.ActorType, &e.ActorID, &e.Action, &e.ObjectType, &e.ObjectID, &e.Result, &e.RequestID, &at, &diff, &e.Bytes); err != nil {
			rows.Close()
			return nil, err
		}
		e.At, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			rows.Close()
			return nil, err
		}
		e.Diff = []byte(diff)
		out = append(out, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// AuditQueryAccessEvent 将访问结果与最终授权复验共同提交；关键词仅留摘要，令牌不持久化。
func (s *Store) AuditQueryAccessEvent(ctx context.Context, a auditquery.Access) error {
	if !oneOf(a.Action, "AUDIT_LOG_QUERY", "AUDIT_LOG_DETAIL", "AUDIT_LOG_DOWNLOAD") || !oneOf(a.Result, "SUCCEEDED", "FAILED") || a.Count < 0 || a.Count > 100000 || a.Subject == "" || a.RequestID == "" || a.At.IsZero() {
		return auditquery.ErrInvalid
	}
	digest := sha256.Sum256([]byte(a.Filter.Keyword))
	raw, err := json.Marshal(map[string]any{"from": a.Filter.From, "to": a.Filter.To, "actorId": a.Filter.ActorID, "objectType": a.Filter.ObjectType, "objectId": a.Filter.ObjectID, "action": a.Filter.Action, "result": a.Filter.Result, "keywordSha256": hex.EncodeToString(digest[:]), "records": a.Count, "redacted": true})
	if err != nil {
		return err
	}
	id, err := identifier.NewUUIDV4()
	if err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		_, fingerprint, err := auditProfile(ctx, tx, a.Subject)
		if err != nil {
			return err
		}
		if fingerprint != a.Fingerprint {
			return auditquery.ErrCursor
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,?,'LOG_QUERY','audit-logs',?,?,?,?)`, id, a.Subject, a.Action, a.Result, a.RequestID, string(raw), utcText(a.At))
		return err
	})
}
