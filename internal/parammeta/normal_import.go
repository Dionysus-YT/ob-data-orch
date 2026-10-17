package parammeta

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
)

// normalImportFiles 与冻结导出清单分开加载，避免把导出继承链或能力状态带入普通导入。
//
//go:embed normal/*.json
var normalImportFiles embed.FS

// LoadNormalImport 加载普通导入首版独立目录；真实执行资格仍由运行门禁与预检查决定。
func LoadNormalImport() (*Catalog, error) {
	raw, err := normalImportFiles.ReadFile("normal/obloader-4.3.5-normal-v1.json")
	if err != nil {
		return nil, err
	}
	// 已发布目录按原始字节冻结；新增或调整参数必须登记新版本，不能静默改写旧快照语义。
	if digest := sha256.Sum256(raw); fmt.Sprintf("%x", digest) != "e77c3d75add8891c7913ce6a686ea3f73128a477afe4060cc2eb3b247ff2bc9b" {
		return nil, errors.New("普通导入参数目录摘要不匹配")
	}
	var source resource
	if json.Unmarshal(raw, &source) != nil || source.Tool != "OBLOADER" || source.ToolVersion != "4.3.5-RELEASE" || source.MetadataVersion != "obloader-4.3.5-normal-v1" || source.CapabilityVersion != "import-odp-local-v1" {
		return nil, errors.New("普通导入参数目录无效")
	}
	byName := make(map[string]int)
	for index, definition := range source.Definitions {
		if definition.LongName == "" || definition.DefinitionID == "" || definition.SourcePolicy == "" {
			return nil, errors.New("普通导入参数定义无效")
		}
		if _, duplicate := byName[definition.LongName]; duplicate {
			return nil, errors.New("普通导入参数重复")
		}
		byName[definition.LongName] = index
	}
	return &Catalog{metadataVersion: source.MetadataVersion, tool: source.Tool, toolVersion: source.ToolVersion, capabilityVersion: source.CapabilityVersion, categoryOrder: source.CategoryOrder, definitions: source.Definitions, byName: byName}, nil
}
