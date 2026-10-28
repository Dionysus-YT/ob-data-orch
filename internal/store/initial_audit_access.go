package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/identifier"
)

// ErrInitialAuditAccessConsumed 拒绝已授予或已消费的一次性入口，撤权后也不能重授。
var ErrInitialAuditAccessConsumed = errors.New("首次审计授予入口已使用或已有独立授权，不能重复授予")

func initialAuditAccess(ctx context.Context, reader sqlQueryReader, subjectID string) (accesscontrol.InitialAuditAccess, error) {
	out := accesscontrol.InitialAuditAccess{}
	profile, err := permissionProfile(ctx, reader, subjectID)
	if err != nil {
		return out, err
	}
	if err = accesscontrol.Validate(profile.Assignment); err != nil {
		return out, err
	}
	if !profile.InitialAdministrator || profile.Subject.AccountStatus != "ACTIVE" || !profile.Assignment.HasRole(accesscontrol.SystemAdmin) {
		return out, ErrPermissionDenied
	}
	if err = reader.QueryRowContext(ctx, `SELECT audit_bootstrap_consumed FROM subject_permission_versions WHERE subject_id=? AND deployment_owner=1`, subjectID).Scan(&out.Consumed); err != nil {
		return out, err
	}
	out.Revision = profile.Revision
	out.Granted = profile.Assignment.HasCapability(accesscontrol.AuditLog)
	return out, nil
}

// ReadInitialAuditAccess 仅供受信部署入口读取确认依据，浏览器没有对应 API。
func (s *Store) ReadInitialAuditAccess(ctx context.Context, subjectID string) (accesscontrol.InitialAuditAccess, error) {
	if s == nil || s.db == nil {
		return accesscontrol.InitialAuditAccess{}, identityUnavailable()
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return accesscontrol.InitialAuditAccess{}, err
	}
	defer tx.Rollback()
	out, err := initialAuditAccess(ctx, tx, subjectID)
	if err != nil {
		return accesscontrol.InitialAuditAccess{}, err
	}
	return out, tx.Commit()
}

// GrantInitialAuditAccess 只新增初始身份的独立审计能力；标记、版本和安全审计原子提交。
// 普通权限页仍禁止自授，服务启动不调用本方法，也不能重放已撤销的授予。
func (s *Store) GrantInitialAuditAccess(ctx context.Context, input accesscontrol.InitialAuditGrant) (int64, error) {
	if s == nil || s.db == nil {
		return 0, identityUnavailable()
	}
	if input.SubjectID == "" || input.ExpectedRevision < 1 || input.ExpectedRevision >= 9007199254740991 || input.At.IsZero() {
		return 0, accesscontrol.ErrInvalid
	}
	if !input.Confirmed {
		return 0, ErrPermissionConfirmation
	}
	var revision int64
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		before, err := initialAuditAccess(ctx, tx, input.SubjectID)
		if err != nil {
			return err
		}
		if before.Consumed || before.Granted {
			return ErrInitialAuditAccessConsumed
		}
		if before.Revision != input.ExpectedRevision {
			return ErrRevisionConflict
		}
		revision = before.Revision + 1
		if _, err = tx.ExecContext(ctx, `INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES(?,'CAP_AUDIT_LOG',?,?)`, input.SubjectID, input.SubjectID, utcText(input.At)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE subject_permission_versions SET revision=?,audit_bootstrap_consumed=1,modified_by=?,modified_at=? WHERE subject_id=?`, revision, input.SubjectID, utcText(input.At), input.SubjectID); err != nil {
			return err
		}
		if err = invalidatePermissionEvidence(ctx, tx, input.SubjectID, input.At); err != nil {
			return err
		}
		id, err := identifier.NewUUIDV4()
		if err != nil {
			return err
		}
		diff, err := json.Marshal(map[string]any{"confirmed": true, "changed": true, "revision": revision})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,'PERMISSION_INITIAL_AUDIT_GRANTED','SUBJECT',?,'SUCCEEDED',?,?,?)`, id, input.SubjectID, input.SubjectID, id, string(diff), utcText(input.At))
		return err
	})
	if err != nil {
		return 0, err
	}
	return revision, nil
}
