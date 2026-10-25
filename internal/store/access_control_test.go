package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/identity"
)

func permissionFixture(t *testing.T) *Store {
	t.Helper()
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	if err := database.BootstrapPermissionOwner(context.Background(), "subject-1", testTime); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureAuthSubject(context.Background(), AuthSubject{SubjectID: "permission-user", ExternalSubject: "synthetic-permission-user", DisplayName: "合成权限用户", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	return database
}

func permissionChange() accesscontrol.Change {
	return accesscontrol.Change{ActorID: "subject-1", SubjectID: "permission-user", ExpectedRevision: 1, Assignment: accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.BasicUser}, Scopes: []accesscontrol.ObjectScope{{Type: accesscontrol.DataSourceUse, ObjectID: "source-1"}, {Type: accesscontrol.NodeUse, ObjectID: "node-1"}}}, Reason: "调整合成测试职责", IdempotencyKey: "permission-change-1", RequestID: "permission-request-1", At: testTime.Add(time.Minute)}
}

func Test权限保存与撤销立即生效且幂等不恢复旧授权(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	input := permissionChange()
	if _, err := database.db.Exec(`UPDATE export_drafts SET owner_subject_id='permission-user' WHERE draft_id='draft-1'`); err != nil {
		t.Fatal(err)
	}
	result, err := database.ChangeSubjectPermissions(ctx, input)
	if err != nil || result.Revision != 2 || !result.Changed || result.Replayed {
		t.Fatalf("保存未完成：%#v %v", result, err)
	}
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: input.SubjectID}
	if database.Authorize(ctx, p, identity.ScopeDataSourceUse, "source-1") != nil || database.Authorize(ctx, p, identity.ScopeDataSourceWrite, "source-1") == nil || database.Authorize(ctx, p, identity.ScopeDataSourceUse, "unscoped-source") == nil {
		t.Fatal("使用与管理或精确范围未隔离")
	}
	var status string
	if err = database.db.QueryRow(`SELECT status FROM precheck_runs WHERE precheck_id='precheck-1'`).Scan(&status); err != nil || status != "INVALIDATED" {
		t.Fatalf("旧预检查未失效：%s %v", status, err)
	}
	replay, err := database.ChangeSubjectPermissions(ctx, input)
	if err != nil || !replay.Replayed || replay.Revision != 2 {
		t.Fatalf("原请求未重放：%#v %v", replay, err)
	}
	changed := input
	changed.Reason = "异内容不得重放"
	if _, err = database.ChangeSubjectPermissions(ctx, changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("异内容幂等未拒绝", err)
	}
	revoke := input
	revoke.ExpectedRevision = 2
	revoke.IdempotencyKey = "permission-revoke"
	revoke.Assignment = accesscontrol.Assignment{}
	revoke.RequestID = "permission-request-revoke"
	if result, err = database.ChangeSubjectPermissions(ctx, revoke); err != nil || result.Revision != 3 {
		t.Fatal(result, err)
	}
	if database.Authorize(ctx, p, identity.ScopeDataSourceUse, "source-1") == nil || database.AuthorizeRole(ctx, p, identity.RoleBasicUser) == nil {
		t.Fatal("撤权仍使用旧权限")
	}
	if replay, err = database.ChangeSubjectPermissions(ctx, input); err != nil || !replay.Replayed || replay.Revision != 2 {
		t.Fatal("原回执重放失败", replay, err)
	}
	if database.Authorize(ctx, p, identity.ScopeDataSourceUse, "source-1") == nil {
		t.Fatal("旧回执恢复了已撤销授权")
	}
}

func Test权限保存版本状态自身与依赖边界(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	for name, mutate := range map[string]func(*accesscontrol.Change){
		"自身只读":  func(c *accesscontrol.Change) { c.SubjectID = c.ActorID },
		"陈旧版本":  func(c *accesscontrol.Change) { c.ExpectedRevision = 9 },
		"不存在对象": func(c *accesscontrol.Change) { c.Assignment.Scopes[0].ObjectID = "unavailable-source" },
		"缺少依赖":  func(c *accesscontrol.Change) { c.Assignment.Roles = nil },
		"新增系统管理员未确认": func(c *accesscontrol.Change) {
			c.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.SystemAdmin}}
		},
		"敏感原因": func(c *accesscontrol.Change) { c.Reason = "token=synthetic-sensitive-input" },
	} {
		t.Run(name, func(t *testing.T) {
			c := permissionChange()
			c.IdempotencyKey = name
			c.RequestID = "request-" + name
			mutate(&c)
			if _, err := database.ChangeSubjectPermissions(ctx, c); err == nil {
				t.Fatal("非法变更未被拒绝")
			}
		})
	}
	profile, err := database.ReadPermissionProfile(ctx, "subject-1", "permission-user")
	if err != nil || profile.Revision != 1 || len(profile.Assignment.Roles) != 0 {
		t.Fatal("失败变更产生部分写入", profile, err)
	}
	if _, err = database.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='permission-user'`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ChangeSubjectPermissions(ctx, permissionChange()); !errors.Is(err, accesscontrol.ErrInactiveAddition) {
		t.Fatal("停用用户仍可新增授权", err)
	}
	var count int
	if err = database.db.QueryRow(`SELECT count(*) FROM audit_events WHERE safe_diff_json LIKE '%synthetic-sensitive-input%'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("失败审计保存了敏感输入", count, err)
	}
}

