package contracts

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestOpenAPICoversConfirmedOperations(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	expected := map[string][]string{
		"/api/v1/session":                                         {"get"},
		"/api/v1/execution-nodes":                                 {"get", "post"},
		"/api/v1/execution-nodes/{nodeId}":                        {"get", "patch", "delete"},
		"/api/v1/execution-nodes/{nodeId}:enrollments":            {"post"},
		"/api/v1/execution-nodes/{nodeId}:environment-check":      {"post"},
		"/api/v1/execution-nodes/{nodeId}:enable":                 {"post"},
		"/api/v1/data-sources":                                    {"get", "post"},
		"/api/v1/data-sources/{dataSourceId}":                     {"get", "patch", "delete"},
		"/api/v1/data-sources/{dataSourceId}:archive":             {"post"},
		"/api/v1/data-sources/{dataSourceId}:test-connection":     {"post"},
		"/api/v1/data-source-connection-tests/{connectionTestId}": {"get"},
		"/api/v1/data-sources/{dataSourceId}:disable":             {"post"},
		"/api/v1/data-sources/{dataSourceId}:enable":              {"post"},
		"/api/v1/export-config-templates":                         {"get", "post"},
		"/api/v1/export-config-templates/{templateId}":            {"get", "patch", "delete"},
		"/api/v1/export-drafts":                                   {"post"},
		"/api/v1/export-drafts/{draftId}":                         {"get", "patch"},
		"/api/v1/export-drafts/{draftId}:precheck":                {"post"},
		"/api/v1/prechecks/{precheckId}":                          {"get"},
		"/api/v1/export-drafts/{draftId}:preview-command":         {"post"},
		"/api/v1/export-drafts/{draftId}:submit":                  {"post"},
		"/api/v1/tasks":                                           {"get"},
		"/api/v1/tasks/{taskId}":                                  {"get"},
		"/api/v1/tasks/{taskId}/snapshot":                         {"get"},
		"/api/v1/tasks/{taskId}/command-evidence":                 {"get"},
		"/api/v1/tasks/{taskId}/execution":                        {"get"},
		"/api/v1/tasks/{taskId}/logs":                             {"get"},
		"/api/v1/tasks/{taskId}/logs/stream":                      {"get"},
		"/api/v1/tasks/{taskId}/logs:download":                    {"post"},
		"/agent/v1/enrollments:exchange":                          {"post"},
		"/agent/v1/runtime-configuration:sync":                    {"post"},
		"/agent/v1/heartbeats":                                    {"post"},
		"/agent/v1/executions:claim":                              {"post"},
		"/agent/v1/executions/{executionId}:acknowledge-lease":    {"post"},
		"/agent/v1/executions/{executionId}:renew-lease":          {"post"},
		"/agent/v1/executions/{executionId}:poll-control":         {"post"},
		"/agent/v1/executions/{executionId}/secret-slots:resolve": {"post"},
		"/agent/v1/executions/{executionId}/events:append":        {"post"},
		"/agent/v1/executions/{executionId}/logs:append":          {"post"},
		"/agent/v1/executions/{executionId}/logs:gap":             {"post"},
		"/agent/v1/executions/{executionId}:reconcile":            {"post"},
		"/agent/v1/executions/{executionId}:release":              {"post"},
		"/agent/v1/prechecks:claim-next":                          {"post"},
		"/agent/v1/prechecks/{precheckId}:acknowledge-lease":      {"post"},
		"/agent/v1/prechecks/{precheckId}/secret-slots:resolve":   {"post"},
		"/agent/v1/prechecks/{precheckId}:complete":               {"post"},
		"/agent/v1/data-source-connection-tests:claim-next":       {"post"},
		"/agent/v1/data-source-connection-tests/{connectionTestId}:acknowledge-lease": {
			"post",
		},
		"/agent/v1/data-source-connection-tests/{connectionTestId}/secret-slots:resolve": {
			"post",
		},
		"/agent/v1/data-source-connection-tests/{connectionTestId}:complete": {"post"},
		"/agent/v1/execution-node-environment-checks/{checkId}:complete":     {"post"},
		"/api/v1/storage-credentials":                                        {"get", "post"},
		"/api/v1/storage-credentials/{storageCredentialId}:rotate":           {"post"},
		"/api/v1/storage-credentials/{storageCredentialId}":                  {"delete"},
		"/api/v1/tasks/{taskId}:rebuild-draft":                               {"post"},
		"/api/v1/tasks/{taskId}:resume-checkpoint":                           {"post"},
		"/api/v1/tasks/{taskId}:cancel":                                      {"post"},
		"/api/v1/tasks/{taskId}:save-template":                               {"post"},
		"/api/v1/export-config-templates/{templateId}:create-draft":          {"post"},
		"/api/v1/data-sources/{dataSourceId}:search-export-objects":          {"post"},
		"/api/v1/export-object-catalog-queries/{queryId}":                    {"get"},
	}
	paths := object(t, spec, "paths")
	if len(paths) != len(expected) {
		t.Fatalf("path count = %d, want %d", len(paths), len(expected))
	}
	operationIDs := make(map[string]string)
	operationCount := 0
	for path, methods := range expected {
		pathItem := object(t, paths, path)
		for _, method := range methods {
			operation := object(t, pathItem, method)
			operationCount++
			operationID, ok := operation["operationId"].(string)
			if !ok || operationID == "" {
				t.Fatalf("%s %s has no operationId", method, path)
			}
			if previous, duplicate := operationIDs[operationID]; duplicate {
				t.Fatalf("duplicate operationId %q on %s and %s", operationID, previous, path)
			}
			operationIDs[operationID] = path
			assertSecurityDomain(t, path, operation)
		}
	}
	if operationCount != 72 {
		t.Fatalf("operation count = %d, want 72", operationCount)
	}
}

func TestOpenAPIExplainsRequestIDFailureClosureAndLegacyIDCompatibility(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	schemas := object(t, object(t, spec, "components"), "schemas")
	opaqueID := object(t, schemas, "OpaqueId")
	if opaqueID["format"] != "uuid" || !strings.Contains(fmt.Sprint(opaqueID["description"]), "Agent v1") {
		t.Fatalf("OpaqueId must document UUID generation and Agent v1 compatibility: %#v", opaqueID)
	}
	errorEnvelope := object(t, schemas, "ErrorEnvelope")
	for _, required := range errorEnvelope["required"].([]any) {
		if required == "requestId" {
			t.Fatal("ErrorEnvelope must allow requestId omission for entropy failure")
		}
	}
	requestID := object(t, object(t, errorEnvelope, "properties"), "requestId")
	if !strings.Contains(fmt.Sprint(requestID["description"]), "REQUEST_ID_UNAVAILABLE") {
		t.Fatalf("ErrorEnvelope requestId omission must be constrained to the entropy failure: %#v", requestID)
	}
}

func TestOpenAPI限制本机MVP真实执行(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	if enabled, ok := spec["x-real-execution-enabled"].(bool); !ok || enabled {
		t.Fatal("OpenAPI must explicitly keep real execution disabled")
	}
	paths := object(t, spec, "paths")
	for path := range paths {
		lower := strings.ToLower(path)
		if strings.HasPrefix(path, "/api/v1/") && (strings.Contains(lower, ":retry") || strings.Contains(lower, ":execute")) {
			t.Fatalf("unconfirmed browser execution operation present: %s", path)
		}
	}
	submit := object(t, object(t, paths, "/api/v1/export-drafts/{draftId}:submit"), "post")
	if submit["x-real-execution"] != "WINDOWS_LOCAL_MVP_EXPLICIT_SUBMIT" {
		t.Fatal("task submission contract does not expose the Windows local MVP execution gate")
	}
}

func TestOpenAPI导出提交只要求预检查并保留旧确认字段兼容(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	components := object(t, spec, "components")
	paths := object(t, spec, "paths")
	requestBodies := object(t, components, "requestBodies")
	schemas := object(t, components, "schemas")
	submit := object(t, object(t, paths, "/api/v1/export-drafts/{draftId}:submit"), "post")
	if fmt.Sprint(object(t, submit, "requestBody")["$ref"]) != "#/components/requestBodies/ExportTaskSubmit" {
		t.Fatal("export submit must use the strict task submission request body")
	}
	submitBody := object(t, requestBodies, "ExportTaskSubmit")
	if submitBody["required"] != true {
		t.Fatal("export task submission body must be required")
	}
	submitSchema := object(t, object(t, object(t, submitBody, "content"), "application/json"), "schema")
	if submitSchema["$ref"] != "#/components/schemas/ExportTaskSubmit" {
		t.Fatal("export task submission body must reference its strict schema")
	}
	submitRequest := object(t, schemas, "ExportTaskSubmit")
	if submitRequest["additionalProperties"] != false {
		t.Fatal("export task submission must reject unknown fields")
	}
	assertRequiredProperties(t, submitRequest, "precheckId")
	submitProperties := object(t, submitRequest, "properties")
	legacyConfirmation := object(t, submitProperties, "sensitiveCommandConfirmation")
	if legacyConfirmation["deprecated"] != true {
		t.Fatal("legacy sensitive command confirmation must be deprecated")
	}
	if !strings.Contains(fmt.Sprint(submitRequest["description"]), "普通任务授权") {
		t.Fatal("export submission must describe query-sql as ordinary task authorization")
	}
	confirmation := object(t, schemas, "SensitiveCommandConfirmation")
	if confirmation["additionalProperties"] != false {
		t.Fatal("sensitive command confirmation must reject unknown fields")
	}
	if confirmation["deprecated"] != true {
		t.Fatal("legacy sensitive command confirmation schema must be deprecated")
	}
	assertRequiredProperties(t, confirmation, "capability", "riskFingerprint", "confirmed")
	confirmationProperties := object(t, confirmation, "properties")
	if object(t, confirmationProperties, "capability")["const"] != "CAP_SENSITIVE_COMMAND" || object(t, confirmationProperties, "confirmed")["const"] != true {
		t.Fatal("sensitive command confirmation must fix capability and affirmative confirmation")
	}
	if fmt.Sprint(object(t, confirmationProperties, "riskFingerprint")["$ref"]) != "#/components/schemas/SHA256Digest" {
		t.Fatal("sensitive command confirmation must bind a SHA-256 risk fingerprint")
	}
}

