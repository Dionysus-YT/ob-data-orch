// Package querytext 只校验 OBDUMPER 结果集参数的文本边界，不执行 SQL 或读取数据库。
package querytext

import (
	"errors"
	"strings"
	"unicode"
)

// NormalizeSelect 把结果集入口收窄为单条直接 SELECT，允许正常排版的换行。
// 字节上限由原始输入或包装后执行边界分别给出；分隔符、注释与文件引用继续失败关闭。
func NormalizeSelect(query string, maxBytes int) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > maxBytes || len(query) <= len("SELECT") || !strings.EqualFold(query[:len("SELECT")], "SELECT") || !unicode.IsSpace(rune(query[len("SELECT")])) {
		return "", errors.New("query result SQL must be a bounded single SELECT")
	}
	if strings.ContainsAny(query, ";\x00") || strings.Contains(query, "--") || strings.Contains(query, "/*") || strings.Contains(query, "*/") || strings.Contains(strings.ToLower(query), "file://") {
		return "", errors.New("query result SQL contains an unsupported token")
	}
	return query, nil
}
