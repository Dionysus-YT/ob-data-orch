package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

// DirectLoadDraft 保存独立旁路活动配置；不持有普通导入参数或任何秘密。
type DirectLoadDraft struct {
	ID                string    `json:"id"`
	OwnerSubjectID    string    `json:"-"`
	DataSourceID      string    `json:"dataSourceId"`
	NodeID            string    `json:"nodeId,omitempty"`
	Revision          int64     `json:"revision"`
	ConfigJSON        string    `json:"-"`
	ConfigFingerprint string    `json:"configFingerprint"`
	SourceTaskID      string    `json:"sourceTaskId,omitempty"`
	SourceDerivation  string    `json:"sourceDerivation,omitempty"`
	ParametersLocked  bool      `json:"parametersLocked"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	Replayed          bool      `json:"-"`
}

// DirectLoadDraftWrite 绑定所有者、修订、幂等键与审计请求。
type DirectLoadDraftWrite struct {
	DirectLoadDraft
	ExpectedRevision int64
	RequestID        string
	IdempotencyKey   string
	RequestDigest    string
}

// DirectLoadCheck 只投影当前所有者的独立旁路预检查及输入清单。
type DirectLoadCheck struct {
	Run           PrecheckRun
	CheckKind     string
	InputManifest *importfiles.Manifest
	// SubmittedTaskID 只用于所有者核对已提交结果，不能赋予新的执行资格。
	SubmittedTaskID string
	// SubmittedTaskName 仅供服务端比较重放请求，不投影到预检查响应。
	SubmittedTaskName string
	// SubmittedIdempotencyKey 仅在原键有效期内识别原请求；不得向浏览器回传。
	SubmittedIdempotencyKey string
}

func validDirectLoadConfigFingerprint(kind string, results []PrecheckCheckResult, fingerprint string) bool {
	for _, result := range results {
		if kind == "DIRECT_LOAD_PREFLIGHT" && result.Check == "DIRECT_LOAD_CONFIG" && result.Status == "PASSED" {
			return isSHA256(fingerprint)
		}
	}
	return fingerprint == ""
}

// SaveDirectLoadDraft 原子保存旁路草稿并使原检查失效，不创建任务或执行工具。
func (s *Store) SaveDirectLoadDraft(ctx context.Context, input DirectLoadDraftWrite) (DirectLoadDraft, error) {
	config, err := directload.Decode(input.ConfigJSON)
	if err != nil || input.ID == "" || input.OwnerSubjectID == "" || input.DataSourceID == "" || input.RequestID == "" || input.UpdatedAt.IsZero() || input.ExpectedRevision < 0 || !isSHA256(input.ConfigFingerprint) || config.DataSourceID != input.DataSourceID || config.NodeID != input.NodeID || validateSafeObjectJSON(input.ConfigJSON) != nil {
		return DirectLoadDraft{}, errors.New("旁路导入草稿无效")
	}
	if (input.SourceTaskID == "") != (input.SourceDerivation == "") || input.SourceTaskID != "" && !directload.CanDeriveTaskDraft("FAILED", false, input.SourceDerivation) {
		return DirectLoadDraft{}, ErrPrecheckInvalid
	}
	id, replayed := input.ID, false
	err = s.withWrite(ctx, func(tx *sql.Tx) error {
		if input.ExpectedRevision == 0 {
			if len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 || !isSHA256(input.RequestDigest) {
				return ErrIdempotencyConflict
			}
			var digest, existing string
			err := tx.QueryRowContext(ctx, `SELECT request_digest,resource_id FROM request_idempotency WHERE subject_id=? AND operation='CREATE_DIRECT_LOAD_DRAFT' AND idempotency_key=?`, input.OwnerSubjectID, input.IdempotencyKey).Scan(&digest, &existing)
			if err == nil {
				if digest != input.RequestDigest {
					return ErrIdempotencyConflict
				}
				id, replayed = existing, true
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var source DataSourceSummary
		if err := tx.QueryRowContext(ctx, `SELECT connection_kind,compatibility_mode,state,COALESCE(last_test_status,'') FROM data_sources WHERE data_source_id=?`, input.DataSourceID).Scan(&source.ConnectionKind, &source.CompatibilityMode, &source.State, &source.LastTestStatus); err != nil || !DirectLoadSourceMatches(config.Scenario, source) {
			return ErrDataSourceNotFound
		}
		platform := ""
		if input.NodeID != "" {
			if tx.QueryRowContext(ctx, `SELECT platform FROM execution_nodes WHERE node_id=? AND management_state='ENABLED'`, input.NodeID).Scan(&platform) != nil {
				return ErrExecutionNodeNotFound
			}
		}
		// 草稿持久层独立复核活动投影；不能靠内部调用绕过 HTTP 参数边界或改变指纹对应内容。
		active, issues := directload.Normalize(config, source.CompatibilityMode, platform, false)
		before, _ := json.Marshal(config)
		after, _ := json.Marshal(active)
		if len(issues) != 0 || string(before) != string(after) {
			return ErrPrecheckInvalid
		}
		if input.ExpectedRevision == 0 {
			if input.SourceTaskID != "" {
				source, err := readTaskDerivationSourceTx(ctx, tx, input.SourceTaskID, input.OwnerSubjectID)
				if err != nil {
					return err
				}
				expected, err := DirectLoadReusableConfig(source)
				want, _ := json.Marshal(expected)
				if err != nil || !directload.CanDeriveTaskDraft(source.State, source.ReconciliationRequired, input.SourceDerivation) || string(want) != string(before) {
					return ErrPrecheckInvalid
				}
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO export_drafts(draft_id,owner_subject_id,data_source_id,node_id,revision,tool_version,metadata_version,capability_version,config_version,config_json,config_fingerprint,invalidation_json,created_at,updated_at,draft_kind,source_task_id,source_derivation) VALUES(?,?,?,NULLIF(?,''),1,?,?,?,?,?,?,'{}',?,?,'OBLOADER_DIRECT_LOAD',NULLIF(?,''),NULLIF(?,''))`, input.ID, input.OwnerSubjectID, input.DataSourceID, input.NodeID, directload.ToolVersion, directload.MetadataVersion, directload.CapabilityVersion, directload.ConfigVersion, input.ConfigJSON, input.ConfigFingerprint, utcText(input.UpdatedAt), utcText(input.UpdatedAt), input.SourceTaskID, input.SourceDerivation)
			if err != nil {
				return err
			}
			if input.SourceTaskID != "" {
				if err = insertAudit(ctx, tx, "SUBJECT", input.OwnerSubjectID, "TASK_DRAFT_DERIVED", "TASK", input.SourceTaskID, "SUCCEEDED", input.RequestID, input.UpdatedAt); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id,operation,idempotency_key,request_digest,result_status,resource_kind,resource_id,response_json,created_at,expires_at) VALUES(?,'CREATE_DIRECT_LOAD_DRAFT',?,?,201,'DIRECT_LOAD_DRAFT',?,'{}',?,?)`, input.OwnerSubjectID, input.IdempotencyKey, input.RequestDigest, input.ID, utcText(input.UpdatedAt), utcText(input.UpdatedAt.Add(24*time.Hour))); err != nil {
				return err
			}
		} else {
			var previousRaw, derivation string
			if tx.QueryRowContext(ctx, `SELECT config_json,COALESCE(source_derivation,'') FROM export_drafts WHERE draft_id=? AND owner_subject_id=? AND draft_kind='OBLOADER_DIRECT_LOAD' AND deleted_at IS NULL`, input.ID, input.OwnerSubjectID).Scan(&previousRaw, &derivation) != nil {
				return ErrDataSourceNotFound
			}
			// 从头执行允许对同一活动配置重绑当前事实，不能借 PATCH 修改任何配置值。
			if derivation == "RERUN_FROM_SCRATCH" {
				previous, err := directload.Decode(previousRaw)
				previousJSON, _ := json.Marshal(previous)
				if err != nil || string(previousJSON) != string(before) {
					return ErrPrecheckInvalid
				}
			}
			// 已提交草稿及其预检查属于冻结证据；编辑不能使历史任务依赖的证明失效。
			result, err := tx.ExecContext(ctx, `UPDATE export_drafts SET data_source_id=?,node_id=NULLIF(?,''),revision=revision+1,config_json=?,config_fingerprint=?,invalidation_json='{"precheck":true}',updated_at=? WHERE draft_id=? AND owner_subject_id=? AND revision=? AND draft_kind='OBLOADER_DIRECT_LOAD' AND deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM precheck_runs p JOIN tasks t ON t.precheck_id=p.precheck_id WHERE p.draft_id=export_drafts.draft_id)`, input.DataSourceID, input.NodeID, input.ConfigJSON, input.ConfigFingerprint, utcText(input.UpdatedAt), input.ID, input.OwnerSubjectID, input.ExpectedRevision)
			if err != nil {
				return err
			}
			count, _ := result.RowsAffected()
			if count != 1 {
				return ErrRevisionConflict
			}
			if _, err = tx.ExecContext(ctx, `UPDATE precheck_runs SET status='INVALIDATED' WHERE draft_id=? AND status IN ('PENDING','LEASED','SUCCEEDED')`, input.ID); err != nil {
				return err
			}
		}
		return insertAudit(ctx, tx, "SUBJECT", input.OwnerSubjectID, "DIRECT_LOAD_DRAFT_SAVED", "DIRECT_LOAD_DRAFT", id, "SUCCEEDED", input.RequestID, input.UpdatedAt)
	})
	if err != nil {
		return DirectLoadDraft{}, err
	}
	item, err := s.GetDirectLoadDraft(ctx, id, input.OwnerSubjectID)
	item.Replayed = replayed
	return item, err
}

// DirectLoadSourceMatches 根据数据源的唯一连接类型校验显式旁路场景，不能以手工场景覆盖源事实。
func DirectLoadSourceMatches(scenario string, source DataSourceSummary) bool {
	if source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" {
		return false
	}
	return scenario == "" && oneOf(source.ConnectionKind, "ODP", "PUBLIC_CLOUD", "OBSERVER_DIRECT") || scenario == "PRIVATE_ODP" && source.ConnectionKind == "ODP" || scenario == "CLOUD_ODP" && source.ConnectionKind == "PUBLIC_CLOUD" || scenario == "OBSERVER_DIRECT" && source.ConnectionKind == "OBSERVER_DIRECT"
}

// GetDirectLoadDraft 在同一查询限定所有者及旁路类型，无权对象保持不存在语义。
func (s *Store) GetDirectLoadDraft(ctx context.Context, id, owner string) (DirectLoadDraft, error) {
	var item DirectLoadDraft
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT draft_id,owner_subject_id,data_source_id,COALESCE(node_id,''),revision,config_json,config_fingerprint,created_at,updated_at,COALESCE(source_task_id,''),COALESCE(source_derivation,'') FROM export_drafts WHERE draft_id=? AND owner_subject_id=? AND draft_kind='OBLOADER_DIRECT_LOAD' AND deleted_at IS NULL`, id, owner).Scan(&item.ID, &item.OwnerSubjectID, &item.DataSourceID, &item.NodeID, &item.Revision, &item.ConfigJSON, &item.ConfigFingerprint, &created, &updated, &item.SourceTaskID, &item.SourceDerivation)
	if errors.Is(err, sql.ErrNoRows) {
		return DirectLoadDraft{}, ErrDataSourceNotFound
	}
	if err != nil {
		return DirectLoadDraft{}, err
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return DirectLoadDraft{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	item.ParametersLocked = item.SourceDerivation == "RERUN_FROM_SCRATCH"
	return item, err
}

// ListDirectLoadDrafts 只返回所有者的安全摘要，不披露配置、路径或凭据。
func (s *Store) ListDirectLoadDrafts(ctx context.Context, owner string) ([]DirectLoadDraft, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT draft_id,data_source_id,COALESCE(node_id,''),revision,config_fingerprint,created_at,updated_at,COALESCE(source_task_id,''),COALESCE(source_derivation,'') FROM export_drafts WHERE owner_subject_id=? AND draft_kind='OBLOADER_DIRECT_LOAD' AND deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM precheck_runs p JOIN tasks t ON t.precheck_id=p.precheck_id WHERE p.draft_id=export_drafts.draft_id) ORDER BY updated_at DESC,draft_id LIMIT 100`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []DirectLoadDraft{}
	for rows.Next() {
		var item DirectLoadDraft
		var created, updated string
		if err = rows.Scan(&item.ID, &item.DataSourceID, &item.NodeID, &item.Revision, &item.ConfigFingerprint, &created, &updated, &item.SourceTaskID, &item.SourceDerivation); err != nil {
			return nil, err
		}
		item.OwnerSubjectID = owner
		item.ParametersLocked = item.SourceDerivation == "RERUN_FROM_SCRATCH"
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// DeleteDirectLoadDraft 只逻辑删除未提交且无活动检查的所有者草稿，保留历史证据。
func (s *Store) DeleteDirectLoadDraft(ctx context.Context, id, owner string, revision int64, requestID string, now time.Time) error {
	if revision < 1 || requestID == "" || now.IsZero() {
		return ErrRevisionConflict
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE export_drafts SET deleted_at=?,revision=revision+1,updated_at=? WHERE draft_id=? AND owner_subject_id=? AND revision=? AND draft_kind='OBLOADER_DIRECT_LOAD' AND deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM precheck_runs p JOIN tasks t ON t.precheck_id=p.precheck_id WHERE p.draft_id=export_drafts.draft_id) AND NOT EXISTS(SELECT 1 FROM precheck_runs p WHERE p.draft_id=export_drafts.draft_id AND p.status='LEASED' AND (julianday(p.lease_expires_at) IS NULL OR julianday(p.lease_expires_at)>julianday(?)))`, utcText(now), utcText(now), id, owner, revision, utcText(now))
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return ErrRevisionConflict
		}
		return insertAudit(ctx, tx, "SUBJECT", owner, "DIRECT_LOAD_DRAFT_DELETED", "DIRECT_LOAD_DRAFT", id, "SUCCEEDED", requestID, now)
	})
}

