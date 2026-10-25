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

func readSchedulingTiming(ctx context.Context, reader sqlQueryReader) (schedulingpolicy.TimingOverrides, error) {
	var raw schedulingpolicy.TimingOverrides
	var heartbeat, startup, polling sql.NullInt64
	var actor, at sql.NullString
	if err := reader.QueryRowContext(ctx, `SELECT heartbeat_seconds,startup_seconds,polling_seconds,revision,modified_by,modified_at FROM platform_scheduling_timing_policy WHERE policy_id='execution-timing'`).Scan(&heartbeat, &startup, &polling, &raw.Revision, &actor, &at); err != nil {
		return raw, err
	}
	for _, item := range []struct {
		value  sql.NullInt64
		target **int
	}{{heartbeat, &raw.HeartbeatSeconds}, {startup, &raw.StartupSeconds}, {polling, &raw.PollingSeconds}} {
		if item.value.Valid {
			value := int(item.value.Int64)
			*item.target = &value
		}
	}
	raw.ModifiedBy = actor.String
	if at.Valid {
		stamp, err := time.Parse(time.RFC3339Nano, at.String)
		if err != nil {
			return raw, schedulingpolicy.ErrUnavailable
		}
		raw.ModifiedAt = &stamp
	}
	if _, err := schedulingpolicy.ResolveTiming(raw, schedulingpolicy.DefaultHeartbeatTimeout); err != nil {
		return raw, err
	}
	return raw, nil
}

// ReadCurrentSchedulingTiming 只供控制面领域端口读取覆盖，不向浏览器绕过授权返回配置。
func (s *Store) ReadCurrentSchedulingTiming(ctx context.Context, baseline time.Duration) (schedulingpolicy.TimingPolicy, error) {
	if s == nil || s.db == nil {
		return schedulingpolicy.TimingPolicy{}, schedulingpolicy.ErrUnavailable
	}
	raw, err := readSchedulingTiming(ctx, s.db)
	if err != nil {
		return schedulingpolicy.TimingPolicy{}, err
	}
	return schedulingpolicy.ResolveTiming(raw, baseline)
}

// ReadAuthorizedSchedulingTiming 在同一读取快照复验系统管理身份并解析真实心跳来源。
func (s *Store) ReadAuthorizedSchedulingTiming(ctx context.Context, actor string, baseline time.Duration) (schedulingpolicy.TimingPolicy, error) {
	var policy schedulingpolicy.TimingPolicy
	if s == nil || s.db == nil {
		return policy, schedulingpolicy.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return policy, err
	}
	defer tx.Rollback()
	if err = permissionManager(ctx, tx, actor); err != nil {
		return policy, schedulingAuthorizationError(err)
	}
	raw, err := readSchedulingTiming(ctx, tx)
	if err != nil {
		return policy, err
	}
	policy, err = schedulingpolicy.ResolveTiming(raw, baseline)
	if err != nil {
		return policy, err
	}
	return policy, tx.Commit()
}

