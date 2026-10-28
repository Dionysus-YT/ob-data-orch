package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/overview"
)

func submitPermissionTask(t *testing.T, database *Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := database.ChangeSubjectPermissions(ctx, permissionChange()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`UPDATE export_drafts SET owner_subject_id='permission-user' WHERE draft_id='draft-1'`); err != nil {
		t.Fatal(err)
	}
	input := validTaskSubmission("task-1")
	input.CreatorSubjectID, input.AuditActorID = "permission-user", "permission-user"
	if err := database.SubmitTask(ctx, input); err != nil {
		t.Fatal(err)
	}
}

func Test权限提交冻结及待领取任务撤权(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	submitPermissionTask(t, database)
	var revision int64
	var snapshot string
	if err := database.db.QueryRow(`SELECT submitted_permission_revision,submitted_permission_scope_json FROM tasks WHERE task_id='task-1'`).Scan(&revision, &snapshot); err != nil || revision != 2 || !strings.Contains(snapshot, `"dataSourceId":"source-1"`) || strings.Contains(snapshot, "secret") {
		t.Fatal("权限冻结事实错误", revision, snapshot, err)
	}
	if _, err := database.GetAuthorizedTaskSummary(ctx, "task-1", "permission-user"); err != nil {
		t.Fatal(err)
	}
	revoke := permissionChange()
	revoke.ExpectedRevision = 2
	revoke.IdempotencyKey = "revoke-waiting"
	revoke.RequestID = "request-revoke-waiting"
	revoke.Assignment = accesscontrol.Assignment{}
	if _, err := database.ChangeSubjectPermissions(ctx, revoke); err != nil {
		t.Fatal(err)
	}
	if err := database.ClaimTask(ctx, validClaim("execution-1", "lease-1", "event-1", "claim-after-revoke")); !errors.Is(err, ErrClaimIneligible) {
		t.Fatal("撤权任务仍可领取", err)
	}
	if _, err := database.GetAuthorizedTaskSummary(ctx, "task-1", "permission-user"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("撤角色后仍凭 creator 字段读取", err)
	}
	if _, err := database.GetAuthorizedTaskSnapshotGroups(ctx, "task-1", "permission-user", false); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("快照绕过角色撤销", err)
	}
	items, err := database.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "permission-user", Limit: 10})
	if err != nil || len(items) != 0 {
		t.Fatal("任务列表保留撤权数据", items, err)
	}
	items, err = database.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "permission-user", GlobalRead: true, Limit: 10})
	if err != nil || len(items) != 0 {
		t.Fatal("历史全局提示绕过当前角色", items, err)
	}
	if _, err := database.readTaskSummary(ctx, "task-1", "permission-user", true); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("详情旧全局提示绕过当前授权", err)
	}
	query, _ := overview.NewQuery("permission-user", false, testTime.Add(time.Minute*4), "24h")
	facts, err := database.ReadOverviewFacts(ctx, query)
	if err != nil || !facts.TasksRestricted || !facts.MyTasksRestricted || facts.Metrics != nil || facts.MyTasks != nil {
		t.Fatal("权限受限误报可信零值", facts, err)
	}
	var frozen string
	if err := database.db.QueryRow(`SELECT submitted_permission_scope_json FROM tasks WHERE task_id='task-1'`).Scan(&frozen); err != nil || frozen != snapshot {
		t.Fatal("撤权改写历史快照", err)
	}
}

