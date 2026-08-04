// Package precheckcontract 定义控制面与 Agent 共享的固定预检查结果契约。
// 它只表达六项检查、受控状态和有限证据码，不包含路径、SQL、命令或秘密。
package precheckcontract

var fixedChecks = []string{
	"DATABASE_CONNECTIVITY",
	"OBJECT_ACCESS",
	"TOOL_ENVIRONMENT",
	"OUTPUT_PATH",
	"OUTPUT_EMPTY",
	"AVAILABLE_SPACE",
}

var allowedEvidenceCodes = map[string]map[string]map[string]struct{}{
	"DATABASE_CONNECTIVITY": {
		"PASSED":  {"SYNTHETIC_OK": {}, "DATABASE_CONNECTED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "DATABASE_CONNECTION_FAILED": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "DATABASE_CONNECTION_UNAVAILABLE": {}},
	},
	"OBJECT_ACCESS": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OBJECT_ACCESSIBLE": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OBJECT_NOT_ACCESSIBLE": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OBJECT_ACCESS_UNAVAILABLE": {}},
	},
	"TOOL_ENVIRONMENT": {
		"PASSED":  {"SYNTHETIC_OK": {}, "TOOL_RUNTIME_READY": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "TOOL_RUNTIME_INVALID": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "TOOL_RUNTIME_UNAVAILABLE": {}},
	},
	"OUTPUT_PATH": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_WRITABLE": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_NOT_WRITABLE": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OUTPUT_PATH_UNAVAILABLE": {}},
	},
	"OUTPUT_EMPTY": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_EMPTY": {}, "OUTPUT_EMPTY_CHECK_SKIPPED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_NOT_EMPTY": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OUTPUT_PATH_UNAVAILABLE": {}},
	},
	"AVAILABLE_SPACE": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OUTPUT_SPACE_SUFFICIENT": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OUTPUT_SPACE_INSUFFICIENT": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OUTPUT_SPACE_UNAVAILABLE": {}},
	},
}

// FixedChecks 返回固定检查清单的副本，调用方不能借由修改切片增加检查范围。
func FixedChecks() []string {
	return append([]string(nil), fixedChecks...)
}

// ValidResult 只接受固定检查与状态组合的受控证据码。
// 新证据码必须先在此契约、协议 schema 和负例测试中共同增加，不能由 Agent 自由上传。
func ValidResult(check, status, evidenceCode string) bool {
	statuses, found := allowedEvidenceCodes[check]
	if !found {
		return false
	}
	codes, found := statuses[status]
	if !found {
		return false
	}
	_, found = codes[evidenceCode]
	return found
}
