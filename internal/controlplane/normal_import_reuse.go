package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/store"
)

// prepareNormalImportReuse 根据当前引用校验复用配置；绑定变化不能静默清除原活动参数。
func (s *Server) prepareNormalImportReuse(ctx context.Context, p identity.Principal, config importdomain.Config, sourceID, nodeID string) (importdomain.Config, string, error) {
	if s.normalImports == nil || s.dataSource == nil || s.nodes == nil || !s.authorizedNormalImport(ctx, p, sourceID, nodeID) {
		return importdomain.Config{}, "", store.ErrDataSourceNotFound
	}
	source, err := s.dataSource.GetDataSourceSummary(ctx, sourceID)
	if err != nil || source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" || source.ConnectionKind != "ODP" {
		return importdomain.Config{}, "", store.ErrDataSourceNotFound
	}
	node, err := s.nodes.GetExecutionNodeFact(ctx, nodeID)
	if err != nil {
		return importdomain.Config{}, "", store.ErrDataSourceNotFound
	}
	config, err = importdomain.BindReusableConfig(config, sourceID, nodeID, source.CompatibilityMode, string(node.Platform))
	if err != nil {
		return importdomain.Config{}, "", err
	}
	raw, _ := json.Marshal(config)
	fingerprint := normalImportDigest(string(raw))
	if _, issues := importdomain.Normalize(config, source.CompatibilityMode, string(node.Platform), true); len(issues) == 0 {
		preview, _, _, err := s.generateNormalImport(ctx, config)
		if err != nil {
			return importdomain.Config{}, "", importdomain.ErrReuseIncompatible
		}
		fingerprint = preview.ConfigFingerprint
	}
	return config, fingerprint, nil
}

func (s *Server) rebuildNormalImportDraft(w http.ResponseWriter, r *http.Request, p identity.Principal, source store.TaskDerivationSource, derivation, key string) {
	if s.normalImports == nil {
		writeError(w, 503, "NORMAL_IMPORT_UNAVAILABLE", "普通导入草稿暂时不可用", false)
		return
	}
	if !importdomain.CanDeriveTaskDraft(source.State, source.ReconciliationRequired, derivation) {
		writeError(w, 422, "TASK_DERIVATION_UNAVAILABLE", "当前任务状态不允许此派生操作", false)
		return
	}
	config, err := store.NormalImportReusableConfig(source)
	if err != nil {
		writeError(w, 422, "TASK_DERIVATION_UNSUPPORTED", "该普通导入快照不支持派生草稿", false)
		return
	}
	config, fingerprint, err := s.prepareNormalImportReuse(r.Context(), p, config, source.DataSourceID, source.NodeID)
	if s.writeNormalImportReuseError(w, r, err) {
		return
	}
	raw, _ := json.Marshal(config)
	created, ok := s.persistNormalImportReuse(w, r, p, config, string(raw), fingerprint, key, taskDerivationDigest(source.TaskID, derivation), source.TaskID, derivation)
	if !ok {
		return
	}
	status := http.StatusCreated
	if created.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(w), "draftId": created.ID, "sourceTaskId": source.TaskID, "derivation": derivation, "taskType": importdomain.TaskType, "replayed": created.Replayed})
}

func (s *Server) persistNormalImportReuse(w http.ResponseWriter, r *http.Request, p identity.Principal, config importdomain.Config, raw, fingerprint, key, digest, sourceID, derivation string) (store.NormalImportDraft, bool) {
	id := newOpaqueID()
	if id == "" {
		writeError(w, 503, "IDENTIFIER_UNAVAILABLE", "标识暂时不可用", true)
		return store.NormalImportDraft{}, false
	}
	item, err := s.normalImports.SaveNormalImportDraft(r.Context(), store.NormalImportDraftWrite{NormalImportDraft: store.NormalImportDraft{ID: id, OwnerSubjectID: p.ID, DataSourceID: config.DataSourceID, NodeID: config.NodeID, ConfigJSON: raw, ConfigFingerprint: fingerprint, SourceTaskID: sourceID, SourceDerivation: derivation, UpdatedAt: time.Now().UTC()}, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: digest})
	return item, !s.writeNormalImportReuseError(w, r, err)
}

func (s *Server) writeNormalImportReuseError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrDataSourceNotFound), errors.Is(err, store.ErrExecutionNodeNotFound):
		notFound(w, r)
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
	case errors.Is(err, importdomain.ErrReuseIncompatible), errors.Is(err, store.ErrPrecheckInvalid):
		writeError(w, 422, "NORMAL_IMPORT_REUSE_REJECTED", "来源配置、类型或当前绑定不能用于新草稿，请核对后重试", false)
	default:
		writeError(w, 503, "NORMAL_IMPORT_REUSE_UNAVAILABLE", "普通导入复用暂时不可用，请稍后重试", true)
	}
	return true
}

func (s *Server) saveNormalImportTemplate(w http.ResponseWriter, r *http.Request, p identity.Principal, source store.TaskDerivationSource, name, key string) {
	raw, err := store.NormalImportTemplateConfig(source)
	if err != nil {
		writeError(w, 422, "TEMPLATE_SOURCE_UNSUPPORTED", "该普通导入快照不支持保存模板", false)
		return
	}
	id := newOpaqueID()
	if id == "" {
		writeError(w, 503, "IDENTIFIER_UNAVAILABLE", "标识暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	created, err := s.templates.CreateExportConfigTemplate(r.Context(), store.ExportConfigTemplateCreate{ExportConfigTemplate: store.ExportConfigTemplate{TaskType: importdomain.TaskType, TemplateID: id, OwnerSubjectID: p.ID, DisplayName: name, CapabilityVersion: importdomain.CapabilityVersion, ConfigJSON: raw, ConfigFingerprint: normalImportDigest(raw), SourceTaskID: source.TaskID, CreatedAt: now, UpdatedAt: now}, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: normalImportDigest(source.TaskID + "/SAVE_TEMPLATE/" + name)})
	if s.writeNormalImportReuseError(w, r, err) {
		return
	}
	status := http.StatusCreated
	if created.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(w), "id": created.TemplateID, "sourceTaskId": source.TaskID, "replayed": created.Replayed})
}

func (s *Server) createNormalImportDraftFromTemplate(w http.ResponseWriter, r *http.Request, p identity.Principal, template store.ExportConfigTemplate, request templateCreateDraftRequest, key string) {
	if template.CapabilityVersion != importdomain.CapabilityVersion {
		writeError(w, 422, "TEMPLATE_SOURCE_UNSUPPORTED", "模板能力版本不支持普通导入", false)
		return
	}
	config, err := importdomain.Decode(template.ConfigJSON)
	if err != nil || config.DataSourceID != "" || config.NodeID != "" || config.TaskName != "" {
		writeError(w, 422, "TEMPLATE_CONFIG_UNAVAILABLE", "模板配置不符合普通导入边界", false)
		return
	}
	config, fingerprint, err := s.prepareNormalImportReuse(r.Context(), p, config, request.DataSourceID, request.NodeID)
	if s.writeNormalImportReuseError(w, r, err) {
		return
	}
	raw, _ := json.Marshal(config)
	digest := normalImportDigest(template.TemplateID + "/CREATE_DRAFT/" + request.DataSourceID + "/" + request.NodeID)
	created, ok := s.persistNormalImportReuse(w, r, p, config, string(raw), fingerprint, key, digest, "", "")
	if !ok {
		return
	}
	status := http.StatusCreated
	if created.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(w), "draftId": created.ID, "templateId": template.TemplateID, "taskType": importdomain.TaskType, "replayed": created.Replayed})
}
