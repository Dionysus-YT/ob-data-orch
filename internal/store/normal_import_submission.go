package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/importdomain"
)

func taskType(input TaskSubmission) string {
	if input.TaskType == "" {
		return "OBDUMPER_EXPORT"
	}
	return input.TaskType
}

func normalImportSnapshot(raw string) (NormalImportSnapshot, bool) {
	var snapshot NormalImportSnapshot
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&snapshot) != nil || decoder.Decode(new(any)) != io.EOF || snapshot.ConfigVersion != importdomain.ConfigVersion || snapshot.TaskType != importdomain.TaskType || len([]rune(snapshot.TaskName)) > 200 || strings.ContainsFunc(snapshot.TaskName, unicode.IsControl) || !snapshot.InputManifest.Valid(snapshot.Config) {
		return NormalImportSnapshot{}, false
	}
	return snapshot, true
}

func validateNormalImportSubmission(input TaskSubmission) bool {
	if input.SnapshotVersion != importdomain.ConfigVersion || input.MetadataVersion != importdomain.MetadataVersion || input.CapabilityVersion != importdomain.CapabilityVersion || input.ToolVersion != importdomain.ToolVersion || input.StorageCredentialID != "" || input.StorageCredentialRevision != 0 {
		return false
	}
	snapshot, ok := normalImportSnapshot(input.SnapshotJSON)
	if !ok || snapshot.Config.DataSourceID != input.DataSourceID || snapshot.Config.NodeID != input.NodeID {
		return false
	}
	var argv []string
	return json.Unmarshal([]byte(input.PlannedArgvJSON), &argv) == nil && executionargv.ValidNormalImport(argv)
}

// validateTaskPrecheckKind 阻止跨类型提交及以文件发现替代完整预检查，并核对持久化清单。
func validateTaskPrecheckKind(ctx context.Context, tx *sql.Tx, input TaskSubmission, binding PrecheckBinding) error {
	var kind, draftKind, raw, manifest, owner string
	if err := tx.QueryRowContext(ctx, `SELECT p.check_kind,d.draft_kind,d.config_json,COALESCE(p.input_manifest_json,''),d.owner_subject_id FROM precheck_runs p JOIN export_drafts d ON d.draft_id=p.draft_id WHERE p.precheck_id=?`, input.PrecheckID).Scan(&kind, &draftKind, &raw, &manifest, &owner); err != nil {
		return ErrPrecheckInvalid
	}
	if taskType(input) == "OBDUMPER_EXPORT" {
		if kind != "EXPORT_PREFLIGHT" || draftKind != "OBDUMPER_EXPORT" {
			return ErrPrecheckInvalid
		}
		return nil
	}
	if kind != "NORMAL_IMPORT_PREFLIGHT" || draftKind != importdomain.TaskType || owner != input.CreatorSubjectID {
		return ErrPrecheckInvalid
	}
	snapshot, ok := normalImportSnapshot(input.SnapshotJSON)
	if !ok {
		return ErrPrecheckInvalid
	}
	config, err := importdomain.Decode(raw)
	if err != nil {
		return ErrPrecheckInvalid
	}
	wantConfig, _ := json.Marshal(config)
	gotConfig, _ := json.Marshal(snapshot.Config)
	wantManifest, _ := json.Marshal(snapshot.InputManifest)
	var storedManifest any
	if json.Unmarshal([]byte(manifest), &storedManifest) != nil {
		return ErrPrecheckInvalid
	}
	canonicalManifest, _ := json.Marshal(storedManifest)
	var proposedManifest any
	_ = json.Unmarshal(wantManifest, &proposedManifest)
	canonicalProposed, _ := json.Marshal(proposedManifest)
	if string(wantConfig) != string(gotConfig) || string(canonicalManifest) != string(canonicalProposed) || binding.ConfigFingerprint != input.ConfigFingerprint {
		return ErrPrecheckInvalid
	}
	return nil
}