func TestOpenAPI导出V1重定版字段语义(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	schemas := object(t, object(t, spec, "components"), "schemas")
	config := object(t, schemas, "GeneralizedExportConfig")
	configProperties := object(t, config, "properties")

	dataFormat := object(t, configProperties, "dataFormat")
	if !strings.Contains(fmt.Sprint(dataFormat["description"]), "CSV/CUT/SQL") || !strings.Contains(fmt.Sprint(dataFormat["description"]), "历史任务") {
		t.Fatal("data format contract must distinguish ordinary CSV/CUT/SQL creation from historical formats")
	}

	outputConfig := object(t, configProperties, "outputConfig")
	retainEmptyFiles := object(t, object(t, outputConfig, "properties"), "retainEmptyFiles")
	retainDescription := fmt.Sprint(retainEmptyFiles["description"])
	if !strings.Contains(retainDescription, "DATA_ONLY") || !strings.Contains(retainDescription, "DDL_ONLY") || !strings.Contains(retainDescription, "skipHeader") {
		t.Fatal("retain-empty-files contract must cover data content and CSV header behavior")
	}

	filterConfig := object(t, configProperties, "filterConfig")
	querySQL := object(t, object(t, filterConfig, "properties"), "querySql")
	queryDescription := fmt.Sprint(querySQL["description"])
	if !strings.Contains(queryDescription, "普通高级参数") || !strings.Contains(queryDescription, "file://") || !strings.Contains(queryDescription, "flashbackTimestamp") {
		t.Fatal("query-sql contract must describe ordinary authorization, file rejection and conflicts")
	}
}

func TestOpenAPI任务详情读取使用独立安全投影(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	components := object(t, spec, "components")
	responses := object(t, components, "responses")
	schemas := object(t, components, "schemas")
	for _, name := range []string{"TaskOverviewRead", "TaskSnapshotRead", "TaskCommandEvidenceRead", "TaskExecutionRead"} {
		if _, ok := responses[name]; !ok {
			t.Fatalf("missing task detail response %s", name)
		}
	}
	forbidden := map[string][]string{
		"TaskOverviewRead":        {"configFingerprint", "command", "executionId"},
		"TaskSnapshotRead":        {"filePath", "logPath", "credential", "command"},
		"TaskCommandEvidenceRead": {"argv", "credential", "snapshot", "executionId"},
		// EX-I8 起 resultSummary 是受控安全投影（相对路径/大小/检查点存在性），不再是禁止的原始证据。
		"TaskExecutionRead": {"command", "snapshot", "processEvidence"},
	}
	for schemaName, fields := range forbidden {
		properties := object(t, object(t, schemas, schemaName), "properties")
		for _, field := range fields {
			if _, found := properties[field]; found {
				t.Fatalf("%s exposes forbidden field %s", schemaName, field)
			}
		}
	}
	snapshot := object(t, schemas, "TaskSnapshotRead")
	snapshotProperties := object(t, snapshot, "properties")
	assertExactStringEnum(t, object(t, snapshotProperties, "format"), []string{"CSV", "CUT", "SQL", "POS", "PARQUET", "ORC", "AVRO", "DDL", "DDL_CSV", "DDL_CUT", "DDL_SQL"})
	execution := object(t, schemas, "TaskExecutionRead")
	properties := object(t, execution, "properties")
	if object(t, properties, "stageEvidence")["const"] != "UNAVAILABLE" || object(t, properties, "progressEvidence")["const"] != "UNAVAILABLE" {
		t.Fatal("task execution must explicitly keep unsupported stage and progress evidence unavailable")
	}
	for _, field := range []string{"exitCode", "elapsedMs", "errorCode", "errorSummary", "plannedActualMatch", "outputLocation"} {
		if _, found := properties[field]; !found {
			t.Fatalf("task execution must expose structured evidence field %s", field)
		}
	}
	outputLocation := object(t, schemas, "TaskOutputLocation")
	if outputLocation["additionalProperties"] != false {
		t.Fatal("task output location must reject unknown fields")
	}
	assertRequiredProperties(t, outputLocation, "kind", "redaction")
	// EX-I8：结果摘要必须是受控安全投影——只含相对路径与大小，绝不含内容、校验和或绝对路径。
	summaryRef := object(t, properties, "resultSummary")
	if summaryRef["$ref"] != "#/components/schemas/ExecutionResultSummary" {
		t.Fatalf("resultSummary must use the controlled summary schema: %#v", summaryRef)
	}
	summary := object(t, schemas, "ExecutionResultSummary")
	if summary["additionalProperties"] != false {
		t.Fatal("execution result summary must reject unknown fields")
	}
	assertRequiredProperties(t, summary, "result", "fileCount", "totalBytes", "files", "checkpointPresent", "observedAt")
	assertExactStringEnum(t, object(t, object(t, summary, "properties"), "result"), []string{"VERIFIED", "FAILED"})
	summaryProperties := object(t, summary, "properties")
	for _, forbidden := range []string{"content", "checksum", "rowCount", "absolutePath", "outputPath", "contentType"} {
		if _, found := summaryProperties[forbidden]; found {
			t.Fatalf("execution result summary must not expose %s", forbidden)
		}
	}
	file := object(t, schemas, "ExecutionResultFile")
	if file["additionalProperties"] != false {
		t.Fatal("execution result file must reject unknown fields")
	}
	assertRequiredProperties(t, file, "path", "size")
	if object(t, object(t, file, "properties"), "path")["maxLength"] != float64(512) {
		t.Fatal("execution result file path must keep the 512-char bound")
	}
}

func TestOpenAPITaskDerivationSurface(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	components := object(t, spec, "components")
	paths := object(t, spec, "paths")
	schemas := object(t, components, "schemas")
	responses := object(t, components, "responses")

	rebuild := object(t, object(t, paths, "/api/v1/tasks/{taskId}:rebuild-draft"), "post")
	if rebuild["x-idempotency"] != "REQUIRED" || !hasParameterReference(rebuild, "#/components/parameters/CsrfToken") || !hasParameterReference(rebuild, "#/components/parameters/IdempotencyKey") {
		t.Fatalf("rebuild draft must require CSRF and idempotency: %#v", rebuild)
	}
	if responseReference(t, rebuild, "201") != "#/components/responses/TaskDerivedDraftCreated" {
		t.Fatal("rebuild draft must use the derived-draft response")
	}
	resume := object(t, object(t, paths, "/api/v1/tasks/{taskId}:resume-checkpoint"), "post")
	if resume["x-idempotency"] != "REQUIRED" || !hasParameterReference(resume, "#/components/parameters/CsrfToken") {
		t.Fatalf("resume checkpoint must require CSRF and idempotency: %#v", resume)
	}
	if responseReference(t, resume, "201") != "#/components/responses/TaskCheckpointResumeCreated" {
		t.Fatal("resume checkpoint must use the derived-task response")
	}
	assertNoStoreResponse(t, responses, "TaskDerivedDraftCreated")
	assertNoStoreResponse(t, responses, "TaskCheckpointResumeCreated")

	derivation := object(t, schemas, "TaskDerivationRequest")
	if derivation["additionalProperties"] != false {
		t.Fatal("task derivation request must reject unknown fields")
	}
	assertExactStringEnum(t, object(t, object(t, derivation, "properties"), "derivation"), []string{"REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH"})
	resumeEnvelope := object(t, schemas, "TaskCheckpointResumeEnvelope")
	if object(t, object(t, resumeEnvelope, "properties"), "derivationKind")["const"] != "CHECKPOINT_RESUME" {
		t.Fatal("checkpoint resume envelope must fix the derivation kind")
	}
	// 派生响应绝不携带凭据、快照原文或命令。
	for _, schemaName := range []string{"TaskDerivedDraftEnvelope", "TaskCheckpointResumeEnvelope"} {
		envelope := object(t, schemas, schemaName)
		properties := object(t, envelope, "properties")
		for _, forbidden := range []string{"snapshot", "command", "argv", "credential", "config", "password"} {
			if _, found := properties[forbidden]; found {
				t.Fatalf("%s must not expose %s", schemaName, forbidden)
			}
		}
	}
	// 任务概览的派生关系成对出现且枚举受控。
	overview := object(t, schemas, "TaskOverviewRead")
	overviewProperties := object(t, overview, "properties")
	assertExactStringEnum(t, object(t, overviewProperties, "derivationKind"), []string{"REBUILD_FROM_CONFIG", "RERUN_FROM_SCRATCH", "CHECKPOINT_RESUME"})
}

func TestOpenAPI任务日志仅遮蔽秘密并保留运行上下文(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	for _, path := range []string{"/api/v1/tasks/{taskId}/logs", "/api/v1/tasks/{taskId}/logs/stream"} {
		operation := object(t, object(t, paths, path), "get")
		if operation["x-log-content-boundary"] != "SECRETS_ONLY_WITH_OPERATIONAL_CONTEXT" {
			t.Fatalf("%s 未声明已确认的任务日志可见边界", path)
		}
	}
	agentLogs := object(t, object(t, paths, "/agent/v1/executions/{executionId}/logs:append"), "post")
	if agentLogs["x-redaction"] != "AGENT_BEFORE_UPLOAD_SECRETS_ONLY" || agentLogs["x-log-content-boundary"] != "SECRETS_ONLY_WITH_OPERATIONAL_CONTEXT" {
		t.Fatal("Agent 日志上传没有声明秘密遮蔽与运行上下文保留边界")
	}
}

func TestOpenAPIWriteOperationsDeclareBrowserSafetyHeaders(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	for path, rawPathItem := range paths {
		if !strings.HasPrefix(path, "/api/v1/") {
			continue
		}
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			t.Fatalf("path item %s is invalid", path)
		}
		for _, method := range []string{"post", "patch", "delete"} {
			rawOperation, exists := pathItem[method]
			if !exists {
				continue
			}
			operation := rawOperation.(map[string]any)
			if !hasParameterReference(operation, "#/components/parameters/CsrfToken") {
				t.Errorf("%s %s has no CSRF parameter", method, path)
			}
			if operation["x-idempotency"] == "REQUIRED" && !hasParameterReference(operation, "#/components/parameters/IdempotencyKey") {
				t.Errorf("%s %s requires idempotency but has no header", method, path)
			}
			if operation["x-optimistic-lock"] == "REQUIRED" && !hasParameterReference(operation, "#/components/parameters/IfMatch") {
				t.Errorf("%s %s requires optimistic locking but has no If-Match header", method, path)
			}
		}
	}
}

