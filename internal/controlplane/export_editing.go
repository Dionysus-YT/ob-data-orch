package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/exportdomain"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

func rejectEditingExportDraft(w http.ResponseWriter, draft store.ExportDraft) bool {
	if draft.CapabilityVersion != store.ExportEditingCapabilityVersion {
		return false
	}
	writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_INCOMPLETE", "草稿尚未完成配置，请补齐并保存后重试", false)
	return true
}

// 编辑写入只读取授权引用并保存配置，不访问秘密、生成命令或签发 Agent 租约。
func (s *Server) saveEditingExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, request exportDraftWriteRequest, previous *store.ExportDraft, expected int64) {
	if s.dataSource == nil || exportdomain.ValidateEditingConfig(request.Config) != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿字段无效", false)
		return
	}
	source, err := s.dataSource.GetDataSourceSummary(r.Context(), request.DataSourceID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil || source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" || source.ConnectionKind != "ODP" {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "请选择已启用且连接测试成功的数据源", false)
		return
	}
	configJSON, err := editingExportDocument(request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿字段无效", false)
		return
	}
	now := time.Now().UTC()
	if previous == nil {
		id := newOpaqueID()
		if id == "" {
			writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
			return
		}
		result, saveErr := s.drafts.CreateExportDraft(r.Context(), store.ExportDraftCreate{ExportDraft: store.ExportDraft{
			DraftID: id, OwnerSubjectID: principal.ID, DataSourceID: request.DataSourceID, NodeID: request.NodeID,
			ToolVersion: "4.3.5-RELEASE", MetadataVersion: exportMetadataExpertParameters, CapabilityVersion: store.ExportEditingCapabilityVersion,
			ConfigVersion: "v6", ConfigJSON: configJSON, ConfigFingerprint: exportDraftDigest(configJSON), InvalidationJSON: `{"editing":true}`, CreatedAt: now, UpdatedAt: now,
		}, RequestID: requestID(w), IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")), RequestDigest: exportDraftDigest(configJSON)})
		if errors.Is(saveErr, store.ErrIdempotencyConflict) {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
			return
		}
		if saveErr != nil {
			writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "无法保存导出草稿，请核对引用与字段", false)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID(w), "id": result.DraftID, "replayed": result.Replayed})
		return
	}
	revision, err := s.drafts.UpdateDraft(r.Context(), store.DraftUpdate{
		DraftID: previous.DraftID, ExpectedRevision: expected, ConfigJSON: configJSON, ConfigFingerprint: exportDraftDigest(configJSON),
		InvalidationJSON: `{"editing":true}`, UpdatedAt: now, ConfigVersion: "v6", MetadataVersion: exportMetadataExpertParameters,
		CapabilityVersion: store.ExportEditingCapabilityVersion, RebindEditing: previous.CapabilityVersion == store.ExportEditingCapabilityVersion && previous.SourceTaskID == "",
		DataSourceID: request.DataSourceID, NodeID: request.NodeID,
		ActorSubjectID: principal.ID, RequestID: requestID(w),
	})
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "DRAFT_REVISION_CONFLICT", "草稿已发生变化，请刷新后重试", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "无法保存导出草稿，请核对引用与字段", false)
		return
	}
	draft := *previous
	draft.DataSourceID, draft.NodeID, draft.Revision = request.DataSourceID, request.NodeID, revision
	draft.ConfigJSON, draft.ConfigFingerprint, draft.ConfigVersion = configJSON, exportDraftDigest(configJSON), "v6"
	draft.MetadataVersion, draft.CapabilityVersion, draft.InvalidationJSON = exportMetadataExpertParameters, store.ExportEditingCapabilityVersion, `{"editing":true}`
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": draftResponse(draft)})
}

func editingExportDocument(request exportDraftWriteRequest) (string, error) {
	config := request.Config
	names := make([]string, 0, len(config.ObjectScope.Expressions))
	for _, expression := range config.ObjectScope.Expressions {
		names = append(names, expression.Name)
	}
	document := storedDraftConfigV6{ConfigVersion: "v6", DraftState: "EDITING", DataSourceID: request.DataSourceID, NodeID: request.NodeID,
		Database: config.ObjectScope.Database, ScopeKind: config.ObjectScope.ScopeKind, Table: strings.Join(names, ","), ContentKind: config.ContentSelection.ContentKind,
		Format: draftDisplayFormat(config.ContentSelection.ContentKind, config.DataFormat.FormatKind), FilePath: config.OutputConfig.FilePath,
		LogPath: config.OutputConfig.LogPath, SkipCheckDir: config.OutputConfig.SkipCheckDir, Config: config}
	encoded, err := json.Marshal(document)
	return string(encoded), err
}
