package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"ob-data-orch/internal/accesscontrol"
)

const managedPermissionExists = `EXISTS(SELECT 1 FROM subject_permission_versions WHERE deployment_owner=1)`

// taskPermissionEligibility 只接收源码中的固定任务别名；所有对象仍来自绑定任务字段。
// 未引导的隔离旧仓储夹具保留旧契约，正式组合根必须先完成可信权限引导。
func taskPermissionEligibility(alias string) string {
	return strings.ReplaceAll(`(NOT `+managedPermissionExists+` OR (
        EXISTS(SELECT 1 FROM auth_subjects pa WHERE pa.subject_id=$T.creator_subject_id AND pa.account_status='ACTIVE')
        AND EXISTS(SELECT 1 FROM subject_grants pg WHERE pg.subject_id=$T.creator_subject_id AND pg.grant_code='ROLE_BASIC_USER')
        AND EXISTS(SELECT 1 FROM data_sources pd WHERE pd.data_source_id=$T.data_source_id AND pd.state='ENABLED')
        AND EXISTS(SELECT 1 FROM execution_nodes pn WHERE pn.node_id=$T.node_id AND pn.management_state='ENABLED')
        AND EXISTS(SELECT 1 FROM subject_object_scopes ps WHERE ps.subject_id=$T.creator_subject_id AND ps.scope_type='DATA_SOURCE_USE' AND ps.object_id=$T.data_source_id)
        AND EXISTS(SELECT 1 FROM subject_object_scopes ps WHERE ps.subject_id=$T.creator_subject_id AND ps.scope_type='NODE_USE' AND ps.object_id=$T.node_id)
        AND (($T.task_type != 'OBLOADER_DIRECT_LOAD' AND COALESCE(json_extract($T.submitted_permission_scope_json,'$.expertConfig'),CASE WHEN $T.metadata_version IN ('obdumper-4.3.5-slice-v11-expert-parameters','obdumper-4.3.5-slice-v12-schema-boolean') OR $T.task_type='OBLOADER_NORMAL_IMPORT' THEN 1 ELSE 0 END) != 1) OR EXISTS(SELECT 1 FROM subject_grants pg WHERE pg.subject_id=$T.creator_subject_id AND pg.grant_code='CAP_EXPERT_CONFIG'))
        AND (NOT EXISTS(SELECT 1 FROM data_sources pd WHERE pd.data_source_id=$T.data_source_id AND pd.environment IN ('PROD','PRODUCTION')) OR EXISTS(SELECT 1 FROM subject_grants pg WHERE pg.subject_id=$T.creator_subject_id AND pg.grant_code='CAP_PRODUCTION_TASK'))
    ))`, "$T", alias)
}

// managedTaskOwnerRead 避免撤销基础角色后仍凭历史 creator 字段读取任务。
func managedTaskOwnerRead(alias string) string {
	return `(NOT ` + managedPermissionExists + ` OR EXISTS(SELECT 1 FROM subject_grants pr WHERE pr.subject_id=` + alias + `.creator_subject_id AND pr.grant_code='ROLE_BASIC_USER'))`
}

// managedTaskOperatorRead 在已有数据源任务范围内复验运维角色，防止遗留范围单独生效。
func managedTaskOperatorRead(scopeAlias string) string {
	return `(NOT ` + managedPermissionExists + ` OR EXISTS(SELECT 1 FROM subject_grants pr WHERE pr.subject_id=` + scopeAlias + `.subject_id AND pr.grant_code='ROLE_OPERATOR'))`
}

// managedGlobalTaskRead 把调用方的全局读取提示与同次查询的当前部署身份/角色绑定，防止撤权竞态。
// 返回的固定 SQL 恰好消费一个当前主体绑定参数，旧隔离夹具没有引导标记时保持既有契约。
func managedGlobalTaskRead() string {
	return `(NOT ` + managedPermissionExists + ` OR EXISTS(SELECT 1 FROM subject_permission_versions pv JOIN auth_subjects pa ON pa.subject_id=pv.subject_id WHERE pv.subject_id=? AND pv.deployment_owner=1 AND pa.account_status='ACTIVE'
        AND EXISTS(SELECT 1 FROM subject_grants pg WHERE pg.subject_id=pv.subject_id AND pg.grant_code='ROLE_SYSTEM_ADMIN')
        AND EXISTS(SELECT 1 FROM subject_grants pg WHERE pg.subject_id=pv.subject_id AND pg.grant_code='ROLE_OPERATOR')))`
}

