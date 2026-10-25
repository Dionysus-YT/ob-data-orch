package store

import (
	"context"
	"database/sql"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/identity"
)

// Authorize 每次读取当前持久角色与具体范围，权限加载失败绝不回退到初始管理员或缓存。
func (s *Store) Authorize(ctx context.Context, p identity.Principal, scope identity.Scope, objectID string) error {
	if s == nil || s.db == nil || identity.Validate(p, identity.BrowserPrincipal) != nil || objectID == "" {
		return identity.ErrDenied
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return identity.ErrUnavailable
	}
	defer tx.Rollback()
	profile, err := permissionProfile(ctx, tx, p.ID)
	if err != nil || profile.Subject.AccountStatus != "ACTIVE" {
		return identity.ErrDenied
	}
	value := profile.Assignment
	allowed := false
	switch scope {
	case identity.ScopeExpertConfiguration:
		allowed = objectID == "platform" && value.HasCapability(accesscontrol.ExpertConfig)
	case identity.ScopeTemplateCreate:
		allowed = objectID == "platform" && profile.InitialAdministrator && value.CanCreateTask() && value.HasRole(accesscontrol.Operator)
	case identity.ScopePermissionManage, identity.ScopeLogRetentionRead, identity.ScopeLogRetentionWrite, identity.ScopeSchedulingPolicyRead, identity.ScopeSchedulingPolicyWrite, identity.ScopeSecurityPolicyRead, identity.ScopeToolCompatibilityRead:
		allowed = objectID == "platform" && value.HasRole(accesscontrol.SystemAdmin)
	case identity.ScopeDataSourceRead, identity.ScopeDataSourceUse, identity.ScopeDataSourceWrite:
		manage := value.HasScope(accesscontrol.DataSourceManage, objectID)
		use := value.HasScope(accesscontrol.DataSourceUse, objectID)
		if use {
			var environment string
			if tx.QueryRowContext(ctx, `SELECT environment FROM data_sources WHERE data_source_id=?`, objectID).Scan(&environment) != nil {
				use = false
			} else if environment == "PRODUCTION" || environment == "PROD" {
				use = value.HasCapability(accesscontrol.ProductionTask)
			}
		}
		switch scope {
		case identity.ScopeDataSourceRead:
			allowed = manage || use
		case identity.ScopeDataSourceUse:
			allowed = use
		case identity.ScopeDataSourceWrite:
			allowed = manage
		}
	case identity.ScopeNodeUse:
		allowed = value.HasScope(accesscontrol.NodeUse, objectID)
	case identity.ScopeNodeManage:
		allowed = value.HasScope(accesscontrol.NodeManage, objectID)
	case identity.ScopeNodeOperate:
		allowed = value.HasScope(accesscontrol.NodeOperate, objectID)
	case identity.ScopeTaskReadAll:
		allowed = objectID == "*" && initialGlobalTaskRead(profile)
	case identity.ScopeTaskRead:
		var creator, source string
		if tx.QueryRowContext(ctx, `SELECT creator_subject_id,data_source_id FROM tasks WHERE task_id=?`, objectID).Scan(&creator, &source) == nil {
			allowed = value.CanReadTask(p.ID, creator, source) || initialGlobalTaskRead(profile)
		}
	case identity.ScopeTemplateUse, identity.ScopeTemplateManage:
		var owner string
		if tx.QueryRowContext(ctx, `SELECT owner_subject_id FROM export_config_templates WHERE template_id=?`, objectID).Scan(&owner) == nil {
			if scope == identity.ScopeTemplateUse {
				allowed = value.CanCreateTask() && (owner == p.ID || value.HasScope(accesscontrol.TemplateUse, objectID))
			} else {
				allowed = value.HasRole(accesscontrol.Operator) && (owner == p.ID || value.HasScope(accesscontrol.TemplateManage, objectID))
			}
		}
	}
	if !allowed {
		return identity.ErrDenied
	}
	if tx.Commit() != nil {
		return identity.ErrUnavailable
	}
	return nil
}

// AuthorizeRole 使用固定持久角色；兼容全局只读角色只属于已绑定的初始部署身份。
func (s *Store) AuthorizeRole(ctx context.Context, p identity.Principal, role identity.Role) error {
	if s == nil || s.db == nil || identity.Validate(p, identity.BrowserPrincipal) != nil {
		return identity.ErrDenied
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return identity.ErrUnavailable
	}
	defer tx.Rollback()
	profile, err := permissionProfile(ctx, tx, p.ID)
	if err != nil || profile.Subject.AccountStatus != "ACTIVE" {
		return identity.ErrDenied
	}
	allowed := false
	if role == identity.RoleGlobalAdmin {
		allowed = initialGlobalTaskRead(profile)
	} else {
		switch role {
		case identity.RoleBasicUser, identity.RoleOperator, identity.RoleDataSourceAdmin, identity.RoleNodeAdmin, identity.RoleSystemAdmin:
			allowed = profile.Assignment.HasRole(accesscontrol.Role(role))
		}
	}
	if !allowed {
		return identity.ErrDenied
	}
	if tx.Commit() != nil {
		return identity.ErrUnavailable
	}
	return nil
}

func initialGlobalTaskRead(profile accesscontrol.Profile) bool {
	// DEC-048/2026-10-10 明确授予的初始身份兼容能力不由普通系统管理员角色推导。
	return profile.InitialAdministrator && profile.Assignment.HasRole(accesscontrol.SystemAdmin) && profile.Assignment.HasRole(accesscontrol.Operator)
}
