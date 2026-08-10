package commandgen

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"ob-data-orch/internal/parammeta"
)

const testCredentialID = "11111111-1111-4111-8111-111111111111"

func TestGenerateConfirmedWindowsSlice(t *testing.T) {
	generator := mustDefaultGenerator(t)
	request := validRequest(PlatformWindowsAMD64)

	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	wantArgv := []string{
		"-h127.0.0.1",
		"-P2881",
		"-usynthetic_user@synthetic_tenant",
		"--database", "synthetic_db",
		"--table", "synthetic_table",
		"--csv",
		"--file-path", "/E:/workespace/ob-data-orch/tmp/synthetic-output",
	}
	if !reflect.DeepEqual(result.ArgvTemplate, wantArgv) {
		t.Fatalf("ArgvTemplate = %#v, want %#v", result.ArgvTemplate, wantArgv)
	}
	if containsToken(result.ArgvTemplate, "-p") {
		t.Fatal("argv must not contain the password option")
	}
	if len(result.SecretSlots) != 1 {
		t.Fatalf("SecretSlots length = %d, want 1", len(result.SecretSlots))
	}
	slot := result.SecretSlots[0]
	if slot.Target != secretSlotTarget || slot.SecurityProperty != "oceanbase.jdbc.password" {
		t.Fatalf("SecretSlots[0] = %#v", slot)
	}
	if slot.CredentialReference.CredentialID != testCredentialID || slot.CredentialReference.Revision != 1 {
		t.Fatalf("credential reference = %#v", slot.CredentialReference)
	}
	if !strings.Contains(result.RedactedCommand, "-usynthetic_user@synthetic_tenant") || !strings.Contains(result.RedactedCommand, "-p ******") {
		t.Fatalf("RedactedCommand did not retain the non-password identifier: %q", result.RedactedCommand)
	}
	if strings.Contains(result.RedactedCommand, "--password") {
		t.Fatalf("RedactedCommand contains --password: %q", result.RedactedCommand)
	}
	if len(result.ConfigFingerprint) != 64 {
		t.Fatalf("fingerprint length = %d, want 64", len(result.ConfigFingerprint))
	}
	password := findNormalizedField(t, result, "--password")
	if password.State != StateDerived || password.Value.Secret == nil {
		t.Fatalf("normalized password = %#v", password)
	}
	if len(result.TokenEvidence) != 7 {
		t.Fatalf("TokenEvidence length = %d, want 7", len(result.TokenEvidence))
	}

}

func TestGenerateOptionalLogPathAndSkipDirectoryCheck(t *testing.T) {
	generator := mustDefaultGenerator(t)
	request := validRequest(PlatformWindowsAMD64)
	request.Fields = append(request.Fields, FieldInput{Name: "--skip-check-dir", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: false}})
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() with disabled skip flag error = %v", err)
	}
	if containsToken(result.ArgvTemplate, "--skip-check-dir") {
		t.Fatalf("disabled skip flag entered argv: %#v", result.ArgvTemplate)
	}

	request = validRequest(PlatformWindowsAMD64)
	request.Fields = append(request.Fields,
		stringField("--log-path", SourceUser, "/E:/workespace/ob-data-orch/tmp/synthetic-logs"),
		FieldInput{Name: "--skip-check-dir", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	result, err = generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !containsToken(result.ArgvTemplate, "--log-path") || !containsToken(result.ArgvTemplate, "--skip-check-dir") {
		t.Fatalf("optional output arguments missing: %#v", result.ArgvTemplate)
	}
	if containsToken(result.ArgvTemplate, "--column-quote-mode") {
		t.Fatalf("column quote mode must remain unset: %#v", result.ArgvTemplate)
	}
}

func TestGenerateIsDeterministicAcrossRunsAndInputOrder(t *testing.T) {
	generator := mustDefaultGenerator(t)
	request := validRequest(PlatformWindowsAMD64)
	first, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	for run := 0; run < 100; run++ {
		reordered := request
		reordered.Fields = reverseFields(request.Fields)
		if run%2 == 0 {
			reordered.Fields = request.Fields
		}
		result, err := generator.Generate(reordered)
		if err != nil {
			t.Fatalf("Generate() run %d error = %v", run, err)
		}
		if !reflect.DeepEqual(result.ArgvTemplate, first.ArgvTemplate) {
			t.Fatalf("run %d argv changed: %#v", run, result.ArgvTemplate)
		}
		if result.ConfigFingerprint != first.ConfigFingerprint {
			t.Fatalf("run %d fingerprint = %q, want %q", run, result.ConfigFingerprint, first.ConfigFingerprint)
		}
	}
}

func TestGenerateValidatesTargetPlatformPath(t *testing.T) {
	generator := mustDefaultGenerator(t)

	linux := validRequest(PlatformLinuxARM64)
	setStringField(t, &linux, "--file-path", "/var/lib/ob-data-orch/synthetic-output")
	result, err := generator.Generate(linux)
	if err != nil {
		t.Fatalf("Linux Generate() error = %v", err)
	}
	if got := result.ArgvTemplate[len(result.ArgvTemplate)-1]; got != "/var/lib/ob-data-orch/synthetic-output" {
		t.Fatalf("Linux output path = %q", got)
	}

	tests := []struct {
		name     string
		platform Platform
		path     string
	}{
		{name: "relative Windows", platform: PlatformWindowsAMD64, path: `tmp\output`},
		{name: "Linux syntax on Windows", platform: PlatformWindowsAMD64, path: "/tmp/output"},
		{name: "legacy Windows syntax", platform: PlatformWindowsAMD64, path: `E:\tmp\output`},
		{name: "Windows syntax on Linux", platform: PlatformLinuxAMD64, path: `E:\tmp\output`},
		{name: "relative Linux", platform: PlatformLinuxARM64, path: "tmp/output"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest(test.platform)
			setStringField(t, &request, "--file-path", test.path)
			assertValidationIssue(t, generator, request, "OUTPUT_PATH_NOT_ABSOLUTE", "--file-path")
		})
	}
}

