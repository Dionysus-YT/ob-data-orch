package precheckcontract

import (
	"reflect"
	"testing"
)

func TestFixedChecksReturnIndependentFixedOrder(t *testing.T) {
	want := []string{
		"DATABASE_CONNECTIVITY",
		"OBJECT_ACCESS",
		"TOOL_ENVIRONMENT",
		"OUTPUT_PATH",
		"OUTPUT_EMPTY",
		"AVAILABLE_SPACE",
	}
	checks := FixedChecks()
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("固定预检查清单 = %#v，期望 %#v", checks, want)
	}
	checks[0] = "ALTERED"
	if FixedChecks()[0] != want[0] {
		t.Fatal("FixedChecks() 返回的切片可修改全局固定检查清单")
	}
}

func TestValidResultRejectsEncodedOrMismatchedEvidence(t *testing.T) {
	tests := []struct {
		name     string
		check    string
		status   string
		evidence string
		want     bool
	}{
		{name: "合成通过", check: "OUTPUT_PATH", status: "PASSED", evidence: "SYNTHETIC_OK", want: true},
		{name: "数据库连接通过", check: "DATABASE_CONNECTIVITY", status: "PASSED", evidence: "DATABASE_CONNECTED", want: true},
		{name: "数据库连接失败", check: "DATABASE_CONNECTIVITY", status: "FAILED", evidence: "DATABASE_CONNECTION_FAILED", want: true},
		{name: "数据库不可用", check: "DATABASE_CONNECTIVITY", status: "UNKNOWN", evidence: "DATABASE_CONNECTION_UNAVAILABLE", want: true},
		{name: "对象可访问", check: "OBJECT_ACCESS", status: "PASSED", evidence: "OBJECT_ACCESSIBLE", want: true},
		{name: "对象不可访问", check: "OBJECT_ACCESS", status: "FAILED", evidence: "OBJECT_NOT_ACCESSIBLE", want: true},
		{name: "工具运行时无效", check: "TOOL_ENVIRONMENT", status: "FAILED", evidence: "TOOL_RUNTIME_INVALID", want: true},
		{name: "输出路径不可写", check: "OUTPUT_PATH", status: "FAILED", evidence: "OUTPUT_PATH_NOT_WRITABLE", want: true},
		{name: "输出目录非空", check: "OUTPUT_EMPTY", status: "FAILED", evidence: "OUTPUT_PATH_NOT_EMPTY", want: true},
		{name: "输出空间不足", check: "AVAILABLE_SPACE", status: "FAILED", evidence: "OUTPUT_SPACE_INSUFFICIENT", want: true},
		{name: "编码路径", check: "OUTPUT_PATH", status: "FAILED", evidence: "E_WORKSPACE_SECRET_ABC", want: false},
		{name: "状态与证据不匹配", check: "DATABASE_CONNECTIVITY", status: "FAILED", evidence: "DATABASE_CONNECTED", want: false},
		{name: "其他检查伪造数据库证据", check: "OBJECT_ACCESS", status: "FAILED", evidence: "DATABASE_CONNECTION_FAILED", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidResult(test.check, test.status, test.evidence); got != test.want {
				t.Fatalf("ValidResult(%q, %q, %q) = %t，期望 %t", test.check, test.status, test.evidence, got, test.want)
			}
		})
	}
}
