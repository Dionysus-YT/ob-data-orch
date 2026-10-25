package store

import (
	"context"
	"testing"

	"ob-data-orch/internal/identity"
)

func Test安全状态范围仅当前系统管理员且不接受机器身份(t *testing.T) {
	s := permissionFixture(t)
	ctx := context.Background()
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: "subject-1"}
	if err := s.Authorize(ctx, p, identity.ScopeSecurityPolicyRead, "platform"); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		principal identity.Principal
		object    string
	}{{identity.Principal{Type: identity.BrowserPrincipal, ID: "permission-user"}, "platform"}, {identity.Principal{Type: identity.AgentPrincipal, ID: "subject-1"}, "platform"}, {p, "*"}} {
		if err := s.Authorize(ctx, input.principal, identity.ScopeSecurityPolicyRead, input.object); err == nil {
			t.Fatal("错误角色、机器身份或对象被放行")
		}
	}
	if _, err := s.db.Exec(`DELETE FROM subject_grants WHERE subject_id='subject-1' AND grant_code='ROLE_SYSTEM_ADMIN'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Authorize(ctx, p, identity.ScopeSecurityPolicyRead, "platform"); err == nil {
		t.Fatal("撤权后仍可读取")
	}
	if _, err := s.db.Exec(`INSERT INTO subject_grants(subject_id,grant_code,granted_by,granted_at) VALUES('subject-1','ROLE_SYSTEM_ADMIN','subject-1',?); UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-1'`, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	if err := s.Authorize(ctx, p, identity.ScopeSecurityPolicyRead, "platform"); err == nil {
		t.Fatal("停用主体仍可读取")
	}
}
