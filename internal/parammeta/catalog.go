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
	defaultRevisionResource           = "resources/obdumper-4.3.5-slice-v5.json"
	legacyGeneralizedRevisionResource = "resources/obdumper-4.3.5-slice-v6.json"
	// EX-I7 第二批（2026-08-13 受控实测定版）：v7 在 v6 之上登记时间戳值格式、
	// --partition/--exclude-data-types/--enable-hidden-pk 与 --add-extra-message。
	generalizedRevisionResource      = "resources/obdumper-4.3.5-slice-v7.json"
	currentMetadataVersion           = "obdumper-4.3.5-slice-v5"
	legacyGeneralizedMetadataVersion = "obdumper-4.3.5-slice-v6"
	generalizedMetadataVersion       = "obdumper-4.3.5-slice-v7"
	// maxInheritanceDepth 限制清单继承链深度，避免循环或过长的依赖链。
	maxInheritanceDepth = 4
)

// resourceFiles 只嵌入 resources/ 下已确认可加载的参数元数据。
// 尚未完成事实核验或加载器支持的设计稿放在 drafts/ 目录，不被嵌入也不得被声明为已支持；
// 接入前必须先通过对应切片的验证门禁并发布新的不可变版本。
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
	// CapabilityVersions 声明参数适用的能力切片；为空表示当前目录的全部能力均参与。
	CapabilityVersions []string `json:"capabilityVersions,omitempty"`
	OfficialEvidence   []string `json:"officialEvidence"`
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
	MetadataVersion string `json:"metadataVersion"`
	BaseVersion     string `json:"baseVersion"`
	BaseResource    string `json:"baseResource"`
	BaseSHA256      string `json:"baseSha256"`
	// Inherits 声明先应用的上游修订清单文件名；链式加载从基线逐层叠加。
	Inherits          string `json:"inherits,omitempty"`
	CapabilityVersion string `json:"capabilityVersion,omitempty"`
	// CategoryOrder 与 SourceDocuments 允许泛化清单在继承链末端声明新的分类顺序与证据来源。
	CategoryOrder   []string             `json:"categoryOrder,omitempty"`
	SourceDocuments []string             `json:"sourceDocuments,omitempty"`
	RevisionReason  string               `json:"revisionReason"`
	Overrides       []definitionOverride `json:"overrides"`
	Additions       []Definition         `json:"additions"`
}

