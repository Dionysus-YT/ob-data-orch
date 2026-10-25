package contracts

import "testing"

func Test权限契约只提供投影与原子授权不创建身份(t *testing.T) {
	spec := loadOpenAPI(t)
	schemas := object(t, object(t, spec, "components"), "schemas")
	for _, name := range []string{"PermissionSubject", "PermissionAssignment", "PermissionScope", "PermissionScopeObject", "PermissionProfile", "PermissionWrite", "PermissionCandidate", "PermissionChangeResult"} {
		schema := object(t, schemas, name)
		if schema["additionalProperties"] != false {
			t.Fatalf("%s 必须拒绝未知字段", name)
		}
	}
	write := object(t, schemas, "PermissionWrite")
	required := map[string]bool{}
	for _, field := range write["required"].([]any) {
		required[field.(string)] = true
	}
	for _, field := range []string{"roles", "capabilities", "scopes", "reason", "confirmed"} {
		if !required[field] {
			t.Fatal("完整替换缺少字段", field)
		}
	}
	paths := object(t, spec, "paths")
	for _, path := range []string{"/api/v1/access-control/subjects", "/api/v1/access-control/subjects/{subjectId}", "/api/v1/access-control/candidates"} {
		item := object(t, paths, path)
		if _, ok := item["post"]; ok {
			t.Fatal("权限接口不得创建认证账号", path)
		}
	}
	operation := object(t, object(t, paths, "/api/v1/access-control/subjects/{subjectId}"), "patch")
	if operation["x-optimistic-lock"] != "REQUIRED" {
		t.Fatal("缺少版本前置条件")
	}
}
