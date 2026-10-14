package exportdomain

import (
	"testing"

	"ob-data-orch/internal/store"
)

func TestExpertParametersRequireApplicableScopeAndMode(t *testing.T) {
	base := Draft{ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{"synthetic_table"}, ContentKind: "DATA_ONLY", Format: "CSV", WeakRead: true, EnableHiddenPk: true}
	if err := ValidateExpertParameters(base); err != nil || !base.NeedsExpertMetadata() || base.HasZeroOptions() {
		t.Fatal("活动专家参数没有进入版本与失效边界")
	}
	for _, bad := range []Draft{
		{ScopeKind: "QUERY_RESULT", ContentKind: "DATA_ONLY", Format: "CSV", WeakRead: true},
		{ScopeKind: "SPECIFIED", ObjectType: "VIEW", ContentKind: "DDL_AND_DATA", EnableHiddenPk: true},
		{ScopeKind: "SPECIFIED", ObjectType: "TABLE", ContentKind: "DDL_ONLY", SequencePolicy: "RESTART"},
		{ScopeKind: "ALL", ContentKind: "DDL_ONLY", SequencePolicy: "UNKNOWN"},
	} {
		if ValidateExpertParameters(bad) == nil {
			t.Fatal("越界专家参数被接受")
		}
	}
	base.TimestampFormats = store.TimestampFormatConfig{TimeValueFormat: "HH:mm:ss", TimestampValueFormat: "yyyy-MM-dd HH:mm:ss", PreserveZeroDatetime: true}
	if ValidateExpertCompatibility(base, "MYSQL", false) != nil || ValidateExpertCompatibility(base, "ORACLE", false) == nil {
		t.Fatal("MySQL 日期时间边界失效")
	}
	base.TimestampFormats = store.TimestampFormatConfig{DateValueFormat: "yyyy-MM-dd HH:mm:ss", TimestampTzValueFormat: "yyyy-MM-dd HH:mm:ss", NlsDateFormat: "YYYY-MM-DD HH24:MI:SS"}
	if ValidateExpertCompatibility(base, "ORACLE", false) != nil || ValidateExpertCompatibility(base, "MYSQL", false) == nil {
		t.Fatal("Oracle 日期时间边界失效")
	}
	base.AddExtraMessage, base.ContentKind = true, "DDL_AND_DATA"
	if ValidateExpertCompatibility(base, "ORACLE", false) == nil || ValidateExpertCompatibility(base, "ORACLE", true) != nil {
		t.Fatal("sys 凭据前置条件失效")
	}
}

func TestExpertParameterFieldsAreComplete(t *testing.T) {
	input := Draft{ScopeKind: "ALL", ContentKind: "DDL_AND_DATA", Format: "CSV", FilePath: "/E:/synthetic-output", AddExtraMessage: true, SequencePolicy: "RESTART", WeakRead: true, EnableHiddenPk: true,
		TimestampFormats: store.TimestampFormatConfig{DateValueFormat: "date", TimeValueFormat: "time", DateTimeValueFormat: "datetime", TimestampValueFormat: "timestamp", TimestampTzValueFormat: "tz", TimestampLtzValueFormat: "ltz", NlsDateFormat: "nlsdate", NlsTimestampFormat: "nlsts", NlsTimestampTzFormat: "nlstz", PreserveZeroDatetime: true}}
	fields, err := BuildGeneralizedFields(input)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, field := range fields {
		names[field.Name] = true
		if field.Name == "--sequence-policy" && field.Value.String != "restart" {
			t.Fatal("序列策略没有转换为官方枚举")
		}
	}
	for _, name := range []string{"--weak-read", "--enable-hidden-pk", "--add-extra-message", "--sequence-policy", "--preserve-zero-datetime", "--date-value-format", "--time-value-format", "--datetime-value-format", "--timestamp-value-format", "--timestamp-tz-value-format", "--timestamp-ltz-value-format", "--nls-date-format", "--nls-timestamp-format", "--nls-timestamp-tz-format"} {
		if !names[name] {
			t.Fatalf("参数映射缺失: %s", name)
		}
	}
}