type definitionOverride struct {
	DefinitionID     string `json:"definitionId"`
	ShortName        string `json:"shortName,omitempty"`
	EmissionTarget   string `json:"emissionTarget"`
	SecurityProperty string `json:"securityProperty,omitempty"`
	// SupportState 允许泛化修订把已取证参数从 VALIDATION_GATED 提升为 ENABLED；空值表示保持基线状态。
	SupportState string `json:"supportState,omitempty"`
	RequiredWhen *Rule  `json:"requiredWhen,omitempty"`
	// Activation 允许泛化修订扩展参数激活规则（如把 CSV 专属参数扩展为 CSV/CUT 多格式）；空值表示保持基线规则。
	Activation     *Rule    `json:"activation,omitempty"`
	AppendEvidence []string `json:"appendEvidence"`
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

// LoadGeneralized 加载泛化能力目录（v7 链：v1 基线 → v5 → v6 → v7 已取证子集）。
// 该目录 capabilityVersion 为空，由命令生成器按请求能力筛选参数子集。
func LoadGeneralized() (*Catalog, error) {
	return loadFromFS(resourceFiles, generalizedRevisionResource)
}

// LoadLegacyGeneralized 加载冻结的 v6 泛化目录，只用于重放升级前已经持久化的草稿。
// 新草稿不得选择该目录，避免旧定义继续扩散；历史草稿也不得被静默提升到 v7。
func LoadLegacyGeneralized() (*Catalog, error) {
	return loadFromFS(resourceFiles, legacyGeneralizedRevisionResource)
}

func loadFromFS(files fs.FS, revisionResource string) (*Catalog, error) {
	raw, manifest, err := loadRevisionChain(files, revisionResource, 0)
	if err != nil {
		return nil, err
	}
	raw.MetadataVersion = manifest.MetadataVersion
	// 顶层清单的能力版本是权威值：泛化目录用空值表示多能力子集模式。
	raw.CapabilityVersion = manifest.CapabilityVersion
	if len(manifest.CategoryOrder) != 0 {
		raw.CategoryOrder = append([]string(nil), manifest.CategoryOrder...)
	}
	if len(manifest.SourceDocuments) != 0 {
		raw.SourceDocuments = append([]string(nil), manifest.SourceDocuments...)
	}
	return buildCatalog(raw, manifest.BaseVersion, manifest.RevisionReason)
}

// loadRevisionChain 递归应用继承链：先加载上游修订结果，再叠加当前清单的 overrides/additions。
func loadRevisionChain(files fs.FS, revisionResource string, depth int) (resource, revisionManifest, error) {
	if depth > maxInheritanceDepth {
		return resource{}, revisionManifest{}, errors.New("parameter metadata inheritance chain is too deep")
	}
	manifestContent, err := fs.ReadFile(files, revisionResource)
	if err != nil {
		return resource{}, revisionManifest{}, fmt.Errorf("read parameter metadata revision: %w", err)
	}
	manifest, err := decodeManifest(manifestContent)
	if err != nil {
		return resource{}, revisionManifest{}, err
	}
	var raw resource
	if manifest.Inherits != "" {
		inherited, _, err := loadRevisionChain(files, "resources/"+manifest.Inherits, depth+1)
		if err != nil {
			return resource{}, revisionManifest{}, err
		}
		raw = inherited
	} else {
		baseContent, err := fs.ReadFile(files, "resources/"+manifest.BaseResource)
		if err != nil {
			return resource{}, revisionManifest{}, fmt.Errorf("read parameter metadata base: %w", err)
		}
		digest := sha256.Sum256(baseContent)
		if fmt.Sprintf("%x", digest) != manifest.BaseSHA256 {
			return resource{}, revisionManifest{}, errors.New("parameter metadata base checksum mismatch")
		}
		raw, err = decodeResource(baseContent)
		if err != nil {
			return resource{}, revisionManifest{}, err
		}
		if raw.MetadataVersion != manifest.BaseVersion {
			return resource{}, revisionManifest{}, errors.New("parameter metadata base version mismatch")
		}
		for index := range raw.Definitions {
			raw.Definitions[index].EmissionTarget = "ARGV"
		}
	}
	if err := applyOverrides(&raw, manifest.Overrides); err != nil {
		return resource{}, revisionManifest{}, err
	}
	if err := applyAdditions(&raw, manifest.Additions); err != nil {
		return resource{}, revisionManifest{}, err
	}
	return raw, manifest, nil
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
	if manifest.BaseVersion != "obdumper-4.3.5-slice-v1" || manifest.BaseResource != "obdumper-4.3.5-slice-v1.json" || len(manifest.BaseSHA256) != 64 || manifest.RevisionReason == "" {
		return revisionManifest{}, errors.New("parameter metadata revision base is invalid")
	}
	switch manifest.MetadataVersion {
	case currentMetadataVersion:
		// v5 是冻结的单表 CSV 基线：固定 4 overrides + 2 additions，能力版本必填。
		if manifest.Inherits != "" || manifest.CapabilityVersion != "export-odp-single-table-csv-v1" || len(manifest.CategoryOrder) != 0 || len(manifest.SourceDocuments) != 0 {
			return revisionManifest{}, errors.New("parameter metadata revision identity is unsupported")
		}
		if len(manifest.Overrides) != 4 || len(manifest.Additions) != 2 {
			return revisionManifest{}, errors.New("parameter metadata revision content is invalid")
		}
	case legacyGeneralizedMetadataVersion:
		// v6 是 v7 继承链的中间层（EX-I7 第一批 2026-08-11）：继承 v5，additions 41、overrides 10。
		if manifest.Inherits != "obdumper-4.3.5-slice-v5.json" || manifest.CapabilityVersion != "" {
			return revisionManifest{}, errors.New("parameter metadata revision identity is unsupported")
		}
		if len(manifest.CategoryOrder) == 0 || len(manifest.SourceDocuments) == 0 || len(manifest.Overrides) != 10 || len(manifest.Additions) != 41 {
			return revisionManifest{}, errors.New("parameter metadata revision content is invalid")
		}
	case generalizedMetadataVersion:
		// v7 是泛化能力的已取证子集：必须继承 v6，不固定单一能力版本。
		// 第二批（2026-08-13 受控实测）新增时间戳值格式 9 个、--partition/--exclude-data-types/
		// --enable-hidden-pk 与 --add-extra-message，additions 由 41 增至 54；
		// 1 项 override 撤销 v6 对 --table 的错误 shortName（-t 是 --tenant 的短选项）。
		if manifest.Inherits != "obdumper-4.3.5-slice-v6.json" || manifest.CapabilityVersion != "" {
			return revisionManifest{}, errors.New("parameter metadata revision identity is unsupported")
		}
		if len(manifest.CategoryOrder) == 0 || len(manifest.SourceDocuments) == 0 || len(manifest.Overrides) != 1 || len(manifest.Additions) != 13 {
			return revisionManifest{}, errors.New("parameter metadata revision content is invalid")
		}
	default:
		return revisionManifest{}, errors.New("parameter metadata revision identity is unsupported")
	}
	return manifest, nil
}

// applyAdditions 将当前版本确认新增的参数定义附加到不可变基础元数据。
// 仅允许清单声明的受控定义进入目录，避免调用方以自由字段绕过参数校验。
func applyAdditions(raw *resource, additions []Definition) error {
	if raw == nil {
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
		if override.SupportState != "" {
			if !oneOf(override.SupportState, "ENABLED", "VALIDATION_GATED") {
				return fmt.Errorf("parameter metadata override %q has unsupported support state", override.DefinitionID)
			}
			raw.Definitions[index].SupportState = override.SupportState
		}
		if override.RequiredWhen != nil {
			raw.Definitions[index].RequiredWhen = *override.RequiredWhen
		}
		if override.Activation != nil {
			raw.Definitions[index].Activation = *override.Activation
		}
		raw.Definitions[index].OfficialEvidence = append(raw.Definitions[index].OfficialEvidence, override.AppendEvidence...)
	}
	return nil
}

func validateResource(raw resource) error {
	if raw.Tool != "OBDUMPER" || raw.ToolVersion != "4.3.5-RELEASE" {
		return errors.New("parameter metadata identity does not match the confirmed slice")
	}
	if len(raw.SourceDocuments) == 0 {
		return errors.New("parameter metadata has no source documents")
	}
	// 版本感知身份校验：v5 保持冻结基线，v6/v7 分别用于历史重放与现行泛化能力。
	switch raw.MetadataVersion {
	case currentMetadataVersion:
		if raw.CapabilityVersion != "export-odp-single-table-csv-v1" {
			return errors.New("parameter metadata capability version is unsupported")
		}
		expectedCategoryOrder := []string{"CONNECTION", "DATABASE_CONNECTION", "OBJECT_SCOPE", "CONTENT_FORMAT", "FORMAT_SERIALIZATION", "OUTPUT_FILE"}
		if strings.Join(raw.CategoryOrder, "\x00") != strings.Join(expectedCategoryOrder, "\x00") {
			return errors.New("parameter metadata category order does not match the confirmed command order")
		}
		if len(raw.Definitions) != 18 {
			return fmt.Errorf("parameter metadata has %d definitions, want 18", len(raw.Definitions))
		}
	case legacyGeneralizedMetadataVersion, generalizedMetadataVersion:
		// v6/v7 目录不固定单一能力版本，由生成器按请求能力筛选参数子集。
		if raw.CapabilityVersion != "" {
			return errors.New("parameter metadata capability version is unsupported")
		}
		expectedCategoryOrder := []string{"CONNECTION", "DATABASE_CONNECTION", "OBJECT_SCOPE", "CONTENT_FORMAT", "FORMAT_SERIALIZATION", "OUTPUT_FILE", "DATA_FILTER", "PERFORMANCE", "COMPRESSION"}
		if strings.Join(raw.CategoryOrder, "\x00") != strings.Join(expectedCategoryOrder, "\x00") {
			return errors.New("parameter metadata category order does not match the confirmed command order")
		}
		expectedDefinitions := 59
		if raw.MetadataVersion == generalizedMetadataVersion {
			// v7 登记第二批 13 个定义；尚未完成专用验证或预检查的定义仍保持门禁。
			expectedDefinitions = 72
		}
		if len(raw.Definitions) != expectedDefinitions {
			return fmt.Errorf("parameter metadata has %d definitions, want %d", len(raw.Definitions), expectedDefinitions)
		}
	default:
		return errors.New("parameter metadata identity does not match the confirmed slice")
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
		if err := validateRule(definition.LongName, definition.Activation, "ALWAYS", "FORMAT_IS", "FORMAT_IN"); err != nil {
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
	switch raw.MetadataVersion {
	case currentMetadataVersion:
		if err := requireNamesByState(raw.Definitions, "ENABLED", []string{
			"--host", "--port", "--user", "--password", "--database", "--table", "--csv", "--file-path", "--log-path", "--skip-check-dir",
		}); err != nil {
			return err
		}
	case legacyGeneralizedMetadataVersion, generalizedMetadataVersion:
		// v6 已取证子集：EX-I2 六项 + EX-I3 的 CSV 序列化、压缩、文件布局、筛选与资源参数
		// + EX-I4 的 CUT/SQL 数据格式及其专属序列化参数
		// + EX-I4 POS 定版（2026-08-07 实测）的 --pos/--ctl-path 与 CUT 专属 --column-splitter
		// + EX-I5 结构化格式（2026-08-07）的 --par/--orc/--avro
		// + EX-I6 对象存储（2026-08-07）的 --tmp-path
		// + EX-I7 DDL 行为（2026-08-10）的 --drop-object/--retain-schema
		// + EX-I7 文件拆分（2026-08-10）的 --block-size（可读格式与全量 CSV 能力，MB/ROW 已实测）
		// + EX-I7 压缩等级（2026-08-10）的 --compression-level（官方分算法范围）
		// + EX-I7 剩余参数第一批（2026-08-11）的 --compact-schema（表 DDL）/--where（明确表范围）/--snapshot；
		// --weak-read 缺少副本与权限预检查，--retry 缺少 EX-I8 保存点恢复链路，继续保持 VALIDATION_GATED。
		enabled := []string{
			"--host", "--port", "--user", "--password", "--database", "--table", "--csv", "--cut", "--sql", "--pos", "--par", "--orc", "--avro", "--file-path", "--log-path", "--skip-check-dir",
			"--all", "--view", "--ddl", "--exclude-table",
			"--skip-header", "--column-separator", "--column-quote", "--column-quote-mode", "--escape-character", "--line-separator", "--null-string", "--file-encoding",
			"--with-trim", "--trail-delimiter", "--remove-newline", "--column-splitter", "--compress", "--compression-algo", "--compression-level", "--no-nested-dir", "--max-file-size", "--retain-empty-files",
			"--ctl-path", "--tmp-path", "--drop-object", "--retain-schema", "--compact-schema",
			"--query-sql", "--where", "--snapshot", "--include-column-names", "--exclude-column-names", "--exclude-virtual-columns", "--flashback-scn", "--flashback-timestamp",
			"--thread", "--page-size", "--parallel-macro", "--fetch-size", "--mem", "--block-size",
		}
		if raw.MetadataVersion == generalizedMetadataVersion {
			// 第二批仅四项已观察到可验收效果：MySQL DATE/DATETIME 格式、分区和类型排除。
			enabled = append(enabled, "--date-value-format", "--datetime-value-format", "--partition", "--exclude-data-types")
		}
		if err := requireNamesByState(raw.Definitions, "ENABLED", enabled); err != nil {
			return err
		}
	}
	if err := requireNamesByState(raw.Definitions, "VALIDATION_GATED", gatedNamesByVersion(raw.MetadataVersion)); err != nil {
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

// validateRule 校验参数的激活与必填规则。FORMAT_IS 是单格式匹配；
// FORMAT_IN 是逗号分隔的多格式匹配（如 "CSV,CUT"），用于跨格式共享的序列化参数。
// 规则值白名单只含已取证的数据格式（CSV/CUT/POS/SQL）；POS 于 2026-08-07 受控实测定版后加入。
func validateRule(parameter string, rule Rule, allowed ...string) error {
	if !oneOf(rule.Kind, allowed...) {
		return fmt.Errorf("parameter %q has unsupported rule kind %q", parameter, rule.Kind)
	}
	switch rule.Kind {
	case "FORMAT_IS":
		if !oneOf(rule.Value, "CSV", "CUT", "POS", "SQL") {
			return fmt.Errorf("parameter %q has unsupported format rule", parameter)
		}
	case "FORMAT_IN":
		for _, format := range strings.Split(rule.Value, ",") {
			if !oneOf(format, "CSV", "CUT", "SQL", "PARQUET", "ORC", "AVRO") {
				return fmt.Errorf("parameter %q has unsupported format rule", parameter)
			}
		}
	default:
		if rule.Value != "" {
			return fmt.Errorf("parameter %q has an unexpected rule value", parameter)
		}
	}
	return nil
}

// gatedNamesByVersion 返回各版本期望的 VALIDATION_GATED 参数名单。
// v5 保留 CSV 序列化八项为 gated；v6 只保留恢复与弱读门禁；
// v7 对已登记但尚未观察行为或缺少专用预检查的第二批参数继续失败关闭。
func gatedNamesByVersion(metadataVersion string) []string {
	switch metadataVersion {
	case legacyGeneralizedMetadataVersion:
		return []string{"--retry", "--weak-read"}
	case generalizedMetadataVersion:
		return []string{
			"--retry", "--weak-read",
			"--time-value-format", "--timestamp-value-format", "--timestamp-tz-value-format", "--timestamp-ltz-value-format",
			"--nls-date-format", "--nls-timestamp-format", "--nls-timestamp-tz-format",
			"--enable-hidden-pk", "--add-extra-message",
		}
	}
	return []string{
		"--skip-header", "--column-separator", "--column-quote", "--column-quote-mode",
		"--escape-character", "--line-separator", "--null-string", "--file-encoding",
	}
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
	result.CapabilityVersions = append([]string(nil), input.CapabilityVersions...)
	result.OfficialEvidence = append([]string(nil), input.OfficialEvidence...)
	return result
}
