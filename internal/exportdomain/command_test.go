package exportdomain

import (
	"reflect"
	"testing"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/store"
)

func fieldNames(fields []commandgen.FieldInput) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	return names
}

func TestBuildGeneralizedFieldsPreservesOrder(t *testing.T) {
	compressionLevel := int64(5)
	thread := 4
	input := Draft{
		Database: "synthetic_db", ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{"orders", "items"},
		ExcludeTables: []string{"audit_log"}, ContentKind: "DATA_ONLY", Format: "CSV",
		FilePath: "/E:/tmp/out", LogPath: "/E:/tmp/log", SkipCheckDir: true,
		CsvOptions:  store.CsvOptions{SkipHeader: true, ColumnSeparator: ",", FileEncoding: "UTF-8", WithTrim: true},
		NoNestedDir: true, MaxFileSize: int64Pointer(1024), RetainEmptyFiles: true,
		Compress: true, CompressionAlgo: "zstd", CompressionLevel: &compressionLevel,
		Where: "id > 0", Snapshot: true, Partition: "p0,p2", ExcludeDataTypes: []string{"BLOB"},
		TimestampFormats: store.TimestampFormatConfig{DateValueFormat: "yyyy/MM/dd", DateTimeValueFormat: "yyyy/MM/dd HH:mm:ss"},
		IncludeColumns:   []string{"id", "name"}, ExcludeColumns: []string{"secret"}, ExcludeVirtualColumns: true,
		FlashbackScn: int64Pointer(100), FlashbackTimestamp: "2026-01-01 00:00:00", Thread: &thread, JvmMemory: "2G", BlockSize: "1024MB",
	}

	fields, err := BuildGeneralizedFields(input)
	if err != nil {
		t.Fatalf("构建字段失败：%v", err)
	}
	want := []string{
		"--table", "--exclude-table", "--csv", "--file-path", "--log-path", "--skip-check-dir",
		"--skip-header", "--column-separator", "--file-encoding", "--with-trim", "--no-nested-dir",
		"--max-file-size", "--retain-empty-files", "--compress", "--compression-algo", "--compression-level",
		"--where", "--snapshot", "--partition", "--exclude-data-types", "--date-value-format", "--datetime-value-format",
		"--include-column-names", "--exclude-column-names", "--exclude-virtual-columns", "--flashback-scn",
		"--flashback-timestamp", "--thread", "--mem", "--block-size",
	}
	if got := fieldNames(fields); !reflect.DeepEqual(got, want) {
		t.Fatalf("字段顺序不匹配：got=%v want=%v", got, want)
	}
}

func TestBuildGeneralizedFieldsFormatMatrix(t *testing.T) {
	tests := []struct {
		name   string
		input  Draft
		fields []string
	}{
		{name: "全部范围 DDL", input: Draft{ScopeKind: "ALL", ContentKind: "DDL_ONLY", FilePath: "/E:/tmp/out"}, fields: []string{"--all", "--ddl", "--file-path", "--skip-check-dir"}},
		{name: "视图 SQL", input: Draft{ScopeKind: "SPECIFIED", ObjectType: "VIEW", Objects: []string{"v_orders"}, ContentKind: "DATA_ONLY", Format: "SQL", FilePath: "/E:/tmp/out", CsvOptions: store.CsvOptions{LineSeparator: "\\n"}}, fields: []string{"--view", "--sql", "--file-path", "--skip-check-dir", "--line-separator"}},
		{name: "POS 控制文件", input: Draft{ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{"orders"}, ContentKind: "DATA_ONLY", Format: "POS", FilePath: "/E:/tmp/out", ControlFilePath: "/E:/tmp/ctl"}, fields: []string{"--table", "--pos", "--file-path", "--ctl-path", "--skip-check-dir"}},
		{name: "结构化编码", input: Draft{ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{"orders"}, ContentKind: "DATA_ONLY", Format: "PARQUET", FilePath: "/E:/tmp/out", CsvOptions: store.CsvOptions{FileEncoding: "UTF-8"}}, fields: []string{"--table", "--par", "--file-path", "--skip-check-dir", "--file-encoding"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, err := BuildGeneralizedFields(tt.input)
			if err != nil {
				t.Fatalf("构建字段失败：%v", err)
			}
			if got := fieldNames(fields); !reflect.DeepEqual(got, tt.fields) {
				t.Fatalf("字段不匹配：got=%v want=%v", got, tt.fields)
			}
		})
	}
}

func TestBuildGeneralizedFieldsRejectsUnsupportedShape(t *testing.T) {
	for name, input := range map[string]Draft{
		"未知范围": {ScopeKind: "UNKNOWN", ContentKind: "DATA_ONLY", Format: "CSV"},
		"未知内容": {ScopeKind: "ALL", ContentKind: "UNKNOWN", Format: "CSV"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := BuildGeneralizedFields(input)
			if err == nil {
				t.Fatal("不支持的草稿形状应失败关闭")
			}
		})
	}
}

func TestBuildGeneralizedFieldsMapsOrdinaryQuerySQL(t *testing.T) {
	input := Draft{ScopeKind: "ALL", ContentKind: "DATA_ONLY", Format: "SQL", FilePath: "/E:/tmp/out", QuerySql: "SELECT id FROM orders"}
	fields, err := BuildGeneralizedFields(input)
	if err != nil {
		t.Fatalf("ordinary query-sql failed: %v", err)
	}
	if got := fieldNames(fields); len(got) == 0 || got[len(got)-1] != "--query-sql" {
		t.Fatalf("query-sql field missing or out of order: %v", got)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
