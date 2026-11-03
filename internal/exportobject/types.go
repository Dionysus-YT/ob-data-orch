// Package exportobject 定义 OBDUMPER 指定对象导出的固定类型与兼容模式边界。
package exportobject

var orderedTypes = []string{"TABLE", "VIEW", "FUNCTION", "PROCEDURE", "PACKAGE", "TRIGGER", "SYNONYM", "PUBLIC_SYNONYM", "TYPE", "SEQUENCE"}

// Types 返回展示与命令使用的稳定类型顺序，调用方不能修改内部目录。
func Types() []string { return append([]string(nil), orderedTypes...) }

// Supported 判断类型是否属于已接入的固定对象参数。
func Supported(kind string) bool {
	for _, candidate := range orderedTypes {
		if kind == candidate {
			return true
		}
	}
	return false
}

// OracleOnly 标识仅 Oracle 兼容模式允许导出的五类对象。
func OracleOnly(kind string) bool {
	switch kind {
	case "PACKAGE", "TRIGGER", "SYNONYM", "PUBLIC_SYNONYM", "TYPE":
		return true
	default:
		return false
	}
}

// ValidForMode 在未知模式或 Oracle 专属对象用于 MySQL 时失败关闭。
func ValidForMode(kind, mode string) bool {
	return Supported(kind) && (mode == "ORACLE" || mode == "MYSQL" && !OracleOnly(kind))
}

// TypesForMode 返回该模式的完整分类集合；未知模式不提供候选。
func TypesForMode(mode string) []string {
	var result []string
	for _, kind := range orderedTypes {
		if ValidForMode(kind, mode) {
			result = append(result, kind)
		}
	}
	return result
}
