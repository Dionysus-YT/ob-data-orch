package executionargv

import (
	"strconv"
	"strings"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/parammeta"
)

// ValidDirectLoad 只接受旁路已启用的非秘密长参数；普通导入、恢复和自由 SQL 不能混入。
func ValidDirectLoad(argv []string) bool {
	_, ok := directLoadValues(argv)
	return ok
}

func directLoadValues(argv []string) (map[string]string, bool) {
	if !Valid(argv) {
		return nil, false
	}
	catalog, err := parammeta.LoadDirectLoad()
	if err != nil {
		return nil, false
	}
	values := map[string]string{}
	for i := 0; i < len(argv); i++ {
		name := argv[i]
		// 固定研究目录保留官方全集；执行仍按当前本地范围拒绝其他部署与未确认格式。
		if name == "--public-cloud" || name == "--sql" || name == "--par" || name == "--orc" {
			return nil, false
		}
		d, exists := catalog.Definition(name)
		if !exists || d.SupportState != "ENABLED" || d.Sensitivity == "SECRET" || d.EmissionTarget == "SECURITY_FILE" || values[name] != "" {
			return nil, false
		}
		value := "true"
		if d.ValueType != "flag" {
			i++
			if i >= len(argv) || strings.HasPrefix(argv[i], "-") {
				return nil, false
			}
			value = argv[i]
		}
		values[name] = value
	}
	for _, name := range []string{"--host", "--port", "--user", "--database", "--file-path", "--table", "--rpc-port", "--direct"} {
		if values[name] == "" {
			return nil, false
		}
	}
	formats := 0
	for _, name := range []string{"--csv", "--sql", "--par", "--orc"} {
		if values[name] != "" {
			formats++
		}
	}
	if formats != 1 || (values["--yes"] != "") != (values["--truncate-table"] != "") {
		return nil, false
	}
	for _, name := range []string{"--port", "--rpc-port", "--thread", "--parallel"} {
		if raw := values[name]; raw != "" {
			n, err := strconv.ParseInt(raw, 10, 32)
			if err != nil || n < 1 || (name == "--port" || name == "--rpc-port") && n > 65535 || strconv.FormatInt(n, 10) != raw {
				return nil, false
			}
		}
	}
	return values, true
}

// MatchesDirectLoad 复核冻结 argv 与领域配置完全一致；这里只比较唯一生成器的产物，不拼装执行命令。
func MatchesDirectLoad(argv []string, c directload.Config) bool {
	values, ok := directLoadValues(argv)
	if !ok || values["--database"] != c.Database || c.Format != "CSV" {
		return false
	}
	switch c.Scenario {
	case "PRIVATE_ODP":
		if values["--public-cloud"] != "" || values["--tenant"] == "" || values["--cluster"] == "" {
			return false
		}
	case "OBSERVER_DIRECT":
		if values["--public-cloud"] != "" || values["--tenant"] == "" || values["--cluster"] != "" {
			return false
		}
	default:
		return false
	}
	for _, name := range []string{"--host", "--port", "--user", "--database", "--public-cloud", "--tenant", "--cluster"} {
		delete(values, name)
	}
	if c.TruncateTable {
		delete(values, "--yes")
	}
	for _, f := range directload.Fields(c) {
		expected := f.Value.String
		switch f.Value.Kind {
		case commandgen.ValueInteger:
			expected = strconv.FormatInt(f.Value.Integer, 10)
		case commandgen.ValueBoolean:
			expected = "true"
		}
		if values[f.Name] != expected {
			return false
		}
		delete(values, f.Name)
	}
	return len(values) == 0
}
