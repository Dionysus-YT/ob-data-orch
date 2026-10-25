// Package accesscontrol 定义固定角色、显式对象范围和授权变更规则，不负责认证账号或会话。
package accesscontrol

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Role 是已确认的五类固定职责，系统管理员不会因此获得业务对象权限。
type Role string

const (
	BasicUser       Role = "ROLE_BASIC_USER"
	Operator        Role = "ROLE_OPERATOR"
	DataSourceAdmin Role = "ROLE_DATA_SOURCE_ADMIN"
	NodeAdmin       Role = "ROLE_NODE_ADMIN"
	SystemAdmin     Role = "ROLE_SYSTEM_ADMIN"
)

// Capability 是独立授予的能力；实际业务仍须复验对象范围和当前安全门禁。
type Capability string

const (
	ProductionTask      Capability = "CAP_PRODUCTION_TASK"
	ExpertConfig        Capability = "CAP_EXPERT_CONFIG"
	SensitiveCommand    Capability = "CAP_SENSITIVE_COMMAND"
	AuditLog            Capability = "CAP_AUDIT_LOG"
	GlobalSchedulingLog Capability = "CAP_GLOBAL_SCHEDULING_LOG"
)

// ScopeType 区分对象使用、管理和任务运维，不接受全部对象或范围表达式。
type ScopeType string

const (
	DataSourceUse           ScopeType = "DATA_SOURCE_USE"
	DataSourceManage        ScopeType = "DATA_SOURCE_MANAGE"
	NodeUse                 ScopeType = "NODE_USE"
	NodeOperate             ScopeType = "NODE_OPERATE"
	NodeManage              ScopeType = "NODE_MANAGE"
	TaskOperateByDataSource ScopeType = "TASK_OPERATE_BY_DATA_SOURCE"
	TemplateUse             ScopeType = "TEMPLATE_USE"
	TemplateManage          ScopeType = "TEMPLATE_MANAGE"
)

// ObjectScope 只绑定不可变对象标识，不用显示名称、路径或通配符定位对象。
type ObjectScope struct {
	Type     ScopeType `json:"type"`
	ObjectID string    `json:"objectId"`
}

// Assignment 是完整授权配置；空范围合法，但不代表任意对象权限。
type Assignment struct {
	Roles        []Role        `json:"roles"`
	Capabilities []Capability  `json:"capabilities"`
	Scopes       []ObjectScope `json:"scopes"`
}

// ErrInvalid 表示配置不满足固定枚举、依赖或有界输入要求，不携带用户原值。
var ErrInvalid = errors.New("授权配置无效")

// ErrInactiveAddition 表示停用或未知主体只能撤销既有授权，不能新增授权。
var ErrInactiveAddition = errors.New("当前账号状态不允许新增授权")

// Validate 验证完整替换配置；角色依赖缺失时拒绝潜伏授权，调用方不得静默补全角色。
func Validate(value Assignment) error {
	if len(value.Roles) > 5 || len(value.Capabilities) > 5 || len(value.Scopes) > 200 {
		return ErrInvalid
	}
	seenRoles := map[Role]bool{}
	for _, role := range value.Roles {
		if !slices.Contains([]Role{BasicUser, Operator, DataSourceAdmin, NodeAdmin, SystemAdmin}, role) || seenRoles[role] {
			return ErrInvalid
		}
		seenRoles[role] = true
	}
	seenCapabilities := map[Capability]bool{}
	for _, capability := range value.Capabilities {
		if !slices.Contains([]Capability{ProductionTask, ExpertConfig, SensitiveCommand, AuditLog, GlobalSchedulingLog}, capability) || seenCapabilities[capability] {
			return ErrInvalid
		}
		if capability == ExpertConfig && !value.HasRole(BasicUser) {
			return ErrInvalid
		}
		if capability == ProductionTask || capability == SensitiveCommand {
			if !value.HasRole(BasicUser) && !value.HasRole(Operator) {
				return ErrInvalid
			}
		}
		seenCapabilities[capability] = true
	}
	seenScopes := map[ObjectScope]bool{}
	for _, scope := range value.Scopes {
		if !validID(scope.ObjectID) || seenScopes[scope] || !scopeRole(value, scope.Type) {
			return ErrInvalid
		}
		seenScopes[scope] = true
	}
	return nil
}

