// Package precheckcontract 定义控制面与 Agent 共享的固定预检查结果契约。
// 它只表达固定检查、受控状态和有限证据码，不包含路径、SQL、命令或秘密。
//
// 当前本地清单为七项，对象存储清单为九项；未授权或不可用的检查保持 UNKNOWN。
// 存储认证与写权限真实探测归 EX-V1，不能用连通性事实替代。
package precheckcontract

const (
	// EvidenceBlockedByTool 表示工具环境未通过，数据库相关检查未执行。
	EvidenceBlockedByTool = "PRECHECK_BLOCKED_BY_TOOL"
	// EvidenceBlockedByDatabase 表示数据库连接未通过，对象检查未执行。
	EvidenceBlockedByDatabase = "PRECHECK_BLOCKED_BY_DATABASE"
	// EvidenceBlockedByStorageConnectivity 表示存储端点连通性未通过，凭据检查未执行。
	EvidenceBlockedByStorageConnectivity = "PRECHECK_BLOCKED_BY_STORAGE_CONNECTIVITY"
	// EvidenceBlockedByStorageAuth 表示存储鉴权未通过，未执行写权限检查。
	EvidenceBlockedByStorageAuth = "PRECHECK_BLOCKED_BY_STORAGE_AUTH"
	// EvidenceBlockedByOutputPath 表示输出目录无法安全定位，依赖目录的检查未执行。
	EvidenceBlockedByOutputPath = "PRECHECK_BLOCKED_BY_OUTPUT_PATH"
)

var fixedChecks = []string{
	"DATABASE_CONNECTIVITY",
	"OBJECT_ACCESS",
	"TOOL_ENVIRONMENT",
	"NODE_RESOURCE",
	"OUTPUT_PATH",
	"OUTPUT_EMPTY",
	"AVAILABLE_SPACE",
}

// storageChecks 固定按连通性、认证、写权限排列，任何非通过状态都阻断提交。
var storageChecks = []string{
	"STORAGE_CONNECTIVITY",
	"STORAGE_AUTH",
	"STORAGE_WRITE",
}

var allowedEvidenceCodes = map[string]map[string]map[string]struct{}{
	"DATABASE_CONNECTIVITY": {
		"PASSED":  {"SYNTHETIC_OK": {}, "DATABASE_CONNECTED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "DATABASE_CONNECTION_FAILED": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "DATABASE_CONNECTION_UNAVAILABLE": {}, EvidenceBlockedByTool: {}},
	},
	"OBJECT_ACCESS": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OBJECT_ACCESSIBLE": {}, "PARAMETER_REQUIREMENTS_VERIFIED": {}, "OBJECT_SELECTION_VERIFIED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OBJECT_NOT_ACCESSIBLE": {}, "PARAMETER_REQUIREMENTS_REJECTED": {}, "OBJECT_SELECTION_REJECTED": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OBJECT_ACCESS_UNAVAILABLE": {}, EvidenceBlockedByTool: {}, EvidenceBlockedByDatabase: {}},
	},
	"TOOL_ENVIRONMENT": {
		"PASSED":  {"SYNTHETIC_OK": {}, "TOOL_RUNTIME_READY": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "TOOL_RUNTIME_INVALID": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "TOOL_RUNTIME_UNAVAILABLE": {}},
	},
	"NODE_RESOURCE": {
		"PASSED":  {"SYNTHETIC_OK": {}, "NODE_RESOURCE_READY": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "NODE_RESOURCE_INSUFFICIENT": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "NODE_RESOURCE_UNAVAILABLE": {}},
	},
	"OUTPUT_PATH": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_WRITABLE": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_NOT_WRITABLE": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OUTPUT_PATH_UNAVAILABLE": {}},
	},
	"OUTPUT_EMPTY": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_EMPTY": {}, "OUTPUT_EMPTY_CHECK_SKIPPED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OUTPUT_PATH_NOT_EMPTY": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OUTPUT_PATH_UNAVAILABLE": {}, EvidenceBlockedByOutputPath: {}},
	},
	"AVAILABLE_SPACE": {
		"PASSED":  {"SYNTHETIC_OK": {}, "OUTPUT_SPACE_SUFFICIENT": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "OUTPUT_SPACE_INSUFFICIENT": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "OUTPUT_SPACE_UNAVAILABLE": {}, EvidenceBlockedByOutputPath: {}},
	},
	// EX-I6 存储专用预检查：证据码只表达受控结论，不携带端点、凭据或云服务错误原文。
	"STORAGE_CONNECTIVITY": {
		"PASSED":  {"SYNTHETIC_OK": {}, "STORAGE_ENDPOINT_REACHABLE": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "STORAGE_ENDPOINT_UNREACHABLE": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "STORAGE_CONNECTIVITY_UNAVAILABLE": {}},
	},
	"STORAGE_AUTH": {
		"PASSED":  {"SYNTHETIC_OK": {}, "STORAGE_CREDENTIAL_VERIFIED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "STORAGE_CREDENTIAL_REJECTED": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "STORAGE_AUTH_UNAVAILABLE": {}, EvidenceBlockedByStorageConnectivity: {}},
	},
	"STORAGE_WRITE": {
		"PASSED":  {"SYNTHETIC_OK": {}, "STORAGE_WRITE_VERIFIED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "STORAGE_WRITE_REJECTED": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "STORAGE_WRITE_UNAVAILABLE": {}, "STORAGE_WRITE_CLEANUP_UNCONFIRMED": {}, EvidenceBlockedByStorageAuth: {}},
	},
}

// StorageOutputChecks 返回对象存储的固定清单，辅助目录、资源和写权限均不可省略。
func StorageOutputChecks() []string {
	return append([]string{"DATABASE_CONNECTIVITY", "OBJECT_ACCESS", "TOOL_ENVIRONMENT", "NODE_RESOURCE", "OUTPUT_PATH", "AVAILABLE_SPACE"}, storageChecks...)
}

// LegacyChecks 仅用于还原已持久化的历史六项报告，不允许新的完成回执使用旧清单。
func LegacyChecks(storage bool) []string {
	if storage {
		return []string{"DATABASE_CONNECTIVITY", "OBJECT_ACCESS", "TOOL_ENVIRONMENT", "AVAILABLE_SPACE", "STORAGE_CONNECTIVITY", "STORAGE_AUTH"}
	}
	return []string{"DATABASE_CONNECTIVITY", "OBJECT_ACCESS", "TOOL_ENVIRONMENT", "OUTPUT_PATH", "OUTPUT_EMPTY", "AVAILABLE_SPACE"}
}

// FixedChecks 返回当前本地检查清单副本，调用方不能修改全局范围。
func FixedChecks() []string {
	return append([]string(nil), fixedChecks...)
}

// StorageChecks 返回对象存储专用检查清单的副本。
func StorageChecks() []string {
	return append([]string(nil), storageChecks...)
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
