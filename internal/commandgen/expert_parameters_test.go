package commandgen

import (
	"strings"
	"testing"
)

func boolField(name string, value bool) FieldInput {
	return FieldInput{Name: name, Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: value}}
}

func TestOracleNlsSQLParameterGeneration(t *testing.T) {
	req := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-sql-v1")
	req.MetadataVersion = "obdumper-4.3.5-slice-v11-expert-parameters"
	req.Fields = append(req.Fields, boolField("--all", true), FieldInput{Name: "--sql", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}}, stringField("--nls-date-format", SourceUser, "YYYY-MM-DD HH24:MI:SS"))
	result, err := mustGeneralizedGenerator(t).Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	if !containsToken(result.ArgvTemplate, "--nls-date-format") {
		t.Fatal("Oracle SQL 会话格式未发射")
	}
}

func TestExpertMetadataPreservesOldGatesAndSysIsolation(t *testing.T) {
	g := mustGeneralizedGenerator(t)
	req := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-csv-v1")
	req.MetadataVersion = "obdumper-4.3.5-slice-v11-expert-parameters"
	req.Fields = append(req.Fields, boolField("--all", true), FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}}, FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		boolField("--weak-read", true), boolField("--enable-hidden-pk", true), boolField("--add-extra-message", true), boolField("--preserve-zero-datetime", true),
		stringField("--sequence-policy", SourceUser, "restart"), stringField("--time-value-format", SourceUser, "HH:mm:ss"), stringField("--timestamp-value-format", SourceUser, "yyyy-MM-dd HH:mm:ss"),
		stringField("--sys-user", SourceSecurity, "synthetic_sys_account"), FieldInput{Name: "--sys-password", Source: SourceSecurity, Value: Value{Kind: ValueSecretReference, Secret: &CredentialReference{CredentialID: "33333333-3333-4333-8333-333333333333", Revision: 2}}})
	result, err := g.Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SecretSlots) != 2 || containsToken(result.ArgvTemplate, "--sys-password") || strings.Contains(result.RedactedCommand, "synthetic_sys_account") {
		t.Fatal("sys 槽位或显示脱敏失效")
	}
	withoutSys := req
	withoutSys.Fields = append([]FieldInput(nil), req.Fields[:len(req.Fields)-2]...)
	if _, err := g.Generate(withoutSys); err == nil {
		t.Fatal("附加定义信息没有要求 sys 槽位")
	}
	legacy := req
	legacy.MetadataVersion = "obdumper-4.3.5-slice-v7"
	if _, err := g.Generate(legacy); err == nil {
		t.Fatal("旧元数据被静默解锁")
	}
}
