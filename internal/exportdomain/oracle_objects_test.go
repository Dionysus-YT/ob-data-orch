package exportdomain

import (
	"testing"

	"ob-data-orch/internal/exportobject"
	"ob-data-orch/internal/store"
)

func TestOracle十类对象混选及兼容模式边界(t *testing.T) {
	config := &store.ExportConfig{
		ObjectScope:      store.ObjectScope{Database: "synthetic_schema", ScopeKind: "SPECIFIED", ObjectTypes: exportobject.Types()},
		ContentSelection: store.ContentSelection{ContentKind: "DDL_ONLY"},
	}
	for _, kind := range exportobject.Types() {
		// 各类型故意同名，验证名称不会覆盖对象归属。
		config.ObjectScope.Expressions = append(config.ObjectScope.Expressions, store.ObjectExpression{ObjectType: kind, Name: "synthetic_object"})
	}
	selection, err := NormalizeSelection(config, nil)
	if err != nil || len(selection.ObjectsByType) != 10 {
		t.Fatalf("十类对象未完整归一: %v", err)
	}
	draft := Draft{ScopeKind: selection.ScopeKind, ContentKind: selection.ContentKind, Objects: selection.Objects, ObjectsByType: selection.ObjectsByType}
	if ValidateObjectCompatibility(draft, "ORACLE") != nil || !draft.NeedsOracleObjectMetadata() {
		t.Fatal("Oracle 对象未选择新版本")
	}
	if ValidateObjectCompatibility(draft, "MYSQL") == nil || ValidateObjectCompatibility(draft, "UNKNOWN") == nil {
		t.Fatal("非 Oracle 模式接受了专属对象")
	}
	fields, err := BuildGeneralizedFields(draft)
	if err != nil || len(fields) != 12 {
		t.Fatalf("十类参数发射不完整: %v", err)
	}
	for index, kind := range exportobject.Types() {
		if fields[index].Name != objectParameterName(kind) || fields[index].Value.String != "synthetic_object" {
			t.Fatal("跨类型同名对象参数丢失")
		}
	}
	config.ContentSelection.ContentKind = "DATA_ONLY"
	config.DataFormat.FormatKind = "CSV"
	if _, err := NormalizeSelection(config, nil); err == nil {
		t.Fatal("非表对象被允许导出数据")
	}
}
