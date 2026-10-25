package store

import (
	"context"
	"testing"

	"ob-data-orch/internal/identity"
)

// Test工具状态只允许当前系统管理员且拒绝机器与其他范围 使用真实 SQLite 授权覆盖撤权和停用。
func Test工具状态只允许当前系统管理员且拒绝机器与其他范围(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: "subject-1"}
	if err := s.Authorize(ctx, p, identity.ScopeToolCompatibilityRead, "platform"); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		principal identity.Principal
		object    string
	}{{identity.Principal{Type: identity.BrowserPrincipal, ID: "permission-user"}, "platform"}, {identity.Principal{Type: identity.AgentPrincipal, ID: "subject-1"}, "platform"}, {p, "*"}} {
		if s.Authorize(ctx, input.principal, identity.ScopeToolCompatibilityRead, input.object) == nil {
			t.Fatal("错误角色、机器或对象范围被允许")
		}
	}
	if _, err := s.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_SYSTEM_ADMIN'`); err != nil {
		t.Fatal(err)
	}
	if s.Authorize(ctx, p, identity.ScopeToolCompatibilityRead, "platform") == nil {
		t.Fatal("撤权后仍可读取工具基线")
	}
	if _, err := s.db.Exec(`INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES('subject-1','ROLE_SYSTEM_ADMIN','subject-1',?); UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-1'`, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	if s.Authorize(ctx, p, identity.ScopeToolCompatibilityRead, "platform") == nil {
		t.Fatal("停用后仍可读取工具基线")
	}
}