func TestGenerateFailsClosedOnRequestAndFieldErrors(t *testing.T) {
	generator := mustDefaultGenerator(t)

	tests := []struct {
		name  string
		edit  func(*Request)
		code  string
		field string
	}{
		{name: "metadata version", edit: func(r *Request) { r.MetadataVersion = "other" }, code: "METADATA_VERSION_MISMATCH", field: "metadataVersion"},
		{name: "non ODP connection", edit: func(r *Request) { r.ConnectionKind = ConnectionObserverDirect }, code: "CONNECTION_KIND_NOT_IN_SLICE", field: "connectionKind"},
		{name: "unknown parameter", edit: func(r *Request) { r.Fields = append(r.Fields, stringField("--unknown", SourceUser, "x")) }, code: "UNKNOWN_PARAMETER", field: "--unknown"},
		{name: "duplicate parameter", edit: func(r *Request) { r.Fields = append(r.Fields, stringField("--database", SourceUser, "other")) }, code: "DUPLICATE_PARAMETER", field: "--database"},
		{name: "missing required", edit: func(r *Request) { removeField(r, "--table") }, code: "REQUIRED_PARAMETER_MISSING", field: "--table"},
		{name: "source mismatch", edit: func(r *Request) { fieldByName(t, r, "--database").Source = SourceDataSource }, code: "PARAMETER_SOURCE_MISMATCH", field: "--database"},
		{name: "multiple tables", edit: func(r *Request) { setStringField(t, r, "--table", "a,b") }, code: "TABLE_SCOPE_NOT_SINGLE", field: "--table"},
		{name: "invalid port", edit: func(r *Request) { fieldByName(t, r, "--port").Value.Integer = 70000 }, code: "PARAMETER_VALUE_OUT_OF_RANGE", field: "--port"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest(PlatformWindowsAMD64)
			test.edit(&request)
			assertValidationIssue(t, generator, request, test.code, test.field)
		})
	}
}

func TestGenerateBlocksValidationGatedParameters(t *testing.T) {
	generator := mustDefaultGenerator(t)

	request := validRequest(PlatformWindowsAMD64)
	request.Fields = append(request.Fields, stringField("--column-separator", SourceUser, "|"))
	assertValidationIssue(t, generator, request, "PARAMETER_VALIDATION_GATED", "--column-separator")

	request = validRequest(PlatformWindowsAMD64)
	request.Fields = append(request.Fields, FieldInput{
		Name:   "--skip-header",
		Source: SourceUser,
		Value:  Value{Kind: ValueBoolean, Boolean: false},
	})
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("false optional flag should remain unset: %v", err)
	}
	if containsToken(result.ArgvTemplate, "--skip-header") {
		t.Fatal("false optional flag entered argv")
	}
}

func TestGenerateRequiresOnlyVersionedSecretReference(t *testing.T) {
	generator := mustDefaultGenerator(t)

	request := validRequest(PlatformWindowsAMD64)
	password := fieldByName(t, &request, "--password")
	const plaintext = "plaintext-must-never-be-accepted"
	password.Value = Value{Kind: ValueString, String: plaintext}
	if _, err := generator.Generate(request); err == nil || strings.Contains(err.Error(), plaintext) {
		t.Fatalf("plaintext rejection error = %v", err)
	}
	assertValidationIssue(t, generator, request, "SECRET_REFERENCE_REQUIRED", "--password")

	request = validRequest(PlatformWindowsAMD64)
	password = fieldByName(t, &request, "--password")
	password.Value.Secret.CredentialID = "not-a-uuid"
	assertValidationIssue(t, generator, request, "SECRET_REFERENCE_INVALID", "--password")
}

func TestInactiveStaleValueIsExcluded(t *testing.T) {
	base, err := parammeta.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault(): %v", err)
	}
	definitions := base.Definitions()
	for index := range definitions {
		if definitions[index].LongName == "--column-separator" {
			definitions[index].Activation = parammeta.Rule{Kind: "FORMAT_IS", Value: "CUT"}
		}
	}
	generator, err := newGenerator(testCatalog{
		metadataVersion:   base.MetadataVersion(),
		tool:              base.Tool(),
		toolVersion:       base.ToolVersion(),
		capabilityVersion: base.CapabilityVersion(),
		categoryOrder:     base.CategoryOrder(),
		definitions:       definitions,
	})
	if err != nil {
		t.Fatalf("newGenerator(): %v", err)
	}
	request := validRequest(PlatformWindowsAMD64)
	request.Fields = append(request.Fields, stringField("--column-separator", SourceUser, "|"))
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	field := findNormalizedField(t, result, "--column-separator")
	if field.State != StateInactive {
		t.Fatalf("field state = %s, want %s", field.State, StateInactive)
	}
	if containsToken(result.ArgvTemplate, "--column-separator") {
		t.Fatal("inactive stale value entered argv")
	}
}

