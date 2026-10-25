package exportdomain

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"ob-data-orch/internal/store"
)

// ValidateEditingConfig 只校验受控草稿的结构边界，允许尚未填写的必填项。
// 此结果不表示参数、路径、对象或权限已通过检查；执行前仍须生成完整配置。
func ValidateEditingConfig(config *store.ExportConfig) error {
	if config == nil || !editingOneOf(config.ObjectScope.ScopeKind, "ALL", "SPECIFIED", "QUERY_RESULT") ||
		!editingOneOf(config.ContentSelection.ContentKind, "", "DATA_ONLY", "DDL_ONLY", "DDL_AND_DATA") ||
		!editingOneOf(config.DataFormat.FormatKind, "", "CSV", "CUT", "SQL") ||
		!editingOneOf(config.OutputConfig.OutputKind, "LOCAL", "OSS", "S3", "COS", "OBS") || config.PerformanceConfig.Retry || config.OutputConfig.ControlFilePath != "" {
		return errors.New("导出编辑草稿结构无效")
	}
	for _, kind := range config.ObjectScope.ObjectTypes {
		if !editingOneOf(kind, "TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE") {
			return errors.New("导出编辑草稿对象类型无效")
		}
	}
	for _, expression := range config.ObjectScope.Expressions {
		if expression.RawInput != "" || !editingOneOf(expression.ObjectType, "", "TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE") {
			return errors.New("导出编辑草稿对象表达无效")
		}
	}
	if config.OutputConfig.OutputKind == "LOCAL" && config.OutputConfig.StorageLocationDraft != nil {
		return errors.New("本地输出不能保存远端位置")
	}
	if config.OutputConfig.OutputKind != "LOCAL" && !safeEditingStorage(config.OutputConfig) {
		return errors.New("编辑中的存储位置包含不允许的 URI 字段")
	}
	// 类型化配置排除秘密和任意选项；额外限制 NUL 与整体体积，避免编辑接口变成任意内容存储。
	encoded, err := json.Marshal(config)
	if err != nil || len(encoded) > 1<<20 || strings.Contains(string(encoded), `\u0000`) {
		return errors.New("导出编辑草稿内容无效")
	}
	return nil
}

// 缺失 Bucket 或路径可以稍后补齐；用户信息与密钥查询参数不能借编辑状态进入持久化。
func safeEditingStorage(output store.OutputConfig) bool {
	if location := output.StorageLocationDraft; location != nil {
		if strings.ContainsAny(location.Bucket, "@?#\r\n") || strings.ContainsAny(location.Path, "?#\r\n") || strings.ContainsAny(location.Endpoint+location.Region, "@?#&\r\n") {
			return false
		}
	}
	if output.FilePath == "" {
		return true
	}
	parsed, err := url.Parse(output.FilePath)
	if err != nil || !strings.EqualFold(parsed.Scheme, output.OutputKind) || parsed.User != nil || parsed.Fragment != "" || strings.ContainsAny(output.FilePath, "\r\n") {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	for key, values := range query {
		if key != "endpoint" && key != "region" && key != "storage-class" {
			return false
		}
		for _, value := range values {
			if strings.ContainsAny(value, "@?#&\r\n") {
				return false
			}
		}
	}
	return true
}

func editingOneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
