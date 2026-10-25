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
	"ob-data-orch/internal/logretention"
)

func readLogRetention(ctx context.Context, reader sqlQueryReader) (logretention.Record, error) {
	var result logretention.Record
	var days sql.NullInt64
	var actor, at sql.NullString
	err := reader.QueryRowContext(ctx, `SELECT retention_days,revision,modified_by,modified_at FROM ordinary_log_retention_policy WHERE policy_id='ordinary-task-logs'`).Scan(&days, &result.Revision, &actor, &at)
	if err != nil {
		return result, err
	}
	if days.Valid {
		value := int(days.Int64)
		result.Days = &value
	}
	result.ModifiedBy = actor.String
	if at.Valid {
		value, err := time.Parse(time.RFC3339Nano, at.String)
		if err != nil {
			return result, logretention.ErrUnavailable
		}
		result.ModifiedAt = &value
	}
	return result, nil
}

// ReadCurrentLogRetention 仅供控制面组合的清理策略服务读取，不提供浏览器绕过入口。
func (s *Store) ReadCurrentLogRetention(ctx context.Context) (logretention.Record, error) {
	if s == nil || s.db == nil {
		return logretention.Record{}, logretention.ErrUnavailable
	}
	return readLogRetention(ctx, s.db)
}

// ReadAuthorizedLogRetention 在同一读取快照内检查当前系统管理员与策略。
func (s *Store) ReadAuthorizedLogRetention(ctx context.Context, actor string) (logretention.Record, error) {
	if s == nil || s.db == nil {
		return logretention.Record{}, logretention.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return logretention.Record{}, err
	}
	defer tx.Rollback()
	if err = permissionManager(ctx, tx, actor); err != nil {
		return logretention.Record{}, retentionAuthorizationError(err)
	}
	result, err := readLogRetention(ctx, tx)
	if err == nil {
		err = tx.Commit()
	}
	return result, err
}

func retentionAuthorizationError(err error) error {
	if errors.Is(err, ErrPermissionDenied) {
		return logretention.ErrDenied
	}
	return err
}

// ChangeLogRetention 在短事务内复验权限、版本、降低确认，并与审计及幂等回执同时提交。
func (s *Store) ChangeLogRetention(ctx context.Context, input logretention.Change, deploymentDays int) (logretention.Receipt, error) {
	var result logretention.Receipt
	if input.ActorID == "" || input.RequestID == "" || len(input.RequestID) > 200 || input.At.IsZero() || input.ExpectedRevision < 1 || input.ExpectedRevision >= 9007199254740990 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey || strings.ContainsAny(input.IdempotencyKey, "\r\n\t") {
		return result, logretention.ErrInvalid
	}
	input.Reason = strings.TrimSpace(input.Reason)
	encoded, _ := json.Marshal(struct {
		Revision  int64
		Days      int
		Reason    string
		Confirmed bool
	}{input.ExpectedRevision, input.RetentionDays, input.Reason, input.Confirmed})
	hash := sha256.Sum256(encoded)
	digest := hex.EncodeToString(hash[:])
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if err := permissionManager(ctx, tx, input.ActorID); err != nil {
			return retentionAuthorizationError(err)
		}
		var oldDigest, response string
		err := tx.QueryRowContext(ctx, `SELECT request_digest,response_json FROM request_idempotency WHERE subject_id=? AND operation='CHANGE_LOG_RETENTION' AND idempotency_key=?`, input.ActorID, input.IdempotencyKey).Scan(&oldDigest, &response)
		if err == nil {
			if digest != oldDigest {
				return logretention.ErrIdempotency
			}
			if json.Unmarshal([]byte(response), &result) != nil || result.Revision < 1 {
				return logretention.ErrUnavailable
			}
			result.Replayed = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		record, err := readLogRetention(ctx, tx)
		if err != nil {
			return err
		}
		before, err := logretention.Resolve(record, deploymentDays)
		if err != nil {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return logretention.ErrRevision
		}
		if input.RetentionDays < 7 || input.RetentionDays > 365 || validPermissionReason(input.Reason) != nil {
			return logretention.ErrInvalid
		}
		if input.RetentionDays < before.RetentionDays && !input.Confirmed {
			return logretention.ErrConfirmation
		}
		result = logretention.Receipt{Revision: before.Revision, Changed: input.RetentionDays != before.RetentionDays}
		afterSource := before.Source
		if result.Changed {
			result.Revision++
			afterSource = "PLATFORM_SETTING"
			if _, err = tx.ExecContext(ctx, `UPDATE ordinary_log_retention_policy SET retention_days=?,revision=?,modified_by=?,modified_at=? WHERE policy_id='ordinary-task-logs'`, input.RetentionDays, result.Revision, input.ActorID, utcText(input.At)); err != nil {
				return err
			}
		}
		diff := map[string]any{"beforeDays": before.RetentionDays, "afterDays": input.RetentionDays, "beforeSource": before.Source, "afterSource": afterSource, "revision": result.Revision, "changed": result.Changed, "reason": input.Reason, "confirmed": input.Confirmed}
		if err = insertRetentionAudit(ctx, tx, input, "SUCCEEDED", diff); err != nil {
			return err
		}
		responseBytes, _ := json.Marshal(result)
		_, err = tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES(?,'CHANGE_LOG_RETENTION',?,?,200,'PLATFORM_SETTING','ordinary-task-logs',?,?,?)`, input.ActorID, input.IdempotencyKey, digest, string(responseBytes), utcText(input.At), utcText(input.At.Add(24*time.Hour)))
		return err
	})
	if err != nil {
		// 回滚后的失败审计仅含固定原因；非法原因与原始存储错误不得持久化。
		_ = s.withWrite(ctx, func(tx *sql.Tx) error {
			if permissionManager(ctx, tx, input.ActorID) != nil {
				return nil
			}
			return insertRetentionAudit(ctx, tx, input, "FAILED", map[string]any{"reasonCode": "LOG_RETENTION_CHANGE_REJECTED"})
		})
	}
	return result, err
}

func insertRetentionAudit(ctx context.Context, tx *sql.Tx, input logretention.Change, result string, diff map[string]any) error {
	id, err := identifier.NewUUIDV4()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(diff)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,'LOG_RETENTION_CHANGED','PLATFORM_SETTING','ordinary-task-logs',?,?,?,?)`, id, input.ActorID, result, input.RequestID, string(raw), utcText(input.At))
	return err
}
