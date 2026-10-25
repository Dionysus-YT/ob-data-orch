package controlplane

import (
	"context"

	"ob-data-orch/internal/identity"
)

// browserActorContextKey 只能由已认证浏览器入口赋值，浏览器请求字段不能覆盖。
type browserActorContextKey struct{}

func (s *Server) authorizeExpertConfiguration(ctx context.Context, required bool) error {
	// 正式组合根装配持久权限端口；旧隔离端口夹具继续使用其显式测试授权。
	if !required || s.permissions == nil {
		return nil
	}
	p, ok := ctx.Value(browserActorContextKey{}).(identity.Principal)
	if !ok {
		return identity.ErrDenied
	}
	return identity.Can(ctx, s.authorizer, p, identity.ScopeExpertConfiguration, "platform")
}
