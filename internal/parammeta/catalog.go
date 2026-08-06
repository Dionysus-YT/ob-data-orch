package parammeta

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
)

const (
	defaultRevisionResource = "resources/obdumper-4.3.5-slice-v5.json"
	currentMetadataVersion  = "obdumper-4.3.5-slice-v5"
)

// resourceFiles 只嵌入 resources/ 下已确认可加载的参数元数据。
// 尚未完成事实核验或加载器支持的设计稿（如 v6 泛化元数据）放在 drafts/ 目录，
// 不被嵌入也不得被声明为已支持；接入前必须先通过对应切片的验证门禁。
//
//go:embed resources/*.json
var resourceFiles embed.FS

type Rule struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type Definition struct {
	DefinitionID     string   `json:"definitionId"`
	ProductFieldID   string   `json:"productFieldId"`
	LongName         string   `json:"longName"`
	ShortName        string   `json:"shortName,omitempty"`
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
	EmissionTarget   string   `json:"emissionTarget,omitempty"`
	SecurityProperty string   `json:"securityProperty,omitempty"`
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

type revisionManifest struct {
	MetadataVersion   string               `json:"metadataVersion"`
	BaseVersion       string               `json:"baseVersion"`
	BaseResource      string               `json:"baseResource"`
	BaseSHA256        string               `json:"baseSha256"`
	CapabilityVersion string               `json:"capabilityVersion,omitempty"`
	RevisionReason    string               `json:"revisionReason"`
	Overrides         []definitionOverride `json:"overrides"`
	Additions         []Definition         `json:"additions"`
}

type definitionOverride struct {
	DefinitionID     string   `json:"definitionId"`
	ShortName        string   `json:"shortName,omitempty"`
	EmissionTarget   string   `json:"emissionTarget"`
	SecurityProperty string   `json:"securityProperty,omitempty"`
	AppendEvidence   []string `json:"appendEvidence"`
}

type Catalog struct {
	metadataVersion   string
	baseVersion       string
	revisionReason    string
	tool              string
	toolVersion       string
	capabilityVersion string
	categoryOrder     []string
	definitions       []Definition
	byName            map[string]int
}

func LoadDefault() (*Catalog, error) {
	return loadFromFS(resourceFiles, defaultRevisionResource)
}

func loadFromFS(files fs.FS, revisionResource string) (*Catalog, error) {
	manifestContent, err := fs.ReadFile(files, revisionResource)
	if err != nil {
		return nil, fmt.Errorf("read parameter metadata revision: %w", err)
	}
	manifest, err := decodeManifest(manifestContent)
	if err != nil {
		return nil, err
	}
	baseContent, err := fs.ReadFile(files, "resources/"+manifest.BaseResource)
	if err != nil {
		return nil, fmt.Errorf("read parameter metadata base: %w", err)
	}
	digest := sha256.Sum256(baseContent)
	if fmt.Sprintf("%x", digest) != manifest.BaseSHA256 {
		return nil, errors.New("parameter metadata base checksum mismatch")
	}
	raw, err := decodeResource(baseContent)
	if err != nil {
		return nil, err
	}
	if raw.MetadataVersion != manifest.BaseVersion {
		return nil, errors.New("parameter metadata base version mismatch")
	}
	for index := range raw.Definitions {
		raw.Definitions[index].EmissionTarget = "ARGV"
	}
	if err := applyOverrides(&raw, manifest.Overrides); err != nil {
		return nil, err
	}
	if err := applyAdditions(&raw, manifest.Additions); err != nil {
		return nil, err
	}
	raw.MetadataVersion = manifest.MetadataVersion
	if manifest.CapabilityVersion != "" {
		raw.CapabilityVersion = manifest.CapabilityVersion
	}
	return buildCatalog(raw, manifest.BaseVersion, manifest.RevisionReason)
}

func (c *Catalog) MetadataVersion() string   { return c.metadataVersion }
func (c *Catalog) BaseVersion() string       { return c.baseVersion }
func (c *Catalog) RevisionReason() string    { return c.revisionReason }
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

func decodeResource(content []byte) (resource, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var raw resource
	if err := decoder.Decode(&raw); err != nil {
		return resource{}, fmt.Errorf("decode parameter metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return resource{}, errors.New("parameter metadata contains trailing JSON values")
		}
		return resource{}, fmt.Errorf("decode parameter metadata trailer: %w", err)
	}
	return raw, nil
}

func buildCatalog(raw resource, baseVersion string, revisionReason string) (*Catalog, error) {
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
		baseVersion:       baseVersion,
		revisionReason:    revisionReason,
		tool:              raw.Tool,
		toolVersion:       raw.ToolVersion,
		capabilityVersion: raw.CapabilityVersion,
		categoryOrder:     append([]string(nil), raw.CategoryOrder...),
		definitions:       definitions,
		byName:            byName,
	}, nil
}

