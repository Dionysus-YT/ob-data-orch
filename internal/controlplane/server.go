package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

type Server struct {
	build        buildinfo.Info
	provider     identity.Provider
	authorizer   identity.Authorizer
	roles        identity.RoleAuthorizer
	dataSource   DataSourceReader
	creator      DataSourceCreator
	stateChanger DataSourceStateChanger
	updater      DataSourceUpdater
	credentials  DataSourceCredentialReferenceReader
	drafts       ExportDraftStore
	prechecks    ExportPrecheckStore
	tasks        ExportTaskStore
	nodes        ExecutionNodeReader
	generator    ExportCommandGenerator
	precheckTTL  time.Duration
	coordinator  *agentstate.Coordinator
	encryptor    CredentialEncryptor
	csrf         CSRFValidator
	keyID        string
}

// DataSourceReader exposes only a non-sensitive data-source projection. The
// write API is deferred until credential encryption and audit are transactional.
type DataSourceReader interface {
	ListDataSourceSummaries(context.Context) ([]store.DataSourceSummary, error)
	GetDataSourceSummary(context.Context, string) (store.DataSourceSummary, error)
}

// DataSourceCreator is the transaction boundary that stores a source together
// with its encrypted credential, audit event, and idempotency record.
type DataSourceCreator interface {
	CreateDataSource(context.Context, store.DataSourceCreate) (store.DataSourceCreateResult, error)
}

// DataSourceStateChanger 将启停动作收敛为独立的原子存储边界。
// HTTP 层不能自行更新状态或绕过审计。
type DataSourceStateChanger interface {
	ChangeDataSourceState(context.Context, store.DataSourceStateChange) (store.DataSourceStateChangeResult, error)
}

// DataSourceUpdater 提供包含审计与可选凭据轮换的原子更新边界。
type DataSourceUpdater interface {
	UpdateDataSource(context.Context, store.DataSourceUpdate) (store.DataSourceUpdateResult, error)
}

// DataSourceCredentialReferenceReader 只向已授权写路径提供当前引用。
// 返回值不得进入响应、日志或错误信息。
type DataSourceCredentialReferenceReader interface {
	GetDataSourceCredentialReference(context.Context, string) (store.DataSourceCredentialReference, error)
}

// ExportDraftStore 组合草稿创建、读取和乐观锁更新能力。
type ExportDraftStore interface {
	CreateExportDraft(context.Context, store.ExportDraftCreate) (store.ExportDraftCreateResult, error)
	GetExportDraft(context.Context, string) (store.ExportDraft, error)
	UpdateDraft(context.Context, store.DraftUpdate) (int64, error)
}

// ExecutionNodeFact 是命令生成需要的最小、非敏感节点事实。
type ExecutionNodeFact struct {
	NodeID        string
	Platform      commandgen.Platform
	FactsVersion  string
	FactsRevision int64
}

// ExecutionNodeReader 只读取选中节点的生成事实，不执行任何节点操作。
type ExecutionNodeReader interface {
	GetExecutionNodeFact(context.Context, string) (ExecutionNodeFact, error)
}

// ExportCommandGenerator 约束预览与提交共用确定性命令生成器。
type ExportCommandGenerator interface {
	Generate(commandgen.Request) (commandgen.Result, error)
}

// ExportPrecheckStore 负责保存预检查绑定与读取其安全状态投影。
type ExportPrecheckStore interface {
	CreatePrecheck(context.Context, store.PrecheckCreate) (store.PrecheckCreateResult, error)
	GetPrecheckRun(context.Context, string) (store.PrecheckRun, error)
	CompletePrecheck(context.Context, store.PrecheckCompletion) error
}

// ExportTaskStore 只暴露任务冻结、预检查读取和安全详情投影。
// HTTP 层不能自行绕过预检查条件拼装任务记录。
type ExportTaskStore interface {
	GetPrecheckRun(context.Context, string) (store.PrecheckRun, error)
	SubmitTaskIdempotent(context.Context, store.TaskSubmission, string, string) (store.TaskSubmissionResult, error)
	GetTaskSummary(context.Context, string) (store.TaskSummary, error)
}

// CredentialEncryptor keeps raw root-key material out of the HTTP package.
type CredentialEncryptor interface {
	Encrypt(string, credential.Reference, []byte) (credential.Envelope, error)
}

// CSRFValidator is intentionally separate from authentication because a valid
// browser session alone must not authorize a state-changing request.
type CSRFValidator interface {
	ValidateCSRF(*http.Request) error
}

// Dependencies make the HTTP boundary testable without creating a runtime
// authentication bypass. Nil production dependencies continue to fail closed.
type Dependencies struct {
	Identity        identity.Provider
	Authorizer      identity.Authorizer
	Roles           identity.RoleAuthorizer
	DataSources     DataSourceReader
	Creator         DataSourceCreator
	StateChanger    DataSourceStateChanger
	Updater         DataSourceUpdater
	CredentialRefs  DataSourceCredentialReferenceReader
	Drafts          ExportDraftStore
	Prechecks       ExportPrecheckStore
	Tasks           ExportTaskStore
	Nodes           ExecutionNodeReader
	Generator       ExportCommandGenerator
	PrecheckTTL     time.Duration
	Coordinator     *agentstate.Coordinator
	Encryptor       CredentialEncryptor
	CSRF            CSRFValidator
	CredentialKeyID string
}

func NewHandler(build buildinfo.Info) http.Handler {
	return NewHandlerWithDependencies(build, Dependencies{})
}

// NewHandlerWithIdentity exists for integration tests and future deployment
// adapters. Passing nil is the production default until a real identity source
// is selected and configured.
func NewHandlerWithIdentity(build buildinfo.Info, provider identity.Provider) http.Handler {
	return NewHandlerWithDependencies(build, Dependencies{Identity: provider})
}

