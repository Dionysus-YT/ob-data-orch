package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// prepareDirectLoadReuse 用当前引用原样复验旁路配置，不复制旧文件证明或风险确认。
func (s *Server) prepareDirectLoadReuse(ctx context.Context, p identity.Principal, c directload.Config, sourceID, nodeID string) (directload.Config, string, error) {
	if s.directLoads == nil || s.dataSource == nil || s.nodes == nil || !s.authorizedDirectLoad(ctx, p, sourceID, nodeID) {
		return directload.Config{}, "", store.ErrDataSourceNotFound
	}
	source, err := s.dataSource.GetDataSourceSummary(ctx, sourceID)
	if err != nil || !store.DirectLoadSourceMatches(c.Scenario, source) {
		return directload.Config{}, "", store.ErrDataSourceNotFound
	}
	node, err := s.nodes.GetExecutionNodeFact(ctx, nodeID)
	if err != nil {
		return directload.Config{}, "", store.ErrExecutionNodeNotFound
	}
	c, err = directload.BindReusableConfig(c, sourceID, nodeID, source.CompatibilityMode, string(node.Platform))
	if err != nil {
		return directload.Config{}, "", err
	}
	raw, _ := json.Marshal(c)
	fingerprint := directLoadDigest(string(raw))
	if _, issues := directload.Normalize(c, source.CompatibilityMode, string(node.Platform), true); len(issues) == 0 {
		preview, _, _, err := s.generateDirectLoad(ctx, c)
		if err != nil {
			return directload.Config{}, "", directload.ErrReuseIncompatible
		}
		fingerprint = preview.ConfigFingerprint
	}
	return c, fingerprint, nil
}

func (s *Server) rebuildDirectLoadDraft(w http.ResponseWriter, r *http.Request, p identity.Principal, source store.TaskDerivationSource, derivation, key string) {
	if s.directLoads == nil {
		writeError(w, 503, "DIRECT_LOAD_UNAVAILABLE", "旁路导入草稿暂时不可用", false)
		return
	}
	if !directload.CanDeriveTaskDraft(source.State, source.ReconciliationRequired, derivation) {
		writeError(w, 422, "TASK_DERIVATION_UNAVAILABLE", "当前任务状态不允许此派生操作", false)
		return
	}
	c, err := store.DirectLoadReusableConfig(source)
	if err != nil {
		writeError(w, 422, "TASK_DERIVATION_UNSUPPORTED", "该旁路导入冻结证明不支持派生草稿", false)
		return
	}
	c, fingerprint, err := s.prepareDirectLoadReuse(r.Context(), p, c, source.DataSourceID, source.NodeID)
	if s.writeDirectLoadReuseError(w, r, err) {
		return
	}
	id := newOpaqueID()
	if id == "" {
		writeError(w, 503, "IDENTIFIER_UNAVAILABLE", "标识暂时不可用", true)
		return
	}
	raw, _ := json.Marshal(c)
	created, err := s.directLoads.SaveDirectLoadDraft(r.Context(), store.DirectLoadDraftWrite{DirectLoadDraft: store.DirectLoadDraft{ID: id, OwnerSubjectID: p.ID, DataSourceID: c.DataSourceID, NodeID: c.NodeID, ConfigJSON: string(raw), ConfigFingerprint: fingerprint, SourceTaskID: source.TaskID, SourceDerivation: derivation, UpdatedAt: time.Now().UTC()}, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: taskDerivationDigest(source.TaskID, derivation)})
	if s.writeDirectLoadReuseError(w, r, err) {
		return
	}
	status := http.StatusCreated
	if created.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(w), "draftId": created.ID, "sourceTaskId": source.TaskID, "derivation": derivation, "taskType": directload.TaskType, "replayed": created.Replayed})
}

func (s *Server) writeDirectLoadReuseError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrDataSourceNotFound), errors.Is(err, store.ErrExecutionNodeNotFound):
		notFound(w, r)
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
	case errors.Is(err, directload.ErrReuseIncompatible), errors.Is(err, store.ErrPrecheckInvalid):
		writeError(w, 422, "DIRECT_LOAD_REUSE_REJECTED", "来源配置、版本或当前绑定不能用于旁路草稿", false)
	default:
		writeError(w, 503, "DIRECT_LOAD_REUSE_UNAVAILABLE", "旁路配置复用暂时不可用", true)
	}
	return true
}
