package store

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maximumODPJDBCIdentityBytes = 256

// PrivateODPCommandIdentity 为已授权的命令预览和受控执行参数生成私有 ODP 组合用户名。
// 返回值只能进入受控命令路径，不能记录到普通日志、错误或审计中。
func PrivateODPCommandIdentity(username, tenantName, clusterName string) (string, bool) {
	if !validPrivateODPIdentityPart(username) || !validPrivateODPIdentityPart(tenantName) || !validPrivateODPIdentityPart(clusterName) {
		return "", false
	}
	length := len(username) + len(tenantName) + len(clusterName) + 2
	if length > maximumODPJDBCIdentityBytes {
		return "", false
	}
	return username + "@" + tenantName + "#" + clusterName, true
}

// composePrivateODPJDBCIdentity 将分字段保存的私有 ODP 身份组合为 JDBC 驱动需要的短时字节。
// 当前首条切片固定使用 username@tenant#cluster；调用方必须在使用结束后清零返回值，不能持久化完整身份。
func composePrivateODPJDBCIdentity(username, tenantName, clusterName string) ([]byte, bool) {
	if !validPrivateODPIdentityPart(username) || !validPrivateODPIdentityPart(tenantName) || !validPrivateODPIdentityPart(clusterName) {
		return nil, false
	}
	length := len(username) + len(tenantName) + len(clusterName) + 2
	if length > maximumODPJDBCIdentityBytes {
		return nil, false
	}
	identity := make([]byte, 0, length)
	identity = append(identity, username...)
	identity = append(identity, '@')
	identity = append(identity, tenantName...)
	identity = append(identity, '#')
	identity = append(identity, clusterName...)
	return identity, true
}

func validPrivateODPIdentityPart(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == '@' || character == '#' || character == ':' || unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}
