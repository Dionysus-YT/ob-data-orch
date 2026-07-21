package parammeta

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

//go:embed resources/obdumper-4.3.5-slice-v1.json
var defaultResource []byte

type Rule struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type Definition struct {
	DefinitionID     string   `json:"definitionId"`
	ProductFieldID   string   `json:"productFieldId"`
	LongName         string   `json:"longName"`
	Category         string   `json:"category"`
	Order            int      `json:"order"`
	ValueType        string   `json:"valueType"`
	AllowedValues    []string `json:"allowedValues,omitempty"`
	Cardinality      string   `json:"cardinality"`
	Activation       Rule     `json:"activation"`
	RequiredWhen     Rule     `json:"requiredWhen"`
	ConflictsWith    []string `json:"conflictsWith"`
	DependsOn        []string `json:"dependsOn"`
	SourcePolicy     string   `json:"sourcePolicy"`
	SupportState     string   `json:"supportState"`
	Sensitivity      string   `json:"sensitivity"`
	RiskLevel        string   `json:"riskLevel"`
	ConfirmationRule string   `json:"confirmationRule"`
	OfficialEvidence []string `json:"officialEvidence"`
}

type resource struct {
	MetadataVersion   string       `json:"metadataVersion"`
	Tool              string       `json:"tool"`
	ToolVersion       string       `json:"toolVersion"`
	CapabilityVersion string       `json:"capabilityVersion"`
	SourceDocuments   []string     `json:"sourceDocuments"`
	CategoryOrder     []string     `json:"categoryOrder"`
	Definitions       []Definition `json:"definitions"`
}

type Catalog struct {
	metadataVersion   string
	tool              string
	toolVersion       string
	capabilityVersion string
	categoryOrder     []string
	definitions       []Definition
	byName            map[string]int
}

func LoadDefault() (*Catalog, error) {
	return load(defaultResource)
}

func (c *Catalog) MetadataVersion() string   { return c.metadataVersion }
func (c *Catalog) Tool() string              { return c.tool }
func (c *Catalog) ToolVersion() string       { return c.toolVersion }
func (c *Catalog) CapabilityVersion() string { return c.capabilityVersion }

func (c *Catalog) CategoryOrder() []string {
	return append([]string(nil), c.categoryOrder...)
}

func (c *Catalog) Definitions() []Definition {
	result := make([]Definition, len(c.definitions))
	for index, definition := range c.definitions {
		result[index] = cloneDefinition(definition)
	}
	return result
}

func (c *Catalog) Definition(longName string) (Definition, bool) {
	index, ok := c.byName[longName]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(c.definitions[index]), true
}

func load(content []byte) (*Catalog, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var raw resource
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode parameter metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("parameter metadata contains trailing JSON values")
		}
		return nil, fmt.Errorf("decode parameter metadata trailer: %w", err)
	}
	if err := validateResource(raw); err != nil {
		return nil, err
	}
	definitions := make([]Definition, len(raw.Definitions))
	byName := make(map[string]int, len(raw.Definitions))
	for index, definition := range raw.Definitions {
		definitions[index] = cloneDefinition(definition)
		byName[definition.LongName] = index
	}
	return &Catalog{
		metadataVersion:   raw.MetadataVersion,
		tool:              raw.Tool,
		toolVersion:       raw.ToolVersion,
		capabilityVersion: raw.CapabilityVersion,
		categoryOrder:     append([]string(nil), raw.CategoryOrder...),
		definitions:       definitions,
		byName:            byName,
	}, nil
}

