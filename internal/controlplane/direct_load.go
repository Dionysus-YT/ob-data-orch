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
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/parammeta"
	"ob-data-orch/internal/store"
)

// DirectLoadDraftStore 提供独立导入配置的所有者存储边界，不接受导出配置。
type DirectLoadDraftStore interface {
	SaveDirectLoadDraft(context.Context, store.DirectLoadDraftWrite) (store.DirectLoadDraft, error)
	GetDirectLoadDraft(context.Context, string, string) (store.DirectLoadDraft, error)
	ListDirectLoadDrafts(context.Context, string) ([]store.DirectLoadDraft, error)
	DeleteDirectLoadDraft(context.Context, string, string, int64, string, time.Time) error
	GetDirectLoadCheck(context.Context, string, string) (store.DirectLoadCheck, error)
	CreateDirectLoadPrecheck(context.Context, store.PrecheckCreate) (store.PrecheckCreateResult, error)
}

// DirectLoadCommandGenerator 仅暴露唯一生成器的结构化输入，不接收 Shell 文本或秘密明文。
type DirectLoadCommandGenerator interface {
	Generate(commandgen.Request) (commandgen.Result, error)
}

type directLoadWriteRequest struct {
	ConfigVersion string            `json:"configVersion"`
	Config        directload.Config `json:"config"`
}

