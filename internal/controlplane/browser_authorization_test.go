package controlplane

import (
	"context"
	"testing"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

func Test专家配置复用可信浏览器身份且拒绝未授予能力(t *testing.T) {
	p := identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-expert-user"}
	port := &permissionPort{}
	s := &Server{permissions: port, authorizer: permissionExpertAuthorizer{allowed: true}}
	if s.authorizeExpertConfiguration(context.Background(), true) == nil {
		t.Fatal("缺少可信主体仍允许专家配置")
	}
	ctx := context.WithValue(context.Background(), browserActorContextKey{}, p)
	if s.authorizeExpertConfiguration(ctx, true) != nil {
		t.Fatal("精确独立能力未通过")
	}
	s.authorizer = permissionExpertAuthorizer{}
	if s.authorizeExpertConfiguration(ctx, true) == nil || s.authorizeExpertConfiguration(ctx, false) != nil {
		t.Fatal("普通配置与专家配置未隔离")
	}
	for _, ordinary := range []normalizedExportDraft{{ScopeKind: "QUERY_RESULT", QuerySql: "select 1"}, {TimestampFormats: store.TimestampFormatConfig{DateValueFormat: "yyyy-MM-dd"}}} {
		if ordinary.NeedsExpertMetadata() || s.authorizeExpertConfiguration(ctx, ordinary.NeedsExpertMetadata()) != nil {
			t.Fatal("普通结果集或日期格式错误升级为专家权限")
		}
	}
}

type permissionExpertAuthorizer struct{ allowed bool }

func (a permissionExpertAuthorizer) Authorize(_ context.Context, p identity.Principal, scope identity.Scope, id string) error {
	if a.allowed && p.ID == "synthetic-expert-user" && scope == identity.ScopeExpertConfiguration && id == "platform" {
		return nil
	}
	return identity.ErrDenied
}
