package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"ob-data-orch/internal/importdomain"
)

// NormalImportReusableConfig 只提取当前受支持快照中的独立配置，不复制名称、输入证据或运行事实。
func NormalImportReusableConfig(source TaskDerivationSource) (importdomain.Config, error) {
	if source.TaskType != importdomain.TaskType || source.SnapshotVersion != importdomain.ConfigVersion || source.MetadataVersion != importdomain.MetadataVersion || source.CapabilityVersion != importdomain.CapabilityVersion || source.ToolVersion != importdomain.ToolVersion {
		return importdomain.Config{}, errors.New("普通导入快照版本不支持复用")
	}
	snapshot, ok := normalImportSnapshot(source.SnapshotJSON)
	if !ok || snapshot.Config.DataSourceID != source.DataSourceID || snapshot.Config.NodeID != source.NodeID {
		return importdomain.Config{}, errors.New("普通导入快照不可用")
	}
	config := snapshot.Config
	config.TaskName = ""
	return config, nil
}

// NormalImportTemplateConfig 默认使用时重选数据源与节点，保留显式路径及活动参数。
func NormalImportTemplateConfig(source TaskDerivationSource) (string, error) {
	config, err := NormalImportReusableConfig(source)
	if err != nil {
		return "", err
	}
	config.DataSourceID, config.NodeID = "", ""
	raw, err := json.Marshal(config)
	return string(raw), err
}

// validateNormalImportTemplateSource 在模板写事务内复验授权、可信成功与精确配置，防止调用方混入运行事实。
func validateNormalImportTemplateSource(ctx context.Context, tx *sql.Tx, input ExportConfigTemplateCreate) error {
	source, err := readTaskDerivationSourceTx(ctx, tx, input.SourceTaskID, input.OwnerSubjectID)
	if err != nil {
		return err
	}
	raw, err := NormalImportTemplateConfig(source)
	digest := sha256.Sum256([]byte(raw))
	if err != nil || source.State != "SUCCEEDED" || source.ReconciliationRequired || input.CapabilityVersion != importdomain.CapabilityVersion || input.ConfigJSON != raw || input.ConfigFingerprint != hex.EncodeToString(digest[:]) {
		return ErrPrecheckInvalid
	}
	return nil
}