// taskSubmissionPermissions 在插入不可变任务之前冻结当前授权，不对已插入任务执行 UPDATE。
func taskSubmissionPermissions(ctx context.Context, tx *sql.Tx, subject, source, node string, expertRequired bool) (any, any, error) {
	var managed bool
	if err := tx.QueryRowContext(ctx, "SELECT "+managedPermissionExists).Scan(&managed); err != nil {
		return nil, nil, err
	}
	if !managed {
		return nil, nil, nil
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT `+taskPermissionEligibility("t")+` FROM (SELECT ? AS creator_subject_id,? AS data_source_id,? AS node_id,NULL AS submitted_permission_scope_json,'' AS metadata_version,'' AS task_type) t`, subject, source, node).Scan(&allowed); err != nil {
		return nil, nil, err
	}
	if !allowed {
		return nil, nil, ErrClaimIneligible
	}
	profile, err := permissionProfile(ctx, tx, subject)
	if err != nil || profile.Subject.AccountStatus != "ACTIVE" || expertRequired && !profile.Assignment.HasCapability(accesscontrol.ExpertConfig) {
		return nil, nil, ErrClaimIneligible
	}
	encoded, err := json.Marshal(map[string]any{"dataSourceId": source, "nodeId": node, "taskCreation": true, "productionTask": profile.Assignment.HasCapability(accesscontrol.ProductionTask), "expertConfig": expertRequired})
	if err != nil {
		return nil, nil, err
	}
	return profile.Revision, string(encoded), nil
}

func submissionNeedsExpert(input TaskSubmission) bool {
	if input.TaskType == "OBLOADER_DIRECT_LOAD" {
		return true
	}
	if input.TaskType == "OBLOADER_NORMAL_IMPORT" {
		snapshot, ok := normalImportSnapshot(input.SnapshotJSON)
		return ok && snapshot.Config.HighRisk()
	}
	if input.ExpertConfigRequired != nil {
		return *input.ExpertConfigRequired
	}
	return input.MetadataVersion == "obdumper-4.3.5-slice-v11-expert-parameters" || input.MetadataVersion == "obdumper-4.3.5-slice-v12-schema-boolean"
}

// resumedTaskNeedsExpert 优先沿用来源任务已冻结的领域分类，避免仅凭版本把普通配置提升为专家。
// 历史没有分类证据时保留收缩判断，不回写旧任务。
func resumedTaskNeedsExpert(ctx context.Context, tx *sql.Tx, source TaskDerivationSource) (bool, error) {
	var raw sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT submitted_permission_scope_json FROM tasks WHERE task_id=?`, source.TaskID).Scan(&raw); err != nil {
		return false, err
	}
	input := TaskSubmission{TaskType: source.TaskType, MetadataVersion: source.MetadataVersion, SnapshotJSON: source.SnapshotJSON}
	if raw.Valid {
		var frozen struct {
			Expert *bool `json:"expertConfig"`
		}
		if json.Unmarshal([]byte(raw.String), &frozen) != nil {
			return false, ErrCheckpointResumeUnavailable
		}
		input.ExpertConfigRequired = frozen.Expert
	}
	return submissionNeedsExpert(input), nil
}

// grantCreatedObjectScopes 将创建职责明确落为新对象范围，不重新授予旧对象或任何角色。
func grantCreatedObjectScopes(ctx context.Context, tx *sql.Tx, subjectID, kind, objectID string, at time.Time) error {
	var managed bool
	if err := tx.QueryRowContext(ctx, "SELECT "+managedPermissionExists).Scan(&managed); err != nil {
		return err
	}
	if !managed {
		return nil
	}
	profile, err := permissionProfile(ctx, tx, subjectID)
	if err != nil || profile.Subject.AccountStatus != "ACTIVE" {
		return ErrPermissionDenied
	}
	types := []accesscontrol.ScopeType{}
	switch kind {
	case "DATA_SOURCE":
		if !profile.Assignment.HasRole(accesscontrol.DataSourceAdmin) {
			return ErrPermissionDenied
		}
		types = append(types, accesscontrol.DataSourceManage)
		if profile.Assignment.HasRole(accesscontrol.BasicUser) {
			var environment string
			if err := tx.QueryRowContext(ctx, `SELECT environment FROM data_sources WHERE data_source_id=?`, objectID).Scan(&environment); err != nil {
				return err
			}
			if environment != "PROD" && environment != "PRODUCTION" || profile.Assignment.HasCapability(accesscontrol.ProductionTask) {
				types = append(types, accesscontrol.DataSourceUse)
			}
		}
	case "NODE":
		if !profile.Assignment.HasRole(accesscontrol.NodeAdmin) {
			return ErrPermissionDenied
		}
		types = append(types, accesscontrol.NodeManage)
		if profile.Assignment.HasRole(accesscontrol.BasicUser) || profile.Assignment.HasRole(accesscontrol.DataSourceAdmin) {
			types = append(types, accesscontrol.NodeUse)
		}
	case "TEMPLATE":
		if !profile.InitialAdministrator || !profile.Assignment.CanCreateTask() || !profile.Assignment.HasRole(accesscontrol.Operator) {
			return ErrPermissionDenied
		}
		types = append(types, accesscontrol.TemplateManage)
		if profile.Assignment.HasRole(accesscontrol.BasicUser) {
			types = append(types, accesscontrol.TemplateUse)
		}
	default:
		return accesscontrol.ErrInvalid
	}
	if len(profile.Assignment.Scopes)+len(types) > 200 {
		return accesscontrol.ErrInvalid
	}
	if profile.Revision >= 9007199254740991 {
		return accesscontrol.ErrInvalid
	}
	for _, scope := range types {
		if _, err := tx.ExecContext(ctx, `INSERT INTO subject_object_scopes(subject_id,scope_type,object_id,granted_by,granted_at) VALUES(?,?,?,?,?)`, subjectID, scope, objectID, subjectID, utcText(at)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_permission_versions(subject_id,revision,modified_by,modified_at) VALUES(?,?,?,?) ON CONFLICT(subject_id) DO UPDATE SET revision=subject_permission_versions.revision+1,modified_by=excluded.modified_by,modified_at=excluded.modified_at`, subjectID, profile.Revision+1, subjectID, utcText(at)); err != nil {
		return err
	}
	return insertAudit(ctx, tx, "SUBJECT", subjectID, "PERMISSION_OBJECT_BOUND", kind, objectID, "SUCCEEDED", "permission-created-"+kind+"-"+objectID, at)
}
