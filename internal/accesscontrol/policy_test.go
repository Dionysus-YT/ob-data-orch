package accesscontrol

import (
	"errors"
	"reflect"
	"testing"
)

func Test固定角色与精确对象范围(t *testing.T) {
	admin := Assignment{Roles: []Role{SystemAdmin}}
	if Validate(admin) != nil || admin.HasCapability(AuditLog) || admin.CanCreateTask() || admin.HasScope(DataSourceUse, "source-1") || admin.CanReadTask("admin", "other", "source-1") {
		t.Fatal("系统管理不能隐式取得业务权限")
	}
	value := Assignment{Roles: []Role{BasicUser, DataSourceAdmin, NodeAdmin}, Scopes: []ObjectScope{{DataSourceUse, "source-1"}, {DataSourceManage, "source-2"}, {NodeManage, "node-1"}}}
	if Validate(value) != nil || !value.HasScope(DataSourceUse, "source-1") || value.HasScope(DataSourceManage, "source-1") || value.HasScope(DataSourceUse, "source-2") || value.HasScope(NodeUse, "node-1") || value.HasScope(DataSourceUse, "source-3") {
		t.Fatal("使用、管理和范围必须独立")
	}
	if !value.CanReadTask("user-1", "user-1", "source-9") || value.CanReadTask("user-1", "user-2", "source-1") {
		t.Fatal("本人规则不扩大到数据源上其他任务")
	}
	operator := Assignment{Roles: []Role{Operator}, Scopes: []ObjectScope{{TaskOperateByDataSource, "source-1"}, {NodeOperate, "node-1"}}}
	if Validate(operator) != nil || operator.CanCreateTask() || !operator.CanReadTask("user-1", "user-2", "source-1") || operator.HasScope(DataSourceUse, "source-1") || operator.HasScope(NodeManage, "node-1") {
		t.Fatal("任务运维和节点运行操作不得扩大权限")
	}
}

func Test授权配置拒绝潜伏或未知权限(t *testing.T) {
	for _, value := range []Assignment{
		{Roles: []Role{"ROLE_UNKNOWN"}}, {Roles: []Role{BasicUser, BasicUser}},
		{Capabilities: []Capability{"CAP_UNKNOWN"}}, {Capabilities: []Capability{ProductionTask}},
		{Roles: []Role{BasicUser}, Capabilities: []Capability{AuditLog, AuditLog}},
		{Scopes: []ObjectScope{{DataSourceUse, "source-1"}}},
		{Roles: []Role{BasicUser}, Scopes: []ObjectScope{{DataSourceUse, "*"}}},
		{Roles: []Role{BasicUser}, Scopes: []ObjectScope{{DataSourceUse, " source-1"}}},
		{Roles: []Role{BasicUser}, Scopes: []ObjectScope{{DataSourceUse, "source\n1"}}},
		{Roles: []Role{BasicUser}, Scopes: []ObjectScope{{"UNKNOWN", "source-1"}}},
		{Roles: []Role{BasicUser}, Scopes: []ObjectScope{{DataSourceUse, "source-1"}, {DataSourceUse, "source-1"}}},
	} {
		if !errors.Is(Validate(value), ErrInvalid) {
			t.Fatalf("必须拒绝非法配置：%#v", value)
		}
	}
	if err := Validate(Assignment{Roles: []Role{BasicUser}}); err != nil {
		t.Fatal("允许保存明确标为不生效的空范围角色")
	}
}

func Test停用与未知主体只能撤销(t *testing.T) {
	before := Assignment{Roles: []Role{BasicUser, Operator}, Capabilities: []Capability{ProductionTask}, Scopes: []ObjectScope{{DataSourceUse, "source-1"}}}
	for _, status := range []string{"DISABLED", "UNKNOWN", ""} {
		if ValidateChange(before, Assignment{Roles: []Role{BasicUser}}, status) != nil {
			t.Fatal("异常身份仍可撤销")
		}
		for _, after := range []Assignment{
			{Roles: []Role{BasicUser, NodeAdmin}},
			{Roles: []Role{BasicUser}, Capabilities: []Capability{ExpertConfig}},
			{Roles: []Role{BasicUser}, Scopes: []ObjectScope{{DataSourceUse, "source-2"}}},
		} {
			if !errors.Is(ValidateChange(before, after, status), ErrInactiveAddition) {
				t.Fatal("异常身份不得新增授权")
			}
		}
	}
}

func Test规范化不会修改输入或借未知能力放行(t *testing.T) {
	input := Assignment{Roles: []Role{Operator, BasicUser}, Scopes: []ObjectScope{{NodeUse, "node-1"}, {DataSourceUse, "source-2"}}}
	before := append([]Role{}, input.Roles...)
	result := Canonical(input)
	if !reflect.DeepEqual(input.Roles, before) || result.Roles[0] != BasicUser || input.HasCapability("constructor") || input.HasScope("UNKNOWN", "source-1") {
		t.Fatal("规范化或未知成员处理错误")
	}
	if len(Canonical(Assignment{}).Roles) != 0 || Canonical(Assignment{}).Roles == nil {
		t.Fatal("空授权应序列化为空集合")
	}
}
