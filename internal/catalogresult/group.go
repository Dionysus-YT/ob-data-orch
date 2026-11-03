// Package catalogresult 定义一次受控元数据查询中的有界分类结果。
package catalogresult

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/exportobject"
)

// Group 是固定对象目录中的一个分类；不可用时不得携带对象名。
type Group struct {
	ObjectType  string   `json:"objectType"`
	Objects     []string `json:"objects"`
	Truncated   bool     `json:"truncated"`
	Unavailable bool     `json:"unavailable"`
}

// MaxResponseBytes 是目录响应的资源预算；超限必须整体失败，不能截断为成功。
const MaxResponseBytes = 16 << 20

// ValidGroups 拒绝缺类、截断及不安全名称；对象数量不限，响应字节预算由传输边界执行。
func ValidGroups(groups []Group) bool {
	mode := "ORACLE"
	if len(groups) == 5 {
		mode = "MYSQL"
	}
	return ValidGroupsForMode(groups, mode)
}

// ValidGroupsForMode 按冻结模式要求完整目录，旧 Oracle Agent 的五类响应不得冒充十类成功。
func ValidGroupsForMode(groups []Group, mode string) bool {
	orderedTypes := exportobject.TypesForMode(mode)
	if len(orderedTypes) == 0 || len(groups) != len(orderedTypes) {
		return false
	}
	for index, group := range groups {
		if group.ObjectType != orderedTypes[index] || group.Objects == nil || group.Truncated ||
			(group.Unavailable && (len(group.Objects) != 0 || group.Truncated)) {
			return false
		}
		seen := make(map[string]bool, len(group.Objects))
		for _, name := range group.Objects {
			if name == "" || len(name) > 256 || !utf8.ValidString(name) || strings.TrimSpace(name) != name || strings.ContainsAny(name, "*,") || seen[name] {
				return false
			}
			for _, character := range name {
				if unicode.IsControl(character) {
					return false
				}
			}
			seen[name] = true
		}
	}
	return true
}
