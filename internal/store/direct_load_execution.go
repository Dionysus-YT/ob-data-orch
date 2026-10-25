package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/executionargv"
)

// ConfigureDirectLoadCapabilities 只供组合根设置独立旁路准入；默认关闭，停用立即阻断新的领取与秘密解析。
// 该方法不执行探针或工具，两个开关均不能由普通导入或导出的授权推导。
func (s *Store) ConfigureDirectLoadCapabilities(probeEnabled, executionEnabled bool) {
	s.directLoadProbeEnabled.Store(probeEnabled)
	s.directLoadExecutionEnabled.Store(probeEnabled && executionEnabled)
}

func directLoadExecutionInput(kind, raw, tool, metadata, capability, mode string, argv []string) (*directloadfiles.ExecutionInput, bool) {
	if kind != directload.TaskType || tool != directload.ToolVersion || metadata != directload.MetadataVersion || capability != directload.CapabilityVersion || !oneOf(mode, "MYSQL", "ORACLE") {
		return nil, false
	}
	snapshot, ok := directLoadSnapshot(raw)
	if !ok || !executionargv.MatchesDirectLoad(argv, snapshot.Config) {
		return nil, false
	}
	return &directloadfiles.ExecutionInput{Config: snapshot.Config, Manifest: snapshot.InputManifest, CompatibilityMode: mode, DirectLoadConfigFingerprint: snapshot.DirectLoadConfigFingerprint}, true
}

func directLoadTaskEnvelope(base string, input *directloadfiles.ExecutionInput) (string, error) {
	encoded, err := json.Marshal(struct {
		Base     string                          `json:"base"`
		TaskType string                          `json:"taskType"`
		Input    *directloadfiles.ExecutionInput `json:"input"`
	}{base, directload.TaskType, input})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// validateDirectLoadExecutionTx 在领取与槽位解析时重核独立冻结证明，不能由其他导入检查或旧目录替代。
func validateDirectLoadExecutionTx(ctx context.Context, tx *sql.Tx, taskID string) error {
	var input TaskSubmission
	// 原任务复验必须保留自身标识，才能与同一预检的第二次冻结请求区分。
	input.TaskID = taskID
	if tx.QueryRowContext(ctx, `SELECT task_type,creator_subject_id,data_source_id,node_id,precheck_id,credential_id,credential_revision,config_fingerprint,tool_version,metadata_version,capability_version,snapshot_version,snapshot_json,planned_argv_json,COALESCE(storage_credential_id,''),COALESCE(storage_credential_revision,0),COALESCE(parent_task_id,''),COALESCE(derivation_kind,'') FROM tasks WHERE task_id=?`, taskID).Scan(&input.TaskType, &input.CreatorSubjectID, &input.DataSourceID, &input.NodeID, &input.PrecheckID, &input.CredentialID, &input.CredentialRevision, &input.ConfigFingerprint, &input.ToolVersion, &input.MetadataVersion, &input.CapabilityVersion, &input.SnapshotVersion, &input.SnapshotJSON, &input.PlannedArgvJSON, &input.StorageCredentialID, &input.StorageCredentialRevision, &input.ParentTaskID, &input.DerivationKind) != nil || !validateDirectLoadSubmission(input) {
		return ErrClaimIneligible
	}
	precheck, err := readStoredPrecheck(ctx, tx, input.PrecheckID)
	if err != nil || precheck.Status != "SUCCEEDED" || precheck.IntegrityStatus != "COMPLETE" || validateCurrentPrecheckBinding(ctx, tx, precheck.Binding.BindingAgentID, precheck.Binding) != nil || validateSubmissionPrecheckProof(ctx, tx, precheck.Binding) != nil || validateTaskPrecheckKind(ctx, tx, input, precheck.Binding) != nil {
		return ErrClaimIneligible
	}
	return nil
}