func TestMetadataVersionParticipatesInFingerprint(t *testing.T) {
	base, err := parammeta.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault(): %v", err)
	}
	defaultGenerator := mustDefaultGenerator(t)
	request := validRequest(PlatformWindowsAMD64)
	first, err := defaultGenerator.Generate(request)
	if err != nil {
		t.Fatalf("default Generate(): %v", err)
	}

	const nextVersion = "obdumper-4.3.5-slice-v2-test-revision"
	revised, err := newGenerator(testCatalog{
		metadataVersion:   nextVersion,
		tool:              base.Tool(),
		toolVersion:       base.ToolVersion(),
		capabilityVersion: base.CapabilityVersion(),
		categoryOrder:     base.CategoryOrder(),
		definitions:       base.Definitions(),
	})
	if err != nil {
		t.Fatalf("newGenerator(): %v", err)
	}
	request.MetadataVersion = nextVersion
	second, err := revised.Generate(request)
	if err != nil {
		t.Fatalf("revised Generate(): %v", err)
	}
	if second.ConfigFingerprint == first.ConfigFingerprint {
		t.Fatal("metadata version change did not change fingerprint")
	}
}

type testCatalog struct {
	metadataVersion   string
	tool              string
	toolVersion       string
	capabilityVersion string
	categoryOrder     []string
	definitions       []parammeta.Definition
}

func (c testCatalog) MetadataVersion() string   { return c.metadataVersion }
func (c testCatalog) Tool() string              { return c.tool }
func (c testCatalog) ToolVersion() string       { return c.toolVersion }
func (c testCatalog) CapabilityVersion() string { return c.capabilityVersion }
func (c testCatalog) CategoryOrder() []string   { return append([]string(nil), c.categoryOrder...) }
func (c testCatalog) Definitions() []parammeta.Definition {
	return append([]parammeta.Definition(nil), c.definitions...)
}

func mustDefaultGenerator(t *testing.T) *Generator {
	t.Helper()
	generator, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault(): %v", err)
	}
	return generator
}

func mustGeneralizedGenerator(t *testing.T) *Generator {
	t.Helper()
	generator, err := NewGeneralized()
	if err != nil {
		t.Fatalf("NewGeneralized(): %v", err)
	}
	return generator
}

// validGeneralizedRequest 构造泛化目录下的基础请求：只含连接与输出字段，对象与内容字段由用例追加。
func validGeneralizedRequest(platform Platform, capability string) Request {
	return Request{
		Tool:                  "OBDUMPER",
		ToolVersion:           "4.3.5-RELEASE",
		MetadataVersion:       "obdumper-4.3.5-slice-v6",
		CapabilityVersion:     capability,
		ConnectionKind:        ConnectionODP,
		DataSourceFactVersion: "ds-rev-1",
		NodeFactVersion:       "node-facts-1",
		TargetPlatform:        platform,
		Fields: []FieldInput{
			stringField("--host", SourceDataSource, "127.0.0.1"),
			{Name: "--port", Source: SourceDataSource, Value: Value{Kind: ValueInteger, Integer: 2881}},
			stringField("--user", SourceDataSource, "synthetic_user@synthetic_tenant"),
			{
				Name:   "--password",
				Source: SourceSecurity,
				Value: Value{Kind: ValueSecretReference, Secret: &CredentialReference{
					CredentialID: testCredentialID,
					Revision:     1,
				}},
			},
			stringField("--database", SourceUser, "synthetic_db"),
			stringField("--file-path", SourceUser, windowsOrLinuxPath(platform)),
		},
	}
}

