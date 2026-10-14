package precheckcontract

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidSelectionIdentifiers 限定固定元数据检查的字面名称列表，拒绝通配符、重复项与控制字符。
// 名称可以包含数据库支持的 Unicode 字符，但只能被用作元数据值或字面比较。
func ValidSelectionIdentifiers(values []string) bool {
	if len(values) > 4096 {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsAny(value, "*,") || seen[value] {
			return false
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return false
			}
		}
		seen[value] = true
	}
	return true
}
