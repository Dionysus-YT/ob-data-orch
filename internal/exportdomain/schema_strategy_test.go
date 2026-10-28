package exportdomain

import "testing"

func Test保留Schema选择工具已有路径并拒绝冲突(t *testing.T) {
	for _, mode := range []string{"MYSQL", "ORACLE"} {
		input := Draft{ContentKind: "DDL_ONLY", RetainSchema: true}
		resolved, err := input.ResolveSchemaStrategy(mode)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "MYSQL" && (resolved.CompactSchemaValue == nil || *resolved.CompactSchemaValue) {
			t.Fatal("MySQL 未显式关闭原生建表路径")
		}
		if mode == "ORACLE" && resolved.CompactSchemaValue != nil {
			t.Fatal("Oracle 策略被无关改写")
		}
		if input.CompactSchemaValue != nil {
			t.Fatal("策略修改了原始输入")
		}
	}
	for _, pointer := range []bool{false, true} {
		input := Draft{ContentKind: "DDL_ONLY", RetainSchema: true, CompactSchema: !pointer}
		if pointer {
			value := true
			input.CompactSchemaValue = &value
		}
		if _, err := input.ResolveSchemaStrategy("MYSQL"); err == nil {
			t.Fatal("原生路径与 MySQL 保留 Schema 冲突未拒绝")
		}
	}
	value := false
	resolved, err := (Draft{ContentKind: "DDL_ONLY", RetainSchema: true, CompactSchemaValue: &value}).ResolveSchemaStrategy("MYSQL")
	if err != nil || resolved.CompactSchemaValue == nil || *resolved.CompactSchemaValue {
		t.Fatal("显式关闭被吞掉")
	}
	if _, err := (Draft{ContentKind: "DDL_ONLY", CompactSchemaValue: &value}).ResolveSchemaStrategy("MYSQL"); err == nil {
		t.Fatal("独立重建模式超出本轮范围却被接受")
	}
}
