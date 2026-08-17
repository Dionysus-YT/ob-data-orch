// Package precheckcontract 定义控制面与 Agent 共享的固定预检查结果契约。
// 它只表达固定检查、受控状态和有限证据码，不包含路径、SQL、命令或秘密。
//
// EX-I6 存储专用预检查（2026-08-14）：在冻结六项本地检查之外新增两项存储层检查——
// STORAGE_CONNECTIVITY（受控 URI 端点的网络可达性）与 STORAGE_AUTH（对象存储凭据有效性）。
// 本地输出任务仍使用冻结六项；对象存储输出任务使用存储形态清单（见 agentpreflight.ChecksForOutputKind）。
// 两项存储检查默认为 UNKNOWN（探测未授权/不可用），提交门禁按结果失败关闭；真实取证归 EX-V1。
package precheckcontract

var fixedChecks = []string{
	"DATABASE_CONNECTIVITY",
	"OBJECT_ACCESS",
	"TOOL_ENVIRONMENT",
	"OUTPUT_PATH",
	"OUTPUT_EMPTY",
	"AVAILABLE_SPACE",
}

// storageChecks 是对象存储输出任务在六项基础检查之外的存储专用检查。
// 顺序固定：先网络可达性（无凭据），后凭据有效性；任何一项 UNKNOWN/FAILED 都阻断提交。
var storageChecks = []string{
	"STORAGE_CONNECTIVITY",
	"STORAGE_AUTH",
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
	// EX-I6 存储专用预检查：证据码只表达受控结论，不携带端点、凭据或云服务错误原文。
	"STORAGE_CONNECTIVITY": {
		"PASSED":  {"SYNTHETIC_OK": {}, "STORAGE_ENDPOINT_REACHABLE": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "STORAGE_ENDPOINT_UNREACHABLE": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "STORAGE_CONNECTIVITY_UNAVAILABLE": {}},
	},
	"STORAGE_AUTH": {
		"PASSED":  {"SYNTHETIC_OK": {}, "STORAGE_CREDENTIAL_VERIFIED": {}},
		"FAILED":  {"SYNTHETIC_OK": {}, "STORAGE_CREDENTIAL_REJECTED": {}},
		"UNKNOWN": {"SYNTHETIC_OK": {}, "STORAGE_AUTH_UNAVAILABLE": {}},
	},
}

// FixedChecks 返回固定六项本地检查清单的副本，调用方不能借由修改切片增加检查范围。
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