func decodeManifest(content []byte) (revisionManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var manifest revisionManifest
	if err := decoder.Decode(&manifest); err != nil {
		return revisionManifest{}, fmt.Errorf("decode parameter metadata revision: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return revisionManifest{}, errors.New("parameter metadata revision contains trailing JSON values")
	}
	if manifest.MetadataVersion != currentMetadataVersion || manifest.BaseVersion != "obdumper-4.3.5-slice-v1" {
		return revisionManifest{}, errors.New("parameter metadata revision identity is unsupported")
	}
	if manifest.BaseResource != "obdumper-4.3.5-slice-v1.json" || len(manifest.BaseSHA256) != 64 {
		return revisionManifest{}, errors.New("parameter metadata revision base is invalid")
	}
	if manifest.RevisionReason == "" || len(manifest.Overrides) != 4 || len(manifest.Additions) != 2 {
		return revisionManifest{}, errors.New("parameter metadata revision content is invalid")
	}
	return manifest, nil
}

// applyAdditions 将当前版本确认新增的参数定义附加到不可变基础元数据。
// 仅允许清单声明的受控定义进入目录，避免调用方以自由字段绕过参数校验。
func applyAdditions(raw *resource, additions []Definition) error {
	if raw == nil || len(additions) != 2 {
		return errors.New("parameter metadata additions are invalid")
	}
	raw.Definitions = append(raw.Definitions, additions...)
	return nil
}

func applyOverrides(raw *resource, overrides []definitionOverride) error {
	byID := make(map[string]int, len(raw.Definitions))
	for index, definition := range raw.Definitions {
		byID[definition.DefinitionID] = index
	}
	seen := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		if _, duplicate := seen[override.DefinitionID]; duplicate {
			return fmt.Errorf("duplicate parameter metadata override %q", override.DefinitionID)
		}
		seen[override.DefinitionID] = struct{}{}
		index, ok := byID[override.DefinitionID]
		if !ok {
			return fmt.Errorf("parameter metadata override %q is unknown", override.DefinitionID)
		}
		raw.Definitions[index].EmissionTarget = override.EmissionTarget
		raw.Definitions[index].SecurityProperty = override.SecurityProperty
		raw.Definitions[index].ShortName = override.ShortName
		raw.Definitions[index].OfficialEvidence = append(raw.Definitions[index].OfficialEvidence, override.AppendEvidence...)
	}
	return nil
}

func validateResource(raw resource) error {
	if raw.MetadataVersion != currentMetadataVersion || raw.Tool != "OBDUMPER" || raw.ToolVersion != "4.3.5-RELEASE" {
		return errors.New("parameter metadata identity does not match the confirmed slice")
	}
	if raw.CapabilityVersion != "export-odp-single-table-csv-v1" {
		return errors.New("parameter metadata capability version is unsupported")
	}
	if len(raw.SourceDocuments) == 0 {
		return errors.New("parameter metadata has no source documents")
	}
	expectedCategoryOrder := []string{"CONNECTION", "DATABASE_CONNECTION", "OBJECT_SCOPE", "CONTENT_FORMAT", "FORMAT_SERIALIZATION", "OUTPUT_FILE"}
	if strings.Join(raw.CategoryOrder, "\x00") != strings.Join(expectedCategoryOrder, "\x00") {
		return errors.New("parameter metadata category order does not match the confirmed command order")
	}
	if len(raw.Definitions) != 18 {
		return fmt.Errorf("parameter metadata has %d definitions, want 18", len(raw.Definitions))
	}

	ids := make(map[string]struct{}, len(raw.Definitions))
	names := make(map[string]struct{}, len(raw.Definitions))
	shortNames := make(map[string]struct{}, len(raw.Definitions))
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
		if definition.ShortName != "" {
			if !strings.HasPrefix(definition.ShortName, "-") || strings.HasPrefix(definition.ShortName, "--") || len(definition.ShortName) != 2 {
				return fmt.Errorf("parameter %q has an invalid short name", definition.LongName)
			}
			if _, duplicate := shortNames[definition.ShortName]; duplicate {
				return fmt.Errorf("duplicate short parameter name %q", definition.ShortName)
			}
			shortNames[definition.ShortName] = struct{}{}
		}
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
		if !oneOf(definition.EmissionTarget, "ARGV", "SECURITY_FILE") {
			return fmt.Errorf("parameter %q has unsupported emission target", definition.LongName)
		}
		if definition.EmissionTarget == "SECURITY_FILE" && definition.SecurityProperty == "" {
			return fmt.Errorf("parameter %q has no security property", definition.LongName)
		}
		if definition.EmissionTarget != "SECURITY_FILE" && definition.SecurityProperty != "" {
			return fmt.Errorf("parameter %q has an unexpected security property", definition.LongName)
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
		"--host", "--port", "--user", "--password", "--database", "--table", "--csv", "--file-path", "--log-path", "--skip-check-dir",
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
	password := raw.Definitions[passwordIndex]
	if password.EmissionTarget != "SECURITY_FILE" || password.SecurityProperty != "oceanbase.jdbc.password" {
		return errors.New("password metadata must target the official security file")
	}
	for _, definition := range raw.Definitions {
		if definition.LongName != "--password" && definition.EmissionTarget != "ARGV" {
			return fmt.Errorf("parameter %q must remain an argv target", definition.LongName)
		}
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