// NewHandlerWithDependencies is used by the composition root and integration
// tests. Supplying an identity provider alone does not grant access to data.
func NewHandlerWithDependencies(build buildinfo.Info, dependencies Dependencies) http.Handler {
	server := &Server{build: build, provider: dependencies.Identity, authorizer: dependencies.Authorizer, roles: dependencies.Roles, dataSource: dependencies.DataSources, creator: dependencies.Creator, stateChanger: dependencies.StateChanger, updater: dependencies.Updater, credentials: dependencies.CredentialRefs, drafts: dependencies.Drafts, prechecks: dependencies.Prechecks, tasks: dependencies.Tasks, nodes: dependencies.Nodes, generator: dependencies.Generator, precheckTTL: dependencies.PrecheckTTL, coordinator: dependencies.Coordinator, encryptor: dependencies.Encryptor, csrf: dependencies.CSRF, keyID: dependencies.CredentialKeyID}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /version", server.version)
	// DEV-04 starts with executable API-domain boundaries. No request may be
	// treated as authenticated until the deployment identity contract exists.
	if server.provider == nil {
		mux.HandleFunc("/api/", server.browserUnavailable)
		mux.HandleFunc("/agent/", server.agentUnavailable)
	} else {
		mux.HandleFunc("/api/", server.browserAuthenticated)
		mux.HandleFunc("/agent/", server.agentAuthenticated)
	}
	mux.HandleFunc("/", notFound)
	return securityHeaders(mux)
}

func (s *Server) browserAuthenticated(w http.ResponseWriter, r *http.Request) {
	principal, err := s.provider.AuthenticateBrowser(r)
	if err != nil || identity.Validate(principal, identity.BrowserPrincipal) != nil {
		writeError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "身份认证失败", false)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/data-sources" {
		s.listDataSources(w, r, principal)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/data-sources" {
		s.createDataSource(w, r, principal)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/export-drafts" {
		s.createExportDraft(w, r, principal)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/export-drafts/") {
		if draftID, action, ok := parseExportDraftAction(r.URL.Path); ok {
			switch {
			case r.Method == http.MethodPost && action == "precheck":
				s.createExportPrecheck(w, r, principal, draftID)
				return
			case r.Method == http.MethodPost && action == "preview-command":
				s.previewExportDraft(w, r, principal, draftID)
				return
			case r.Method == http.MethodPost && action == "submit":
				s.submitExportDraft(w, r, principal, draftID)
				return
			case r.Method == http.MethodPatch && action == "":
				s.updateExportDraft(w, r, principal, draftID)
				return
			case r.Method == http.MethodGet && action == "":
				s.getExportDraft(w, r, principal, draftID)
				return
			}
		}
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
		if taskID != "" && !strings.Contains(taskID, "/") {
			s.getTask(w, r, principal, taskID)
			return
		}
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/prechecks/") {
		precheckID := strings.TrimPrefix(r.URL.Path, "/api/v1/prechecks/")
		if precheckID != "" && !strings.Contains(precheckID, "/") {
			s.getExportPrecheck(w, r, principal, precheckID)
			return
		}
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		if dataSourceID, targetState, ok := parseDataSourceStateAction(r.URL.Path); ok {
			s.changeDataSourceState(w, r, principal, dataSourceID, targetState)
			return
		}
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		dataSourceID := strings.TrimPrefix(r.URL.Path, "/api/v1/data-sources/")
		if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
			s.changeDataSourceState(w, r, principal, dataSourceID, "ARCHIVED")
			return
		}
	}
	if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		dataSourceID := strings.TrimPrefix(r.URL.Path, "/api/v1/data-sources/")
		if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
			s.updateDataSource(w, r, principal, dataSourceID)
			return
		}
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		dataSourceID := strings.TrimPrefix(r.URL.Path, "/api/v1/data-sources/")
		if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
			s.getDataSource(w, r, principal, dataSourceID)
			return
		}
	}
	notFound(w, r)
}

// parseDataSourceStateAction 只识别已登记的启停动作，避免把路径后缀解释成通用命令。
func parseDataSourceStateAction(path string) (string, string, bool) {
	const prefix = "/api/v1/data-sources/"
	for suffix, targetState := range map[string]string{":disable": "DISABLED", ":enable": "ENABLED"} {
		if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) {
			dataSourceID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
			if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
				return dataSourceID, targetState, true
			}
		}
	}
	return "", "", false
}

