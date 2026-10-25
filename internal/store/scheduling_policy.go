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

	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/schedulingpolicy"
)

// readSchedulingPolicy 只读取独立上限与版本，缺失或损坏时失败关闭。
func readSchedulingPolicy(ctx context.Context, reader sqlQueryReader) (schedulingpolicy.Policy, error) {
	var result schedulingpolicy.Policy
	var maximum sql.NullInt64
	var actor, at sql.NullString
	err := reader.QueryRowContext(ctx, `SELECT max_concurrent_executions,revision,modified_by,modified_at FROM platform_scheduling_policy WHERE policy_id='platform-executions'`).Scan(&maximum, &result.Revision, &actor, &at)
	if err != nil {
		return result, err
	}
	result.Source = "UNCONFIGURED"
	result.ApplicationMode = "NEXT_CLAIM"
	if result.Revision < 1 || result.Revision >= 9007199254740991 {
		return result, schedulingpolicy.ErrUnavailable
	}
	if maximum.Valid {
		if maximum.Int64 < 1 || maximum.Int64 > schedulingpolicy.MaxExecutions || !actor.Valid || actor.String == "" || !at.Valid {
			return result, schedulingpolicy.ErrUnavailable
		}
		value := int(maximum.Int64)
		result.MaxConcurrentExecutions = &value
		stamp, err := time.Parse(time.RFC3339Nano, at.String)
		if err != nil || stamp.IsZero() {
			return result, schedulingpolicy.ErrUnavailable
		}
		result.ModifiedBy = actor.String
		result.ModifiedAt = &stamp
		result.Source = "PLATFORM_SETTING"
	} else if actor.Valid || at.Valid {
		return result, schedulingpolicy.ErrUnavailable
	}
	return result, nil
}

