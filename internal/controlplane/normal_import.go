package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/store"
)

// NormalImportDraftStore 提供独立导入配置的所有者存储边界，不接受导出配置。
type NormalImportDraftStore interface {
	SaveNormalImportDraft(context.Context, store.NormalImportDraftWrite) (store.NormalImportDraft, error)
	GetNormalImportDraft(context.Context, string, string) (store.NormalImportDraft, error)
	ListNormalImportDrafts(context.Context, string) ([]store.NormalImportDraft, error)
	DeleteNormalImportDraft(context.Context, string, string, int64, string, time.Time) error
	GetNormalImportCheck(context.Context, string, string) (store.NormalImportCheck, error)
	CreateNormalImportPrecheck(context.Context, store.PrecheckCreate) (store.PrecheckCreateResult, error)
}

// NormalImportCommandGenerator 仅暴露唯一生成器的结构化输入，不接收 Shell 文本或秘密明文。
type NormalImportCommandGenerator interface {
	Generate(commandgen.Request) (commandgen.Result, error)
}

type normalImportWriteRequest struct {
	ConfigVersion string              `json:"configVersion"`
	Config        importdomain.Config `json:"config"`
}
type normalImportSubmitRequest struct {
	PrecheckID                 string `json:"precheckId"`
	ConfirmedConfigFingerprint string `json:"confirmedConfigFingerprint,omitempty"`
	TaskName                   string `json:"taskName,omitempty"`
}

// normalImportEntry 仅分派普通导入的显式路由，复用浏览器认证外壳而不复制导出业务。
func (s *Server) normalImportEntry(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	base := "/api/v1/normal-import-drafts"
	if r.URL.Path != base && !strings.HasPrefix(r.URL.Path, base+"/") && !strings.HasPrefix(r.URL.Path, "/api/v1/normal-import-prechecks/") {
		return false
	}
	if s.normalImports == nil || s.authorizer == nil || s.dataSource == nil {
		writeError(w, 503, "NORMAL_IMPORT_UNAVAILABLE", "普通导入服务暂时不可用", false)
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/normal-import-prechecks/") {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/normal-import-prechecks/")
		if r.Method != http.MethodGet || !validTaskPathID(id) {
			notFound(w, r)
			return true
		}
		s.getNormalImportCheck(w, r, p, id)
		return true
	}
	if r.URL.Path == base {
		switch r.Method {
		case http.MethodGet:
			items, err := s.normalImports.ListNormalImportDrafts(r.Context(), p.ID)
			if err != nil {
				writeError(w, 503, "NORMAL_IMPORT_UNAVAILABLE", "普通导入草稿暂时不可用", true)
				return true
			}
			visible := []store.NormalImportDraft{}
			for _, item := range items {
				if s.authorizedNormalImport(r.Context(), p, item.DataSourceID, item.NodeID) {
					visible = append(visible, item)
				}
			}
			writeJSON(w, 200, map[string]any{"items": visible, "executionEnabled": s.normalImportExecutionEnabled})
		case http.MethodPost:
			s.saveNormalImport(w, r, p, "")
		default:
			notFound(w, r)
		}
		return true
	}
	suffix := strings.TrimPrefix(r.URL.Path, base+"/")
	parts := strings.SplitN(suffix, ":", 2)
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if !validTaskPathID(id) {
		notFound(w, r)
		return true
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		item, ok := s.loadNormalImport(w, r, p, id)
		if ok {
			s.writeNormalImport(w, item)
		}
	case action == "" && r.Method == http.MethodPatch:
		s.saveNormalImport(w, r, p, id)
	case action == "" && r.Method == http.MethodDelete:
		item, ok := s.loadNormalImport(w, r, p, id)
		if !ok {
			return true
		}
		revision, key, ok := s.normalImportWriteGuard(w, r, true)
		_ = key
		if !ok {
			return true
		}
		if revision != item.Revision {
			writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
			return true
		}
		if err := s.normalImports.DeleteNormalImportDraft(r.Context(), id, p.ID, revision, requestID(w), time.Now().UTC()); err != nil {
			writeError(w, 409, "DRAFT_DELETE_REJECTED", "草稿已有提交或活动检查，不能删除", false)
		} else {
			writeJSON(w, 200, map[string]any{"deleted": true})
		}
	case action == "preview-command" && r.Method == http.MethodPost:
		s.previewNormalImport(w, r, p, id)
	case (action == "precheck" || action == "discover-files") && r.Method == http.MethodPost:
		s.createNormalImportCheck(w, r, p, id, action == "discover-files")
	case action == "submit" && r.Method == http.MethodPost:
		s.submitNormalImport(w, r, p, id)
	default:
		notFound(w, r)
	}
	return true
}

func (s *Server) authorizedNormalImport(ctx context.Context, p identity.Principal, source, node string) bool {
	return identity.Can(ctx, s.authorizer, p, identity.ScopeDataSourceUse, source) == nil && (node == "" || identity.Can(ctx, s.authorizer, p, identity.ScopeNodeUse, node) == nil)
}

