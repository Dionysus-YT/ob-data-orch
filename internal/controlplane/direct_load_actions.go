package controlplane

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/precheckcontract"
	"ob-data-orch/internal/store"
)

func validDirectLoadProjectionFingerprint(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func (s *Server) createDirectLoadCheck(w http.ResponseWriter, r *http.Request, p identity.Principal, id string, filesOnly bool) {
	revision, key, ok := s.directLoadWriteGuard(w, r, true)
	if !ok {
		return
	}
	var body struct{}
	if !decodeBrowserJSON(w, r, &body) {
		return
	}
	draft, ok := s.loadDirectLoad(w, r, p, id)
	if !ok {
		return
	}
	if draft.Revision != revision {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	if s.prechecks == nil || s.agentPrechecks == nil || s.credentials == nil || s.nodes == nil || s.precheckTTL <= 0 {
		writeError(w, 503, "DIRECT_LOAD_PRECHECK_UNAVAILABLE", "旁路导入检查暂时不可用", false)
		return
	}
	config, err := directload.Decode(draft.ConfigJSON)
	if err != nil {
		writeError(w, 422, "DIRECT_LOAD_CONFIG_INVALID", "旁路导入配置无效", false)
		return
	}
	source, err := s.dataSource.GetDataSourceSummary(r.Context(), draft.DataSourceID)
	if err != nil {
		notFound(w, r)
		return
	}
	node, err := s.nodes.GetExecutionNodeFact(r.Context(), draft.NodeID)
	if err != nil {
		notFound(w, r)
		return
	}
	config, issues := directload.Normalize(config, source.CompatibilityMode, string(node.Platform), !filesOnly)
	if len(issues) != 0 {
		s.writeDirectLoadIssues(w, issues)
		return
	}
	if config.DataSourceID == "" || config.NodeID == "" || config.Database == "" || config.Scenario == "" || config.FilePath == "" || config.Table == "" || config.Format == "" {
		writeError(w, 422, "DIRECT_LOAD_INPUT_REQUIRED", "请先配置数据源、节点、数据库、场景、路径、目标表与数据格式", false)
		return
	}
	kind := "DIRECT_LOAD_FILES"
	if !filesOnly {
		kind = "DIRECT_LOAD_PREFLIGHT"
		preview, _, _, err := s.generateDirectLoad(r.Context(), config)
		if err != nil || preview.ConfigFingerprint != draft.ConfigFingerprint {
			writeError(w, 412, "DRAFT_BINDING_CHANGED", "配置或引用事实已变更，请重新保存", false)
			return
		}
	}
	ref, err := s.credentials.GetDataSourceCredentialReference(r.Context(), draft.DataSourceID)
	if err != nil {
		writeError(w, 503, "CREDENTIAL_REFERENCE_UNAVAILABLE", "凭据引用暂时不可用", true)
		return
	}
	checkID := newOpaqueID()
	if checkID == "" {
		writeError(w, 503, "IDENTIFIER_UNAVAILABLE", "标识暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	result, err := s.directLoads.CreateDirectLoadPrecheck(r.Context(), store.PrecheckCreate{CheckKind: kind, PrecheckRun: store.PrecheckRun{PrecheckID: checkID, DraftID: draft.ID, DraftRevision: draft.Revision, ConfigFingerprint: draft.ConfigFingerprint, DataSourceID: draft.DataSourceID, CredentialID: ref.CredentialID, CredentialRevision: ref.Revision, NodeID: draft.NodeID, CreatedAt: now, ValidUntil: now.Add(s.precheckTTL)}, CreatorSubjectID: p.ID, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: directLoadDigest(fmt.Sprintf("%s/%s/%d/%s", kind, draft.ID, draft.Revision, draft.ConfigFingerprint))})
	if err != nil {
		writeError(w, 422, "DIRECT_LOAD_PRECHECK_REJECTED", "检查绑定已失效或幂等请求冲突，请重新读取后重试", false)
		return
	}
	writeJSON(w, 202, map[string]any{"id": result.PrecheckID, "replayed": result.Replayed, "checkKind": kind})
}

func (s *Server) getDirectLoadCheck(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	item, err := s.directLoads.GetDirectLoadCheck(r.Context(), id, p.ID)
	if err != nil || !s.authorizedDirectLoad(r.Context(), p, item.Run.DataSourceID, item.Run.NodeID) {
		notFound(w, r)
		return
	}
	results := item.Run.Results
	if len(results) == 0 {
		for _, check := range precheckcontract.DirectLoadChecks(item.CheckKind) {
			code := "DIRECT_LOAD_UNAVAILABLE"
			results = append(results, store.PrecheckCheckResult{Check: check, Status: "UNKNOWN", EvidenceCode: code})
		}
	}
	response := map[string]any{"id": item.Run.PrecheckID, "draftId": item.Run.DraftID, "draftRevision": item.Run.DraftRevision, "configFingerprint": item.Run.ConfigFingerprint, "status": item.Run.Status, "integrityStatus": item.Run.IntegrityStatus, "validUntil": item.Run.ValidUntil, "checkKind": item.CheckKind, "results": results, "inputManifest": item.InputManifest, "executionEnabled": s.directLoadExecutionEnabled}
	if item.Run.DirectLoadConfigFingerprint != "" {
		response["directLoadConfigFingerprint"] = item.Run.DirectLoadConfigFingerprint
	}
	if item.SubmittedTaskID != "" {
		response["submittedTaskId"] = item.SubmittedTaskID
	}
	writeJSON(w, 200, response)
}

// submitDirectLoad 只在独立旁路执行准入、当前完整证明及高风险确认同时满足时冻结任务。
func (s *Server) submitDirectLoad(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	revision, key, ok := s.directLoadWriteGuard(w, r, true)
	if !ok {
		return
	}
	draft, ok := s.loadDirectLoad(w, r, p, id)
	if !ok {
		return
	}
	if revision != draft.Revision {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	var request directLoadSubmitRequest
	if !decodeBrowserJSON(w, r, &request) {
		return
	}
	if identity.Can(r.Context(), s.authorizer, p, identity.ScopeDataSourceWrite, draft.DataSourceID) != nil {
		writeError(w, 403, "DIRECT_LOAD_OPERATION_FORBIDDEN", "当前身份没有目标数据源的写入权限", false)
		return
	}
	if strings.TrimSpace(request.TaskName) == "" || len([]rune(strings.TrimSpace(request.TaskName))) > 200 || strings.ContainsFunc(request.TaskName, unicode.IsControl) {
		writeError(w, 422, "TASK_NAME_INVALID", "请输入不超过 200 个字符且不含控制字符的任务名称", false)
		return
	}
	if !s.directLoadExecutionEnabled || !s.realExecutionEnabled || s.tasks == nil {
		writeError(w, 422, "DIRECT_LOAD_EXECUTION_GATED", "旁路导入执行未启用；需满足独立准入与完整预检查", false)
		return
	}
	if identity.HasRole(r.Context(), s.roles, p, identity.RoleGlobalAdmin) != nil {
		writeError(w, 403, "DIRECT_LOAD_OPERATION_FORBIDDEN", "当前身份不能提交旁路导入操作", false)
		return
	}
	check, err := s.directLoads.GetDirectLoadCheck(r.Context(), request.PrecheckID, p.ID)
	// 已提交结果的核对不再次启动工具，也不依赖已过期的预检资格；仍要求当前授权及原请求绑定。
	if err == nil && check.SubmittedTaskID != "" && check.CheckKind == "DIRECT_LOAD_PREFLIGHT" && check.Run.DraftID == id && check.Run.DraftRevision == revision {
		if key != check.SubmittedIdempotencyKey || request.ConfirmedConfigFingerprint != check.Run.ConfigFingerprint || strings.TrimSpace(request.TaskName) != check.SubmittedTaskName {
			writeError(w, 409, "DIRECT_LOAD_SUBMISSION_CONFLICT", "此预检查已提交任务，请先核对原任务；更改配置后需重新预检查", false)
			return
		}
		writeJSON(w, 201, map[string]any{"id": check.SubmittedTaskID, "replayed": true, "state": "WAITING_SCHEDULE", "type": directload.TaskType})
		return
	}
	if err != nil || check.CheckKind != "DIRECT_LOAD_PREFLIGHT" || check.Run.DraftID != id || check.Run.DraftRevision != revision || check.Run.Status != "SUCCEEDED" || check.Run.IntegrityStatus != "COMPLETE" || !check.Run.ValidUntil.After(time.Now()) || check.InputManifest == nil || !completeDirectLoadChecks(check.Run.Results) || !validDirectLoadProjectionFingerprint(check.Run.DirectLoadConfigFingerprint) {
		writeError(w, 422, "PRECHECK_REQUIRED", "需要当前配置的完整九项旁路预检查", false)
		return
	}
	config, err := directload.Decode(draft.ConfigJSON)
	if err != nil {
		writeError(w, 422, "DIRECT_LOAD_CONFIG_INVALID", "旁路导入配置无效", false)
		return
	}
	preview, _, _, err := s.generateDirectLoad(r.Context(), config)
	if err != nil || preview.ConfigFingerprint != draft.ConfigFingerprint || preview.ConfigFingerprint != check.Run.ConfigFingerprint || !directloadfiles.Valid(*check.InputManifest, config) {
		writeError(w, 422, "PRECHECK_REQUIRED", "配置或单表文件证明已失效，请重新检查", false)
		return
	}
	if request.ConfirmedConfigFingerprint != preview.ConfigFingerprint {
		writeError(w, 422, "DIRECT_LOAD_CONFIRMATION_REQUIRED", "需确认当前冻结旁路配置的整表提交与写入影响", false)
		return
	}
	taskID := newOpaqueID()
	if taskID == "" {
		writeError(w, 503, "IDENTIFIER_UNAVAILABLE", "标识暂时不可用", true)
		return
	}
	snapshot, err := json.Marshal(store.DirectLoadSnapshot{ConfigVersion: directload.ConfigVersion, TaskType: directload.TaskType, TaskName: strings.TrimSpace(request.TaskName), Config: config, InputManifest: *check.InputManifest, DirectLoadConfigFingerprint: check.Run.DirectLoadConfigFingerprint, PrecheckProof: store.DirectLoadPrecheckProof{ID: check.Run.PrecheckID, DraftRevision: revision, ConfigFingerprint: preview.ConfigFingerprint, Results: check.Run.Results}})
	if err != nil {
		writeError(w, 422, "DIRECT_LOAD_SUBMIT_REJECTED", "旁路冻结证明无效", false)
		return
	}
	argv, _ := json.Marshal(preview.ArgvTemplate)
	now := time.Now().UTC()
	result, err := s.tasks.SubmitTaskIdempotent(r.Context(), store.TaskSubmission{TaskType: directload.TaskType, TaskID: taskID, CreatorSubjectID: p.ID, AuditActorID: p.ID, DataSourceID: draft.DataSourceID, NodeID: draft.NodeID, PrecheckID: check.Run.PrecheckID, CredentialID: check.Run.CredentialID, CredentialRevision: check.Run.CredentialRevision, ConfigFingerprint: preview.ConfigFingerprint, ToolVersion: preview.ToolVersion, MetadataVersion: preview.MetadataVersion, CapabilityVersion: preview.CapabilityVersion, SnapshotVersion: directload.ConfigVersion, SnapshotJSON: string(snapshot), PlannedArgvJSON: string(argv), PlannedCommandRedacted: preview.RedactedCommand, ParentTaskID: draft.SourceTaskID, DerivationKind: draft.SourceDerivation, RequestID: requestID(w), SubmittedAt: now}, key, directLoadDigest(fmt.Sprintf("%s/%d/%s/%s/%s", draft.ID, revision, check.Run.PrecheckID, preview.ConfigFingerprint, strings.TrimSpace(request.TaskName))))
	if err != nil {
		writeError(w, 422, "DIRECT_LOAD_SUBMIT_REJECTED", "提交失败，配置、权限、预检查或幂等绑定已变化", false)
		return
	}
	writeJSON(w, 201, map[string]any{"id": result.TaskID, "replayed": result.Replayed, "state": "WAITING_SCHEDULE", "type": directload.TaskType})
}

func completeDirectLoadChecks(results []store.PrecheckCheckResult) bool {
	checks := precheckcontract.DirectLoadChecks("DIRECT_LOAD_PREFLIGHT")
	if len(results) != len(checks) {
		return false
	}
	for i, check := range checks {
		if results[i].Check != check || results[i].Status != "PASSED" || !precheckcontract.ValidDirectLoadResult(check, results[i].Status, results[i].EvidenceCode) {
			return false
		}
	}
	return true
}
