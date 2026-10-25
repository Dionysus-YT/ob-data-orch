package store

import (
	"errors"

	"ob-data-orch/internal/directload"
)

// DirectLoadReusableConfig 只提取独立旁路冻结配置，并核对原证明身份，不复制执行事实。
func DirectLoadReusableConfig(source TaskDerivationSource) (directload.Config, error) {
	if source.TaskType != directload.TaskType || source.SnapshotVersion != directload.ConfigVersion || source.ToolVersion != directload.ToolVersion || source.MetadataVersion != directload.MetadataVersion || source.CapabilityVersion != directload.CapabilityVersion {
		return directload.Config{}, errors.New("旁路导入快照版本不支持复用")
	}
	snapshot, ok := directLoadSnapshot(source.SnapshotJSON)
	if !ok || snapshot.Config.DataSourceID != source.DataSourceID || snapshot.Config.NodeID != source.NodeID || snapshot.PrecheckProof.ID != source.PrecheckID || snapshot.PrecheckProof.ConfigFingerprint != source.ConfigFingerprint {
		return directload.Config{}, errors.New("旁路导入冻结证明不可用")
	}
	return snapshot.Config, nil
}