type dataSourceCreateRequest struct {
	DisplayName       string `json:"displayName"`
	Environment       string `json:"environment"`
	ConnectionKind    string `json:"connectionKind"`
	CompatibilityMode string `json:"compatibilityMode"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Username          string `json:"username"`
	DefaultDatabase   string `json:"defaultDatabase"`
	Password          string `json:"password"`
}

// dataSourceUpdateRequest 使用指针与 RawMessage 保留 PATCH 的字段存在语义。
// 密码仅用于本次加密，不能被写回请求结构或安全响应。
type dataSourceUpdateRequest struct {
	DisplayName       *string         `json:"displayName"`
	Environment       *string         `json:"environment"`
	ConnectionKind    *string         `json:"connectionKind"`
	CompatibilityMode *string         `json:"compatibilityMode"`
	Host              *string         `json:"host"`
	Port              *int            `json:"port"`
	Username          *string         `json:"username"`
	DefaultDatabase   json.RawMessage `json:"defaultDatabase"`
	Password          *string         `json:"password"`
}

// exportDraftWriteRequest 固定首条切片的单表 CSV 输入，不接收任意参数文本。
type exportDraftWriteRequest struct {
	DataSourceID string `json:"dataSourceId"`
	NodeID       string `json:"nodeId"`
	Database     string `json:"database"`
	Table        string `json:"table"`
	Format       string `json:"format"`
	FilePath     string `json:"filePath"`
}

// createDataSource never renders or persists the password itself. Its
// idempotency digest deliberately records only that a password was supplied,
// rather than retaining a password-derived hash.
func (s *Server) createDataSource(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.creator == nil || s.encryptor == nil || s.csrf == nil || s.roles == nil || strings.TrimSpace(s.keyID) == "" {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源创建依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if identity.HasRole(r.Context(), s.roles, principal, identity.RoleDataSourceAdmin) != nil {
		writeError(w, http.StatusForbidden, "ROLE_REQUIRED", "当前身份不具备数据源管理能力", false)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	var request dataSourceCreateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	password := []byte(request.Password)
	request.Password = ""
	defer credential.Zero(password)
	if len(password) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "PASSWORD_REQUIRED", "密码不能为空", false)
		return
	}
	dataSourceID, credentialID, err := newOpaqueID(), newOpaqueID(), error(nil)
	if dataSourceID == "" || credentialID == "" {
		err = errors.New("generate identifier")
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	envelope, err := s.encryptor.Encrypt(s.keyID, credential.Reference{CredentialID: credentialID, Revision: 1, SecretType: credential.DatabasePassword, DataSourceID: dataSourceID}, password)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_ENCRYPTION_UNAVAILABLE", "凭据安全上下文不可用", true)
		return
	}
	defer credential.Zero(envelope.Nonce)
	defer credential.Zero(envelope.Ciphertext)
	result, err := s.creator.CreateDataSource(r.Context(), store.DataSourceCreate{
		DataSourceID: dataSourceID, CredentialID: credentialID, CreatorSubjectID: principal.ID,
		DisplayName: request.DisplayName, NormalizedName: normalizeName(request.DisplayName),
		Environment: request.Environment, ConnectionKind: request.ConnectionKind, CompatibilityMode: request.CompatibilityMode,
		Host: request.Host, Port: request.Port, Username: request.Username, DefaultDatabase: request.DefaultDatabase,
		KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext,
		RequestID: requestID(), IdempotencyKey: idempotencyKey, RequestDigest: createRequestDigest(request), CreatedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "DATA_SOURCE_CREATE_REJECTED", "数据源字段不符合要求", false)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID(), "id": result.DataSourceID, "replayed": result.Replayed})
}

// updateDataSource 先完成对象授权与当前安全摘要读取，再合并 PATCH 字段。
// 密码轮换产生新的加密 revision；缺失密码严格表示保持现有凭据不变。
func (s *Server) updateDataSource(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID string) {
	if s.dataSource == nil || s.updater == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源更新依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	expectedRevision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的数据版本号", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceWrite, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	current, err := s.dataSource.GetDataSourceSummary(r.Context(), dataSourceID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_QUERY_UNAVAILABLE", "数据源暂时不可用", true)
		return
	}
	var request dataSourceUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	if !request.hasChanges() {
		writeError(w, http.StatusUnprocessableEntity, "UPDATE_EMPTY", "至少需要更新一个数据源字段", false)
		return
	}
	merged, err := request.merge(current)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "DATA_SOURCE_UPDATE_REJECTED", "数据源字段不符合要求", false)
		return
	}
	update := store.DataSourceUpdate{
		DataSourceID: dataSourceID, ActorSubjectID: principal.ID, ExpectedRevision: expectedRevision,
		DisplayName: merged.DisplayName, NormalizedName: normalizeName(merged.DisplayName), Environment: merged.Environment,
		ConnectionKind: merged.ConnectionKind, CompatibilityMode: merged.CompatibilityMode, Host: merged.Host,
		Port: merged.Port, Username: merged.Username, DefaultDatabase: merged.DefaultDatabase,
		RequestID: requestID(), UpdatedAt: time.Now().UTC(),
	}
	if request.Password != nil {
		if s.credentials == nil || s.encryptor == nil || strings.TrimSpace(s.keyID) == "" {
			writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_UPDATE_NOT_CONFIGURED", "当前环境尚未配置凭据轮换依赖", false)
			return
		}
		password := []byte(*request.Password)
		*request.Password = ""
		defer credential.Zero(password)
		if len(password) == 0 {
			writeError(w, http.StatusUnprocessableEntity, "PASSWORD_REQUIRED", "密码不能为空", false)
			return
		}
		reference, refErr := s.credentials.GetDataSourceCredentialReference(r.Context(), dataSourceID)
		if errors.Is(refErr, store.ErrDataSourceNotFound) {
			notFound(w, r)
			return
		}
		if refErr != nil {
			writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_REFERENCE_UNAVAILABLE", "凭据引用暂时不可用", true)
			return
		}
		envelope, encryptErr := s.encryptor.Encrypt(s.keyID, credential.Reference{CredentialID: reference.CredentialID, Revision: reference.Revision + 1, SecretType: credential.DatabasePassword, DataSourceID: dataSourceID}, password)
		if encryptErr != nil {
			writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_ENCRYPTION_UNAVAILABLE", "凭据安全上下文不可用", true)
			return
		}
		defer credential.Zero(envelope.Nonce)
		defer credential.Zero(envelope.Ciphertext)
		update.Password = &store.EncryptedDataSourcePassword{CredentialID: envelope.Reference.CredentialID, Revision: envelope.Reference.Revision, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext}
	}
	result, err := s.updater.UpdateDataSource(r.Context(), update)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "DATA_SOURCE_REVISION_CONFLICT", "数据源已发生变化，请刷新后重试", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "DATA_SOURCE_UPDATE_REJECTED", "数据源字段不符合要求", false)
		return
	}
	current.DisplayName, current.Environment, current.ConnectionKind, current.CompatibilityMode = merged.DisplayName, merged.Environment, merged.ConnectionKind, merged.CompatibilityMode
	current.Host, current.Port, current.Username, current.DefaultDatabase = merged.Host, merged.Port, merged.Username, merged.DefaultDatabase
	current.Revision, current.CredentialRevision = result.Revision, result.CredentialRevision
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "item": newDataSourceResponse(current)})
}

func (r dataSourceUpdateRequest) hasChanges() bool {
	return r.DisplayName != nil || r.Environment != nil || r.ConnectionKind != nil || r.CompatibilityMode != nil || r.Host != nil || r.Port != nil || r.Username != nil || r.DefaultDatabase != nil || r.Password != nil
}

func (r dataSourceUpdateRequest) merge(current store.DataSourceSummary) (store.DataSourceSummary, error) {
	merged := current
	if r.DisplayName != nil {
		merged.DisplayName = *r.DisplayName
	}
	if r.Environment != nil {
		merged.Environment = *r.Environment
	}
	if r.ConnectionKind != nil {
		merged.ConnectionKind = *r.ConnectionKind
	}
	if r.CompatibilityMode != nil {
		merged.CompatibilityMode = *r.CompatibilityMode
	}
	if r.Host != nil {
		merged.Host = *r.Host
	}
	if r.Port != nil {
		merged.Port = *r.Port
	}
	if r.Username != nil {
		merged.Username = *r.Username
	}
	if r.DefaultDatabase != nil {
		var value *string
		if err := json.Unmarshal(r.DefaultDatabase, &value); err != nil {
			return store.DataSourceSummary{}, err
		}
		if value == nil {
			merged.DefaultDatabase = ""
		} else {
			merged.DefaultDatabase = *value
		}
	}
	return merged, nil
}

// parseExportDraftAction 仅识别固定草稿资源和已登记的命令预览动作。
func parseExportDraftAction(path string) (string, string, bool) {
	const prefix = "/api/v1/export-drafts/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	value := strings.TrimPrefix(path, prefix)
	for suffix, action := range map[string]string{":preview-command": "preview-command", ":precheck": "precheck", ":submit": "submit"} {
		if strings.HasSuffix(value, suffix) {
			id := strings.TrimSuffix(value, suffix)
			return id, action, id != "" && !strings.Contains(id, "/")
		}
	}
	return value, "", value != "" && !strings.Contains(value, "/")
}

// createExportDraft 在入库前使用同一命令生成器校验单表 CSV 配置。
func (s *Server) createExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.drafts == nil || s.dataSource == nil || s.nodes == nil || s.credentials == nil || s.generator == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置导出草稿依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	request, ok := decodeExportDraftRequest(w, r)
	if !ok {
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, request.DataSourceID) != nil || identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeUse, request.NodeID) != nil {
		notFound(w, r)
		return
	}
	preview, source, node, err := s.generateExportDraft(r.Context(), request)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	configJSON, _ := json.Marshal(request)
	draftID := newOpaqueID()
	if draftID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	result, err := s.drafts.CreateExportDraft(r.Context(), store.ExportDraftCreate{ExportDraft: store.ExportDraft{
		DraftID: draftID, OwnerSubjectID: principal.ID, DataSourceID: source.DataSourceID, NodeID: node.NodeID, ToolVersion: preview.ToolVersion,
		MetadataVersion: preview.MetadataVersion, CapabilityVersion: preview.CapabilityVersion, ConfigJSON: string(configJSON), ConfigFingerprint: preview.ConfigFingerprint,
		InvalidationJSON: `{}`, CreatedAt: now, UpdatedAt: now,
	}, RequestID: requestID(), IdempotencyKey: key, RequestDigest: exportDraftDigest(request)})
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID(), "id": result.DraftID, "replayed": result.Replayed})
}

// getExportDraft 仅允许草稿所有者读取，避免新增未确认的草稿共享规则。
func (s *Server) getExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	draft, ok := s.loadOwnedDraft(w, r, principal, draftID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "item": draftResponse(draft)})
}

// updateExportDraft 用乐观锁替换整个固定切片配置，并重新计算指纹。
func (s *Server) updateExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	if s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置草稿更新依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	expected, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的草稿版本号", false)
		return
	}
	draft, ok := s.loadOwnedDraft(w, r, principal, draftID)
	if !ok {
		return
	}
	request, ok := decodeExportDraftRequest(w, r)
	if !ok {
		return
	}
	if request.DataSourceID != draft.DataSourceID || request.NodeID != draft.NodeID {
		writeError(w, http.StatusUnprocessableEntity, "DRAFT_BINDING_IMMUTABLE", "首条切片草稿不能通过更新更换数据源或执行节点", false)
		return
	}
	preview, _, _, err := s.generateExportDraft(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	configJSON, _ := json.Marshal(request)
	newRevision, err := s.drafts.UpdateDraft(r.Context(), store.DraftUpdate{DraftID: draftID, ExpectedRevision: expected, ConfigJSON: string(configJSON), ConfigFingerprint: preview.ConfigFingerprint, InvalidationJSON: `{}`, UpdatedAt: time.Now().UTC()})
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "DRAFT_REVISION_CONFLICT", "草稿已发生变化，请刷新后重试", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	draft.Revision, draft.ConfigJSON, draft.ConfigFingerprint = newRevision, string(configJSON), preview.ConfigFingerprint
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "item": draftResponse(draft)})
}

// previewExportDraft 只重算脱敏命令，不解析凭据或创建任何执行任务。
func (s *Server) previewExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	if s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置命令预览依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	expected, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的草稿版本号", false)
		return
	}
	draft, ok := s.loadOwnedDraft(w, r, principal, draftID)
	if !ok {
		return
	}
	if draft.Revision != expected {
		writeError(w, http.StatusPreconditionFailed, "DRAFT_REVISION_CONFLICT", "草稿已发生变化，请刷新后重试", false)
		return
	}
	var request exportDraftWriteRequest
	if err := json.Unmarshal([]byte(draft.ConfigJSON), &request); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DRAFT_CONFIGURATION_UNAVAILABLE", "草稿配置暂时不可用", true)
		return
	}
	preview, _, _, err := s.generateExportDraft(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "command": preview.RedactedCommand, "argvTemplate": preview.ArgvTemplate, "configFingerprint": preview.ConfigFingerprint, "tokenEvidence": preview.TokenEvidence, "secretSourceSummary": preview.SecretSourceSummary})
}

// createExportPrecheck 固定当前草稿与凭据版本，之后的 Agent 只能领取该绑定。
// 有效期来自部署依赖，而不是由浏览器、Agent 或请求体提供。
func (s *Server) createExportPrecheck(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	if s.prechecks == nil || s.coordinator == nil || s.csrf == nil || s.precheckTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置预检查依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	expected, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的草稿版本号", false)
		return
	}
	draft, ok := s.loadOwnedDraft(w, r, principal, draftID)
	if !ok {
		return
	}
	if draft.Revision != expected {
		writeError(w, http.StatusPreconditionFailed, "DRAFT_REVISION_CONFLICT", "草稿已发生变化，请刷新后重试", false)
		return
	}
	var request exportDraftWriteRequest
	if err := json.Unmarshal([]byte(draft.ConfigJSON), &request); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DRAFT_CONFIGURATION_UNAVAILABLE", "草稿配置暂时不可用", true)
		return
	}
	preview, _, node, err := s.generateExportDraft(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	credentialReference, err := s.credentials.GetDataSourceCredentialReference(r.Context(), draft.DataSourceID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_REFERENCE_UNAVAILABLE", "凭据引用暂时不可用", true)
		return
	}
	precheckID := newOpaqueID()
	if precheckID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	if node.FactsRevision < 1 || s.coordinator.SchedulePrecheck(agentstate.PrecheckBinding{PrecheckID: precheckID, NodeID: draft.NodeID, DraftRevision: draft.Revision, ConfigFingerprint: preview.ConfigFingerprint, CredentialRevision: credentialReference.Revision, NodeFactsVersion: node.FactsRevision}) != nil {
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_SCHEDULING_UNAVAILABLE", "预检查暂时无法排队", true)
		return
	}
	result, err := s.prechecks.CreatePrecheck(r.Context(), store.PrecheckCreate{PrecheckRun: store.PrecheckRun{
		PrecheckID: precheckID, DraftID: draft.DraftID, DraftRevision: draft.Revision, ConfigFingerprint: preview.ConfigFingerprint,
		DataSourceID: draft.DataSourceID, CredentialID: credentialReference.CredentialID, CredentialRevision: credentialReference.Revision,
		NodeID: draft.NodeID, CreatedAt: now, ValidUntil: now.Add(s.precheckTTL),
	}, CreatorSubjectID: principal.ID, RequestID: requestID(), IdempotencyKey: key, RequestDigest: precheckDigest(draft, preview.ConfigFingerprint)})
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if errors.Is(err, store.ErrPrecheckInvalid) {
		writeError(w, http.StatusUnprocessableEntity, "PRECHECK_BINDING_INVALID", "预检查绑定已失效，请刷新草稿后重试", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_CREATE_UNAVAILABLE", "预检查暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"requestId": requestID(), "id": result.PrecheckID, "replayed": result.Replayed})
}

// getExportPrecheck 通过预检查关联的草稿所有者控制可见性。
func (s *Server) getExportPrecheck(w http.ResponseWriter, r *http.Request, principal identity.Principal, precheckID string) {
	if s.prechecks == nil || s.drafts == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置预检查依赖", false)
		return
	}
	run, err := s.prechecks.GetPrecheckRun(r.Context(), precheckID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_QUERY_UNAVAILABLE", "预检查暂时不可用", true)
		return
	}
	draft, err := s.drafts.GetExportDraft(r.Context(), run.DraftID)
	if errors.Is(err, store.ErrDataSourceNotFound) || (err == nil && draft.OwnerSubjectID != principal.ID) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_QUERY_UNAVAILABLE", "预检查暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "item": map[string]any{
		"id": run.PrecheckID, "draftId": run.DraftID, "draftRevision": run.DraftRevision, "configFingerprint": run.ConfigFingerprint,
		"nodeId": run.NodeID, "status": run.Status, "integrityStatus": run.IntegrityStatus, "validUntil": run.ValidUntil.Format(time.RFC3339Nano),
	}})
}

func (s *Server) loadOwnedDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) (store.ExportDraft, bool) {
	if s.drafts == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置导出草稿依赖", false)
		return store.ExportDraft{}, false
	}
	draft, err := s.drafts.GetExportDraft(r.Context(), draftID)
	if errors.Is(err, store.ErrDataSourceNotFound) || (err == nil && draft.OwnerSubjectID != principal.ID) {
		notFound(w, r)
		return store.ExportDraft{}, false
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_DRAFT_QUERY_UNAVAILABLE", "导出草稿暂时不可用", true)
		return store.ExportDraft{}, false
	}
	return draft, true
}

func decodeExportDraftRequest(w http.ResponseWriter, r *http.Request) (exportDraftWriteRequest, bool) {
	var request exportDraftWriteRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return exportDraftWriteRequest{}, false
	}
	return request, true
}

// decodeBrowserJSON 统一约束非草稿写请求的体积和字段集合。
func decodeBrowserJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return false
	}
	return true
}

func (s *Server) generateExportDraft(ctx context.Context, request exportDraftWriteRequest) (commandgen.Result, store.DataSourceSummary, ExecutionNodeFact, error) {
	if request.DataSourceID == "" || request.NodeID == "" || request.Database == "" || request.Table == "" || request.Format != "CSV" || request.FilePath == "" {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("export draft fields are incomplete")
	}
	source, err := s.dataSource.GetDataSourceSummary(ctx, request.DataSourceID)
	if err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	node, err := s.nodes.GetExecutionNodeFact(ctx, request.NodeID)
	if err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	credentialReference, err := s.credentials.GetDataSourceCredentialReference(ctx, request.DataSourceID)
	if err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	result, err := s.generator.Generate(commandgen.Request{Tool: "OBDUMPER", ToolVersion: "4.3.5-RELEASE", MetadataVersion: "obdumper-4.3.5-slice-v2", CapabilityVersion: "export-direct-single-table-csv-v1", ConnectionKind: commandgen.ConnectionKind(source.ConnectionKind), DataSourceFactVersion: fmt.Sprintf("ds-rev-%d", source.Revision), NodeFactVersion: node.FactsVersion, TargetPlatform: node.Platform, Fields: []commandgen.FieldInput{
		{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: source.Host}}, {Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: int64(source.Port)}}, {Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: source.Username}}, {Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: credentialReference.CredentialID, Revision: credentialReference.Revision}}}, {Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: request.Database}}, {Name: "--table", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: request.Table}}, {Name: "--csv", Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}}, {Name: "--file-path", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: request.FilePath}},
	}})
	return result, source, node, err
}

func draftResponse(draft store.ExportDraft) map[string]any {
	return map[string]any{"id": draft.DraftID, "dataSourceId": draft.DataSourceID, "nodeId": draft.NodeID, "revision": draft.Revision, "toolVersion": draft.ToolVersion, "metadataVersion": draft.MetadataVersion, "capabilityVersion": draft.CapabilityVersion, "config": json.RawMessage(draft.ConfigJSON), "configFingerprint": draft.ConfigFingerprint, "invalidation": json.RawMessage(draft.InvalidationJSON)}
}

func exportDraftDigest(request exportDraftWriteRequest) string {
	raw, _ := json.Marshal(request)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func precheckDigest(draft store.ExportDraft, fingerprint string) string {
	raw, _ := json.Marshal(struct {
		DraftID     string `json:"draftId"`
		Revision    int64  `json:"revision"`
		Fingerprint string `json:"fingerprint"`
	}{DraftID: draft.DraftID, Revision: draft.Revision, Fingerprint: fingerprint})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func taskSubmitDigest(draft store.ExportDraft, precheckID, fingerprint string) string {
	raw, _ := json.Marshal(struct {
		DraftID     string `json:"draftId"`
		Revision    int64  `json:"revision"`
		PrecheckID  string `json:"precheckId"`
		Fingerprint string `json:"fingerprint"`
	}{DraftID: draft.DraftID, Revision: draft.Revision, PrecheckID: precheckID, Fingerprint: fingerprint})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// changeDataSourceState 执行幂等的启停动作。对象授权先于存储读取，
// 因此无权主体无法通过状态接口枚举数据源是否存在。
func (s *Server) changeDataSourceState(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID, targetState string) {
	if s.stateChanger == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源状态依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	expectedRevision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的数据版本号", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceWrite, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	result, err := s.stateChanger.ChangeDataSourceState(r.Context(), store.DataSourceStateChange{
		DataSourceID: dataSourceID, ActorSubjectID: principal.ID, TargetState: targetState, ExpectedRevision: expectedRevision,
		RequestID: requestID(), ChangedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "DATA_SOURCE_STATE_CONFLICT", "数据源状态已发生变化，请刷新后重试", true)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_STATE_UNAVAILABLE", "数据源状态暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(), "id": dataSourceID, "state": result.State,
		"revision": result.Revision, "replayed": result.Replayed,
	})
}

// exportTaskSubmitRequest 只接收已创建预检查的标识，不能从浏览器接收命令或任务快照。
type exportTaskSubmitRequest struct {
	PrecheckID string `json:"precheckId"`
}

// submitExportDraft 将已通过的预检查与当前草稿重新核对后冻结为任务。
// G2 阶段的排队只进入内存协调器，绝不启动真实工具或网络连接。
func (s *Server) submitExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	if s.tasks == nil || s.coordinator == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置任务提交依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	expected, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的草稿版本号", false)
		return
	}
	draft, ok := s.loadOwnedDraft(w, r, principal, draftID)
	if !ok {
		return
	}
	if draft.Revision != expected {
		writeError(w, http.StatusPreconditionFailed, "DRAFT_REVISION_CONFLICT", "草稿已发生变化，请刷新后重试", false)
		return
	}
	var request exportTaskSubmitRequest
	if !decodeBrowserJSON(w, r, &request) {
		return
	}
	run, err := s.tasks.GetPrecheckRun(r.Context(), request.PrecheckID)
	if err != nil || run.DraftID != draft.DraftID || run.DraftRevision != draft.Revision || run.Status != "SUCCEEDED" || run.IntegrityStatus != "COMPLETE" || !run.ValidUntil.After(time.Now().UTC()) {
		writeError(w, http.StatusUnprocessableEntity, "PRECHECK_REQUIRED", "需要当前草稿的有效成功预检查", false)
		return
	}
	var draftRequest exportDraftWriteRequest
	if err := json.Unmarshal([]byte(draft.ConfigJSON), &draftRequest); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DRAFT_CONFIGURATION_UNAVAILABLE", "草稿配置暂时不可用", true)
		return
	}
	preview, _, _, err := s.generateExportDraft(r.Context(), draftRequest)
	if err != nil || preview.ConfigFingerprint != run.ConfigFingerprint {
		writeError(w, http.StatusUnprocessableEntity, "PRECHECK_REQUIRED", "草稿或预检查已失效，请重新预检查", false)
		return
	}
	argv, err := json.Marshal(preview.ArgvTemplate)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_SUBMISSION_UNAVAILABLE", "任务暂时无法提交", true)
		return
	}
	taskID := newOpaqueID()
	if taskID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	result, err := s.tasks.SubmitTaskIdempotent(r.Context(), store.TaskSubmission{
		TaskID: taskID, CreatorSubjectID: principal.ID, AuditActorID: principal.ID, DataSourceID: draft.DataSourceID, NodeID: draft.NodeID,
		PrecheckID: run.PrecheckID, CredentialID: run.CredentialID, CredentialRevision: run.CredentialRevision,
		ConfigFingerprint: preview.ConfigFingerprint, ToolVersion: preview.ToolVersion, MetadataVersion: preview.MetadataVersion,
		CapabilityVersion: preview.CapabilityVersion, SnapshotJSON: draft.ConfigJSON, PlannedArgvJSON: string(argv),
		PlannedCommandRedacted: preview.RedactedCommand, RequestID: requestID(), SubmittedAt: now,
	}, key, taskSubmitDigest(draft, run.PrecheckID, preview.ConfigFingerprint))
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if errors.Is(err, store.ErrPrecheckInvalid) {
		writeError(w, http.StatusUnprocessableEntity, "PRECHECK_REQUIRED", "预检查已失效，请重新执行", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_SUBMISSION_UNAVAILABLE", "任务暂时无法提交", true)
		return
	}
	if err := s.coordinator.Schedule(agentstate.TaskSchedule{TaskID: result.TaskID, NodeID: draft.NodeID}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_SCHEDULING_UNAVAILABLE", "任务已冻结但暂时无法进入合成队列", true)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(), "id": result.TaskID, "replayed": result.Replayed, "state": "WAITING_SCHEDULE", "realExecutionEnabled": false})
}

// getTask 只允许任务创建者读取冻结的安全投影。
func (s *Server) getTask(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置任务查询依赖", false)
		return
	}
	summary, err := s.tasks.GetTaskSummary(r.Context(), taskID)
	if errors.Is(err, store.ErrDataSourceNotFound) || (err == nil && summary.CreatorSubjectID != principal.ID) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_QUERY_UNAVAILABLE", "任务暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "item": map[string]any{
		"id": summary.TaskID, "dataSourceId": summary.DataSourceID, "nodeId": summary.NodeID, "precheckId": summary.PrecheckID,
		"configFingerprint": summary.ConfigFingerprint, "toolVersion": summary.ToolVersion, "metadataVersion": summary.MetadataVersion,
		"capabilityVersion": summary.CapabilityVersion, "plannedCommand": summary.PlannedCommandRedacted, "state": summary.State,
		"executionId": summary.ExecutionID, "submittedAt": summary.SubmittedAt.Format(time.RFC3339Nano), "realExecutionEnabled": false,
	}})
}

// parseIfMatchRevision 只接受 API 契约规定的强版本格式，避免将弱 ETag、
// 通配符或客户端自定义文本误当成并发控制依据。
func parseIfMatchRevision(value string) (int64, bool) {
	if len(value) < 7 || !strings.HasPrefix(value, `"rev-`) || !strings.HasSuffix(value, `"`) {
		return 0, false
	}
	revision, err := strconv.ParseInt(value[5:len(value)-1], 10, 64)
	if err != nil || revision < 1 {
		return 0, false
	}
	return revision, true
}

func normalizeName(displayName string) string { return strings.ToLower(strings.TrimSpace(displayName)) }

func createRequestDigest(request dataSourceCreateRequest) string {
	// Password content is not incorporated; retaining a password hash would be
	// a new sensitive persistence surface. Presence still distinguishes omission.
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%s|%s|password=true", request.DisplayName, request.Environment, request.ConnectionKind, request.CompatibilityMode, request.Host, request.Port, request.Username, request.DefaultDatabase)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func newOpaqueID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

// getDataSource authorizes the requested ID before reading it, then maps both
// absence and an out-of-scope object to the same 404 response.
func (s *Server) getDataSource(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID string) {
	if s.dataSource == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源 API 依赖", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	summary, err := s.dataSource.GetDataSourceSummary(r.Context(), dataSourceID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_QUERY_UNAVAILABLE", "数据源暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "item": newDataSourceResponse(summary)})
}

func (s *Server) agentAuthenticated(w http.ResponseWriter, r *http.Request) {
	principal, err := s.provider.AuthenticateAgent(r)
	if err != nil || identity.Validate(principal, identity.AgentPrincipal) != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/agent/v1/prechecks:claim" {
		s.claimSyntheticPrecheck(w, r, principal)
		return
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/agent/v1/prechecks/") && strings.HasSuffix(r.URL.Path, ":complete") {
		precheckID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agent/v1/prechecks/"), ":complete")
		if precheckID != "" && !strings.Contains(precheckID, "/") {
			s.completeSyntheticPrecheck(w, r, principal, precheckID)
			return
		}
	}
	notFound(w, r)
}

// syntheticPrecheckClaimRequest 只包含固定预检查的领取标识，不能夹带命令或 SQL。
type syntheticPrecheckClaimRequest struct {
	RequestID  string `json:"requestId"`
	PrecheckID string `json:"precheckId"`
	NodeID     string `json:"nodeId"`
	LeaseID    string `json:"leaseId"`
}

// syntheticPrecheckCompletionRequest 只能上报完成布尔事实，不允许覆盖冻结绑定。
type syntheticPrecheckCompletionRequest struct {
	RequestID  string `json:"requestId"`
	LeaseID    string `json:"leaseId"`
	LeaseEpoch int64  `json:"leaseEpoch"`
	Succeeded  bool   `json:"succeeded"`
}

// claimSyntheticPrecheck 仅为 G2 合成 Agent 提供固定预检查租约。
func (s *Server) claimSyntheticPrecheck(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.coordinator == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_NOT_CONFIGURED", "当前环境尚未配置合成 Agent 协议", false)
		return
	}
	var request syntheticPrecheckClaimRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	grant, err := s.coordinator.ClaimPrecheck(agentstate.PrecheckClaimRequest{RequestID: request.RequestID, PrecheckID: request.PrecheckID, NodeID: request.NodeID, AgentID: principal.ID, LeaseID: request.LeaseID, LeaseTTL: s.precheckTTL})
	if errors.Is(err, agentstate.ErrClaimIneligible) || errors.Is(err, agentstate.ErrUnknownExecution) {
		writeError(w, http.StatusConflict, "PRECHECK_CLAIM_REJECTED", "当前 Agent 无可领取的预检查", true)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "grant": map[string]any{"precheckId": grant.PrecheckID, "leaseId": grant.LeaseID, "leaseEpoch": grant.LeaseEpoch, "expiresAt": grant.ExpiresAt.Format(time.RFC3339Nano), "binding": grant.Binding}})
}

// completeSyntheticPrecheck 先校验协调器中的租约和冻结绑定，再写入 SQLite 状态。
func (s *Server) completeSyntheticPrecheck(w http.ResponseWriter, r *http.Request, principal identity.Principal, precheckID string) {
	if s.coordinator == nil || s.prechecks == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_NOT_CONFIGURED", "当前环境尚未配置合成 Agent 协议", false)
		return
	}
	var request syntheticPrecheckCompletionRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	snapshot, err := s.coordinator.PrecheckSnapshot(precheckID)
	if err != nil || snapshot.AgentID != principal.ID {
		writeError(w, http.StatusConflict, "PRECHECK_LEASE_REJECTED", "预检查租约无效", false)
		return
	}
	completed, err := s.coordinator.CompletePrecheck(agentstate.PrecheckCompletion{RequestID: request.RequestID, PrecheckID: precheckID, LeaseID: request.LeaseID, LeaseEpoch: request.LeaseEpoch, Binding: snapshot.Binding, Complete: true, Succeeded: request.Succeeded})
	if err != nil {
		writeError(w, http.StatusConflict, "PRECHECK_LEASE_REJECTED", "预检查租约无效", false)
		return
	}
	if err := s.prechecks.CompletePrecheck(r.Context(), store.PrecheckCompletion{PrecheckID: precheckID, Succeeded: completed.State == agentstate.PrecheckSucceeded, IntegrityStatus: "COMPLETE", ResultJSON: `{"mode":"synthetic"}`, CompletedAt: time.Now().UTC()}); err != nil && !errors.Is(err, store.ErrPrecheckInvalid) {
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_COMPLETION_UNAVAILABLE", "预检查结果暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "status": string(completed.State)})
}

// decodeAgentJSON 对 Agent 协议同样限制大小和未知字段，避免测试适配放宽边界。
func decodeAgentJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return false
	}
	return true
}

// listDataSources filters each object before it reaches JSON. It never returns
// a total count, so unauthorized sources are not discoverable through the API.
func (s *Server) listDataSources(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.dataSource == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源 API 依赖", false)
		return
	}
	summaries, err := s.dataSource.ListDataSourceSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_QUERY_UNAVAILABLE", "数据源暂时不可用", true)
		return
	}
	items := make([]dataSourceResponse, 0, len(summaries))
	for _, summary := range summaries {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, summary.DataSourceID) != nil {
			continue
		}
		items = append(items, newDataSourceResponse(summary))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(), "items": items})
}

// dataSourceResponse keeps JSON camelCase and cannot inherit credential-only
// fields if the persistence model later grows additional columns.
type dataSourceResponse struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"displayName"`
	Environment        string `json:"environment"`
	ConnectionKind     string `json:"connectionKind"`
	CompatibilityMode  string `json:"compatibilityMode"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	DefaultDatabase    string `json:"defaultDatabase,omitempty"`
	State              string `json:"state"`
	Revision           int64  `json:"revision"`
	CredentialRevision int64  `json:"credentialRevision"`
	LastTestStatus     string `json:"lastTestStatus,omitempty"`
}

func newDataSourceResponse(summary store.DataSourceSummary) dataSourceResponse {
	return dataSourceResponse{
		ID: summary.DataSourceID, DisplayName: summary.DisplayName, Environment: summary.Environment,
		ConnectionKind: summary.ConnectionKind, CompatibilityMode: summary.CompatibilityMode,
		Host: summary.Host, Port: summary.Port, Username: summary.Username,
		DefaultDatabase: summary.DefaultDatabase, State: summary.State, Revision: summary.Revision,
		CredentialRevision: summary.CredentialRevision, LastTestStatus: summary.LastTestStatus,
	}
}

func (s *Server) browserUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusServiceUnavailable, "AUTHENTICATION_NOT_CONFIGURED", "当前环境尚未配置浏览器身份认证", false)
}

func (s *Server) agentUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusServiceUnavailable, "AGENT_AUTHENTICATION_NOT_CONFIGURED", "当前环境尚未配置 Agent 机器认证", false)
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "请求的资源不存在", false)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "ok",
		"stage":                "G1",
		"realExecutionEnabled": false,
	})
}

func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "ready",
		"scope":                "process-only",
		"stage":                "G1",
		"realExecutionEnabled": false,
	})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.build)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	writeJSON(w, status, map[string]any{
		"requestId":   requestID(),
		"code":        code,
		"message":     message,
		"retryable":   retryable,
		"fieldErrors": []any{},
		"safeDetails": map[string]any{},
	})
}

func requestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(bytes)
}
