package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/accesscontrol"
)

func initialAuditInput() accesscontrol.InitialAuditGrant {
	return accesscontrol.InitialAuditGrant{SubjectID: "subject-1", ExpectedRevision: 1, Confirmed: true, At: testTime.Add(time.Minute)}
}

func Test首次审计授予仅增加独立能力且普通自身编辑仍拒绝(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	before, err := database.ReadPermissionProfile(ctx, "subject-1", "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := database.AuditQueryAccess(ctx, "subject-1"); err != nil || allowed {
		t.Fatal("初始身份不应隐含审计", err)
	}
	if revision, err := database.GrantInitialAuditAccess(ctx, initialAuditInput()); err != nil || revision != 2 {
		t.Fatal("首次授予失败", revision, err)
	}
	after, err := database.ReadPermissionProfile(ctx, "subject-1", "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	expected := before.Assignment
	expected.Capabilities = append(slices.Clone(expected.Capabilities), accesscontrol.AuditLog)
	if after.Revision != 2 || after.Editable || !reflect.DeepEqual(after.Assignment, accesscontrol.Canonical(expected)) {
		t.Fatal("首次授予改变了其他职责或自身编辑边界")
	}
	if allowed, err := database.AuditQueryAccess(ctx, "subject-1"); err != nil || !allowed {
		t.Fatal("显式能力未参与真实读取", err)
	}
	change := permissionChange()
	change.SubjectID, change.ExpectedRevision, change.Assignment = "subject-1", 2, after.Assignment
	if _, err := database.ChangeSubjectPermissions(ctx, change); !errors.Is(err, ErrPermissionSelfReadOnly) {
		t.Fatal("普通自身编辑被首次入口放开", err)
	}
	var count int
	if err := database.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='PERMISSION_INITIAL_AUDIT_GRANTED' AND json_extract(safe_diff_json,'$.confirmed')=1 AND json_extract(safe_diff_json,'$.revision')=2`).Scan(&count); err != nil || count != 1 {
		t.Fatal("首次授予审计不完整", count, err)
	}
}

func Test首次审计资格与确认版本失败关闭(t *testing.T) {
	for _, name := range []string{"未确认", "旧版本", "非初始身份", "停用", "缺系统职责", "非法版本", "无时间"} {
		t.Run(name, func(t *testing.T) {
			database := permissionFixture(t)
			input := initialAuditInput()
			switch name {
			case "未确认":
				input.Confirmed = false
			case "旧版本":
				input.ExpectedRevision = 2
			case "非初始身份":
				input.SubjectID = "permission-user"
			case "停用":
				if _, err := database.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-1'`); err != nil {
					t.Fatal(err)
				}
			case "缺系统职责":
				if _, err := database.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_SYSTEM_ADMIN'`); err != nil {
					t.Fatal(err)
				}
			case "非法版本":
				input.ExpectedRevision = 9007199254740991
			case "无时间":
				input.At = time.Time{}
			}
			if revision, err := database.GrantInitialAuditAccess(context.Background(), input); err == nil || revision != 0 {
				t.Fatal("非法首次授予仍成功")
			}
			var count, consumed, revision int
			if err := database.db.QueryRow(`SELECT (SELECT count(*) FROM subject_grants WHERE grant_code='CAP_AUDIT_LOG'),audit_bootstrap_consumed,revision FROM subject_permission_versions WHERE subject_id='subject-1'`).Scan(&count, &consumed, &revision); err != nil || count != 0 || consumed != 0 || revision != 1 {
				t.Fatal("拒绝后留下部分授予", err)
			}
		})
	}
}