// CreateDirectLoadPrecheck 只接受两个旁路固定用途，不能借用普通导入预检查。
func (s *Store) CreateDirectLoadPrecheck(ctx context.Context, input PrecheckCreate) (PrecheckCreateResult, error) {
	if len(precheckcontract.DirectLoadChecks(input.CheckKind)) == 0 {
		return PrecheckCreateResult{}, ErrPrecheckInvalid
	}
	return s.createPrecheck(ctx, input)
}

// GetDirectLoadCheck 在任何事实读取前限定草稿所有者与旁路类型。
func (s *Store) GetDirectLoadCheck(ctx context.Context, id, owner string) (DirectLoadCheck, error) {
	var kind string
	var raw sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT p.check_kind,p.input_manifest_json FROM precheck_runs p JOIN export_drafts d ON d.draft_id=p.draft_id WHERE p.precheck_id=? AND d.owner_subject_id=? AND d.draft_kind='OBLOADER_DIRECT_LOAD' AND d.deleted_at IS NULL`, id, owner).Scan(&kind, &raw); err != nil {
		return DirectLoadCheck{}, ErrDataSourceNotFound
	}
	run, err := s.GetPrecheckRun(ctx, id)
	if err != nil {
		return DirectLoadCheck{}, err
	}
	item := DirectLoadCheck{Run: run, CheckKind: kind}
	rows, err := s.db.QueryContext(ctx, `SELECT t.task_id,COALESCE(json_extract(t.snapshot_json,'$.taskName'),''),COALESCE((SELECT i.idempotency_key FROM request_idempotency i WHERE i.resource_id=t.task_id AND i.resource_kind='TASK' AND i.subject_id=t.creator_subject_id AND i.operation='SUBMIT_DIRECT_LOAD_DRAFT' ORDER BY i.created_at LIMIT 1),'') FROM tasks t WHERE t.precheck_id=? AND t.creator_subject_id=? AND t.task_type='OBLOADER_DIRECT_LOAD' ORDER BY t.submitted_at,t.task_id LIMIT 2`, id, owner)
	if err != nil {
		return DirectLoadCheck{}, err
	}
	defer rows.Close()
	if rows.Next() {
		if rows.Scan(&item.SubmittedTaskID, &item.SubmittedTaskName, &item.SubmittedIdempotencyKey) != nil || rows.Next() {
			return DirectLoadCheck{}, ErrPrecheckInvalid
		}
	}
	if rows.Err() != nil {
		return DirectLoadCheck{}, ErrPrecheckInvalid
	}
	if raw.Valid {
		var manifest importfiles.Manifest
		if json.Unmarshal([]byte(raw.String), &manifest) != nil {
			return DirectLoadCheck{}, ErrPrecheckInvalid
		}
		item.InputManifest = &manifest
	}
	return item, nil
}

// directLoadContext 从冻结草稿与当前节点事实生成固定检查输入，不能接受客户端事实。
func directLoadContext(ctx context.Context, tx *sql.Tx, binding PrecheckBinding, raw, mode, platform, rootsJSON string) (PrecheckExecutionContext, error) {
	var kind string
	if tx.QueryRowContext(ctx, `SELECT p.check_kind FROM precheck_runs p JOIN export_drafts d ON d.draft_id=p.draft_id WHERE p.precheck_id=? AND d.draft_kind='OBLOADER_DIRECT_LOAD'`, binding.PrecheckID).Scan(&kind) != nil || len(precheckcontract.DirectLoadChecks(kind)) == 0 {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	c, err := directload.Decode(raw)
	if err != nil {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	c, issues := directload.Normalize(c, mode, platform, kind == "DIRECT_LOAD_PREFLIGHT")
	if len(issues) != 0 || c.DataSourceID == "" || c.NodeID == "" || c.Database == "" || c.Scenario == "" || c.FilePath == "" || c.Table == "" || c.Format == "" {
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
	return PrecheckExecutionContext{DirectLoad: &c, CheckKind: kind, CompatibilityMode: mode, Database: c.Database, ContentKind: "DATA_ONLY", TargetPlatform: platform, AllowedRoots: roots, ResourceRequirement: requirement}, nil
}

func matchesDirectLoadShape(results []PrecheckCheckResult, kind string) bool {
	expected := precheckcontract.DirectLoadChecks(kind)
	if len(expected) == 0 || len(expected) != len(results) {
		return false
	}
	for index, check := range expected {
		result := results[index]
		if result.Check != check || !precheckcontract.ValidDirectLoadResult(check, result.Status, result.EvidenceCode) {
			return false
		}
	}
	return true
}

func validDirectLoadManifest(c directload.Config, results []PrecheckCheckResult, manifest *importfiles.Manifest) bool {
	passed := false
	for _, result := range results {
		if result.Check == "INPUT_FILES" && result.Status == "PASSED" {
			passed = true
		}
	}
	return passed && manifest != nil && directloadfiles.Valid(*manifest, c) || !passed && manifest == nil
}
