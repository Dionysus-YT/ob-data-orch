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
