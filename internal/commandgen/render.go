package commandgen

import "strings"

func renderCommand(platform Platform, program string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, program)
	for _, arg := range args {
		if platform == PlatformWindowsAMD64 {
			parts = append(parts, quoteWindowsDisplay(arg))
		} else {
			parts = append(parts, quoteLinuxDisplay(arg))
		}
	}
	return strings.Join(parts, " ")
}

func quoteWindowsDisplay(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\v\f\r\"") {
		return value
	}
	var result strings.Builder
	result.WriteByte('"')
	backslashes := 0
	for _, character := range value {
		if character == '\\' {
			backslashes++
			continue
		}
		if character == '"' {
			result.WriteString(strings.Repeat("\\", backslashes*2+1))
			result.WriteRune(character)
			backslashes = 0
			continue
		}
		result.WriteString(strings.Repeat("\\", backslashes))
		backslashes = 0
		result.WriteRune(character)
	}
	result.WriteString(strings.Repeat("\\", backslashes*2))
	result.WriteByte('"')
	return result.String()
}

func quoteLinuxDisplay(value string) string {
	if value != "" && isLinuxDisplaySafe(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func isLinuxDisplaySafe(value string) bool {
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			strings.ContainsRune("_@%+=:,./-", character) {
			continue
		}
		return false
	}
	return true
}