func TestOpenAPIReferencesResolveAndSecretInputsAreWriteOnly(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	walkReferences(t, spec, spec)
	components := object(t, spec, "components")
	schemas := object(t, components, "schemas")
	dataSourceWrite := object(t, schemas, "DataSourceWrite")
	properties := object(t, dataSourceWrite, "properties")
	password := object(t, properties, "password")
	if password["writeOnly"] != true {
		t.Fatal("data source password must be a write-only API field")
	}
	compatibilityMode := object(t, properties, "compatibilityMode")
	compatibilityModes, ok := compatibilityMode["enum"].([]any)
	if !ok || len(compatibilityModes) != 2 || compatibilityModes[0] != "MYSQL" || compatibilityModes[1] != "ORACLE" {
		t.Fatal("data source compatibility mode must be an OceanBase MySQL or Oracle type")
	}
	clusterName := object(t, properties, "clusterName")
	if _, exists := clusterName["minLength"]; exists {
		t.Fatal("data source clusterName must remain optional in the write contract")
	}
	tenantName := object(t, properties, "tenantName")
	if tenantName["minLength"] != float64(1) {
		t.Fatal("data source tenantName must remain required by the write contract")
	}
	if _, exists := properties["state"]; exists {
		t.Fatal("data source state must use dedicated state actions, not the write schema")
	}
	gated := object(t, schemas, "GatedCsvOptions")
	if gated["x-support-state"] != "ENABLED" {
		t.Fatal("CSV serialization options must be enabled since EX-I3")
	}
	csvProperties := object(t, gated, "properties")
	if _, exists := csvProperties["withTrim"]; !exists {
		t.Fatal("CSV options must include withTrim")
	}
	cutOptions := object(t, schemas, "CutOptions")
	if cutOptions["x-support-state"] != "ENABLED" {
		t.Fatal("cut options must be enabled since EX-I4")
	}
	generalized := object(t, schemas, "GeneralizedExportConfig")
	dataFormat := object(t, object(t, generalized, "properties"), "dataFormat")
	dataFormatProperties := object(t, dataFormat, "properties")
	if _, exists := dataFormatProperties["cutOptions"]; !exists {
		t.Fatal("generalized data format must carry cut options")
	}
	if _, exists := dataFormatProperties["csvOptions"]; !exists {
		t.Fatal("generalized data format must carry csv options")
	}
	// 2026-08-10：objectScope 必须声明 database 与 scopeKind 必填；
	// outputConfig 必须携带 EX-I3 压缩字段；数据格式与输出类型枚举覆盖全部已启用能力。
	generalizedProperties := object(t, generalized, "properties")
	objectScope := object(t, generalizedProperties, "objectScope")
	required, ok := objectScope["required"].([]any)
	if !ok || len(required) != 2 {
		t.Fatal("object scope must declare exactly database and scopeKind as required")
	}
	requiredSet := map[string]bool{}
	for _, name := range required {
		requiredSet[fmt.Sprint(name)] = true
	}
	if !requiredSet["database"] || !requiredSet["scopeKind"] {
		t.Fatal("object scope required fields must include database and scopeKind")
	}
	outputConfig := object(t, generalizedProperties, "outputConfig")
	outputConfigProperties := object(t, outputConfig, "properties")
	for _, field := range []string{"compress", "compressionAlgo"} {
		if _, exists := outputConfigProperties[field]; !exists {
			t.Fatalf("output config must carry %s", field)
		}
	}
	formatKind := object(t, dataFormatProperties, "formatKind")
	formatKinds, ok := formatKind["enum"].([]any)
	if !ok || len(formatKinds) != 7 {
		t.Fatal("data format kind must enumerate all seven supported formats")
	}
	outputKind := object(t, outputConfigProperties, "outputKind")
	outputKinds, ok := outputKind["enum"].([]any)
	if !ok || len(outputKinds) != 5 {
		t.Fatal("output kind must enumerate LOCAL plus four controlled storage types")
	}
}

func TestOpenAPI数据源列表与详情用户名投影分离(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	components := object(t, spec, "components")
	paths := object(t, spec, "paths")
	responses := object(t, components, "responses")
	schemas := object(t, components, "schemas")

	list := object(t, object(t, paths, "/api/v1/data-sources"), "get")
	if responseReference(t, list, "200") != "#/components/responses/DataSourceList" {
		t.Fatal("data source list must use the authorized list response")
	}
	if description := fmt.Sprint(list["description"]); !strings.Contains(description, "读取范围") || !strings.Contains(description, "普通业务用户名") {
		t.Fatal("data source list must document its read-authorized business username projection")
	}
	detailPath := object(t, paths, "/api/v1/data-sources/{dataSourceId}")
	for _, method := range []string{"get", "patch"} {
		if responseReference(t, object(t, detailPath, method), "200") != "#/components/responses/DataSourceDetail" {
			t.Fatalf("data source %s must use the authorized detail response", method)
		}
	}
	for _, responseName := range []string{"DataSourceList", "DataSourceDetail"} {
		assertNoStoreResponse(t, responses, responseName)
	}

	listItem := object(t, schemas, "DataSourceListItem")
	detail := object(t, schemas, "DataSourceDetail")
	for schemaName, schema := range map[string]map[string]any{
		"DataSourceListItem": listItem,
		"DataSourceDetail":   detail,
	} {
		if schema["additionalProperties"] != false {
			t.Fatalf("%s must reject unregistered response fields", schemaName)
		}
		assertRequiredProperties(t, schema, "id", "displayName", "revision", "credentialRevision", "sysCredentialState")
	}
	assertRequiredProperties(t, listItem, "username")
	listProperties := object(t, listItem, "properties")
	if _, exists := listProperties["username"]; !exists {
		t.Fatal("data source list item must expose the authorized business username")
	}
	for _, forbidden := range []string{"sysUser", "password", "sysPassword", "combinedUsername", "credentialId", "ciphertext", "nonce"} {
		if _, exists := listProperties[forbidden]; exists {
			t.Fatalf("data source list item must not expose %s", forbidden)
		}
	}
	detailProperties := object(t, detail, "properties")
	if _, exists := detailProperties["username"]; !exists {
		t.Fatal("data source detail must model the management-authorized business username")
	}
	for _, forbidden := range []string{"sysUser", "password", "sysPassword", "combinedUsername", "credentialId", "ciphertext", "nonce"} {
		if _, exists := detailProperties[forbidden]; exists {
			t.Fatalf("data source detail must not expose %s", forbidden)
		}
	}
	for _, raw := range detail["required"].([]any) {
		if raw == "username" {
			t.Fatal("business username must remain absent when management scope is not confirmed")
		}
	}

	listEnvelope := object(t, schemas, "DataSourceListEnvelope")
	itemsSchema := object(t, object(t, listEnvelope, "properties"), "items")
	if fmt.Sprint(object(t, itemsSchema, "items")["$ref"]) != "#/components/schemas/DataSourceListItem" {
		t.Fatal("data source list envelope must use the authorized item schema")
	}
	detailEnvelope := object(t, schemas, "DataSourceDetailEnvelope")
	if fmt.Sprint(object(t, object(t, detailEnvelope, "properties"), "item")["$ref"]) != "#/components/schemas/DataSourceDetail" {
		t.Fatal("data source detail envelope must use the authorized detail schema")
	}
}

func TestOpenAPIStorageCredentialSurface(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	components := object(t, spec, "components")
	paths := object(t, spec, "paths")
	responses := object(t, components, "responses")
	schemas := object(t, components, "schemas")

	// 写请求体：provider 白名单 + 密钥 writeOnly，绝不回显。
	write := object(t, schemas, "StorageCredentialWrite")
	provider := object(t, object(t, write, "properties"), "provider")
	assertExactStringEnum(t, provider, []string{"OSS", "S3", "COS", "OBS"})
	for _, secret := range []string{"accessKey", "secretKey"} {
		property := object(t, object(t, write, "properties"), secret)
		if property["writeOnly"] != true {
			t.Fatalf("storage credential %s must be writeOnly", secret)
		}
	}
	// 安全投影绝不暴露密钥字段或信封材料。
	item := object(t, schemas, "StorageCredentialListItem")
	if item["additionalProperties"] != false {
		t.Fatal("storage credential item must reject unregistered response fields")
	}
	assertRequiredProperties(t, item, "id", "displayName", "provider", "currentRevision", "revision", "updatedAt")
	itemProperties := object(t, item, "properties")
	for _, forbidden := range []string{"accessKey", "secretKey", "ciphertext", "nonce", "keyId", "credentialId"} {
		if _, exists := itemProperties[forbidden]; exists {
			t.Fatalf("storage credential item must not expose %s", forbidden)
		}
	}
	// 创建与轮换必须声明 CSRF；轮换与删除必须声明 If-Match 乐观锁；创建/轮换必须声明幂等键。
	create := object(t, object(t, paths, "/api/v1/storage-credentials"), "post")
	if create["x-idempotency"] != "REQUIRED" || !hasParameterReference(create, "#/components/parameters/CsrfToken") || !hasParameterReference(create, "#/components/parameters/IdempotencyKey") {
		t.Fatalf("create storage credential must require CSRF and idempotency key: %#v", create)
	}
	if responseReference(t, create, "201") != "#/components/responses/StorageCredentialItemCreated" {
		t.Fatal("create storage credential must use the item-created response")
	}
	rotate := object(t, object(t, paths, "/api/v1/storage-credentials/{storageCredentialId}:rotate"), "post")
	if rotate["x-optimistic-lock"] != "REQUIRED" || !hasParameterReference(rotate, "#/components/parameters/IfMatch") || !hasParameterReference(rotate, "#/components/parameters/IdempotencyKey") {
		t.Fatalf("rotate storage credential must require If-Match and idempotency key: %#v", rotate)
	}
	del := object(t, object(t, paths, "/api/v1/storage-credentials/{storageCredentialId}"), "delete")
	if del["x-optimistic-lock"] != "REQUIRED" || !hasParameterReference(del, "#/components/parameters/IfMatch") || !hasParameterReference(del, "#/components/parameters/CsrfToken") {
		t.Fatalf("delete storage credential must require CSRF and If-Match: %#v", del)
	}
	// 全部响应必须 no-store；列表信封必须引用安全投影。
	for _, responseName := range []string{"StorageCredentialList", "StorageCredentialItemCreated", "StorageCredentialItem", "StorageCredentialDeletion"} {
		assertNoStoreResponse(t, responses, responseName)
	}
	listEnvelope := object(t, schemas, "StorageCredentialListEnvelope")
	itemsSchema := object(t, object(t, listEnvelope, "properties"), "items")
	if fmt.Sprint(object(t, itemsSchema, "items")["$ref"]) != "#/components/schemas/StorageCredentialListItem" {
		t.Fatal("storage credential list envelope must use the safe item schema")
	}
	itemEnvelope := object(t, schemas, "StorageCredentialItemEnvelope")
	if fmt.Sprint(object(t, object(t, itemEnvelope, "properties"), "item")["$ref"]) != "#/components/schemas/StorageCredentialListItem" {
		t.Fatal("storage credential item envelope must use the safe item schema")
	}
}