func TestGeneralizedFullCSVAllScope(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	request.Fields = append(request.Fields,
		FieldInput{Name: "--all", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	wantArgv := []string{
		"-h127.0.0.1", "-P2881", "-usynthetic_user@synthetic_tenant",
		"--database", "synthetic_db",
		"--all", "--csv",
		"--file-path", "/E:/workespace/ob-data-orch/tmp/synthetic-output",
	}
	if !reflect.DeepEqual(result.ArgvTemplate, wantArgv) {
		t.Fatalf("ArgvTemplate = %#v, want %#v", result.ArgvTemplate, wantArgv)
	}
	if result.CapabilityVersion != "export-odp-full-csv-v1" || containsToken(result.ArgvTemplate, "-p") {
		t.Fatalf("unexpected capability or password token: %#v", result)
	}
}

func TestGeneralizedFullCSVMultiTableWithExclusions(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "table_one,table_two"),
		stringField("--exclude-table", SourceUser, "table_tmp"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !containsToken(result.ArgvTemplate, "-ttable_one,table_two") || !containsToken(result.ArgvTemplate, "--exclude-table") || !containsToken(result.ArgvTemplate, "table_tmp") {
		t.Fatalf("multi-table argv missing: %#v", result.ArgvTemplate)
	}
}

func TestGeneralizedDDLOnlyOmitsDataFormatParameters(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-v1")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "table_one"),
		FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	wantArgv := []string{
		"-h127.0.0.1", "-P2881", "-usynthetic_user@synthetic_tenant",
		"--database", "synthetic_db",
		"-ttable_one", "--ddl",
		"--file-path", "/E:/workespace/ob-data-orch/tmp/synthetic-output",
	}
	if !reflect.DeepEqual(result.ArgvTemplate, wantArgv) {
		t.Fatalf("ArgvTemplate = %#v, want %#v", result.ArgvTemplate, wantArgv)
	}
	if containsToken(result.ArgvTemplate, "--csv") {
		t.Fatalf("DDL-only command must not contain data format parameters: %#v", result.ArgvTemplate)
	}
}

func TestGeneralizedDDLAndCSVCombinesContentFlags(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-csv-v1")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "table_one,table_two"),
		FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !containsToken(result.ArgvTemplate, "--ddl") || !containsToken(result.ArgvTemplate, "--csv") || !containsToken(result.ArgvTemplate, "-ttable_one,table_two") {
		t.Fatalf("DDL+CSV argv missing: %#v", result.ArgvTemplate)
	}
}

func TestGeneralizedViewDDLUsesViewParameter(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-v1")
	request.Fields = append(request.Fields,
		stringField("--view", SourceUser, "view_one"),
		FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !containsToken(result.ArgvTemplate, "--view") || !containsToken(result.ArgvTemplate, "view_one") || containsToken(result.ArgvTemplate, "-t") {
		t.Fatalf("view DDL argv unexpected: %#v", result.ArgvTemplate)
	}
}

func TestGeneralizedCapabilitySubsetsFailClosed(t *testing.T) {
	generator := mustGeneralizedGenerator(t)

	// --view 只绑定 DDL 能力，在 full-csv 中提供按未知参数失败关闭。
	fullWithView := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	fullWithView.Fields = append(fullWithView.Fields,
		stringField("--view", SourceUser, "view_one"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, fullWithView, "UNKNOWN_PARAMETER", "--view")

	// 冻结单表能力仍拒绝逗号多表。
	multiTable := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-single-table-csv-v1")
	multiTable.Fields = append(multiTable.Fields,
		stringField("--table", SourceUser, "table_one,table_two"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, multiTable, "TABLE_SCOPE_NOT_SINGLE", "--table")

	// 通配表达式在任何能力下都失败关闭。
	wildcard := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	wildcard.Fields = append(wildcard.Fields,
		stringField("--table", SourceUser, "table_*"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, wildcard, "TABLE_SCOPE_NOT_SINGLE", "--table")

	wildcardView := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-v1")
	wildcardView.Fields = append(wildcardView.Fields,
		stringField("--view", SourceUser, "view_*"),
		FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, wildcardView, "PARAMETER_VALUE_INVALID", "--view")

	// 逗号列表中出现空名称失败关闭。
	emptyName := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	emptyName.Fields = append(emptyName.Fields,
		stringField("--table", SourceUser, "table_one,,table_two"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, emptyName, "PARAMETER_VALUE_INVALID", "--table")

	// v5 冻结目录不认识泛化参数。
	defaultGenerator := mustDefaultGenerator(t)
	v5WithAll := validRequest(PlatformWindowsAMD64)
	v5WithAll.Fields = append(v5WithAll.Fields, FieldInput{Name: "--all", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}})
	assertValidationIssue(t, defaultGenerator, v5WithAll, "UNKNOWN_PARAMETER", "--all")
}

// TestGeneralizedEXI3ParameterRules 验证 EX-I3 参数的激活、清值、枚举与整数边界。
func TestGeneralizedEXI3ParameterRules(t *testing.T) {
	generator := mustGeneralizedGenerator(t)

	// 全量 CSV 选项与压缩/筛选/资源参数按元数据进入 argv。
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--skip-header", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--column-separator", SourceUser, "|"),
		stringField("--column-quote-mode", SourceUser, "minimal"),
		FieldInput{Name: "--with-trim", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--compress", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--compression-algo", SourceUser, "zstd"),
		FieldInput{Name: "--max-file-size", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 1048576}},
		stringField("--query-sql", SourceUser, "select 1"),
		stringField("--include-column-names", SourceUser, "col_a,col_b"),
		FieldInput{Name: "--flashback-scn", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 100}},
		FieldInput{Name: "--thread", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 4}},
		stringField("--mem", SourceUser, "4G"),
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	for _, token := range []string{"--skip-header", "--column-separator", "|", "--column-quote-mode", "minimal", "--with-trim", "--compress", "--compression-algo", "zstd", "--max-file-size", "1048576", "--query-sql", "select 1", "--include-column-names", "col_a,col_b", "--flashback-scn", "100", "--thread", "4", "--mem", "4G"} {
		if !containsToken(result.ArgvTemplate, token) {
			t.Fatalf("EX-I3 argv missing %q: %#v", token, result.ArgvTemplate)
		}
	}

	// 清值：flag 为 false 或选项为空时不进入 argv。
	cleared := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	cleared.Fields = append(cleared.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--skip-header", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: false}},
		FieldInput{Name: "--compress", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: false}},
	)
	clearedResult, err := generator.Generate(cleared)
	if err != nil {
		t.Fatalf("cleared Generate() error = %v", err)
	}
	if containsToken(clearedResult.ArgvTemplate, "--skip-header") || containsToken(clearedResult.ArgvTemplate, "--compress") {
		t.Fatalf("cleared flags entered argv: %#v", clearedResult.ArgvTemplate)
	}

	// 枚举与类型边界失败关闭。
	invalidMode := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	invalidMode.Fields = append(invalidMode.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--column-quote-mode", SourceUser, "bogus"),
	)
	assertValidationIssue(t, generator, invalidMode, "PARAMETER_ENUM_INVALID", "--column-quote-mode")

	invalidAlgo := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	invalidAlgo.Fields = append(invalidAlgo.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--compress", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--compression-algo", SourceUser, "brotli"),
	)
	assertValidationIssue(t, generator, invalidAlgo, "PARAMETER_ENUM_INVALID", "--compression-algo")

	typeMismatch := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	typeMismatch.Fields = append(typeMismatch.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--max-file-size", SourceUser, "not-a-number"),
	)
	assertValidationIssue(t, generator, typeMismatch, "PARAMETER_TYPE_MISMATCH", "--max-file-size")

	// 仅 DDL 能力下 CSV 序列化参数因 FORMAT_IS 非活动而未被要求，也不会被输出。
	ddlRequest := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-v1")
	ddlRequest.Fields = append(ddlRequest.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	ddlResult, err := generator.Generate(ddlRequest)
	if err != nil {
		t.Fatalf("ddl Generate() error = %v", err)
	}
	if containsToken(ddlResult.ArgvTemplate, "--skip-header") || containsToken(ddlResult.ArgvTemplate, "--compress") {
		t.Fatalf("ddl-only argv must not carry data options: %#v", ddlResult.ArgvTemplate)
	}
}

// TestGeneralizedCUTFormat 验证 CUT 能力：--cut 与 CUT 专属/共享文本/压缩参数进入 argv。
func TestGeneralizedCUTFormat(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-cut-v1")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--cut", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--trail-delimiter", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--remove-newline", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--column-splitter", SourceUser, "|"),
		stringField("--escape-character", SourceUser, "\\"),
		stringField("--line-separator", SourceUser, "\\n"),
		stringField("--null-string", SourceUser, "NULL"),
		stringField("--file-encoding", SourceUser, "UTF-8"),
		FieldInput{Name: "--with-trim", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--compress", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--compression-algo", SourceUser, "zstd"),
		// 官方复核：文件布局、筛选与性能参数不限定格式，CUT 能力同样生成。
		FieldInput{Name: "--no-nested-dir", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--max-file-size", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 1048576}},
		FieldInput{Name: "--retain-empty-files", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--query-sql", SourceUser, "select 1"),
		stringField("--include-column-names", SourceUser, "col_a,col_b"),
		FieldInput{Name: "--exclude-virtual-columns", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--flashback-scn", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 100}},
		FieldInput{Name: "--thread", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 4}},
		FieldInput{Name: "--page-size", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 1000}},
		FieldInput{Name: "--parallel-macro", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 8}},
		FieldInput{Name: "--fetch-size", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 100}},
		stringField("--mem", SourceUser, "4G"),
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.CapabilityVersion != "export-odp-cut-v1" {
		t.Fatalf("capability = %s", result.CapabilityVersion)
	}
	for _, token := range []string{"--cut", "--trail-delimiter", "--remove-newline", "--column-splitter", "|", "--escape-character", "--line-separator", "--null-string", "NULL", "--file-encoding", "UTF-8", "--with-trim", "--compress", "--compression-algo", "zstd", "--no-nested-dir", "--max-file-size", "1048576", "--retain-empty-files", "--query-sql", "select 1", "--include-column-names", "col_a,col_b", "--exclude-virtual-columns", "--flashback-scn", "100", "--thread", "4", "--page-size", "1000", "--parallel-macro", "8", "--fetch-size", "100", "--mem", "4G"} {
		if !containsToken(result.ArgvTemplate, token) {
			t.Fatalf("CUT argv missing %q: %#v", token, result.ArgvTemplate)
		}
	}
	for _, forbidden := range []string{"--csv", "--skip-header", "--column-separator", "--column-quote", "--column-quote-mode"} {
		if containsToken(result.ArgvTemplate, forbidden) {
			t.Fatalf("CUT argv must not contain %q: %#v", forbidden, result.ArgvTemplate)
		}
	}
	// CSV 专属 base 字段未声明能力切片：CUT 下按非活动忽略，不进入 argv，也不报错。
	skipHeader := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-cut-v1")
	skipHeader.Fields = append(skipHeader.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--cut", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--skip-header", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	skipResult, err := generator.Generate(skipHeader)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if containsToken(skipResult.ArgvTemplate, "--skip-header") {
		t.Fatalf("CUT argv must not contain --skip-header: %#v", skipResult.ArgvTemplate)
	}
	if field := findNormalizedField(t, skipResult, "--skip-header"); field.State != StateInactive {
		t.Fatalf("--skip-header state = %s, want %s", field.State, StateInactive)
	}
	// 官方复核：筛选与资源参数绑定 CUT 能力，正常进入 argv 而不是被拒绝。
	querySQL := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-cut-v1")
	querySQL.Fields = append(querySQL.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--cut", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--query-sql", SourceUser, "select 1"),
		FieldInput{Name: "--thread", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 4}},
	)
	queryResult, err := generator.Generate(querySQL)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	for _, token := range []string{"--query-sql", "select 1", "--thread", "4"} {
		if !containsToken(queryResult.ArgvTemplate, token) {
			t.Fatalf("CUT argv missing %q: %#v", token, queryResult.ArgvTemplate)
		}
	}
	// 闪回时间点仅 Oracle 兼容模式，但 CUT 能力下属于已绑定参数，命令生成不再拒绝。
	flashback := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-cut-v1")
	flashback.Fields = append(flashback.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--cut", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--flashback-timestamp", SourceUser, "2026-08-06 00:00:00"),
	)
	flashbackResult, err := generator.Generate(flashback)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !containsToken(flashbackResult.ArgvTemplate, "--flashback-timestamp") {
		t.Fatalf("CUT argv missing --flashback-timestamp: %#v", flashbackResult.ArgvTemplate)
	}
}

