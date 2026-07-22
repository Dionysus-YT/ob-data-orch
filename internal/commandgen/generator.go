package commandgen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ob-data-orch/internal/parammeta"
)

const (
	secretSlotID        = "database-password"
	secretSlotTarget    = "OFFICIAL_SECURITY_FILE_PROPERTY"
	secretSourceSummary = "密码来源：任务级官方加密配置（已脱敏）"
	redactedIdentifier  = "******"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type metadataCatalog interface {
	MetadataVersion() string
	Tool() string
	ToolVersion() string
	CapabilityVersion() string
	CategoryOrder() []string
	Definitions() []parammeta.Definition
}

type Generator struct {
	metadataVersion   string
	tool              string
	toolVersion       string
	capabilityVersion string
	definitions       []parammeta.Definition
	byName            map[string]parammeta.Definition
}

func NewDefault() (*Generator, error) {
	catalog, err := parammeta.LoadDefault()
	if err != nil {
		return nil, err
	}
	return newGenerator(catalog)
}

func newGenerator(catalog metadataCatalog) (*Generator, error) {
	if catalog == nil {
		return nil, fmt.Errorf("parameter catalog is nil")
	}
	categoryOrder := catalog.CategoryOrder()
	categoryIndex := make(map[string]int, len(categoryOrder))
	for index, category := range categoryOrder {
		categoryIndex[category] = index
	}
	definitions := catalog.Definitions()
	sort.SliceStable(definitions, func(i, j int) bool {
		left, leftOK := categoryIndex[definitions[i].Category]
		right, rightOK := categoryIndex[definitions[j].Category]
		if !leftOK || !rightOK {
			return definitions[i].Category < definitions[j].Category
		}
		if left != right {
			return left < right
		}
		if definitions[i].Order != definitions[j].Order {
			return definitions[i].Order < definitions[j].Order
		}
		return definitions[i].LongName < definitions[j].LongName
	})
	byName := make(map[string]parammeta.Definition, len(definitions))
	for _, definition := range definitions {
		if _, exists := categoryIndex[definition.Category]; !exists {
			return nil, fmt.Errorf("parameter %q has an unknown category", definition.LongName)
		}
		if _, duplicate := byName[definition.LongName]; duplicate {
			return nil, fmt.Errorf("duplicate parameter %q", definition.LongName)
		}
		byName[definition.LongName] = definition
	}
	return &Generator{
		metadataVersion:   catalog.MetadataVersion(),
		tool:              catalog.Tool(),
		toolVersion:       catalog.ToolVersion(),
		capabilityVersion: catalog.CapabilityVersion(),
		definitions:       definitions,
		byName:            byName,
	}, nil
}

func (g *Generator) Generate(request Request) (Result, error) {
	issues := g.validateRequestIdentity(request)
	provided := make(map[string]FieldInput, len(request.Fields))
	for _, field := range request.Fields {
		if _, known := g.byName[field.Name]; !known {
			issues = append(issues, Issue{Code: "UNKNOWN_PARAMETER", Field: field.Name})
			continue
		}
		if _, duplicate := provided[field.Name]; duplicate {
			issues = append(issues, Issue{Code: "DUPLICATE_PARAMETER", Field: field.Name})
			continue
		}
		provided[field.Name] = cloneFieldInput(field)
	}

	format := ""
	if csv, ok := provided["--csv"]; ok && csv.Value.Kind == ValueBoolean && csv.Value.Boolean {
		format = "CSV"
	}
	normalized := make([]NormalizedField, 0, len(g.definitions))
	for _, definition := range g.definitions {
		field, present := provided[definition.LongName]
		active := isActive(definition, format)
		if !active {
			normalized = append(normalized, NormalizedField{
				DefinitionID: definition.DefinitionID,
				Name:         definition.LongName,
				State:        StateInactive,
			})
			continue
		}
		if !present || (definition.ValueType == "flag" && field.Value.Kind == ValueBoolean && !field.Value.Boolean) {
			state := StateUnset
			if definition.RequiredWhen.Kind == "SLICE_SUBMISSION" {
				state = StateBlocked
				issues = append(issues, Issue{Code: "REQUIRED_PARAMETER_MISSING", Field: definition.LongName})
			}
			normalized = append(normalized, NormalizedField{
				DefinitionID: definition.DefinitionID,
				Name:         definition.LongName,
				State:        state,
			})
			continue
		}
		state := stateForSource(field.Source)
		if !sourceMatches(definition.SourcePolicy, field.Source) {
			state = StateBlocked
			issues = append(issues, Issue{Code: "PARAMETER_SOURCE_MISMATCH", Field: definition.LongName})
		}
		value, valueIssues := normalizeValue(definition, field.Value, request.TargetPlatform)
		if len(valueIssues) > 0 {
			state = StateBlocked
			for _, code := range valueIssues {
				issues = append(issues, Issue{Code: code, Field: definition.LongName})
			}
		}
		if definition.SupportState == "VALIDATION_GATED" {
			state = StateBlocked
			issues = append(issues, Issue{Code: "PARAMETER_VALIDATION_GATED", Field: definition.LongName})
		}
		normalized = append(normalized, NormalizedField{
			DefinitionID: definition.DefinitionID,
			Name:         definition.LongName,
			State:        state,
			Source:       field.Source,
			Value:        value,
		})
	}
	if len(issues) > 0 {
		sortIssues(issues)
		return Result{}, &ValidationError{Issues: issues}
	}

	argv := make([]string, 0, 15)
	display := make([]string, 0, 15)
	tokenEvidence := make([]TokenEvidence, 0, 7)
	secretSlots := make([]SecretSlot, 0, 1)
	for index, field := range normalized {
		if field.State != StateExplicit && field.State != StateDerived {
			continue
		}
		definition := g.definitions[index]
		if definition.EmissionTarget == "SECURITY_FILE" {
			secretSlots = append(secretSlots, SecretSlot{
				SlotID:              secretSlotID,
				Target:              secretSlotTarget,
				SecurityProperty:    definition.SecurityProperty,
				CredentialReference: *field.Value.Secret,
			})
			continue
		}
		start := len(argv)
		argv = append(argv, definition.LongName)
		display = append(display, definition.LongName)
		if definition.ValueType != "flag" {
			value := normalizedValueString(field.Value)
			argv = append(argv, value)
			if definition.Sensitivity == "IDENTIFIER" {
				display = append(display, redactedIdentifier)
			} else {
				display = append(display, value)
			}
		}
		tokenEvidence = append(tokenEvidence, TokenEvidence{
			DefinitionID: definition.DefinitionID,
			Parameter:    definition.LongName,
			Source:       field.Source,
			ArgvStart:    start,
			ArgvLength:   len(argv) - start,
		})
	}
	fingerprint, err := fingerprint(request, g, normalized)
	if err != nil {
		return Result{}, fmt.Errorf("build configuration fingerprint: %w", err)
	}
	return Result{
		Tool:                g.tool,
		ToolVersion:         g.toolVersion,
		MetadataVersion:     g.metadataVersion,
		CapabilityVersion:   g.capabilityVersion,
		NormalizedFields:    normalized,
		ArgvTemplate:        argv,
		RedactedCommand:     renderCommand(request.TargetPlatform, "obdumper", display),
		SecretSlots:         secretSlots,
		SecretSourceSummary: secretSourceSummary,
		TokenEvidence:       tokenEvidence,
		ConfigFingerprint:   fingerprint,
	}, nil
}

func (g *Generator) validateRequestIdentity(request Request) []Issue {
	var issues []Issue
	if request.Tool != g.tool {
		issues = append(issues, Issue{Code: "TOOL_MISMATCH", Field: "tool"})
	}
	if request.ToolVersion != g.toolVersion {
		issues = append(issues, Issue{Code: "TOOL_VERSION_MISMATCH", Field: "toolVersion"})
	}
	if request.MetadataVersion != g.metadataVersion {
		issues = append(issues, Issue{Code: "METADATA_VERSION_MISMATCH", Field: "metadataVersion"})
	}
	if request.CapabilityVersion != g.capabilityVersion {
		issues = append(issues, Issue{Code: "CAPABILITY_VERSION_MISMATCH", Field: "capabilityVersion"})
	}
	if request.ConnectionKind != ConnectionODP {
		issues = append(issues, Issue{Code: "CONNECTION_KIND_NOT_IN_SLICE", Field: "connectionKind"})
	}
	if request.DataSourceFactVersion == "" {
		issues = append(issues, Issue{Code: "DATA_SOURCE_FACT_VERSION_MISSING", Field: "dataSourceFactVersion"})
	}
	if request.NodeFactVersion == "" {
		issues = append(issues, Issue{Code: "NODE_FACT_VERSION_MISSING", Field: "nodeFactVersion"})
	}
	if request.TargetPlatform != PlatformWindowsAMD64 && request.TargetPlatform != PlatformLinuxAMD64 && request.TargetPlatform != PlatformLinuxARM64 {
		issues = append(issues, Issue{Code: "TARGET_PLATFORM_UNSUPPORTED", Field: "targetPlatform"})
	}
	return issues
}

func normalizeValue(definition parammeta.Definition, value Value, platform Platform) (NormalizedValue, []string) {
	if hasMixedValue(value) {
		return NormalizedValue{}, []string{"PARAMETER_VALUE_AMBIGUOUS"}
	}
	switch definition.ValueType {
	case "flag":
		if value.Kind != ValueBoolean {
			return NormalizedValue{}, []string{"PARAMETER_TYPE_MISMATCH"}
		}
		copy := value.Boolean
		return NormalizedValue{Kind: value.Kind, Boolean: &copy}, nil
	case "integer":
		if value.Kind != ValueInteger {
			return NormalizedValue{}, []string{"PARAMETER_TYPE_MISMATCH"}
		}
		if definition.LongName == "--port" && (value.Integer < 1 || value.Integer > 65535) {
			return NormalizedValue{}, []string{"PARAMETER_VALUE_OUT_OF_RANGE"}
		}
		copy := value.Integer
		return NormalizedValue{Kind: value.Kind, Integer: &copy}, nil
	case "string", "enum", "path":
		if value.Kind != ValueString {
			return NormalizedValue{}, []string{"PARAMETER_TYPE_MISMATCH"}
		}
		if value.String == "" || strings.ContainsRune(value.String, 0) {
			return NormalizedValue{}, []string{"PARAMETER_VALUE_INVALID"}
		}
		if definition.ValueType == "enum" && !contains(definition.AllowedValues, value.String) {
			return NormalizedValue{}, []string{"PARAMETER_ENUM_INVALID"}
		}
		if definition.LongName == "--table" && (strings.Contains(value.String, "*") || strings.Contains(value.String, ",")) {
			return NormalizedValue{}, []string{"TABLE_SCOPE_NOT_SINGLE"}
		}
		if definition.ValueType == "path" && !validAbsolutePath(platform, value.String) {
			return NormalizedValue{}, []string{"OUTPUT_PATH_NOT_ABSOLUTE"}
		}
		return NormalizedValue{Kind: value.Kind, String: value.String}, nil
	case "secret-slot":
		if value.Kind != ValueSecretReference || value.Secret == nil {
			return NormalizedValue{}, []string{"SECRET_REFERENCE_REQUIRED"}
		}
		if !uuidPattern.MatchString(value.Secret.CredentialID) || value.Secret.Revision < 1 {
			return NormalizedValue{}, []string{"SECRET_REFERENCE_INVALID"}
		}
		copy := *value.Secret
		return NormalizedValue{Kind: value.Kind, Secret: &copy}, nil
	default:
		return NormalizedValue{}, []string{"PARAMETER_TYPE_UNSUPPORTED"}
	}
}

func hasMixedValue(value Value) bool {
	switch value.Kind {
	case ValueString:
		return value.Secret != nil || value.Integer != 0 || value.Boolean
	case ValueInteger:
		return value.Secret != nil || value.String != "" || value.Boolean
	case ValueBoolean:
		return value.Secret != nil || value.String != "" || value.Integer != 0
	case ValueSecretReference:
		return value.Secret == nil || value.String != "" || value.Integer != 0 || value.Boolean
	default:
		return true
	}
}

func isActive(definition parammeta.Definition, format string) bool {
	switch definition.Activation.Kind {
	case "ALWAYS":
		return true
	case "FORMAT_IS":
		return definition.Activation.Value == format
	default:
		return false
	}
}

func sourceMatches(policy string, source ValueSource) bool {
	return policy == string(source)
}

func stateForSource(source ValueSource) FieldState {
	if source == SourceDataSource || source == SourceSecurity {
		return StateDerived
	}
	return StateExplicit
}

func normalizedValueString(value NormalizedValue) string {
	switch value.Kind {
	case ValueString:
		return value.String
	case ValueInteger:
		return strconv.FormatInt(*value.Integer, 10)
	case ValueBoolean:
		return strconv.FormatBool(*value.Boolean)
	default:
		return ""
	}
}

func validAbsolutePath(platform Platform, value string) bool {
	switch platform {
	case PlatformWindowsAMD64:
		return len(value) >= 3 && isASCIILetter(value[0]) && value[1] == ':' && value[2] == '\\'
	case PlatformLinuxAMD64, PlatformLinuxARM64:
		return strings.HasPrefix(value, "/")
	default:
		return false
	}
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Field != issues[j].Field {
			return issues[i].Field < issues[j].Field
		}
		return issues[i].Code < issues[j].Code
	})
}

