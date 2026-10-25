package precheckcontract

import "testing"

func Test旁路结构原因不得跨检查项或状态使用(t *testing.T) {
	for _, code := range []string{"DIRECT_LOAD_TABLE_NOT_FOUND", "DIRECT_LOAD_BIT_BLOCKED", "DIRECT_LOAD_BINARY_BLOCKED", "DIRECT_LOAD_VIRTUAL_BLOCKED", "DIRECT_LOAD_REPLACE_INDEX_BLOCKED", "DIRECT_LOAD_REPLACE_LOB_BLOCKED"} {
		if !ValidDirectLoadResult("DIRECT_LOAD_STRUCTURE", "FAILED", code) || ValidDirectLoadResult("WRITE_ACCESS", "FAILED", code) || ValidDirectLoadResult("DIRECT_LOAD_STRUCTURE", "PASSED", code) || ValidDirectLoadResult("DIRECT_LOAD_STRUCTURE", "UNKNOWN", code) {
			t.Fatal("结构原因的类型或状态边界错误", code)
		}
	}
	if !ValidDirectLoadResult("DIRECT_LOAD_STRUCTURE", "UNKNOWN", "DIRECT_LOAD_TABLE_FACT_UNAVAILABLE") || ValidDirectLoadResult("DIRECT_LOAD_STRUCTURE", "FAILED", "DIRECT_LOAD_TABLE_FACT_UNAVAILABLE") || ValidDirectLoadResult("DATABASE_CONNECTIVITY", "UNKNOWN", "DIRECT_LOAD_TABLE_FACT_UNAVAILABLE") || ValidDirectLoadResult("DIRECT_LOAD_STRUCTURE", "FAILED", "UNRECOGNIZED") {
		t.Fatal("未知事实被扩大为失败、其他检查事实或未知码")
	}
}