// TestGeneralizedSQLFormat 验证 SQL 能力：--sql 与行分隔符/文件编码进入 argv，CUT 专属参数失败关闭。
func TestGeneralizedSQLFormat(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-sql-v1")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--sql", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--line-separator", SourceUser, "\\n"),
		stringField("--file-encoding", SourceUser, "UTF-8"),
		FieldInput{Name: "--compress", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.CapabilityVersion != "export-odp-sql-v1" {
		t.Fatalf("capability = %s", result.CapabilityVersion)
	}
	for _, token := range []string{"--sql", "--line-separator", "--file-encoding", "UTF-8"} {
		if !containsToken(result.ArgvTemplate, token) {
			t.Fatalf("SQL argv missing %q: %#v", token, result.ArgvTemplate)
		}
	}
	for _, forbidden := range []string{"--csv", "--cut", "--escape-character", "--null-string", "--with-trim", "--trail-delimiter", "--remove-newline", "--skip-header"} {
		if containsToken(result.ArgvTemplate, forbidden) {
			t.Fatalf("SQL argv must not contain %q: %#v", forbidden, result.ArgvTemplate)
		}
	}
	// 官方复核：文件布局、筛选与性能参数不限定格式，SQL 能力同样生成。
	layout := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-sql-v1")
	layout.Fields = append(layout.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--sql", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--no-nested-dir", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--retain-empty-files", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--query-sql", SourceUser, "select 1"),
		stringField("--exclude-column-names", SourceUser, "col_c"),
		FieldInput{Name: "--flashback-scn", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 100}},
		FieldInput{Name: "--thread", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 4}},
		FieldInput{Name: "--fetch-size", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 100}},
		stringField("--mem", SourceUser, "4G"),
	)
	layoutResult, err := generator.Generate(layout)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	for _, token := range []string{"--no-nested-dir", "--retain-empty-files", "--query-sql", "select 1", "--exclude-column-names", "col_c", "--flashback-scn", "100", "--thread", "4", "--fetch-size", "100", "--mem", "4G"} {
		if !containsToken(layoutResult.ArgvTemplate, token) {
			t.Fatalf("SQL argv missing %q: %#v", token, layoutResult.ArgvTemplate)
		}
	}
	// 共享文本与 CUT 专属参数不绑定 SQL 能力。
	withTrim := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-sql-v1")
	withTrim.Fields = append(withTrim.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--sql", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--with-trim", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, withTrim, "UNKNOWN_PARAMETER", "--with-trim")
	trail := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-sql-v1")
	trail.Fields = append(trail.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--sql", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--trail-delimiter", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, trail, "UNKNOWN_PARAMETER", "--trail-delimiter")
}

