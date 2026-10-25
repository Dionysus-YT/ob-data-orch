package importfiles

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ob-data-orch/internal/importdomain"
)

var controlColumn = regexp.MustCompile("(?i)^([a-z_][a-z_0-9$]*|`[a-z_][a-z_0-9$]*`|\\[[a-z_][a-z_0-9$]*\\])(?:\\s+\"NONE\")?(?:\\s+map\\(([1-9][0-9]*)\\))?$")

// controlFiles 只在允许根目录内冻结 .ctrl 文件，不提供控制文件浏览或执行预处理的接口。
func controlFiles(ctx context.Context, c importdomain.Config, roots []string) (Manifest, error) {
	if c.Mapping == nil || c.Mapping.Kind != "CONTROL" {
		return Manifest{}, errors.New("控制文件上下文无效")
	}
	return ScanMatched(ctx, c.Mapping.CtlPath, roots, func(name string) (string, string, bool) {
		if !strings.HasSuffix(name, ".ctrl") {
			return "", "", false
		}
		return "CONTROL", strings.TrimSuffix(name, ".ctrl"), true
	})
}

func controlMappings(ctx context.Context, c importdomain.Config, m Manifest, roots []string) (map[string][]string, error) {
	facts, err := controlFiles(ctx, c, roots)
	if err != nil || facts.Digest != m.ControlDigest || facts.FileCount < 1 {
		return nil, errors.New("控制文件绑定变化")
	}
	result := map[string][]string{}
	for _, entry := range facts.Files {
		if entry.Size > 32*1024 || filepath.Base(entry.Path) != entry.Object+".ctrl" {
			return nil, errMappingUnsupported
		}
		prefix, whole, err := mappingPrefix(ctx, c.Mapping.CtlPath, entry, roots, nil)
		if err != nil {
			return nil, err
		}
		names, ok := parseControl(prefix)
		for i := range prefix {
			prefix[i] = 0
		}
		if !whole || !ok || result[entry.Object] != nil {
			return nil, errMappingUnsupported
		}
		result[entry.Object] = names
	}
	return result, nil
}

// parseControl 仅核验无副作用的普通列和 NONE/map 顺序；函数、生成列、重复与遗漏源位置保持未知。
func parseControl(raw []byte) ([]string, bool) {
	text := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(text, "lang=java") {
		return nil, false
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "lang=java"))
	if !strings.HasPrefix(text, "(") || !strings.HasSuffix(text, ");") {
		return nil, false
	}
	parts := strings.Split(strings.TrimSpace(text[1:len(text)-2]), ",")
	if len(parts) < 1 || len(parts) > 4096 {
		return nil, false
	}
	byPosition := make([]string, len(parts))
	seen := map[string]bool{}
	for index, part := range parts {
		match := controlColumn.FindStringSubmatch(strings.TrimSpace(part))
		// 实测省略 map 的列并不继承声明位置；必须逐列明确源位置，避免静默错列。
		if match == nil || match[2] == "" {
			return nil, false
		}
		name := strings.Trim(match[1], "`[]")
		position := index + 1
		if match[2] != "" {
			var err error
			position, err = strconv.Atoi(match[2])
			if err != nil {
				return nil, false
			}
		}
		if position < 1 || position > len(parts) || byPosition[position-1] != "" || seen[name] {
			return nil, false
		}
		byPosition[position-1] = name
		seen[name] = true
	}
	return byPosition, true
}
