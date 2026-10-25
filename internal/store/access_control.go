package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"ob-data-orch/internal/accesscontrol"
)

// ErrPermissionDenied 统一表示主体、角色或授权目录不可访问，不泄露目标是否存在。
var ErrPermissionDenied = errors.New("权限配置不可访问")

// ErrPermissionSelfReadOnly 禁止通过普通权限页面修改自己的任何授权。
var ErrPermissionSelfReadOnly = errors.New("当前用户的权限只能查看")

// ErrPermissionConfirmation 表示本次新增的高风险职责或范围尚未确认。
var ErrPermissionConfirmation = errors.New("权限变更需要风险确认")

// ErrPermissionAdditionGated 阻断尚未接入完整消费门禁的能力或范围新增，旧记录仍可保留或撤销。
var ErrPermissionAdditionGated = errors.New("该独立能力或运行范围尚未准入新增授权")

// permissionProfile 在调用方已有事务中读取角色、范围和版本，绝不读取业务秘密。
func permissionProfile(ctx context.Context, reader sqlQueryReader, subjectID string) (accesscontrol.Profile, error) {
	var result accesscontrol.Profile
	var modifiedAt string
	err := reader.QueryRowContext(ctx, `SELECT a.subject_id,a.external_subject,a.display_name,a.account_status,
        COALESCE(v.revision,1),COALESCE(v.deployment_owner,0),COALESCE(v.modified_by,''),COALESCE(v.modified_at,'')
        FROM auth_subjects a LEFT JOIN subject_permission_versions v ON v.subject_id=a.subject_id WHERE a.subject_id=?`, subjectID).
		Scan(&result.Subject.ID, &result.Subject.Login, &result.Subject.DisplayName, &result.Subject.AccountStatus, &result.Revision, &result.InitialAdministrator, &result.ModifiedBy, &modifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrPermissionDenied
	}
	if err != nil {
		return result, err
	}
	if result.InitialAdministrator {
		// DEC-048 的固定登录名已确认；external_subject 继续保留为不可变认证映射。
		result.Subject.Login = "admin"
	}
	if modifiedAt != "" {
		at, err := time.Parse(time.RFC3339Nano, modifiedAt)
		if err != nil {
			return result, err
		}
		result.ModifiedAt = &at
	}
	result.Assignment = accesscontrol.Canonical(accesscontrol.Assignment{})
	result.ScopeObjects = []accesscontrol.ScopeObject{}
	rows, err := reader.QueryContext(ctx, `SELECT grant_code FROM subject_grants WHERE subject_id=? ORDER BY grant_code`, subjectID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			rows.Close()
			return result, err
		}
		if strings.HasPrefix(code, "ROLE_") {
			result.Assignment.Roles = append(result.Assignment.Roles, accesscontrol.Role(code))
		} else {
			result.Assignment.Capabilities = append(result.Assignment.Capabilities, accesscontrol.Capability(code))
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = reader.QueryContext(ctx, `SELECT scope_type,object_id FROM subject_object_scopes WHERE subject_id=? ORDER BY scope_type,object_id LIMIT 201`, subjectID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item accesscontrol.ObjectScope
		if err := rows.Scan(&item.Type, &item.ObjectID); err != nil {
			rows.Close()
			return result, err
		}
		result.Assignment.Scopes = append(result.Assignment.Scopes, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Assignment.Scopes) > 200 {
		return result, accesscontrol.ErrInvalid
	}
	return result, nil
}

func permissionManager(ctx context.Context, reader sqlQueryReader, actorID string) error {
	var allowed bool
	if err := reader.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_subjects a JOIN subject_grants g ON g.subject_id=a.subject_id
        WHERE a.subject_id=? AND a.account_status='ACTIVE' AND g.grant_code='ROLE_SYSTEM_ADMIN')`, actorID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

// ReadPermissionProfile 只向当前 ACTIVE 权限管理员返回目标主体；自身始终只读。
func (s *Store) ReadPermissionProfile(ctx context.Context, actorID, subjectID string) (accesscontrol.Profile, error) {
	if s == nil || s.db == nil {
		return accesscontrol.Profile{}, identityUnavailable()
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return accesscontrol.Profile{}, err
	}
	defer tx.Rollback()
	if err = permissionManager(ctx, tx, actorID); err != nil {
		return accesscontrol.Profile{}, err
	}
	result, err := permissionProfile(ctx, tx, subjectID)
	if err != nil {
		return result, err
	}
	result.Editable = actorID != subjectID
	for _, scope := range result.Assignment.Scopes {
		item := accesscontrol.ScopeObject{Type: scope.Type, ObjectID: scope.ObjectID, DisplayName: "对象已不可新增"}
		var query string
		switch scope.Type {
		case accesscontrol.DataSourceUse, accesscontrol.DataSourceManage, accesscontrol.TaskOperateByDataSource:
			query = `SELECT display_name,environment FROM data_sources WHERE data_source_id=?`
		case accesscontrol.NodeUse, accesscontrol.NodeOperate, accesscontrol.NodeManage:
			query = `SELECT display_name,'' FROM execution_nodes WHERE node_id=?`
		case accesscontrol.TemplateUse, accesscontrol.TemplateManage:
			query = `SELECT display_name,'' FROM export_config_templates WHERE template_id=?`
		default:
			return result, accesscontrol.ErrInvalid
		}
		err = tx.QueryRowContext(ctx, query, scope.ObjectID).Scan(&item.DisplayName, &item.Environment)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		item.Available, _, err = permissionObjectFact(ctx, tx, scope)
		if err != nil {
			return result, err
		}
		result.ScopeObjects = append(result.ScopeObjects, item)
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

// ListPermissionSubjects 的筛选只作用于已认证投影；先鉴权再计量，分页不查询无权主体。
func (s *Store) ListPermissionSubjects(ctx context.Context, actorID string, q accesscontrol.SubjectQuery) (accesscontrol.SubjectPage, error) {
	result := accesscontrol.SubjectPage{Items: []accesscontrol.Profile{}}
	if s == nil || s.db == nil {
		return result, identityUnavailable()
	}
	if q.Limit < 1 || q.Limit > 50 || !utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 128 {
		return result, accesscontrol.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = permissionManager(ctx, tx, actorID); err != nil {
		return result, err
	}
	clauses := []string{"a.subject_id>?"}
	args := []any{q.AfterID}
	if q.Search != "" {
		clauses = append(clauses, "(instr(lower(CASE WHEN EXISTS(SELECT 1 FROM subject_permission_versions pv WHERE pv.subject_id=a.subject_id AND pv.deployment_owner=1) THEN 'admin' ELSE a.external_subject END),lower(?))>0 OR instr(lower(a.display_name),lower(?))>0)")
		args = append(args, q.Search, q.Search)
	}
	if q.Status != "" {
		clauses = append(clauses, "a.account_status=?")
		args = append(args, q.Status)
	}
	for _, grant := range []string{string(q.Role), string(q.Capability)} {
		if grant != "" {
			clauses = append(clauses, "EXISTS(SELECT 1 FROM subject_grants g WHERE g.subject_id=a.subject_id AND g.grant_code=?)")
			args = append(args, grant)
		}
	}
	args = append(args, q.Limit+1)
	rows, err := tx.QueryContext(ctx, "SELECT a.subject_id FROM auth_subjects a WHERE "+strings.Join(clauses, " AND ")+" ORDER BY a.subject_id LIMIT ?", args...)
	if err != nil {
		return result, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(ids) > q.Limit {
		result.HasMore = true
		ids = ids[:q.Limit]
	}
	for _, id := range ids {
		profile, err := permissionProfile(ctx, tx, id)
		if err != nil {
			return result, err
		}
		profile.Editable = id != actorID
		result.Items = append(result.Items, profile)
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

// ListPermissionCandidates 是授权元信息目录，权限管理资格不转化为业务详情访问资格。
func (s *Store) ListPermissionCandidates(ctx context.Context, actorID string, kind accesscontrol.ScopeType, search, after string, limit int) ([]accesscontrol.Candidate, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, identityUnavailable()
	}
	if limit < 1 || limit > 50 || !utf8.ValidString(search) || utf8.RuneCountInString(search) > 128 {
		return nil, false, accesscontrol.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if err = permissionManager(ctx, tx, actorID); err != nil {
		return nil, false, err
	}
	var query string
	switch kind {
	case accesscontrol.DataSourceUse, accesscontrol.DataSourceManage, accesscontrol.TaskOperateByDataSource:
		query = `SELECT data_source_id,display_name,'DATA_SOURCE',environment,CASE WHEN ?='DATA_SOURCE_USE' THEN state='ENABLED' ELSE state!='ARCHIVED' END FROM data_sources WHERE data_source_id>? AND instr(lower(display_name),lower(?))>0 ORDER BY data_source_id LIMIT ?`
	case accesscontrol.NodeUse, accesscontrol.NodeManage, accesscontrol.NodeOperate:
		query = `SELECT node_id,display_name,'NODE','',CASE WHEN ?='NODE_USE' THEN management_state='ENABLED' ELSE management_state!='ARCHIVED' END FROM execution_nodes WHERE node_id>? AND instr(lower(display_name),lower(?))>0 ORDER BY node_id LIMIT ?`
	case accesscontrol.TemplateUse, accesscontrol.TemplateManage:
		query = `SELECT template_id,display_name,'TEMPLATE','',? IN ('TEMPLATE_USE','TEMPLATE_MANAGE') FROM export_config_templates WHERE template_id>? AND instr(lower(display_name),lower(?))>0 ORDER BY template_id LIMIT ?`
	default:
		return nil, false, accesscontrol.ErrInvalid
	}
	rows, err := tx.QueryContext(ctx, query, kind, after, search, limit+1)
	if err != nil {
		return nil, false, err
	}
	result := []accesscontrol.Candidate{}
	for rows.Next() {
		var item accesscontrol.Candidate
		if err = rows.Scan(&item.ID, &item.DisplayName, &item.Kind, &item.Environment, &item.Available); err != nil {
			rows.Close()
			return nil, false, err
		}
		result = append(result, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	more := len(result) > limit
	if more {
		result = result[:limit]
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return result, more, nil
}

// BootstrapPermissionOwner 只在可信组合根为现有初始管理员引导一次；重启不恢复撤销项。
func (s *Store) BootstrapPermissionOwner(ctx context.Context, subjectID string, at time.Time) error {
	if s == nil || s.db == nil {
		return identityUnavailable()
	}
	if subjectID == "" || at.IsZero() {
		return accesscontrol.ErrInvalid
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		var active bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_subjects WHERE subject_id=? AND account_status='ACTIVE')`, subjectID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return ErrPermissionDenied
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT subject_id FROM subject_permission_versions WHERE deployment_owner=1`).Scan(&existing)
		if err == nil {
			if existing != subjectID {
				return ErrPermissionDenied
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO subject_permission_versions(subject_id,revision,deployment_owner,modified_by,modified_at) VALUES(?,1,1,?,?) ON CONFLICT(subject_id) DO UPDATE SET deployment_owner=1,modified_by=excluded.modified_by,modified_at=excluded.modified_at`, subjectID, subjectID, utcText(at)); err != nil {
			return err
		}
		for _, code := range []string{string(accesscontrol.BasicUser), string(accesscontrol.Operator), string(accesscontrol.DataSourceAdmin), string(accesscontrol.NodeAdmin), string(accesscontrol.SystemAdmin), string(accesscontrol.ProductionTask), string(accesscontrol.ExpertConfig)} {
			if _, err = tx.ExecContext(ctx, `INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, subjectID, code, subjectID, utcText(at)); err != nil {
				return err
			}
		}
		for _, scope := range []struct{ kind, query string }{{"DATA_SOURCE_USE", "SELECT data_source_id AS object_id FROM data_sources"}, {"DATA_SOURCE_MANAGE", "SELECT data_source_id AS object_id FROM data_sources"}, {"NODE_USE", "SELECT node_id AS object_id FROM execution_nodes"}, {"NODE_MANAGE", "SELECT node_id AS object_id FROM execution_nodes"}, {"TEMPLATE_USE", "SELECT template_id AS object_id FROM export_config_templates WHERE owner_subject_id=?"}, {"TEMPLATE_MANAGE", "SELECT template_id AS object_id FROM export_config_templates WHERE owner_subject_id=?"}} {
			args := []any{subjectID, scope.kind, subjectID, utcText(at)}
			if strings.Contains(scope.query, "?") {
				args = append(args, subjectID)
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO subject_object_scopes(subject_id,scope_type,object_id,granted_by,granted_at) SELECT ?,?,object_id,?,? FROM (`+scope.query+`) AS scope_objects WHERE 1 ON CONFLICT DO NOTHING`, args...); err != nil {
				return err
			}
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM subject_object_scopes WHERE subject_id=?`, subjectID).Scan(&count); err != nil {
			return err
		}
		// 超过当前固定上限时整次引导失败关闭，不截断旧范围或启动后留下无法读取的管理员。
		if count > 200 {
			return accesscontrol.ErrInvalid
		}
		return insertAudit(ctx, tx, "SYSTEM", subjectID, "PERMISSION_INITIAL_ADMIN_BOUND", "SUBJECT", subjectID, "SUCCEEDED", "permission-bootstrap", at)
	})
}
