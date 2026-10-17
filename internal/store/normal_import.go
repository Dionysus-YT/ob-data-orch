package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

// normalImportContext 从当前草稿与节点事实生成独立导入检查上下文，不能接受客户端运行事实。
func normalImportContext(ctx context.Context, tx *sql.Tx, binding PrecheckBinding, raw, mode, platform, rootsJSON string) (PrecheckExecutionContext, error) {
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT p.check_kind FROM precheck_runs p JOIN export_drafts d ON d.draft_id=p.draft_id WHERE p.precheck_id=? AND d.draft_kind='OBLOADER_NORMAL_IMPORT'`, binding.PrecheckID).Scan(&kind); err != nil {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	if len(precheckcontract.NormalImportChecks(kind)) == 0 {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	config, err := importdomain.Decode(raw)
	if err != nil {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	config, issues := importdomain.Normalize(config, mode, platform, kind == "NORMAL_IMPORT_PREFLIGHT")
	if len(issues) != 0 || config.Content == "" || config.FilePath == "" || config.HasData() && config.Format == "" {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	var roots []string
	if json.Unmarshal([]byte(rootsJSON), &roots) != nil || !ValidateExecutionNodeConfiguration(platform, roots) {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	requirement, err := readPrecheckResourceRequirement(ctx, tx, binding, raw)
	if err != nil {
		return PrecheckExecutionContext{}, err
	}
	return PrecheckExecutionContext{NormalImport: &config, CheckKind: kind, CompatibilityMode: mode, Database: config.Database, ContentKind: config.Content, TargetPlatform: platform, AllowedRoots: roots, ResourceRequirement: requirement}, nil
}

func validNormalImportManifest(config importdomain.Config, results []PrecheckCheckResult, manifest *importfiles.Manifest) bool {
	passed := false
	for _, result := range results {
		if result.Check == "INPUT_FILES" && result.Status == "PASSED" {
			passed = true
		}
	}
	return passed && manifest != nil && manifest.Valid(config) || !passed && manifest == nil
}

// NormalImportCheck 是所有者可见的导入检查事实，文件清单只属于输入证据。
type NormalImportCheck struct {
	Run           PrecheckRun
	CheckKind     string
	InputManifest *importfiles.Manifest
}

// NormalImportSnapshot 冻结独立配置与输入证据；不复制导出快照、结果文件或秘密。
type NormalImportSnapshot struct {
	ConfigVersion string `json:"configVersion"`
	TaskType      string `json:"taskType"`
	// TaskName 是提交时冻结的展示名称，不改变已预检查的领域配置。
	TaskName      string               `json:"taskName,omitempty"`
	Config        importdomain.Config  `json:"config"`
	InputManifest importfiles.Manifest `json:"inputManifest"`
}

// GetNormalImportCheck 在读取任何检查事实前限定普通导入类型及草稿所有者。
func (s *Store) GetNormalImportCheck(ctx context.Context, id, owner string) (NormalImportCheck, error) {
	var kind string
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT p.check_kind,p.input_manifest_json FROM precheck_runs p JOIN export_drafts d ON d.draft_id=p.draft_id WHERE p.precheck_id=? AND d.owner_subject_id=? AND d.draft_kind='OBLOADER_NORMAL_IMPORT' AND d.deleted_at IS NULL`, id, owner).Scan(&kind, &raw)
	if err != nil {
		return NormalImportCheck{}, ErrDataSourceNotFound
	}
	run, err := s.GetPrecheckRun(ctx, id)
	if err != nil {
		return NormalImportCheck{}, err
	}
	item := NormalImportCheck{Run: run, CheckKind: kind}
	if raw.Valid {
		var manifest importfiles.Manifest
		if json.Unmarshal([]byte(raw.String), &manifest) != nil {
			return NormalImportCheck{}, ErrPrecheckInvalid
		}
		item.InputManifest = &manifest
	}
	return item, nil
}

// CreateNormalImportPrecheck 使用平台租约持久化，但只接受两个固定导入用途。
func (s *Store) CreateNormalImportPrecheck(ctx context.Context, input PrecheckCreate) (PrecheckCreateResult, error) {
	if input.CheckKind != "NORMAL_IMPORT_FILES" && input.CheckKind != "NORMAL_IMPORT_PREFLIGHT" {
		return PrecheckCreateResult{}, ErrPrecheckInvalid
	}
	return s.createPrecheck(ctx, input)
}

