package importfiles

import (
	"context"
	"errors"

	"ob-data-orch/internal/importdomain"
)

// InspectMappingForPlatform 将固定工具的平台默认解析值用于检查副本，不改写配置、命令或指纹。
// OBLOADER 4.3.5 CUT 的 DEFAULT_LINE_SEPARATOR 来自 Java System.lineSeparator；
// 检查必须使用执行节点声明的平台，不能使用控制面或测试主机的平台猜测。
func InspectMappingForPlatform(ctx context.Context, c importdomain.Config, manifest Manifest, roots []string, platform string) (MappingPlan, error) {
	if c.Format == "CUT" && c.Parser.LineSeparator == "" {
		switch platform {
		case "WINDOWS_AMD64":
			c.Parser.LineSeparator = `\r\n`
		case "LINUX_AMD64", "LINUX_ARM64":
			c.Parser.LineSeparator = `\n`
		default:
			return MappingPlan{}, errors.New("导入检查平台未知")
		}
	}
	return InspectMapping(ctx, c, manifest, roots)
}
