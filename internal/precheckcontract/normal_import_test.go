package precheckcontract

import "testing"

func Test普通导入拒绝与未知证据不能升级为通过或跨检查使用(t *testing.T) {
	for _, item := range []struct{ check, status, code string }{
		{"WRITE_ACCESS", "FAILED", "IMPORT_WRITE_PERMISSION_REJECTED"},
		{"WRITE_ACCESS", "UNKNOWN", "IMPORT_WRITE_PERMISSION_UNAVAILABLE"},
		{"INPUT_MAPPING", "FAILED", "IMPORT_INPUT_MAPPING_MISMATCH"},
		{"INPUT_MAPPING", "FAILED", "IMPORT_CSV_WHITESPACE_UNQUOTED"},
		{"INPUT_MAPPING", "UNKNOWN", "IMPORT_INPUT_MAPPING_UNAVAILABLE"},
		{"INPUT_MAPPING", "UNKNOWN", "IMPORT_INPUT_MAPPING_UNSUPPORTED"},
		{"NODE_RESOURCE", "FAILED", EvidenceImportJVMHeapTooSmall},
	} {
		if !ValidResult(item.check, item.status, item.code) || ValidResult(item.check, "PASSED", item.code) || ValidResult("INPUT_FILES", item.status, item.code) {
			t.Fatal("普通导入证据状态或用途白名单不正确")
		}
	}
}

func Test普通导入细化原因只允许绑定各自非通过状态(t *testing.T) {
	for _, check := range []string{"WRITE_ACCESS", "INPUT_MAPPING"} {
		for _, status := range []string{"FAILED", "UNKNOWN"} {
			for code := range allowedEvidenceCodes[check][status] {
				if code == "SYNTHETIC_OK" {
					continue
				}
				if ValidResult(check, "PASSED", code) || ValidResult("INPUT_FILES", status, code) && code != "IMPORT_REJECTED" && code != "IMPORT_UNAVAILABLE" && code != EvidenceBlockedByTool && code != EvidenceBlockedByDatabase {
					t.Fatal("细化原因越过状态或用途边界")
				}
			}
		}
	}
}
