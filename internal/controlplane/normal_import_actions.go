package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/precheckcontract"
	"ob-data-orch/internal/store"
)

func precheckChecksForContext(c store.PrecheckExecutionContext) []agentpreflight.CheckID {
	if c.DirectLoad != nil {
		var checks []agentpreflight.CheckID
		for _, name := range precheckcontract.DirectLoadChecks(c.CheckKind) {
			checks = append(checks, agentpreflight.CheckID(name))
		}
		return checks
	}
	if c.NormalImport == nil {
		return agentpreflight.ChecksForOutputKind(agentpreflight.OutputKind(c.OutputKind))
	}
	var checks []agentpreflight.CheckID
	for _, name := range precheckcontract.NormalImportChecks(c.CheckKind) {
		checks = append(checks, agentpreflight.CheckID(name))
	}
	return checks
}

func (s *Server) createNormalImportCheck(w http.ResponseWriter, r *http.Request, p identity.Principal, id string, filesOnly bool) {
	revision, key, ok := s.normalImportWriteGuard(w, r, true)
	if !ok {
		return
	}
	var body struct{}
	if !decodeBrowserJSON(w, r, &body) {
		return
	}
	draft, ok := s.loadNormalImport(w, r, p, id)
	if !ok {
		return
	}
	if draft.Revision != revision {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	if s.prechecks == nil || s.agentPrechecks == nil || s.credentials == nil || s.nodes == nil || s.precheckTTL <= 0 {
		writeError(w, 503, "NORMAL_IMPORT_PRECHECK_UNAVAILABLE", "普通导入检查暂时不可用", false)
		return
	}
	config, err := importdomain.Decode(draft.ConfigJSON)
	if err != nil {
		writeError(w, 422, "NORMAL_IMPORT_CONFIG_INVALID", "普通导入配置无效", false)
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
	config, issues := importdomain.Normalize(config, source.CompatibilityMode, string(node.Platform), !filesOnly)
	if len(issues) != 0 {
		s.writeNormalImportIssues(w, issues)
		return
	}
	if config.FilePath == "" || config.NodeID == "" || config.Content == "" || config.HasData() && config.Format == "" {
		writeError(w, 422, "NORMAL_IMPORT_INPUT_REQUIRED", "请先配置节点、路径、内容与数据格式", false)
		return
	}
	kind := "NORMAL_IMPORT_FILES"
	if !filesOnly {
		kind = "NORMAL_IMPORT_PREFLIGHT"
		preview, _, _, err := s.generateNormalImport(r.Context(), config)
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
	result, err := s.normalImports.CreateNormalImportPrecheck(r.Context(), store.PrecheckCreate{CheckKind: kind, PrecheckRun: store.PrecheckRun{PrecheckID: checkID, DraftID: draft.ID, DraftRevision: draft.Revision, ConfigFingerprint: draft.ConfigFingerprint, DataSourceID: draft.DataSourceID, CredentialID: ref.CredentialID, CredentialRevision: ref.Revision, NodeID: draft.NodeID, CreatedAt: now, ValidUntil: now.Add(s.precheckTTL)}, CreatorSubjectID: p.ID, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: normalImportDigest(fmt.Sprintf("%s/%s/%d/%s", kind, draft.ID, draft.Revision, draft.ConfigFingerprint))})
	if err != nil {
		writeError(w, 422, "NORMAL_IMPORT_PRECHECK_REJECTED", "检查绑定已失效或幂等请求冲突，请重新读取后重试", false)
		return
	}
	writeJSON(w, 202, map[string]any{"id": result.PrecheckID, "replayed": result.Replayed, "checkKind": kind})
}

func (s *Server) getNormalImportCheck(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	item, err := s.normalImports.GetNormalImportCheck(r.Context(), id, p.ID)
	if err != nil || !s.authorizedNormalImport(r.Context(), p, item.Run.DataSourceID, item.Run.NodeID) {
		notFound(w, r)
		return
	}
	results := item.Run.Results
	if len(results) == 0 {
		for _, check := range precheckcontract.NormalImportChecks(item.CheckKind) {
			code := "IMPORT_UNAVAILABLE"
			switch check {
			case "DATABASE_CONNECTIVITY":
				code = "DATABASE_CONNECTION_UNAVAILABLE"
			case "TOOL_ENVIRONMENT":
				code = "TOOL_RUNTIME_UNAVAILABLE"
			case "NODE_RESOURCE":
				code = "NODE_RESOURCE_UNAVAILABLE"
			}
			results = append(results, store.PrecheckCheckResult{Check: check, Status: "UNKNOWN", EvidenceCode: code})
		}
	}
	writeJSON(w, 200, map[string]any{"id": item.Run.PrecheckID, "draftId": item.Run.DraftID, "draftRevision": item.Run.DraftRevision, "configFingerprint": item.Run.ConfigFingerprint, "status": item.Run.Status, "integrityStatus": item.Run.IntegrityStatus, "validUntil": item.Run.ValidUntil, "checkKind": item.CheckKind, "results": results, "inputManifest": item.InputManifest, "executionEnabled": s.normalImportExecutionEnabled})
}

func (s *Server) submitNormalImport(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	revision, key, ok := s.normalImportWriteGuard(w, r, true)
	if !ok {
		return
	}
	draft, ok := s.loadNormalImport(w, r, p, id)
	if !ok {
		return
	}
	if !s.normalImportExecutionEnabled || !s.realExecutionEnabled || s.tasks == nil {
		writeError(w, 422, "NORMAL_IMPORT_EXECUTION_GATED", "普通导入真实执行尚未开放", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, p, identity.ScopeDataSourceWrite, draft.DataSourceID) != nil {
		writeError(w, 403, "IMPORT_OPERATION_FORBIDDEN", "当前身份没有目标数据源的写入权限", false)
		return
	}
	if draft.Revision != revision {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	var request normalImportSubmitRequest
	if !decodeBrowserJSON(w, r, &request) {
		return
	}
	check, err := s.normalImports.GetNormalImportCheck(r.Context(), request.PrecheckID, p.ID)
	if err != nil || check.CheckKind != "NORMAL_IMPORT_PREFLIGHT" || check.Run.DraftID != id || check.Run.DraftRevision != revision || check.Run.Status != "SUCCEEDED" || check.Run.IntegrityStatus != "COMPLETE" || !check.Run.ValidUntil.After(time.Now()) || check.InputManifest == nil {
		writeError(w, 422, "PRECHECK_REQUIRED", "需要当前配置的完整普通导入预检查", false)
		return
	}
	config, err := importdomain.Decode(draft.ConfigJSON)
	if err != nil {
		writeError(w, 422, "NORMAL_IMPORT_CONFIG_INVALID", "普通导入配置无效", false)
		return
	}
	preview, source, _, err := s.generateNormalImport(r.Context(), config)
	if err != nil || preview.ConfigFingerprint != draft.ConfigFingerprint || preview.ConfigFingerprint != check.Run.ConfigFingerprint || !check.InputManifest.Valid(config) {
		writeError(w, 422, "PRECHECK_REQUIRED", "配置或文件证据已失效，请重新检查", false)
		return
	}
	if config.HighRisk() || source.Environment == "PRODUCTION" {
		// 当前部署的全局管理员承载生产和专家操作；缺失可信角色或冻结确认均失败关闭。
		if identity.HasRole(r.Context(), s.roles, p, identity.RoleGlobalAdmin) != nil {
			writeError(w, 403, "IMPORT_OPERATION_FORBIDDEN", "当前身份不能提交该导入操作", false)
			return
		}
		if request.ConfirmedConfigFingerprint != preview.ConfigFingerprint {
			writeError(w, 422, "IMPORT_CONFIRMATION_REQUIRED", "需确认当前冻结配置的写入与结构影响", false)
			return
		}
	}
	argv, _ := json.Marshal(preview.ArgvTemplate)
	taskID := newOpaqueID()
	if taskID == "" {
		writeError(w, 503, "IDENTIFIER_UNAVAILABLE", "标识暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	name, nameErr := importdomain.TaskName(request.TaskName, config.Database, config.Format, taskID, now)
	if nameErr != nil {
		writeError(w, 422, "TASK_NAME_INVALID", "任务名称最多 200 个字符，且不能包含控制字符", false)
		return
	}
	snapshot, _ := json.Marshal(store.NormalImportSnapshot{ConfigVersion: importdomain.ConfigVersion, TaskType: importdomain.TaskType, TaskName: name, Config: config, InputManifest: *check.InputManifest})
	result, err := s.tasks.SubmitTaskIdempotent(r.Context(), store.TaskSubmission{TaskType: importdomain.TaskType, ParentTaskID: draft.SourceTaskID, DerivationKind: draft.SourceDerivation, TaskID: taskID, CreatorSubjectID: p.ID, AuditActorID: p.ID, DataSourceID: draft.DataSourceID, NodeID: draft.NodeID, PrecheckID: check.Run.PrecheckID, CredentialID: check.Run.CredentialID, CredentialRevision: check.Run.CredentialRevision, ConfigFingerprint: preview.ConfigFingerprint, ToolVersion: preview.ToolVersion, MetadataVersion: preview.MetadataVersion, CapabilityVersion: preview.CapabilityVersion, SnapshotVersion: importdomain.ConfigVersion, SnapshotJSON: string(snapshot), PlannedArgvJSON: string(argv), PlannedCommandRedacted: preview.RedactedCommand, RequestID: requestID(w), SubmittedAt: now}, key, normalImportDigest(fmt.Sprintf("%s/%d/%s/%s/%s", draft.ID, revision, check.Run.PrecheckID, preview.ConfigFingerprint, strings.TrimSpace(request.TaskName))))
	if err != nil {
		writeError(w, 422, "NORMAL_IMPORT_SUBMIT_REJECTED", "提交失败，配置、权限、预检查或幂等绑定已变化", false)
		return
	}
	writeJSON(w, 201, map[string]any{"id": result.TaskID, "replayed": result.Replayed, "state": "WAITING_SCHEDULE", "type": importdomain.TaskType})
}