// NormalImportDraft 是普通导入独立的非敏感持久化记录，不使用导出配置模型。
type NormalImportDraft struct {
	ID                string    `json:"id"`
	OwnerSubjectID    string    `json:"-"`
	DataSourceID      string    `json:"dataSourceId"`
	NodeID            string    `json:"nodeId"`
	Revision          int64     `json:"revision"`
	ConfigJSON        string    `json:"-"`
	ConfigFingerprint string    `json:"configFingerprint,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// NormalImportDraftWrite 绑定所有者、乐观锁、审计和创建幂等；不包含秘密明文。
type NormalImportDraftWrite struct {
	NormalImportDraft
	ExpectedRevision int64
	RequestID        string
	IdempotencyKey   string
	RequestDigest    string
}

// SaveNormalImportDraft 原子保存普通导入草稿并使旧检查失效；不会生成导出记录或执行任务。
func (s *Store) SaveNormalImportDraft(ctx context.Context, input NormalImportDraftWrite) (NormalImportDraft, error) {
	if input.ID == "" || input.OwnerSubjectID == "" || input.DataSourceID == "" || input.RequestID == "" || input.UpdatedAt.IsZero() || input.ExpectedRevision < 0 || !isSHA256(input.ConfigFingerprint) || !json.Valid([]byte(input.ConfigJSON)) {
		return NormalImportDraft{}, errors.New("普通导入草稿无效")
	}
	config, err := importdomain.Decode(input.ConfigJSON)
	if err != nil || config.DataSourceID != input.DataSourceID || config.NodeID != input.NodeID || validateSafeObjectJSON(input.ConfigJSON) != nil {
		return NormalImportDraft{}, errors.New("普通导入草稿无效")
	}
	result := input.NormalImportDraft
	err = s.withWrite(ctx, func(tx *sql.Tx) error {
		if input.ExpectedRevision == 0 {
			if len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 || !isSHA256(input.RequestDigest) {
				return ErrIdempotencyConflict
			}
			var digest, id string
			replayErr := tx.QueryRowContext(ctx, `SELECT request_digest,resource_id FROM request_idempotency WHERE subject_id=? AND operation='CREATE_NORMAL_IMPORT_DRAFT' AND idempotency_key=?`, input.OwnerSubjectID, input.IdempotencyKey).Scan(&digest, &id)
			if replayErr == nil {
				if digest != input.RequestDigest {
					return ErrIdempotencyConflict
				}
				result.ID = id
				return nil
			}
			if !errors.Is(replayErr, sql.ErrNoRows) {
				return replayErr
			}
		}
		var eligible int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_sources WHERE data_source_id=? AND state='ENABLED' AND connection_kind='ODP' AND last_test_status='SUCCEEDED'`, input.DataSourceID).Scan(&eligible); err != nil {
			return err
		}
		if eligible != 1 {
			return ErrDataSourceNotFound
		}
		if input.NodeID != "" {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM execution_nodes WHERE node_id=? AND management_state='ENABLED'`, input.NodeID).Scan(&eligible); err != nil {
				return err
			}
			if eligible != 1 {
				return ErrExecutionNodeNotFound
			}
		}
		if input.ExpectedRevision == 0 {
			_, err := tx.ExecContext(ctx, `INSERT INTO export_drafts(draft_id,owner_subject_id,data_source_id,node_id,revision,tool_version,metadata_version,capability_version,config_version,config_json,config_fingerprint,invalidation_json,created_at,updated_at,draft_kind) VALUES(?,?,?,NULLIF(?,''),1,?,?,?,?,?,NULLIF(?,''),'{}',?,?,'OBLOADER_NORMAL_IMPORT')`, input.ID, input.OwnerSubjectID, input.DataSourceID, input.NodeID, importdomain.ToolVersion, importdomain.MetadataVersion, importdomain.CapabilityVersion, importdomain.ConfigVersion, input.ConfigJSON, input.ConfigFingerprint, utcText(input.UpdatedAt), utcText(input.UpdatedAt))
			if err != nil {
				return err
			}
			result.Revision = 1
			result.CreatedAt = input.UpdatedAt
			if _, err := tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES(?,'CREATE_NORMAL_IMPORT_DRAFT',?,?,201,'NORMAL_IMPORT_DRAFT',?,'{}',?,?)`, input.OwnerSubjectID, input.IdempotencyKey, input.RequestDigest, input.ID, utcText(input.UpdatedAt), utcText(input.UpdatedAt.Add(24*time.Hour))); err != nil {
				return err
			}
		} else {
			res, err := tx.ExecContext(ctx, `UPDATE export_drafts SET data_source_id=?,node_id=NULLIF(?,''),revision=revision+1,config_json=?,config_fingerprint=NULLIF(?,''),invalidation_json='{"precheck":true}',updated_at=? WHERE draft_id=? AND owner_subject_id=? AND revision=? AND draft_kind='OBLOADER_NORMAL_IMPORT' AND deleted_at IS NULL`, input.DataSourceID, input.NodeID, input.ConfigJSON, input.ConfigFingerprint, utcText(input.UpdatedAt), input.ID, input.OwnerSubjectID, input.ExpectedRevision)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n != 1 {
				return ErrRevisionConflict
			}
			if _, err := tx.ExecContext(ctx, `UPDATE precheck_runs SET status='INVALIDATED' WHERE draft_id=? AND status IN ('PENDING','LEASED','SUCCEEDED')`, input.ID); err != nil {
				return err
			}
			result.Revision = input.ExpectedRevision + 1
		}
		return insertAudit(ctx, tx, "SUBJECT", input.OwnerSubjectID, "NORMAL_IMPORT_DRAFT_SAVED", "NORMAL_IMPORT_DRAFT", result.ID, "SUCCEEDED", input.RequestID, input.UpdatedAt)
	})
	if err != nil {
		return NormalImportDraft{}, err
	}
	return s.GetNormalImportDraft(ctx, result.ID, input.OwnerSubjectID)
}