func TestOpenAPITaskListUsesDedicatedSafeProjection(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	operation := object(t, object(t, paths, "/api/v1/tasks"), "get")
	if responseReference(t, operation, "200") != "#/components/responses/TaskList" {
		t.Fatal("task list must use its dedicated safe response")
	}
	if !hasParameterReference(operation, "#/components/parameters/TaskPageSize") {
		t.Fatal("task list must declare its bounded page-size parameter")
	}
	schemas := object(t, object(t, spec, "components"), "schemas")
	item := object(t, schemas, "TaskListItem")
	properties := object(t, item, "properties")
	for _, required := range []string{"id", "type", "state", "stageEvidence", "progressEvidence", "reconciliationRequired", "ownedByCurrentUser"} {
		if _, ok := properties[required]; !ok {
			t.Fatalf("task list safe property %s is missing", required)
		}
	}
	envelope := object(t, schemas, "TaskListEnvelope")
	envelopeProperties := object(t, envelope, "properties")
	if _, ok := envelopeProperties["totalPages"]; !ok {
		t.Fatal("task list must declare authorization-scoped totalPages")
	}
	for _, forbidden := range []string{"snapshot", "plannedCommand", "actualCommand", "error", "logs", "credentialId", "creatorSubjectId"} {
		if _, ok := properties[forbidden]; ok {
			t.Fatalf("task list exposes forbidden property %s", forbidden)
		}
	}
}

func TestOpenAPIAgentEnrollmentAndHeartbeatContracts(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	components := object(t, spec, "components")
	requestBodies := object(t, components, "requestBodies")
	responses := object(t, components, "responses")
	schemas := object(t, components, "schemas")

	issue := object(t, object(t, paths, "/api/v1/execution-nodes/{nodeId}:enrollments"), "post")
	if _, exists := issue["requestBody"]; exists {
		t.Fatal("enrollment material issuance must not accept a request body")
	}
	if issue["x-secret-response"] != "DISPLAY_ONCE" {
		t.Fatal("enrollment material issuance must declare display-once semantics")
	}
	if !hasParameterReference(issue, "#/components/parameters/CsrfToken") {
		t.Fatal("enrollment material issuance must require CSRF validation")
	}
	if responseReference(t, issue, "201") != "#/components/responses/EnrollmentMaterialIssued" {
		t.Fatal("enrollment material issuance must use its dedicated response")
	}
	assertNoStoreResponse(t, responses, "EnrollmentMaterialIssued")
	issued := object(t, schemas, "EnrollmentMaterialIssuedEnvelope")
	assertRequiredProperties(t, issued, "requestId", "enrollmentId", "nodeId", "enrollmentMaterial", "expiresAt", "displayedOnce")
	if object(t, object(t, issued, "properties"), "displayedOnce")["const"] != true {
		t.Fatal("enrollment material issuance must mark the response as displayed once")
	}

	exchange := object(t, object(t, paths, "/agent/v1/enrollments:exchange"), "post")
	if responseReference(t, exchange, "200") != "#/components/responses/AgentEnrollmentAccepted" {
		t.Fatal("enrollment exchange must use a non-generic response")
	}
	assertNoStoreResponse(t, responses, "AgentEnrollmentAccepted")
	exchangeBody := object(t, requestBodies, "EnrollmentExchange")
	if fmt.Sprint(object(t, exchangeBody, "content")["application/json"].(map[string]any)["schema"].(map[string]any)["$ref"]) != "#/components/schemas/EnrollmentExchange" {
		t.Fatal("enrollment exchange must reference its strict request schema")
	}
	exchangeSchema := object(t, schemas, "EnrollmentExchange")
	assertRequiredProperties(t, exchangeSchema, "requestId", "enrollmentId", "enrollmentMaterial", "agentId", "nodeId", "machineCredential", "protocolVersion")
	exchangeProperties := object(t, exchangeSchema, "properties")
	for _, secret := range []string{"enrollmentMaterial", "machineCredential"} {
		if object(t, exchangeProperties, secret)["writeOnly"] != true {
			t.Fatalf("enrollment exchange %s must be write-only", secret)
		}
	}
	exchangeResponse := object(t, schemas, "AgentEnrollmentResponseEnvelope")
	exchangePayload := object(t, object(t, exchangeResponse, "properties"), "payload")
	for _, secret := range []string{"enrollmentMaterial", "machineCredential"} {
		if _, exists := object(t, exchangePayload, "properties")[secret]; exists {
			t.Fatalf("enrollment exchange response must not echo %s", secret)
		}
	}

	heartbeat := object(t, object(t, paths, "/agent/v1/heartbeats"), "post")
	if responseReference(t, heartbeat, "200") != "#/components/responses/AgentHeartbeatAccepted" {
		t.Fatal("heartbeat must use a non-generic response")
	}
	assertNoStoreResponse(t, responses, "AgentHeartbeatAccepted")
	heartbeatBody := object(t, requestBodies, "AgentHeartbeat")
	heartbeatSchemaReference := fmt.Sprint(object(t, object(t, heartbeatBody, "content"), "application/json")["schema"].(map[string]any)["$ref"])
	if heartbeatSchemaReference != "#/components/schemas/AgentHeartbeat" {
		t.Fatalf("heartbeat request schema = %q, want strict AgentHeartbeat", heartbeatSchemaReference)
	}
	heartbeatSchema := object(t, schemas, "AgentHeartbeat")
	if heartbeatSchema["additionalProperties"] != false {
		t.Fatal("heartbeat envelope must reject unknown fields")
	}
	assertRequiredProperties(t, heartbeatSchema, "protocolVersion", "agentId", "nodeId", "bootId", "requestId", "sentAt", "payloadType", "payload")
	heartbeatProperties := object(t, heartbeatSchema, "properties")
	if object(t, heartbeatProperties, "protocolVersion")["const"] != "agent-v1" || object(t, heartbeatProperties, "payloadType")["const"] != "HEARTBEAT" {
		t.Fatal("heartbeat envelope must fix protocolVersion and payloadType")
	}
	payloadReference := fmt.Sprint(object(t, heartbeatProperties, "payload")["$ref"])
	if payloadReference != "#/components/schemas/AgentHeartbeatPayload" {
		t.Fatalf("heartbeat payload schema = %q, want strict AgentHeartbeatPayload", payloadReference)
	}
	payload := object(t, schemas, "AgentHeartbeatPayload")
	if payload["additionalProperties"] != false {
		t.Fatal("heartbeat payload must reject unknown fields")
	}
	assertRequiredProperties(t, payload, "operatingSystem", "architecture", "agentVersion", "observedAt", "capacityTotal", "capacityUsed")
	if object(t, object(t, payload, "properties"), "capacityTotal")["const"] != float64(1) {
		t.Fatal("heartbeat capacityTotal must remain fixed at one")
	}
}

