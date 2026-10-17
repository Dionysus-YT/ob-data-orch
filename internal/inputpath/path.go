// Package inputpath 定义普通导入原样传递的本地路径语法，不套用导出的 Hadoop 路径转换。
package inputpath

import "strings"

// Valid 检查匹配节点平台的本地绝对路径；存在性、允许根目录和链接由 Agent 另行核对。
func Valid(platform, value string) bool {
	if value == "" || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	switch platform {
	case "WINDOWS_AMD64":
		return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '/' || value[2] == '\\') && !strings.ContainsAny(value[2:], `<>":|?*`)
	case "LINUX_AMD64", "LINUX_ARM64":
		return strings.HasPrefix(value, "/")
	default:
		return false
	}
}