// TestGeneralizedPOSFormat 验证 EX-I4 POS 定版（2026-08-07 实测）：
// 独立 --pos 必须搭配 --ctl-path 控制文件目录，通用文件布局/筛选/性能参数随 POS 能力进入 argv；
// 其他格式参数在 POS 能力下失败关闭，POS 参数在其他格式能力下失败关闭。
func TestGeneralizedPOSFormat(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-pos-v1")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--pos", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--ctl-path", SourceUser, "/E:/workespace/ob-data-orch/tmp/synthetic-controls"),
		FieldInput{Name: "--compress", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--compression-algo", SourceUser, "zstd"),
		FieldInput{Name: "--no-nested-dir", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--query-sql", SourceUser, "select 1"),
		FieldInput{Name: "--thread", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 4}},
		stringField("--mem", SourceUser, "4G"),
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.CapabilityVersion != "export-odp-pos-v1" {
		t.Fatalf("capability = %s", result.CapabilityVersion)
	}
	for _, token := range []string{"--pos", "--ctl-path", "/E:/workespace/ob-data-orch/tmp/synthetic-controls", "--compress", "--compression-algo", "zstd", "--no-nested-dir", "--query-sql", "select 1", "--thread", "4", "--mem", "4G"} {
		if !containsToken(result.ArgvTemplate, token) {
			t.Fatalf("POS argv missing %q: %#v", token, result.ArgvTemplate)
		}
	}
	for _, forbidden := range []string{"--csv", "--cut", "--sql", "--skip-header", "--trail-delimiter", "--column-splitter", "--with-trim", "--escape-character", "--null-string"} {
		if containsToken(result.ArgvTemplate, forbidden) {
			t.Fatalf("POS argv must not contain %q: %#v", forbidden, result.ArgvTemplate)
		}
	}
	// POS 能力下其他格式参数失败关闭（能力过滤先于互斥检测，因此为未知参数）。
	cutInPOS := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-pos-v1")
	cutInPOS.Fields = append(cutInPOS.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--pos", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--ctl-path", SourceUser, "/E:/workespace/ob-data-orch/tmp/synthetic-controls"),
		FieldInput{Name: "--cut", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, cutInPOS, "UNKNOWN_PARAMETER", "--cut")
	// CSV 能力下 POS 参数失败关闭；POS 能力下 CUT 专属参数失败关闭。
	csvWithPOS := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	csvWithPOS.Fields = append(csvWithPOS.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--pos", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, csvWithPOS, "UNKNOWN_PARAMETER", "--pos")
	posWithSplitter := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-pos-v1")
	posWithSplitter.Fields = append(posWithSplitter.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--pos", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--ctl-path", SourceUser, "/E:/workespace/ob-data-orch/tmp/synthetic-controls"),
		stringField("--column-splitter", SourceUser, "|"),
	)
	assertValidationIssue(t, generator, posWithSplitter, "UNKNOWN_PARAMETER", "--column-splitter")
}

// TestGeneralizedStructuredFormats 验证 EX-I5 结构化格式（2026-08-07）：
// --par/--orc/--avro 各自能力进入 argv，文件编码与通用筛选/性能/文件布局参数随格式表活动；
// 压缩（仅可读格式）与序列化选项对结构化格式失败关闭。
func TestGeneralizedStructuredFormats(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	for _, testCase := range []struct {
		capability string
		formatArg  string
	}{
		{"export-odp-parquet-v1", "--par"},
		{"export-odp-orc-v1", "--orc"},
		{"export-odp-avro-v1", "--avro"},
	} {
		request := validGeneralizedRequest(PlatformWindowsAMD64, testCase.capability)
		request.Fields = append(request.Fields,
			stringField("--table", SourceUser, "synthetic_table"),
			FieldInput{Name: testCase.formatArg, Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
			stringField("--file-encoding", SourceUser, "UTF-8"),
			FieldInput{Name: "--flashback-scn", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 100}},
			FieldInput{Name: "--thread", Source: SourceUser, Value: Value{Kind: ValueInteger, Integer: 4}},
			stringField("--mem", SourceUser, "4G"),
		)
		result, err := generator.Generate(request)
		if err != nil {
			t.Fatalf("%s Generate() error = %v", testCase.capability, err)
		}
		if result.CapabilityVersion != testCase.capability {
			t.Fatalf("capability = %s", result.CapabilityVersion)
		}
		for _, token := range []string{testCase.formatArg, "--file-encoding", "UTF-8", "--flashback-scn", "100", "--thread", "4", "--mem", "4G"} {
			if !containsToken(result.ArgvTemplate, token) {
				t.Fatalf("%s argv missing %q: %#v", testCase.capability, token, result.ArgvTemplate)
			}
		}
		for _, forbidden := range []string{"--csv", "--cut", "--pos", "--sql", "--compress", "--escape-character", "--null-string", "--line-separator", "--with-trim", "--skip-header", "--ctl-path"} {
			if containsToken(result.ArgvTemplate, forbidden) {
				t.Fatalf("%s argv must not contain %q: %#v", testCase.capability, forbidden, result.ArgvTemplate)
			}
		}
	}
	// 压缩只支持可读格式：结构化能力下 --compress 为未知参数。
	parquetWithCompress := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-parquet-v1")
	parquetWithCompress.Fields = append(parquetWithCompress.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--par", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--compress", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, parquetWithCompress, "UNKNOWN_PARAMETER", "--compress")
	// 序列化选项对结构化格式失败关闭（未绑定能力）。
	// base 参数（如转义字符）未声明能力切片时按非活动忽略，不进入 argv 也不报错。
	orcWithEscape := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-orc-v1")
	orcWithEscape.Fields = append(orcWithEscape.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--orc", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--escape-character", SourceUser, "\\"),
	)
	orcResult, err := generator.Generate(orcWithEscape)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if containsToken(orcResult.ArgvTemplate, "--escape-character") {
		t.Fatalf("ORC argv must not contain --escape-character: %#v", orcResult.ArgvTemplate)
	}
	if field := findNormalizedField(t, orcResult, "--escape-character"); field.State != StateInactive {
		t.Fatalf("--escape-character state = %s, want %s", field.State, StateInactive)
	}
	// 可读格式能力下结构化格式参数为未知参数。
	csvWithPar := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	csvWithPar.Fields = append(csvWithPar.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--par", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, csvWithPar, "UNKNOWN_PARAMETER", "--par")
}

// TestGeneralizedObjectStorageOutput 验证 EX-I6 对象存储输出（2026-08-07）：
// --file-path 在受控对象存储输出下按 URI 校验（无密钥参数），--tmp-path 随能力发射；
// 密钥参数进 URI、scheme 与输出类型不符、未知查询参数失败关闭。
func TestGeneralizedObjectStorageOutput(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	request := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	request.OutputKind = "OSS"
	removeField(&request, "--file-path")
	request.Fields = append(request.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--file-path", SourceUser, "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou-internal.aliyuncs.com"),
		stringField("--tmp-path", SourceUser, "/E:/workespace/ob-data-orch/tmp/synthetic-upload-buffer"),
	)
	result, err := generator.Generate(request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	for _, token := range []string{"oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou-internal.aliyuncs.com", "--tmp-path", "/E:/workespace/ob-data-orch/tmp/synthetic-upload-buffer"} {
		if !containsToken(result.ArgvTemplate, token) {
			t.Fatalf("OSS argv missing %q: %#v", token, result.ArgvTemplate)
		}
	}
	// 本地输出下 OSS URI 仍按绝对路径校验失败关闭。
	localWithURI := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	localWithURI.OutputKind = "LOCAL"
	removeField(&localWithURI, "--file-path")
	localWithURI.Fields = append(localWithURI.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		stringField("--file-path", SourceUser, "oss://bucket/path"),
	)
	assertValidationIssue(t, generator, localWithURI, "OUTPUT_PATH_NOT_ABSOLUTE", "--file-path")
	// 对象存储输出下密钥参数进 URI 失败关闭；scheme 与输出类型不符失败关闭；未知查询参数失败关闭。
	for _, testCase := range []struct {
		name     string
		output   string
		filePath string
	}{
		{"URI 携带密钥参数", "OSS", "oss://bucket/path?endpoint=x&access-key=AK&secret-key=SK"},
		{"scheme 与类型不符", "OSS", "s3://bucket/path?endpoint=x"},
		{"未知查询参数", "S3", "s3://bucket/path?region=x&unknown=y"},
		{"缺 bucket", "COS", "cos:///path?region=x"},
	} {
		invalid := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
		invalid.OutputKind = testCase.output
		removeField(&invalid, "--file-path")
		invalid.Fields = append(invalid.Fields,
			stringField("--table", SourceUser, "synthetic_table"),
			FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
			stringField("--file-path", SourceUser, testCase.filePath),
		)
		assertValidationIssue(t, generator, invalid, "STORAGE_URI_INVALID", "--file-path")
	}
}

// TestGeneralizedFormatSelectionFailsClosed 验证 CSV/CUT/SQL 单选规则与能力越界。
func TestGeneralizedFormatSelectionFailsClosed(t *testing.T) {
	generator := mustGeneralizedGenerator(t)

	// 元数据误配置导致 CSV/CUT/SQL 同时参与同一能力时，格式单选仍失败关闭。
	base, err := parammeta.LoadGeneralized()
	if err != nil {
		t.Fatalf("LoadGeneralized(): %v", err)
	}
	definitions := base.Definitions()
	for index := range definitions {
		switch definitions[index].LongName {
		case "--cut", "--sql":
			definitions[index].CapabilityVersions = []string{"export-odp-full-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1"}
		}
	}
	mutexGenerator, err := newGenerator(testCatalog{
		metadataVersion:   base.MetadataVersion(),
		tool:              base.Tool(),
		toolVersion:       base.ToolVersion(),
		capabilityVersion: base.CapabilityVersion(),
		categoryOrder:     base.CategoryOrder(),
		definitions:       definitions,
	})
	if err != nil {
		t.Fatalf("newGenerator(): %v", err)
	}
	multiFormat := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	multiFormat.Fields = append(multiFormat.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--cut", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--sql", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, mutexGenerator, multiFormat, "MUTUALLY_EXCLUSIVE_FORMATS", "--cut")
	assertValidationIssue(t, mutexGenerator, multiFormat, "MUTUALLY_EXCLUSIVE_FORMATS", "--sql")

	// 真元数据下 CUT 参数在 CSV 能力中按未知参数失败关闭。
	csvWithTrail := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-full-csv-v1")
	csvWithTrail.Fields = append(csvWithTrail.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--trail-delimiter", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, csvWithTrail, "UNKNOWN_PARAMETER", "--trail-delimiter")

	// 真元数据下 CUT 专属序列化参数在 SQL 能力中按未知参数失败关闭。
	sqlWithTrail := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-sql-v1")
	sqlWithTrail.Fields = append(sqlWithTrail.Fields,
		stringField("--table", SourceUser, "synthetic_table"),
		FieldInput{Name: "--sql", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		FieldInput{Name: "--trail-delimiter", Source: SourceUser, Value: Value{Kind: ValueBoolean, Boolean: true}},
	)
	assertValidationIssue(t, generator, sqlWithTrail, "UNKNOWN_PARAMETER", "--trail-delimiter")
}

func TestGeneralizedFingerprintDiffersAcrossCapabilities(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	fields := func() []FieldInput {
		return []FieldInput{
			stringField("--table", SourceUser, "table_one"),
			FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
			FieldInput{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
		}
	}
	ddlRequest := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-v1")
	ddlRequest.Fields = append(ddlRequest.Fields, fields()...)
	ddlResult, err := generator.Generate(ddlRequest)
	if err != nil {
		t.Fatalf("ddl Generate() error = %v", err)
	}
	combined := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-csv-v1")
	combined.Fields = append(combined.Fields, fields()...)
	combinedResult, err := generator.Generate(combined)
	if err != nil {
		t.Fatalf("ddl-csv Generate() error = %v", err)
	}
	if ddlResult.ConfigFingerprint == combinedResult.ConfigFingerprint {
		t.Fatalf("fingerprint must differ across capabilities: %s", ddlResult.ConfigFingerprint)
	}
}

func validRequest(platform Platform) Request {
	return Request{
		Tool:                  "OBDUMPER",
		ToolVersion:           "4.3.5-RELEASE",
		MetadataVersion:       "obdumper-4.3.5-slice-v5",
		CapabilityVersion:     "export-odp-single-table-csv-v1",
		ConnectionKind:        ConnectionODP,
		DataSourceFactVersion: "ds-rev-1",
		NodeFactVersion:       "node-facts-1",
		TargetPlatform:        platform,
		Fields: []FieldInput{
			stringField("--host", SourceDataSource, "127.0.0.1"),
			{Name: "--port", Source: SourceDataSource, Value: Value{Kind: ValueInteger, Integer: 2881}},
			stringField("--user", SourceDataSource, "synthetic_user@synthetic_tenant"),
			{
				Name:   "--password",
				Source: SourceSecurity,
				Value: Value{Kind: ValueSecretReference, Secret: &CredentialReference{
					CredentialID: testCredentialID,
					Revision:     1,
				}},
			},
			stringField("--database", SourceUser, "synthetic_db"),
			stringField("--table", SourceUser, "synthetic_table"),
			{Name: "--csv", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}},
			stringField("--file-path", SourceUser, windowsOrLinuxPath(platform)),
		},
	}
}

func windowsOrLinuxPath(platform Platform) string {
	if platform == PlatformWindowsAMD64 {
		return "/E:/workespace/ob-data-orch/tmp/synthetic-output"
	}
	return "/var/lib/ob-data-orch/synthetic-output"
}

func stringField(name string, source ValueSource, value string) FieldInput {
	return FieldInput{Name: name, Source: source, Value: Value{Kind: ValueString, String: value}}
}

func fieldByName(t *testing.T, request *Request, name string) *FieldInput {
	t.Helper()
	for index := range request.Fields {
		if request.Fields[index].Name == name {
			return &request.Fields[index]
		}
	}
	t.Fatalf("field %s not found", name)
	return nil
}

func setStringField(t *testing.T, request *Request, name, value string) {
	t.Helper()
	fieldByName(t, request, name).Value = Value{Kind: ValueString, String: value}
}

func removeField(request *Request, name string) {
	for index := range request.Fields {
		if request.Fields[index].Name == name {
			request.Fields = append(request.Fields[:index], request.Fields[index+1:]...)
			return
		}
	}
}

func reverseFields(fields []FieldInput) []FieldInput {
	result := append([]FieldInput(nil), fields...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func assertValidationIssue(t *testing.T, generator *Generator, request Request, code, field string) {
	t.Helper()
	result, err := generator.Generate(request)
	if err == nil {
		t.Fatalf("Generate() result = %#v, want validation error", result)
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("Generate() returned partial output on failure: %#v", result)
	}
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Generate() error type = %T, want *ValidationError", err)
	}
	for _, issue := range validation.Issues {
		if issue.Code == code && issue.Field == field {
			return
		}
	}
	t.Fatalf("issues = %#v, want %s/%s", validation.Issues, code, field)
}

func findNormalizedField(t *testing.T, result Result, name string) NormalizedField {
	t.Helper()
	for _, field := range result.NormalizedFields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("normalized field %s not found", name)
	return NormalizedField{}
}

func containsToken(tokens []string, target string) bool {
	for _, token := range tokens {
		if token == target {
			return true
		}
	}
	return false
}
