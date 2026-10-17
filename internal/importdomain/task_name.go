package importdomain

import (
	"errors"
	"strings"
	"time"
	"unicode"
)

// TaskName 在提交时验证可选名称；空值使用带 UTC 时间和任务标识的确定格式，不参与命令指纹。
func TaskName(input, database, format, id string, now time.Time) (string, error) {
	name := strings.TrimSpace(input)
	if len([]rune(name)) > 200 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", errors.New("任务名称无效")
	}
	if name != "" {
		return name, nil
	}
	if format == "" {
		format = "DDL"
	}
	runes := []rune(database)
	if len(runes) > 64 {
		database = string(runes[:64])
	}
	if len(id) > 8 {
		id = id[:8]
	}
	return "普通导入-" + database + "-" + format + "-" + now.UTC().Format("20060102T150405Z") + "-" + id, nil
}