func Test权限审计失败整次回滚(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	if _, err := database.db.Exec(`UPDATE export_drafts SET owner_subject_id='permission-user' WHERE draft_id='draft-1'; CREATE TRIGGER permission_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='SUBJECT_PERMISSIONS_CHANGED' AND NEW.result='SUCCEEDED' BEGIN SELECT RAISE(ABORT,'合成审计失败'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ChangeSubjectPermissions(ctx, permissionChange()); err == nil {
		t.Fatal("审计失败仍宣称成功")
	}
	profile, err := database.ReadPermissionProfile(ctx, "subject-1", "permission-user")
	if err != nil || profile.Revision != 1 || len(profile.Assignment.Roles) != 0 || len(profile.Assignment.Scopes) != 0 {
		t.Fatal("授权未回滚", profile, err)
	}
	var status string
	if err = database.db.QueryRow(`SELECT status FROM precheck_runs WHERE precheck_id='precheck-1'`).Scan(&status); err != nil || status != "SUCCEEDED" {
		t.Fatal("预检查失效未回滚", status, err)
	}
	var count int
	if err = database.db.QueryRow(`SELECT count(*) FROM request_idempotency WHERE operation='CHANGE_SUBJECT_PERMISSIONS'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("失败变更留下成功回执", count, err)
	}
}

func Test初始管理员仅引导一次且普通系统管理员无业务权限(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	if _, err := database.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_DATA_SOURCE_ADMIN'`); err != nil {
		t.Fatal(err)
	}
	if err := database.BootstrapPermissionOwner(ctx, "subject-1", testTime.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: "subject-1"}
	if database.AuthorizeRole(ctx, p, identity.RoleDataSourceAdmin) == nil {
		t.Fatal("重启恢复了已撤销职责")
	}
	if err := database.BootstrapPermissionOwner(ctx, "permission-user", testTime); !errors.Is(err, ErrPermissionDenied) {
		t.Fatal("初始身份可被切换", err)
	}
	c := permissionChange()
	c.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.SystemAdmin}}
	c.Confirmed = true
	if _, err := database.ChangeSubjectPermissions(ctx, c); err != nil {
		t.Fatal(err)
	}
	p.ID = "permission-user"
	if database.Authorize(ctx, p, identity.ScopePermissionManage, "platform") != nil {
		t.Fatal("系统管理员没有权限管理资格")
	}
	for _, check := range []struct {
		scope identity.Scope
		id    string
	}{{identity.ScopeDataSourceRead, "source-1"}, {identity.ScopeNodeManage, "node-1"}, {identity.ScopeTaskReadAll, "*"}} {
		if database.Authorize(ctx, p, check.scope, check.id) == nil {
			t.Fatal("系统管理天然获得了业务权限", check.scope)
		}
	}
	if database.AuthorizeRole(ctx, p, identity.RoleGlobalAdmin) == nil {
		t.Fatal("系统管理员继承了部署全局只读身份")
	}
}

func Test授权目录只返回必要事实并先校验管理员(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	if page, err := database.ListPermissionSubjects(ctx, "permission-user", accesscontrol.SubjectQuery{Limit: 10}); !errors.Is(err, ErrPermissionDenied) || len(page.Items) != 0 {
		t.Fatal("无权限主体可枚举用户", page, err)
	}
	items, more, err := database.ListPermissionCandidates(ctx, "subject-1", accesscontrol.DataSourceUse, "", "", 10)
	if err != nil || more || len(items) != 1 || !items[0].Available {
		t.Fatal("授权目录未返回事实", items, more, err)
	}
	encoded, _ := json.Marshal(items)
	for _, forbidden := range []string{"127.0.0.1", "synthetic_user", "synthetic-tenant", "credential-1", "javaPath", "token"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("目录泄露了额外数据", forbidden)
		}
	}
	if _, _, err = database.ListPermissionCandidates(ctx, "permission-user", accesscontrol.DataSourceUse, "", "", 10); !errors.Is(err, ErrPermissionDenied) {
		t.Fatal("无权限可读取候选", err)
	}
	page, err := database.ListPermissionSubjects(ctx, "subject-1", accesscontrol.SubjectQuery{Limit: 1})
	if err != nil || !page.HasMore || len(page.Items) != 1 {
		t.Fatal("有界分页失败", page, err)
	}
	if _, err := database.ListPermissionSubjects(ctx, "subject-1", accesscontrol.SubjectQuery{Limit: 10, Search: strings.Repeat("合", 128)}); err != nil {
		t.Fatal("中文搜索错误按字节截断", err)
	}
	if _, err := database.ListPermissionSubjects(ctx, "subject-1", accesscontrol.SubjectQuery{Limit: 10, Search: strings.Repeat("合", 129)}); !errors.Is(err, accesscontrol.ErrInvalid) {
		t.Fatal("超长搜索未阻断", err)
	}
	if _, err = database.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_SYSTEM_ADMIN'`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ChangeSubjectPermissions(ctx, permissionChange()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatal("旧页面继续使用已撤销管理权", err)
	}
}

func Test权限初次引导超出范围上限整次回滚不截断(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	if err := database.withWrite(ctx, func(tx *sql.Tx) error {
		for i := 0; i < 201; i++ {
			if _, err := tx.ExecContext(ctx, `INSERT INTO subject_object_scopes VALUES('subject-1','NODE_USE',?,'subject-1',?)`, "synthetic-retained-node-"+strconv.Itoa(i), utcText(testTime)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.BootstrapPermissionOwner(ctx, "subject-1", testTime); !errors.Is(err, accesscontrol.ErrInvalid) {
		t.Fatal("超过上限仍提交初次引导", err)
	}
	assertCount(t, database.db, `SELECT count(*) FROM subject_object_scopes WHERE subject_id='subject-1'`, 201)
	assertCount(t, database.db, `SELECT count(*) FROM subject_permission_versions WHERE deployment_owner=1`, 0)
	assertCount(t, database.db, `SELECT count(*) FROM subject_grants WHERE grant_code='ROLE_SYSTEM_ADMIN'`, 0)
}