// GetNormalImportDraft 在同一查询中限定所有者与类型，无权对象保持不存在语义。
func (s *Store) GetNormalImportDraft(ctx context.Context, id, owner string) (NormalImportDraft, error) {
	var item NormalImportDraft
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT draft_id,owner_subject_id,data_source_id,COALESCE(node_id,''),revision,config_json,COALESCE(config_fingerprint,''),created_at,updated_at FROM export_drafts WHERE draft_id=? AND owner_subject_id=? AND draft_kind='OBLOADER_NORMAL_IMPORT' AND deleted_at IS NULL`, id, owner).Scan(&item.ID, &item.OwnerSubjectID, &item.DataSourceID, &item.NodeID, &item.Revision, &item.ConfigJSON, &item.ConfigFingerprint, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return NormalImportDraft{}, ErrDataSourceNotFound
	}
	if err != nil {
		return NormalImportDraft{}, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return item, nil
}

// ListNormalImportDrafts 只返回当前所有者的受限安全摘要，不泄露配置、路径或凭据。
func (s *Store) ListNormalImportDrafts(ctx context.Context, owner string) ([]NormalImportDraft, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.draft_id,d.data_source_id,COALESCE(d.node_id,''),d.revision,d.created_at,d.updated_at FROM export_drafts d WHERE d.owner_subject_id=? AND d.draft_kind='OBLOADER_NORMAL_IMPORT' AND d.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM tasks t JOIN precheck_runs p ON p.precheck_id=t.precheck_id WHERE p.draft_id=d.draft_id) ORDER BY d.updated_at DESC,d.draft_id DESC LIMIT 100`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []NormalImportDraft{}
	for rows.Next() {
		var item NormalImportDraft
		var created, updated string
		if err := rows.Scan(&item.ID, &item.DataSourceID, &item.NodeID, &item.Revision, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

// DeleteNormalImportDraft 保留历史检查与审计引用，只对当前所有者的未提交、无活动租约记录作逻辑删除。
func (s *Store) DeleteNormalImportDraft(ctx context.Context, id, owner string, revision int64, requestID string, now time.Time) error {
	if !validAgentOpaqueValue(id, 256) || !validAgentOpaqueValue(owner, 256) || revision < 1 || requestID == "" || now.IsZero() {
		return ErrRevisionConflict
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE export_drafts SET deleted_at=?,revision=revision+1,updated_at=? WHERE draft_id=? AND owner_subject_id=? AND revision=? AND draft_kind='OBLOADER_NORMAL_IMPORT' AND deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM precheck_runs p JOIN tasks t ON t.precheck_id=p.precheck_id WHERE p.draft_id=export_drafts.draft_id) AND NOT EXISTS(SELECT 1 FROM precheck_runs p WHERE p.draft_id=export_drafts.draft_id AND p.status='LEASED' AND p.lease_expires_at>?)`, utcText(now), utcText(now), id, owner, revision, utcText(now))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrRevisionConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE precheck_runs SET status='INVALIDATED' WHERE draft_id=? AND status IN ('PENDING','SUCCEEDED')`, id)
		if err != nil {
			return err
		}
		return insertAudit(ctx, tx, "SUBJECT", owner, "NORMAL_IMPORT_DRAFT_DELETED", "NORMAL_IMPORT_DRAFT", id, "SUCCEEDED", requestID, now)
	})
}
