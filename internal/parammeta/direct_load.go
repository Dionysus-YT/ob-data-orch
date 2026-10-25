package parammeta

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
)

// directLoadFiles 与普通导入、导出分别冻结，避免共享 help 被误解释成共享能力。
//
//go:embed direct/*.json
var directLoadFiles embed.FS

// LoadDirectLoad 加载完整 103 参数旁路处置目录；ENABLED 只说明参数证据，不授予真实执行资格。
func LoadDirectLoad() (*Catalog, error) {
	raw, err := directLoadFiles.ReadFile("direct/obloader-4.3.5-direct-v1.json")
	if err != nil {
		return nil, err
	}
	if digest := sha256.Sum256(raw); fmt.Sprintf("%x", digest) != "0cdc8c49f6058d5c6c1519aa6eb2a82c0897b7da41c19d0b0337c372b12fa6fa" {
		return nil, errors.New("旁路参数目录摘要不匹配")
	}
	var source resource
	if json.Unmarshal(raw, &source) != nil || source.Tool != "OBLOADER" || source.ToolVersion != "4.3.5-RELEASE" || source.MetadataVersion != "obloader-4.3.5-direct-v1" || source.CapabilityVersion != "direct-load-local-v1" || len(source.Definitions) != 103 {
		return nil, errors.New("旁路参数目录无效")
	}
	byName := make(map[string]int, len(source.Definitions))
	ids := map[string]bool{}
	for index, definition := range source.Definitions {
		if definition.LongName == "" || definition.DefinitionID == "" || definition.SourcePolicy == "" || ids[definition.DefinitionID] || !oneOf(definition.SupportState, "ENABLED", "VALIDATION_GATED", "UNSUPPORTED") {
			return nil, errors.New("旁路参数定义无效")
		}
		if _, duplicate := byName[definition.LongName]; duplicate {
			return nil, errors.New("旁路参数重复")
		}
		ids[definition.DefinitionID] = true
		byName[definition.LongName] = index
	}
	return &Catalog{metadataVersion: source.MetadataVersion, tool: source.Tool, toolVersion: source.ToolVersion, capabilityVersion: source.CapabilityVersion, categoryOrder: source.CategoryOrder, definitions: source.Definitions, byName: byName}, nil
}
