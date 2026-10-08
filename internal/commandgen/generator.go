package commandgen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ob-data-orch/internal/outputpath"
	"ob-data-orch/internal/parammeta"
)

const (
	secretSlotID        = "database-password"
	secretSlotTarget    = "OFFICIAL_SECURITY_FILE_PROPERTY"
	secretSourceSummary = "密码来源：任务级官方加密配置（已脱敏）"
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
	// versions 只存在于泛化生成器入口，用于按草稿冻结的元数据版本重放历史命令。
	// 子生成器不再持有该映射，避免递归分派或把旧版本当作新建草稿默认值。
	versions map[string]*Generator
}

func NewDefault() (*Generator, error) {
	catalog, err := parammeta.LoadDefault()
	if err != nil {
		return nil, err
	}
	return newGenerator(catalog)
}

// NewGeneralized 构建现行 v7 泛化生成器，并加载只用于历史草稿重放的冻结 v6 目录。
// 新请求默认由调用方声明 v7；只有携带已冻结 v6 metadataVersion 的请求才会分派到旧目录。
func NewGeneralized() (*Generator, error) {
	catalog, err := parammeta.LoadGeneralized()
	if err != nil {
		return nil, err
	}
	current, err := newGenerator(catalog)
	if err != nil {
		return nil, err
	}
	legacyCatalog, err := parammeta.LoadLegacyGeneralized()
	if err != nil {
		return nil, err
	}
	legacy, err := newGenerator(legacyCatalog)
	if err != nil {
		return nil, err
	}
	objectCatalog, err := parammeta.LoadObjectSelection()
	if err != nil {
		return nil, err
	}
	objectGenerator, err := newGenerator(objectCatalog)
	if err != nil {
		return nil, err
	}
	combinedCatalog, err := parammeta.LoadCombinedObjectSelection()
	if err != nil {
		return nil, err
	}
	combinedGenerator, err := newGenerator(combinedCatalog)
	if err != nil {
		return nil, err
	}
	ddlTextCatalog, err := parammeta.LoadDDLTextFormats()
	if err != nil {
		return nil, err
	}
	ddlTextGenerator, err := newGenerator(ddlTextCatalog)
	if err != nil {
		return nil, err
	}
	current.versions = map[string]*Generator{
		current.metadataVersion:           current,
		legacy.metadataVersion:            legacy,
		objectGenerator.metadataVersion:   objectGenerator,
		combinedGenerator.metadataVersion: combinedGenerator,
		ddlTextGenerator.metadataVersion:  ddlTextGenerator,
	}
	return current, nil
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
	if len(g.versions) != 0 && request.MetadataVersion != g.metadataVersion {
		if versioned, ok := g.versions[request.MetadataVersion]; ok {
			return versioned.Generate(request)
		}
	}
	issues := g.validateRequestIdentity(request)
	provided := make(map[string]FieldInput, len(request.Fields))
	for _, field := range request.Fields {
		definition, known := g.byName[field.Name]
		if !known || !g.participatesInCapability(definition, request.CapabilityVersion) {
			issues = append(issues, Issue{Code: "UNKNOWN_PARAMETER", Field: field.Name})
			continue
		}
		if _, duplicate := provided[field.Name]; duplicate {
			issues = append(issues, Issue{Code: "DUPLICATE_PARAMETER", Field: field.Name})
			continue
		}
		provided[field.Name] = cloneFieldInput(field)
	}

	// 数据格式参数单选：CSV/CUT/POS/SQL/PARQUET/ORC/AVRO 同时启用时失败关闭，防止多格式命令形态模糊。
	// POS 于 2026-08-07 受控实测定版后加入候选（独立 --pos + --ctl-path）；
	// EX-I5 结构化格式（2026-08-07）加入 --par/--orc/--avro 候选。
	format := ""
	for _, candidate := range []struct {
		name   string
		format string
	}{{"--csv", "CSV"}, {"--cut", "CUT"}, {"--pos", "POS"}, {"--sql", "SQL"}, {"--par", "PARQUET"}, {"--orc", "ORC"}, {"--avro", "AVRO"}} {
		field, ok := provided[candidate.name]
		if !ok || field.Value.Kind != ValueBoolean || !field.Value.Boolean {
			continue
		}
		if format != "" {
			issues = append(issues, Issue{Code: "MUTUALLY_EXCLUSIVE_FORMATS", Field: candidate.name})
			continue
		}
		format = candidate.format
	}
	normalized := make([]NormalizedField, 0, len(g.definitions))
	for _, definition := range g.definitions {
		if !g.participatesInCapability(definition, request.CapabilityVersion) {
			continue
		}
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
		value, valueIssues := normalizeValue(definition, field.Value, request.TargetPlatform, request.CapabilityVersion, request.OutputKind)
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
	subset := make([]parammeta.Definition, 0, len(normalized))
	for _, definition := range g.definitions {
		if g.participatesInCapability(definition, request.CapabilityVersion) {
			subset = append(subset, definition)
		}
	}
	for index, field := range normalized {
		if field.State != StateExplicit && field.State != StateDerived {
			continue
		}
		definition := subset[index]
		if definition.EmissionTarget == "SECURITY_FILE" {
			secretSlots = append(secretSlots, SecretSlot{
				SlotID:              secretSlotID,
				Target:              secretSlotTarget,
				SecurityProperty:    definition.SecurityProperty,
				CredentialReference: *field.Value.Secret,
			})
			// 预览用 -p ****** 表达密码逻辑位置；实际 argv 不含 -p，避免无人值守 Agent 进入交互式密码提示。
			display = append(display, displayParameterName(definition), "******")
			continue
		}
		start := len(argv)
		if definition.ValueType != "flag" {
			value := normalizedValueString(field.Value)
			argv = append(argv, renderExecutionParameter(definition, value)...)
			display = append(display, renderExecutionParameter(definition, value)...)
		} else {
			argv = append(argv, displayParameterName(definition))
			display = append(display, displayParameterName(definition))
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
		CapabilityVersion:   request.CapabilityVersion,
		NormalizedFields:    normalized,
		ArgvTemplate:        argv,
		RedactedCommand:     renderCommand(request.TargetPlatform, "obdumper", display),
		SecretSlots:         secretSlots,
		SecretSourceSummary: secretSourceSummary,
		TokenEvidence:       tokenEvidence,
		ConfigFingerprint:   fingerprint,
	}, nil
}

// participatesInCapability 判断定义是否参与请求能力：
// 未声明 capabilityVersions 的定义参与当前目录的全部能力（v5 基线行为），
// 已声明的定义只在列出的能力切片内活动。
func (g *Generator) participatesInCapability(definition parammeta.Definition, capability string) bool {
	if len(definition.CapabilityVersions) == 0 {
		return true
	}
	return contains(definition.CapabilityVersions, capability)
}

// displayParameterName 优先返回已核验的短参数；未配置短参数时保留元数据中的规范长参数。
func displayParameterName(definition parammeta.Definition) string {
	if definition.ShortName != "" {
		return definition.ShortName
	}
	return definition.LongName
}

// renderExecutionParameter 将支持紧凑写法的短参数和值合并为单个 argv 令牌。
// 这是本机 4.3.5 已验证的写法，避免命令预览与实际参数序列出现两套拼装规则。
func renderExecutionParameter(definition parammeta.Definition, value string) []string {
	if definition.ShortName != "" {
		return []string{definition.ShortName + value}
	}
	return []string{definition.LongName, value}
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
	// 固定单能力目录（v5）要求能力版本逐字节一致；
	// 泛化目录（能力版本为空）只要求请求声明非空能力，子集由定义绑定筛选。
	if g.capabilityVersion != "" {
		if request.CapabilityVersion != g.capabilityVersion {
			issues = append(issues, Issue{Code: "CAPABILITY_VERSION_MISMATCH", Field: "capabilityVersion"})
		}
	} else if request.CapabilityVersion == "" {
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

func normalizeValue(definition parammeta.Definition, value Value, platform Platform, capability, outputKind string) (NormalizedValue, []string) {
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
		if code := validateObjectNameList(definition.LongName, value.String, capability); code != "" {
			return NormalizedValue{}, []string{code}
		}
		if definition.ValueType == "path" {
			// EX-I6 对象存储（2026-08-07）：--file-path 在受控对象存储输出下按 URI 校验；
			// 其余 path 参数（本地路径/日志/控制文件/临时分块）仍要求平台绝对路径。
			if definition.LongName == "--file-path" && isStorageOutputKind(outputKind) {
				if !validControlledStorageURI(outputKind, value.String) {
					return NormalizedValue{}, []string{"STORAGE_URI_INVALID"}
				}
			} else if !validAbsolutePath(platform, value.String) {
				return NormalizedValue{}, []string{"OUTPUT_PATH_NOT_ABSOLUTE"}
			}
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

// validateObjectNameList 校验对象类参数的取值：禁止通配符；
// 单表 CSV 冻结能力仍要求单一表名（逗号拒绝），泛化能力允许逗号连接的多名称，
// 每个名称必须非空、不超过 256 字符且不含逗号与通配符。
func validateObjectNameList(longName, value, capability string) string {
	switch longName {
	case "--table", "--view", "--exclude-table":
	default:
		return ""
	}
	if strings.Contains(value, "*") {
		if longName == "--table" {
			return "TABLE_SCOPE_NOT_SINGLE"
		}
		return "PARAMETER_VALUE_INVALID"
	}
	if capability == "export-odp-single-table-csv-v1" || capability == "" {
		if longName == "--table" && strings.Contains(value, ",") {
			return "TABLE_SCOPE_NOT_SINGLE"
		}
		return ""
	}
	if !strings.Contains(value, ",") {
		return ""
	}
	for _, name := range strings.Split(value, ",") {
		if name == "" || len(name) > 256 || strings.Contains(name, "*") {
			return "PARAMETER_VALUE_INVALID"
		}
	}
	return ""
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
	case "FORMAT_IN":
		// 逗号分隔的多格式激活：如 "CSV,CUT" 表示参数在任一列出的格式下活动。
		for _, candidate := range strings.Split(definition.Activation.Value, ",") {
			if candidate == format {
				return true
			}
		}
		return false
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
	return outputpath.IsExportOutputPath(string(platform), value)
}

// isStorageOutputKind 判断输出目标是否为受控对象存储（EX-I6，2026-08-07）。
func isStorageOutputKind(outputKind string) bool {
	return outputKind == "OSS" || outputKind == "S3" || outputKind == "COS" || outputKind == "OBS"
}

// validControlledStorageURI 校验对象存储输出 URI（与控制面归一化同一规则）：
// scheme 与输出类型一致且在白名单内，bucket 非空，路径以 / 开头，
// query 参数只允许 endpoint/region/storage-class；拒绝任何密钥参数，存储凭据走执行槽位。
func validControlledStorageURI(outputKind, uri string) bool {
	if uri == "" || len(uri) > 4096 || strings.ContainsRune(uri, 0) || strings.ContainsAny(uri, "\r\n") {
		return false
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/") {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if !isStorageScheme(scheme) || !strings.EqualFold(scheme, outputKind) {
		return false
	}
	for key := range parsed.Query() {
		if key != "endpoint" && key != "region" && key != "storage-class" {
			return false
		}
	}
	return true
}

// isStorageScheme 判断 scheme 是否在 V1.0 受控对象存储白名单内。
func isStorageScheme(scheme string) bool {
	return scheme == "oss" || scheme == "s3" || scheme == "cos" || scheme == "obs"
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
		Tool:            generator.tool,
		ToolVersion:     generator.toolVersion,
		MetadataVersion: generator.metadataVersion,
		// 指纹以请求能力版本为准：泛化目录下同一字段集在不同能力切片必须产生不同指纹。
		CapabilityVersion:     request.CapabilityVersion,
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
