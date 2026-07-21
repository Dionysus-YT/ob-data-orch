package contracts

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOpenAPICoversConfirmedOperations(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	expected := map[string][]string{
		"/api/v1/session":                                         {"get"},
		"/api/v1/execution-nodes":                                 {"get", "post"},
		"/api/v1/execution-nodes/{nodeId}":                        {"get", "patch"},
		"/api/v1/execution-nodes/{nodeId}:create-enrollment":      {"post"},
		"/api/v1/data-sources":                                    {"get", "post"},
		"/api/v1/data-sources/{dataSourceId}":                     {"get", "patch", "delete"},
		"/api/v1/data-sources/{dataSourceId}:test-connection":     {"post"},
		"/api/v1/data-sources/{dataSourceId}:disable":             {"post"},
		"/api/v1/data-sources/{dataSourceId}:enable":              {"post"},
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
		"/agent/v1/heartbeats":                                    {"post"},
		"/agent/v1/executions:claim":                              {"post"},
		"/agent/v1/executions/{executionId}:acknowledge-lease":    {"post"},
		"/agent/v1/executions/{executionId}:renew-lease":          {"post"},
		"/agent/v1/executions/{executionId}/secret-slots:resolve": {"post"},
		"/agent/v1/executions/{executionId}/events:append":        {"post"},
		"/agent/v1/executions/{executionId}/logs:append":          {"post"},
		"/agent/v1/executions/{executionId}:reconcile":            {"post"},
		"/agent/v1/executions/{executionId}:release":              {"post"},
		"/agent/v1/prechecks:claim":                               {"post"},
		"/agent/v1/prechecks/{precheckId}:acknowledge-lease":      {"post"},
		"/agent/v1/prechecks/{precheckId}/secret-slots:resolve":   {"post"},
		"/agent/v1/prechecks/{precheckId}:complete":               {"post"},
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
	if operationCount != 43 {
		t.Fatalf("operation count = %d, want 43", operationCount)
	}
}

func TestOpenAPIKeepsRealExecutionClosed(t *testing.T) {
	t.Parallel()
	spec := loadOpenAPI(t)
	if enabled, ok := spec["x-real-execution-enabled"].(bool); !ok || enabled {
		t.Fatal("OpenAPI must explicitly keep real execution disabled")
	}
	paths := object(t, spec, "paths")
	for path := range paths {
		lower := strings.ToLower(path)
		if strings.HasPrefix(path, "/api/v1/") && (strings.Contains(lower, ":cancel") || strings.Contains(lower, ":retry") || strings.Contains(lower, ":execute")) {
			t.Fatalf("unconfirmed browser execution operation present: %s", path)
		}
	}
	submit := object(t, object(t, paths, "/api/v1/export-drafts/{draftId}:submit"), "post")
	if submit["x-real-execution"] != "DISABLED_UNTIL_G3" {
		t.Fatal("task submission contract does not expose the G3 execution gate")
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
	gated := object(t, schemas, "GatedCsvOptions")
	if gated["x-support-state"] != "VALIDATION_GATED" {
		t.Fatal("CSV options must remain validation gated")
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