func TestOpenAPIAgentPrecheckContracts(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	components := object(t, spec, "components")
	requestBodies := object(t, components, "requestBodies")
	responses := object(t, components, "responses")
	schemas := object(t, components, "schemas")

	operations := []struct {
		path            string
		requestBodyName string
		requestSchema   string
		responseName    string
		responseSchema  string
		payloadType     string
	}{
		{"/agent/v1/prechecks:claim-next", "AgentPrecheckClaimNext", "AgentPrecheckClaimNext", "AgentPrecheckClaimed", "AgentPrecheckClaimResponseEnvelope", "EXPORT_PREFLIGHT_CLAIM_NEXT"},
		{"/agent/v1/prechecks/{precheckId}:acknowledge-lease", "AgentPrecheckLeaseAcknowledgement", "AgentPrecheckLeaseAcknowledgement", "AgentPrecheckLeaseAcknowledged", "AgentPrecheckLeaseAcknowledgedResponseEnvelope", "EXPORT_PREFLIGHT_ACKNOWLEDGE_LEASE"},
		{"/agent/v1/prechecks/{precheckId}/secret-slots:resolve", "AgentPrecheckSecretSlotResolve", "AgentPrecheckSecretSlotResolve", "AgentPrecheckSecretSlotResolved", "AgentPrecheckSecretSlotResponseEnvelope", "EXPORT_PREFLIGHT_RESOLVE_SECRET_SLOTS"},
		{"/agent/v1/prechecks/{precheckId}:complete", "AgentPrecheckCompletion", "AgentPrecheckCompletion", "AgentPrecheckCompleted", "AgentPrecheckCompletedResponseEnvelope", "EXPORT_PREFLIGHT_COMPLETE"},
	}
	for _, operationContract := range operations {
		operation := object(t, object(t, paths, operationContract.path), "post")
		if requestBodyReference(t, object(t, operation, "requestBody")) != "#/components/requestBodies/"+operationContract.requestBodyName {
			t.Fatalf("%s does not use its strict precheck request body", operationContract.path)
		}
		if responseReference(t, operation, "200") != "#/components/responses/"+operationContract.responseName {
			t.Fatalf("%s does not use its strict precheck success response", operationContract.path)
		}
		assertNoStoreResponse(t, responses, operationContract.responseName)
		body := object(t, requestBodies, operationContract.requestBodyName)
		content := object(t, body, "content")
		schemaReference := fmt.Sprint(object(t, object(t, content, "application/json"), "schema")["$ref"])
		if schemaReference != "#/components/schemas/"+operationContract.requestSchema {
			t.Fatalf("%s request schema = %q", operationContract.path, schemaReference)
		}
		envelope := object(t, schemas, operationContract.requestSchema)
		if envelope["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown envelope fields", operationContract.requestSchema)
		}
		assertRequiredProperties(t, envelope, "protocolVersion", "agentId", "nodeId", "bootId", "requestId", "sentAt", "payloadType", "payload")
		properties := object(t, envelope, "properties")
		if object(t, properties, "protocolVersion")["const"] != "agent-v1" || object(t, properties, "payloadType")["const"] != operationContract.payloadType {
			t.Fatalf("%s must fix protocol and payload type", operationContract.requestSchema)
		}
		response := object(t, schemas, operationContract.responseSchema)
		if response["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown response fields", operationContract.responseSchema)
		}
		assertRequiredProperties(t, response, "requestId", "serverTime", "status", "payload")
	}

	claimNext := object(t, object(t, paths, "/agent/v1/prechecks:claim-next"), "post")
	if responseReference(t, claimNext, "204") != "#/components/responses/NoWork" {
		t.Fatal("claim-next must declare its no-work response")
	}
	assertNoStoreResponse(t, responses, "NoWork")
	claimPayload := object(t, schemas, "AgentPrecheckClaimNextPayload")
	if claimPayload["additionalProperties"] != false {
		t.Fatal("precheck claim-next payload must reject unknown fields")
	}
	assertRequiredProperties(t, claimPayload, "capability")
	if object(t, object(t, claimPayload, "properties"), "capability")["const"] != "EXPORT_PREFLIGHT" {
		t.Fatal("precheck claim-next must only request EXPORT_PREFLIGHT")
	}
	for _, forbidden := range []string{"precheckId", "leaseId", "path", "command", "sql"} {
		if _, exists := object(t, claimPayload, "properties")[forbidden]; exists {
			t.Fatalf("precheck claim-next payload must not accept %s", forbidden)
		}
	}

	for _, payloadName := range []string{"AgentPrecheckLeaseAcknowledgementPayload", "AgentPrecheckSecretSlotResolvePayload", "AgentPrecheckCompletionPayload"} {
		payload := object(t, schemas, payloadName)
		if payload["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown fields", payloadName)
		}
		assertRequiredProperties(t, payload, "leaseId", "leaseEpoch", "bindingDigest")
		if fmt.Sprint(object(t, object(t, payload, "properties"), "bindingDigest")["$ref"]) != "#/components/schemas/SHA256Digest" {
			t.Fatalf("%s must bind the lease to a SHA-256 digest", payloadName)
		}
	}
	secretResolvePayload := object(t, schemas, "AgentPrecheckSecretSlotResolvePayload")
	if !reflect.DeepEqual(object(t, object(t, secretResolvePayload, "properties"), "slot")["enum"], []any{"DATABASE_CONNECTION", "STORAGE_CREDENTIAL"}) {
		t.Fatal("precheck secret resolution must be limited to the two fixed secret slots")
	}
	completionPayload := object(t, schemas, "AgentPrecheckCompletionPayload")
	assertRequiredProperties(t, completionPayload, "results")
	if _, exists := object(t, completionPayload, "properties")["succeeded"]; exists {
		t.Fatal("precheck completion must derive success from fixed results, not accept succeeded")
	}

	binding := object(t, schemas, "AgentPrecheckBinding")
	if binding["additionalProperties"] != false {
		t.Fatal("precheck binding must reject unknown fields")
	}
	assertRequiredProperties(t, binding, "precheckId", "nodeId", "draftRevision", "configFingerprint", "credentialRevision", "nodeFactsVersion")
	if fmt.Sprint(object(t, object(t, binding, "properties"), "configFingerprint")["$ref"]) != "#/components/schemas/SHA256Digest" {
		t.Fatal("precheck binding must carry a SHA-256 configuration fingerprint")
	}

	checkSet := object(t, schemas, "AgentPrecheckCheckSet")
	checkVariants, ok := checkSet["oneOf"].([]any)
	if !ok || len(checkVariants) != 2 {
		t.Fatal("precheck check set must offer exactly the local and storage shapes")
	}
	localCheckVariant, storageCheckVariant := variantObject(t, checkVariants[0]), variantObject(t, checkVariants[1])
	assertFixedPrecheckArray(t, localCheckVariant, []string{"DATABASE_CONNECTIVITY", "OBJECT_ACCESS", "TOOL_ENVIRONMENT", "OUTPUT_PATH", "OUTPUT_EMPTY", "AVAILABLE_SPACE"})
	assertFixedPrecheckArray(t, storageCheckVariant, []string{"DATABASE_CONNECTIVITY", "OBJECT_ACCESS", "TOOL_ENVIRONMENT", "AVAILABLE_SPACE", "STORAGE_CONNECTIVITY", "STORAGE_AUTH"})
	results := object(t, schemas, "AgentPrecheckResults")
	resultVariants, ok := results["oneOf"].([]any)
	if !ok || len(resultVariants) != 2 {
		t.Fatal("precheck results must offer exactly the local and storage shapes")
	}
	for variantIndex, variant := range resultVariants {
		variantObject := variantObject(t, variant)
		if variantObject["items"] != false {
			t.Fatal("precheck results must reject extra checks")
		}
		if rawPrefixItems, ok := variantObject["prefixItems"].([]any); !ok || len(rawPrefixItems) != 6 {
			t.Fatalf("precheck results variant %d must contain the six fixed checks in order", variantIndex)
		}
	}
	// 存储形态的最后两项必须是受控存储检查结果 schema。
	storagePrefixItems := variantObject(t, resultVariants[1])["prefixItems"].([]any)
	if fmt.Sprint(storagePrefixItems[4].(map[string]any)["$ref"]) != "#/components/schemas/AgentPrecheckStorageConnectivityResult" || fmt.Sprint(storagePrefixItems[5].(map[string]any)["$ref"]) != "#/components/schemas/AgentPrecheckStorageAuthResult" {
		t.Fatal("storage precheck results must reference the storage check result schemas")
	}
	for _, resultContract := range []struct {
		schema   string
		check    string
		evidence []string
	}{
		{"AgentPrecheckDatabaseConnectivityResult", "DATABASE_CONNECTIVITY", []string{"SYNTHETIC_OK", "DATABASE_CONNECTED", "DATABASE_CONNECTION_FAILED", "DATABASE_CONNECTION_UNAVAILABLE"}},
		{"AgentPrecheckObjectAccessResult", "OBJECT_ACCESS", []string{"SYNTHETIC_OK", "OBJECT_ACCESSIBLE", "OBJECT_NOT_ACCESSIBLE", "OBJECT_ACCESS_UNAVAILABLE"}},
		{"AgentPrecheckToolEnvironmentResult", "TOOL_ENVIRONMENT", []string{"SYNTHETIC_OK", "TOOL_RUNTIME_READY", "TOOL_RUNTIME_INVALID", "TOOL_RUNTIME_UNAVAILABLE"}},
		{"AgentPrecheckOutputPathResult", "OUTPUT_PATH", []string{"SYNTHETIC_OK", "OUTPUT_PATH_WRITABLE", "OUTPUT_PATH_NOT_WRITABLE", "OUTPUT_PATH_UNAVAILABLE"}},
		{"AgentPrecheckOutputEmptyResult", "OUTPUT_EMPTY", []string{"SYNTHETIC_OK", "OUTPUT_PATH_EMPTY", "OUTPUT_EMPTY_CHECK_SKIPPED", "OUTPUT_PATH_NOT_EMPTY", "OUTPUT_PATH_UNAVAILABLE"}},
		{"AgentPrecheckAvailableSpaceResult", "AVAILABLE_SPACE", []string{"SYNTHETIC_OK", "OUTPUT_SPACE_SUFFICIENT", "OUTPUT_SPACE_INSUFFICIENT", "OUTPUT_SPACE_UNAVAILABLE"}},
		{"AgentPrecheckStorageConnectivityResult", "STORAGE_CONNECTIVITY", []string{"SYNTHETIC_OK", "STORAGE_ENDPOINT_REACHABLE", "STORAGE_ENDPOINT_UNREACHABLE", "STORAGE_CONNECTIVITY_UNAVAILABLE"}},
		{"AgentPrecheckStorageAuthResult", "STORAGE_AUTH", []string{"SYNTHETIC_OK", "STORAGE_CREDENTIAL_VERIFIED", "STORAGE_CREDENTIAL_REJECTED", "STORAGE_AUTH_UNAVAILABLE"}},
	} {
		result := object(t, schemas, resultContract.schema)
		if result["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown result fields", resultContract.schema)
		}
		assertRequiredProperties(t, result, "check", "status", "evidenceCode")
		properties := object(t, result, "properties")
		if object(t, properties, "check")["const"] != resultContract.check {
			t.Fatalf("%s check is not fixed", resultContract.schema)
		}
		statuses, ok := object(t, properties, "status")["enum"].([]any)
		if !ok || len(statuses) != 3 || statuses[0] != "PASSED" || statuses[1] != "FAILED" || statuses[2] != "UNKNOWN" {
			t.Fatalf("%s status set is not fixed", resultContract.schema)
		}
		evidenceCodes, ok := object(t, properties, "evidenceCode")["enum"].([]any)
		if !ok || len(evidenceCodes) != len(resultContract.evidence) {
			t.Fatalf("%s evidence code set is not fixed", resultContract.schema)
		}
		for index, code := range resultContract.evidence {
			if evidenceCodes[index] != code {
				t.Fatalf("%s evidence code[%d] = %v, want %q", resultContract.schema, index, evidenceCodes[index], code)
			}
		}
	}

	claimResponse := object(t, schemas, "AgentPrecheckClaimResponseEnvelope")
	if object(t, object(t, claimResponse, "properties"), "status")["const"] != "PRECHECK_CLAIMED" {
		t.Fatal("precheck claim response must have PRECHECK_CLAIMED status")
	}
	claimResponsePayload := object(t, object(t, claimResponse, "properties"), "payload")
	if claimResponsePayload["additionalProperties"] != false {
		t.Fatal("precheck claim response payload must reject unknown fields")
	}
	assertRequiredProperties(t, claimResponsePayload, "precheckId", "leaseId", "leaseEpoch", "expiresAt", "binding", "bindingDigest", "checkSet", "executionContext", "realExecutionEnabled")
	if object(t, object(t, claimResponsePayload, "properties"), "realExecutionEnabled")["const"] != false {
		t.Fatal("precheck claim response must keep real execution disabled")
	}
	executionContext := object(t, schemas, "AgentPrecheckExecutionContext")
	if executionContext["additionalProperties"] != false {
		t.Fatal("precheck execution context must reject unknown fields")
	}
	assertRequiredProperties(t, executionContext, "compatibilityMode", "database", "table", "outputPath", "logPath", "skipCheckDir", "targetPlatform", "allowedRoots")
	if !reflect.DeepEqual(object(t, object(t, executionContext, "properties"), "compatibilityMode")["enum"], []any{"MYSQL", "ORACLE"}) {
		t.Fatal("precheck execution context must constrain the OceanBase compatibility mode")
	}
	if fmt.Sprint(object(t, object(t, claimResponsePayload, "properties"), "executionContext")["$ref"]) != "#/components/schemas/AgentPrecheckExecutionContext" {
		t.Fatal("precheck claim response must use the strict execution context schema")
	}

	acknowledgedResponse := object(t, schemas, "AgentPrecheckLeaseAcknowledgedResponseEnvelope")
	if object(t, object(t, acknowledgedResponse, "properties"), "status")["const"] != "PRECHECK_LEASE_ACKNOWLEDGED" {
		t.Fatal("precheck acknowledgement response must have PRECHECK_LEASE_ACKNOWLEDGED status")
	}
	assertPrecheckResponseExecutionDisabled(t, acknowledgedResponse)
	secretResponse := object(t, schemas, "AgentPrecheckSecretSlotResponseEnvelope")
	if object(t, object(t, secretResponse, "properties"), "status")["const"] != "PRECHECK_SECRET_SLOTS_RESOLVED" {
		t.Fatal("precheck secret response must have PRECHECK_SECRET_SLOTS_RESOLVED status")
	}
	secretResponsePayload := object(t, object(t, secretResponse, "properties"), "payload")
	secretSlotVariants, ok := secretResponsePayload["oneOf"].([]any)
	if !ok || len(secretSlotVariants) != 2 {
		t.Fatal("precheck secret response must have exactly database and storage slot variants")
	}
	databaseSlotPayload, storageSlotPayload := variantObject(t, secretSlotVariants[0]), variantObject(t, secretSlotVariants[1])
	if databaseSlotPayload["additionalProperties"] != false || storageSlotPayload["additionalProperties"] != false {
		t.Fatal("precheck secret slot response variants must reject unknown fields")
	}
	assertRequiredProperties(t, databaseSlotPayload, "agentRequestId", "precheckId", "leaseId", "leaseEpoch", "bindingDigest", "slot", "connection", "realExecutionEnabled")
	assertRequiredProperties(t, storageSlotPayload, "agentRequestId", "precheckId", "leaseId", "leaseEpoch", "bindingDigest", "slot", "storageCredential", "realExecutionEnabled")
	if object(t, object(t, databaseSlotPayload, "properties"), "slot")["const"] != "DATABASE_CONNECTION" ||
		object(t, object(t, storageSlotPayload, "properties"), "slot")["const"] != "STORAGE_CREDENTIAL" {
		t.Fatal("precheck secret response variants must fix their respective slots")
	}
	for _, payload := range []map[string]any{databaseSlotPayload, storageSlotPayload} {
		if fmt.Sprint(object(t, object(t, payload, "properties"), "bindingDigest")["$ref"]) != "#/components/schemas/SHA256Digest" {
			t.Fatal("precheck secret response must bind the returned slot to a SHA-256 digest")
		}
	}
	connection := object(t, schemas, "AgentPrecheckDatabaseConnectionSlot")
	if connection["additionalProperties"] != false {
		t.Fatal("precheck database connection slot must reject unknown fields")
	}
	assertRequiredProperties(t, connection, "host", "port", "username", "password")
	password := object(t, object(t, connection, "properties"), "password")
	if password["x-agent-memory-only"] != true || password["contentEncoding"] != "base64" {
		t.Fatal("precheck database password must be an Agent-only short-lived byte value")
	}
	for _, forbidden := range []string{"example", "default", "minLength", "maxLength"} {
		if _, found := password[forbidden]; found {
			t.Fatalf("precheck database password must not expose %s", forbidden)
		}
	}
	storageCredential := object(t, schemas, "AgentPrecheckStorageCredentialSlot")
	if storageCredential["additionalProperties"] != false {
		t.Fatal("precheck storage credential slot must reject unknown fields")
	}
	assertRequiredProperties(t, storageCredential, "provider", "accessKey", "secretKey")
	storageSecretKey := object(t, object(t, storageCredential, "properties"), "secretKey")
	if storageSecretKey["x-agent-memory-only"] != true || storageSecretKey["contentEncoding"] != "base64" {
		t.Fatal("precheck storage secret key must be an Agent-only short-lived byte value")
	}
	assertPrecheckResponseExecutionDisabled(t, secretResponse)
	completedResponse := object(t, schemas, "AgentPrecheckCompletedResponseEnvelope")
	if object(t, object(t, completedResponse, "properties"), "status")["const"] != "PRECHECK_COMPLETED" {
		t.Fatal("precheck complete response must have PRECHECK_COMPLETED status")
	}
	assertPrecheckResponseExecutionDisabled(t, completedResponse)
	completedPayload := object(t, object(t, completedResponse, "properties"), "payload")
	completedStatuses, ok := object(t, object(t, completedPayload, "properties"), "status")["enum"].([]any)
	if !ok || len(completedStatuses) != 2 || completedStatuses[0] != "SUCCEEDED" || completedStatuses[1] != "FAILED" {
		t.Fatal("precheck complete response must only confirm SUCCEEDED or FAILED")
	}
}

