//go:build !windows

package agentexec

import (
	"strings"
	"unicode/utf8"
)

// toolArgumentsRepresentable 在非 Windows 平台只保留原有 UTF-8 与 NUL 边界，不外推原生工具支持。
func toolArgumentsRepresentable(arguments []string) bool {
	for _, argument := range arguments {
		if !utf8.ValidString(argument) || strings.IndexByte(argument, 0) >= 0 {
			return false
		}
	}
	return true
}
