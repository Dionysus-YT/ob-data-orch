package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrExportDraftDeleteIneligible 表示草稿已提交或有未结束的预检查租约，不允许删除。
var ErrExportDraftDeleteIneligible = errors.New("export draft cannot be deleted")

// ExportDraftListQuery 只查询当前主体自己的未提交草稿；没有全部账号范围。
type ExportDraftListQuery struct {
	SubjectID     string
	Keyword       string
	Limit         int
	BeforeUpdated time.Time
	BeforeID      string
}

// ExportDraftListItem 是草稿管理的安全投影，不含路径、配置 JSON、凭据引用或命令。
type ExportDraftListItem struct {
	ID               string    `json:"id"`
	Type             string    `json:"type"`
	DisplayName      string    `json:"displayName"`
	DataSourceID     string    `json:"dataSourceId"`
	NodeID           string    `json:"nodeId"`
	Revision         int64     `json:"revision"`
	SourceTaskID     string    `json:"sourceTaskId,omitempty"`
	SourceDerivation string    `json:"sourceDerivation,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	AvailableActions []string  `json:"availableActions"`
}

// ListOwnedExportDrafts 的授权、关键字、数量与游标均在 SQL 中生效，已提交或删除的草稿不进入列表。
func (s *Store) ListOwnedExportDrafts(ctx context.Context, query ExportDraftListQuery) ([]ExportDraftListItem, int, error) {
	if query.SubjectID == "" || query.Limit < 1 || query.Limit > 100 || len(query.Keyword) > 128 || strings.ContainsAny(query.Keyword, "\x00\r\n") || (query.BeforeID == "") != query.BeforeUpdated.IsZero() {
		return nil, 0, errors.New("export draft list query is invalid")
	}
	predicate := `d.owner_subject_id = ? AND d.deleted_at IS NULL AND d.draft_kind = 'OBDUMPER_EXPORT'
      AND EXISTS (SELECT 1 FROM auth_subjects a WHERE a.subject_id = d.owner_subject_id AND a.account_status = 'ACTIVE')
      AND NOT EXISTS (SELECT 1 FROM tasks t JOIN precheck_runs p ON p.precheck_id=t.precheck_id WHERE p.draft_id=d.draft_id)`
	args := []any{query.SubjectID}
	if query.Keyword != "" {
		predicate += ` AND (d.draft_id LIKE ? ESCAPE '\' OR COALESCE(json_extract(d.config_json,'$.database'),json_extract(d.config_json,'$.objectScope.database'),'') LIKE ? ESCAPE '\' OR COALESCE(json_extract(d.config_json,'$.table'),'') LIKE ? ESCAPE '\')`
		pattern := "%" + escapeLikePattern(strings.TrimSpace(query.Keyword)) + "%"
		args = append(args, pattern, pattern, pattern)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM export_drafts d WHERE `+predicate, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if !query.BeforeUpdated.IsZero() {
		predicate += ` AND (d.updated_at < ? OR (d.updated_at = ? AND d.draft_id < ?))`
		args = append(args, utcText(query.BeforeUpdated), utcText(query.BeforeUpdated), query.BeforeID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.draft_id,d.data_source_id,COALESCE(d.node_id,''),d.revision,
      COALESCE(json_extract(d.config_json,'$.database'),json_extract(d.config_json,'$.objectScope.database'),''), COALESCE(json_extract(d.config_json,'$.table'),''),
      COALESCE(d.source_task_id,''),COALESCE(d.source_derivation,''),d.created_at,d.updated_at,
      EXISTS (SELECT 1 FROM precheck_runs p WHERE p.draft_id=d.draft_id AND p.status='LEASED' AND (julianday(p.lease_expires_at) IS NULL OR julianday(p.lease_expires_at)>julianday(?)))
      FROM export_drafts d WHERE `+predicate+` ORDER BY d.updated_at DESC,d.draft_id DESC LIMIT ?`, append(append([]any{utcText(time.Now())}, args...), query.Limit+1)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []ExportDraftListItem{}
	for rows.Next() {
		var item ExportDraftListItem
		var database, table, created, updated string
		var leased bool
		if err := rows.Scan(&item.ID, &item.DataSourceID, &item.NodeID, &item.Revision, &database, &table, &item.SourceTaskID, &item.SourceDerivation, &created, &updated, &leased); err != nil {
			return nil, 0, err
		}
		item.Type = "OBDUMPER_EXPORT"
		item.DisplayName = "导出草稿"
		if database != "" {
			item.DisplayName = database
			if table != "" {
				item.DisplayName += "." + table
			}
		}
		item.AvailableActions = []string{"CONTINUE_EDIT", "SUBMIT"}
		if !leased {
			item.AvailableActions = append(item.AvailableActions, "DELETE")
		}
		if item.CreatedAt, err = parseTaskListTime(created, true); err != nil {
			return nil, 0, err
		}
		if item.UpdatedAt, err = parseTaskListTime(updated, true); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// DeleteOwnedExportDraft 在短事务中复验所有权、版本、提交和租约，失效预检查并登记删除与幂等结果。
func (s *Store) DeleteOwnedExportDraft(ctx context.Context, id, subjectID string, revision int64, key, requestID string, now time.Time) (bool, error) {
	if !validAgentOpaqueValue(id, 256) || !validAgentOpaqueValue(subjectID, 256) || revision < 1 || len(key) < 16 || len(key) > 200 || requestID == "" || now.IsZero() {
		return false, errors.New("export draft deletion input is invalid")
	}
	sum := sha256.Sum256([]byte(id + "|" + strconv.FormatInt(revision, 10)))
	digest := hex.EncodeToString(sum[:])
	replayed := false
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var actualRevision int64
		var deleted sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT d.revision,d.deleted_at FROM export_drafts d WHERE d.draft_id=? AND d.draft_kind='OBDUMPER_EXPORT' AND d.owner_subject_id=? AND EXISTS(SELECT 1 FROM auth_subjects a WHERE a.subject_id=? AND a.account_status='ACTIVE')`, id, subjectID, subjectID).Scan(&actualRevision, &deleted)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDataSourceNotFound
		}
		if err != nil {
			return err
		}
		var previous string
		err = tx.QueryRowContext(ctx, `SELECT request_digest FROM request_idempotency WHERE subject_id=? AND operation='DELETE_EXPORT_DRAFT' AND idempotency_key=?`, subjectID, key).Scan(&previous)
		if err == nil {
			if previous != digest {
				return ErrIdempotencyConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if deleted.Valid {
			return ErrDataSourceNotFound
		}
		if actualRevision != revision {
			return ErrRevisionConflict
		}
		var used, leased bool
		// 缺失或无法解析的租约截止时间属于未知事实，不能当作已结束并允许删除。
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks t JOIN precheck_runs p ON p.precheck_id=t.precheck_id WHERE p.draft_id=?),EXISTS(SELECT 1 FROM precheck_runs p WHERE p.draft_id=? AND p.status='LEASED' AND (julianday(p.lease_expires_at) IS NULL OR julianday(p.lease_expires_at)>julianday(?)))`, id, id, utcText(now)).Scan(&used, &leased); err != nil {
			return err
		}
		if used || leased {
			return ErrExportDraftDeleteIneligible
		}
		if _, err := tx.ExecContext(ctx, `UPDATE precheck_runs SET status='INVALIDATED',completed_at=? WHERE draft_id=? AND status IN ('PENDING','LEASED','SUCCEEDED')`, utcText(now), id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE export_drafts SET deleted_at=?,updated_at=?,revision=revision+1 WHERE draft_id=? AND deleted_at IS NULL AND revision=?`, utcText(now), utcText(now), id, revision); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, "SUBJECT", subjectID, "EXPORT_DRAFT_DELETED", "EXPORT_DRAFT", id, "SUCCEEDED", requestID, now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES (?,'DELETE_EXPORT_DRAFT',?,?,200,'EXPORT_DRAFT',?,'{}',?,?)`, subjectID, key, digest, id, utcText(now), utcText(now.Add(24*time.Hour)))
		return err
	})
	return replayed, err
}
