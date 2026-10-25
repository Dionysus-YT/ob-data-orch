package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
)

type executionTaskReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// executionTaskEnvelopeDigest 将普通导入配置与完整输入清单纳入租约摘要。
// 历史导出摘要保持不变；任何跨类型、损坏快照或不匹配参数均不能解析秘密或写入日志。
func executionTaskEnvelopeDigest(ctx context.Context, reader executionTaskReader, taskID, nodeID, fingerprint, tool, metadata, capability string, argv []string) (string, error) {
	base, err := executionEnvelopeDigest(taskID, nodeID, fingerprint, tool, metadata, capability, argv)
	if err != nil {
		return "", err
	}
	var kind, raw, mode string
	if err := reader.QueryRowContext(ctx, `SELECT t.task_type,t.snapshot_json,COALESCE(ds.compatibility_mode,'') FROM tasks t LEFT JOIN data_sources ds ON ds.data_source_id=t.data_source_id WHERE t.task_id=?`, taskID).Scan(&kind, &raw, &mode); err != nil {
		return "", err
	}
	if kind == "OBDUMPER_EXPORT" {
		return base, nil
	}
	if kind == directload.TaskType {
		input, ok := directLoadExecutionInput(kind, raw, tool, metadata, capability, mode, argv)
		if !ok {
			return "", ErrClaimIneligible
		}
		return directLoadTaskEnvelope(base, input)
	}
	input, ok := normalImportExecutionInput(kind, raw, tool, metadata, capability, argv)
	if !ok {
		return "", ErrClaimIneligible
	}
	encoded, err := json.Marshal(struct {
		Base     string                      `json:"base"`
		TaskType string                      `json:"taskType"`
		Input    *importfiles.ExecutionInput `json:"input"`
	}{base, kind, input})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func normalImportExecutionInput(kind, raw, tool, metadata, capability string, argv []string) (*importfiles.ExecutionInput, bool) {
	if kind != importdomain.TaskType || tool != importdomain.ToolVersion || metadata != importdomain.MetadataVersion || capability != importdomain.CapabilityVersion || !executionargv.ValidNormalImport(argv) {
		return nil, false
	}
	snapshot, ok := normalImportSnapshot(raw)
	if !ok {
		return nil, false
	}
	return &importfiles.ExecutionInput{Config: snapshot.Config, Manifest: snapshot.InputManifest}, true
}