func (s *Server) loadNormalImport(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) (store.NormalImportDraft, bool) {
	item, err := s.normalImports.GetNormalImportDraft(r.Context(), id, p.ID)
	if err != nil || !s.authorizedNormalImport(r.Context(), p, item.DataSourceID, item.NodeID) {
		notFound(w, r)
		return store.NormalImportDraft{}, false
	}
	return item, true
}

func (s *Server) normalImportWriteGuard(w http.ResponseWriter, r *http.Request, revisionRequired bool) (int64, string, bool) {
	if s.csrf == nil || s.csrf.ValidateCSRF(r) != nil {
		writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return 0, "", false
	}
	revision := int64(0)
	if revisionRequired {
		var ok bool
		revision, ok = parseIfMatchRevision(r.Header.Get("If-Match"))
		if !ok {
			writeError(w, 428, "REVISION_REQUIRED", "需要有效的草稿版本", false)
			return 0, "", false
		}
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if r.Method == http.MethodPost && (len(key) < 16 || len(key) > 200) {
		writeError(w, 400, "IDEMPOTENCY_KEY_INVALID", "幂等键无效", false)
		return 0, "", false
	}
	return revision, key, true
}

func (s *Server) saveNormalImport(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	revision, key, ok := s.normalImportWriteGuard(w, r, id != "")
	if !ok {
		return
	}
	if id != "" {
		previous, found := s.loadNormalImport(w, r, p, id)
		if !found {
			return
		}
		if previous.Revision != revision {
			writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
			return
		}
	}
	var request normalImportWriteRequest
	if !decodeBrowserJSON(w, r, &request) {
		return
	}
	if request.ConfigVersion != importdomain.ConfigVersion {
		writeError(w, 422, "NORMAL_IMPORT_VERSION_UNSUPPORTED", "普通导入配置版本不受支持", false)
		return
	}
	if !s.authorizedNormalImport(r.Context(), p, request.Config.DataSourceID, request.Config.NodeID) {
		notFound(w, r)
		return
	}
	source, err := s.dataSource.GetDataSourceSummary(r.Context(), request.Config.DataSourceID)
	if err != nil || source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" || source.ConnectionKind != "ODP" {
		notFound(w, r)
		return
	}
	var node ExecutionNodeFact
	if request.Config.NodeID != "" {
		if s.nodes == nil {
			writeError(w, 503, "NORMAL_IMPORT_UNAVAILABLE", "执行节点事实暂时不可用", false)
			return
		}
		node, err = s.nodes.GetExecutionNodeFact(r.Context(), request.Config.NodeID)
		if err != nil {
			notFound(w, r)
			return
		}
	}
	config, issues := importdomain.Normalize(request.Config, source.CompatibilityMode, string(node.Platform), false)
	if len(issues) != 0 {
		s.writeNormalImportIssues(w, issues)
		return
	}
	raw, _ := json.Marshal(config)
	fingerprint := normalImportDigest(string(raw))
	// 未完成草稿仍可保存；只有完整配置才绑定唯一生成器的指纹。
	if _, completeIssues := importdomain.Normalize(config, source.CompatibilityMode, string(node.Platform), true); len(completeIssues) == 0 {
		preview, _, _, generateErr := s.generateNormalImport(r.Context(), config)
		if generateErr != nil {
			writeError(w, 422, "NORMAL_IMPORT_CONFIG_INVALID", "普通导入配置不能生成命令", false)
			return
		}
		fingerprint = preview.ConfigFingerprint
	}
	status := http.StatusOK
	if id == "" {
		id = newOpaqueID()
		status = http.StatusCreated
	}
	if id == "" {
		writeError(w, 503, "IDENTIFIER_UNAVAILABLE", "标识暂时不可用", true)
		return
	}
	item, err := s.normalImports.SaveNormalImportDraft(r.Context(), store.NormalImportDraftWrite{NormalImportDraft: store.NormalImportDraft{ID: id, OwnerSubjectID: p.ID, DataSourceID: config.DataSourceID, NodeID: config.NodeID, ConfigJSON: string(raw), ConfigFingerprint: fingerprint, UpdatedAt: time.Now().UTC()}, ExpectedRevision: revision, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: normalImportDigest(string(raw))})
	if errors.Is(err, store.ErrPrecheckInvalid) {
		// 仓储拒绝冻结重跑配置的改写属于业务约束，不能误报成可重试的服务故障。
		writeError(w, 422, "NORMAL_IMPORT_CONFIG_LOCKED", "从头重新执行必须保留原配置；如需修改参数，请基于原配置新建", false)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if err != nil {
		writeError(w, 503, "NORMAL_IMPORT_SAVE_UNAVAILABLE", "普通导入草稿保存失败", true)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, item.Revision))
	writeJSON(w, status, s.normalImportResponse(item))
}

func (s *Server) writeNormalImportIssues(w http.ResponseWriter, issues []importdomain.Issue) {
	fields := make([]fieldErrorResponse, 0, len(issues))
	for _, issue := range issues {
		fields = append(fields, fieldErrorResponse{Field: issue.Field, Code: issue.Code, Message: "请检查该参数的适用条件或取值"})
	}
	writeErrorWithFields(w, 422, "NORMAL_IMPORT_CONFIG_INVALID", "普通导入配置不完整或存在冲突", false, fields)
}

