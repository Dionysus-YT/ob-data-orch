package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/logstream"
)

// ChangeSubjectPermissions 在同一短事务中完成复验、全部授权、预检查失效、审计和幂等回执。
// 它不接触认证材料，不修改身份状态，不取消已经运行的任务。
func (s *Store) ChangeSubjectPermissions(ctx context.Context, input accesscontrol.Change) (accesscontrol.ChangeResult, error) {
	result := accesscontrol.ChangeResult{}
	if s == nil || s.db == nil {
		return result, identityUnavailable()
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ActorID == "" || input.SubjectID == "" || input.ExpectedRevision < 1 || input.ExpectedRevision >= 9007199254740991 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || input.RequestID == "" || input.At.IsZero() {
		return result, accesscontrol.ErrInvalid
	}
	input.Assignment = accesscontrol.Canonical(input.Assignment)
	digestInput, _ := json.Marshal(struct {
		Subject    string
		Revision   int64
		Assignment accesscontrol.Assignment
		Reason     string
		Confirmed  bool
	}{input.SubjectID, input.ExpectedRevision, input.Assignment, input.Reason, input.Confirmed})
	digestBytes := sha256.Sum256(digestInput)
	digest := hex.EncodeToString(digestBytes[:])
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if err := permissionManager(ctx, tx, input.ActorID); err != nil {
			return err
		}
		if input.ActorID == input.SubjectID {
			return ErrPermissionSelfReadOnly
		}
		before, err := permissionProfile(ctx, tx, input.SubjectID)
		if err != nil {
			return err
		}
		// 重放也先复验当前管理资格；旧回执只含提交版本，不能恢复已撤销的读取能力。
		var priorDigest, priorJSON string
		err = tx.QueryRowContext(ctx, `SELECT request_digest,response_json FROM request_idempotency WHERE subject_id=? AND operation='CHANGE_SUBJECT_PERMISSIONS' AND idempotency_key=?`, input.ActorID, input.IdempotencyKey).Scan(&priorDigest, &priorJSON)
		if err == nil {
			if priorDigest != digest {
				return ErrIdempotencyConflict
			}
			if json.Unmarshal([]byte(priorJSON), &result) != nil {
				return accesscontrol.ErrInvalid
			}
			result.Replayed = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return ErrRevisionConflict
		}
		if err = validPermissionReason(input.Reason); err != nil {
			return err
		}
		if err = accesscontrol.ValidateChange(before.Assignment, input.Assignment, before.Subject.AccountStatus); err != nil {
			return err
		}
		// 仅审计能力已接入完整消费与撤权边界；其他独立能力不提前开放新授权。
		for _, capability := range input.Assignment.Capabilities {
			if capability != accesscontrol.AuditLog && !slices.Contains(before.Assignment.Capabilities, capability) {
				return ErrPermissionAdditionGated
			}
		}
		risk := newPermissionRisk(before.Assignment, input.Assignment)
		for _, scope := range input.Assignment.Scopes {
			added := !slices.Contains(before.Assignment.Scopes, scope)
			if added && scope.Type == accesscontrol.NodeOperate {
				return ErrPermissionAdditionGated
			}
			available, production, err := permissionObjectFact(ctx, tx, scope)
			if err != nil {
				return err
			}
			if added && !available {
				return accesscontrol.ErrInvalid
			}
			if production && added {
				risk = true
			}
			if production && (scope.Type == accesscontrol.DataSourceUse || scope.Type == accesscontrol.TaskOperateByDataSource) && !input.Assignment.HasCapability(accesscontrol.ProductionTask) {
				return accesscontrol.ErrInvalid
			}
		}
		if risk && !input.Confirmed {
			return ErrPermissionConfirmation
		}
		result.Changed = !reflect.DeepEqual(accesscontrol.Canonical(before.Assignment), input.Assignment)
		result.Revision = before.Revision
		if result.Changed {
			if _, err = tx.ExecContext(ctx, `DELETE FROM subject_grants WHERE subject_id=?`, input.SubjectID); err != nil {
				return err
			}
			for _, code := range permissionCodes(input.Assignment) {
				if _, err = tx.ExecContext(ctx, `INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES(?,?,?,?)`, input.SubjectID, code, input.ActorID, utcText(input.At)); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM subject_object_scopes WHERE subject_id=?`, input.SubjectID); err != nil {
				return err
			}
			for _, scope := range input.Assignment.Scopes {
				if _, err = tx.ExecContext(ctx, `INSERT INTO subject_object_scopes(subject_id,scope_type,object_id,granted_by,granted_at) VALUES(?,?,?,?,?)`, input.SubjectID, scope.Type, scope.ObjectID, input.ActorID, utcText(input.At)); err != nil {
					return err
				}
			}
			result.Revision++
			// 初始身份一旦经普通授权获得或撤销审计能力，也永久消耗部署首次授予入口。
			auditConsumed := before.InitialAdministrator && (before.Assignment.HasCapability(accesscontrol.AuditLog) || input.Assignment.HasCapability(accesscontrol.AuditLog))
			if _, err = tx.ExecContext(ctx, `INSERT INTO subject_permission_versions(subject_id,revision,modified_by,modified_at,audit_bootstrap_consumed) VALUES(?,?,?,?,?) ON CONFLICT(subject_id) DO UPDATE SET revision=excluded.revision,modified_by=excluded.modified_by,modified_at=excluded.modified_at,audit_bootstrap_consumed=MAX(subject_permission_versions.audit_bootstrap_consumed,excluded.audit_bootstrap_consumed)`, input.SubjectID, result.Revision, input.ActorID, utcText(input.At), auditConsumed); err != nil {
				return err
			}
			if err = invalidatePermissionEvidence(ctx, tx, input.SubjectID, input.At); err != nil {
				return err
			}
		}
		diff := map[string]any{"before": accesscontrol.Canonical(before.Assignment), "after": input.Assignment, "reason": input.Reason, "confirmed": input.Confirmed, "changed": result.Changed, "revision": result.Revision}
		if err = insertPermissionAudit(ctx, tx, input, "SUCCEEDED", diff); err != nil {
			return err
		}
		response, _ := json.Marshal(result)
		_, err = tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES(?,'CHANGE_SUBJECT_PERMISSIONS',?,?,200,'SUBJECT',?,?,?,?)`, input.ActorID, input.IdempotencyKey, digest, input.SubjectID, string(response), utcText(input.At), utcText(input.At.Add(24*time.Hour)))
		return err
	})
	if err != nil {
		// 失败记录与已回滚的授权事务隔离；不记录非法输入、秘密或原始错误。
		// 审计不可用时仍返回原失败，绝不因此宣称变更成功。
		_ = s.withWrite(ctx, func(tx *sql.Tx) error {
			if permissionManager(ctx, tx, input.ActorID) != nil {
				return nil
			}
			if _, readErr := permissionProfile(ctx, tx, input.SubjectID); readErr != nil {
				return nil
			}
			return insertPermissionAudit(ctx, tx, input, "FAILED", map[string]any{"reasonCode": "PERMISSION_CHANGE_REJECTED"})
		})
		return accesscontrol.ChangeResult{}, err
	}
	return result, err
}

