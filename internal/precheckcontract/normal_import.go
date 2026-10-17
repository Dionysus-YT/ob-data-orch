package precheckcontract

// EvidenceNormalImportProbeNotConfigured 表示普通导入专用检查未装配，不等同于目标连接或权限失败。
const EvidenceNormalImportProbeNotConfigured = "NORMAL_IMPORT_PROBE_NOT_CONFIGURED"

// EvidenceImportJVMHeapTooSmall 表示数据导入堆低于固定工具下限，与节点额度不足分开呈现。
const EvidenceImportJVMHeapTooSmall = "IMPORT_JVM_HEAP_TOO_SMALL"

// NormalImportChecks 返回独立导入检查清单；文件发现不能替代完整预检查。
func NormalImportChecks(kind string) []string {
	if kind == "NORMAL_IMPORT_FILES" {
		return []string{"INPUT_FILES"}
	}
	if kind == "NORMAL_IMPORT_PREFLIGHT" {
		return []string{"DATABASE_CONNECTIVITY", "WRITE_ACCESS", "TOOL_ENVIRONMENT", "NODE_RESOURCE", "INPUT_FILES", "INPUT_MAPPING"}
	}
	return nil
}

func init() {
	allowedEvidenceCodes["NODE_RESOURCE"]["FAILED"][EvidenceImportJVMHeapTooSmall] = struct{}{}
	allowedEvidenceCodes["DATABASE_CONNECTIVITY"]["UNKNOWN"][EvidenceNormalImportProbeNotConfigured] = struct{}{}
	for _, check := range []string{"INPUT_FILES", "INPUT_MAPPING", "WRITE_ACCESS"} {
		allowedEvidenceCodes[check] = map[string]map[string]struct{}{
			"PASSED":  {"SYNTHETIC_OK": {}, "IMPORT_VERIFIED": {}},
			"FAILED":  {"SYNTHETIC_OK": {}, "IMPORT_REJECTED": {}},
			"UNKNOWN": {"SYNTHETIC_OK": {}, "IMPORT_UNAVAILABLE": {}, EvidenceBlockedByTool: {}, EvidenceBlockedByDatabase: {}},
		}
		if check != "INPUT_FILES" {
			allowedEvidenceCodes[check]["UNKNOWN"][EvidenceNormalImportProbeNotConfigured] = struct{}{}
		}
	}
	allowedEvidenceCodes["WRITE_ACCESS"]["FAILED"]["IMPORT_WRITE_PERMISSION_REJECTED"] = struct{}{}
	allowedEvidenceCodes["WRITE_ACCESS"]["UNKNOWN"]["IMPORT_WRITE_PERMISSION_UNAVAILABLE"] = struct{}{}
	allowedEvidenceCodes["INPUT_MAPPING"]["FAILED"]["IMPORT_INPUT_MAPPING_MISMATCH"] = struct{}{}
	for _, code := range []string{"IMPORT_INPUT_MAPPING_UNAVAILABLE", "IMPORT_INPUT_MAPPING_UNSUPPORTED"} {
		allowedEvidenceCodes["INPUT_MAPPING"]["UNKNOWN"][code] = struct{}{}
	}
	for _, code := range []string{"IMPORT_WRITE_INSERT_REQUIRED", "IMPORT_WRITE_UPDATE_REQUIRED", "IMPORT_WRITE_DELETE_REQUIRED", "IMPORT_WRITE_CREATE_REQUIRED", "IMPORT_WRITE_ALTER_REQUIRED", "IMPORT_WRITE_DROP_REQUIRED"} {
		allowedEvidenceCodes["WRITE_ACCESS"]["FAILED"][code] = struct{}{}
	}
	for _, code := range []string{"IMPORT_WRITE_GRANTS_UNAVAILABLE", "IMPORT_WRITE_GRANTS_UNSUPPORTED", "IMPORT_WRITE_DDL_UNSUPPORTED", "IMPORT_WRITE_INPUT_UNAVAILABLE", "IMPORT_WRITE_SCHEMA_UNAVAILABLE", "IMPORT_WRITE_CLEANUP_SCOPE_UNAVAILABLE"} {
		allowedEvidenceCodes["WRITE_ACCESS"]["UNKNOWN"][code] = struct{}{}
	}
	for _, code := range []string{"IMPORT_COLUMN_COUNT_MISMATCH", "IMPORT_COLUMN_NAME_MISMATCH", "IMPORT_REQUIRED_COLUMN_MISSING"} {
		allowedEvidenceCodes["INPUT_MAPPING"]["FAILED"][code] = struct{}{}
	}
	for _, code := range []string{"IMPORT_TARGET_METADATA_UNAVAILABLE", "IMPORT_TARGET_STRUCTURE_MISSING", "IMPORT_INPUT_DEFINITION_UNSUPPORTED", "IMPORT_AUTO_COLUMNS_UNSUPPORTED", "IMPORT_INPUT_FILES_CHANGED"} {
		allowedEvidenceCodes["INPUT_MAPPING"]["UNKNOWN"][code] = struct{}{}
	}
}