// ReadAuthorizedSchedulingPolicy 在同一读取快照内检查当前系统管理员与策略。
func (s *Store) ReadAuthorizedSchedulingPolicy(ctx context.Context, actor string) (schedulingpolicy.Policy, error) {
	if s == nil || s.db == nil {
		return schedulingpolicy.Policy{}, schedulingpolicy.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return schedulingpolicy.Policy{}, err
	}
	defer tx.Rollback()
	if err = permissionManager(ctx, tx, actor); err != nil {
		return schedulingpolicy.Policy{}, schedulingAuthorizationError(err)
	}
	result, err := readSchedulingPolicy(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Usage = schedulingpolicy.Usage{Visibility: "RESTRICTED", ObservedAt: time.Now().UTC()}
	profile, err := permissionProfile(ctx, tx, actor)
	if err != nil {
		return result, err
	}
	if initialGlobalTaskRead(profile) {
		held, running, err := schedulingUsage(ctx, tx)
		if err != nil {
			return result, err
		}
		result.Usage.Visibility = "AVAILABLE"
		result.Usage.HeldExecutions = &held
		result.Usage.RunningExecutions = &running
	}
	if err == nil {
		err = tx.Commit()
	}
	return result, err
}

func schedulingAuthorizationError(err error) error {
	if errors.Is(err, ErrPermissionDenied) {
		return schedulingpolicy.ErrDenied
	}
	return err
}

// ChangeSchedulingPolicy 在短事务内复验权限、版本、降低确认，并与审计及幂等回执同时提交。
func (s *Store) ChangeSchedulingPolicy(ctx context.Context, input schedulingpolicy.Change) (schedulingpolicy.Receipt, error) {
	var result schedulingpolicy.Receipt
	if input.ActorID == "" || input.RequestID == "" || len(input.RequestID) > 200 || input.At.IsZero() || input.ExpectedRevision < 1 || input.ExpectedRevision >= 9007199254740990 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey || strings.ContainsAny(input.IdempotencyKey, "\r\n\t") {
		return result, schedulingpolicy.ErrInvalid
	}
	input.Reason = strings.TrimSpace(input.Reason)
	encoded, _ := json.Marshal(struct {
		Revision  int64
		Maximum   int
		Reason    string
		Confirmed bool
	}{input.ExpectedRevision, input.MaxConcurrentExecutions, input.Reason, input.Confirmed})
	hash := sha256.Sum256(encoded)
	digest := hex.EncodeToString(hash[:])
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if err := permissionManager(ctx, tx, input.ActorID); err != nil {
			return schedulingAuthorizationError(err)
		}
		var oldDigest, response string
		err := tx.QueryRowContext(ctx, `SELECT request_digest,response_json FROM request_idempotency WHERE subject_id=? AND operation='CHANGE_SCHEDULING_POLICY' AND idempotency_key=?`, input.ActorID, input.IdempotencyKey).Scan(&oldDigest, &response)
		if err == nil {
			if digest != oldDigest {
				return schedulingpolicy.ErrIdempotency
			}
			if json.Unmarshal([]byte(response), &result) != nil || result.Revision < 1 {
				return schedulingpolicy.ErrUnavailable
			}
			result.Replayed = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		before, err := readSchedulingPolicy(ctx, tx)
		if err != nil {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return schedulingpolicy.ErrRevision
		}
		if input.MaxConcurrentExecutions < 1 || input.MaxConcurrentExecutions > schedulingpolicy.MaxExecutions || validPermissionReason(input.Reason) != nil {
			return schedulingpolicy.ErrInvalid
		}
		if schedulingpolicy.Tightens(before.MaxConcurrentExecutions, input.MaxConcurrentExecutions) && !input.Confirmed {
			return schedulingpolicy.ErrConfirmation
		}
		result = schedulingpolicy.Receipt{Revision: before.Revision, Changed: before.MaxConcurrentExecutions == nil || input.MaxConcurrentExecutions != *before.MaxConcurrentExecutions}
		afterSource := before.Source
		if result.Changed {
			result.Revision++
			afterSource = "PLATFORM_SETTING"
			if _, err = tx.ExecContext(ctx, `UPDATE platform_scheduling_policy SET max_concurrent_executions=?,revision=?,modified_by=?,modified_at=? WHERE policy_id='platform-executions'`, input.MaxConcurrentExecutions, result.Revision, input.ActorID, utcText(input.At)); err != nil {
				return err
			}
		}
		diff := map[string]any{"beforeMaximum": before.MaxConcurrentExecutions, "afterMaximum": input.MaxConcurrentExecutions, "beforeSource": before.Source, "afterSource": afterSource, "revision": result.Revision, "changed": result.Changed, "reason": input.Reason, "confirmed": input.Confirmed}
		if err = insertSchedulingAudit(ctx, tx, input, "SUCCEEDED", diff); err != nil {
			return err
		}
		responseBytes, _ := json.Marshal(result)
		_, err = tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES(?,'CHANGE_SCHEDULING_POLICY',?,?,200,'PLATFORM_SETTING','platform-executions',?,?,?)`, input.ActorID, input.IdempotencyKey, digest, string(responseBytes), utcText(input.At), utcText(input.At.Add(24*time.Hour)))
		return err
	})
	if err != nil {
		// 回滚后的失败审计仅含固定原因；非法原因与原始存储错误不得持久化。
		_ = s.withWrite(ctx, func(tx *sql.Tx) error {
			if permissionManager(ctx, tx, input.ActorID) != nil {
				return nil
			}
			return insertSchedulingAudit(ctx, tx, input, "FAILED", map[string]any{"reasonCode": "SCHEDULING_POLICY_CHANGE_REJECTED"})
		})
	}
	return result, err
}

func insertSchedulingAudit(ctx context.Context, tx *sql.Tx, input schedulingpolicy.Change, result string, diff map[string]any) error {
	id, err := identifier.NewUUIDV4()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(diff)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,'SCHEDULING_POLICY_CHANGED','PLATFORM_SETTING','platform-executions',?,?,?,?)`, id, input.ActorID, result, input.RequestID, string(raw), utcText(input.At))
	return err
}
