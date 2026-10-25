// Package identity defines the fail-closed authentication and authorization
// seam used by the control plane. It deliberately provides no HTTP header,
// cookie, token, or test-user implementation.
package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrUnauthenticated = errors.New("identity is unauthenticated")
	ErrUnavailable     = errors.New("identity provider is unavailable")
	ErrDenied          = errors.New("access is not authorized")
)

type PrincipalType string

const (
	BrowserPrincipal PrincipalType = "BROWSER"
	AgentPrincipal   PrincipalType = "AGENT"
)

type Principal struct {
	Type PrincipalType
	ID   string
}

// Provider receives the whole request so a future deployment integration can
// validate its own secure session or machine credential. The platform does not
// infer identity from a user-controlled header.
type Provider interface {
	AuthenticateBrowser(*http.Request) (Principal, error)
	AuthenticateAgent(*http.Request) (Principal, error)
}

// Scope 是固定功能与对象边界，调用者必须同时提供明确对象。
type Scope string

const (
	ScopeDataSourceRead Scope = "DATA_SOURCE_READ"
	// ScopeDataSourceUse 要求任务职责和明确使用范围，管理或读取权不能替代。
	ScopeDataSourceUse   Scope = "DATA_SOURCE_USE"
	ScopeDataSourceWrite Scope = "DATA_SOURCE_WRITE"
	ScopeNodeUse         Scope = "NODE_USE"
	ScopeNodeManage      Scope = "NODE_MANAGE"
	// ScopeNodeOperate 仅允许既有节点运行操作，不允许编辑工具或注册配置。
	ScopeNodeOperate Scope = "NODE_OPERATE"
	// ScopeTemplateUse 与 ScopeTemplateManage 分离模板派生和管理，不扩大底层对象权限。
	ScopeTemplateUse    Scope = "TEMPLATE_USE"
	ScopeTemplateManage Scope = "TEMPLATE_MANAGE"
	// ScopeTemplateCreate 仅保留初始部署身份已有的模板保存入口，不从普通运维角色推导。
	ScopeTemplateCreate Scope = "TEMPLATE_CREATE"
	// ScopeExpertConfiguration 只检验已存在的专家能力及基础角色，不由其他管理员职责推导。
	ScopeExpertConfiguration Scope = "CAP_EXPERT_CONFIG"
	// ScopePermissionManage 只允许权限元信息及授权目录，不赋予业务详情访问。
	ScopePermissionManage Scope = "PERMISSION_MANAGE"
	// ScopeLogRetentionRead 与写权限分开表达，仅限普通日志策略元信息。
	ScopeLogRetentionRead Scope = "LOG_RETENTION_READ"
	// ScopeLogRetentionWrite 不授予任务、审计日志或部署安全配置的访问。
	ScopeLogRetentionWrite Scope = "LOG_RETENTION_WRITE"
	// ScopeSchedulingPolicyRead 仅允许读取安全调度设置，不能推导全局任务读取。
	ScopeSchedulingPolicyRead Scope = "SCHEDULING_POLICY_READ"
	// ScopeSchedulingPolicyWrite 只允许修改平台并发及时间约束，不授予业务任务访问。
	ScopeSchedulingPolicyWrite Scope = "SCHEDULING_POLICY_WRITE"
	// ScopeSecurityPolicyRead 只读启动安全状态，不授予秘密、审计正文或配置修改权限。
	ScopeSecurityPolicyRead Scope = "SECURITY_POLICY_READ"
	// ScopeToolCompatibilityRead 只读内置工具基线，不授予节点、参数发布或任务权限。
	ScopeToolCompatibilityRead Scope = "TOOL_COMPATIBILITY_READ"
	ScopeTaskRead              Scope = "TASK_READ"
	// ScopeTaskReadAll 只由可信身份投影授予全局任务读取，不接受浏览器自报。
	ScopeTaskReadAll Scope = "TASK_READ_ALL"
)

// Authorizer 必须明确判定固定能力与对象，不隐含管理员或全对象通配授权。
type Authorizer interface {
	Authorize(context.Context, Principal, Scope, string) error
}

// Role 沿用已确认的固定产品职责，与具体对象范围分开；创建动作尚无既有对象 ID。
type Role string

const (
	RoleBasicUser       Role = "ROLE_BASIC_USER"
	RoleOperator        Role = "ROLE_OPERATOR"
	RoleDataSourceAdmin Role = "ROLE_DATA_SOURCE_ADMIN"
	RoleNodeAdmin       Role = "ROLE_NODE_ADMIN"
	RoleSystemAdmin     Role = "ROLE_SYSTEM_ADMIN"
	// RoleGlobalAdmin 对应用户确认的全局管理员；节点管理角色不能推导此角色。
	RoleGlobalAdmin Role = "ROLE_GLOBAL_ADMIN"
)

// RoleAuthorizer 从可信身份投影复验当前角色，不因角色授予任意对象范围。
type RoleAuthorizer interface {
	AuthorizeRole(context.Context, Principal, Role) error
}

func Validate(principal Principal, expected PrincipalType) error {
	if principal.Type != expected || strings.TrimSpace(principal.ID) == "" {
		return ErrUnauthenticated
	}
	return nil
}

func Can(ctx context.Context, authorizer Authorizer, principal Principal, scope Scope, objectID string) error {
	if authorizer == nil {
		return ErrUnavailable
	}
	if strings.TrimSpace(objectID) == "" || scope == "" {
		return ErrDenied
	}
	if err := authorizer.Authorize(ctx, principal, scope, objectID); err != nil {
		return ErrDenied
	}
	return nil
}

// HasRole is used only where the product permits an action before an object
// exists, such as creating a data source. It fails closed if the role service
// is absent, returns an error, or receives an invalid principal/role.
func HasRole(ctx context.Context, authorizer RoleAuthorizer, principal Principal, role Role) error {
	if authorizer == nil || Validate(principal, BrowserPrincipal) != nil || role == "" {
		return ErrDenied
	}
	if err := authorizer.AuthorizeRole(ctx, principal, role); err != nil {
		return ErrDenied
	}
	return nil
}