func Test权限撤销不取消已领取租约且孤立范围不生效(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	submitPermissionTask(t, database)
	if err := database.ClaimTask(ctx, validClaim("execution-1", "lease-1", "event-1", "claim-before-revoke")); err != nil {
		t.Fatal(err)
	}
	submission := validTaskSubmission("task-1")
	var argv []string
	if err := json.Unmarshal([]byte(submission.PlannedArgvJSON), &argv); err != nil {
		t.Fatal(err)
	}
	digest, err := executionTaskEnvelopeDigest(ctx, database.db, submission.TaskID, submission.NodeID, submission.ConfigFingerprint, submission.ToolVersion, submission.MetadataVersion, submission.CapabilityVersion, argv)
	if err != nil {
		t.Fatal(err)
	}
	secretRequest := ExecutionSecretResolutionRequest{AgentID: "agent-1", ExecutionID: "execution-1", LeaseID: "lease-1", LeaseEpoch: 1, EnvelopeDigest: digest, RequestID: "synthetic-secret-recheck", RequestDigest: strings.Repeat("a", 64), Now: testTime.Add(4 * time.Minute)}
	material, err := database.ResolveExecutionDatabaseConnection(ctx, secretRequest)
	if err != nil {
		t.Fatal("撤权前受控材料解析资格错误", err)
	}
	material.Destroy()
	revoke := permissionChange()
	revoke.ExpectedRevision = 2
	revoke.IdempotencyKey = "revoke-running"
	revoke.RequestID = "request-revoke-running"
	revoke.Assignment = accesscontrol.Assignment{}
	if _, err := database.ChangeSubjectPermissions(ctx, revoke); err != nil {
		t.Fatal(err)
	}
	material, err = database.ResolveExecutionDatabaseConnection(ctx, secretRequest)
	material.Destroy()
	if !errors.Is(err, ErrEventRejected) {
		t.Fatal("原解析回执绕过当前撤权", err)
	}
	var state string
	if err := database.db.QueryRow(`SELECT state FROM task_executions WHERE execution_id='execution-1'`).Scan(&state); err != nil || state != "STARTING" {
		t.Fatal("撤权擅自取消运行租约", state, err)
	}
	// 模拟历史遗留孤立范围，运行时仍必须要求当前运维角色。
	if _, err := database.db.Exec(`INSERT INTO subject_object_scopes(subject_id,scope_type,object_id,granted_by,granted_at) VALUES('permission-user','TASK_OPERATE_BY_DATA_SOURCE','source-1','subject-1',?)`, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetAuthorizedTaskSummary(ctx, "task-1", "permission-user"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("孤立运维范围仍生效", err)
	}
}

func Test权限专家分类来自领域结果而非元数据版本(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	if _, err := database.ChangeSubjectPermissions(ctx, permissionChange()); err != nil {
		t.Fatal(err)
	}
	ordinary := false
	expert := true
	for _, version := range []string{"obdumper-4.3.5-slice-v11-expert-parameters", "obdumper-4.3.5-slice-v12-schema-boolean"} {
		for _, test := range []struct {
			name string
			flag *bool
			want bool
		}{{"普通日期", &ordinary, false}, {"专家参数", &expert, true}, {"历史未知", nil, true}} {
			input := TaskSubmission{MetadataVersion: version, ExpertConfigRequired: test.flag}
			if submissionNeedsExpert(input) != test.want {
				t.Fatal("版本覆盖了领域分类", test.name)
			}
		}
	}
	if err := database.withWrite(ctx, func(tx *sql.Tx) error {
		_, raw, err := taskSubmissionPermissions(ctx, tx, "permission-user", "source-1", "node-1", false)
		if err != nil {
			return err
		}
		var allowed bool
		err = tx.QueryRowContext(ctx, `SELECT `+taskPermissionEligibility("t")+` FROM (SELECT 'permission-user' AS creator_subject_id,'source-1' AS data_source_id,'node-1' AS node_id,'OBDUMPER_EXPORT' AS task_type,'obdumper-4.3.5-slice-v11-expert-parameters' AS metadata_version,? AS submitted_permission_scope_json) t`, raw).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed {
			t.Fatal("普通分类被新版元数据错误阻断")
		}
		for _, classification := range []struct {
			expression string
			want       bool
		}{
			{"?", true},
			{"json_set(?, '$.expertConfig', json('true'))", false},
			{"json_remove(?, '$.expertConfig')", false},
		} {
			query := `SELECT ` + taskPermissionEligibility("t") + ` FROM (SELECT 'permission-user' AS creator_subject_id,'source-1' AS data_source_id,'node-1' AS node_id,'OBDUMPER_EXPORT' AS task_type,'obdumper-4.3.5-slice-v12-schema-boolean' AS metadata_version,` + classification.expression + ` AS submitted_permission_scope_json) t`
			if err := tx.QueryRowContext(ctx, query, raw).Scan(&allowed); err != nil {
				return err
			}
			if allowed != classification.want {
				t.Fatal("结构修订绕过专家权限或丢失普通分类")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func Test权限模板使用管理隔离与创建版本竞争(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	input := ExportConfigTemplateCreate{ExportConfigTemplate: ExportConfigTemplate{TemplateID: "permission-template", OwnerSubjectID: "subject-1", DisplayName: "合成权限模板", CapabilityVersion: "export-odp-full-csv-v1", ConfigJSON: `{"objectScope":{"database":"synthetic_db","scopeKind":"SPECIFIED","objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"contentSelection":{"contentKind":"DATA_ONLY"},"dataFormat":{"formatKind":"CSV"},"outputConfig":{"outputKind":"LOCAL","filePath":"/tmp/synthetic-out"}}`, ConfigFingerprint: testFingerprint, CreatedAt: testTime, UpdatedAt: testTime}, RequestID: "permission-template-create", IdempotencyKey: "permission-template-key", RequestDigest: strings.Repeat("b", 64)}
	if _, err := database.CreateExportConfigTemplate(ctx, input); err != nil {
		t.Fatal(err)
	}
	owner, err := database.ReadPermissionProfile(ctx, "subject-1", "subject-1")
	if err != nil || owner.Revision != 2 || !owner.Assignment.HasScope(accesscontrol.TemplateUse, input.TemplateID) {
		t.Fatal("创建未原子绑定具体范围和版本", owner, err)
	}
	c := permissionChange()
	c.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.Operator}, Scopes: []accesscontrol.ObjectScope{{Type: accesscontrol.TemplateManage, ObjectID: input.TemplateID}}}
	if _, err := database.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: "permission-user"}
	if database.Authorize(ctx, p, identity.ScopeTemplateManage, input.TemplateID) != nil || database.Authorize(ctx, p, identity.ScopeTemplateUse, input.TemplateID) == nil || database.Authorize(ctx, p, identity.ScopeTemplateCreate, "platform") == nil {
		t.Fatal("模板管理推导使用或创建")
	}
	items, err := database.ListConfigTemplates(ctx, p.ID)
	if err != nil || len(items) != 1 || items[0].ConfigJSON != "" || items[0].SourceTaskID != "" {
		t.Fatal("共享模板目录投影错误", items, err)
	}
	if _, err := database.UpdateExportConfigTemplate(ctx, ExportConfigTemplateUpdate{TemplateID: input.TemplateID, ActorSubjectID: p.ID, ExpectedRevision: 1, DisplayName: "合成改名", RequestID: "permission-template-rename", UpdatedAt: testTime.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	c.ExpectedRevision = 2
	c.IdempotencyKey = "permission-template-use"
	c.RequestID = "permission-template-use-request"
	c.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.BasicUser}, Scopes: []accesscontrol.ObjectScope{{Type: accesscontrol.TemplateUse, ObjectID: input.TemplateID}}}
	if _, err := database.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	if database.Authorize(ctx, p, identity.ScopeTemplateUse, input.TemplateID) != nil || database.Authorize(ctx, p, identity.ScopeTemplateManage, input.TemplateID) == nil {
		t.Fatal("模板使用推导管理")
	}
	if err := database.DeleteExportConfigTemplate(ctx, input.TemplateID, p.ID, 2, "permission-template-delete", testTime.Add(2*time.Minute)); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("模板使用者仍可删除", err)
	}
	c.ExpectedRevision = 3
	c.IdempotencyKey = "permission-admin"
	c.RequestID = "permission-admin-request"
	c.Confirmed = true
	c.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.SystemAdmin}}
	if _, err := database.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.ActorID = "permission-user"
	c.SubjectID = "subject-1"
	c.ExpectedRevision = 1
	c.IdempotencyKey = "stale-owner"
	c.RequestID = "stale-owner-request"
	c.Assignment = owner.Assignment
	if _, err := database.ChangeSubjectPermissions(ctx, c); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("对象创建绑定未参与权限版本竞争", err)
	}
}