// UnmarshalJSON 在 HTTP 边界复用领域的严格配置解析，重复键与空值不能被标准解码器静默覆盖。
func (request *directLoadWriteRequest) UnmarshalJSON(raw []byte) error {
	// 顶层同样只接受精确契约键；大小写别名和重复配置不能在进入领域前被静默覆盖。
	keys := json.NewDecoder(strings.NewReader(string(raw)))
	if token, err := keys.Token(); err != nil || token != json.Delim('{') {
		return errors.New("旁路请求结构无效")
	}
	seen := map[string]bool{}
	for keys.More() {
		token, err := keys.Token()
		name, ok := token.(string)
		if err != nil || !ok || seen[name] || name != "configVersion" && name != "config" {
			return errors.New("旁路请求字段无效")
		}
		seen[name] = true
		var value json.RawMessage
		if err := keys.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := keys.Token(); err != nil {
		return err
	}
	var wire struct {
		ConfigVersion string          `json:"configVersion"`
		Config        json.RawMessage `json:"config"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	config, err := directload.Decode(string(wire.Config))
	if err != nil {
		return err
	}
	request.ConfigVersion, request.Config = wire.ConfigVersion, config
	return nil
}

type directLoadSubmitRequest struct {
	PrecheckID                 string `json:"precheckId"`
	ConfirmedConfigFingerprint string `json:"confirmedConfigFingerprint,omitempty"`
	TaskName                   string `json:"taskName,omitempty"`
}

// directLoadEntry 仅分派旁路导入的显式路由，复用浏览器认证外壳而不复制导出业务。
func (s *Server) directLoadEntry(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/direct-load-parameter-metadata" {
		catalog, err := parammeta.LoadDirectLoad()
		if err != nil {
			writeError(w, 503, "DIRECT_LOAD_METADATA_UNAVAILABLE", "旁路参数目录暂时不可用", false)
			return true
		}
		writeJSON(w, 200, map[string]any{"tool": catalog.Tool(), "toolVersion": catalog.ToolVersion(), "metadataVersion": catalog.MetadataVersion(), "capabilityVersion": catalog.CapabilityVersion(), "definitions": catalog.Definitions()})
		return true
	}
	base := "/api/v1/direct-load-drafts"
	if r.URL.Path != base && !strings.HasPrefix(r.URL.Path, base+"/") && !strings.HasPrefix(r.URL.Path, "/api/v1/direct-load-prechecks/") {
		return false
	}
	if s.directLoads == nil || s.authorizer == nil || s.dataSource == nil {
		writeError(w, 503, "DIRECT_LOAD_UNAVAILABLE", "旁路导入服务暂时不可用", false)
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/direct-load-prechecks/") {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/direct-load-prechecks/")
		if r.Method != http.MethodGet || !validTaskPathID(id) {
			notFound(w, r)
			return true
		}
		s.getDirectLoadCheck(w, r, p, id)
		return true
	}
	if r.URL.Path == base {
		switch r.Method {
		case http.MethodGet:
			items, err := s.directLoads.ListDirectLoadDrafts(r.Context(), p.ID)
			if err != nil {
				writeError(w, 503, "DIRECT_LOAD_UNAVAILABLE", "旁路导入草稿暂时不可用", true)
				return true
			}
			visible := []store.DirectLoadDraft{}
			for _, item := range items {
				if s.authorizedDirectLoad(r.Context(), p, item.DataSourceID, item.NodeID) {
					visible = append(visible, item)
				}
			}
			writeJSON(w, 200, map[string]any{"items": visible, "executionEnabled": s.directLoadExecutionEnabled})
		case http.MethodPost:
			s.saveDirectLoad(w, r, p, "")
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
		item, ok := s.loadDirectLoad(w, r, p, id)
		if ok {
			s.writeDirectLoad(w, item)
		}
	case action == "" && r.Method == http.MethodPatch:
		s.saveDirectLoad(w, r, p, id)
	case action == "" && r.Method == http.MethodDelete:
		item, ok := s.loadDirectLoad(w, r, p, id)
		if !ok {
			return true
		}
		revision, key, ok := s.directLoadWriteGuard(w, r, true)
		_ = key
		if !ok {
			return true
		}
		if revision != item.Revision {
			writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
			return true
		}
		if err := s.directLoads.DeleteDirectLoadDraft(r.Context(), id, p.ID, revision, requestID(w), time.Now().UTC()); err != nil {
			writeError(w, 409, "DRAFT_DELETE_REJECTED", "草稿已有提交或活动检查，不能删除", false)
		} else {
			writeJSON(w, 200, map[string]any{"deleted": true})
		}
	case action == "preview-command" && r.Method == http.MethodPost:
		s.previewDirectLoad(w, r, p, id)
	case (action == "precheck" || action == "discover-files") && r.Method == http.MethodPost:
		s.createDirectLoadCheck(w, r, p, id, action == "discover-files")
	case action == "submit" && r.Method == http.MethodPost:
		s.submitDirectLoad(w, r, p, id)
	default:
		notFound(w, r)
	}
	return true
}

func (s *Server) authorizedDirectLoad(ctx context.Context, p identity.Principal, source, node string) bool {
	return identity.Can(ctx, s.authorizer, p, identity.ScopeDataSourceUse, source) == nil && (node == "" || identity.Can(ctx, s.authorizer, p, identity.ScopeNodeUse, node) == nil)
}

func (s *Server) loadDirectLoad(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) (store.DirectLoadDraft, bool) {
	item, err := s.directLoads.GetDirectLoadDraft(r.Context(), id, p.ID)
	if err != nil || !s.authorizedDirectLoad(r.Context(), p, item.DataSourceID, item.NodeID) {
		notFound(w, r)
		return store.DirectLoadDraft{}, false
	}
	return item, true
}

func (s *Server) directLoadWriteGuard(w http.ResponseWriter, r *http.Request, revisionRequired bool) (int64, string, bool) {
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

func (s *Server) saveDirectLoad(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	revision, key, ok := s.directLoadWriteGuard(w, r, id != "")
	if !ok {
		return
	}
	var previous store.DirectLoadDraft
	if id != "" {
		var found bool
		previous, found = s.loadDirectLoad(w, r, p, id)
		if !found {
			return
		}
		if previous.Revision != revision {
			writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
			return
		}
	}
	var request directLoadWriteRequest
	if !decodeBrowserJSON(w, r, &request) {
		return
	}
	if request.ConfigVersion != directload.ConfigVersion {
		writeError(w, 422, "DIRECT_LOAD_VERSION_UNSUPPORTED", "旁路导入配置版本不受支持", false)
		return
	}
	if previous.SourceDerivation == "RERUN_FROM_SCRATCH" {
		original, err := directload.Decode(previous.ConfigJSON)
		before, _ := json.Marshal(original)
		after, _ := json.Marshal(request.Config)
		if err != nil || string(before) != string(after) {
			writeError(w, 422, "DIRECT_LOAD_PARAMETERS_LOCKED", "从头执行保留原配置；如需修改，请基于配置新建", false)
			return
		}
	}
	if !s.authorizedDirectLoad(r.Context(), p, request.Config.DataSourceID, request.Config.NodeID) {
		notFound(w, r)
		return
	}
	source, err := s.dataSource.GetDataSourceSummary(r.Context(), request.Config.DataSourceID)
	if err != nil || source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" || !store.DirectLoadSourceMatches(request.Config.Scenario, source) {
		notFound(w, r)
		return
	}
	var node ExecutionNodeFact
	if request.Config.NodeID != "" {
		if s.nodes == nil {
			writeError(w, 503, "DIRECT_LOAD_UNAVAILABLE", "执行节点事实暂时不可用", false)
			return
		}
		node, err = s.nodes.GetExecutionNodeFact(r.Context(), request.Config.NodeID)
		if err != nil {
			notFound(w, r)
			return
		}
	}
	config, issues := directload.Normalize(request.Config, source.CompatibilityMode, string(node.Platform), false)
	if len(issues) != 0 {
		s.writeDirectLoadIssues(w, issues)
		return
	}
	raw, _ := json.Marshal(config)
	fingerprint := directLoadDigest(string(raw))
	// 未完成草稿仍可保存；只有完整配置才绑定唯一生成器的指纹。
	if _, completeIssues := directload.Normalize(config, source.CompatibilityMode, string(node.Platform), true); len(completeIssues) == 0 {
		preview, _, _, generateErr := s.generateDirectLoad(r.Context(), config)
		if generateErr != nil {
			writeError(w, 422, "DIRECT_LOAD_CONFIG_INVALID", "旁路导入配置不能生成命令", false)
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
	item, err := s.directLoads.SaveDirectLoadDraft(r.Context(), store.DirectLoadDraftWrite{DirectLoadDraft: store.DirectLoadDraft{ID: id, OwnerSubjectID: p.ID, DataSourceID: config.DataSourceID, NodeID: config.NodeID, ConfigJSON: string(raw), ConfigFingerprint: fingerprint, SourceTaskID: previous.SourceTaskID, SourceDerivation: previous.SourceDerivation, UpdatedAt: time.Now().UTC()}, ExpectedRevision: revision, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: directLoadDigest(string(raw))})
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if errors.Is(err, store.ErrPrecheckInvalid) {
		writeError(w, 422, "DIRECT_LOAD_CONFIG_INVALID", "旁路配置、来源或参数锁定已变化", false)
		return
	}
	if err != nil {
		writeError(w, 503, "DIRECT_LOAD_SAVE_UNAVAILABLE", "旁路导入草稿保存失败", true)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, item.Revision))
	writeJSON(w, status, s.directLoadResponse(item))
}

func (s *Server) writeDirectLoadIssues(w http.ResponseWriter, issues []directload.Issue) {
	fields := make([]fieldErrorResponse, 0, len(issues))
	for _, issue := range issues {
		fields = append(fields, fieldErrorResponse{Field: issue.Field, Code: issue.Code, Message: "请检查该参数的适用条件或取值"})
	}
	writeErrorWithFields(w, 422, "DIRECT_LOAD_CONFIG_INVALID", "旁路导入配置不完整或存在冲突", false, fields)
}

func (s *Server) directLoadResponse(item store.DirectLoadDraft) map[string]any {
	config, _ := directload.Decode(item.ConfigJSON)
	result := map[string]any{"id": item.ID, "revision": item.Revision, "configVersion": directload.ConfigVersion, "config": config, "configFingerprint": item.ConfigFingerprint, "metadataVersion": directload.MetadataVersion, "capabilityVersion": directload.CapabilityVersion, "executionEnabled": s.directLoadExecutionEnabled, "updatedAt": item.UpdatedAt, "rpcPortSource": directload.RPCSource, "parametersLocked": item.SourceDerivation == "RERUN_FROM_SCRATCH"}
	if item.SourceTaskID != "" {
		result["sourceTaskId"], result["sourceDerivation"] = item.SourceTaskID, item.SourceDerivation
	}
	return result
}
func (s *Server) writeDirectLoad(w http.ResponseWriter, item store.DirectLoadDraft) {
	w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, item.Revision))
	writeJSON(w, 200, s.directLoadResponse(item))
}
func directLoadDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Server) generateDirectLoad(ctx context.Context, c directload.Config) (commandgen.Result, store.DataSourceSummary, ExecutionNodeFact, error) {
	if err := s.authorizeExpertConfiguration(ctx, true); err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	if s.directLoadGenerator == nil || s.credentials == nil || s.nodes == nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("旁路导入依赖未配置")
	}
	source, err := s.dataSource.GetDataSourceSummary(ctx, c.DataSourceID)
	if err != nil {
		return commandgen.Result{}, source, ExecutionNodeFact{}, err
	}
	node, err := s.nodes.GetExecutionNodeFact(ctx, c.NodeID)
	if err != nil {
		return commandgen.Result{}, source, node, err
	}
	if source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" || !store.DirectLoadSourceMatches(c.Scenario, source) || (source.CompatibilityMode != "MYSQL" && source.CompatibilityMode != "ORACLE") {
		return commandgen.Result{}, source, node, errors.New("旁路导入数据源不适用")
	}
	c, issues := directload.Normalize(c, source.CompatibilityMode, string(node.Platform), true)
	if len(issues) != 0 {
		return commandgen.Result{}, source, node, errors.New("旁路导入配置不完整")
	}
	ref, err := s.credentials.GetDataSourceCredentialReference(ctx, c.DataSourceID)
	if err != nil {
		return commandgen.Result{}, source, node, err
	}
	username := source.Username
	if c.Scenario == "PRIVATE_ODP" {
		var valid bool
		username, valid = store.PrivateODPCommandIdentity(source.Username, source.TenantName, source.ClusterName)
		if !valid {
			return commandgen.Result{}, source, node, errors.New("旁路导入连接身份无效")
		}
	}
	if c.Scenario == "OBSERVER_DIRECT" {
		var valid bool
		username, valid = store.ObserverDirectCommandIdentity(source.Username, source.TenantName)
		if !valid || source.ClusterName != "" {
			return commandgen.Result{}, source, node, errors.New("旁路直连身份无效")
		}
	}
	fields := []commandgen.FieldInput{
		{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: source.Host}},
		{Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: int64(source.Port)}},
		{Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: username}},
		{Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: c.Database}},
		{Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: ref.CredentialID, Revision: ref.Revision}}},
	}
	fields = append(fields, directload.Fields(c)...)
	if c.TruncateTable {
		fields = append(fields, commandgen.FieldInput{Name: "--yes", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	connectionKind := commandgen.ConnectionKind(source.ConnectionKind)
	if c.Scenario == "PRIVATE_ODP" || c.Scenario == "OBSERVER_DIRECT" {
		if source.TenantName == "" {
			return commandgen.Result{}, source, node, errors.New("旁路导入租户事实缺失")
		}
		fields = append(fields, commandgen.FieldInput{Name: "--tenant", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: source.TenantName}})
	}
	if c.Scenario == "PRIVATE_ODP" {
		if source.ClusterName == "" {
			return commandgen.Result{}, source, node, errors.New("旁路导入集群事实缺失")
		}
		fields = append(fields, commandgen.FieldInput{Name: "--cluster", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: source.ClusterName}})
	}
	result, err := s.directLoadGenerator.Generate(commandgen.Request{Tool: "OBLOADER", ToolVersion: directload.ToolVersion, MetadataVersion: directload.MetadataVersion, CapabilityVersion: directload.CapabilityVersion, ConnectionKind: connectionKind, DataSourceFactVersion: fmt.Sprintf("ds-rev-%d", source.Revision), NodeFactVersion: node.FactsVersion, TargetPlatform: node.Platform, Fields: fields})
	return result, source, node, err
}

func (s *Server) previewDirectLoad(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	var body struct{}
	if !decodeBrowserJSON(w, r, &body) {
		return
	}
	revision, _, ok := s.directLoadWriteGuard(w, r, true)
	if !ok {
		return
	}
	item, ok := s.loadDirectLoad(w, r, p, id)
	if !ok {
		return
	}
	if item.Revision != revision {
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已变更，请重新读取", false)
		return
	}
	config, err := directload.Decode(item.ConfigJSON)
	if err != nil {
		writeError(w, 422, "DIRECT_LOAD_CONFIG_INVALID", "旁路导入配置无效", false)
		return
	}
	result, _, _, err := s.generateDirectLoad(r.Context(), config)
	if err != nil {
		writeError(w, 422, "DIRECT_LOAD_CONFIG_INVALID", "请先补齐并保存旁路导入配置", false)
		return
	}
	if result.ConfigFingerprint != item.ConfigFingerprint {
		writeError(w, 412, "DRAFT_BINDING_CHANGED", "数据源或节点事实已变更，请重新保存", false)
		return
	}
	writeJSON(w, 200, map[string]any{"redactedCommand": result.RedactedCommand, "configFingerprint": result.ConfigFingerprint, "metadataVersion": result.MetadataVersion, "toolVersion": result.ToolVersion, "capabilityVersion": result.CapabilityVersion, "executionEnabled": s.directLoadExecutionEnabled})
}
