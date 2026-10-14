package controlplane

import (
	"context"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// hasGlobalTaskRead 只接受可信角色和明确的全局读取能力，不把节点管理员或客户端筛选当作全局授权。
func (s *Server) hasGlobalTaskRead(ctx context.Context, principal identity.Principal) bool {
	return identity.HasRole(ctx, s.roles, principal, identity.RoleGlobalAdmin) == nil && identity.Can(ctx, s.authorizer, principal, identity.ScopeTaskReadAll, "*") == nil
}

// readTaskSummary 在每次读取及 SSE 续读时重验当前全局权限；撤销后退回原对象范围校验。
func (s *Server) readTaskSummary(ctx context.Context, principal identity.Principal, taskID string) (store.TaskSummary, error) {
	if s.hasGlobalTaskRead(ctx, principal) {
		if reader, ok := s.tasks.(interface {
			GetGlobalTaskSummary(context.Context, string, string) (store.TaskSummary, error)
		}); ok {
			return reader.GetGlobalTaskSummary(ctx, taskID, principal.ID)
		}
	}
	return s.tasks.GetAuthorizedTaskSummary(ctx, taskID, principal.ID)
}