func validateResource(raw resource) error {
	if raw.MetadataVersion != "obdumper-4.3.5-slice-v1" || raw.Tool != "OBDUMPER" || raw.ToolVersion != "4.3.5-RELEASE" {
		return errors.New("parameter metadata identity does not match the confirmed slice")
	}
	if raw.CapabilityVersion != "export-direct-single-table-csv-v1" {
		return errors.New("parameter metadata capability version is unsupported")
	}
	if len(raw.SourceDocuments) == 0 {
		return errors.New("parameter metadata has no source documents")
	}
	expectedCategoryOrder := []string{"CONNECTION", "DATABASE_CONNECTION", "OBJECT_SCOPE", "CONTENT_FORMAT", "FORMAT_SERIALIZATION", "OUTPUT_FILE"}
	if strings.Join(raw.CategoryOrder, "\x00") != strings.Join(expectedCategoryOrder, "\x00") {
		return errors.New("parameter metadata category order does not match the confirmed command order")
	}
	if len(raw.Definitions) != 16 {
		return fmt.Errorf("parameter metadata has %d definitions, want 16", len(raw.Definitions))
	}

	ids := make(map[string]struct{}, len(raw.Definitions))
	names := make(map[string]struct{}, len(raw.Definitions))
	orders := make(map[string]struct{}, len(raw.Definitions))
	for _, definition := range raw.Definitions {
		if definition.DefinitionID == "" || !strings.HasPrefix(definition.ProductFieldID, "EX-F") {
			return fmt.Errorf("parameter %q has no stable product trace", definition.LongName)
		}
		if _, duplicate := ids[definition.DefinitionID]; duplicate {
			return fmt.Errorf("duplicate definition ID %q", definition.DefinitionID)
		}
		ids[definition.DefinitionID] = struct{}{}
		if !strings.HasPrefix(definition.LongName, "--") {
			return fmt.Errorf("parameter %q is not a canonical long name", definition.LongName)
		}
		if _, duplicate := names[definition.LongName]; duplicate {
			return fmt.Errorf("duplicate parameter name %q", definition.LongName)
		}
		names[definition.LongName] = struct{}{}
		orderKey := fmt.Sprintf("%s/%08d", definition.Category, definition.Order)
		if definition.Category == "" || definition.Order <= 0 {
			return fmt.Errorf("parameter %q has no stable category order", definition.LongName)
		}
		if _, duplicate := orders[orderKey]; duplicate {
			return fmt.Errorf("duplicate parameter order %q", orderKey)
		}
		orders[orderKey] = struct{}{}
		if definition.Cardinality != "SINGLE" {
			return fmt.Errorf("parameter %q has unsupported cardinality", definition.LongName)
		}
		if !oneOf(definition.ValueType, "flag", "string", "integer", "enum", "path", "secret-slot") {
			return fmt.Errorf("parameter %q has unsupported value type", definition.LongName)
		}
		if !oneOf(definition.SupportState, "ENABLED", "VALIDATION_GATED") {
			return fmt.Errorf("parameter %q has unsupported support state", definition.LongName)
		}
		if !oneOf(definition.SourcePolicy, "USER", "DATA_SOURCE", "SECURITY", "FORMAT") {
			return fmt.Errorf("parameter %q has unsupported source policy", definition.LongName)
		}
		if !oneOf(definition.Sensitivity, "NORMAL", "IDENTIFIER", "SECRET") {
			return fmt.Errorf("parameter %q has unsupported sensitivity", definition.LongName)
		}
		if !oneOf(definition.RiskLevel, "LOW", "MEDIUM", "HIGH") {
			return fmt.Errorf("parameter %q has unsupported risk level", definition.LongName)
		}
		if !oneOf(definition.ConfirmationRule, "NONE", "SECRET_REFERENCE_ONLY") {
			return fmt.Errorf("parameter %q has unsupported confirmation rule", definition.LongName)
		}
		if len(definition.OfficialEvidence) == 0 {
			return fmt.Errorf("parameter %q has no evidence reference", definition.LongName)
		}
		if err := validateRule(definition.LongName, definition.Activation, "ALWAYS", "FORMAT_IS"); err != nil {
			return err
		}
		if err := validateRule(definition.LongName, definition.RequiredWhen, "SLICE_SUBMISSION", "NEVER"); err != nil {
			return err
		}
		if definition.ValueType == "enum" && len(definition.AllowedValues) == 0 {
			return fmt.Errorf("enum parameter %q has no allowed values", definition.LongName)
		}
	}
	dependencies := make(map[string][]string, len(raw.Definitions))
	for _, definition := range raw.Definitions {
		for _, reference := range append(append([]string(nil), definition.DependsOn...), definition.ConflictsWith...) {
			if reference == definition.LongName {
				return fmt.Errorf("parameter %q references itself", definition.LongName)
			}
			if _, ok := names[reference]; !ok {
				return fmt.Errorf("parameter %q references unknown parameter %q", definition.LongName, reference)
			}
		}
		dependencies[definition.LongName] = append([]string(nil), definition.DependsOn...)
	}
	if err := rejectDependencyCycles(dependencies); err != nil {
		return err
	}
	if err := requireNamesByState(raw.Definitions, "ENABLED", []string{
		"--host", "--port", "--user", "--password", "--database", "--table", "--csv", "--file-path",
	}); err != nil {
		return err
	}
	if err := requireNamesByState(raw.Definitions, "VALIDATION_GATED", []string{
		"--skip-header", "--column-separator", "--column-quote", "--column-quote-mode",
		"--escape-character", "--line-separator", "--null-string", "--file-encoding",
	}); err != nil {
		return err
	}
	passwordIndex := -1
	for index, definition := range raw.Definitions {
		if definition.LongName == "--password" {
			passwordIndex = index
			break
		}
	}
	if passwordIndex < 0 || raw.Definitions[passwordIndex].ValueType != "secret-slot" || raw.Definitions[passwordIndex].Sensitivity != "SECRET" {
		return errors.New("password metadata must be a secret slot")
	}
	return nil
}

func rejectDependencyCycles(dependencies map[string][]string) error {
	state := make(map[string]uint8, len(dependencies))
	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("parameter dependency cycle includes %q", name)
		case 2:
			return nil
		}
		state[name] = 1
		for _, dependency := range dependencies[name] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	for name := range dependencies {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func validateRule(parameter string, rule Rule, allowed ...string) error {
	if !oneOf(rule.Kind, allowed...) {
		return fmt.Errorf("parameter %q has unsupported rule kind %q", parameter, rule.Kind)
	}
	if rule.Kind == "FORMAT_IS" && rule.Value != "CSV" {
		return fmt.Errorf("parameter %q has unsupported format rule", parameter)
	}
	if rule.Kind != "FORMAT_IS" && rule.Value != "" {
		return fmt.Errorf("parameter %q has an unexpected rule value", parameter)
	}
	return nil
}

func requireNamesByState(definitions []Definition, state string, expected []string) error {
	actual := make([]string, 0, len(expected))
	for _, definition := range definitions {
		if definition.SupportState == state {
			actual = append(actual, definition.LongName)
		}
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if strings.Join(actual, "\x00") != strings.Join(expected, "\x00") {
		return fmt.Errorf("%s parameter set does not match the confirmed slice", state)
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func cloneDefinition(input Definition) Definition {
	result := input
	result.AllowedValues = append([]string(nil), input.AllowedValues...)
	result.ConflictsWith = append([]string(nil), input.ConflictsWith...)
	result.DependsOn = append([]string(nil), input.DependsOn...)
	result.OfficialEvidence = append([]string(nil), input.OfficialEvidence...)
	return result
}