func validID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 200 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || strings.ContainsRune("*?[]/\\", r) {
			return false
		}
	}
	return true
}

func scopeRole(value Assignment, kind ScopeType) bool {
	switch kind {
	case DataSourceUse, TemplateUse:
		return value.HasRole(BasicUser)
	case NodeUse:
		// 数据源管理员还需选择节点进行固定连接诊断；数据源使用范围仍单独要求任务职责。
		return value.HasRole(BasicUser) || value.HasRole(DataSourceAdmin)
	case DataSourceManage:
		return value.HasRole(DataSourceAdmin)
	case NodeManage:
		return value.HasRole(NodeAdmin)
	case NodeOperate, TaskOperateByDataSource, TemplateManage:
		return value.HasRole(Operator)
	default:
		return false
	}
}

// ValidateChange 对状态异常主体只允许集合收缩；身份状态与当前版本必须在保存事务中重读。
func ValidateChange(before, after Assignment, accountStatus string) error {
	if err := Validate(after); err != nil {
		return err
	}
	if accountStatus == "ACTIVE" {
		return nil
	}
	for _, role := range after.Roles {
		if !slices.Contains(before.Roles, role) {
			return ErrInactiveAddition
		}
	}
	for _, capability := range after.Capabilities {
		if !slices.Contains(before.Capabilities, capability) {
			return ErrInactiveAddition
		}
	}
	for _, scope := range after.Scopes {
		if !slices.Contains(before.Scopes, scope) {
			return ErrInactiveAddition
		}
	}
	return nil
}

// HasRole 仅判断显式角色，不将角色名称解释为对象范围或任意权限。
func (value Assignment) HasRole(role Role) bool { return slices.Contains(value.Roles, role) }

// HasScope 同时要求依赖角色与精确对象，空范围和未知类型均拒绝。
func (value Assignment) HasScope(kind ScopeType, objectID string) bool {
	return validID(objectID) && scopeRole(value, kind) && slices.Contains(value.Scopes, ObjectScope{kind, objectID})
}

// HasCapability 计算固定角色固有能力和显式独立能力，不替代业务安全条件。
func (value Assignment) HasCapability(capability Capability) bool {
	switch capability {
	case AuditLog:
		// 日志契约要求显式独立授权，设置管理职责和初始身份均不能隐含授予。
		return slices.Contains(value.Capabilities, AuditLog)
	case GlobalSchedulingLog:
		return value.HasRole(Operator) || slices.Contains(value.Capabilities, GlobalSchedulingLog)
	case ExpertConfig:
		return value.HasRole(BasicUser) && slices.Contains(value.Capabilities, capability)
	case ProductionTask, SensitiveCommand:
		return (value.HasRole(BasicUser) || value.HasRole(Operator)) && slices.Contains(value.Capabilities, capability)
	default:
		return false
	}
}

// CanCreateTask 只判断固定任务职责，仍须分别校验类型、数据源、节点和预检查。
func (value Assignment) CanCreateTask() bool {
	return value.HasRole(BasicUser)
}

// CanReadTask 的本人规则与数据源运维范围独立，不从节点或数据源管理权推导任务权限。
func (value Assignment) CanReadTask(subjectID, creatorID, dataSourceID string) bool {
	return (subjectID != "" && creatorID == subjectID && value.CanCreateTask()) || value.HasScope(TaskOperateByDataSource, dataSourceID)
}

// Canonical 复制并稳定排序配置，供确定性摘要、比较和重放使用，不修改调用方数组。
func Canonical(value Assignment) Assignment {
	result := Assignment{Roles: append([]Role{}, value.Roles...), Capabilities: append([]Capability{}, value.Capabilities...), Scopes: append([]ObjectScope{}, value.Scopes...)}
	slices.Sort(result.Roles)
	slices.Sort(result.Capabilities)
	slices.SortFunc(result.Scopes, func(a, b ObjectScope) int {
		if a.Type != b.Type {
			return strings.Compare(string(a.Type), string(b.Type))
		}
		return strings.Compare(a.ObjectID, b.ObjectID)
	})
	return result
}