func (s *Server) normalImportResponse(item store.NormalImportDraft) map[string]any {
	config, _ := importdomain.Decode(item.ConfigJSON)
	return map[string]any{"id": item.ID, "revision": item.Revision, "configVersion": importdomain.ConfigVersion, "config": config, "configFingerprint": item.ConfigFingerprint, "metadataVersion": importdomain.MetadataVersion, "capabilityVersion": importdomain.CapabilityVersion, "executionEnabled": s.normalImportExecutionEnabled, "updatedAt": item.UpdatedAt, "sourceTaskId": item.SourceTaskID, "sourceDerivation": item.SourceDerivation}
}
func (s *Server) writeNormalImport(w http.ResponseWriter, item store.NormalImportDraft) {
	w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, item.Revision))
	writeJSON(w, 200, s.normalImportResponse(item))
}
func normalImportDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Server) generateNormalImport(ctx context.Context, c importdomain.Config) (commandgen.Result, store.DataSourceSummary, ExecutionNodeFact, error) {
	if err := s.authorizeExpertConfiguration(ctx, c.HighRisk()); err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	if s.normalImportGenerator == nil || s.credentials == nil || s.nodes == nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("普通导入依赖未配置")
	}
	source, err := s.dataSource.GetDataSourceSummary(ctx, c.DataSourceID)
	if err != nil {
		return commandgen.Result{}, source, ExecutionNodeFact{}, err
	}
	node, err := s.nodes.GetExecutionNodeFact(ctx, c.NodeID)
	if err != nil {
		return commandgen.Result{}, source, node, err
	}
	if source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" || source.ConnectionKind != "ODP" || (source.CompatibilityMode != "MYSQL" && source.CompatibilityMode != "ORACLE") {
		return commandgen.Result{}, source, node, errors.New("普通导入数据源不适用")
	}
	c, issues := importdomain.Normalize(c, source.CompatibilityMode, string(node.Platform), true)
	if len(issues) != 0 {
		return commandgen.Result{}, source, node, errors.New("普通导入配置不完整")
	}
	ref, err := s.credentials.GetDataSourceCredentialReference(ctx, c.DataSourceID)
	if err != nil {
		return commandgen.Result{}, source, node, err
	}
	username, valid := store.PrivateODPCommandIdentity(source.Username, source.TenantName, source.ClusterName)
	if !valid {
		return commandgen.Result{}, source, node, errors.New("普通导入连接身份无效")
	}
	fields := []commandgen.FieldInput{
		{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: source.Host}},
		{Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: int64(source.Port)}},
		{Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: username}},
		{Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: c.Database}},
		{Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: ref.CredentialID, Revision: ref.Revision}}},
		{Name: "--no-sys", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}},
	}
	fields = append(fields, importdomain.Fields(c)...)
	if c.Cleanup == "TRUNCATE" || c.Cleanup == "DELETE" {
		fields = append(fields, commandgen.FieldInput{Name: "--yes", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	result, err := s.normalImportGenerator.Generate(commandgen.Request{Tool: "OBLOADER", ToolVersion: importdomain.ToolVersion, MetadataVersion: importdomain.MetadataVersion, CapabilityVersion: importdomain.CapabilityVersion, ConnectionKind: commandgen.ConnectionODP, DataSourceFactVersion: fmt.Sprintf("ds-rev-%d", source.Revision), NodeFactVersion: node.FactsVersion, TargetPlatform: node.Platform, Fields: fields})
	return result, source, node, err
}

func (s *Server) previewNormalImport(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	var body struct{}
	if !decodeBrowserJSON(w, r, &body) {
		return
	}
	revision, _, ok := s.normalImportWriteGuard(w, r, true)
	if !ok {
		return
	}
	item, ok := s.loadNormalImport(w, r, p, id)
	if !ok {
		return
	}
	if item.Revision != revision {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	config, err := importdomain.Decode(item.ConfigJSON)
	if err != nil {
		writeError(w, 422, "NORMAL_IMPORT_CONFIG_INVALID", "普通导入配置无效", false)
		return
	}
	result, _, _, err := s.generateNormalImport(r.Context(), config)
	if err != nil {
		writeError(w, 422, "NORMAL_IMPORT_CONFIG_INVALID", "请先补齐并保存普通导入配置", false)
		return
	}
	if result.ConfigFingerprint != item.ConfigFingerprint {
		writeError(w, 412, "DRAFT_BINDING_CHANGED", "数据源或节点事实已变更，请重新保存", false)
		return
	}
	writeJSON(w, 200, map[string]any{"redactedCommand": result.RedactedCommand, "configFingerprint": result.ConfigFingerprint, "metadataVersion": result.MetadataVersion, "toolVersion": result.ToolVersion, "capabilityVersion": result.CapabilityVersion, "executionEnabled": s.normalImportExecutionEnabled})
}