// Test权限未接入能力新增失败关闭 验证审计以外的未接入能力与节点运行仍拒绝新增。
func Test权限未接入能力新增失败关闭(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	for _, capability := range []accesscontrol.Capability{accesscontrol.ProductionTask, accesscontrol.ExpertConfig, accesscontrol.SensitiveCommand, accesscontrol.GlobalSchedulingLog} {
		c := permissionChange()
		c.Assignment.Capabilities = []accesscontrol.Capability{capability}
		c.Confirmed = true
		if _, err := database.ChangeSubjectPermissions(ctx, c); !errors.Is(err, ErrPermissionAdditionGated) {
			t.Fatal("未接入能力仍可新增", capability, err)
		}
	}
	c := permissionChange()
	c.Assignment.Roles = append(c.Assignment.Roles, accesscontrol.Operator)
	c.Assignment.Scopes = append(c.Assignment.Scopes, accesscontrol.ObjectScope{Type: accesscontrol.NodeOperate, ObjectID: "node-1"})
	if _, err := database.ChangeSubjectPermissions(ctx, c); !errors.Is(err, ErrPermissionAdditionGated) {
		t.Fatal("运行操作范围仍可新增", err)
	}
}

func Test权限专家撤销阻断新提交且不改变已运行状态(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	if err := database.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatal(err)
	}
	if err := database.ClaimTask(ctx, validClaim("execution-1", "lease-1", "event-1", "claim-expert")); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='CAP_EXPERT_CONFIG'`); err != nil {
		t.Fatal(err)
	}
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: "subject-1"}
	if database.Authorize(ctx, p, identity.ScopeExpertConfiguration, "platform") == nil {
		t.Fatal("撤销专家能力后仍有效")
	}
	err := database.withWrite(ctx, func(tx *sql.Tx) error {
		_, _, err := taskSubmissionPermissions(ctx, tx, "subject-1", "source-1", "node-1", true)
		return err
	})
	if !errors.Is(err, ErrClaimIneligible) {
		t.Fatal("专家配置缺少独立能力仍可冻结", err)
	}
	// 普通任务不借专家能力决定其授权，撤销专家能力不取消当前普通执行。
	var state string
	if err := database.db.QueryRow(`SELECT state FROM task_executions WHERE execution_id='execution-1'`).Scan(&state); err != nil || state != "STARTING" {
		t.Fatal(state, err)
	}
}

func Test权限生产管理新增须确认且不推导生产使用(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	if _, err := database.db.Exec(`UPDATE data_sources SET environment='PRODUCTION' WHERE data_source_id='source-1'`); err != nil {
		t.Fatal(err)
	}
	c := permissionChange()
	c.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.DataSourceAdmin}, Scopes: []accesscontrol.ObjectScope{{Type: accesscontrol.DataSourceManage, ObjectID: "source-1"}}}
	if _, err := database.ChangeSubjectPermissions(ctx, c); !errors.Is(err, ErrPermissionConfirmation) {
		t.Fatal("生产管理范围未经确认即保存", err)
	}
	c.Confirmed = true
	if _, err := database.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: c.SubjectID}
	if database.Authorize(ctx, p, identity.ScopeDataSourceWrite, "source-1") != nil || database.Authorize(ctx, p, identity.ScopeDataSourceUse, "source-1") == nil {
		t.Fatal("生产管理推导生产任务能力")
	}
}