func Test首次审计审计失败回滚标记版本和授权(t *testing.T) {
	database := permissionFixture(t)
	if _, err := database.db.Exec(`CREATE TRIGGER initial_audit_fixture_failure BEFORE INSERT ON audit_events WHEN NEW.action='PERMISSION_INITIAL_AUDIT_GRANTED' BEGIN SELECT RAISE(ABORT,'合成审计失败'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GrantInitialAuditAccess(context.Background(), initialAuditInput()); err == nil {
		t.Fatal("审计失败仍成功")
	}
	state, err := database.ReadInitialAuditAccess(context.Background(), "subject-1")
	if err != nil || state.Consumed || state.Granted || state.Revision != 1 {
		t.Fatal("失败没有原子回滚", err)
	}
	if _, err := database.db.Exec(`DROP TRIGGER initial_audit_fixture_failure`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GrantInitialAuditAccess(context.Background(), initialAuditInput()); err != nil {
		t.Fatal("回滚后不能重试", err)
	}
}

func Test首次审计并发只提交一次且证据不可清除(t *testing.T) {
	database := permissionFixture(t)
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Go(func() {
			_, err := database.GrantInitialAuditAccess(context.Background(), initialAuditInput())
			results <- err
		})
	}
	group.Wait()
	close(results)
	successes, consumed := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrInitialAuditAccessConsumed) {
			consumed++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || consumed != 1 {
		t.Fatal("并发首次授予重复提交")
	}
	for _, query := range []string{`UPDATE subject_permission_versions SET audit_bootstrap_consumed=0 WHERE subject_id='subject-1'`, `DELETE FROM subject_permission_versions WHERE subject_id='subject-1'`} {
		if _, err := database.db.Exec(query); err == nil {
			t.Fatal("一次性证据可清除")
		}
	}
	var count int
	if err := database.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='PERMISSION_INITIAL_AUDIT_GRANTED'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("并发产生重复审计", count, err)
	}
}

func Test首次审计撤权与重开不恢复且普通授予也消耗入口(t *testing.T) {
	for _, bootstrap := range []bool{true, false} {
		t.Run(map[bool]string{true: "部署首次授予", false: "普通管理员显式授予"}[bootstrap], func(t *testing.T) {
			database := permissionFixture(t)
			ctx := context.Background()
			manager := permissionChange()
			manager.Assignment = accesscontrol.Assignment{Roles: []accesscontrol.Role{accesscontrol.SystemAdmin}}
			manager.Confirmed = true
			if _, err := database.ChangeSubjectPermissions(ctx, manager); err != nil {
				t.Fatal(err)
			}
			profile, err := database.ReadPermissionProfile(ctx, "subject-1", "subject-1")
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap {
				if _, err = database.GrantInitialAuditAccess(ctx, initialAuditInput()); err != nil {
					t.Fatal(err)
				}
			} else {
				grant := permissionChange()
				grant.ActorID, grant.SubjectID = "permission-user", "subject-1"
				grant.Assignment = profile.Assignment
				grant.Assignment.Capabilities = append(slices.Clone(profile.Assignment.Capabilities), accesscontrol.AuditLog)
				grant.Confirmed = true
				if _, err = database.ChangeSubjectPermissions(ctx, grant); err != nil {
					t.Fatal(err)
				}
			}
			revoke := permissionChange()
			revoke.ActorID, revoke.SubjectID, revoke.ExpectedRevision, revoke.IdempotencyKey = "permission-user", "subject-1", 2, "initial-audit-revoke"
			revoke.Assignment = profile.Assignment
			if _, err = database.ChangeSubjectPermissions(ctx, revoke); err != nil {
				t.Fatal(err)
			}
			var sequence int
			var name, path string
			if err = database.db.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
				t.Fatal(err)
			}
			if err = database.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if err = reopened.BootstrapPermissionOwner(ctx, "subject-1", testTime.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			state, err := reopened.ReadInitialAuditAccess(ctx, "subject-1")
			if err != nil || state.Granted || !state.Consumed || state.Revision != 3 {
				t.Fatal("重开恢复了撤权或丢失证据", err)
			}
			input := initialAuditInput()
			input.ExpectedRevision = 3
			if _, err = reopened.GrantInitialAuditAccess(ctx, input); !errors.Is(err, ErrInitialAuditAccessConsumed) {
				t.Fatal("撤权后仍可部署重授", err)
			}
		})
	}
}
