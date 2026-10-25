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
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/schedulingpolicy"
)

func readCandidatePolicy(ctx context.Context, reader sqlQueryReader) (schedulingpolicy.CandidatePolicy, error) {
	policy := schedulingpolicy.CandidatePolicy{SelectionMode: "EXPLICIT_SELECTION", ApplicationMode: "NEXT_CANDIDATE_READ"}
	var actor, at sql.NullString
	if err := reader.QueryRowContext(ctx, `SELECT candidate_order,revision,modified_by,modified_at FROM platform_scheduling_candidate_policy WHERE policy_id='candidate-order'`).Scan(&policy.Order, &policy.Revision, &actor, &at); err != nil {
		return policy, err
	}
	policy.ModifiedBy = actor.String
	if at.Valid {
		stamp, err := time.Parse(time.RFC3339Nano, at.String)
		if err != nil {
			return policy, schedulingpolicy.ErrUnavailable
		}
		policy.ModifiedAt = &stamp
	}
	return policy, schedulingpolicy.ValidateCandidatePolicy(policy)
}

// ReadTaskNodeSelection 在同一只读快照内先核验身份和精确节点范围，再读取候选事实及预算。
// 延续普通/旁路导入的 NODE_MANAGE 读取范围，不附加任务汇总，隐藏坏节点不能阻断候选。
func (s *Store) ReadTaskNodeSelection(ctx context.Context, actor string) ([]ExecutionNode, map[string]NodeBudgetUsage, error) {
	if s == nil || s.db == nil {
		return nil, nil, schedulingpolicy.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	profile, err := permissionProfile(ctx, tx, actor)
	if err != nil || profile.Subject.AccountStatus != "ACTIVE" {
		return nil, nil, schedulingpolicy.ErrDenied
	}
	rows, err := tx.QueryContext(ctx, `SELECT node_id FROM execution_nodes WHERE management_state != 'ARCHIVED' ORDER BY normalized_name`)
	if err != nil {
		return nil, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if profile.Assignment.HasScope(accesscontrol.NodeManage, id) {
			ids = append(ids, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	nodes, err := queryExecutionNodes(ctx, tx, &ids)
	if err != nil {
		return nil, nil, err
	}
	budgets, err := queryNodeBudgetUsages(ctx, tx, &ids)
	if err != nil {
		return nil, nil, err
	}
	return nodes, budgets, tx.Commit()
}

// ReadCurrentCandidatePolicy 供控制面候选读取使用，不绕过浏览器设置权限公开配置。
func (s *Store) ReadCurrentCandidatePolicy(ctx context.Context) (schedulingpolicy.CandidatePolicy, error) {
	if s == nil || s.db == nil {
		return schedulingpolicy.CandidatePolicy{}, schedulingpolicy.ErrUnavailable
	}
	return readCandidatePolicy(ctx, s.db)
}

// ReadAuthorizedCandidatePolicy 在同一只读快照中确认系统管理身份与策略事实。
func (s *Store) ReadAuthorizedCandidatePolicy(ctx context.Context, actor string) (schedulingpolicy.CandidatePolicy, error) {
	var policy schedulingpolicy.CandidatePolicy
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
	policy, err = readCandidatePolicy(ctx, tx)
	if err != nil {
		return policy, err
	}
	return policy, tx.Commit()
}

// ChangeCandidatePolicy 将策略、版本、安全审计与幂等回执放入同一短事务。
func (s *Store) ChangeCandidatePolicy(ctx context.Context, input schedulingpolicy.CandidateChange) (schedulingpolicy.Receipt, error) {
	var result schedulingpolicy.Receipt
	if s == nil || s.db == nil {
		return result, schedulingpolicy.ErrUnavailable
	}
	if input.ActorID == "" || input.RequestID == "" || len(input.RequestID) > 200 || input.At.IsZero() || input.ExpectedRevision < 1 || input.ExpectedRevision >= 9007199254740990 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey || strings.ContainsAny(input.IdempotencyKey, "\r\n\t") {
		return result, schedulingpolicy.ErrInvalid
	}
	input.Reason = strings.TrimSpace(input.Reason)
	encoded, _ := json.Marshal(struct {
		Revision      int64
		Order, Reason string
		Confirmed     bool
	}{input.ExpectedRevision, input.Order, input.Reason, input.Confirmed})
	hash := sha256.Sum256(encoded)
	digest := hex.EncodeToString(hash[:])
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if err := permissionManager(ctx, tx, input.ActorID); err != nil {
			return schedulingAuthorizationError(err)
		}
		var previous, response string
		err := tx.QueryRowContext(ctx, `SELECT request_digest,response_json FROM request_idempotency WHERE subject_id=? AND operation='CHANGE_CANDIDATE_ORDER' AND idempotency_key=?`, input.ActorID, input.IdempotencyKey).Scan(&previous, &response)
		if err == nil {
			if previous != digest {
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
		before, err := readCandidatePolicy(ctx, tx)
		if err != nil {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return schedulingpolicy.ErrRevision
		}
		if !schedulingpolicy.ValidCandidateOrder(input.Order) || validPermissionReason(input.Reason) != nil {
			return schedulingpolicy.ErrInvalid
		}
		result = schedulingpolicy.Receipt{Revision: before.Revision, Changed: input.Order != before.Order}
		if result.Changed {
			if !input.Confirmed {
				return schedulingpolicy.ErrConfirmation
			}
			result.Revision++
			if _, err = tx.ExecContext(ctx, `UPDATE platform_scheduling_candidate_policy SET candidate_order=?,revision=?,modified_by=?,modified_at=? WHERE policy_id='candidate-order'`, input.Order, result.Revision, input.ActorID, utcText(input.At)); err != nil {
				return err
			}
		}
		if err = insertCandidateAudit(ctx, tx, input, "SUCCEEDED", map[string]any{"beforeOrder": before.Order, "afterOrder": input.Order, "revision": result.Revision, "changed": result.Changed, "reason": input.Reason, "confirmed": input.Confirmed}); err != nil {
			return err
		}
		responseBytes, _ := json.Marshal(result)
		_, err = tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES(?,'CHANGE_CANDIDATE_ORDER',?,?,200,'PLATFORM_SETTING','candidate-order',?,?,?)`, input.ActorID, input.IdempotencyKey, digest, string(responseBytes), utcText(input.At), utcText(input.At.Add(24*time.Hour)))
		return err
	})
	if err != nil {
		// 失败审计仅记录固定原因，不保存非法枚举、原因原文或存储错误。
		_ = s.withWrite(ctx, func(tx *sql.Tx) error {
			if permissionManager(ctx, tx, input.ActorID) != nil {
				return nil
			}
			return insertCandidateAudit(ctx, tx, input, "FAILED", map[string]any{"reasonCode": "CANDIDATE_ORDER_CHANGE_REJECTED"})
		})
	}
	return result, err
}

func insertCandidateAudit(ctx context.Context, tx *sql.Tx, input schedulingpolicy.CandidateChange, result string, diff map[string]any) error {
	id, err := identifier.NewUUIDV4()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(diff)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,'CANDIDATE_ORDER_CHANGED','PLATFORM_SETTING','candidate-order',?,?,?,?)`, id, input.ActorID, result, input.RequestID, string(raw), utcText(input.At))
	return err
}

// ReadCandidateHeartbeat 仅在调用方已核验节点对象权限后读取最小心跳事实。
// 无 ACTIVE Agent 或事实版本未知时返回空值；不读取机器路径、环境报文或任务内容。
func (s *Store) ReadCandidateHeartbeat(ctx context.Context, nodeID string) (*time.Time, error) {
	if s == nil || s.db == nil {
		return nil, schedulingpolicy.ErrUnavailable
	}
	var heartbeat sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT a.last_heartbeat_at FROM execution_nodes n LEFT JOIN agents a ON a.node_id=n.node_id AND a.status='ACTIVE' AND a.facts_revision>0 WHERE n.node_id=? AND n.management_state='ENABLED'`, nodeID).Scan(&heartbeat)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !heartbeat.Valid {
		return nil, nil
	}
	stamp, err := time.Parse(time.RFC3339Nano, heartbeat.String)
	if err != nil || stamp.IsZero() {
		return nil, schedulingpolicy.ErrUnavailable
	}
	return &stamp, nil
}