func TestOpenAPIDataSourceConnectionTestContracts(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	components := object(t, spec, "components")
	requestBodies := object(t, components, "requestBodies")
	responses := object(t, components, "responses")
	schemas := object(t, components, "schemas")

	browserTest := object(t, object(t, paths, "/api/v1/data-sources/{dataSourceId}:test-connection"), "post")
	if browserTest["x-idempotency"] != "REQUIRED" || browserTest["x-optimistic-lock"] != "REQUIRED" {
		t.Fatal("connection test creation must require idempotency and optimistic locking")
	}
	for _, parameter := range []string{"#/components/parameters/CsrfToken", "#/components/parameters/IdempotencyKey", "#/components/parameters/IfMatch"} {
		if !hasParameterReference(browserTest, parameter) {
			t.Fatalf("connection test creation must require %s", parameter)
		}
	}
	if requestBodyReference(t, object(t, browserTest, "requestBody")) != "#/components/requestBodies/DataSourceConnectionTestRequest" {
		t.Fatal("connection test creation must use its strict browser request body")
	}
	for _, status := range []string{"200", "202"} {
		if responseReference(t, browserTest, status) != "#/components/responses/DataSourceConnectionTestAccepted" {
			t.Fatalf("connection test creation %s response must use its dedicated envelope", status)
		}
	}
	assertNoStoreResponse(t, responses, "DataSourceConnectionTestAccepted")
	browserBody := object(t, requestBodies, "DataSourceConnectionTestRequest")
	browserContent := object(t, browserBody, "content")
	browserSchemaReference := fmt.Sprint(object(t, object(t, browserContent, "application/json"), "schema")["$ref"])
	if browserSchemaReference != "#/components/schemas/DataSourceConnectionTestRequest" {
		t.Fatalf("connection test browser request schema = %q", browserSchemaReference)
	}
	browserSchema := object(t, schemas, "DataSourceConnectionTestRequest")
	if browserSchema["additionalProperties"] != false {
		t.Fatal("connection test browser request must reject unknown fields")
	}
	assertRequiredProperties(t, browserSchema, "nodeId")
	browserProperties := object(t, browserSchema, "properties")
	if fmt.Sprint(object(t, browserProperties, "nodeId")["$ref"]) != "#/components/schemas/OpaqueId" {
		t.Fatal("connection test browser request must identify only the selected node")
	}
	for _, forbidden := range []string{"connectionTestId", "host", "port", "username", "password", "url", "jdbcUrl", "sql", "path", "command"} {
		if _, exists := browserProperties[forbidden]; exists {
			t.Fatalf("connection test browser request must not accept %s", forbidden)
		}
	}

	readPath := object(t, paths, "/api/v1/data-source-connection-tests/{connectionTestId}")
	if !hasParameterReference(readPath, "#/components/parameters/ConnectionTestId") {
		t.Fatal("connection test read path must require an opaque connection test ID")
	}
	read := object(t, readPath, "get")
	if responseReference(t, read, "200") != "#/components/responses/DataSourceConnectionTestRead" {
		t.Fatal("connection test read must use its dedicated envelope")
	}
	assertNoStoreResponse(t, responses, "DataSourceConnectionTestRead")

	run := object(t, schemas, "DataSourceConnectionTestRun")
	if run["additionalProperties"] != false {
		t.Fatal("connection test run projection must reject unknown fields")
	}
	assertRequiredProperties(t, run, "id", "dataSourceId", "nodeId", "nodeFactsRevision", "status", "verificationSource", "realConnectionVerified", "sysCredentialConfigured", "createdAt")
	runProperties := object(t, run, "properties")
	for _, idProperty := range []string{"id", "dataSourceId", "nodeId"} {
		if fmt.Sprint(object(t, runProperties, idProperty)["$ref"]) != "#/components/schemas/OpaqueId" {
			t.Fatalf("connection test run %s must use an opaque ID", idProperty)
		}
	}
	assertExactStringEnum(t, object(t, runProperties, "status"), []string{"PENDING", "LEASED", "SUCCEEDED", "FAILED", "UNKNOWN", "EXPIRED", "INVALIDATED"})
	assertExactStringEnum(t, object(t, runProperties, "verificationSource"), []string{"G2_SYNTHETIC", "AGENT_JDBC"})
	assertExactStringEnum(t, object(t, runProperties, "sysVerificationStatus"), []string{"NOT_CONFIGURED", "SUCCEEDED", "FAILED", "UNKNOWN"})
	for _, forbidden := range []string{"host", "port", "username", "password", "jdbcUrl", "databaseVersion", "exception", "metadata"} {
		if _, exists := runProperties[forbidden]; exists {
			t.Fatalf("connection test browser projection must not expose %s", forbidden)
		}
	}
	for _, envelopeName := range []string{"DataSourceConnectionTestMutationEnvelope", "DataSourceConnectionTestReadEnvelope"} {
		envelope := object(t, schemas, envelopeName)
		if envelope["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown fields", envelopeName)
		}
		assertRequiredProperties(t, envelope, "requestId", "item")
		if fmt.Sprint(object(t, object(t, envelope, "properties"), "item")["$ref"]) != "#/components/schemas/DataSourceConnectionTestRun" {
			t.Fatalf("%s must return only the safe connection test projection", envelopeName)
		}
	}
	assertRequiredProperties(t, object(t, schemas, "DataSourceConnectionTestMutationEnvelope"), "replayed")

	agentOperations := []struct {
		path              string
		requestBodyName   string
		requestSchemaName string
		responseName      string
		responseSchema    string
		payloadSchema     string
		payloadType       string
		responseStatus    string
	}{
		{"/agent/v1/data-source-connection-tests:claim-next", "AgentDataSourceConnectionTestClaimNext", "AgentDataSourceConnectionTestClaimNext", "AgentDataSourceConnectionTestClaimed", "AgentDataSourceConnectionTestClaimResponseEnvelope", "AgentDataSourceConnectionTestClaimNextPayload", "DATA_SOURCE_CONNECTION_TEST_CLAIM_NEXT", "DATA_SOURCE_CONNECTION_TEST_CLAIMED"},
		{"/agent/v1/data-source-connection-tests/{connectionTestId}:acknowledge-lease", "AgentDataSourceConnectionTestLeaseAcknowledgement", "AgentDataSourceConnectionTestLeaseAcknowledgement", "AgentDataSourceConnectionTestLeaseAcknowledged", "AgentDataSourceConnectionTestLeaseAcknowledgedResponseEnvelope", "AgentDataSourceConnectionTestLeasePayload", "DATA_SOURCE_CONNECTION_TEST_ACKNOWLEDGE_LEASE", "DATA_SOURCE_CONNECTION_TEST_LEASE_ACKNOWLEDGED"},
		{"/agent/v1/data-source-connection-tests/{connectionTestId}/secret-slots:resolve", "AgentDataSourceConnectionTestSecretSlotResolve", "AgentDataSourceConnectionTestSecretSlotResolve", "AgentDataSourceConnectionTestSecretSlotResolved", "AgentDataSourceConnectionTestSecretSlotResponseEnvelope", "AgentDataSourceConnectionTestSecretSlotPayload", "DATA_SOURCE_CONNECTION_TEST_RESOLVE_SECRET_SLOTS", "DATA_SOURCE_CONNECTION_TEST_SECRET_SLOTS_RESOLVED"},
		{"/agent/v1/data-source-connection-tests/{connectionTestId}:complete", "AgentDataSourceConnectionTestCompletion", "AgentDataSourceConnectionTestCompletion", "AgentDataSourceConnectionTestCompleted", "AgentDataSourceConnectionTestCompletedResponseEnvelope", "AgentDataSourceConnectionTestCompletionPayload", "DATA_SOURCE_CONNECTION_TEST_COMPLETE", "DATA_SOURCE_CONNECTION_TEST_COMPLETED"},
	}
	for _, operationContract := range agentOperations {
		operation := object(t, object(t, paths, operationContract.path), "post")
		if requestBodyReference(t, object(t, operation, "requestBody")) != "#/components/requestBodies/"+operationContract.requestBodyName {
			t.Fatalf("%s must use its strict Agent request body", operationContract.path)
		}
		if responseReference(t, operation, "200") != "#/components/responses/"+operationContract.responseName {
			t.Fatalf("%s must use its strict Agent response", operationContract.path)
		}
		assertNoStoreResponse(t, responses, operationContract.responseName)
		requestBody := object(t, requestBodies, operationContract.requestBodyName)
		requestContent := object(t, requestBody, "content")
		requestSchemaReference := fmt.Sprint(object(t, object(t, requestContent, "application/json"), "schema")["$ref"])
		if requestSchemaReference != "#/components/schemas/"+operationContract.requestSchemaName {
			t.Fatalf("%s request schema = %q", operationContract.path, requestSchemaReference)
		}
		requestSchema := object(t, schemas, operationContract.requestSchemaName)
		if requestSchema["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown envelope fields", operationContract.requestSchemaName)
		}
		assertRequiredProperties(t, requestSchema, "protocolVersion", "agentId", "nodeId", "bootId", "requestId", "sentAt", "payloadType", "payload")
		requestProperties := object(t, requestSchema, "properties")
		if object(t, requestProperties, "protocolVersion")["const"] != "agent-v1" || object(t, requestProperties, "payloadType")["const"] != operationContract.payloadType {
			t.Fatalf("%s must fix the Agent protocol and payload type", operationContract.requestSchemaName)
		}
		if fmt.Sprint(object(t, requestProperties, "payload")["$ref"]) != "#/components/schemas/"+operationContract.payloadSchema {
			t.Fatalf("%s must use its dedicated strict payload", operationContract.requestSchemaName)
		}
		responseSchema := object(t, schemas, operationContract.responseSchema)
		if responseSchema["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown response fields", operationContract.responseSchema)
		}
		assertRequiredProperties(t, responseSchema, "requestId", "serverTime", "status", "payload")
		if object(t, object(t, responseSchema, "properties"), "status")["const"] != operationContract.responseStatus {
			t.Fatalf("%s must have status %s", operationContract.responseSchema, operationContract.responseStatus)
		}
	}

	claim := object(t, object(t, paths, "/agent/v1/data-source-connection-tests:claim-next"), "post")
	if claim["x-capability"] != "DATA_SOURCE_CONNECTION_TEST" || responseReference(t, claim, "204") != "#/components/responses/NoWork" {
		t.Fatal("connection test claim-next must fix its capability and no-work response")
	}
	assertNoStoreResponse(t, responses, "NoWork")
	claimPayload := object(t, schemas, "AgentDataSourceConnectionTestClaimNextPayload")
	if claimPayload["additionalProperties"] != false {
		t.Fatal("connection test claim payload must reject unknown fields")
	}
	assertRequiredProperties(t, claimPayload, "capability")
	if object(t, object(t, claimPayload, "properties"), "capability")["const"] != "DATA_SOURCE_CONNECTION_TEST" {
		t.Fatal("connection test claim payload must request only DATA_SOURCE_CONNECTION_TEST")
	}
	for _, forbidden := range []string{"connectionTestId", "leaseId", "host", "port", "username", "password", "url", "jdbcUrl", "sql", "path", "command"} {
		if _, exists := object(t, claimPayload, "properties")[forbidden]; exists {
			t.Fatalf("connection test claim payload must not accept %s", forbidden)
		}
	}

	for _, payloadName := range []string{"AgentDataSourceConnectionTestLeasePayload", "AgentDataSourceConnectionTestSecretSlotPayload", "AgentDataSourceConnectionTestCompletionPayload"} {
		payload := object(t, schemas, payloadName)
		if payload["additionalProperties"] != false {
			t.Fatalf("%s must reject unknown fields", payloadName)
		}
		assertRequiredProperties(t, payload, "leaseId", "leaseEpoch", "bindingDigest")
		if fmt.Sprint(object(t, object(t, payload, "properties"), "bindingDigest")["$ref"]) != "#/components/schemas/SHA256Digest" {
			t.Fatalf("%s must bind the request to a SHA-256 digest", payloadName)
		}
	}
	secretPayload := object(t, schemas, "AgentDataSourceConnectionTestSecretSlotPayload")
	assertExactStringEnum(t, object(t, object(t, secretPayload, "properties"), "slot"), []string{"DATABASE_CONNECTION", "SYS_CONNECTION"})
	completionPayload := object(t, schemas, "AgentDataSourceConnectionTestCompletionPayload")
	assertRequiredProperties(t, completionPayload, "status", "evidenceCode", "sysVerificationStatus", "sysEvidenceCode")
	if _, exists := object(t, completionPayload, "properties")["succeeded"]; exists {
		t.Fatal("connection test completion must not accept an unconstrained success flag")
	}
	assertExactStringEnum(t, object(t, object(t, completionPayload, "properties"), "status"), []string{"SUCCEEDED", "FAILED", "UNKNOWN"})
	assertExactStringEnum(t, object(t, object(t, completionPayload, "properties"), "evidenceCode"), []string{"SYNTHETIC_OK", "DATABASE_CONNECTED", "DATABASE_HOST_UNRESOLVABLE", "DATABASE_TCP_REFUSED", "DATABASE_TCP_TIMEOUT", "DATABASE_TCP_UNREACHABLE", "DATABASE_CONNECTION_FAILED", "DATABASE_CONNECTION_UNAVAILABLE"})
	assertExactStringEnum(t, object(t, object(t, completionPayload, "properties"), "sysVerificationStatus"), []string{"NOT_CONFIGURED", "SUCCEEDED", "FAILED", "UNKNOWN"})

	binding := object(t, schemas, "AgentDataSourceConnectionTestBinding")
	if binding["additionalProperties"] != false {
		t.Fatal("connection test binding must reject unknown fields")
	}
	assertRequiredProperties(t, binding, "connectionTestId", "dataSourceId", "connectionConfigDigest", "credentialRevision", "nodeId", "nodeFactsRevision")
	bindingProperties := object(t, binding, "properties")
	if fmt.Sprint(object(t, bindingProperties, "connectionConfigDigest")["$ref"]) != "#/components/schemas/SHA256Digest" {
		t.Fatal("connection test binding must carry a SHA-256 connection configuration digest")
	}
	for _, forbidden := range []string{"host", "port", "username", "password", "jdbcUrl"} {
		if _, exists := bindingProperties[forbidden]; exists {
			t.Fatalf("connection test binding must not contain %s", forbidden)
		}
	}

	claimResponse := object(t, schemas, "AgentDataSourceConnectionTestClaimResponseEnvelope")
	claimResponsePayload := object(t, object(t, claimResponse, "properties"), "payload")
	if claimResponsePayload["additionalProperties"] != false {
		t.Fatal("connection test claim response payload must reject unknown fields")
	}
	assertRequiredProperties(t, claimResponsePayload, "connectionTestId", "leaseId", "leaseEpoch", "expiresAt", "binding", "bindingDigest", "verificationSource", "realExecutionEnabled")
	claimResponseProperties := object(t, claimResponsePayload, "properties")
	if fmt.Sprint(object(t, claimResponseProperties, "binding")["$ref"]) != "#/components/schemas/AgentDataSourceConnectionTestBinding" {
		t.Fatal("connection test claim response must carry the immutable binding")
	}
	if fmt.Sprint(object(t, claimResponseProperties, "bindingDigest")["$ref"]) != "#/components/schemas/SHA256Digest" {
		t.Fatal("connection test claim response must carry the binding digest")
	}
	assertExactStringEnum(t, object(t, claimResponseProperties, "verificationSource"), []string{"G2_SYNTHETIC", "AGENT_JDBC"})
	if object(t, claimResponseProperties, "realExecutionEnabled")["const"] != false {
		t.Fatal("connection test claim response must keep real execution disabled")
	}

	secretOperation := object(t, object(t, paths, "/agent/v1/data-source-connection-tests/{connectionTestId}/secret-slots:resolve"), "post")
	if secretOperation["x-secret-response"] != "AGENT_MEMORY_ONLY_NO_STORE" || secretOperation["x-real-execution"] != "DISABLED_UNTIL_G3_VALIDATION" {
		t.Fatal("connection test secret resolution must remain Agent-memory-only and G3-gated")
	}
	secretResponse := object(t, schemas, "AgentDataSourceConnectionTestSecretSlotResponseEnvelope")
	secretResponsePayload := object(t, object(t, secretResponse, "properties"), "payload")
	if secretResponsePayload["additionalProperties"] != false {
		t.Fatal("connection test secret response payload must reject unknown fields")
	}
	assertRequiredProperties(t, secretResponsePayload, "agentRequestId", "connectionTestId", "leaseId", "leaseEpoch", "bindingDigest", "slot", "connection", "realExecutionEnabled")
	secretResponseProperties := object(t, secretResponsePayload, "properties")
	assertExactStringEnum(t, object(t, secretResponseProperties, "slot"), []string{"DATABASE_CONNECTION", "SYS_CONNECTION"})
	if fmt.Sprint(object(t, secretResponseProperties, "bindingDigest")["$ref"]) != "#/components/schemas/SHA256Digest" {
		t.Fatal("connection test secret response must carry the binding digest")
	}
	connection := object(t, schemas, "AgentDataSourceConnectionTestDatabaseConnectionSlot")
	if connection["additionalProperties"] != false {
		t.Fatal("connection test database connection slot must reject unknown fields")
	}
	assertRequiredProperties(t, connection, "host", "port", "username", "password")
	connectionProperties := object(t, connection, "properties")
	for _, secretField := range []string{"username", "password"} {
		field := object(t, connectionProperties, secretField)
		if field["x-agent-memory-only"] != true || field["contentEncoding"] != "base64" {
			t.Fatalf("connection test %s must be an Agent-only short-lived byte value", secretField)
		}
		for _, forbidden := range []string{"example", "default", "minLength", "maxLength"} {
			if _, exists := field[forbidden]; exists {
				t.Fatalf("connection test %s must not expose %s", secretField, forbidden)
			}
		}
	}
	if object(t, secretResponseProperties, "realExecutionEnabled")["const"] != false {
		t.Fatal("connection test secret response must keep real execution disabled")
	}

	completedResponse := object(t, schemas, "AgentDataSourceConnectionTestCompletedResponseEnvelope")
	completedPayload := object(t, object(t, completedResponse, "properties"), "payload")
	if completedPayload["additionalProperties"] != false {
		t.Fatal("connection test complete response payload must reject unknown fields")
	}
	assertRequiredProperties(t, completedPayload, "status", "evidenceCode", "verificationSource", "sysVerificationStatus", "sysEvidenceCode", "realExecutionEnabled")
	completedProperties := object(t, completedPayload, "properties")
	assertExactStringEnum(t, object(t, completedProperties, "status"), []string{"SUCCEEDED", "FAILED", "UNKNOWN"})
	assertExactStringEnum(t, object(t, completedProperties, "evidenceCode"), []string{"SYNTHETIC_OK", "DATABASE_CONNECTED", "DATABASE_HOST_UNRESOLVABLE", "DATABASE_TCP_REFUSED", "DATABASE_TCP_TIMEOUT", "DATABASE_TCP_UNREACHABLE", "DATABASE_CONNECTION_FAILED", "DATABASE_CONNECTION_UNAVAILABLE"})
	assertExactStringEnum(t, object(t, completedProperties, "verificationSource"), []string{"G2_SYNTHETIC", "AGENT_JDBC"})
	assertExactStringEnum(t, object(t, completedProperties, "sysVerificationStatus"), []string{"NOT_CONFIGURED", "SUCCEEDED", "FAILED", "UNKNOWN"})
	if object(t, completedProperties, "realExecutionEnabled")["const"] != false {
		t.Fatal("connection test complete response must keep real execution disabled")
	}
}

