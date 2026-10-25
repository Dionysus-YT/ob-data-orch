package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/importfiles"
)

// DirectLoadPrecheckProof 冻结独立完整检查的身份、修订、指纹和有序安全结果。
// 合成冻结证明不能替代独立旁路真实验证准入。
type DirectLoadPrecheckProof struct {
	ID                string                `json:"id"`
	DraftRevision     int64                 `json:"draftRevision"`
	ConfigFingerprint string                `json:"configFingerprint"`
	Results           []PrecheckCheckResult `json:"results"`
}

// DirectLoadSnapshot 与普通导入快照完全独立，不接收恢复、DDL、SQL 或客户端运行事实。
type DirectLoadSnapshot struct {
	ConfigVersion string                  `json:"configVersion"`
	TaskType      string                  `json:"taskType"`
	TaskName      string                  `json:"taskName"`
	Config        directload.Config       `json:"config"`
	InputManifest importfiles.Manifest    `json:"inputManifest"`
	PrecheckProof DirectLoadPrecheckProof `json:"precheckProof"`
	// DirectLoadConfigFingerprint 只绑定 session.config 的三键安全投影，不含原文件或秘密。
	DirectLoadConfigFingerprint string `json:"directLoadConfigFingerprint"`
}

func directLoadSnapshot(raw string) (DirectLoadSnapshot, bool) {
	var snapshot DirectLoadSnapshot
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 2<<20 || !strictDirectLoadSnapshotJSON(raw) || d.Decode(&snapshot) != nil || d.Decode(new(any)) != io.EOF || snapshot.ConfigVersion != directload.ConfigVersion || snapshot.TaskType != directload.TaskType || strings.TrimSpace(snapshot.TaskName) == "" || len([]rune(snapshot.TaskName)) > 200 || strings.ContainsFunc(snapshot.TaskName, unicode.IsControl) || !directloadfiles.Valid(snapshot.InputManifest, snapshot.Config) || snapshot.PrecheckProof.ID == "" || snapshot.PrecheckProof.DraftRevision < 1 || !isSHA256(snapshot.PrecheckProof.ConfigFingerprint) || !isSHA256(snapshot.DirectLoadConfigFingerprint) || !matchesDirectLoadShape(snapshot.PrecheckProof.Results, "DIRECT_LOAD_PREFLIGHT") {
		return DirectLoadSnapshot{}, false
	}
	for _, result := range snapshot.PrecheckProof.Results {
		if result.Status != "PASSED" {
			return DirectLoadSnapshot{}, false
		}
	}
	return snapshot, true
}

func validateDirectLoadSubmission(input TaskSubmission) bool {
	if input.SnapshotVersion != directload.ConfigVersion || input.ToolVersion != directload.ToolVersion || input.MetadataVersion != directload.MetadataVersion || input.CapabilityVersion != directload.CapabilityVersion || input.StorageCredentialID != "" || input.StorageCredentialRevision != 0 || (input.ParentTaskID == "") != (input.DerivationKind == "") || input.ParentTaskID != "" && !directload.CanDeriveTaskDraft("FAILED", false, input.DerivationKind) {
		return false
	}
	snapshot, ok := directLoadSnapshot(input.SnapshotJSON)
	if !ok || snapshot.Config.DataSourceID != input.DataSourceID || snapshot.Config.NodeID != input.NodeID || snapshot.PrecheckProof.ID != input.PrecheckID || snapshot.PrecheckProof.ConfigFingerprint != input.ConfigFingerprint {
		return false
	}
	var argv []string
	return json.Unmarshal([]byte(input.PlannedArgvJSON), &argv) == nil && directLoadArgumentsMatch(argv, snapshot.Config)
}

// directLoadArgumentsMatch 在平台冻结时复核受控旁路目录与全部活动字段，不能由调用方注入普通导入能力。
func directLoadArgumentsMatch(argv []string, c directload.Config) bool {
	return executionargv.MatchesDirectLoad(argv, c)
}

func validateDirectLoadTaskPrecheck(ctx context.Context, tx *sql.Tx, input TaskSubmission, binding PrecheckBinding, kind, draftKind, raw, manifest, owner, sourceID, derivation string) error {
	if kind != "DIRECT_LOAD_PREFLIGHT" || draftKind != directload.TaskType || owner != input.CreatorSubjectID || input.ParentTaskID != sourceID || input.DerivationKind != derivation {
		return ErrPrecheckInvalid
	}
	// 同一预检查只能冻结一个旁路任务；串行写事务同时覆盖换幂等键、并发窗口和非幂等内部入口。
	// 执行领取复核同一任务时仍可使用原证明，不能把已消费误判为原任务失效。
	var used bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks WHERE precheck_id=? AND task_id<>?)`, input.PrecheckID, input.TaskID).Scan(&used) != nil || used {
		return ErrPrecheckInvalid
	}
	snapshot, ok := directLoadSnapshot(input.SnapshotJSON)
	if !ok {
		return ErrPrecheckInvalid
	}
	config, err := directload.Decode(raw)
	if err != nil {
		return ErrPrecheckInvalid
	}
	if sourceID != "" {
		source, err := readTaskDerivationSourceTx(ctx, tx, sourceID, owner)
		if err != nil || !directload.CanDeriveTaskDraft(source.State, source.ReconciliationRequired, derivation) {
			return ErrPrecheckInvalid
		}
		original, err := DirectLoadReusableConfig(source)
		before, _ := json.Marshal(original)
		after, _ := json.Marshal(config)
		if err != nil || derivation == "RERUN_FROM_SCRATCH" && string(before) != string(after) {
			return ErrPrecheckInvalid
		}
	}
	want, _ := json.Marshal(config)
	got, _ := json.Marshal(snapshot.Config)
	var storedManifest importfiles.Manifest
	if json.Unmarshal([]byte(manifest), &storedManifest) != nil {
		return ErrPrecheckInvalid
	}
	wantManifest, _ := json.Marshal(storedManifest)
	gotManifest, _ := json.Marshal(snapshot.InputManifest)
	var resultsJSON, configProjectionFingerprint string
	if tx.QueryRowContext(ctx, `SELECT COALESCE(result_json,''),COALESCE(direct_load_config_fingerprint,'') FROM precheck_runs WHERE precheck_id=?`, input.PrecheckID).Scan(&resultsJSON, &configProjectionFingerprint) != nil {
		return ErrPrecheckInvalid
	}
	results := decodePrecheckResults(resultsJSON)
	expected, _ := json.Marshal(results)
	actual, _ := json.Marshal(snapshot.PrecheckProof.Results)
	if string(want) != string(got) || string(wantManifest) != string(gotManifest) || string(expected) != string(actual) || snapshot.PrecheckProof.DraftRevision != binding.DraftRevision || snapshot.PrecheckProof.ConfigFingerprint != binding.ConfigFingerprint || snapshot.DirectLoadConfigFingerprint != configProjectionFingerprint {
		return ErrPrecheckInvalid
	}
	return nil
}
