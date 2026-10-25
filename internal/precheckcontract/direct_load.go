package precheckcontract

// DirectLoadChecks 固定旁路独立检查集，文件发现不替代版本、RPC、结构与权限检查。
func DirectLoadChecks(kind string) []string {
	if kind == "DIRECT_LOAD_FILES" {
		return []string{"INPUT_FILES"}
	}
	if kind == "DIRECT_LOAD_PREFLIGHT" {
		return []string{"TOOL_ENVIRONMENT", "NODE_RESOURCE", "INPUT_FILES", "DATABASE_CONNECTIVITY", "RPC_CONNECTIVITY", "DIRECT_LOAD_VERSION", "WRITE_ACCESS", "DIRECT_LOAD_STRUCTURE", "DIRECT_LOAD_CONFIG"}
	}
	return nil
}

// ValidDirectLoadResult 限定旁路原因码，不能沿用普通导入、导出或模拟成功码证明旁路支持。
func ValidDirectLoadResult(check, status, code string) bool {
	found := false
	for _, name := range DirectLoadChecks("DIRECT_LOAD_PREFLIGHT") {
		found = found || name == check
	}
	if !found {
		return false
	}
	switch status {
	case "PASSED":
		return code == "DIRECT_LOAD_VERIFIED"
	case "FAILED":
		if code == "DIRECT_LOAD_REJECTED" {
			return true
		}
		return check == "DIRECT_LOAD_STRUCTURE" && oneOfDirectLoadCode(code, "DIRECT_LOAD_TABLE_NOT_FOUND", "DIRECT_LOAD_BIT_BLOCKED", "DIRECT_LOAD_BINARY_BLOCKED", "DIRECT_LOAD_VIRTUAL_BLOCKED", "DIRECT_LOAD_REPLACE_INDEX_BLOCKED", "DIRECT_LOAD_REPLACE_LOB_BLOCKED")
	case "UNKNOWN":
		return code == "DIRECT_LOAD_UNAVAILABLE" || code == "DIRECT_LOAD_PROBE_NOT_CONFIGURED" || code == EvidenceBlockedByTool || code == EvidenceBlockedByDatabase || check == "DIRECT_LOAD_STRUCTURE" && code == "DIRECT_LOAD_TABLE_FACT_UNAVAILABLE"
	default:
		return false
	}
}

func oneOfDirectLoadCode(code string, values ...string) bool {
	for _, value := range values {
		if code == value {
			return true
		}
	}
	return false
}