func requestBodyReference(t *testing.T, requestBody map[string]any) string {
	t.Helper()
	reference, _ := requestBody["$ref"].(string)
	return reference
}

func assertFixedPrecheckArray(t *testing.T, schema map[string]any, expected []string) {
	t.Helper()
	if schema["items"] != false || schema["minItems"] != float64(len(expected)) || schema["maxItems"] != float64(len(expected)) {
		t.Fatalf("fixed precheck array bounds are invalid: %#v", schema)
	}
	prefixItems, ok := schema["prefixItems"].([]any)
	if !ok || len(prefixItems) != len(expected) {
		t.Fatalf("fixed precheck array length is invalid: %#v", schema)
	}
	for index, expectedCheck := range expected {
		item, ok := prefixItems[index].(map[string]any)
		if !ok || item["const"] != expectedCheck {
			t.Fatalf("fixed precheck item %d = %#v, want %s", index, prefixItems[index], expectedCheck)
		}
	}
}

func assertPrecheckResponseExecutionDisabled(t *testing.T, response map[string]any) {
	t.Helper()
	payload := object(t, object(t, response, "properties"), "payload")
	if variants, ok := payload["oneOf"].([]any); ok {
		if len(variants) == 0 {
			t.Fatal("precheck response payload must declare a non-empty variant set")
		}
		for _, rawVariant := range variants {
			variant := variantObject(t, rawVariant)
			if variant["additionalProperties"] != false {
				t.Fatal("precheck response payload variant must reject unknown fields")
			}
			assertRequiredProperties(t, variant, "realExecutionEnabled")
			if object(t, object(t, variant, "properties"), "realExecutionEnabled")["const"] != false {
				t.Fatal("precheck response must keep real execution disabled")
			}
		}
		return
	}
	if payload["additionalProperties"] != false {
		t.Fatal("precheck response payload must reject unknown fields")
	}
	assertRequiredProperties(t, payload, "realExecutionEnabled")
	if object(t, object(t, payload, "properties"), "realExecutionEnabled")["const"] != false {
		t.Fatal("precheck response must keep real execution disabled")
	}
}

func loadOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	content, err := Files.ReadFile("openapi.json")
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(content, &spec); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	if spec["openapi"] != "3.1.0" || spec["x-development-gate"] != "G2_CONTRACT_ONLY" {
		t.Fatalf("unexpected OpenAPI identity: %#v", spec["info"])
	}
	return spec
}

// variantObject 把 oneOf 数组中的元素断言为对象。
func variantObject(t *testing.T, variant any) map[string]any {
	t.Helper()
	value, ok := variant.(map[string]any)
	if !ok {
		t.Fatalf("oneOf variant is not an object: %#v", variant)
	}
	return value
}

func object(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key]
	if !ok {
		t.Fatalf("missing object %q", key)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%q is %T, want object", key, value)
	}
	return result
}

func assertSecurityDomain(t *testing.T, path string, operation map[string]any) {
	t.Helper()
	security, ok := operation["security"].([]any)
	if !ok {
		t.Fatalf("operation on %s has no explicit security domain", path)
	}
	if path == "/agent/v1/enrollments:exchange" {
		if len(security) != 0 {
			t.Fatal("enrollment exchange must use one-time body material, not an existing Agent credential")
		}
		return
	}
	want := "BrowserSession"
	if strings.HasPrefix(path, "/agent/v1/") {
		want = "AgentCredential"
	}
	if len(security) != 1 {
		t.Fatalf("operation on %s has %d security alternatives, want one", path, len(security))
	}
	entry, ok := security[0].(map[string]any)
	if !ok {
		t.Fatalf("operation on %s has invalid security entry", path)
	}
	if _, ok := entry[want]; !ok || len(entry) != 1 {
		t.Fatalf("operation on %s does not exclusively use %s", path, want)
	}
}

func hasParameterReference(operation map[string]any, reference string) bool {
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, ok := raw.(map[string]any)
		if ok && fmt.Sprint(parameter["$ref"]) == reference {
			return true
		}
	}
	return false
}

func responseReference(t *testing.T, operation map[string]any, status string) string {
	t.Helper()
	response := object(t, object(t, operation, "responses"), status)
	reference, _ := response["$ref"].(string)
	return reference
}

func assertNoStoreResponse(t *testing.T, responses map[string]any, name string) {
	t.Helper()
	response := object(t, responses, name)
	header := object(t, object(t, response, "headers"), "Cache-Control")
	if header["$ref"] != "#/components/headers/NoStore" {
		t.Fatalf("response %s must declare Cache-Control: no-store", name)
	}
}

func assertRequiredProperties(t *testing.T, schema map[string]any, want ...string) {
	t.Helper()
	rawRequired, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema has no required fields: %#v", schema)
	}
	required := make(map[string]bool, len(rawRequired))
	for _, raw := range rawRequired {
		name, ok := raw.(string)
		if !ok {
			t.Fatalf("schema has non-string required field: %#v", raw)
		}
		required[name] = true
	}
	for _, name := range want {
		if !required[name] {
			t.Fatalf("schema does not require %s", name)
		}
	}
}

func assertExactStringEnum(t *testing.T, schema map[string]any, want []string) {
	t.Helper()
	rawValues, ok := schema["enum"].([]any)
	if !ok || len(rawValues) != len(want) {
		t.Fatalf("enum = %#v, want %#v", schema["enum"], want)
	}
	for index, expected := range want {
		value, ok := rawValues[index].(string)
		if !ok || value != expected {
			t.Fatalf("enum[%d] = %#v, want %q", index, rawValues[index], expected)
		}
	}
}

func walkReferences(t *testing.T, root map[string]any, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if reference, ok := typed["$ref"].(string); ok && strings.HasPrefix(reference, "#/") {
			current := any(root)
			for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
				part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
				objectValue, ok := current.(map[string]any)
				if !ok {
					t.Fatalf("reference %q traverses a non-object", reference)
				}
				current, ok = objectValue[part]
				if !ok {
					t.Fatalf("reference %q does not resolve", reference)
				}
			}
		}
		for _, child := range typed {
			walkReferences(t, root, child)
		}
	case []any:
		for _, child := range typed {
			walkReferences(t, root, child)
		}
	}
}
