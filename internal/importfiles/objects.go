package importfiles

import (
	"regexp"
	"strings"

	"ob-data-orch/internal/importdomain"
)

// DefinitionObject 是已完整识别的对象权限事实，不携带 SQL 或过程体。
type DefinitionObject struct {
	Kind string
	Name string
	// Reference 仅允许当前 Schema 内的单个依赖；类型体、包体指向同名声明。
	Reference string
}

var simpleObjectName = regexp.MustCompile(`^[A-Za-z_][A-Za-z_0-9$]{0,127}$`)

// inspectObjectDefinition 仅识别单个声明和无副作用的受限过程体；未知语句、秘密、跨库依赖与尾随操作均失败关闭。
func inspectObjectDefinition(raw, name string, c importdomain.Config) (DefinitionObject, bool) {
	if !simpleObjectName.MatchString(name) || c.ReplaceObject {
		return DefinitionObject{}, false
	}
	text := strings.TrimSpace(raw)
	wrapped := false
	if strings.HasPrefix(strings.ToUpper(text), "DELIMITER ") {
		lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		if len(lines) < 4 || !strings.EqualFold(strings.TrimSpace(lines[len(lines)-1]), "DELIMITER ;") {
			return DefinitionObject{}, false
		}
		marker := strings.TrimSpace(strings.TrimSpace(lines[0])[len("DELIMITER "):])
		if (marker != "/" && marker != "$$") || strings.TrimSpace(lines[len(lines)-2]) != marker {
			return DefinitionObject{}, false
		}
		text = strings.TrimSpace(strings.Join(lines[1:len(lines)-2], "\n"))
		wrapped = true
	}
	text, ok := definitionPrelude(text)
	if !ok {
		return DefinitionObject{}, false
	}
	text = strings.Join(strings.Fields(text), " ")
	object := `(?-i:` + regexp.QuoteMeta(name) + "|`" + regexp.QuoteMeta(name) + "`|\"" + regexp.QuoteMeta(name) + "\")"
	id := `([A-Za-z_][A-Za-z_0-9$]{0,127})`
	patterns := []struct{ kind, body string }{
		{"table-group", `CREATE TABLEGROUP ` + object + `(?: SHARDING\s*=\s*'ADAPTIVE')?\s*;`},
		{"view", `CREATE VIEW ` + object + ` AS SELECT [0-9]+ AS ` + id + `(?: FROM DUAL)?\s*;`},
		{"trigger", `CREATE TRIGGER ` + object + ` BEFORE INSERT ON ` + id + ` FOR EACH ROW SET NEW\s*\.\s*` + id + `\s*=\s*NEW\s*\.\s*` + id + `\s*;`},
		{"trigger", `CREATE TRIGGER ` + object + ` BEFORE INSERT ON ` + id + ` FOR EACH ROW BEGIN :NEW\s*\.\s*` + id + `\s*:=\s*:NEW\s*\.\s*` + id + `\s*; END\s*;\s*/?`},
		// 未提供秘密的休眠账号只接受不可路由来源，不能以空密码创建可登录账号。
		{"obj-user", `CREATE USER ` + object + `\s*@\s*'0\.0\.0\.0'\s*;`},
		{"role", `CREATE ROLE ` + object + `\s*;`},
		{"sequence", `CREATE SEQUENCE ` + object + ` START WITH [1-9][0-9]*(?: INCREMENT BY [1-9][0-9]*)?(?: NOCACHE)?\s*;`},
		{"synonym", `CREATE SYNONYM ` + object + ` FOR ` + id + `\s*;`},
		{"type", `CREATE TYPE ` + object + ` AS OBJECT\s*\(\s*` + id + ` NUMBER(?:\s*,\s*` + id + ` NUMBER)*(?:\s*,\s*MEMBER FUNCTION ` + id + ` RETURN NUMBER)?\s*\)\s*;\s*/?`},
		{"type-body", `CREATE TYPE BODY ` + object + ` (?:AS|IS) MEMBER FUNCTION ` + id + ` RETURN NUMBER (?:AS|IS) BEGIN RETURN [0-9]+\s*; END\s*; END\s*;\s*/?`},
		{"package", `CREATE PACKAGE ` + object + ` (?:AS|IS) PROCEDURE ` + id + `\s*; END\s*;\s*/?`},
		{"package-body", `CREATE PACKAGE BODY ` + object + ` (?:AS|IS) PROCEDURE ` + id + ` (?:AS|IS) BEGIN NULL\s*; END\s*; END\s*;\s*/?`},
		{"function", `CREATE FUNCTION ` + object + `\s*\(\s*\) RETURNS (?:INT|INTEGER|BIGINT) RETURN [0-9]+\s*;`},
		{"procedure", `CREATE PROCEDURE ` + object + `\s*\(\s*\) BEGIN(?: SELECT [0-9]+\s*;)? END\s*;`},
		{"procedure", `CREATE PROCEDURE ` + object + `\s*\(\s*\) SELECT [0-9]+\s*;`},
	}
	for _, pattern := range patterns {
		match := regexp.MustCompile(`(?i)^` + pattern.body + `$`).FindStringSubmatch(text)
		if match == nil {
			continue
		}
		// 官方工具默认按分号拆语句；复合体必须显式使用已核验的文件分隔包装。
		if (pattern.kind == "type-body" || pattern.kind == "package" || pattern.kind == "package-body" || strings.Contains(strings.ToUpper(text), " BEGIN")) && !wrapped {
			return DefinitionObject{}, false
		}
		if c.Scope == "SPECIFIC" && !matchesExpression(name, c.Objects[pattern.kind]) {
			return DefinitionObject{}, false
		}
		if (pattern.kind == "role" || pattern.kind == "obj-user") && (c.Scope != "SPECIFIC" || c.Objects[pattern.kind] != name) {
			return DefinitionObject{}, false
		}
		result := DefinitionObject{Kind: pattern.kind, Name: name}
		switch pattern.kind {
		case "synonym":
			result.Reference = match[1]
		case "trigger":
			if !strings.EqualFold(match[2], match[3]) {
				return DefinitionObject{}, false
			}
			result.Reference = match[1]
		case "type-body", "package-body":
			result.Reference = name
		}
		return result, true
	}
	return DefinitionObject{}, false
}
