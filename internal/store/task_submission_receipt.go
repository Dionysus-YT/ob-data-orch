package store

import (
	"context"
	"database/sql"
	"errors"

	"ob-data-orch/internal/accesscontrol"
)

// ErrTaskSubmissionConflict 表示同一草稿修订已经冻结任务，不能以新请求再次执行。
var ErrTaskSubmissionConflict = errors.New("draft revision has already been submitted")

// TaskSubmissionLookup 绑定原始提交意图；只读取回执，不重新校验已消费证明的时效。
type TaskSubmissionLookup struct {
	SubjectID, TaskType, DraftID, PrecheckID, IdempotencyKey, RequestDigest string
	DraftRevision                                                           int64
}

func taskSubmissionOperation(kind string) string {
	switch kind {
	case "OBLOADER_NORMAL_IMPORT":
		return "SUBMIT_NORMAL_IMPORT_DRAFT"
	case "OBLOADER_DIRECT_LOAD":
		return "SUBMIT_DIRECT_LOAD_DRAFT"
	default:
		return "SUBMIT_EXPORT_DRAFT"
	}
}

// LookupTaskSubmission 在当前所有者与权限范围内找回原回执，不能以历史成功绕过撤权。
func (s *Store) LookupTaskSubmission(ctx context.Context, input TaskSubmissionLookup) (TaskSubmissionResult, error) {
	if (input.TaskType != "OBDUMPER_EXPORT" && input.TaskType != "OBLOADER_NORMAL_IMPORT" && input.TaskType != "OBLOADER_DIRECT_LOAD") || input.SubjectID == "" || input.DraftID == "" || input.DraftRevision < 1 || input.PrecheckID == "" || len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 || !isSHA256(input.RequestDigest) {
		return TaskSubmissionResult{}, ErrIdempotencyConflict
	}
	var result TaskSubmissionResult
	// 只读快照统一回执与权限事实；新冻结仍在 submitTaskTx 的写事务内再次检查消费状态。
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	err = func() error {
		var digest, id string
		err := tx.QueryRowContext(ctx, `SELECT request_digest,COALESCE(resource_id,'') FROM request_idempotency WHERE subject_id=? AND operation=? AND idempotency_key=?`, input.SubjectID, taskSubmissionOperation(input.TaskType), input.IdempotencyKey).Scan(&digest, &id)
		if err == nil {
			if digest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			var matched bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks t JOIN precheck_runs p ON p.precheck_id=t.precheck_id JOIN export_drafts d ON d.draft_id=p.draft_id WHERE t.task_id=? AND t.creator_subject_id=? AND t.task_type=? AND p.draft_id=? AND p.draft_revision=? AND p.precheck_id=? AND d.owner_subject_id=? AND d.deleted_at IS NULL)`, id, input.SubjectID, input.TaskType, input.DraftID, input.DraftRevision, input.PrecheckID, input.SubjectID).Scan(&matched); err != nil {
				return err
			}
			if !matched {
				return ErrIdempotencyConflict
			}
			if err := authorizeTaskSubmissionReceipt(ctx, tx, id, input.SubjectID); err != nil {
				return err
			}
			result = TaskSubmissionResult{TaskID: id, Replayed: true}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		// 历史允许同一证明生成多个任务；仅阻断新写入，不添加会使升级失败的唯一约束。
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT t.task_id FROM tasks t JOIN precheck_runs p ON p.precheck_id=t.precheck_id JOIN export_drafts d ON d.draft_id=p.draft_id WHERE p.draft_id=? AND p.draft_revision=? AND d.owner_subject_id=? AND d.deleted_at IS NULL ORDER BY t.submitted_at,t.task_id LIMIT 1`, input.DraftID, input.DraftRevision, input.SubjectID).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := authorizeTaskSubmissionReceipt(ctx, tx, existing, input.SubjectID); err != nil {
			return err
		}
		return ErrTaskSubmissionConflict
	}()
	if err == nil {
		err = tx.Commit()
	}
	return result, err
}

// authorizeTaskSubmissionReceipt 在同一事务内复验当前提交权限，不要求历史证明或凭据仍有效。
func authorizeTaskSubmissionReceipt(ctx context.Context, tx *sql.Tx, id, subject string) error {
	var allowed, managed bool
	var kind, source, environment, snapshot, metadata string
	if err := tx.QueryRowContext(ctx, `SELECT (`+taskPermissionEligibility("t")+` AND EXISTS(SELECT 1 FROM auth_subjects a WHERE a.subject_id=t.creator_subject_id AND a.account_status='ACTIVE')),`+managedPermissionExists+`,t.task_type,t.data_source_id,d.environment,t.snapshot_json,t.metadata_version FROM tasks t JOIN data_sources d ON d.data_source_id=t.data_source_id WHERE t.task_id=? AND t.creator_subject_id=?`, id, subject).Scan(&allowed, &managed, &kind, &source, &environment, &snapshot, &metadata); err != nil {
		return ErrClaimIneligible
	}
	if !allowed {
		return ErrClaimIneligible
	}
	if !managed || kind == "OBDUMPER_EXPORT" {
		return nil
	}
	profile, err := permissionProfile(ctx, tx, subject)
	if err != nil || !profile.Assignment.HasScope(accesscontrol.DataSourceManage, source) {
		return ErrClaimIneligible
	}
	if (submissionNeedsExpert(TaskSubmission{TaskType: kind, SnapshotJSON: snapshot, MetadataVersion: metadata}) || environment == "PROD" || environment == "PRODUCTION") && !initialGlobalTaskRead(profile) {
		return ErrClaimIneligible
	}
	return nil
}

// submittedDraftTaskID 仅投影唯一且当前仍获授权的结果；历史多任务不猜测用户要恢复哪一个。
func (s *Store) submittedDraftTaskID(ctx context.Context, owner, draft string, revision int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var id string
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(t.task_id),'') FROM tasks t JOIN precheck_runs p ON p.precheck_id=t.precheck_id WHERE p.draft_id=? AND p.draft_revision=? AND t.creator_subject_id=?`, draft, revision, owner).Scan(&count, &id); err != nil {
		return "", err
	}
	if count != 1 || authorizeTaskSubmissionReceipt(ctx, tx, id, owner) != nil {
		return "", nil
	}
	return id, tx.Commit()
}
