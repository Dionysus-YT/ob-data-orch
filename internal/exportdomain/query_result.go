package exportdomain

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// MaxQueryResultRows 是结果集入口接受的最大行数；生成的数值直接写入受控 SQL 包装层。
const MaxQueryResultRows int64 = 2147483647

// NormalizeResultQuery 收窄结果集入口为单条直接 SELECT 文本。
// 查询只会传入 OBDUMPER --query-sql；拒绝语句分隔符和注释，避免包装后的行数上限被截断。
func NormalizeResultQuery(query string, limit int64) (string, error) {
	query = strings.TrimSpace(query)
	if limit < 1 || limit > MaxQueryResultRows || query == "" || len(query) > 60<<10 {
		return "", errors.New("query result SQL or row limit is invalid")
	}
	if len(query) <= len("SELECT") || !strings.EqualFold(query[:len("SELECT")], "SELECT") || !unicode.IsSpace(rune(query[len("SELECT")])) {
		return "", errors.New("query result SQL must be a single SELECT")
	}
	if strings.ContainsAny(query, ";\x00") || strings.Contains(query, "--") || strings.Contains(query, "/*") || strings.Contains(query, "*/") || strings.Contains(strings.ToLower(query), "file://") {
		return "", errors.New("query result SQL contains an unsupported token")
	}
	return query, nil
}

// WrapResultQuery 按已验证数据源兼容模式在 SELECT 外层施加行数上限。
// 不执行或解析用户 SQL；复杂语句由 OBDUMPER 和数据库在任务执行时判定。
func WrapResultQuery(mode, query string, limit int64) (string, error) {
	query, err := NormalizeResultQuery(query, limit)
	if err != nil {
		return "", err
	}
	switch mode {
	case "MYSQL":
		return "SELECT * FROM (\n" + query + "\n) AS obdo_result LIMIT " + strconv.FormatInt(limit, 10), nil
	case "ORACLE":
		return "SELECT * FROM (\n" + query + "\n) obdo_result WHERE ROWNUM <= " + strconv.FormatInt(limit, 10), nil
	default:
		return "", errors.New("query result compatibility mode is unsupported")
	}
}