// invalidatePermissionEvidence 复用普通授权和部署首次授予的失效边界，不改写任务快照或执行租约。
func invalidatePermissionEvidence(ctx context.Context, tx *sql.Tx, subjectID string, at time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE precheck_runs SET status='INVALIDATED',completed_at=? WHERE status IN ('PENDING','LEASED','SUCCEEDED') AND draft_id IN(SELECT draft_id FROM export_drafts WHERE owner_subject_id=?) AND NOT EXISTS(SELECT 1 FROM tasks WHERE tasks.precheck_id=precheck_runs.precheck_id)`, utcText(at), subjectID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE data_source_connection_test_runs SET status='INVALIDATED',completed_at=? WHERE status IN ('PENDING','LEASED') AND creator_subject_id=?`, utcText(at), subjectID)
	return err
}

func validPermissionReason(value string) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 200 {
		return accesscontrol.ErrInvalid
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return accesscontrol.ErrInvalid
		}
	}
	redacted, err := (logstream.Policy{Version: "permission-reason-v1"}).Redact(value)
	if err != nil || redacted != value || strings.Contains(value, "-----BEGIN") {
		return accesscontrol.ErrInvalid
	}
	return nil
}

func permissionCodes(value accesscontrol.Assignment) []string {
	result := make([]string, 0, len(value.Roles)+len(value.Capabilities))
	for _, role := range value.Roles {
		result = append(result, string(role))
	}
	for _, capability := range value.Capabilities {
		result = append(result, string(capability))
	}
	return result
}

func newPermissionRisk(before, after accesscontrol.Assignment) bool {
	if after.HasRole(accesscontrol.SystemAdmin) && !before.HasRole(accesscontrol.SystemAdmin) {
		return true
	}
	for _, capability := range []accesscontrol.Capability{accesscontrol.ProductionTask, accesscontrol.SensitiveCommand, accesscontrol.AuditLog} {
		if slices.Contains(after.Capabilities, capability) && !slices.Contains(before.Capabilities, capability) {
			return true
		}
	}
	return false
}

func permissionObjectFact(ctx context.Context, reader sqlQueryReader, scope accesscontrol.ObjectScope) (available, production bool, err error) {
	var environment string
	switch scope.Type {
	case accesscontrol.DataSourceUse, accesscontrol.DataSourceManage, accesscontrol.TaskOperateByDataSource:
		err = reader.QueryRowContext(ctx, `SELECT CASE WHEN ?='DATA_SOURCE_USE' THEN state='ENABLED' ELSE state!='ARCHIVED' END,environment FROM data_sources WHERE data_source_id=?`, scope.Type, scope.ObjectID).Scan(&available, &environment)
	case accesscontrol.NodeUse, accesscontrol.NodeOperate, accesscontrol.NodeManage:
		err = reader.QueryRowContext(ctx, `SELECT CASE WHEN ?='NODE_USE' THEN management_state='ENABLED' ELSE management_state!='ARCHIVED' END FROM execution_nodes WHERE node_id=?`, scope.Type, scope.ObjectID).Scan(&available)
	case accesscontrol.TemplateUse, accesscontrol.TemplateManage:
		err = reader.QueryRowContext(ctx, `SELECT 1 FROM export_config_templates WHERE template_id=?`, scope.ObjectID).Scan(&available)
	default:
		return false, false, accesscontrol.ErrInvalid
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	return available, environment == "PROD" || environment == "PRODUCTION", err
}

func insertPermissionAudit(ctx context.Context, tx *sql.Tx, input accesscontrol.Change, result string, diff map[string]any) error {
	id, err := identifier.NewUUIDV4()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(diff)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,'SUBJECT_PERMISSIONS_CHANGED','SUBJECT',?,?,?,?,?)`, id, input.ActorID, input.SubjectID, result, input.RequestID, string(encoded), utcText(input.At))
	return err
}

func identityUnavailable() error { return errors.New("权限存储不可用") }
