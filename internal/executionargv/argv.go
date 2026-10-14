// Package executionargv 统一控制面、Agent 协议与固定 Java 启动的非秘密业务参数边界。
package executionargv

import (
	"strings"

	"ob-data-orch/internal/querytext"
)

// Valid 只用于固定 OBDUMPER 执行信封，不能作为任意命令或 SQL 的准入接口。
// 普通令牌仍限 4096 字节且禁止换行；唯一的 --query-sql 值允许单条 SELECT 的换行和包装后 64 KiB。
// 命令生成、任务授权、租约与固定主类仍由各自所有者复核，密码始终不得进入 argv。
func Valid(argv []string) bool {
	if len(argv) == 0 || len(argv) > 160 {
		return false
	}
	querySeen := false
	for index := 0; index < len(argv); index++ {
		value := argv[index]
		if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") || value == "--password" || strings.HasPrefix(value, "--password=") || value == "--sys-password" || strings.HasPrefix(value, "--sys-password=") || strings.HasPrefix(value, "-p") {
			return false
		}
		if value != "--query-sql" {
			continue
		}
		if querySeen || index+1 >= len(argv) {
			return false
		}
		querySeen = true
		index++
		if _, err := querytext.NormalizeSelect(argv[index], 64<<10); err != nil {
			return false
		}
	}
	return true
}
