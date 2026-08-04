// Package outputpath 定义导出目录在控制面、Agent 协议与本机文件系统之间一致的安全路径语义。
// 它只处理固定导出目录，不能用于任意文件访问或跨平台路径转换。
package outputpath

import "strings"

const (
	// PlatformWindowsAMD64 是 Windows AMD64 执行节点的平台标识。
	PlatformWindowsAMD64 = "WINDOWS_AMD64"
	// PlatformLinuxAMD64 是 Linux AMD64 执行节点的平台标识。
	PlatformLinuxAMD64 = "LINUX_AMD64"
	// PlatformLinuxARM64 是 Linux ARM64 执行节点的平台标识。
	PlatformLinuxARM64 = "LINUX_ARM64"
)

// IsExportOutputPath 验证将原样传入 OBDUMPER 的导出目录。
// Windows 只接受 /E:/directory 形式，避免 OBDUMPER 4.3.5 将 E:\\directory 错拼为 Hadoop 文件 URI。
func IsExportOutputPath(platform, value string) bool {
	if !safeValue(value) {
		return false
	}
	switch platform {
	case PlatformWindowsAMD64:
		return windowsForwardDrivePath(value)
	case PlatformLinuxAMD64, PlatformLinuxARM64:
		return linuxAbsolutePath(value)
	default:
		return false
	}
}

// IsAllowedRootPath 验证执行节点的导出目录白名单。
// 为使已关联的旧节点可继续完成目录边界校验，Windows 兼容既有反斜杠白名单；新导出路径仍只能使用正斜杠盘符格式。
func IsAllowedRootPath(platform, value string) bool {
	if !safeValue(value) {
		return false
	}
	switch platform {
	case PlatformWindowsAMD64:
		return windowsForwardDrivePath(value) || windowsLegacyDrivePath(value)
	case PlatformLinuxAMD64, PlatformLinuxARM64:
		return linuxAbsolutePath(value)
	default:
		return false
	}
}

// LocalFilesystemPath 将已经验证的导出目录转换为 Agent 本机文件 API 可读取的路径。
// 该转换只用于 os.Stat、空间检查和结果核验；传入 OBDUMPER 的命令参数始终保持用户填写的正斜杠格式。
func LocalFilesystemPath(platform, value string) (string, bool) {
	if platform == PlatformWindowsAMD64 {
		if windowsForwardDrivePath(value) {
			return string(value[1]) + ":\\" + strings.ReplaceAll(value[4:], "/", "\\"), true
		}
		if windowsLegacyDrivePath(value) {
			return value, true
		}
		return "", false
	}
	if (platform == PlatformLinuxAMD64 || platform == PlatformLinuxARM64) && linuxAbsolutePath(value) {
		return value, true
	}
	return "", false
}

// WithinAllowedRoot 判断导出目录是否位于已登记的白名单目录内。
// 比较在逻辑规范形态进行，因而 /E:/data 与 E:\\data 表示同一 Windows 本机目录，但不会改变命令参数原文。
func WithinAllowedRoot(platform, output, root string) bool {
	outputKey, outputOK := comparisonPath(platform, output)
	rootKey, rootOK := comparisonPath(platform, root)
	if !outputOK || !rootOK {
		return false
	}
	if outputKey == rootKey {
		return true
	}
	if strings.HasSuffix(rootKey, "/") {
		return strings.HasPrefix(outputKey, rootKey)
	}
	return strings.HasPrefix(outputKey, rootKey+"/")
}

func safeValue(value string) bool {
	return len(value) > 0 && len(value) <= 4096 && value == strings.TrimSpace(value) && !strings.ContainsRune(value, 0) && !strings.ContainsAny(value, "\r\n")
}

func windowsForwardDrivePath(value string) bool {
	return len(value) >= 4 && value[0] == '/' && asciiLetter(value[1]) && value[2] == ':' && value[3] == '/' && !strings.Contains(value, "\\") && !strings.Contains(value, "//") && !containsTraversal(value, "/")
}

func windowsLegacyDrivePath(value string) bool {
	return len(value) >= 3 && asciiLetter(value[0]) && value[1] == ':' && value[2] == '\\' && !strings.Contains(value, "/") && !containsTraversal(value, "\\")
}

func linuxAbsolutePath(value string) bool {
	return strings.HasPrefix(value, "/") && !containsTraversal(value, "/")
}

func comparisonPath(platform, value string) (string, bool) {
	if platform == PlatformWindowsAMD64 {
		path, ok := LocalFilesystemPath(platform, value)
		if !ok {
			return "", false
		}
		path = strings.ReplaceAll(path, "\\", "/")
		return strings.ToLower(strings.TrimRight(path, "/")) + "/", true
	}
	if (platform == PlatformLinuxAMD64 || platform == PlatformLinuxARM64) && linuxAbsolutePath(value) {
		if value == "/" {
			return "/", true
		}
		return strings.TrimRight(value, "/"), true
	}
	return "", false
}

func containsTraversal(value, separator string) bool {
	for _, part := range strings.Split(value, separator) {
		if part == "." || part == ".." {
			return true
		}
	}
	return false
}

func asciiLetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