// ChangeSchedulingTiming 将覆盖、版本、审计和回执原子提交，旧回执只确认原事务。
func (s *Store) ChangeSchedulingTiming(ctx context.Context, input schedulingpolicy.TimingChange) (schedulingpolicy.Receipt, error) {
	var result schedulingpolicy.Receipt
	if input.ActorID == "" || input.RequestID == "" || len(input.RequestID) > 200 || input.At.IsZero() || input.ExpectedRevision < 1 || input.ExpectedRevision >= 9007199254740990 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey || strings.ContainsAny(input.IdempotencyKey, "\r\n\t") {
		return result, schedulingpolicy.ErrInvalid
	}
	input.Reason = strings.TrimSpace(input.Reason)
	encoded, _ := json.Marshal(struct {
		Revision         int64
		Heartbeat        int
		Startup, Polling *int
		Reason           string
		Confirmed        bool
	}{input.ExpectedRevision, input.HeartbeatTimeoutSeconds, input.StartupTimeoutSeconds, input.PollingIntervalSeconds, input.Reason, input.Confirmed})
	hash := sha256.Sum256(encoded)
	digest := hex.EncodeToString(hash[:])
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if err := permissionManager(ctx, tx, input.ActorID); err != nil {
			return schedulingAuthorizationError(err)
		}
		var oldDigest, response string
		err := tx.QueryRowContext(ctx, `SELECT request_digest,response_json FROM request_idempotency WHERE subject_id=? AND operation='CHANGE_SCHEDULING_TIMING' AND idempotency_key=?`, input.ActorID, input.IdempotencyKey).Scan(&oldDigest, &response)
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
		raw, err := readSchedulingTiming(ctx, tx)
		if err != nil {
			return err
		}
		before, err := schedulingpolicy.ResolveTiming(raw, input.BaselineHeartbeatTimeout)
		if err != nil {
			return err
		}
		if raw.Revision != input.ExpectedRevision {
			return schedulingpolicy.ErrRevision
		}
		if validPermissionReason(input.Reason) != nil ||
			(input.HeartbeatTimeoutSeconds != before.HeartbeatTimeoutSeconds || raw.HeartbeatSeconds != nil) && (input.HeartbeatTimeoutSeconds < 60 || input.HeartbeatTimeoutSeconds > 3600) ||
			input.StartupTimeoutSeconds != nil && (*input.StartupTimeoutSeconds < 15 || *input.StartupTimeoutSeconds > 3600) ||
			input.PollingIntervalSeconds != nil && (*input.PollingIntervalSeconds < 1 || *input.PollingIntervalSeconds > 60) ||
			raw.StartupSeconds != nil && input.StartupTimeoutSeconds == nil || raw.PollingSeconds != nil && input.PollingIntervalSeconds == nil {
			return schedulingpolicy.ErrInvalid
		}
		if schedulingpolicy.TimingChangeRequiresConfirmation(before, input) && !input.Confirmed {
			return schedulingpolicy.ErrConfirmation
		}
		result = schedulingpolicy.Receipt{Revision: raw.Revision, Changed: input.HeartbeatTimeoutSeconds != before.HeartbeatTimeoutSeconds || !schedulingpolicy.SameSeconds(raw.StartupSeconds, input.StartupTimeoutSeconds) || !schedulingpolicy.SameSeconds(raw.PollingSeconds, input.PollingIntervalSeconds)}
		if result.Changed {
			result.Revision++
			// 仅其他字段变化时不把继承的心跳值写成覆盖，保留实际启动来源。
			if input.HeartbeatTimeoutSeconds != before.HeartbeatTimeoutSeconds {
				raw.HeartbeatSeconds = &input.HeartbeatTimeoutSeconds
			}
			if _, err = tx.ExecContext(ctx, `UPDATE platform_scheduling_timing_policy SET heartbeat_seconds=?,startup_seconds=?,polling_seconds=?,revision=?,modified_by=?,modified_at=? WHERE policy_id='execution-timing'`, raw.HeartbeatSeconds, input.StartupTimeoutSeconds, input.PollingIntervalSeconds, result.Revision, input.ActorID, utcText(input.At)); err != nil {
				return err
			}
		}
		diff := map[string]any{"beforeHeartbeat": before.HeartbeatTimeoutSeconds, "afterHeartbeat": input.HeartbeatTimeoutSeconds, "beforeStartup": before.StartupTimeoutSeconds, "afterStartup": input.StartupTimeoutSeconds, "beforePolling": before.PollingIntervalSeconds, "afterPolling": input.PollingIntervalSeconds, "revision": result.Revision, "changed": result.Changed, "confirmed": input.Confirmed, "reason": input.Reason}
		if err = insertSchedulingTimingAudit(ctx, tx, input, "SUCCEEDED", diff); err != nil {
			return err
		}
		responseBytes, _ := json.Marshal(result)
		_, err = tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES(?,'CHANGE_SCHEDULING_TIMING',?,?,200,'PLATFORM_SETTING','execution-timing',?,?,?)`, input.ActorID, input.IdempotencyKey, digest, string(responseBytes), utcText(input.At), utcText(input.At.Add(24*time.Hour)))
		return err
	})
	if err != nil {
		// 失败记录不持久化非法输入或底层存储错误，审计失败不掩盖原始业务失败。
		_ = s.withWrite(ctx, func(tx *sql.Tx) error {
			if permissionManager(ctx, tx, input.ActorID) != nil {
				return nil
			}
			return insertSchedulingTimingAudit(ctx, tx, input, "FAILED", map[string]any{"reasonCode": "SCHEDULING_TIMING_CHANGE_REJECTED"})
		})
	}
	return result, err
}

func insertSchedulingTimingAudit(ctx context.Context, tx *sql.Tx, input schedulingpolicy.TimingChange, result string, diff map[string]any) error {
	id, err := identifier.NewUUIDV4()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(diff)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,'SCHEDULING_TIMING_CHANGED','PLATFORM_SETTING','execution-timing',?,?,?,?)`, id, input.ActorID, result, input.RequestID, string(raw), utcText(input.At))
	return err
}