func cloneFieldInput(field FieldInput) FieldInput {
	result := field
	if field.Value.Secret != nil {
		copy := *field.Value.Secret
		result.Value.Secret = &copy
	}
	return result
}

type fingerprintDocument struct {
	Tool                  string            `json:"tool"`
	ToolVersion           string            `json:"toolVersion"`
	MetadataVersion       string            `json:"metadataVersion"`
	CapabilityVersion     string            `json:"capabilityVersion"`
	ConnectionKind        ConnectionKind    `json:"connectionKind"`
	DataSourceFactVersion string            `json:"dataSourceFactVersion"`
	NodeFactVersion       string            `json:"nodeFactVersion"`
	TargetPlatform        Platform          `json:"targetPlatform"`
	Fields                []NormalizedField `json:"fields"`
}

func fingerprint(request Request, generator *Generator, fields []NormalizedField) (string, error) {
	document := fingerprintDocument{
		Tool:                  generator.tool,
		ToolVersion:           generator.toolVersion,
		MetadataVersion:       generator.metadataVersion,
		CapabilityVersion:     generator.capabilityVersion,
		ConnectionKind:        request.ConnectionKind,
		DataSourceFactVersion: request.DataSourceFactVersion,
		NodeFactVersion:       request.NodeFactVersion,
		TargetPlatform:        request.TargetPlatform,
		Fields:                fields,
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
