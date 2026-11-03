package agentwire

import (
	"testing"

	"ob-data-orch/internal/exportobject"
)

func TestOracle对象信封保留类型和模式边界(t *testing.T) {
	input := validStoragePrecheckGrantForTest().Context
	input.CompatibilityMode, input.ContentKind = "ORACLE", "DDL_ONLY"
	input.ObjectTypes, input.Objects = exportobject.Types(), nil
	for range input.ObjectTypes {
		input.Objects = append(input.Objects, "synthetic_same_name")
	}
	if !validPrecheckExecutionContext(input) {
		t.Fatal("Oracle 十类对象信封被拒绝")
	}
	input.CompatibilityMode = "MYSQL"
	if validPrecheckExecutionContext(input) {
		t.Fatal("信封允许跨模式对象")
	}
	input.CompatibilityMode, input.ContentKind = "ORACLE", "DATA_ONLY"
	if validPrecheckExecutionContext(input) {
		t.Fatal("信封允许非表数据导出")
	}
}
