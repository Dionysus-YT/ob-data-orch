package exportdomain

import (
	"testing"

	"ob-data-orch/internal/store"
)

func TestNormalizeSelection(t *testing.T) {
	tests := []struct {
		name    string
		config  *store.ExportConfig
		want    Selection
		wantErr bool
		wantRaw string
	}{
		{
			name: "指定表 CSV",
			config: &store.ExportConfig{
				ObjectScope:      store.ObjectScope{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectTypes: []string{"TABLE"}, Expressions: []store.ObjectExpression{{Schema: "synthetic_db", Name: "orders"}}},
				ContentSelection: store.ContentSelection{ContentKind: "DATA_ONLY"},
				DataFormat:       store.DataFormat{FormatKind: "CSV"},
			},
			want:    Selection{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{"orders"}, ContentKind: "DATA_ONLY", Format: "CSV"},
			wantRaw: "synthetic_db.orders",
		},
		{
			name: "全部对象 DDL",
			config: &store.ExportConfig{
				ObjectScope:      store.ObjectScope{Database: "synthetic_db", ScopeKind: "ALL"},
				ContentSelection: store.ContentSelection{ContentKind: "DDL_ONLY"},
			},
			want: Selection{Database: "synthetic_db", ScopeKind: "ALL", ContentKind: "DDL_ONLY"},
		},
		{
			name: "DDL 加数据固定 CSV",
			config: &store.ExportConfig{
				ObjectScope:      store.ObjectScope{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectTypes: []string{"TABLE"}, Expressions: []store.ObjectExpression{{Name: "orders"}}},
				ContentSelection: store.ContentSelection{ContentKind: "DDL_AND_DATA"},
				DataFormat:       store.DataFormat{FormatKind: "CSV"},
			},
			want: Selection{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{"orders"}, ContentKind: "DDL_AND_DATA", Format: "CSV"},
		},
		{
			name: "DDL 加数据拒绝 CUT",
			config: &store.ExportConfig{
				ObjectScope:      store.ObjectScope{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectTypes: []string{"TABLE"}, Expressions: []store.ObjectExpression{{Name: "orders"}}},
				ContentSelection: store.ContentSelection{ContentKind: "DDL_AND_DATA"},
				DataFormat:       store.DataFormat{FormatKind: "CUT"},
			},
			wantErr: true,
		},
		{
			name: "视图拒绝数据",
			config: &store.ExportConfig{
				ObjectScope:      store.ObjectScope{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectTypes: []string{"VIEW"}, Expressions: []store.ObjectExpression{{Name: "orders_view"}}},
				ContentSelection: store.ContentSelection{ContentKind: "DATA_ONLY"},
				DataFormat:       store.DataFormat{FormatKind: "CSV"},
			},
			wantErr: true,
		},
		{
			name: "跨库前缀失败关闭",
			config: &store.ExportConfig{
				ObjectScope:      store.ObjectScope{Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectTypes: []string{"TABLE"}, Expressions: []store.ObjectExpression{{Schema: "other_db", Name: "orders"}}},
				ContentSelection: store.ContentSelection{ContentKind: "DATA_ONLY"},
				DataFormat:       store.DataFormat{FormatKind: "CSV"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeSelection(tt.config, 100, func(name string) error {
				if name == "" {
					t.Fatal("测试夹具不应传入空对象名")
				}
				return nil
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("错误状态不匹配：got=%v wantErr=%v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got.Database != tt.want.Database || got.ScopeKind != tt.want.ScopeKind || got.ObjectType != tt.want.ObjectType || got.ContentKind != tt.want.ContentKind || got.Format != tt.want.Format {
				t.Fatalf("选择结果不匹配：got=%#v want=%#v", got, tt.want)
			}
			if len(got.Objects) != len(tt.want.Objects) || (len(got.Objects) == 1 && got.Objects[0] != tt.want.Objects[0]) {
				t.Fatalf("对象列表不匹配：got=%#v want=%#v", got.Objects, tt.want.Objects)
			}
			if tt.wantRaw != "" && tt.config.ObjectScope.Expressions[0].RawInput != tt.wantRaw {
				t.Fatalf("RawInput 规范化不匹配：got=%q want=%q", tt.config.ObjectScope.Expressions[0].RawInput, tt.wantRaw)
			}
		})
	}
}

func TestCapabilityAndDisplayFormat(t *testing.T) {
	tests := []struct {
		content string
		format  string
		cap     string
		display string
	}{
		{content: "DATA_ONLY", format: "CSV", cap: "export-odp-full-csv-v1", display: "CSV"},
		{content: "DATA_ONLY", format: "CUT", cap: "export-odp-cut-v1", display: "CUT"},
		{content: "DATA_ONLY", format: "SQL", cap: "export-odp-sql-v1", display: "SQL"},
		{content: "DATA_ONLY", format: "POS", cap: "export-odp-pos-v1", display: "POS"},
		{content: "DATA_ONLY", format: "PARQUET", cap: "export-odp-parquet-v1", display: "PARQUET"},
		{content: "DATA_ONLY", format: "ORC", cap: "export-odp-orc-v1", display: "ORC"},
		{content: "DATA_ONLY", format: "AVRO", cap: "export-odp-avro-v1", display: "AVRO"},
		{content: "DDL_ONLY", format: "", cap: "export-odp-ddl-v1", display: "DDL"},
		{content: "DDL_AND_DATA", format: "CSV", cap: "export-odp-ddl-csv-v1", display: "DDL_CSV"},
	}
	for _, tt := range tests {
		if got := CapabilityFor(tt.content, tt.format); got != tt.cap {
			t.Fatalf("能力版本不匹配：content=%q format=%q got=%q want=%q", tt.content, tt.format, got, tt.cap)
		}
		if got := DisplayFormat(tt.content, tt.format); got != tt.display {
			t.Fatalf("显示格式不匹配：content=%q format=%q got=%q want=%q", tt.content, tt.format, got, tt.display)
		}
	}
}

func TestDraftFrozenShape(t *testing.T) {
	draft := Draft{ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{"orders"}, ContentKind: "DATA_ONLY", Format: "CSV", OutputKind: "LOCAL"}
	if !draft.IsFrozenSingleTableCSV() || !draft.HasZeroOptions() {
		t.Fatal("最小单表 CSV 应保持冻结形状")
	}
	draft.QuerySql = "select 1"
	if draft.IsFrozenSingleTableCSV() || draft.HasZeroOptions() {
		t.Fatal("敏感查询参数不应进入冻结形状")
	}
}
