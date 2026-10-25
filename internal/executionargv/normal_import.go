package executionargv

import (
	"strings"

	"ob-data-orch/internal/parammeta"
)

// ValidNormalImport 只接受普通导入目录中的非秘密参数，不能执行任意 SQL 或旁路命令。
func ValidNormalImport(argv []string) bool {
	if !Valid(argv) {
		return false
	}
	catalog, err := parammeta.LoadNormalImport()
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	for index := 0; index < len(argv); index++ {
		name := argv[index]
		// 只接受唯一生成器的严格布尔写法，不把任意 name=value 扩成新参数通道。
		if strings.HasPrefix(name, "--strict=") {
			value := strings.TrimPrefix(name, "--strict=")
			if (value != "true" && value != "false") || seen["--strict"] {
				return false
			}
			seen["--strict"] = true
			continue
		}
		if name == "--strict" {
			return false
		}
		definition, ok := catalog.Definition(name)
		if !ok || seen[name] || definition.EmissionTarget == "SECURITY_FILE" || definition.Sensitivity == "SECRET" {
			return false
		}
		seen[name] = true
		if definition.ValueType != "flag" {
			index++
			if index >= len(argv) {
				return false
			}
		}
	}
	return seen["--host"] && seen["--port"] && seen["--user"] && seen["--database"] && seen["--file-path"] && (seen["--csv"] || seen["--cut"] || seen["--sql"] || seen["--ddl"]) && seen["--no-sys"]
}
