package exportdomain

import "errors"

// NeedsSchemaBooleanMetadata 仅对显式结构策略或 MySQL 保留前缀选择新语法，其他任务沿用原元数据。
func (n Draft) NeedsSchemaBooleanMetadata(compatibilityMode string) bool {
	return n.CompactSchemaValue != nil || (n.RetainSchema && compatibilityMode == "MYSQL")
}

// ResolveSchemaStrategy 在唯一命令输入层选择固定工具已有的结构路径，不重写输出 DDL。
// MySQL SHOW CREATE TABLE 不带前缀，保留前缀必须显式关闭原生路径；旧冻结版本不得调用此转换。
func (n Draft) ResolveSchemaStrategy(compatibilityMode string) (Draft, error) {
	// 本轮只开放保留前缀所需的关闭分支；避免 API 接受界面无法保持的独立重建模式。
	if n.CompactSchemaValue != nil && !*n.CompactSchemaValue && !(n.RetainSchema && compatibilityMode == "MYSQL") {
		return Draft{}, errors.New("显式关闭原生建表语句仅用于 MySQL 保留 Schema")
	}
	if n.RetainSchema && compatibilityMode == "MYSQL" {
		if n.CompactSchema || (n.CompactSchemaValue != nil && *n.CompactSchemaValue) {
			return Draft{}, errors.New("MySQL 保留 Schema 不能同时使用原生建表语句")
		}
		value := false
		n.CompactSchemaValue = &value
	}
	return n, nil
}
