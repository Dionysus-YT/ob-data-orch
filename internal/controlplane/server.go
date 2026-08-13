package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/outputpath"
	"ob-data-orch/internal/store"
)

type Server struct {
	build                          buildinfo.Info
	provider                       identity.Provider
	authorizer                     identity.Authorizer
	roles                          identity.RoleAuthorizer
	dataSource                     DataSourceReader
	creator                        DataSourceCreator
	stateChanger                   DataSourceStateChanger
	deleter                        DataSourceDeleter
	connectionTests                DataSourceConnectionTestStore
	updater                        DataSourceUpdater
	credentials                    DataSourceCredentialReferenceReader
	drafts                         ExportDraftStore
	prechecks                      ExportPrecheckStore
	tasks                          ExportTaskStore
	executions                     AgentExecutionStore
	logs                           *syntheticLogStore
	nodes                          ExecutionNodeReader
	nodeManagement                 ExecutionNodeManagementStore
	nodeEnvironment                ExecutionNodeEnvironmentStore
	nodeDeleter                    ExecutionNodeDeleter
	nodeCandidates                 ExecutionNodeCandidateReader
	agentProtocol                  AgentProtocolStore
	agentEnvironmentChecks         AgentExecutionNodeEnvironmentCheckStore
	agentPrechecks                 AgentPrecheckStore
	precheckSecrets                AgentPrecheckSecretStore
	agentConnectionTests           AgentDataSourceConnectionTestStore
	connectionTestSecrets          AgentDataSourceConnectionTestSecretStore
	agentJDBCConnectionTestEnabled bool
	realExecutionEnabled           bool
	generator                      ExportCommandGenerator
	// generalGenerator 服务泛化能力切片（full-csv/ddl/ddl-csv）；缺失时泛化请求失败关闭。
	generalGenerator   ExportCommandGenerator
	precheckTTL        time.Duration
	connectionTestTTL  time.Duration
	enrollmentTTL      time.Duration
	heartbeatTTL       time.Duration
	coordinator        *agentstate.Coordinator
	encryptor          CredentialEncryptor
	decryptor          CredentialDecryptor
	csrf               CSRFValidator
	keyID              string
	requestIDGenerator func() (string, error)
}

type requestIDResponseWriter struct {
	http.ResponseWriter
	value string
}

// RequestID 返回入口已经生成的请求标识，使同一 HTTP 请求的响应、审计和幂等记录可以稳定关联。
func (w *requestIDResponseWriter) RequestID() string {
	return w.value
}

// Unwrap 保留底层 ResponseWriter，避免请求标识包装破坏标准库对底层响应能力的识别。
func (w *requestIDResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
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

// DataSourceDeleter 将物理删除与历史保护归档收敛为同一个原子存储边界。
// HTTP 层只提交版本化删除意图，不能根据前端状态自行判断引用关系。
type DataSourceDeleter interface {
	DeleteOrArchiveDataSource(context.Context, store.DataSourceDeletion) (store.DataSourceDeletionResult, error)
}

// DataSourceUpdater 提供包含审计与可选凭据轮换的原子更新边界。
type DataSourceUpdater interface {
	UpdateDataSource(context.Context, store.DataSourceUpdate) (store.DataSourceUpdateResult, error)
}

// DataSourceConnectionTestStore 是节点绑定基础连接测试的浏览器持久化边界。
// 它只排队、冻结和读取安全投影；控制面不能借此直接连接数据库、启动 Java 或读取密码。
type DataSourceConnectionTestStore interface {
	RequestDataSourceConnectionTest(context.Context, store.DataSourceConnectionTestCreate) (store.DataSourceConnectionTestCreateResult, error)
	GetDataSourceConnectionTestRun(context.Context, string) (store.DataSourceConnectionTestRun, error)
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

// ExecutionNodeManagementStore 为浏览器节点管理提供受控的配置读写边界。
// 该接口不包含 Agent 关联、心跳、环境检查或任何远程操作，避免页面伪造机器事实。
type ExecutionNodeManagementStore interface {
	ListExecutionNodes(context.Context) ([]store.ExecutionNode, error)
	GetExecutionNode(context.Context, string) (store.ExecutionNode, error)
	CreateExecutionNode(context.Context, store.ExecutionNodeCreate) (store.ExecutionNodeCreateResult, error)
	UpdateExecutionNode(context.Context, store.ExecutionNodeUpdate) (int64, error)
}

// ExecutionNodeEnvironmentStore 收敛节点固定环境检查请求与启用状态转换。
// 它不向控制面提供远程连接、命令、文件或 Agent 身份写入能力。
type ExecutionNodeEnvironmentStore interface {
	RequestExecutionNodeEnvironmentCheck(context.Context, store.ExecutionNodeEnvironmentCheckRequest) (store.ExecutionNodeEnvironmentCheckRequestResult, error)
	EnableExecutionNode(context.Context, store.ExecutionNodeEnable) (int64, error)
}

// ExecutionNodeDeleter 将节点物理删除与历史保护归档收敛为同一个原子存储边界。
// HTTP 层只提交版本化删除意图，不能依据页面投影猜测节点是否被引用。
type ExecutionNodeDeleter interface {
	DeleteOrArchiveExecutionNode(context.Context, store.ExecutionNodeDeletion) (store.ExecutionNodeDeletionResult, error)
}

// ExecutionNodeCandidateReader 只提供草稿选择所需的最小节点安全投影。
// 它不提供目录、工具安装、Agent 凭据或任何可执行性结论。
type ExecutionNodeCandidateReader interface {
	ListExecutionNodeSummaries(context.Context) ([]store.ExecutionNodeSummary, error)
}

// AgentProtocolStore 是受认证机器身份、关联材料与当前心跳事实的唯一持久化边界。
// 它不接收浏览器身份、任务命令、数据库凭据、日志正文或任意机器操作。
type AgentProtocolStore interface {
	IssueAgentEnrollment(context.Context, store.AgentEnrollmentIssue) error
	ExchangeAgentEnrollment(context.Context, store.AgentEnrollmentExchange) (store.AgentEnrollmentResult, error)
	AuthenticateAgent(context.Context, []byte) (store.AgentIdentity, error)
	RecordAgentHeartbeat(context.Context, store.AgentHeartbeat) (int64, error)
}

// AgentExecutionNodeEnvironmentCheckStore 是 Agent 固定本机运行时检查的持久化边界。
// 它只返回待处理的不可解释标识并接收稳定结果码，绝不接收路径、命令、秘密或工具输出。
type AgentExecutionNodeEnvironmentCheckStore interface {
	GetPendingExecutionNodeEnvironmentCheck(context.Context, string, string) (store.PendingExecutionNodeEnvironmentCheck, bool, error)
	CompleteExecutionNodeEnvironmentCheck(context.Context, store.AgentExecutionNodeEnvironmentCheckCompletion) error
}

// AgentExecutionNodeEnvironmentCheckRefresher 是可选的心跳续接边界。
// 只有持久化实现具备该能力时，控制面才会在 Agent 事实版本变化后自动重新排队固定环境检查。
type AgentExecutionNodeEnvironmentCheckRefresher interface {
	EnsureCurrentExecutionNodeEnvironmentCheck(context.Context, store.ExecutionNodeEnvironmentCheckRefresh) (bool, error)
}

// AgentPrecheckStore 是受认证 Agent 固定预检查租约的持久化边界。
// 它只接受已冻结的预检查、固定检查结果和控制面时间，不能成为任意 Agent 操作通道。
type AgentPrecheckStore interface {
	ClaimNextPrecheck(context.Context, store.PrecheckClaimNext) (store.PrecheckLeaseGrant, bool, error)
	AcknowledgePrecheck(context.Context, store.PrecheckAcknowledgement) (store.PrecheckLeaseGrant, error)
	CompleteAgentPrecheck(context.Context, store.AgentPrecheckCompletion) (store.PrecheckCompletionResult, error)
}

// AgentPrecheckSecretStore 是固定预检查秘密槽位的持久化授权边界。
// HTTP 层只传递已认证的机器身份和受控租约字段，绝不读取 SQLite 密文或明文密码。
type AgentPrecheckSecretStore interface {
	ResolvePrecheckDatabaseConnection(context.Context, store.PrecheckSecretResolutionRequest) (store.EncryptedPrecheckDatabaseConnection, error)
	FinishPrecheckSecretResolution(context.Context, store.PrecheckSecretResolutionOutcome) error
}

// AgentDataSourceConnectionTestStore 是受认证 Agent 的独立基础连接测试租约边界。
// 它与 EXPORT_PREFLIGHT 分表、分状态和分回执，不能相互领取或复用完成结果。
type AgentDataSourceConnectionTestStore interface {
	ClaimNextDataSourceConnectionTest(context.Context, store.DataSourceConnectionTestClaimNext) (store.DataSourceConnectionTestLeaseGrant, bool, error)
	AcknowledgeDataSourceConnectionTest(context.Context, store.DataSourceConnectionTestAcknowledgement) (store.DataSourceConnectionTestLeaseGrant, error)
	CompleteAgentDataSourceConnectionTest(context.Context, store.AgentDataSourceConnectionTestCompletion) (store.DataSourceConnectionTestCompletionResult, error)
}

// AgentDataSourceConnectionTestSecretStore 只在已确认的基础连接测试租约内解析唯一数据库或 sys 槽位。
// 浏览器、普通数据源读取和预检查路径均不能调用它。
type AgentDataSourceConnectionTestSecretStore interface {
	ResolveDataSourceConnectionTestDatabaseConnection(context.Context, store.DataSourceConnectionTestSecretResolutionRequest) (store.EncryptedDataSourceConnectionTestDatabaseConnection, error)
	ResolveDataSourceConnectionTestSysCredential(context.Context, store.DataSourceConnectionTestSecretResolutionRequest) (store.EncryptedDataSourceConnectionTestDatabaseConnection, error)
	FinishDataSourceConnectionTestSecretResolution(context.Context, store.DataSourceConnectionTestSecretResolutionOutcome) error
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
	GetAuthorizedTaskSummary(context.Context, string, string) (store.TaskSummary, error)
	ListTaskSummaries(context.Context, store.TaskListQuery) ([]store.TaskListItem, error)
	CountAuthorizedTaskSummaries(context.Context, string) (int, error)
}

// AgentExecutionStore 持久化已由内存协调器接受的领取和事件事实。
// 适配器不得让 Agent 直接写入任务状态或任意事件负载。
type AgentExecutionStore interface {
	ClaimTask(context.Context, store.Claim) error
	RenewExecutionLease(context.Context, store.LeaseRenewal) error
	AppendExecutionEvent(context.Context, store.ExecutionEvent) error
}

// AuthenticatedAgentExecutionStore 是正式机器凭据路径的受控任务领取、秘密槽位和事实投影边界。
// 它不接受浏览器输入、自由命令、自由路径或直接状态写入。
type AuthenticatedAgentExecutionStore interface {
	ClaimNextExecution(context.Context, store.ExecutionClaimNext) (store.ExecutionLeaseGrant, bool, error)
	RenewExecutionLease(context.Context, store.LeaseRenewal) error
	ResolveExecutionDatabaseConnection(context.Context, store.ExecutionSecretResolutionRequest) (store.EncryptedExecutionDatabaseConnection, error)
	FinishExecutionSecretResolution(context.Context, store.ExecutionSecretResolutionOutcome) error
	AppendAuthenticatedExecutionEvent(context.Context, string, store.ExecutionEvent) (string, error)
}

// CredentialEncryptor keeps raw root-key material out of the HTTP package.
type CredentialEncryptor interface {
	Encrypt(string, credential.Reference, []byte) (credential.Envelope, error)
}

// CredentialDecryptor 仅供受控 Agent 秘密槽位解析使用。
// 它不映射为浏览器能力，也不向普通数据源读取路径开放明文。
type CredentialDecryptor interface {
	Decrypt(credential.Envelope) ([]byte, error)
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
	Deleter         DataSourceDeleter
	ConnectionTests DataSourceConnectionTestStore
	Updater         DataSourceUpdater
	CredentialRefs  DataSourceCredentialReferenceReader
	Drafts          ExportDraftStore
	Prechecks       ExportPrecheckStore
	Tasks           ExportTaskStore
	Executions      AgentExecutionStore
	LogLedger       *logstream.BatchLedger
	// PersistentLogs 仅保存已二次脱敏且已 fsync 的当前任务日志段；为空时保持既有合成内存投影。
	// 不能用内存投影冒充跨重启日志或 SSE 证据。
	PersistentLogs         *logstream.PersistentStore
	Nodes                  ExecutionNodeReader
	NodeManagement         ExecutionNodeManagementStore
	NodeEnvironment        ExecutionNodeEnvironmentStore
	NodeDeleter            ExecutionNodeDeleter
	NodeCandidates         ExecutionNodeCandidateReader
	AgentProtocol          AgentProtocolStore
	AgentEnvironmentChecks AgentExecutionNodeEnvironmentCheckStore
	AgentPrechecks         AgentPrecheckStore
	PrecheckSecrets        AgentPrecheckSecretStore
	AgentConnectionTests   AgentDataSourceConnectionTestStore
	ConnectionTestSecrets  AgentDataSourceConnectionTestSecretStore
	Generator              ExportCommandGenerator
	// GeneralizedGenerator 服务 EX-I2 起的泛化能力切片；缺失时泛化请求失败关闭，冻结单表 CSV 不受影响。
	GeneralizedGenerator ExportCommandGenerator
	PrecheckTTL          time.Duration
	ConnectionTestTTL    time.Duration
	// AgentJDBCConnectionTestEnabled 仅在已授权的本机 G3 连接测试中签发 AGENT_JDBC 租约。
	// 默认 false 保持 G2 合成结果，且不影响真实工具执行门禁。
	AgentJDBCConnectionTestEnabled bool
	// RealExecutionEnabled 仅供已经满足 Windows 本机 MVP G3 运行时条件的组合根启用。
	// 它不会绕过任务、租约、槽位、脱敏或 Agent 认证校验。
	RealExecutionEnabled bool
	EnrollmentTTL        time.Duration
	HeartbeatTTL         time.Duration
	Coordinator          *agentstate.Coordinator
	Encryptor            CredentialEncryptor
	Decryptor            CredentialDecryptor
	CSRF                 CSRFValidator
	CredentialKeyID      string
	// RequestIDGenerator 仅供合成测试注入熵源故障；生产为 nil 时固定使用安全 UUIDv4 生成器。
	RequestIDGenerator func() (string, error)
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
	requestIDGenerator := dependencies.RequestIDGenerator
	if requestIDGenerator == nil {
		requestIDGenerator = identifier.NewUUIDV4
	}
	server := &Server{build: build, provider: dependencies.Identity, authorizer: dependencies.Authorizer, roles: dependencies.Roles, dataSource: dependencies.DataSources, creator: dependencies.Creator, stateChanger: dependencies.StateChanger, deleter: dependencies.Deleter, connectionTests: dependencies.ConnectionTests, updater: dependencies.Updater, credentials: dependencies.CredentialRefs, drafts: dependencies.Drafts, prechecks: dependencies.Prechecks, tasks: dependencies.Tasks, executions: dependencies.Executions, logs: newSyntheticLogStore(dependencies.LogLedger, dependencies.PersistentLogs), nodes: dependencies.Nodes, nodeManagement: dependencies.NodeManagement, nodeEnvironment: dependencies.NodeEnvironment, nodeDeleter: dependencies.NodeDeleter, nodeCandidates: dependencies.NodeCandidates, agentProtocol: dependencies.AgentProtocol, agentEnvironmentChecks: dependencies.AgentEnvironmentChecks, agentPrechecks: dependencies.AgentPrechecks, precheckSecrets: dependencies.PrecheckSecrets, agentConnectionTests: dependencies.AgentConnectionTests, connectionTestSecrets: dependencies.ConnectionTestSecrets, agentJDBCConnectionTestEnabled: dependencies.AgentJDBCConnectionTestEnabled, realExecutionEnabled: dependencies.RealExecutionEnabled, generator: dependencies.Generator, generalGenerator: dependencies.GeneralizedGenerator, precheckTTL: dependencies.PrecheckTTL, connectionTestTTL: dependencies.ConnectionTestTTL, enrollmentTTL: dependencies.EnrollmentTTL, heartbeatTTL: dependencies.HeartbeatTTL, coordinator: dependencies.Coordinator, encryptor: dependencies.Encryptor, decryptor: dependencies.Decryptor, csrf: dependencies.CSRF, keyID: dependencies.CredentialKeyID, requestIDGenerator: requestIDGenerator}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /version", server.version)
	// 浏览器与 Agent 认证域分别注册，避免一次性关联被既有机器认证提前阻断。
	if server.provider == nil {
		mux.HandleFunc("/api/", server.browserUnavailable)
	} else {
		mux.HandleFunc("/api/", server.browserAuthenticated)
	}
	if server.agentProtocol != nil || server.provider != nil {
		mux.HandleFunc("/agent/", server.agentEntry)
	} else {
		mux.HandleFunc("/agent/", server.agentUnavailable)
	}
	mux.HandleFunc("/", notFound)
	return securityHeaders(withRequestID(server.requestIDGenerator, mux))
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
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/execution-nodes" {
		if r.URL.Query().Get("eligibleFor") != "" {
			s.listExecutionNodeCandidates(w, r, principal)
		} else {
			s.listExecutionNodes(w, r, principal)
		}
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/execution-nodes" {
		s.createExecutionNode(w, r, principal)
		return
	}
	if r.Method == http.MethodPost {
		if nodeID, ok := parseExecutionNodeEnrollmentAction(r.URL.Path); ok {
			s.issueAgentEnrollment(w, r, principal, nodeID)
			return
		}
		if nodeID, action, ok := parseExecutionNodeEnvironmentAction(r.URL.Path); ok {
			switch action {
			case "environment-check":
				s.requestExecutionNodeEnvironmentCheck(w, r, principal, nodeID)
				return
			case "enable":
				s.enableExecutionNode(w, r, principal, nodeID)
				return
			}
		}
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/data-sources" {
		s.createDataSource(w, r, principal)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/export-drafts" {
		s.createExportDraft(w, r, principal)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks" {
		s.listTasks(w, r, principal)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/execution-nodes/") {
		nodeID := strings.TrimPrefix(r.URL.Path, "/api/v1/execution-nodes/")
		if nodeID != "" && !strings.Contains(nodeID, "/") && !strings.Contains(nodeID, ":") {
			switch r.Method {
			case http.MethodGet:
				s.getExecutionNode(w, r, principal, nodeID)
				return
			case http.MethodPatch:
				s.updateExecutionNode(w, r, principal, nodeID)
				return
			case http.MethodDelete:
				s.deleteExecutionNode(w, r, principal, nodeID)
				return
			}
		}
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
		if taskID, projection, ok := parseTaskReadPath(r.URL.Path); ok {
			switch projection {
			case "overview":
				s.getTask(w, r, principal, taskID)
				return
			case "snapshot":
				s.getTaskSnapshot(w, r, principal, taskID)
				return
			case "command-evidence":
				s.getTaskCommandEvidence(w, r, principal, taskID)
				return
			case "execution":
				s.getTaskExecution(w, r, principal, taskID)
				return
			case "logs":
				s.listSyntheticLogs(w, r, principal, taskID)
				return
			case "logs-stream":
				s.streamTaskLogs(w, r, principal, taskID)
				return
			}
		}
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/prechecks/") {
		precheckID := strings.TrimPrefix(r.URL.Path, "/api/v1/prechecks/")
		if precheckID != "" && !strings.Contains(precheckID, "/") {
			s.getExportPrecheck(w, r, principal, precheckID)
			return
		}
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/data-source-connection-tests/") {
		connectionTestID := strings.TrimPrefix(r.URL.Path, "/api/v1/data-source-connection-tests/")
		if validDataSourceConnectionTestPathID(connectionTestID) {
			s.getDataSourceConnectionTest(w, r, principal, connectionTestID)
			return
		}
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		if dataSourceID, ok := parseDataSourceConnectionTestAction(r.URL.Path); ok {
			s.testDataSourceConnection(w, r, principal, dataSourceID)
			return
		}
		if dataSourceID, targetState, ok := parseDataSourceStateAction(r.URL.Path); ok {
			s.changeDataSourceState(w, r, principal, dataSourceID, targetState)
			return
		}
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/data-sources/") {
		dataSourceID := strings.TrimPrefix(r.URL.Path, "/api/v1/data-sources/")
		if dataSourceID != "" && !strings.Contains(dataSourceID, "/") {
			s.deleteDataSource(w, r, principal, dataSourceID)
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

func parseDataSourceConnectionTestAction(path string) (string, bool) {
	const prefix = "/api/v1/data-sources/"
	const suffix = ":test-connection"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	dataSourceID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return dataSourceID, dataSourceID != "" && !strings.Contains(dataSourceID, "/")
}

// validDataSourceConnectionTestPathID 限制测试状态资源只接受不含路径语义的服务端标识。
// 这避免查询路径被误解释为节点、命令或本地文件位置。
func validDataSourceConnectionTestPathID(value string) bool {
	return validAgentPrecheckPathID(value)
}

// parseExecutionNodeEnrollmentAction 只识别受控的一次性关联材料签发动作。
// 它不把节点详情路径或其他后缀解释为可执行的 Agent 管理命令。
func parseExecutionNodeEnrollmentAction(path string) (string, bool) {
	const prefix = "/api/v1/execution-nodes/"
	const suffix = ":enrollments"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	nodeID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return nodeID, nodeID != "" && !strings.Contains(nodeID, "/")
}

// parseExecutionNodeEnvironmentAction 只识别固定环境检查和启用两个已登记动作。
// 路径后缀不能变成通用的 Agent 控制、远程命令或文件操作入口。
func parseExecutionNodeEnvironmentAction(path string) (string, string, bool) {
	const prefix = "/api/v1/execution-nodes/"
	for suffix, action := range map[string]string{":environment-check": "environment-check", ":enable": "enable"} {
		if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) {
			nodeID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
			if nodeID != "" && !strings.Contains(nodeID, "/") {
				return nodeID, action, true
			}
		}
	}
	return "", "", false
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
	ClusterName       string `json:"clusterName"`
	TenantName        string `json:"tenantName"`
	Username          string `json:"username"`
	DefaultDatabase   string `json:"defaultDatabase"`
	Password          string `json:"password"`
	// SysUser/SysPassword 是可选的 sys 凭据（参考 ODC 数据源高级设置）；
	// 两者要么同时提供（创建 SYS_PASSWORD 加密修订），要么同时为空（不配置 sys 凭据）。
	SysUser     string `json:"sysUser"`
	SysPassword string `json:"sysPassword"`
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
	ClusterName       *string         `json:"clusterName"`
	TenantName        *string         `json:"tenantName"`
	Username          *string         `json:"username"`
	DefaultDatabase   json.RawMessage `json:"defaultDatabase"`
	Password          *string         `json:"password"`
	// SysUser/SysPassword 是可选的 sys 凭据（参考 ODC 数据源高级设置）：
	// 缺省表示保持现状；两者同空表示清除；两者同非空表示设置/轮换。
	SysUser     *string `json:"sysUser"`
	SysPassword *string `json:"sysPassword"`
}

// executionNodeWriteRequest 只接收节点管理员在注册时声明的固定本机配置。
// Agent 会在首次关联后本机复核；心跳、资源采样和检查结论仍不能由浏览器写入。
type executionNodeWriteRequest struct {
	DisplayName  string   `json:"displayName"`
	Platform     string   `json:"platform"`
	AllowedRoots []string `json:"allowedRoots"`
	ToolHome     string   `json:"toolHome"`
	JavaPath     string   `json:"javaPath"`
}

// exportDraftWriteRequest 接受浏览器导出草稿写入，支持 v5 扁平结构与 v6 泛化配置两种版本。
// v5 继续固定首条切片的单表 CSV 输入；v6 只接受嵌套 config 对象，两者字段不得混用。
type exportDraftWriteRequest struct {
	ConfigVersion string              `json:"configVersion,omitempty"`
	DataSourceID  string              `json:"dataSourceId"`
	NodeID        string              `json:"nodeId"`
	Database      string              `json:"database"`
	Table         string              `json:"table"`
	Format        string              `json:"format"`
	FilePath      string              `json:"filePath"`
	LogPath       string              `json:"logPath"`
	SkipCheckDir  bool                `json:"skipCheckDir"`
	Config        *store.ExportConfig `json:"config,omitempty"`
}

// exportDraftV5Config 是 v5 草稿持久化的扁平结构，必须与首条切片保持字节级一致，
// 以兼容存量草稿、预检查上下文解析与幂等摘要。
type exportDraftV5Config struct {
	DataSourceID string `json:"dataSourceId"`
	NodeID       string `json:"nodeId"`
	Database     string `json:"database"`
	Table        string `json:"table"`
	Format       string `json:"format"`
	FilePath     string `json:"filePath"`
	LogPath      string `json:"logPath"`
	SkipCheckDir bool   `json:"skipCheckDir"`
}

// storedDraftConfigV6 是 v6 草稿持久化的标准文档：同时内嵌扁平投影键与泛化配置。
// 扁平键供预检查上下文与任务投影使用，不能作为额外命令输入来源。
// Table 存逗号连接的对象清单（ALL 为空），Format 取 CSV | DDL | DDL_CSV。
type storedDraftConfigV6 struct {
	ConfigVersion string              `json:"configVersion"`
	DataSourceID  string              `json:"dataSourceId"`
	NodeID        string              `json:"nodeId"`
	Database      string              `json:"database"`
	ScopeKind     string              `json:"scopeKind"`
	Table         string              `json:"table"`
	ContentKind   string              `json:"contentKind"`
	Format        string              `json:"format"`
	FilePath      string              `json:"filePath"`
	LogPath       string              `json:"logPath"`
	SkipCheckDir  bool                `json:"skipCheckDir"`
	Config        *store.ExportConfig `json:"config"`
}

// normalizedExportDraft 是 v5 或 v6 草稿归一化后统一的命令生成输入。
// EX-I2 起支持全部/指定对象与 DDL 内容；EX-I3 增加 CSV 序列化、压缩、文件布局、筛选与资源选项；
// EX-I4 增加 CUT/SQL 数据格式及其专属序列化选项。
// 单表 CSV 且未设置任何选项时仍路由到冻结 v5 生成路径。
type normalizedExportDraft struct {
	DataSourceID  string
	NodeID        string
	Database      string
	ScopeKind     string // ALL | SPECIFIED
	ObjectType    string // TABLE | VIEW；ALL 范围时为空
	Objects       []string
	ExcludeTables []string
	ContentKind   string // DATA_ONLY | DDL_ONLY | DDL_AND_DATA
	Format        string // CSV | CUT | SQL；仅 DDL 时为空
	FilePath      string
	LogPath       string
	SkipCheckDir  bool
	// EX-I3/EX-I4 选项：格式序列化、压缩、文件布局、筛选与资源参数。
	CsvOptions      store.CsvOptions
	CutOptions      store.CutOptions
	Compress        bool
	CompressionAlgo string
	// EX-I7 压缩等级（2026-08-10）：--compression-level，官方按算法分范围（zstd 1-22、zlib -1~9；gzip/snappy 不支持）。
	CompressionLevel      *int64
	NoNestedDir           bool
	MaxFileSize           *int64
	RetainEmptyFiles      bool
	QuerySql              string
	IncludeColumns        []string
	ExcludeColumns        []string
	ExcludeVirtualColumns bool
	FlashbackScn          *int64
	FlashbackTimestamp    string
	Thread                *int
	PageSize              *int
	ParallelMacro         *int
	FetchSize             *int
	JvmMemory             string
	// EX-I7 文件拆分（2026-08-10）：--block-size（数字 MB 或数字+MB/ROW 后缀），显式传值已受控实测。
	BlockSize string
	// EX-I4 POS 定版（2026-08-07 实测）：控制文件目录（--ctl-path），仅 POS 格式有效。
	ControlFilePath string
	// EX-I6 对象存储（2026-08-07）：Multipart 本地临时分块目录（--tmp-path）。
	TmpPath string
	// EX-I6 对象存储（2026-08-07）：输出目标类型（LOCAL/OSS/S3/COS/OBS），传递给生成器做路径分流校验。
	OutputKind string
	// EX-I7 DDL 行为（2026-08-10）：前置 DROP、保留 Schema 与紧凑 Schema，仅在 DDL 内容时活动。
	DropObject    bool
	RetainSchema  bool
	CompactSchema bool
	// EX-I7 剩余参数第一批（2026-08-11 受控实测定版）：条件筛选与一致性快照。
	// 备库弱读缺少副本/权限预检查，保存点续跑缺少 EX-I8 恢复链路，继续失败关闭。
	Where    string
	Snapshot bool
}

// maxExportObjectExpressions 限制单个草稿的对象表达式与排除表数量，
// 防止无限对象进入命令、预检查探测与快照。
const maxExportObjectExpressions = 100

// isFrozenSingleTableCSV 判断归一结果是否仍属于首条切片的冻结形状，
// 是则继续使用 v5 元数据与 export-odp-single-table-csv-v1 生成，保证指纹与 argv 不变。
// 任一 EX-I3 选项被设置都会离开冻结形状，改走 v6 泛化生成器；
// EX-I6 起对象存储输出与 --tmp-path 也不走冻结路径（v5 仅本地输出；空 OutputKind 兼容旧调用按本地处理）。
func (n normalizedExportDraft) isFrozenSingleTableCSV() bool {
	return (n.OutputKind == "" || n.OutputKind == "LOCAL") && n.TmpPath == "" && n.ScopeKind == "SPECIFIED" && n.ObjectType == "TABLE" && len(n.Objects) == 1 && len(n.ExcludeTables) == 0 && n.ContentKind == "DATA_ONLY" && n.Format == "CSV" && n.hasZeroOptions()
}

// hasZeroOptions 判断全部 EX-I3/EX-I4 选项均为零值。
func (n normalizedExportDraft) hasZeroOptions() bool {
	return n.CsvOptions == (store.CsvOptions{}) && n.CutOptions == (store.CutOptions{}) && !n.Compress && n.CompressionAlgo == "" && n.CompressionLevel == nil &&
		!n.NoNestedDir && n.MaxFileSize == nil && !n.RetainEmptyFiles &&
		n.QuerySql == "" && len(n.IncludeColumns) == 0 && len(n.ExcludeColumns) == 0 &&
		!n.ExcludeVirtualColumns && n.FlashbackScn == nil && n.FlashbackTimestamp == "" &&
		n.Thread == nil && n.PageSize == nil && n.ParallelMacro == nil && n.FetchSize == nil && n.JvmMemory == "" && n.BlockSize == "" &&
		!n.DropObject && !n.RetainSchema && !n.CompactSchema && n.Where == "" && !n.Snapshot
}

// draftCapability 按归一结果推导泛化能力版本。
// EX-I4：DATA_ONLY 按数据格式返回对应能力；CUT/SQL 无 DDL 组合能力，DDL 内容仍只走 CSV。
func draftCapability(n normalizedExportDraft) string {
	switch n.ContentKind {
	case "DDL_ONLY":
		return "export-odp-ddl-v1"
	case "DDL_AND_DATA":
		return "export-odp-ddl-csv-v1"
	default:
		switch n.Format {
		case "CUT":
			return "export-odp-cut-v1"
		case "SQL":
			return "export-odp-sql-v1"
		case "POS":
			// EX-I4 POS 定版（2026-08-07 实测）：独立 --pos + --ctl-path 能力切片。
			return "export-odp-pos-v1"
		case "PARQUET":
			return "export-odp-parquet-v1"
		case "ORC":
			return "export-odp-orc-v1"
		case "AVRO":
			return "export-odp-avro-v1"
		default:
			return "export-odp-full-csv-v1"
		}
	}
}

// draftDisplayFormat 返回快照与摘要投影使用的格式标识。
// DDL_ONLY 无数据格式；DDL_AND_DATA 固定为 CSV 组合；DATA_ONLY 按实际数据格式投影。
func draftDisplayFormat(contentKind, format string) string {
	switch contentKind {
	case "DDL_ONLY":
		return "DDL"
	case "DDL_AND_DATA":
		return "DDL_CSV"
	default:
		return format
	}
}

// validateExportObjectName 校验单个对象名称：非空、长度受限、无通配符与分隔符、无控制字符且无首尾空白。
func validateExportObjectName(name string) error {
	if name == "" || len(name) > 256 || name != strings.TrimSpace(name) {
		return errors.New("export object name is invalid")
	}
	if strings.ContainsAny(name, "*,\x00\r\n") {
		return errors.New("export object name contains forbidden characters")
	}
	return nil
}

// draftStructuredColumns 是 v6 草稿七个结构化子配置列的序列化结果。
type draftStructuredColumns struct {
	ObjectScopeJSON       string
	ContentSelectionJSON  string
	DataFormatJSON        string
	OutputConfigJSON      string
	PerformanceConfigJSON string
	FilterConfigJSON      string
	DDLBehaviorJSON       string
}

// validateDraftVersionRouting 校验写入请求的版本路由规则，违反即 400 失败关闭。
// configVersion 缺省视为 v5；v5 不得携带 config，v6 不得携带扁平字段。
func validateDraftVersionRouting(request exportDraftWriteRequest) error {
	switch request.ConfigVersion {
	case "", "v5":
		if request.Config != nil {
			return errors.New("v5 export draft must not carry generalized config")
		}
		return nil
	case "v6":
		if request.Config == nil {
			return errors.New("v6 export draft requires generalized config")
		}
		if request.Database != "" || request.Table != "" || request.Format != "" || request.FilePath != "" || request.LogPath != "" || request.SkipCheckDir {
			return errors.New("v6 export draft must not carry flat fields")
		}
		return nil
	default:
		return errors.New("export draft config version is unsupported")
	}
}

// normalizeExportDraftRequest 把已通过版本路由校验的写入请求归一为命令生成输入。
// v6 配置必须严格落在已验证能力矩阵内，超出范围即失败关闭。
func normalizeExportDraftRequest(request exportDraftWriteRequest) (normalizedExportDraft, error) {
	if request.ConfigVersion == "v6" {
		return normalizeExportConfigV6(request.Config, request.DataSourceID, request.NodeID)
	}
	return normalizedExportDraft{
		DataSourceID: request.DataSourceID, NodeID: request.NodeID,
		Database: request.Database, ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{request.Table},
		ContentKind: "DATA_ONLY", Format: request.Format,
		FilePath: request.FilePath, LogPath: request.LogPath, SkipCheckDir: request.SkipCheckDir,
	}, nil
}

// normalizeExportConfigV6 校验 v6 泛化配置只表达 EX-I2 已验证的能力矩阵：
// ALL 或 SPECIFIED（仅 TABLE/VIEW 单一类型、1..100 个表达式）、DATA_ONLY/DDL_ONLY/DDL_AND_DATA 内容、
// CSV 数据格式、LOCAL 输出。多库 schema 前缀、通配符、gated 对象类型与非零的
// 性能/筛选/DDL 选项均失败关闭。
func normalizeExportConfigV6(config *store.ExportConfig, dataSourceID, nodeID string) (normalizedExportDraft, error) {
	scope := config.ObjectScope
	normalized := normalizedExportDraft{
		DataSourceID: dataSourceID, NodeID: nodeID,
		Database: scope.Database, ContentKind: config.ContentSelection.ContentKind,
	}
	if scope.Database == "" || len(scope.Database) > 512 {
		return normalizedExportDraft{}, errors.New("v6 object scope requires a database")
	}
	switch scope.ScopeKind {
	case "ALL":
		if len(scope.ObjectTypes) != 0 || len(scope.Expressions) != 0 {
			return normalizedExportDraft{}, errors.New("v6 ALL scope must not carry object types or expressions")
		}
		normalized.ScopeKind = "ALL"
	case "SPECIFIED":
		if len(scope.ObjectTypes) != 1 || (scope.ObjectTypes[0] != "TABLE" && scope.ObjectTypes[0] != "VIEW") {
			return normalizedExportDraft{}, errors.New("v6 object scope must specify exactly one supported object type")
		}
		if len(scope.Expressions) == 0 || len(scope.Expressions) > maxExportObjectExpressions {
			return normalizedExportDraft{}, errors.New("v6 object expressions must number between 1 and 100")
		}
		normalized.ScopeKind = "SPECIFIED"
		normalized.ObjectType = scope.ObjectTypes[0]
		for index, expression := range scope.Expressions {
			// schema 前缀只允许缺省或与范围数据库一致；跨库表达式（EX-F012）未取证，失败关闭。
			if expression.Schema != "" && expression.Schema != scope.Database {
				return normalizedExportDraft{}, errors.New("v6 multi-database schema prefix is not enabled")
			}
			if err := validateExportObjectName(expression.Name); err != nil {
				return normalizedExportDraft{}, errors.New("v6 object expression name is invalid")
			}
			// RawInput 一律由服务端生成规范值；浏览器自由文本不得进入草稿、快照或 SQLite。
			if expression.Schema != "" {
				config.ObjectScope.Expressions[index].RawInput = expression.Schema + "." + expression.Name
			} else {
				config.ObjectScope.Expressions[index].RawInput = expression.Name
			}
			normalized.Objects = append(normalized.Objects, expression.Name)
		}
	default:
		return normalizedExportDraft{}, errors.New("v6 object scope kind is unsupported")
	}
	if len(scope.ExcludeTables) != 0 {
		if normalized.ObjectType == "VIEW" {
			return normalizedExportDraft{}, errors.New("v6 exclude tables require table scope")
		}
		if len(scope.ExcludeTables) > maxExportObjectExpressions {
			return normalizedExportDraft{}, errors.New("v6 exclude tables exceed the limit")
		}
		for _, name := range scope.ExcludeTables {
			if err := validateExportObjectName(name); err != nil {
				return normalizedExportDraft{}, errors.New("v6 exclude table name is invalid")
			}
		}
		normalized.ExcludeTables = append([]string(nil), scope.ExcludeTables...)
	}
	switch normalized.ContentKind {
	case "DATA_ONLY":
		if normalized.ObjectType == "VIEW" {
			return normalizedExportDraft{}, errors.New("v6 views cannot export data")
		}
		// EX-I4/EX-I5：数据格式单选 CSV/CUT/POS/SQL/PARQUET/ORC/AVRO；
		// POS 映射已实测定版（2026-08-07），结构化格式按官方格式表接入。
		switch config.DataFormat.FormatKind {
		case "CSV", "CUT", "POS", "SQL", "PARQUET", "ORC", "AVRO":
			normalized.Format = config.DataFormat.FormatKind
		default:
			return normalizedExportDraft{}, errors.New("v6 data format is unsupported")
		}
	case "DDL_ONLY":
		if config.DataFormat.FormatKind != "" && config.DataFormat.FormatKind != "CSV" {
			return normalizedExportDraft{}, errors.New("v6 ddl-only content must not declare a data format")
		}
	case "DDL_AND_DATA":
		if normalized.ObjectType == "VIEW" {
			return normalizedExportDraft{}, errors.New("v6 views cannot export data")
		}
		// EX-I4：能力版本体系没有 DDL+CUT/SQL 组合能力，DDL 内容固定 CSV。
		normalized.Format = "CSV"
		if config.DataFormat.FormatKind != "CSV" {
			return normalizedExportDraft{}, errors.New("v6 ddl-and-data content requires csv format")
		}
	default:
		return normalizedExportDraft{}, errors.New("v6 content selection is unsupported")
	}
	output := config.OutputConfig
	// EX-I6 对象存储（2026-08-07）：输出类型接受 LOCAL 与受控对象存储（OSS/S3/COS/OBS）。
	// 本地输出要求本地路径形态；对象存储要求受控 URI（scheme/参数白名单，拒绝密钥参数，密钥走凭据槽位）。
	normalized.OutputKind = output.OutputKind
	switch output.OutputKind {
	case "LOCAL":
		if output.FilePath == "" {
			return normalizedExportDraft{}, errors.New("v6 output config must be a local file path only")
		}
	case "OSS", "S3", "COS", "OBS":
		if err := validateControlledStorageURI(output.OutputKind, output.FilePath); err != nil {
			return normalizedExportDraft{}, err
		}
	default:
		return normalizedExportDraft{}, errors.New("v6 output kind is unsupported")
	}
	if output.MaxFileSize != nil && *output.MaxFileSize < 1 {
		return normalizedExportDraft{}, errors.New("v6 max file size must be positive")
	}
	if output.CompressionAlgo != "" {
		if !output.Compress {
			return normalizedExportDraft{}, errors.New("v6 compression algorithm requires compression enabled")
		}
		if !containsString(compressionAlgoValues, output.CompressionAlgo) {
			return normalizedExportDraft{}, errors.New("v6 compression algorithm is unsupported")
		}
	}
	// EX-I7 压缩等级（2026-08-10）：官方按算法分范围——zstd 1~22（默认 3）、zlib -1~9（默认 -1），
	// gzip/snappy 不支持指定等级；未启用压缩或算法未选择时携带等级即失败关闭。
	if output.CompressionLevel != nil {
		if !output.Compress || output.CompressionAlgo == "" {
			return normalizedExportDraft{}, errors.New("v6 compression level requires compression and an algorithm")
		}
		switch output.CompressionAlgo {
		case "zstd":
			if *output.CompressionLevel < 1 || *output.CompressionLevel > 22 {
				return normalizedExportDraft{}, errors.New("v6 zstd compression level must be within 1..22")
			}
		case "zlib":
			if *output.CompressionLevel < -1 || *output.CompressionLevel > 9 {
				return normalizedExportDraft{}, errors.New("v6 zlib compression level must be within -1..9")
			}
		default:
			// gzip/snappy 官方不支持指定压缩等级。
			return normalizedExportDraft{}, errors.New("v6 compression level is unsupported for this algorithm")
		}
	}
	normalized.NoNestedDir, normalized.MaxFileSize, normalized.RetainEmptyFiles = output.NoNestedDir, output.MaxFileSize, output.RetainEmptyFiles
	normalized.Compress, normalized.CompressionAlgo = output.Compress, output.CompressionAlgo
	normalized.CompressionLevel = output.CompressionLevel
	// EX-I4 POS：控制文件目录（--ctl-path）只允许 POS 格式携带；其他格式携带即失败关闭。
	normalized.ControlFilePath = output.ControlFilePath
	if normalized.Format == "POS" {
		if strings.TrimSpace(normalized.ControlFilePath) == "" {
			return normalizedExportDraft{}, errors.New("v6 pos format requires a control file path")
		}
		if len(normalized.ControlFilePath) > 4096 || strings.ContainsRune(normalized.ControlFilePath, 0) || strings.ContainsAny(normalized.ControlFilePath, "\r\n") {
			return normalizedExportDraft{}, errors.New("v6 control file path is invalid")
		}
	} else if normalized.ControlFilePath != "" {
		return normalizedExportDraft{}, errors.New("v6 control file path requires pos format")
	}
	// EX-I6：对象存储 Multipart 本地临时分块目录（--tmp-path）必须是安全的节点本地路径形态。
	normalized.TmpPath = output.TmpPath
	if normalized.TmpPath != "" {
		if len(normalized.TmpPath) > 4096 || strings.ContainsRune(normalized.TmpPath, 0) || strings.ContainsAny(normalized.TmpPath, "\r\n") {
			return normalizedExportDraft{}, errors.New("v6 tmp path is invalid")
		}
	}

	performance := config.PerformanceConfig
	// --retry 只能由 EX-I8 的失败任务恢复流程在验证 dump.ckpt 与原快照后派生；
	// 新建草稿携带该开关必须失败关闭，不能把已知的工具失败推迟到执行阶段。
	if performance.Retry {
		return normalizedExportDraft{}, errors.New("v6 retry option requires the checkpoint recovery flow")
	}
	normalized.Thread, normalized.PageSize, normalized.ParallelMacro, normalized.FetchSize = performance.Thread, performance.PageSize, performance.ParallelMacro, performance.FetchSize
	for _, value := range []*int{normalized.Thread, normalized.PageSize, normalized.ParallelMacro, normalized.FetchSize} {
		if value != nil && *value < 1 {
			return normalizedExportDraft{}, errors.New("v6 performance values must be positive")
		}
	}
	if performance.JvmMemory != "" && !memSizePattern.MatchString(performance.JvmMemory) {
		return normalizedExportDraft{}, errors.New("v6 jvm memory must match the official K/M/G/T form")
	}
	normalized.JvmMemory = performance.JvmMemory
	// EX-I7 文件拆分（2026-08-10）：--block-size 显式传值按 MB/ROW 生效（2026-08-07 受控实测）；
	// 值为正整数或正整数+MB/ROW 后缀，默认值 0/1024MB 的官方冲突只影响未显式设置场景，不阻断显式传值。
	if performance.BlockSize != "" && !blockSizePattern.MatchString(performance.BlockSize) {
		return normalizedExportDraft{}, errors.New("v6 block size must be a positive integer with optional MB/ROW suffix")
	}
	normalized.BlockSize = performance.BlockSize

	filter := config.FilterConfig
	// EX-I7 剩余参数第一批（2026-08-11 受控实测定版）：--where 条件筛选与 --snapshot 一致性快照已启用；
	// --weak-read 缺少副本与权限预检查，其余列出的筛选项也仍为 VALIDATION_GATED，携带即失败关闭。
	if filter.Partition != "" || len(filter.ExcludeDataTypes) != 0 || filter.EnableHiddenPk != nil || filter.WeakRead != nil {
		return normalizedExportDraft{}, errors.New("v6 filter options are not enabled")
	}
	normalized.Where = filter.Where
	normalized.Snapshot = filter.Snapshot != nil && *filter.Snapshot
	normalized.QuerySql = filter.QuerySql
	normalized.ExcludeVirtualColumns = filter.ExcludeVirtualColumns != nil && *filter.ExcludeVirtualColumns
	normalized.FlashbackScn, normalized.FlashbackTimestamp = filter.FlashbackScn, filter.FlashbackTimestamp
	if normalized.QuerySql != "" {
		if len(normalized.QuerySql) > 64<<10 || strings.ContainsRune(normalized.QuerySql, '\x00') {
			return normalizedExportDraft{}, errors.New("v6 query sql is invalid")
		}
		if normalized.FlashbackScn != nil || normalized.FlashbackTimestamp != "" {
			return normalizedExportDraft{}, errors.New("v6 query sql conflicts with flashback options")
		}
		// 官方约束：--query-sql 与 --where 不能搭配（--where 只能配合 --table）。
		if normalized.Where != "" {
			return normalizedExportDraft{}, errors.New("v6 query sql conflicts with where option")
		}
	}
	if normalized.Where != "" && validateOptionText(normalized.Where, 64<<10) != nil {
		return normalizedExportDraft{}, errors.New("v6 where option is invalid")
	}
	// OBDUMPER 4.3.5 的 --where 只允许与明确的 --table 范围搭配；
	// --all 或视图范围携带时必须在生成命令前失败关闭。
	if normalized.Where != "" && (normalized.ScopeKind != "SPECIFIED" || normalized.ObjectType != "TABLE") {
		return normalizedExportDraft{}, errors.New("v6 where option requires specified table scope")
	}
	if normalized.FlashbackScn != nil && *normalized.FlashbackScn < 1 {
		return normalizedExportDraft{}, errors.New("v6 flashback scn must be positive")
	}
	// --snapshot 与闪回参数的组合尚未完成受控实测；在确认前保持互斥，避免生成语义不明的一致性命令。
	if normalized.Snapshot && (normalized.FlashbackScn != nil || normalized.FlashbackTimestamp != "") {
		return normalizedExportDraft{}, errors.New("v6 snapshot conflicts with flashback options")
	}
	if normalized.FlashbackTimestamp != "" && validateOptionText(normalized.FlashbackTimestamp, 256) != nil {
		return normalizedExportDraft{}, errors.New("v6 flashback timestamp is invalid")
	}
	normalized.IncludeColumns = append([]string(nil), filter.IncludeColumnNames...)
	normalized.ExcludeColumns = append([]string(nil), filter.ExcludeColumnNames...)
	if len(normalized.IncludeColumns) != 0 && len(normalized.ExcludeColumns) != 0 {
		return normalizedExportDraft{}, errors.New("v6 include and exclude column names are mutually exclusive")
	}
	for _, column := range append(append([]string(nil), normalized.IncludeColumns...), normalized.ExcludeColumns...) {
		if err := validateExportObjectName(column); err != nil {
			return normalizedExportDraft{}, errors.New("v6 column name is invalid")
		}
	}
	ddl := config.DDLBehavior
	// EX-I7 DDL 行为（2026-08-10）：--drop-object/--retain-schema 仅在 DDL 内容时活动并随 DDL 能力发射；
	// --compact-schema（2026-08-11 受控实测定版：show create table 检索文本）同样仅限 DDL 内容；
	// --add-extra-message 依赖 sys 凭据可用性（未取证）、--sequence-policy 仍为 VALIDATION_GATED，携带即失败关闭。
	if ddl.AddExtraMessage != nil || ddl.SequencePolicy != "" {
		return normalizedExportDraft{}, errors.New("v6 ddl behavior is not enabled")
	}
	if (ddl.DropObject != nil && *ddl.DropObject) || (ddl.RetainSchema != nil && *ddl.RetainSchema) || (ddl.CompactSchema != nil && *ddl.CompactSchema) {
		if normalized.ContentKind == "DATA_ONLY" {
			return normalizedExportDraft{}, errors.New("v6 ddl behavior requires ddl content")
		}
		if ddl.CompactSchema != nil && *ddl.CompactSchema && normalized.ScopeKind == "SPECIFIED" && normalized.ObjectType != "TABLE" {
			return normalizedExportDraft{}, errors.New("v6 compact schema requires table ddl scope")
		}
		normalized.DropObject = ddl.DropObject != nil && *ddl.DropObject
		normalized.RetainSchema = ddl.RetainSchema != nil && *ddl.RetainSchema
		normalized.CompactSchema = ddl.CompactSchema != nil && *ddl.CompactSchema
	}
	// EX-I4：序列化选项按格式适用性校验；跨格式文本选项随 FORMAT_IN 激活，
	// CSV/CUT 专属字段越界即失败关闭，防止命令生成器收到不可能的参数组合。
	switch normalized.Format {
	case "CSV":
		if err := validateCsvOptions(config.DataFormat.CsvOptions); err != nil {
			return normalizedExportDraft{}, err
		}
		if config.DataFormat.CsvOptions.ColumnSplitter != "" {
			// --column-splitter 为 CUT 专属（POS 定版后仍保持），CSV 携带失败关闭。
			return normalizedExportDraft{}, errors.New("v6 column splitter requires cut format")
		}
		normalized.CsvOptions = config.DataFormat.CsvOptions
		if config.DataFormat.CutOptions != (store.CutOptions{}) {
			return normalizedExportDraft{}, errors.New("v6 cut options require cut format")
		}
	case "CUT":
		csv := config.DataFormat.CsvOptions
		if csv.SkipHeader || csv.ColumnSeparator != "" || csv.ColumnQuote != "" || csv.ColumnQuoteMode != "" {
			return normalizedExportDraft{}, errors.New("v6 csv-only options require csv format")
		}
		if err := validateSharedTextOptions(csv); err != nil {
			return normalizedExportDraft{}, err
		}
		normalized.CsvOptions = csv
		normalized.CutOptions = config.DataFormat.CutOptions
	case "SQL":
		csv := config.DataFormat.CsvOptions
		if csv.SkipHeader || csv.ColumnSeparator != "" || csv.ColumnQuote != "" || csv.ColumnQuoteMode != "" || csv.EscapeCharacter != "" || csv.NullString != "" || csv.WithTrim || csv.ColumnSplitter != "" {
			return normalizedExportDraft{}, errors.New("v6 csv-only options require csv format")
		}
		if err := validateSharedTextOptions(csv); err != nil {
			return normalizedExportDraft{}, err
		}
		normalized.CsvOptions = csv
		if config.DataFormat.CutOptions != (store.CutOptions{}) {
			return normalizedExportDraft{}, errors.New("v6 cut options require cut format")
		}
	case "POS":
		// EX-I4 POS 定版（2026-08-07 实测）：POS 首版只启用 --pos 与 --ctl-path；
		// 共享文本序列化参数对 POS 的适用性未取证，任何 csvOptions/cutOptions 越界即失败关闭。
		if config.DataFormat.CsvOptions != (store.CsvOptions{}) || config.DataFormat.CutOptions != (store.CutOptions{}) {
			return normalizedExportDraft{}, errors.New("v6 serialization options require csv or cut format")
		}
	case "PARQUET", "ORC", "AVRO":
		// EX-I5 结构化格式：官方格式表只列文件编码与闪回等读取层参数；
		// 允许文件编码随 FORMAT_IN 活动，其余序列化选项越界失败关闭；
		// 压缩仅支持可读格式（CSV/CUT/POS/SQL），结构化格式携带压缩即失败关闭。
		csv := config.DataFormat.CsvOptions
		if csv.SkipHeader || csv.ColumnSeparator != "" || csv.ColumnQuote != "" || csv.ColumnQuoteMode != "" || csv.EscapeCharacter != "" || csv.LineSeparator != "" || csv.NullString != "" || csv.WithTrim || csv.ColumnSplitter != "" {
			return normalizedExportDraft{}, errors.New("v6 serialization options require csv or cut format")
		}
		if err := validateOptionText(csv.FileEncoding, 256); err != nil {
			return normalizedExportDraft{}, errors.New("v6 file encoding is invalid")
		}
		normalized.CsvOptions = csv
		if config.DataFormat.CutOptions != (store.CutOptions{}) {
			return normalizedExportDraft{}, errors.New("v6 cut options require cut format")
		}
		if normalized.Compress || normalized.CompressionAlgo != "" {
			return normalizedExportDraft{}, errors.New("v6 compression requires a readable format")
		}
	default:
		// DDL_ONLY 无数据格式：任何序列化选项都失败关闭。
		if config.DataFormat.CsvOptions != (store.CsvOptions{}) || config.DataFormat.CutOptions != (store.CutOptions{}) {
			return normalizedExportDraft{}, errors.New("v6 serialization options require a data format")
		}
	}
	normalized.FilePath, normalized.LogPath, normalized.SkipCheckDir = output.FilePath, output.LogPath, output.SkipCheckDir
	return normalized, nil
}

// storageSchemeValues 是 V1.0 受控对象存储 scheme 白名单；不接受任意 URI。
var storageSchemeValues = []string{"oss", "s3", "cos", "obs"}

// validateControlledStorageURI 校验对象存储输出 URI（EX-I6，2026-08-07）：
// scheme 必须与输出类型一致且在白名单内；bucket（authority）非空；路径以 / 开头；
// query 参数只允许 endpoint/region/storage-class；拒绝 access-key/secret-key 等任何密钥参数，
// 存储凭据必须走执行槽位（Agent 侧 HADOOP_CONF_DIR/core-site.xml），不进入 URI、argv、日志或快照。
func validateControlledStorageURI(outputKind, uri string) error {
	if uri == "" || len(uri) > 4096 || strings.ContainsRune(uri, 0) || strings.ContainsAny(uri, "\r\n") {
		return errors.New("v6 storage uri is invalid")
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/") {
		return errors.New("v6 storage uri must be scheme://bucket/path")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if !containsString(storageSchemeValues, scheme) || !strings.EqualFold(scheme, outputKind) {
		return errors.New("v6 storage uri scheme is unsupported")
	}
	for key := range parsed.Query() {
		if key != "endpoint" && key != "region" && key != "storage-class" {
			return errors.New("v6 storage uri has unsupported parameters")
		}
	}
	return nil
}

// validateSharedTextOptions 校验 CUT/SQL 与 CSV 共享的文本序列化选项：
// 文本长度受限且不含控制字符，转义字符限单字符，列分隔字符串限 CUT 且长度受限。
func validateSharedTextOptions(csv store.CsvOptions) error {
	for _, option := range []struct {
		name  string
		value string
	}{
		{"--line-separator", csv.LineSeparator},
		{"--null-string", csv.NullString},
		{"--file-encoding", csv.FileEncoding},
		{"--column-splitter", csv.ColumnSplitter},
	} {
		if err := validateOptionText(option.value, 256); err != nil {
			return fmt.Errorf("v6 %s is invalid", option.name)
		}
	}
	if csv.EscapeCharacter != "" && (len(csv.EscapeCharacter) != 1 || strings.ContainsAny(csv.EscapeCharacter, "\x00\r\n")) {
		return errors.New("v6 escape character must be a single character")
	}
	return nil
}

// csvQuoteModeValues 是官方 CSV 包围模式的固定枚举。
var csvQuoteModeValues = []string{"all", "all_not_null", "minimal", "non_numeric", "none"}

// compressionAlgoValues 是官方压缩算法的固定枚举。
var compressionAlgoValues = []string{"zstd", "zlib", "gzip", "snappy"}

// memSizePattern 匹配官方 JVM 内存表达：数字加可选 K/M/G/T 后缀。
var memSizePattern = regexp.MustCompile(`^[1-9][0-9]*[KMGTP]?$`)

// blockSizePattern 匹配官方 --block-size 表达：正整数（MB）或正整数+MB/ROW 后缀；
// 不支持 1GB/1M 等格式（2026-08-07 受控实测确认）。
var blockSizePattern = regexp.MustCompile(`^[1-9][0-9]*(MB|ROW)?$`)

// validateOptionText 校验选项文本：非空时长度受限且不含控制字符。
func validateOptionText(value string, maxLength int) error {
	if value == "" {
		return nil
	}
	if len(value) > maxLength || strings.ContainsAny(value, "\x00\r\n") || value != strings.TrimSpace(value) {
		return errors.New("option text is invalid")
	}
	return nil
}

// containsString 判断字符串切片是否包含目标值。
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// validateCsvOptions 校验 CSV 序列化选项的枚举与边界；escape-character 官方仅支持单字符。
func validateCsvOptions(options store.CsvOptions) error {
	for _, option := range []struct {
		name  string
		value string
	}{
		{"--column-separator", options.ColumnSeparator},
		{"--column-quote", options.ColumnQuote},
		{"--line-separator", options.LineSeparator},
		{"--null-string", options.NullString},
		{"--file-encoding", options.FileEncoding},
	} {
		if err := validateOptionText(option.value, 256); err != nil {
			return fmt.Errorf("v6 %s is invalid", option.name)
		}
	}
	if options.ColumnQuoteMode != "" && !containsString(csvQuoteModeValues, options.ColumnQuoteMode) {
		return errors.New("v6 column quote mode is unsupported")
	}
	if options.EscapeCharacter != "" && (len(options.EscapeCharacter) != 1 || strings.ContainsAny(options.EscapeCharacter, "\x00\r\n")) {
		return errors.New("v6 escape character must be a single character")
	}
	return nil
}

// draftNormalized 按草稿 config_version 重建归一化生成输入。
// v5 草稿按扁平结构解析，v6 标准文档必须含完整扁平投影键，缺失或不匹配即失败关闭。
func draftNormalized(draft store.ExportDraft) (normalizedExportDraft, error) {
	switch draft.ConfigVersion {
	case "v5":
		var request exportDraftWriteRequest
		if err := json.Unmarshal([]byte(draft.ConfigJSON), &request); err != nil || request.Config != nil {
			return normalizedExportDraft{}, errors.New("v5 draft configuration is unreadable")
		}
		return normalizedExportDraft{
			DataSourceID: request.DataSourceID, NodeID: request.NodeID,
			Database: request.Database, ScopeKind: "SPECIFIED", ObjectType: "TABLE", Objects: []string{request.Table},
			ContentKind: "DATA_ONLY", Format: request.Format,
			FilePath: request.FilePath, LogPath: request.LogPath, SkipCheckDir: request.SkipCheckDir,
		}, nil
	case "v6":
		var stored storedDraftConfigV6
		if err := json.Unmarshal([]byte(draft.ConfigJSON), &stored); err != nil ||
			stored.ConfigVersion != "v6" || stored.DataSourceID != draft.DataSourceID || stored.NodeID != draft.NodeID || stored.Config == nil {
			return normalizedExportDraft{}, errors.New("v6 draft configuration is unreadable")
		}
		// 嵌套泛化配置必须重新通过能力矩阵校验，并与扁平投影键逐项一致；
		// 数据损坏或语义冲突时失败关闭，不得只按扁平字段执行却冻结矛盾的 v2 快照。
		nested, err := normalizeExportConfigV6(stored.Config, draft.DataSourceID, draft.NodeID)
		if err != nil {
			return normalizedExportDraft{}, errors.New("v6 draft nested configuration is inconsistent")
		}
		if stored.Database != nested.Database || stored.ScopeKind != nested.ScopeKind ||
			stored.Table != strings.Join(nested.Objects, ",") || stored.ContentKind != nested.ContentKind ||
			stored.Format != draftDisplayFormat(nested.ContentKind, nested.Format) ||
			stored.FilePath != nested.FilePath || stored.LogPath != nested.LogPath || stored.SkipCheckDir != nested.SkipCheckDir {
			return normalizedExportDraft{}, errors.New("v6 draft flat projection is inconsistent")
		}
		return nested, nil
	default:
		return normalizedExportDraft{}, errors.New("draft config version is unsupported")
	}
}

// buildDraftPersistence 按版本生成持久化 config_json 与结构化列。
// v5 输出与首条切片字节级一致；v6 输出标准文档并序列化子配置。
func buildDraftPersistence(request exportDraftWriteRequest, normalized normalizedExportDraft) (string, string, draftStructuredColumns, error) {
	if request.ConfigVersion == "v6" {
		stored := storedDraftConfigV6{
			ConfigVersion: "v6", DataSourceID: request.DataSourceID, NodeID: request.NodeID,
			Database: normalized.Database, ScopeKind: normalized.ScopeKind,
			Table: strings.Join(normalized.Objects, ","), ContentKind: normalized.ContentKind,
			Format:   draftDisplayFormat(normalized.ContentKind, normalized.Format),
			FilePath: normalized.FilePath, LogPath: normalized.LogPath, SkipCheckDir: normalized.SkipCheckDir,
			Config: request.Config,
		}
		raw, err := json.Marshal(stored)
		if err != nil {
			return "", "", draftStructuredColumns{}, err
		}
		structured, err := marshalDraftStructuredColumns(request.Config)
		if err != nil {
			return "", "", draftStructuredColumns{}, err
		}
		return "v6", string(raw), structured, nil
	}
	raw, err := json.Marshal(exportDraftV5Config{
		DataSourceID: request.DataSourceID, NodeID: request.NodeID,
		Database: request.Database, Table: request.Table, Format: request.Format,
		FilePath: request.FilePath, LogPath: request.LogPath, SkipCheckDir: request.SkipCheckDir,
	})
	if err != nil {
		return "", "", draftStructuredColumns{}, err
	}
	return "v5", string(raw), draftStructuredColumns{}, nil
}

// marshalDraftStructuredColumns 将 v6 泛化配置的七个子结构分别序列化为结构化列值。
func marshalDraftStructuredColumns(config *store.ExportConfig) (draftStructuredColumns, error) {
	var columns draftStructuredColumns
	pairs := []struct {
		target *string
		value  any
	}{
		{&columns.ObjectScopeJSON, config.ObjectScope},
		{&columns.ContentSelectionJSON, config.ContentSelection},
		{&columns.DataFormatJSON, config.DataFormat},
		{&columns.OutputConfigJSON, config.OutputConfig},
		{&columns.PerformanceConfigJSON, config.PerformanceConfig},
		{&columns.FilterConfigJSON, config.FilterConfig},
		{&columns.DDLBehaviorJSON, config.DDLBehavior},
	}
	for _, pair := range pairs {
		raw, err := json.Marshal(pair.value)
		if err != nil {
			return draftStructuredColumns{}, err
		}
		*pair.target = string(raw)
	}
	return columns, nil
}

// createDataSource 从不渲染或持久化密码本身。
// 幂等摘要只记录是否提供过密码，避免保留由密码派生的哈希。
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
	request.SysUser = strings.TrimSpace(request.SysUser)
	password := []byte(request.Password)
	request.Password = ""
	defer credential.Zero(password)
	if len(password) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "PASSWORD_REQUIRED", "密码不能为空", false)
		return
	}
	// 可选的 sys 凭据必须成对提供（参考 ODC 数据源高级设置）。
	if (request.SysUser == "") != (request.SysPassword == "") {
		writeError(w, http.StatusUnprocessableEntity, "SYS_CREDENTIAL_REQUIRED_PAIR", "sys 账号与密码必须同时提供或同时留空", false)
		return
	}
	sysPassword := []byte(request.SysPassword)
	request.SysPassword = ""
	defer credential.Zero(sysPassword)
	dataSourceID, credentialID, err := newOpaqueID(), newOpaqueID(), error(nil)
	if dataSourceID == "" || credentialID == "" {
		err = errors.New("generate identifier")
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	// 可选的 sys 凭据加密（AAD 绑定 dataSourceID，与解密时一致）。
	var sysEnvelope *credential.Envelope
	var sysCredentialID string
	if request.SysUser != "" {
		sysCredentialID = newOpaqueID()
		if sysCredentialID == "" {
			writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
			return
		}
		encrypted, encryptErr := s.encryptor.Encrypt(s.keyID, credential.Reference{CredentialID: sysCredentialID, Revision: 1, SecretType: credential.SysPassword, DataSourceID: dataSourceID}, sysPassword)
		if encryptErr != nil {
			writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_ENCRYPTION_UNAVAILABLE", "凭据安全上下文不可用", true)
			return
		}
		defer credential.Zero(encrypted.Nonce)
		defer credential.Zero(encrypted.Ciphertext)
		sysEnvelope = &encrypted
	}
	envelope, err := s.encryptor.Encrypt(s.keyID, credential.Reference{CredentialID: credentialID, Revision: 1, SecretType: credential.DatabasePassword, DataSourceID: dataSourceID}, password)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_ENCRYPTION_UNAVAILABLE", "凭据安全上下文不可用", true)
		return
	}
	defer credential.Zero(envelope.Nonce)
	defer credential.Zero(envelope.Ciphertext)
	// 可选的 sys 凭据信封（创建时修订恒为 1）。
	var sysPasswordEnvelope *store.EncryptedDataSourcePassword
	if sysEnvelope != nil {
		sysPasswordEnvelope = &store.EncryptedDataSourcePassword{CredentialID: sysCredentialID, Revision: 1, KeyID: sysEnvelope.KeyID, Nonce: sysEnvelope.Nonce, Ciphertext: sysEnvelope.Ciphertext}
	}
	result, err := s.creator.CreateDataSource(r.Context(), store.DataSourceCreate{
		DataSourceID: dataSourceID, CredentialID: credentialID, CreatorSubjectID: principal.ID,
		DisplayName: request.DisplayName, NormalizedName: normalizeName(request.DisplayName),
		Environment: request.Environment, ConnectionKind: request.ConnectionKind, CompatibilityMode: request.CompatibilityMode,
		Host: request.Host, Port: request.Port, ClusterName: request.ClusterName, TenantName: request.TenantName, Username: request.Username, DefaultDatabase: request.DefaultDatabase,
		KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext,
		SysUser: request.SysUser, SysPassword: sysPasswordEnvelope,
		RequestID: requestID(w), IdempotencyKey: idempotencyKey, RequestDigest: createRequestDigest(request), CreatedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if errors.Is(err, store.ErrDataSourceNameUnavailable) {
		writeDataSourceNameUnavailable(w)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "DATA_SOURCE_CREATE_REJECTED", "数据源字段不符合要求", false)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID(w), "id": result.DataSourceID, "replayed": result.Replayed})
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
		Port: merged.Port, ClusterName: merged.ClusterName, TenantName: merged.TenantName, Username: merged.Username, DefaultDatabase: merged.DefaultDatabase,
		SysUser:   merged.SysUser,
		RequestID: requestID(w), UpdatedAt: time.Now().UTC(),
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
	// 可选的 sys 凭据更新（参考 ODC 数据源高级设置）：账号与密码同空表示清除，同非空表示设置/轮换。
	if request.SysUser != nil || request.SysPassword != nil {
		if s.credentials == nil || s.encryptor == nil || strings.TrimSpace(s.keyID) == "" {
			writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_UPDATE_NOT_CONFIGURED", "当前环境尚未配置凭据轮换依赖", false)
			return
		}
		if merged.SysUser == "" && *request.SysPassword == "" {
			// 清除 sys 凭据：账号已由 merge 置空，仓储将 ACTIVE 的 sys 修订标记 REVOKED。
			update.ClearSysCredential = true
		} else {
			sysPassword := []byte(*request.SysPassword)
			*request.SysPassword = ""
			defer credential.Zero(sysPassword)
			if len(sysPassword) == 0 {
				writeError(w, http.StatusUnprocessableEntity, "PASSWORD_REQUIRED", "密码不能为空", false)
				return
			}
			// 复用当前 sys 凭据版本以轮换，未配置过则新建凭据标识。
			sysCredentialID := current.SysCredentialID
			if sysCredentialID == "" {
				sysCredentialID = newOpaqueID()
				if sysCredentialID == "" {
					writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
					return
				}
			}
			sysEnvelope, encryptErr := s.encryptor.Encrypt(s.keyID, credential.Reference{CredentialID: sysCredentialID, Revision: current.SysCredentialRevision + 1, SecretType: credential.SysPassword, DataSourceID: dataSourceID}, sysPassword)
			if encryptErr != nil {
				writeError(w, http.StatusServiceUnavailable, "CREDENTIAL_ENCRYPTION_UNAVAILABLE", "凭据安全上下文不可用", true)
				return
			}
			defer credential.Zero(sysEnvelope.Nonce)
			defer credential.Zero(sysEnvelope.Ciphertext)
			update.SysUser = merged.SysUser
			update.SysPassword = &store.EncryptedDataSourcePassword{CredentialID: sysCredentialID, Revision: sysEnvelope.Reference.Revision, KeyID: sysEnvelope.KeyID, Nonce: sysEnvelope.Nonce, Ciphertext: sysEnvelope.Ciphertext}
		}
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
	if errors.Is(err, store.ErrDataSourceNameUnavailable) {
		writeDataSourceNameUnavailable(w)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "DATA_SOURCE_UPDATE_REJECTED", "数据源字段不符合要求", false)
		return
	}
	current.DisplayName, current.Environment, current.ConnectionKind, current.CompatibilityMode = merged.DisplayName, merged.Environment, merged.ConnectionKind, merged.CompatibilityMode
	current.Host, current.Port, current.ClusterName, current.TenantName, current.Username, current.DefaultDatabase = merged.Host, merged.Port, merged.ClusterName, merged.TenantName, merged.Username, merged.DefaultDatabase
	current.Revision, current.CredentialRevision = result.Revision, result.CredentialRevision
	if result.ConnectionTestInvalidated {
		current.State = "DISABLED"
		current.LastTestStatus = ""
		current.LastTestedAt = nil
		current.LastTestSafeSummaryJSON = ""
	}
	// PATCH 已完成同一对象的管理范围校验，因此成功响应复用可回显业务用户名的详情投影。
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": newDataSourceDetailResponse(current, true)})
}

func (r dataSourceUpdateRequest) hasChanges() bool {
	return r.DisplayName != nil || r.Environment != nil || r.ConnectionKind != nil || r.CompatibilityMode != nil || r.Host != nil || r.Port != nil || r.ClusterName != nil || r.TenantName != nil || r.Username != nil || r.DefaultDatabase != nil || r.Password != nil || r.SysUser != nil || r.SysPassword != nil
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
	if r.ClusterName != nil {
		merged.ClusterName = *r.ClusterName
	}
	if r.TenantName != nil {
		merged.TenantName = *r.TenantName
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
	// 可选的 sys 凭据（参考 ODC 数据源高级设置）：账号与密码必须同时更新；合并后的账号空值表示清除。
	if (r.SysUser == nil) != (r.SysPassword == nil) {
		return store.DataSourceSummary{}, errors.New("sys credential must be updated as a pair")
	}
	if r.SysUser != nil {
		merged.SysUser = strings.TrimSpace(*r.SysUser)
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
	if err := validateDraftVersionRouting(request); err != nil {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	normalized, err := normalizeExportDraftRequest(request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	preview, source, node, err := s.generateExportDraft(r.Context(), normalized)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, errGeneralizedGeneratorUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置泛化导出能力", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	configVersion, configJSON, structured, err := buildDraftPersistence(request, normalized)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	draftID := newOpaqueID()
	if draftID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	result, err := s.drafts.CreateExportDraft(r.Context(), store.ExportDraftCreate{ExportDraft: store.ExportDraft{
		DraftID: draftID, OwnerSubjectID: principal.ID, DataSourceID: source.DataSourceID, NodeID: node.NodeID, ToolVersion: preview.ToolVersion,
		MetadataVersion: preview.MetadataVersion, CapabilityVersion: preview.CapabilityVersion,
		ConfigVersion: configVersion, ConfigJSON: configJSON, ConfigFingerprint: preview.ConfigFingerprint,
		ObjectScopeJSON: structured.ObjectScopeJSON, ContentSelectionJSON: structured.ContentSelectionJSON,
		DataFormatJSON: structured.DataFormatJSON, OutputConfigJSON: structured.OutputConfigJSON,
		PerformanceConfigJSON: structured.PerformanceConfigJSON, FilterConfigJSON: structured.FilterConfigJSON,
		DDLBehaviorJSON:  structured.DDLBehaviorJSON,
		InvalidationJSON: `{}`, CreatedAt: now, UpdatedAt: now,
	}, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: exportDraftDigest(configJSON)})
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID(w), "id": result.DraftID, "replayed": result.Replayed})
}

// getExportDraft 仅允许草稿所有者读取，避免新增未确认的草稿共享规则。
func (s *Server) getExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	draft, ok := s.loadOwnedDraft(w, r, principal, draftID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": draftResponse(draft)})
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
	if err := validateDraftVersionRouting(request); err != nil {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	normalized, err := normalizeExportDraftRequest(request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	preview, _, _, err := s.generateExportDraft(r.Context(), normalized)
	if errors.Is(err, errGeneralizedGeneratorUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置泛化导出能力", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	configVersion, configJSON, structured, err := buildDraftPersistence(request, normalized)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	newRevision, err := s.drafts.UpdateDraft(r.Context(), store.DraftUpdate{
		DraftID: draftID, ExpectedRevision: expected, ConfigJSON: configJSON, ConfigFingerprint: preview.ConfigFingerprint, InvalidationJSON: `{}`, UpdatedAt: time.Now().UTC(),
		ConfigVersion:   configVersion,
		ObjectScopeJSON: structured.ObjectScopeJSON, ContentSelectionJSON: structured.ContentSelectionJSON,
		DataFormatJSON: structured.DataFormatJSON, OutputConfigJSON: structured.OutputConfigJSON,
		PerformanceConfigJSON: structured.PerformanceConfigJSON, FilterConfigJSON: structured.FilterConfigJSON,
		DDLBehaviorJSON: structured.DDLBehaviorJSON,
	})
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "DRAFT_REVISION_CONFLICT", "草稿已发生变化，请刷新后重试", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	draft.Revision, draft.ConfigJSON, draft.ConfigFingerprint, draft.ConfigVersion = newRevision, configJSON, preview.ConfigFingerprint, configVersion
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": draftResponse(draft)})
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
	normalized, err := draftNormalized(draft)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DRAFT_CONFIGURATION_UNAVAILABLE", "草稿配置暂时不可用", true)
		return
	}
	preview, _, _, err := s.generateExportDraft(r.Context(), normalized)
	if errors.Is(err, errGeneralizedGeneratorUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置泛化导出能力", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "command": preview.RedactedCommand, "argvTemplate": browserPreviewArgv(preview.ArgvTemplate), "configFingerprint": preview.ConfigFingerprint, "tokenEvidence": preview.TokenEvidence, "secretSourceSummary": preview.SecretSourceSummary})
}

// createExportPrecheck 固定当前草稿与凭据版本，之后的 Agent 只能领取该绑定。
// 有效期来自部署依赖，而不是由浏览器、Agent 或请求体提供。
func (s *Server) createExportPrecheck(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	if s.prechecks == nil || s.csrf == nil || s.precheckTTL <= 0 || (s.agentPrechecks == nil && s.coordinator == nil) {
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
	normalized, err := draftNormalized(draft)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DRAFT_CONFIGURATION_UNAVAILABLE", "草稿配置暂时不可用", true)
		return
	}
	preview, _, node, err := s.generateExportDraft(r.Context(), normalized)
	if errors.Is(err, errGeneralizedGeneratorUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置泛化导出能力", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_DRAFT_REJECTED", "导出草稿不符合首条切片要求", false)
		return
	}
	// EX-I6 门禁（2026-08-10）：对象存储输出的存储专用预检查（凭据、网络、权限、空间）尚未完成，
	// 固定预检查只接受本地绝对路径；对象存储草稿返回稳定的功能门禁错误，不把存储 URI 伪装成本地路径。
	if normalized.OutputKind != "" && normalized.OutputKind != "LOCAL" {
		writeError(w, http.StatusUnprocessableEntity, "STORAGE_PRECHECK_UNAVAILABLE", "对象存储输出的存储专用预检查尚未完成，暂不能发起固定预检查", false)
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
	if s.agentPrechecks == nil {
		// 旧合成协议仍由内存协调器持有租约；正式受认证路径由 SQLite 事务冻结事实。
		if node.FactsRevision < 1 || s.coordinator.SchedulePrecheck(agentstate.PrecheckBinding{PrecheckID: precheckID, NodeID: draft.NodeID, DraftRevision: draft.Revision, ConfigFingerprint: preview.ConfigFingerprint, CredentialRevision: credentialReference.Revision, NodeFactsVersion: node.FactsRevision}) != nil {
			writeError(w, http.StatusServiceUnavailable, "PRECHECK_SCHEDULING_UNAVAILABLE", "预检查暂时无法排队", true)
			return
		}
	}
	result, err := s.prechecks.CreatePrecheck(r.Context(), store.PrecheckCreate{PrecheckRun: store.PrecheckRun{
		PrecheckID: precheckID, DraftID: draft.DraftID, DraftRevision: draft.Revision, ConfigFingerprint: preview.ConfigFingerprint,
		DataSourceID: draft.DataSourceID, CredentialID: credentialReference.CredentialID, CredentialRevision: credentialReference.Revision,
		NodeID: draft.NodeID, CreatedAt: now, ValidUntil: now.Add(s.precheckTTL),
	}, CreatorSubjectID: principal.ID, RequestID: requestID(w), IdempotencyKey: key, RequestDigest: precheckDigest(draft, preview.ConfigFingerprint)})
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
	writeJSON(w, http.StatusAccepted, map[string]any{"requestId": requestID(w), "id": result.PrecheckID, "replayed": result.Replayed})
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
	results := browserPrecheckResults(run.Results)
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": map[string]any{
		"id": run.PrecheckID, "draftId": run.DraftID, "draftRevision": run.DraftRevision, "configFingerprint": run.ConfigFingerprint,
		"nodeId": run.NodeID, "status": run.Status, "integrityStatus": run.IntegrityStatus, "results": results, "validUntil": run.ValidUntil.Format(time.RFC3339Nano),
	}})
}

// browserPrecheckResults 为尚未完成的预检查提供固定六项未知投影。
// 浏览器据此只能展示“尚未得到 Agent 结论”，不能把空数组误判为检查通过。
func browserPrecheckResults(results []store.PrecheckCheckResult) []store.PrecheckCheckResult {
	if len(results) > 0 {
		return append([]store.PrecheckCheckResult{}, results...)
	}
	unknownCodes := map[agentpreflight.CheckID]string{
		agentpreflight.CheckDatabaseConnectivity: "DATABASE_CONNECTION_UNAVAILABLE",
		agentpreflight.CheckObjectAccess:         "OBJECT_ACCESS_UNAVAILABLE",
		agentpreflight.CheckToolEnvironment:      "TOOL_RUNTIME_UNAVAILABLE",
		agentpreflight.CheckOutputPath:           "OUTPUT_PATH_UNAVAILABLE",
		agentpreflight.CheckOutputEmpty:          "OUTPUT_PATH_UNAVAILABLE",
		agentpreflight.CheckAvailableSpace:       "OUTPUT_SPACE_UNAVAILABLE",
	}
	projected := make([]store.PrecheckCheckResult, 0, len(agentpreflight.FixedChecks()))
	for _, check := range agentpreflight.FixedChecks() {
		projected = append(projected, store.PrecheckCheckResult{Check: string(check), Status: string(agentpreflight.StatusUnknown), EvidenceCode: unknownCodes[check]})
	}
	return projected
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

// errGeneralizedGeneratorUnavailable 表示泛化能力请求缺少对应的生成器依赖，必须失败关闭。
var errGeneralizedGeneratorUnavailable = errors.New("generalized export generator is not configured")

func (s *Server) generateExportDraft(ctx context.Context, input normalizedExportDraft) (commandgen.Result, store.DataSourceSummary, ExecutionNodeFact, error) {
	if input.DataSourceID == "" || input.NodeID == "" || input.Database == "" || input.FilePath == "" || (input.ScopeKind == "SPECIFIED" && len(input.Objects) == 0) {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("export draft fields are incomplete")
	}
	source, err := s.dataSource.GetDataSourceSummary(ctx, input.DataSourceID)
	if err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	if source.State != "ENABLED" || source.LastTestStatus != "SUCCEEDED" {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("export data source is not eligible")
	}
	// 闪回时间点仅适用于 Oracle 兼容模式（EX-F079），MySQL 模式失败关闭。
	if input.FlashbackTimestamp != "" && source.CompatibilityMode != "ORACLE" {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("flashback timestamp requires oracle compatibility mode")
	}
	node, err := s.nodes.GetExecutionNodeFact(ctx, input.NodeID)
	if err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	// EX-I6 对象存储（2026-08-07）：本地输出要求节点平台绝对路径；
	// 对象存储输出只接受归一化阶段已校验的受控 URI，日志路径仍为节点本地路径。
	if (input.OutputKind == "" || input.OutputKind == "LOCAL") && !outputpath.IsExportOutputPath(string(node.Platform), input.FilePath) {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("export output path is invalid")
	}
	if input.LogPath != "" && !outputpath.IsExportOutputPath(string(node.Platform), input.LogPath) {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("export log path is invalid")
	}
	credentialReference, err := s.credentials.GetDataSourceCredentialReference(ctx, input.DataSourceID)
	if err != nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, err
	}
	commandUsername, ok := store.PrivateODPCommandIdentity(source.Username, source.TenantName, source.ClusterName)
	if !ok {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("private ODP command identity is invalid")
	}
	connectionFields := []commandgen.FieldInput{
		{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: source.Host}}, {Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: int64(source.Port)}}, {Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: commandUsername}}, {Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: credentialReference.CredentialID, Revision: credentialReference.Revision}}}, {Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.Database}},
	}
	requestBase := commandgen.Request{Tool: "OBDUMPER", ToolVersion: "4.3.5-RELEASE", ConnectionKind: commandgen.ConnectionKind(source.ConnectionKind), DataSourceFactVersion: fmt.Sprintf("ds-rev-%d", source.Revision), NodeFactVersion: node.FactsVersion, TargetPlatform: node.Platform, OutputKind: input.OutputKind}
	// 冻结单表 CSV 形状继续使用 v5 元数据与已验证能力，保证指纹与 argv 逐字节不变。
	if input.isFrozenSingleTableCSV() {
		fields := append(append([]commandgen.FieldInput(nil), connectionFields...),
			commandgen.FieldInput{Name: "--table", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.Objects[0]}},
			commandgen.FieldInput{Name: "--csv", Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}},
			commandgen.FieldInput{Name: "--file-path", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.FilePath}},
		)
		if input.LogPath != "" {
			fields = append(fields, commandgen.FieldInput{Name: "--log-path", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.LogPath}})
		}
		fields = append(fields, commandgen.FieldInput{Name: "--skip-check-dir", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: input.SkipCheckDir}})
		requestBase.MetadataVersion = "obdumper-4.3.5-slice-v5"
		requestBase.CapabilityVersion = "export-odp-single-table-csv-v1"
		requestBase.Fields = fields
		result, err := s.generator.Generate(requestBase)
		return result, source, node, err
	}
	if s.generalGenerator == nil {
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errGeneralizedGeneratorUnavailable
	}
	fields := append([]commandgen.FieldInput(nil), connectionFields...)
	switch input.ScopeKind {
	case "ALL":
		fields = append(fields, commandgen.FieldInput{Name: "--all", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	case "SPECIFIED":
		objectParameter := "--table"
		if input.ObjectType == "VIEW" {
			objectParameter = "--view"
		}
		fields = append(fields, commandgen.FieldInput{Name: objectParameter, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: strings.Join(input.Objects, ",")}})
	default:
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("export draft scope is unsupported")
	}
	if len(input.ExcludeTables) != 0 {
		fields = append(fields, commandgen.FieldInput{Name: "--exclude-table", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: strings.Join(input.ExcludeTables, ",")}})
	}
	switch input.ContentKind {
	case "DATA_ONLY":
		// EX-I4/EX-I5：数据格式单选，控制面按归一化结果提供对应格式标志。
		formatParameter := "--csv"
		switch input.Format {
		case "CUT":
			formatParameter = "--cut"
		case "SQL":
			formatParameter = "--sql"
		case "POS":
			formatParameter = "--pos"
		case "PARQUET":
			formatParameter = "--par"
		case "ORC":
			formatParameter = "--orc"
		case "AVRO":
			formatParameter = "--avro"
		}
		fields = append(fields, commandgen.FieldInput{Name: formatParameter, Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	case "DDL_ONLY":
		fields = append(fields, commandgen.FieldInput{Name: "--ddl", Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	case "DDL_AND_DATA":
		fields = append(fields,
			commandgen.FieldInput{Name: "--ddl", Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}},
			commandgen.FieldInput{Name: "--csv", Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}},
		)
	default:
		return commandgen.Result{}, store.DataSourceSummary{}, ExecutionNodeFact{}, errors.New("export draft content kind is unsupported")
	}
	// EX-I7 DDL 行为（2026-08-10）：前置 DROP 与保留 Schema 仅随 DDL 内容发射（归一化已保证非 DDL 内容携带即失败关闭）。
	if input.DropObject {
		fields = append(fields, commandgen.FieldInput{Name: "--drop-object", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	if input.RetainSchema {
		fields = append(fields, commandgen.FieldInput{Name: "--retain-schema", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	// EX-I7 紧凑 Schema（2026-08-11 受控实测定版）：仅随 DDL 内容发射（归一化已保证非 DDL 内容携带即失败关闭）。
	if input.CompactSchema {
		fields = append(fields, commandgen.FieldInput{Name: "--compact-schema", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	fields = append(fields, commandgen.FieldInput{Name: "--file-path", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.FilePath}})
	if input.LogPath != "" {
		fields = append(fields, commandgen.FieldInput{Name: "--log-path", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.LogPath}})
	}
	if input.ControlFilePath != "" {
		// EX-I4 POS 定版：控制文件目录（--ctl-path）只随 POS 格式发射（归一化已保证）。
		fields = append(fields, commandgen.FieldInput{Name: "--ctl-path", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.ControlFilePath}})
	}
	if input.TmpPath != "" {
		// EX-I6 对象存储：Multipart 本地临时分块目录（--tmp-path）。
		fields = append(fields, commandgen.FieldInput{Name: "--tmp-path", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.TmpPath}})
	}
	fields = append(fields, commandgen.FieldInput{Name: "--skip-check-dir", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: input.SkipCheckDir}})
	// EX-I4：序列化选项按格式适用性发射；越界字段已在归一化阶段失败关闭。
	switch input.Format {
	case "CSV":
		csv := input.CsvOptions
		if csv.SkipHeader {
			fields = append(fields, commandgen.FieldInput{Name: "--skip-header", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
		for _, option := range []struct {
			name  string
			value string
		}{
			{"--column-separator", csv.ColumnSeparator},
			{"--column-quote", csv.ColumnQuote},
			{"--column-quote-mode", csv.ColumnQuoteMode},
			{"--escape-character", csv.EscapeCharacter},
			{"--line-separator", csv.LineSeparator},
			{"--null-string", csv.NullString},
			{"--file-encoding", csv.FileEncoding},
		} {
			if option.value != "" {
				fields = append(fields, commandgen.FieldInput{Name: option.name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: option.value}})
			}
		}
		if csv.WithTrim {
			fields = append(fields, commandgen.FieldInput{Name: "--with-trim", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
	case "CUT":
		csv := input.CsvOptions
		for _, option := range []struct {
			name  string
			value string
		}{
			{"--column-splitter", csv.ColumnSplitter},
			{"--escape-character", csv.EscapeCharacter},
			{"--line-separator", csv.LineSeparator},
			{"--null-string", csv.NullString},
			{"--file-encoding", csv.FileEncoding},
		} {
			if option.value != "" {
				fields = append(fields, commandgen.FieldInput{Name: option.name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: option.value}})
			}
		}
		if csv.WithTrim {
			fields = append(fields, commandgen.FieldInput{Name: "--with-trim", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
		if input.CutOptions.TrailDelimiter {
			fields = append(fields, commandgen.FieldInput{Name: "--trail-delimiter", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
		if input.CutOptions.RemoveNewline {
			fields = append(fields, commandgen.FieldInput{Name: "--remove-newline", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
	case "SQL":
		csv := input.CsvOptions
		for _, option := range []struct {
			name  string
			value string
		}{
			{"--line-separator", csv.LineSeparator},
			{"--file-encoding", csv.FileEncoding},
		} {
			if option.value != "" {
				fields = append(fields, commandgen.FieldInput{Name: option.name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: option.value}})
			}
		}
	case "POS":
		// EX-I4 POS 定版：首版无序列化选项，只有 --pos 与 --ctl-path；越界已在归一化失败关闭。
	case "PARQUET", "ORC", "AVRO":
		// EX-I5 结构化格式：只发射官方格式表列出的文件编码；越界已在归一化失败关闭。
		if csv := input.CsvOptions; csv.FileEncoding != "" {
			fields = append(fields, commandgen.FieldInput{Name: "--file-encoding", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: csv.FileEncoding}})
		}
	}
	if input.NoNestedDir {
		fields = append(fields, commandgen.FieldInput{Name: "--no-nested-dir", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	if input.MaxFileSize != nil {
		fields = append(fields, commandgen.FieldInput{Name: "--max-file-size", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: *input.MaxFileSize}})
	}
	if input.RetainEmptyFiles {
		fields = append(fields, commandgen.FieldInput{Name: "--retain-empty-files", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	if input.Compress {
		fields = append(fields, commandgen.FieldInput{Name: "--compress", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	if input.CompressionAlgo != "" {
		fields = append(fields, commandgen.FieldInput{Name: "--compression-algo", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.CompressionAlgo}})
	}
	// EX-I7 压缩等级（2026-08-10）：显式传值时发射（归一化已按算法范围校验）。
	if input.CompressionLevel != nil {
		fields = append(fields, commandgen.FieldInput{Name: "--compression-level", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: *input.CompressionLevel}})
	}
	if input.QuerySql != "" {
		fields = append(fields, commandgen.FieldInput{Name: "--query-sql", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.QuerySql}})
	}
	// EX-I7 条件筛选（2026-08-11 受控实测定版）：--where 显式传值时发射（与 --query-sql 互斥已校验）。
	if input.Where != "" {
		fields = append(fields, commandgen.FieldInput{Name: "--where", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.Where}})
	}
	// EX-I7 一致性（2026-08-11 受控实测定版）：--snapshot 一致性快照为无值开关。
	if input.Snapshot {
		fields = append(fields, commandgen.FieldInput{Name: "--snapshot", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	if len(input.IncludeColumns) != 0 {
		fields = append(fields, commandgen.FieldInput{Name: "--include-column-names", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: strings.Join(input.IncludeColumns, ",")}})
	}
	if len(input.ExcludeColumns) != 0 {
		fields = append(fields, commandgen.FieldInput{Name: "--exclude-column-names", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: strings.Join(input.ExcludeColumns, ",")}})
	}
	if input.ExcludeVirtualColumns {
		fields = append(fields, commandgen.FieldInput{Name: "--exclude-virtual-columns", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	if input.FlashbackScn != nil {
		fields = append(fields, commandgen.FieldInput{Name: "--flashback-scn", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: *input.FlashbackScn}})
	}
	if input.FlashbackTimestamp != "" {
		fields = append(fields, commandgen.FieldInput{Name: "--flashback-timestamp", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.FlashbackTimestamp}})
	}
	for _, option := range []struct {
		name  string
		value *int
	}{
		{"--thread", input.Thread},
		{"--page-size", input.PageSize},
		{"--parallel-macro", input.ParallelMacro},
		{"--fetch-size", input.FetchSize},
	} {
		if option.value != nil {
			fields = append(fields, commandgen.FieldInput{Name: option.name, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: int64(*option.value)}})
		}
	}
	if input.JvmMemory != "" {
		fields = append(fields, commandgen.FieldInput{Name: "--mem", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.JvmMemory}})
	}
	// EX-I7 文件拆分（2026-08-10）：--block-size 显式传值时发射（归一化已按可读格式能力校验）。
	if input.BlockSize != "" {
		fields = append(fields, commandgen.FieldInput{Name: "--block-size", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: input.BlockSize}})
	}
	requestBase.MetadataVersion = "obdumper-4.3.5-slice-v6"
	requestBase.CapabilityVersion = draftCapability(input)
	requestBase.Fields = fields
	result, err := s.generalGenerator.Generate(requestBase)
	return result, source, node, err
}

func draftResponse(draft store.ExportDraft) map[string]any {
	return map[string]any{"id": draft.DraftID, "dataSourceId": draft.DataSourceID, "nodeId": draft.NodeID, "revision": draft.Revision, "toolVersion": draft.ToolVersion, "metadataVersion": draft.MetadataVersion, "capabilityVersion": draft.CapabilityVersion, "configVersion": draft.ConfigVersion, "config": json.RawMessage(draft.ConfigJSON), "configFingerprint": draft.ConfigFingerprint, "invalidation": json.RawMessage(draft.InvalidationJSON)}
}

// browserPreviewArgv 返回命令预览对应的独立参数副本。
// 密码不属于 argv；其余参数可按当前产品规则展示，但该副本仍不能作为 Agent 执行输入。
func browserPreviewArgv(argv []string) []string {
	return append([]string(nil), argv...)
}

// exportDraftDigest 基于持久化 config_json 计算幂等摘要。
// v5 的 config_json 与首条切片字节级一致，因此存量幂等记录的摘要保持可比。
func exportDraftDigest(configJSON string) string {
	digest := sha256.Sum256([]byte(configJSON))
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
		RequestID: requestID(w), ChangedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "DATA_SOURCE_STATE_CONFLICT", "数据源状态已发生变化，请刷新后重试", true)
		return
	}
	if errors.Is(err, store.ErrDataSourceConnectionTestRequired) {
		writeError(w, http.StatusUnprocessableEntity, "DATA_SOURCE_CONNECTION_TEST_REQUIRED", "当前连接配置尚未通过基础连接测试，不能启用", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_STATE_UNAVAILABLE", "数据源状态暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "id": dataSourceID, "state": result.State,
		"revision": result.Revision, "replayed": result.Replayed,
	})
}

// deleteDataSource 先完成 CSRF、对象范围和版本校验，再交由仓储依据历史引用决定删除或归档。
// 不向调用方暴露引用类型、数量或归档对象信息，避免借由失败路径枚举历史事实。
func (s *Server) deleteDataSource(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID string) {
	if s.deleter == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源删除依赖", false)
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
	result, err := s.deleter.DeleteOrArchiveDataSource(r.Context(), store.DataSourceDeletion{
		DataSourceID: dataSourceID, ActorSubjectID: principal.ID, ExpectedRevision: expectedRevision,
		RequestID: requestID(w), DeletedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "DATA_SOURCE_REVISION_CONFLICT", "数据源已发生变化，请刷新后重试", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_DELETE_UNAVAILABLE", "数据源删除暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "id": dataSourceID, "outcome": result.Outcome, "revision": result.Revision,
	})
}

// dataSourceConnectionTestRequest 只允许浏览器选择一个已授权节点。
// 连接地址、用户名、密码、URL、命令和任意 SQL 都不能通过该请求进入 Agent 协议。
type dataSourceConnectionTestRequest struct {
	NodeID string `json:"nodeId"`
}

// testDataSourceConnection 在短事务中冻结一次节点绑定的测试意图。
// 控制面只做授权、版本、幂等与租约协调，绝不在此路径连接数据库或读取明文凭据。
func (s *Server) testDataSourceConnection(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID string) {
	if s.connectionTests == nil || s.authorizer == nil || s.csrf == nil || s.connectionTestTTL <= 0 || s.heartbeatTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "AGENT_CONNECTION_TEST_UNAVAILABLE", "当前环境尚未配置节点侧连接测试", true)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceWrite, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	expectedRevision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的数据版本号", false)
		return
	}
	var request dataSourceConnectionTestRequest
	if !decodeBrowserJSON(w, r, &request) {
		return
	}
	request.NodeID = strings.TrimSpace(request.NodeID)
	if !validDataSourceConnectionTestPathID(request.NodeID) {
		writeErrorWithFields(w, http.StatusUnprocessableEntity, "AGENT_CONNECTION_TEST_FIELDS_INVALID", "连接测试节点不可用", false, []fieldErrorResponse{{Field: "nodeId", Code: "CONNECTION_TEST_NODE_REQUIRED", Message: "请选择一个可用的执行节点"}})
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeUse, request.NodeID) != nil {
		notFound(w, r)
		return
	}
	connectionTestID := newOpaqueID()
	if connectionTestID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	verificationSource := "G2_SYNTHETIC"
	if s.agentJDBCConnectionTestEnabled {
		verificationSource = "AGENT_JDBC"
	}
	result, err := s.connectionTests.RequestDataSourceConnectionTest(r.Context(), store.DataSourceConnectionTestCreate{
		ConnectionTestID: connectionTestID, DataSourceID: dataSourceID, CreatorSubjectID: principal.ID,
		ExpectedDataSourceRevision: expectedRevision, NodeID: request.NodeID, VerificationSource: verificationSource,
		RequestID: requestID(w), IdempotencyKey: key, RequestDigest: dataSourceConnectionTestCreateDigest(dataSourceID, expectedRevision, request.NodeID),
		CreatedAt: now, HeartbeatFreshAfter: now.Add(-s.heartbeatTTL), ValidUntil: now.Add(s.connectionTestTTL),
	})
	if errors.Is(err, store.ErrDataSourceNotFound) || errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "DATA_SOURCE_REVISION_CONFLICT", "数据源已发生变化，请刷新后重试", false)
		return
	}
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if errors.Is(err, store.ErrDataSourceConnectionTestInvalid) || errors.Is(err, store.ErrDataSourceConnectionTestLeaseRejected) {
		writeErrorWithFields(w, http.StatusUnprocessableEntity, "AGENT_CONNECTION_TEST_NODE_UNAVAILABLE", "所选执行节点当前无法接收连接测试", false, []fieldErrorResponse{{Field: "nodeId", Code: "CONNECTION_TEST_NODE_UNAVAILABLE", Message: "请选择已关联且在线的空闲执行节点"}})
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_CONNECTION_TEST_UNAVAILABLE", "当前无法创建节点侧连接测试", true)
		return
	}
	run, err := s.connectionTests.GetDataSourceConnectionTestRun(r.Context(), result.ConnectionTestID)
	if err != nil || run.DataSourceID != dataSourceID || run.NodeID != request.NodeID {
		writeError(w, http.StatusServiceUnavailable, "AGENT_CONNECTION_TEST_UNAVAILABLE", "当前无法核对连接测试状态", true)
		return
	}
	status := http.StatusAccepted
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(w), "item": newDataSourceConnectionTestResponse(run), "replayed": result.Replayed})
}

// getDataSourceConnectionTest 仅返回当前数据源范围内的无秘密测试投影。
// 单节点结果不会经此接口推导其他节点、对象权限、性能或导出任务可行性。
func (s *Server) getDataSourceConnectionTest(w http.ResponseWriter, r *http.Request, principal identity.Principal, connectionTestID string) {
	if s.connectionTests == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_CONNECTION_TEST_UNAVAILABLE", "当前环境尚未配置节点侧连接测试", true)
		return
	}
	run, err := s.connectionTests.GetDataSourceConnectionTestRun(r.Context(), connectionTestID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_CONNECTION_TEST_UNAVAILABLE", "当前无法读取连接测试状态", true)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, run.DataSourceID) != nil {
		notFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": newDataSourceConnectionTestResponse(run)})
}

func dataSourceConnectionTestCreateDigest(dataSourceID string, revision int64, nodeID string) string {
	payload := fmt.Sprintf("%s|%d|%s|DATA_SOURCE_CONNECTION_TEST", dataSourceID, revision, nodeID)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

// exportTaskSubmitRequest 只接收已创建预检查的标识，不能从浏览器接收命令或任务快照。
type exportTaskSubmitRequest struct {
	PrecheckID string `json:"precheckId"`
}

// submitExportDraft 将已通过的预检查与当前草稿重新核对后冻结为任务。
// G2 仍仅进入内存协调器；受控 Windows 本机 MVP 执行模式改由已认证 Agent 从 SQLite 原子领取，绝不由浏览器启动工具。
func (s *Server) submitExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, draftID string) {
	if s.tasks == nil || s.csrf == nil || (!s.realExecutionEnabled && s.coordinator == nil) {
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
	normalized, err := draftNormalized(draft)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DRAFT_CONFIGURATION_UNAVAILABLE", "草稿配置暂时不可用", true)
		return
	}
	preview, _, _, err := s.generateExportDraft(r.Context(), normalized)
	if errors.Is(err, errGeneralizedGeneratorUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置泛化导出能力", false)
		return
	}
	if err != nil || preview.ConfigFingerprint != run.ConfigFingerprint {
		writeError(w, http.StatusUnprocessableEntity, "PRECHECK_REQUIRED", "草稿或预检查已失效，请重新预检查", false)
		return
	}
	// EX-I6 门禁（2026-08-10）：对象存储输出的任务提交同样保持功能门禁阻断，
	// 与预检查门禁一致，存储专用预检查完成前不允许冻结任务。
	if normalized.OutputKind != "" && normalized.OutputKind != "LOCAL" {
		writeError(w, http.StatusUnprocessableEntity, "STORAGE_PRECHECK_UNAVAILABLE", "对象存储输出的存储专用预检查尚未完成，暂不能提交任务", false)
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
		CapabilityVersion: preview.CapabilityVersion, SnapshotVersion: draftSnapshotVersion(draft.ConfigVersion), SnapshotJSON: draft.ConfigJSON, PlannedArgvJSON: string(argv),
		PlannedCommandRedacted: preview.RedactedCommand, RequestID: requestID(w), SubmittedAt: now,
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
	if !s.realExecutionEnabled {
		if err := s.coordinator.Schedule(agentstate.TaskSchedule{TaskID: result.TaskID, NodeID: draft.NodeID}); err != nil {
			writeError(w, http.StatusServiceUnavailable, "TASK_SCHEDULING_UNAVAILABLE", "任务已冻结但暂时无法进入合成队列", true)
			return
		}
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(w), "id": result.TaskID, "replayed": result.Replayed, "state": "WAITING_SCHEDULE", "realExecutionEnabled": s.realExecutionEnabled})
}

// draftSnapshotVersion 把草稿配置版本映射为任务快照版本。
// v5 草稿冻结 v1 扁平快照；v6 草稿冻结 v2 泛化快照；未知版本失败关闭。
func draftSnapshotVersion(configVersion string) string {
	if configVersion == "v6" {
		return "v2"
	}
	return "v1"
}

// parseTaskReadPath 只将固定读取后缀解释为任务只读投影。
// 任务标识和资源后缀必须分别完整匹配，避免路径被扩展成任意子资源入口。
func parseTaskReadPath(path string) (string, string, bool) {
	const prefix = "/api/v1/tasks/"
	value := strings.TrimPrefix(path, prefix)
	if value == "" || value == path {
		return "", "", false
	}
	for suffix, projection := range map[string]string{
		"/snapshot":         "snapshot",
		"/command-evidence": "command-evidence",
		"/execution":        "execution",
		"/logs/stream":      "logs-stream",
		"/logs":             "logs",
	} {
		if strings.HasSuffix(value, suffix) {
			taskID := strings.TrimSuffix(value, suffix)
			return taskID, projection, taskID != "" && !strings.Contains(taskID, "/")
		}
	}
	return value, "overview", !strings.Contains(value, "/")
}

// taskOverviewResponse 是任务详情页头所需的最小不可变标识，不混入配置、命令或执行状态。
type taskOverviewResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	DataSourceID string `json:"dataSourceId"`
	NodeID       string `json:"nodeId"`
	PrecheckID   string `json:"precheckId"`
	SubmittedAt  string `json:"submittedAt"`
}

// taskSnapshotResponse 只返回当前切片可安全解释的冻结配置事实。
// 输出和日志路径、凭据引用、完整原始快照均不下发，后续字段必须先完成专项脱敏规则。
// 派生关系字段在本切片始终为空，待 EX-I8 实现后才会输出。
type taskSnapshotResponse struct {
	Type              string `json:"type"`
	SnapshotVersion   string `json:"snapshotVersion"`
	DataSourceID      string `json:"dataSourceId"`
	NodeID            string `json:"nodeId"`
	PrecheckID        string `json:"precheckId"`
	ObjectSummary     string `json:"objectSummary,omitempty"`
	Format            string `json:"format"`
	ConfigFingerprint string `json:"configFingerprint"`
	ToolVersion       string `json:"toolVersion"`
	MetadataVersion   string `json:"metadataVersion"`
	CapabilityVersion string `json:"capabilityVersion"`
	ParentTaskID      string `json:"parentTaskId,omitempty"`
	DerivedFromTaskID string `json:"derivedFromTaskId,omitempty"`
	TemplateID        string `json:"templateId,omitempty"`
}

// taskCommandEvidenceResponse 只允许浏览器读取提交时冻结的脱敏计划命令。
// 实际启动 argv、秘密槽位和未验证的进程载荷不能通过该接口返回。
type taskCommandEvidenceResponse struct {
	Kind      string `json:"kind"`
	Command   string `json:"command"`
	Redaction string `json:"redaction"`
}

// taskExecutionResponse 只携带平台已经确认的执行状态和时间事实。
// 阶段、进度和结果解析尚无可靠映射时必须显式标为 UNAVAILABLE。
type taskExecutionResponse struct {
	State                  string `json:"state"`
	ExecutionID            string `json:"executionId,omitempty"`
	ReconciliationRequired bool   `json:"reconciliationRequired"`
	StageEvidence          string `json:"stageEvidence"`
	ProgressEvidence       string `json:"progressEvidence"`
	StartedAt              string `json:"startedAt,omitempty"`
	FinishedAt             string `json:"finishedAt,omitempty"`
	UpdatedAt              string `json:"updatedAt"`
}

// loadAuthorizedTaskSummary 对每个任务只读投影重新执行服务端范围校验。
// 请求之间不能复用浏览器端的“已授权”结论，避免权限变更后继续泄露冻结事实。
func (s *Server) loadAuthorizedTaskSummary(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) (store.TaskSummary, bool) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置任务查询依赖", false)
		return store.TaskSummary{}, false
	}
	summary, err := s.tasks.GetAuthorizedTaskSummary(r.Context(), taskID, principal.ID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return store.TaskSummary{}, false
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_QUERY_UNAVAILABLE", "任务暂时不可用", true)
		return store.TaskSummary{}, false
	}
	return summary, true
}

// getTask 只返回任务详情页头的最小安全标识。
func (s *Server) getTask(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	summary, ok := s.loadAuthorizedTaskSummary(w, r, principal, taskID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": taskOverviewResponse{
		ID: summary.TaskID, Type: "OBDUMPER_EXPORT", DataSourceID: summary.DataSourceID, NodeID: summary.NodeID,
		PrecheckID: summary.PrecheckID, SubmittedAt: summary.SubmittedAt.Format(time.RFC3339Nano),
	}})
}

// getTaskSnapshot 返回已提交配置的最小安全快照，不把 SQLite 中的完整 JSON 透传给浏览器。
func (s *Server) getTaskSnapshot(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	summary, ok := s.loadAuthorizedTaskSummary(w, r, principal, taskID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": taskSnapshotResponse{
		Type: "OBDUMPER_EXPORT", SnapshotVersion: summary.SnapshotVersion, DataSourceID: summary.DataSourceID, NodeID: summary.NodeID, PrecheckID: summary.PrecheckID,
		ObjectSummary: taskObjectSummary(summary.Database, summary.Table), Format: summary.Format,
		ConfigFingerprint: summary.ConfigFingerprint, ToolVersion: summary.ToolVersion, MetadataVersion: summary.MetadataVersion,
		CapabilityVersion: summary.CapabilityVersion,
	}})
}

// getTaskCommandEvidence 返回默认脱敏的计划命令；实际命令证据尚未建立，不能伪造或回退为执行 argv。
func (s *Server) getTaskCommandEvidence(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	summary, ok := s.loadAuthorizedTaskSummary(w, r, principal, taskID)
	if !ok {
		return
	}
	if !safePlannedCommandEvidence(summary.PlannedCommandRedacted) {
		writeError(w, http.StatusServiceUnavailable, "TASK_COMMAND_EVIDENCE_UNAVAILABLE", "任务命令证据不符合展示安全规则", false)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": taskCommandEvidenceResponse{
		Kind: "PLANNED", Command: summary.PlannedCommandRedacted, Redaction: "PASSWORD_ONLY",
	}})
}

// safePlannedCommandEvidence 在控制面输出前再次验证密码占位符。
// 即使历史任务记录被意外污染，也不能把 `-p` 的实际值或长参数密码形式发送到浏览器。
func safePlannedCommandEvidence(command string) bool {
	if len(command) == 0 || len(command) > 64<<10 {
		return false
	}
	fields := strings.Fields(command)
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if strings.HasPrefix(strings.ToLower(field), "--password") || strings.HasPrefix(strings.ToLower(field), "password=") {
			return false
		}
		if field == "-p" {
			if index+1 >= len(fields) || fields[index+1] != "******" {
				return false
			}
			index++
			continue
		}
		if strings.HasPrefix(field, "-p") {
			return false
		}
	}
	return true
}

// getTaskExecution 返回可复核的状态投影，不解析或返回进程事件与结果摘要原文。
func (s *Server) getTaskExecution(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	summary, ok := s.loadAuthorizedTaskSummary(w, r, principal, taskID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": taskExecutionResponse{
		State: summary.State, ExecutionID: summary.ExecutionID, ReconciliationRequired: summary.ReconciliationRequired,
		StageEvidence: "UNAVAILABLE", ProgressEvidence: "UNAVAILABLE", StartedAt: optionalTaskTime(summary.StartedAt),
		FinishedAt: optionalTaskTime(summary.FinishedAt), UpdatedAt: summary.UpdatedAt.Format(time.RFC3339Nano),
	}})
}

const taskListDefaultPageSize = 10

type taskListCursor struct {
	SubjectDigest string `json:"subjectDigest"`
	SubmittedAt   string `json:"submittedAt"`
	TaskID        string `json:"taskId"`
}

// taskListResponse 是任务中心允许展示的最小字段集合。
// 阶段和进度没有可靠证据时只返回 UNAVAILABLE，不能从状态或时间猜测百分比。
type taskListResponse struct {
	ID                     string `json:"id"`
	Type                   string `json:"type"`
	DataSourceID           string `json:"dataSourceId"`
	ObjectSummary          string `json:"objectSummary,omitempty"`
	State                  string `json:"state"`
	StageEvidence          string `json:"stageEvidence"`
	ProgressEvidence       string `json:"progressEvidence"`
	ReconciliationRequired bool   `json:"reconciliationRequired"`
	NodeID                 string `json:"nodeId"`
	OwnedByCurrentUser     bool   `json:"ownedByCurrentUser"`
	SubmittedAt            string `json:"submittedAt"`
	StartedAt              string `json:"startedAt,omitempty"`
	FinishedAt             string `json:"finishedAt,omitempty"`
	UpdatedAt              string `json:"updatedAt"`
}

// listTasks 只返回当前主体拥有或按数据源明确授权的任务。
// 总页数同样仅基于该授权范围计算，不能作为全局任务数量或无权对象发现通道。
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置任务查询依赖", false)
		return
	}
	pageSize, ok := taskListPageSize(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "TASK_PAGE_SIZE_INVALID", "任务列表每页条数无效", false)
		return
	}
	query := store.TaskListQuery{SubjectID: principal.ID, Limit: pageSize}
	if cursorValue := strings.TrimSpace(r.URL.Query().Get("cursor")); cursorValue != "" {
		cursor, ok := decodeTaskListCursor(cursorValue, principal.ID)
		if !ok {
			writeError(w, http.StatusBadRequest, "CURSOR_INVALID", "任务列表游标无效", false)
			return
		}
		query.BeforeSubmitted = cursor.BeforeSubmitted
		query.BeforeTaskID = cursor.BeforeTaskID
	}
	totalTasks, err := s.tasks.CountAuthorizedTaskSummaries(r.Context(), principal.ID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_QUERY_UNAVAILABLE", "任务列表暂时不可用", true)
		return
	}
	items, err := s.tasks.ListTaskSummaries(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_QUERY_UNAVAILABLE", "任务列表暂时不可用", true)
		return
	}
	nextCursor := ""
	if len(items) > pageSize {
		last := items[pageSize-1]
		nextCursor = encodeTaskListCursor(principal.ID, last.SubmittedAt, last.TaskID)
		items = items[:pageSize]
	}
	responses := make([]taskListResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, taskListResponse{
			ID: item.TaskID, Type: item.TaskType, DataSourceID: item.DataSourceID,
			ObjectSummary: taskObjectSummary(item.Database, item.Table), State: item.State,
			StageEvidence: "UNAVAILABLE", ProgressEvidence: "UNAVAILABLE",
			ReconciliationRequired: item.ReconciliationRequired, NodeID: item.NodeID,
			OwnedByCurrentUser: item.CreatorSubjectID == principal.ID,
			SubmittedAt:        item.SubmittedAt.Format(time.RFC3339Nano), StartedAt: optionalTaskTime(item.StartedAt),
			FinishedAt: optionalTaskTime(item.FinishedAt), UpdatedAt: item.UpdatedAt.Format(time.RFC3339Nano),
		})
	}
	var nextCursorResponse any
	if nextCursor != "" {
		nextCursorResponse = nextCursor
	}
	totalPages := totalTasks / pageSize
	if totalTasks%pageSize != 0 {
		totalPages++
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "items": responses, "nextCursor": nextCursorResponse, "totalPages": totalPages})
}

// taskListPageSize 只接受经过产品确认的有限页大小，避免读取接口退化为无界数据枚举。
func taskListPageSize(r *http.Request) (int, bool) {
	values, present := r.URL.Query()["limit"]
	if !present {
		return taskListDefaultPageSize, true
	}
	if len(values) != 1 {
		return 0, false
	}
	limit, err := strconv.Atoi(strings.TrimSpace(values[0]))
	if err != nil {
		return 0, false
	}
	switch limit {
	case 10, 20, 50:
		return limit, true
	default:
		return 0, false
	}
}

func decodeTaskListCursor(value, subjectID string) (store.TaskListQuery, bool) {
	if len(value) > 2048 {
		return store.TaskListQuery{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) > 1024 {
		return store.TaskListQuery{}, false
	}
	var cursor taskListCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return store.TaskListQuery{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return store.TaskListQuery{}, false
	}
	submittedAt, err := time.Parse(time.RFC3339Nano, cursor.SubmittedAt)
	if err != nil || subtle.ConstantTimeCompare([]byte(cursor.SubjectDigest), []byte(taskListSubjectDigest(subjectID))) != 1 ||
		strings.TrimSpace(cursor.TaskID) == "" || strings.ContainsAny(cursor.TaskID, "/\\") {
		return store.TaskListQuery{}, false
	}
	return store.TaskListQuery{BeforeSubmitted: submittedAt.UTC(), BeforeTaskID: cursor.TaskID}, true
}

func encodeTaskListCursor(subjectID string, submittedAt time.Time, taskID string) string {
	raw, err := json.Marshal(taskListCursor{SubjectDigest: taskListSubjectDigest(subjectID), SubmittedAt: submittedAt.UTC().Format(time.RFC3339Nano), TaskID: taskID})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func taskListSubjectDigest(subjectID string) string {
	digest := sha256.Sum256([]byte("task-list-cursor-v1|" + subjectID))
	return hex.EncodeToString(digest[:])
}

func taskObjectSummary(database, table string) string {
	if database == "" {
		return table
	}
	if table == "" {
		return database
	}
	return database + "." + table
}

func optionalTaskTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
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
	// 密码内容不进入摘要，避免持久化密码派生哈希形成新的敏感面；只记录是否提供密码。
	// sys 密码只以存在性参与摘要；sys 账号是非秘密连接标识，必须进入摘要以区分不同创建请求。
	sysPresence := "sys=false"
	if request.SysUser != "" || request.SysPassword != "" {
		sysPresence = "sys=true"
	}
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%s|%s|%s|%s|password=true|%s|sysUser=%s", request.DisplayName, request.Environment, request.ConnectionKind, request.CompatibilityMode, request.Host, request.Port, request.ClusterName, request.TenantName, request.Username, request.DefaultDatabase, sysPresence, strings.TrimSpace(request.SysUser))
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func newOpaqueID() string {
	value, err := identifier.NewUUIDV4()
	if err != nil {
		return ""
	}
	return value
}

// newOpaqueSecret 生成只用于一次性关联或机器身份的高熵 URL 安全字节序列。
// 调用方必须在摘要或响应完成后清零返回缓冲，不能把它加入审计、日志或持久化模型。
func newOpaqueSecret() []byte {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil
	}
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(randomBytes)))
	base64.RawURLEncoding.Encode(encoded, randomBytes)
	credential.Zero(randomBytes)
	return encoded
}

// getDataSource 先验证读取范围，再将不存在与超出读取范围统一映射为 404。
// 业务用户名只有在同一对象的管理范围已确认时才会进入详情投影。
func (s *Server) getDataSource(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID string) {
	if s.dataSource == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置数据源 API 依赖", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	// 读取范围不能推定为管理范围。拒绝或无法确认管理范围时失败关闭为不回显用户名。
	includeUsername := identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceWrite, dataSourceID) == nil
	summary, err := s.dataSource.GetDataSourceSummary(r.Context(), dataSourceID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATA_SOURCE_QUERY_UNAVAILABLE", "数据源暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": newDataSourceDetailResponse(summary, includeUsername)})
}

// agentEnrollmentExchangeRequest 仅承载一次性关联所需的固定字段。
// 原始关联材料和机器凭据只在处理期间短时存在，绝不写入日志、审计或响应。
type agentEnrollmentExchangeRequest struct {
	RequestID          string `json:"requestId"`
	EnrollmentID       string `json:"enrollmentId"`
	EnrollmentMaterial string `json:"enrollmentMaterial"`
	AgentID            string `json:"agentId"`
	NodeID             string `json:"nodeId"`
	MachineCredential  string `json:"machineCredential"`
	ProtocolVersion    string `json:"protocolVersion"`
}

// agentHeartbeatRequest 是严格的 Agent 心跳信封。
// payload 只接受已登记的环境与容量摘要，不能退化为任意机器事实或远程操作指令。
type agentHeartbeatRequest struct {
	ProtocolVersion string                `json:"protocolVersion"`
	AgentID         string                `json:"agentId"`
	NodeID          string                `json:"nodeId"`
	BootID          string                `json:"bootId"`
	RequestID       string                `json:"requestId"`
	SentAt          string                `json:"sentAt"`
	PayloadType     string                `json:"payloadType"`
	Payload         agentHeartbeatPayload `json:"payload"`
}

// agentHeartbeatPayload 是本轮允许持久化的最小环境事实。
// 工具、Java、目录、路径、数据库和对象结果都不在此信封中，仍需后续固定环境检查与预检查。
type agentHeartbeatPayload struct {
	OperatingSystem            string                      `json:"operatingSystem"`
	Architecture               string                      `json:"architecture"`
	AgentVersion               string                      `json:"agentVersion"`
	ObservedAt                 string                      `json:"observedAt"`
	CapacityTotal              int                         `json:"capacityTotal"`
	CapacityUsed               int                         `json:"capacityUsed"`
	CPUUsagePercent            *int                        `json:"cpuUsagePercent,omitempty"`
	MemoryUsagePercent         *int                        `json:"memoryUsagePercent,omitempty"`
	RuntimeConfigurationDigest string                      `json:"runtimeConfigurationDigest,omitempty"`
	DataRootUsages             []agentDataRootUsagePayload `json:"dataRootUsages,omitempty"`
}

type agentDataRootUsagePayload struct {
	RootDigest     string `json:"rootDigest"`
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
}

// agentExecutionNodeEnvironmentCheckCompletionRequest 只承载固定本机运行时检查的稳定结果。
// 它不接受路径、命令、工具输出、连接参数或任意可扩展检查字段。
type agentExecutionNodeEnvironmentCheckCompletionRequest struct {
	agentPrecheckEnvelope
	Payload agentExecutionNodeEnvironmentCheckCompletionPayload `json:"payload"`
}

type agentExecutionNodeEnvironmentCheckCompletionPayload struct {
	FactsRevision int64  `json:"factsRevision"`
	Status        string `json:"status"`
	Code          string `json:"code"`
}

// exchangeAgentEnrollment 原子消费一次性关联材料，并只返回不含机器凭据的绑定结果。
func (s *Server) exchangeAgentEnrollment(w http.ResponseWriter, r *http.Request) {
	if s.agentProtocol == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_ENROLLMENT_NOT_CONFIGURED", "当前环境尚未配置 Agent 关联", false)
		return
	}
	var request agentEnrollmentExchangeRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if request.ProtocolVersion != agentwire.Version {
		writeError(w, http.StatusBadRequest, "AGENT_PROTOCOL_VERSION_UNSUPPORTED", "Agent 协议版本不受支持", false)
		return
	}
	material := []byte(request.EnrollmentMaterial)
	machineCredential := []byte(request.MachineCredential)
	request.EnrollmentMaterial = ""
	request.MachineCredential = ""
	defer credential.Zero(material)
	defer credential.Zero(machineCredential)
	if len(material) < 32 || len(machineCredential) < 32 {
		writeError(w, http.StatusBadRequest, "AGENT_ENROLLMENT_INVALID", "Agent 关联请求无效", false)
		return
	}
	materialDigest := sha256.Sum256(material)
	credentialDigest := sha256.Sum256(machineCredential)
	result, err := s.agentProtocol.ExchangeAgentEnrollment(r.Context(), store.AgentEnrollmentExchange{
		EnrollmentID: request.EnrollmentID, NodeID: request.NodeID, AgentID: request.AgentID, RequestID: request.RequestID,
		ProtocolVersion: request.ProtocolVersion, EnrollmentMaterialDigest: materialDigest[:], CredentialDigest: credentialDigest[:], ExchangedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrEnrollmentRejected) || errors.Is(err, store.ErrAgentAlreadyAssociated) {
		writeError(w, http.StatusUnauthorized, "AGENT_ENROLLMENT_REJECTED", "Agent 关联请求已被拒绝", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_ENROLLMENT_UNAVAILABLE", "Agent 关联暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": "ENROLLED",
		"payload": map[string]any{"agentId": result.AgentID, "nodeId": result.NodeID, "protocolVersion": agentwire.Version, "replayed": result.Replayed, "realExecutionEnabled": false,
			"runtimeConfiguration": map[string]any{"platform": result.RuntimeConfiguration.Platform, "toolHome": result.RuntimeConfiguration.ToolHome, "javaPath": result.RuntimeConfiguration.JavaPath, "allowedRoots": result.RuntimeConfiguration.AllowedRoots, "revision": result.RuntimeConfiguration.Revision, "digest": result.RuntimeConfiguration.Digest}},
	})
}

// recordAgentHeartbeat 先验证 Bearer 机器凭据，再交叉校验信封中的 Agent 与节点绑定。
// 它只形成当前在线和只读机器事实，不会把环境状态写成正常、启用节点或开放任务领取。
func (s *Server) recordAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if s.agentProtocol == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_AUTHENTICATION_NOT_CONFIGURED", "当前环境尚未配置 Agent 机器认证", false)
		return
	}
	credentialDigest, ok := agentCredentialDigest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return
	}
	identity, err := s.agentProtocol.AuthenticateAgent(r.Context(), credentialDigest)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return
	}
	var request agentHeartbeatRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if request.ProtocolVersion != agentwire.Version || request.PayloadType != "HEARTBEAT" || request.AgentID != identity.AgentID || request.NodeID != identity.NodeID || request.ProtocolVersion != identity.ProtocolVersion {
		writeError(w, http.StatusUnauthorized, "AGENT_HEARTBEAT_REJECTED", "Agent 心跳请求已被拒绝", false)
		return
	}
	if _, err := time.Parse(time.RFC3339Nano, request.SentAt); err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	observedAt, err := time.Parse(time.RFC3339Nano, request.Payload.ObservedAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	now := time.Now().UTC()
	factsRevision, err := s.agentProtocol.RecordAgentHeartbeat(r.Context(), store.AgentHeartbeat{
		AgentID: request.AgentID, NodeID: request.NodeID, ProtocolVersion: request.ProtocolVersion, BootID: request.BootID,
		RequestID: request.RequestID, ObservedAt: observedAt.UTC(), ReceivedAt: now, CapacityTotal: request.Payload.CapacityTotal,
		CapacityUsed: request.Payload.CapacityUsed, Facts: store.AgentEnvironmentFacts{
			OperatingSystem: request.Payload.OperatingSystem, Architecture: request.Payload.Architecture, AgentVersion: request.Payload.AgentVersion,
			ObservedAt: observedAt.UTC(), CPUUsagePercent: request.Payload.CPUUsagePercent, MemoryUsagePercent: request.Payload.MemoryUsagePercent,
			RuntimeConfigurationDigest: request.Payload.RuntimeConfigurationDigest, DataRootUsages: agentDataRootUsages(request.Payload.DataRootUsages),
		},
	})
	if errors.Is(err, store.ErrAgentHeartbeatRejected) {
		writeError(w, http.StatusUnauthorized, "AGENT_HEARTBEAT_REJECTED", "Agent 心跳请求已被拒绝", false)
		return
	}
	if errors.Is(err, store.ErrAgentHeartbeatConflict) {
		writeError(w, http.StatusConflict, "AGENT_HEARTBEAT_CONFLICT", "Agent 心跳请求与此前请求冲突", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	environmentCheckID := ""
	if s.agentEnvironmentChecks != nil {
		if refresher, ok := s.agentEnvironmentChecks.(AgentExecutionNodeEnvironmentCheckRefresher); ok {
			checkID := newOpaqueID()
			if checkID == "" {
				writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
				return
			}
			if _, refreshErr := refresher.EnsureCurrentExecutionNodeEnvironmentCheck(r.Context(), store.ExecutionNodeEnvironmentCheckRefresh{
				NodeID: identity.NodeID, AgentID: identity.AgentID, CheckID: checkID, RequestID: request.RequestID,
				FactsRevision: factsRevision, RequestedAt: now,
			}); refreshErr != nil {
				writeError(w, http.StatusServiceUnavailable, "AGENT_ENVIRONMENT_CHECK_UNAVAILABLE", "节点环境检查暂时不可用", true)
				return
			}
		}
		pending, found, pendingErr := s.agentEnvironmentChecks.GetPendingExecutionNodeEnvironmentCheck(r.Context(), identity.AgentID, identity.NodeID)
		if pendingErr != nil {
			writeError(w, http.StatusServiceUnavailable, "AGENT_ENVIRONMENT_CHECK_UNAVAILABLE", "节点环境检查暂时不可用", true)
			return
		}
		if found {
			environmentCheckID = pending.CheckID
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "serverTime": now.Format(time.RFC3339Nano), "status": "ACCEPTED",
		"payload": map[string]any{"factsRevision": factsRevision, "environmentStatus": "NOT_CHECKED", "environmentCheckId": environmentCheckID, "realExecutionEnabled": false},
	})
}

func agentDataRootUsages(values []agentDataRootUsagePayload) []store.AgentDataRootUsage {
	if len(values) == 0 {
		return nil
	}
	result := make([]store.AgentDataRootUsage, 0, len(values))
	for _, value := range values {
		result = append(result, store.AgentDataRootUsage{RootDigest: value.RootDigest, TotalBytes: value.TotalBytes, AvailableBytes: value.AvailableBytes})
	}
	return result
}

// agentCredentialDigest 从 Bearer 头短时计算机器凭据摘要，避免把原始材料传入业务或日志层。
func agentCredentialDigest(r *http.Request) ([]byte, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Bearer" || len(parts[1]) < 32 {
		return nil, false
	}
	value := []byte(parts[1])
	defer credential.Zero(value)
	digest := sha256.Sum256(value)
	return append([]byte(nil), digest[:]...), true
}

// agentPrecheckEnvelope 是受认证固定预检查共用的严格机器信封。
// 载荷由每个动作的专用类型承载，不能借此接收自由 JSON 或任意 Agent 指令。
type agentPrecheckEnvelope struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
}

// agentPrecheckClaimNextRequest 只允许受认证 Agent 原子领取自身的下一条固定预检查。
// 载荷没有预检查或租约标识、路径、命令、SQL 或秘密，防止本机输入扩展为任意远程操作。
type agentPrecheckClaimNextRequest struct {
	agentPrecheckEnvelope
	Payload agentPrecheckClaimNextPayload `json:"payload"`
}

type agentPrecheckClaimNextPayload struct {
	Capability string `json:"capability"`
}

// agentPrecheckAcknowledgementRequest 只确认当前租约和服务端摘要。
type agentPrecheckAcknowledgementRequest struct {
	agentPrecheckEnvelope
	Payload agentPrecheckAcknowledgementPayload `json:"payload"`
}

type agentPrecheckAcknowledgementPayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
}

// agentPrecheckSecretResolveRequest 只允许 Agent 在已确认租约中请求唯一的数据库连接槽位。
// 请求体不含凭据引用、用户名、密码、路径、SQL 或命令，所有实际绑定均由控制面重读。
type agentPrecheckSecretResolveRequest struct {
	agentPrecheckEnvelope
	Payload agentPrecheckSecretResolvePayload `json:"payload"`
}

type agentPrecheckSecretResolvePayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
	Slot          string `json:"slot"`
}

// agentPrecheckCompletionRequest 只接收固定六项检查结果。
// 整体成功状态由服务端从每项结果派生，Agent 不能在请求体中声明成功。
type agentPrecheckCompletionRequest struct {
	agentPrecheckEnvelope
	Payload agentPrecheckCompletionPayload `json:"payload"`
}

type agentPrecheckCompletionPayload struct {
	LeaseID       string                       `json:"leaseId"`
	LeaseEpoch    int64                        `json:"leaseEpoch"`
	BindingDigest string                       `json:"bindingDigest"`
	Results       []agentPrecheckResultPayload `json:"results"`
}

type agentPrecheckResultPayload struct {
	Check        agentpreflight.CheckID `json:"check"`
	Status       agentpreflight.Status  `json:"status"`
	EvidenceCode string                 `json:"evidenceCode"`
}

// agentConnectionTestEnvelope 是基础连接测试独立使用的严格机器信封。
// 它不复用 EXPORT_PREFLIGHT 载荷类型，避免对象、路径或工具检查字段进入基础连接测试。
type agentConnectionTestEnvelope struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
}

// agentConnectionTestClaimNextRequest 不允许 Agent 提供测试标识、租约或连接输入。
// 控制面只会领取已经冻结到当前 Agent 和节点的下一条基础连接测试。
type agentConnectionTestClaimNextRequest struct {
	agentConnectionTestEnvelope
	Payload agentConnectionTestClaimNextPayload `json:"payload"`
}

type agentConnectionTestClaimNextPayload struct {
	Capability string `json:"capability"`
}

// agentConnectionTestAcknowledgementRequest 只确认当前租约和绑定摘要。
type agentConnectionTestAcknowledgementRequest struct {
	agentConnectionTestEnvelope
	Payload agentConnectionTestLeasePayload `json:"payload"`
}

// agentConnectionTestSecretResolveRequest 只能请求基础连接测试唯一的 DATABASE_CONNECTION 槽位。
// 请求体不能包含凭据引用、URL、SQL、路径、命令或秘密原文。
type agentConnectionTestSecretResolveRequest struct {
	agentConnectionTestEnvelope
	Payload agentConnectionTestSecretPayload `json:"payload"`
}

type agentConnectionTestLeasePayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
}

type agentConnectionTestSecretPayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
	Slot          string `json:"slot"`
}

// agentConnectionTestCompletionRequest 只接收固定的基础连接测试结论。
// Agent 不能提交 JDBC 原始异常、连接信息、任意成功布尔值或其他自由文本。
type agentConnectionTestCompletionRequest struct {
	agentConnectionTestEnvelope
	Payload agentConnectionTestCompletionPayload `json:"payload"`
}

type agentConnectionTestCompletionPayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
	Status        string `json:"status"`
	EvidenceCode  string `json:"evidenceCode"`
	// SysVerificationStatus/SysEvidenceCode 是可选的 sys 凭据验证结果（与数据库结果相互独立）。
	SysVerificationStatus string `json:"sysVerificationStatus"`
	SysEvidenceCode       string `json:"sysEvidenceCode"`
}

// authenticatedPrecheckAgent 使用独立机器凭据认证预检查请求。
// 浏览器身份、关联材料和任意请求头都不能代替已登记机器身份。
func (s *Server) authenticatedPrecheckAgent(w http.ResponseWriter, r *http.Request) (store.AgentIdentity, bool) {
	if s.agentProtocol == nil || s.agentPrechecks == nil || s.precheckTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PRECHECK_NOT_CONFIGURED", "当前环境尚未配置受认证 Agent 预检查", false)
		return store.AgentIdentity{}, false
	}
	credentialDigest, ok := agentCredentialDigest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, false
	}
	machine, err := s.agentProtocol.AuthenticateAgent(r.Context(), credentialDigest)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, false
	}
	return machine, true
}

func validPrecheckEnvelope(machine store.AgentIdentity, envelope agentPrecheckEnvelope, payloadType string) bool {
	if envelope.ProtocolVersion != agentwire.Version || envelope.ProtocolVersion != machine.ProtocolVersion || envelope.PayloadType != payloadType ||
		envelope.AgentID != machine.AgentID || envelope.NodeID != machine.NodeID ||
		!validAgentPrecheckOpaque(envelope.BootID) || !validAgentPrecheckOpaque(envelope.RequestID) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, envelope.SentAt)
	return err == nil
}

func validAgentPrecheckOpaque(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 256 && !strings.ContainsRune(value, 0)
}

func validAgentPrecheckPathID(value string) bool {
	return validAgentPrecheckOpaque(value) && !strings.ContainsAny(value, "/\\?#:")
}

func validAgentPrecheckDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	return value == strings.ToLower(value)
}

// authenticatedExecutionNodeEnvironmentCheckAgent 使用机器凭据认证固定环境检查回执。
// 它不复用预检查依赖，避免“节点启用前检查”被错误绑定到某个导出草稿或秘密槽位。
func (s *Server) authenticatedExecutionNodeEnvironmentCheckAgent(w http.ResponseWriter, r *http.Request) (store.AgentIdentity, bool) {
	if s.agentProtocol == nil || s.agentEnvironmentChecks == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_ENVIRONMENT_CHECK_NOT_CONFIGURED", "当前环境尚未配置受认证 Agent 环境检查", false)
		return store.AgentIdentity{}, false
	}
	credentialDigest, ok := agentCredentialDigest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, false
	}
	machine, err := s.agentProtocol.AuthenticateAgent(r.Context(), credentialDigest)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, false
	}
	return machine, true
}

// completeAuthenticatedExecutionNodeEnvironmentCheck 持久化当前 Agent 对唯一固定运行时检查的结果。
// 检查项和证据码均为协议常量，任何路径、命令、异常文本或自由结果字段都会被拒绝。
func (s *Server) completeAuthenticatedExecutionNodeEnvironmentCheck(w http.ResponseWriter, r *http.Request, checkID string) {
	machine, ok := s.authenticatedExecutionNodeEnvironmentCheckAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionNodeEnvironmentCheckCompletionRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validPrecheckEnvelope(machine, request.agentPrecheckEnvelope, "EXECUTION_NODE_ENVIRONMENT_CHECK_COMPLETE") ||
		!validAgentPrecheckPathID(checkID) || request.Payload.FactsRevision < 1 ||
		!validExecutionNodeEnvironmentCheckResult(request.Payload.Status, request.Payload.Code) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	err := s.agentEnvironmentChecks.CompleteExecutionNodeEnvironmentCheck(r.Context(), store.AgentExecutionNodeEnvironmentCheckCompletion{
		NodeID: machine.NodeID, AgentID: machine.AgentID, CheckID: checkID, FactsRevision: request.Payload.FactsRevision,
		Status: request.Payload.Status, Code: request.Payload.Code, RequestID: request.RequestID, CompletedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrExecutionNodeEnvironmentCheckRequired) {
		writeError(w, http.StatusConflict, "ENVIRONMENT_CHECK_REJECTED", "节点环境检查已失效，请等待下一次心跳", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_ENVIRONMENT_CHECK_UNAVAILABLE", "节点环境检查暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": "ENVIRONMENT_CHECK_COMPLETED",
		"payload": map[string]any{"realExecutionEnabled": false},
	})
}

func validExecutionNodeEnvironmentCheckResult(status, code string) bool {
	return (status == "PASSED" && code == "TOOL_RUNTIME_READY") ||
		(status == "FAILED" && (code == "TOOL_RUNTIME_INVALID" || code == "TOOL_RUNTIME_UNAVAILABLE"))
}

// agentPrecheckRequestDigest 对已验证的机器信封和固定载荷生成稳定摘要。
// 摘要让同一个 requestId 只能安全重放完全相同的操作，不能跨操作或改变绑定复用。
func agentPrecheckRequestDigest(operation string, envelope agentPrecheckEnvelope, precheckID string, payload any) string {
	raw, err := json.Marshal(struct {
		Operation       string `json:"operation"`
		ProtocolVersion string `json:"protocolVersion"`
		AgentID         string `json:"agentId"`
		NodeID          string `json:"nodeId"`
		BootID          string `json:"bootId"`
		RequestID       string `json:"requestId"`
		SentAt          string `json:"sentAt"`
		PayloadType     string `json:"payloadType"`
		PrecheckID      string `json:"precheckId"`
		Payload         any    `json:"payload"`
	}{
		Operation: operation, ProtocolVersion: envelope.ProtocolVersion, AgentID: envelope.AgentID,
		NodeID: envelope.NodeID, BootID: envelope.BootID, RequestID: envelope.RequestID,
		SentAt: envelope.SentAt, PayloadType: envelope.PayloadType, PrecheckID: precheckID, Payload: payload,
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// writeAgentPrecheckResponse 保持 Agent 客户端可验证的严格响应信封。
// 载荷只包含当前动作的最小安全投影，避免将内部租约、错误或秘密扩散到 Agent 状态文件。
func writeAgentPrecheckResponse(w http.ResponseWriter, status string, payload any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": status, "payload": payload,
	})
}

func writeAgentPrecheckStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "PRECHECK_REQUEST_CONFLICT", "预检查请求与此前请求冲突", false)
	case errors.Is(err, store.ErrPrecheckLeaseExpired):
		writeError(w, http.StatusConflict, "PRECHECK_LEASE_EXPIRED", "预检查租约已过期", false)
	case errors.Is(err, store.ErrPrecheckLeaseRejected), errors.Is(err, store.ErrPrecheckInvalid):
		writeError(w, http.StatusConflict, "PRECHECK_LEASE_REJECTED", "预检查租约无效", false)
	default:
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_PROTOCOL_UNAVAILABLE", "预检查协议暂时不可用", true)
	}
}

// authenticatedConnectionTestAgent 使用独立租约依赖校验基础连接测试的机器身份。
// 浏览器身份、关联材料和任意请求头都不能替代已经登记的 Agent 机器凭据。
func (s *Server) authenticatedConnectionTestAgent(w http.ResponseWriter, r *http.Request) (store.AgentIdentity, bool) {
	if s.agentProtocol == nil || s.agentConnectionTests == nil || s.connectionTestTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "AGENT_CONNECTION_TEST_NOT_CONFIGURED", "当前环境尚未配置受认证 Agent 连接测试", false)
		return store.AgentIdentity{}, false
	}
	credentialDigest, ok := agentCredentialDigest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, false
	}
	machine, err := s.agentProtocol.AuthenticateAgent(r.Context(), credentialDigest)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, false
	}
	return machine, true
}

// validConnectionTestEnvelope 将基础连接测试限定为当前已认证 Agent 的固定信封。
// sentAt 仅用于协议可审计性，租约和过期判定一律使用控制面时钟。
func validConnectionTestEnvelope(machine store.AgentIdentity, envelope agentConnectionTestEnvelope, payloadType string) bool {
	if envelope.ProtocolVersion != agentwire.Version || envelope.ProtocolVersion != machine.ProtocolVersion || envelope.PayloadType != payloadType ||
		envelope.AgentID != machine.AgentID || envelope.NodeID != machine.NodeID ||
		!validAgentPrecheckOpaque(envelope.BootID) || !validAgentPrecheckOpaque(envelope.RequestID) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, envelope.SentAt)
	return err == nil
}

// agentConnectionTestRequestDigest 让相同 requestId 只能重放完全相同的基础连接测试动作。
// 操作、测试标识和载荷均进入摘要，防止跨动作、跨租约或跨绑定复用。
func agentConnectionTestRequestDigest(operation string, envelope agentConnectionTestEnvelope, connectionTestID string, payload any) string {
	raw, err := json.Marshal(struct {
		Operation        string `json:"operation"`
		ProtocolVersion  string `json:"protocolVersion"`
		AgentID          string `json:"agentId"`
		NodeID           string `json:"nodeId"`
		BootID           string `json:"bootId"`
		RequestID        string `json:"requestId"`
		SentAt           string `json:"sentAt"`
		PayloadType      string `json:"payloadType"`
		ConnectionTestID string `json:"connectionTestId"`
		Payload          any    `json:"payload"`
	}{
		Operation: operation, ProtocolVersion: envelope.ProtocolVersion, AgentID: envelope.AgentID, NodeID: envelope.NodeID,
		BootID: envelope.BootID, RequestID: envelope.RequestID, SentAt: envelope.SentAt, PayloadType: envelope.PayloadType,
		ConnectionTestID: connectionTestID, Payload: payload,
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// writeAgentConnectionTestResponse 固定 Agent 响应信封，避免 Agent 客户端接受无结构或带秘密的响应。
func writeAgentConnectionTestResponse(w http.ResponseWriter, status string, payload any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": status, "payload": payload,
	})
}

func writeAgentConnectionTestStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "CONNECTION_TEST_REQUEST_CONFLICT", "连接测试请求与此前请求冲突", false)
	case errors.Is(err, store.ErrDataSourceConnectionTestLeaseExpired):
		writeError(w, http.StatusConflict, "CONNECTION_TEST_LEASE_EXPIRED", "连接测试租约已过期", false)
	case errors.Is(err, store.ErrDataSourceConnectionTestLeaseRejected), errors.Is(err, store.ErrDataSourceConnectionTestInvalid):
		writeError(w, http.StatusConflict, "CONNECTION_TEST_LEASE_REJECTED", "连接测试租约无效", false)
	default:
		writeError(w, http.StatusServiceUnavailable, "CONNECTION_TEST_PROTOCOL_UNAVAILABLE", "连接测试协议暂时不可用", true)
	}
}

// claimNextAuthenticatedPrecheck 在单个服务端事务内选择并领取当前机器的下一条固定预检查。
// 请求不能指定预检查或租约；领取只写入 SQLite 租约与回执，不请求秘密、不连接数据库且不启动任何工具。
func (s *Server) claimNextAuthenticatedPrecheck(w http.ResponseWriter, r *http.Request) {
	machine, ok := s.authenticatedPrecheckAgent(w, r)
	if !ok {
		return
	}
	var request agentPrecheckClaimNextRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validPrecheckEnvelope(machine, request.agentPrecheckEnvelope, "EXPORT_PREFLIGHT_CLAIM_NEXT") || request.Payload.Capability != string(agentpreflight.CapabilityExportPreflight) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	leaseID, err := identifier.NewUUIDV4()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_PROTOCOL_UNAVAILABLE", "预检查协议暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	grant, found, err := s.agentPrechecks.ClaimNextPrecheck(r.Context(), store.PrecheckClaimNext{
		AgentID: machine.AgentID, NodeID: machine.NodeID, LeaseID: leaseID, RequestID: request.RequestID,
		RequestDigest: agentPrecheckRequestDigest("CLAIM_NEXT", request.agentPrecheckEnvelope, "", request.Payload), LeaseTTL: s.precheckTTL, Now: now,
	})
	if err != nil {
		writeAgentPrecheckStoreError(w, err)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeAgentPrecheckResponse(w, "PRECHECK_CLAIMED", map[string]any{
		"precheckId": grant.PrecheckID, "leaseId": grant.LeaseID, "leaseEpoch": grant.LeaseEpoch,
		"expiresAt": grant.ExpiresAt.Format(time.RFC3339Nano),
		"binding": map[string]any{
			"precheckId": grant.Binding.PrecheckID, "nodeId": grant.Binding.NodeID, "draftRevision": grant.Binding.DraftRevision,
			"configFingerprint": grant.Binding.ConfigFingerprint, "credentialRevision": grant.Binding.CredentialRevision,
			"nodeFactsVersion": grant.Binding.NodeFactsRevision,
		},
		"bindingDigest": grant.Binding.BindingDigest, "checkSet": agentpreflight.FixedChecks(), "realExecutionEnabled": false,
		"executionContext": map[string]any{
			"compatibilityMode": grant.ExecutionContext.CompatibilityMode,
			"database":          grant.ExecutionContext.Database, "objects": grant.ExecutionContext.Objects,
			"contentKind": grant.ExecutionContext.ContentKind,
			"outputPath":  grant.ExecutionContext.OutputPath, "targetPlatform": grant.ExecutionContext.TargetPlatform,
			"logPath": grant.ExecutionContext.LogPath, "skipCheckDir": grant.ExecutionContext.SkipCheckDir,
			"allowedRoots": grant.ExecutionContext.AllowedRoots,
		},
	})
}

// acknowledgeAuthenticatedPrecheck 持久化 Agent 对当前租约和固定检查集的确认。
// 不确认的租约不能提交检查结果，避免响应丢失后将未知绑定误当成可完成。
func (s *Server) acknowledgeAuthenticatedPrecheck(w http.ResponseWriter, r *http.Request, precheckID string) {
	machine, ok := s.authenticatedPrecheckAgent(w, r)
	if !ok {
		return
	}
	var request agentPrecheckAcknowledgementRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validPrecheckEnvelope(machine, request.agentPrecheckEnvelope, "EXPORT_PREFLIGHT_ACKNOWLEDGE_LEASE") || !validAgentPrecheckPathID(precheckID) ||
		!validAgentPrecheckOpaque(request.Payload.LeaseID) || request.Payload.LeaseEpoch < 1 || !validAgentPrecheckDigest(request.Payload.BindingDigest) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	_, err := s.agentPrechecks.AcknowledgePrecheck(r.Context(), store.PrecheckAcknowledgement{
		AgentID: machine.AgentID, PrecheckID: precheckID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
		BindingDigest: request.Payload.BindingDigest, RequestID: request.RequestID,
		RequestDigest: agentPrecheckRequestDigest("ACKNOWLEDGE", request.agentPrecheckEnvelope, precheckID, request.Payload), Now: time.Now().UTC(),
	})
	if err != nil {
		writeAgentPrecheckStoreError(w, err)
		return
	}
	writeAgentPrecheckResponse(w, "PRECHECK_LEASE_ACKNOWLEDGED", map[string]any{"realExecutionEnabled": false})
}

// resolveAuthenticatedPrecheckSecret 在已确认短租约中返回唯一的数据库连接槽位。
// 槽位仅以短时字节在 HTTPS 响应中存在；本函数不缓存明文，也不会把它写入审计、错误或 SQLite。
func (s *Server) resolveAuthenticatedPrecheckSecret(w http.ResponseWriter, r *http.Request, precheckID string) {
	machine, ok := s.authenticatedPrecheckAgent(w, r)
	if !ok {
		return
	}
	if s.precheckSecrets == nil || s.decryptor == nil {
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_SECRET_RESOLUTION_NOT_CONFIGURED", "当前环境尚未配置预检查秘密槽位", false)
		return
	}
	var request agentPrecheckSecretResolveRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validPrecheckEnvelope(machine, request.agentPrecheckEnvelope, "EXPORT_PREFLIGHT_RESOLVE_SECRET_SLOTS") ||
		!validAgentPrecheckPathID(precheckID) || !validAgentPrecheckOpaque(request.Payload.LeaseID) ||
		request.Payload.LeaseEpoch < 1 || !validAgentPrecheckDigest(request.Payload.BindingDigest) ||
		request.Payload.Slot != "DATABASE_CONNECTION" {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	now := time.Now().UTC()
	requestDigest := agentPrecheckRequestDigest("RESOLVE_SECRET", request.agentPrecheckEnvelope, precheckID, request.Payload)
	encrypted, err := s.precheckSecrets.ResolvePrecheckDatabaseConnection(r.Context(), store.PrecheckSecretResolutionRequest{
		AgentID: machine.AgentID, PrecheckID: precheckID, LeaseID: request.Payload.LeaseID,
		LeaseEpoch: request.Payload.LeaseEpoch, BindingDigest: request.Payload.BindingDigest,
		RequestID: request.RequestID, RequestDigest: requestDigest, Now: now,
	})
	if err != nil {
		writeAgentPrecheckStoreError(w, err)
		return
	}
	defer encrypted.Destroy()
	finish := func(succeeded bool) error {
		return s.precheckSecrets.FinishPrecheckSecretResolution(r.Context(), store.PrecheckSecretResolutionOutcome{
			AgentID: machine.AgentID, PrecheckID: precheckID, LeaseID: request.Payload.LeaseID,
			LeaseEpoch: request.Payload.LeaseEpoch, BindingDigest: request.Payload.BindingDigest,
			RequestID: request.RequestID, RequestDigest: requestDigest, Succeeded: succeeded, Now: time.Now().UTC(),
		})
	}
	owner := identity.Principal{Type: identity.BrowserPrincipal, ID: encrypted.OwnerSubjectID}
	dataSourceAuthorizationErr := identity.Can(r.Context(), s.authorizer, owner, identity.ScopeDataSourceRead, encrypted.DataSourceID)
	nodeAuthorizationErr := identity.Can(r.Context(), s.authorizer, owner, identity.ScopeNodeUse, encrypted.NodeID)
	if identity.Validate(owner, identity.BrowserPrincipal) != nil || dataSourceAuthorizationErr != nil || nodeAuthorizationErr != nil {
		if finishErr := finish(false); finishErr != nil {
			writeAgentPrecheckStoreError(w, finishErr)
			return
		}
		writeAgentPrecheckStoreError(w, store.ErrPrecheckLeaseRejected)
		return
	}
	plaintext, decryptErr := s.decryptor.Decrypt(credential.Envelope{
		FormatVersion: credential.FormatVersion,
		KeyID:         encrypted.KeyID,
		Reference: credential.Reference{
			CredentialID: encrypted.CredentialID,
			Revision:     encrypted.Revision,
			SecretType:   credential.DatabasePassword,
			DataSourceID: encrypted.DataSourceID,
		},
		Nonce:      encrypted.Nonce,
		Ciphertext: encrypted.Ciphertext,
	})
	if decryptErr != nil {
		finishErr := finish(false)
		if finishErr != nil {
			writeAgentPrecheckStoreError(w, finishErr)
			return
		}
		writeError(w, http.StatusServiceUnavailable, "PRECHECK_SECRET_RESOLUTION_UNAVAILABLE", "预检查秘密槽位暂时不可用", true)
		return
	}
	defer credential.Zero(plaintext)
	if err := finish(true); err != nil {
		writeAgentPrecheckStoreError(w, err)
		return
	}
	writeAgentPrecheckResponse(w, "PRECHECK_SECRET_SLOTS_RESOLVED", map[string]any{
		"agentRequestId": request.RequestID,
		"precheckId":     precheckID,
		"leaseId":        request.Payload.LeaseID,
		"leaseEpoch":     request.Payload.LeaseEpoch,
		"bindingDigest":  request.Payload.BindingDigest,
		"slot":           "DATABASE_CONNECTION",
		"connection": map[string]any{
			"host": encrypted.Host, "port": encrypted.Port, "username": encrypted.Username, "password": plaintext,
		},
		"realExecutionEnabled": false,
	})
}

// completeAuthenticatedPrecheck 持久化固定六项检查的最终安全投影。
// 控制面根据每项 PASSED/FAILED/UNKNOWN 推导结果；此路径不接收整体成功布尔值或自由证据文本。
func (s *Server) completeAuthenticatedPrecheck(w http.ResponseWriter, r *http.Request, precheckID string) {
	machine, ok := s.authenticatedPrecheckAgent(w, r)
	if !ok {
		return
	}
	var request agentPrecheckCompletionRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validPrecheckEnvelope(machine, request.agentPrecheckEnvelope, "EXPORT_PREFLIGHT_COMPLETE") || !validAgentPrecheckPathID(precheckID) ||
		!validAgentPrecheckOpaque(request.Payload.LeaseID) || request.Payload.LeaseEpoch < 1 || !validAgentPrecheckDigest(request.Payload.BindingDigest) ||
		len(request.Payload.Results) != len(agentpreflight.FixedChecks()) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	results := make([]store.PrecheckCheckResult, 0, len(request.Payload.Results))
	report := agentpreflight.Report{PrecheckID: precheckID, Succeeded: true, Results: make([]agentpreflight.Result, 0, len(request.Payload.Results))}
	for _, item := range request.Payload.Results {
		results = append(results, store.PrecheckCheckResult{Check: string(item.Check), Status: string(item.Status), EvidenceCode: item.EvidenceCode})
		report.Results = append(report.Results, agentpreflight.Result{Check: item.Check, Status: item.Status, EvidenceCode: item.EvidenceCode})
		if item.Status != agentpreflight.StatusPassed {
			report.Succeeded = false
		}
	}
	if err := agentpreflight.ValidateReport(report); err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	result, err := s.agentPrechecks.CompleteAgentPrecheck(r.Context(), store.AgentPrecheckCompletion{
		AgentID: machine.AgentID, PrecheckID: precheckID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
		BindingDigest: request.Payload.BindingDigest, RequestID: request.RequestID,
		RequestDigest: agentPrecheckRequestDigest("COMPLETE", request.agentPrecheckEnvelope, precheckID, request.Payload), Results: results, Now: time.Now().UTC(),
	})
	if err != nil {
		writeAgentPrecheckStoreError(w, err)
		return
	}
	writeAgentPrecheckResponse(w, "PRECHECK_COMPLETED", map[string]any{"status": result.Status, "realExecutionEnabled": false})
}

// claimNextAuthenticatedConnectionTest 在一个短事务内领取当前 Agent 的下一条基础连接测试。
// 请求不能指定测试、租约、地址或凭据；领取本身不解析秘密、不连接数据库且不启动 Java。
func (s *Server) claimNextAuthenticatedConnectionTest(w http.ResponseWriter, r *http.Request) {
	machine, ok := s.authenticatedConnectionTestAgent(w, r)
	if !ok {
		return
	}
	var request agentConnectionTestClaimNextRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validConnectionTestEnvelope(machine, request.agentConnectionTestEnvelope, "DATA_SOURCE_CONNECTION_TEST_CLAIM_NEXT") || request.Payload.Capability != "DATA_SOURCE_CONNECTION_TEST" {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	leaseID, err := identifier.NewUUIDV4()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "CONNECTION_TEST_PROTOCOL_UNAVAILABLE", "连接测试协议暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	grant, found, err := s.agentConnectionTests.ClaimNextDataSourceConnectionTest(r.Context(), store.DataSourceConnectionTestClaimNext{
		AgentID: machine.AgentID, NodeID: machine.NodeID, LeaseID: leaseID, RequestID: request.RequestID,
		RequestDigest: agentConnectionTestRequestDigest("CLAIM_NEXT", request.agentConnectionTestEnvelope, "", request.Payload),
		LeaseTTL:      s.connectionTestTTL, Now: now,
	})
	if err != nil {
		writeAgentConnectionTestStoreError(w, err)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeAgentConnectionTestResponse(w, "DATA_SOURCE_CONNECTION_TEST_CLAIMED", map[string]any{
		"connectionTestId": grant.ConnectionTestID, "leaseId": grant.LeaseID, "leaseEpoch": grant.LeaseEpoch,
		"expiresAt": grant.ExpiresAt.UTC().Format(time.RFC3339Nano), "binding": map[string]any{
			"connectionTestId": grant.Binding.ConnectionTestID, "dataSourceId": grant.Binding.DataSourceID,
			"connectionConfigDigest": grant.Binding.ConnectionConfigDigest, "credentialRevision": grant.Binding.CredentialRevision,
			"nodeId": grant.Binding.NodeID, "nodeFactsRevision": grant.Binding.NodeFactsRevision,
			"sysCredentialId": grant.Binding.SysCredentialID, "sysCredentialRevision": grant.Binding.SysCredentialRevision,
		},
		"bindingDigest": grant.Binding.BindingDigest, "verificationSource": grant.Binding.VerificationSource,
		"realExecutionEnabled": false,
	})
}

// acknowledgeAuthenticatedConnectionTest 记录 Agent 对当前测试租约和冻结摘要的确认。
// 未确认的租约不能解析秘密或提交结果，避免响应丢失后继续使用未知绑定。
func (s *Server) acknowledgeAuthenticatedConnectionTest(w http.ResponseWriter, r *http.Request, connectionTestID string) {
	machine, ok := s.authenticatedConnectionTestAgent(w, r)
	if !ok {
		return
	}
	var request agentConnectionTestAcknowledgementRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validConnectionTestEnvelope(machine, request.agentConnectionTestEnvelope, "DATA_SOURCE_CONNECTION_TEST_ACKNOWLEDGE_LEASE") ||
		!validDataSourceConnectionTestPathID(connectionTestID) || !validAgentPrecheckOpaque(request.Payload.LeaseID) ||
		request.Payload.LeaseEpoch < 1 || !validAgentPrecheckDigest(request.Payload.BindingDigest) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	_, err := s.agentConnectionTests.AcknowledgeDataSourceConnectionTest(r.Context(), store.DataSourceConnectionTestAcknowledgement{
		AgentID: machine.AgentID, ConnectionTestID: connectionTestID, LeaseID: request.Payload.LeaseID,
		LeaseEpoch: request.Payload.LeaseEpoch, BindingDigest: request.Payload.BindingDigest, RequestID: request.RequestID,
		RequestDigest: agentConnectionTestRequestDigest("ACKNOWLEDGE", request.agentConnectionTestEnvelope, connectionTestID, request.Payload),
		Now:           time.Now().UTC(),
	})
	if err != nil {
		writeAgentConnectionTestStoreError(w, err)
		return
	}
	writeAgentConnectionTestResponse(w, "DATA_SOURCE_CONNECTION_TEST_LEASE_ACKNOWLEDGED", map[string]any{"realExecutionEnabled": false})
}

// resolveAuthenticatedConnectionTestSecret 只在已确认的基础连接测试租约内返回唯一数据库槽位。
// 明文仅短时存在于 HTTPS 响应编码期间，不进入日志、审计、SQLite 或浏览器响应。
func (s *Server) resolveAuthenticatedConnectionTestSecret(w http.ResponseWriter, r *http.Request, connectionTestID string) {
	machine, ok := s.authenticatedConnectionTestAgent(w, r)
	if !ok {
		return
	}
	if s.connectionTestSecrets == nil || s.decryptor == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "CONNECTION_TEST_SECRET_RESOLUTION_NOT_CONFIGURED", "当前环境尚未配置连接测试秘密槽位", false)
		return
	}
	var request agentConnectionTestSecretResolveRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validConnectionTestEnvelope(machine, request.agentConnectionTestEnvelope, "DATA_SOURCE_CONNECTION_TEST_RESOLVE_SECRET_SLOTS") ||
		!validDataSourceConnectionTestPathID(connectionTestID) || !validAgentPrecheckOpaque(request.Payload.LeaseID) ||
		request.Payload.LeaseEpoch < 1 || !validAgentPrecheckDigest(request.Payload.BindingDigest) ||
		(request.Payload.Slot != "DATABASE_CONNECTION" && request.Payload.Slot != "SYS_CONNECTION") {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	now := time.Now().UTC()
	requestDigest := agentConnectionTestRequestDigest("RESOLVE_SECRET", request.agentConnectionTestEnvelope, connectionTestID, request.Payload)
	var encrypted store.EncryptedDataSourceConnectionTestDatabaseConnection
	var err error
	if request.Payload.Slot == "SYS_CONNECTION" {
		// 可选的 sys 凭据槽位：仅绑定冻结了 sys 引用时可由 Store 解析（参考 ODC 的 sys 账号验证）。
		encrypted, err = s.connectionTestSecrets.ResolveDataSourceConnectionTestSysCredential(r.Context(), store.DataSourceConnectionTestSecretResolutionRequest{
			AgentID: machine.AgentID, ConnectionTestID: connectionTestID, LeaseID: request.Payload.LeaseID,
			LeaseEpoch: request.Payload.LeaseEpoch, BindingDigest: request.Payload.BindingDigest, RequestID: request.RequestID,
			RequestDigest: requestDigest, Now: now,
		})
	} else {
		encrypted, err = s.connectionTestSecrets.ResolveDataSourceConnectionTestDatabaseConnection(r.Context(), store.DataSourceConnectionTestSecretResolutionRequest{
			AgentID: machine.AgentID, ConnectionTestID: connectionTestID, LeaseID: request.Payload.LeaseID,
			LeaseEpoch: request.Payload.LeaseEpoch, BindingDigest: request.Payload.BindingDigest, RequestID: request.RequestID,
			RequestDigest: requestDigest, Now: now,
		})
	}
	if err != nil {
		writeAgentConnectionTestStoreError(w, err)
		return
	}
	defer encrypted.Destroy()
	finish := func(succeeded bool) error {
		return s.connectionTestSecrets.FinishDataSourceConnectionTestSecretResolution(r.Context(), store.DataSourceConnectionTestSecretResolutionOutcome{
			AgentID: machine.AgentID, ConnectionTestID: connectionTestID, LeaseID: request.Payload.LeaseID,
			LeaseEpoch: request.Payload.LeaseEpoch, BindingDigest: request.Payload.BindingDigest, RequestID: request.RequestID,
			RequestDigest: requestDigest, Succeeded: succeeded, Now: time.Now().UTC(),
		})
	}
	owner := identity.Principal{Type: identity.BrowserPrincipal, ID: encrypted.OwnerSubjectID}
	if identity.Validate(owner, identity.BrowserPrincipal) != nil ||
		identity.Can(r.Context(), s.authorizer, owner, identity.ScopeDataSourceRead, encrypted.DataSourceID) != nil ||
		identity.Can(r.Context(), s.authorizer, owner, identity.ScopeNodeUse, encrypted.NodeID) != nil {
		if finishErr := finish(false); finishErr != nil {
			writeAgentConnectionTestStoreError(w, finishErr)
			return
		}
		writeAgentConnectionTestStoreError(w, store.ErrDataSourceConnectionTestLeaseRejected)
		return
	}
	plaintext, decryptErr := s.decryptor.Decrypt(credential.Envelope{
		FormatVersion: credential.FormatVersion, KeyID: encrypted.KeyID,
		Reference: credential.Reference{CredentialID: encrypted.CredentialID, Revision: encrypted.Revision, SecretType: connectionTestSecretType(request.Payload.Slot), DataSourceID: encrypted.DataSourceID},
		Nonce:     encrypted.Nonce, Ciphertext: encrypted.Ciphertext,
	})
	if decryptErr != nil {
		if finishErr := finish(false); finishErr != nil {
			writeAgentConnectionTestStoreError(w, finishErr)
			return
		}
		writeError(w, http.StatusServiceUnavailable, "CONNECTION_TEST_SECRET_RESOLUTION_UNAVAILABLE", "连接测试秘密槽位暂时不可用", true)
		return
	}
	defer credential.Zero(plaintext)
	if err := finish(true); err != nil {
		writeAgentConnectionTestStoreError(w, err)
		return
	}
	writeAgentConnectionTestResponse(w, "DATA_SOURCE_CONNECTION_TEST_SECRET_SLOTS_RESOLVED", map[string]any{
		"agentRequestId": request.RequestID, "connectionTestId": connectionTestID, "leaseId": request.Payload.LeaseID,
		"leaseEpoch": request.Payload.LeaseEpoch, "bindingDigest": request.Payload.BindingDigest, "slot": request.Payload.Slot,
		"connection":           map[string]any{"host": encrypted.Host, "port": encrypted.Port, "username": encrypted.Username, "password": plaintext},
		"realExecutionEnabled": false,
	})
}

// connectionTestSecretType 按槽位类型返回对应的加密信封秘密类型（sys 凭据与数据库密码互不通用）。
func connectionTestSecretType(slot string) string {
	if slot == "SYS_CONNECTION" {
		return credential.SysPassword
	}
	return credential.DatabasePassword
}

// completeAuthenticatedConnectionTest 保存基础连接测试的三种固定结果。
// 状态与来源会由 Store 再次复验当前绑定；G2 合成结论不能写为数据源启用事实。
func (s *Server) completeAuthenticatedConnectionTest(w http.ResponseWriter, r *http.Request, connectionTestID string) {
	machine, ok := s.authenticatedConnectionTestAgent(w, r)
	if !ok {
		return
	}
	if s.connectionTests == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_CONNECTION_TEST_NOT_CONFIGURED", "当前环境尚未配置受认证 Agent 连接测试", false)
		return
	}
	var request agentConnectionTestCompletionRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validConnectionTestEnvelope(machine, request.agentConnectionTestEnvelope, "DATA_SOURCE_CONNECTION_TEST_COMPLETE") ||
		!validDataSourceConnectionTestPathID(connectionTestID) || !validAgentPrecheckOpaque(request.Payload.LeaseID) ||
		request.Payload.LeaseEpoch < 1 || !validAgentPrecheckDigest(request.Payload.BindingDigest) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	run, err := s.connectionTests.GetDataSourceConnectionTestRun(r.Context(), connectionTestID)
	if err != nil {
		writeAgentConnectionTestStoreError(w, err)
		return
	}
	if !validConnectionTestResult(run.VerificationSource, request.Payload.Status, request.Payload.EvidenceCode) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	// 可选的 sys 凭据验证结果受 store 枚举校验（与数据库结果相互独立）。
	result, err := s.agentConnectionTests.CompleteAgentDataSourceConnectionTest(r.Context(), store.AgentDataSourceConnectionTestCompletion{
		AgentID: machine.AgentID, ConnectionTestID: connectionTestID, LeaseID: request.Payload.LeaseID,
		LeaseEpoch: request.Payload.LeaseEpoch, BindingDigest: request.Payload.BindingDigest, RequestID: request.RequestID,
		RequestDigest: agentConnectionTestRequestDigest("COMPLETE", request.agentConnectionTestEnvelope, connectionTestID, request.Payload),
		Status:        request.Payload.Status, EvidenceCode: request.Payload.EvidenceCode, VerificationSource: run.VerificationSource,
		SysVerificationStatus: request.Payload.SysVerificationStatus, SysResultCode: request.Payload.SysEvidenceCode, Now: time.Now().UTC(),
	})
	if err != nil {
		writeAgentConnectionTestStoreError(w, err)
		return
	}
	writeAgentConnectionTestResponse(w, "DATA_SOURCE_CONNECTION_TEST_COMPLETED", map[string]any{
		"status": result.Status, "evidenceCode": result.EvidenceCode, "verificationSource": result.VerificationSource,
		"sysVerificationStatus": result.SysVerificationStatus, "sysEvidenceCode": result.SysResultCode,
		"realExecutionEnabled": false,
	})
}

func validConnectionTestResult(verificationSource, status, evidenceCode string) bool {
	if verificationSource == "G2_SYNTHETIC" {
		return status == "SUCCEEDED" && evidenceCode == "SYNTHETIC_OK"
	}
	return verificationSource == "AGENT_JDBC" &&
		((status == "SUCCEEDED" && evidenceCode == "DATABASE_CONNECTED") ||
			(status == "FAILED" && evidenceCode == "DATABASE_HOST_UNRESOLVABLE") ||
			(status == "FAILED" && evidenceCode == "DATABASE_TCP_REFUSED") ||
			(status == "FAILED" && evidenceCode == "DATABASE_TCP_TIMEOUT") ||
			(status == "FAILED" && evidenceCode == "DATABASE_TCP_UNREACHABLE") ||
			(status == "FAILED" && evidenceCode == "DATABASE_CONNECTION_FAILED") ||
			(status == "UNKNOWN" && evidenceCode == "DATABASE_CONNECTION_UNAVAILABLE"))
}

func parseAuthenticatedPrecheckAction(path string) (string, string, bool) {
	const prefix = "/agent/v1/prechecks/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	value := strings.TrimPrefix(path, prefix)
	for suffix, action := range map[string]string{":acknowledge-lease": "acknowledge-lease", "/secret-slots:resolve": "resolve-secret-slots", ":complete": "complete"} {
		if strings.HasSuffix(value, suffix) {
			precheckID := strings.TrimSuffix(value, suffix)
			return precheckID, action, validAgentPrecheckPathID(precheckID)
		}
	}
	return "", "", false
}

// parseAuthenticatedConnectionTestAction 只识别基础连接测试的三种固定后续动作。
// 任何其他后缀都不能被解释为远程命令、文件操作或通用 Agent 控制接口。
func parseAuthenticatedConnectionTestAction(path string) (string, string, bool) {
	const prefix = "/agent/v1/data-source-connection-tests/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	value := strings.TrimPrefix(path, prefix)
	for suffix, action := range map[string]string{":acknowledge-lease": "acknowledge-lease", "/secret-slots:resolve": "resolve-secret-slots", ":complete": "complete"} {
		if strings.HasSuffix(value, suffix) {
			connectionTestID := strings.TrimSuffix(value, suffix)
			return connectionTestID, action, validDataSourceConnectionTestPathID(connectionTestID)
		}
	}
	return "", "", false
}

// parseAuthenticatedExecutionNodeEnvironmentCheckCompletion 只匹配固定环境检查回执路径。
// 检查标识是控制面生成的不透明值，不能携带路径语义或扩展为通用 Agent 操作。
func parseAuthenticatedExecutionNodeEnvironmentCheckCompletion(path string) (string, bool) {
	const prefix = "/agent/v1/execution-node-environment-checks/"
	const suffix = ":complete"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	checkID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return checkID, validAgentPrecheckPathID(checkID)
}

// agentEntry 让首次关联在没有既有机器凭据时到达受控交换端点。
// 默认只开放 G2 固定能力；Windows 本机 MVP 显式执行模式才额外开放受租约约束的 OBDUMPER 导出路径。
func (s *Server) agentEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/agent/v1/enrollments:exchange" {
		s.exchangeAgentEnrollment(w, r)
		return
	}
	if s.agentProtocol != nil {
		if r.Method == http.MethodPost && r.URL.Path == "/agent/v1/heartbeats" {
			s.recordAgentHeartbeat(w, r)
			return
		}
		if r.Method == http.MethodPost {
			if checkID, ok := parseAuthenticatedExecutionNodeEnvironmentCheckCompletion(r.URL.Path); ok {
				s.completeAuthenticatedExecutionNodeEnvironmentCheck(w, r, checkID)
				return
			}
		}
		if r.Method == http.MethodPost && r.URL.Path == "/agent/v1/prechecks:claim-next" {
			s.claimNextAuthenticatedPrecheck(w, r)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/agent/v1/data-source-connection-tests:claim-next" {
			s.claimNextAuthenticatedConnectionTest(w, r)
			return
		}
		if s.realExecutionEnabled && r.Method == http.MethodPost && r.URL.Path == "/agent/v1/executions:claim-next" {
			s.claimNextAuthenticatedExecution(w, r)
			return
		}
		if r.Method == http.MethodPost {
			if precheckID, action, ok := parseAuthenticatedPrecheckAction(r.URL.Path); ok {
				switch action {
				case "acknowledge-lease":
					s.acknowledgeAuthenticatedPrecheck(w, r, precheckID)
					return
				case "resolve-secret-slots":
					s.resolveAuthenticatedPrecheckSecret(w, r, precheckID)
					return
				case "complete":
					s.completeAuthenticatedPrecheck(w, r, precheckID)
					return
				}
			}
			if connectionTestID, action, ok := parseAuthenticatedConnectionTestAction(r.URL.Path); ok {
				switch action {
				case "acknowledge-lease":
					s.acknowledgeAuthenticatedConnectionTest(w, r, connectionTestID)
					return
				case "resolve-secret-slots":
					s.resolveAuthenticatedConnectionTestSecret(w, r, connectionTestID)
					return
				case "complete":
					s.completeAuthenticatedConnectionTest(w, r, connectionTestID)
					return
				}
			}
			if s.realExecutionEnabled {
				if executionID, action, ok := parseAuthenticatedExecutionAction(r.URL.Path); ok {
					switch action {
					case "acknowledge-lease":
						s.acknowledgeAuthenticatedExecution(w, r, executionID)
						return
					case "renew-lease":
						s.renewAuthenticatedExecution(w, r, executionID)
						return
					case "resolve-secret-slots":
						s.resolveAuthenticatedExecutionSecret(w, r, executionID)
						return
					case "append-events":
						s.appendAuthenticatedExecutionEvent(w, r, executionID)
						return
					case "append-logs":
						s.appendAuthenticatedExecutionLog(w, r, executionID)
						return
					case "append-log-gap":
						s.appendAuthenticatedExecutionLogGap(w, r, executionID)
						return
					}
				}
			}
		}
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_G2_ONLY", "当前 Agent 协议仅开放关联、心跳、固定预检查与基础连接测试", false)
		return
	}
	s.agentAuthenticated(w, r)
}

// agentAuthenticated 保留既有合成 Agent 协议测试适配。
// 正式机器凭据仓储启用后，agentEntry 会在此函数之前拒绝未实现的执行相关端点。
func (s *Server) agentAuthenticated(w http.ResponseWriter, r *http.Request) {
	principal, err := s.provider.AuthenticateAgent(r)
	if err != nil || identity.Validate(principal, identity.AgentPrincipal) != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/agent/v1/executions:claim" {
		s.claimSyntheticExecution(w, r, principal)
		return
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/agent/v1/executions/") && strings.HasSuffix(r.URL.Path, ":renew") {
		executionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agent/v1/executions/"), ":renew")
		if executionID != "" && !strings.Contains(executionID, "/") {
			s.renewSyntheticExecution(w, r, principal, executionID)
			return
		}
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/agent/v1/executions/") && strings.HasSuffix(r.URL.Path, ":events:append") {
		executionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agent/v1/executions/"), ":events:append")
		if executionID != "" && !strings.Contains(executionID, "/") {
			s.appendSyntheticExecutionEvent(w, r, principal, executionID)
			return
		}
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/agent/v1/executions/") && strings.HasSuffix(r.URL.Path, ":logs:append") {
		executionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agent/v1/executions/"), ":logs:append")
		if executionID != "" && !strings.Contains(executionID, "/") {
			s.appendSyntheticLogBatch(w, r, principal, executionID)
			return
		}
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/agent/v1/executions/") && strings.HasSuffix(r.URL.Path, ":logs:gap") {
		executionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agent/v1/executions/"), ":logs:gap")
		if executionID != "" && !strings.Contains(executionID, "/") {
			s.appendSyntheticLogGap(w, r, principal, executionID)
			return
		}
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

// syntheticExecutionClaimRequest 只描述固定任务的领取租约，不承载命令、凭据或路径。
type syntheticExecutionClaimRequest struct {
	RequestID   string `json:"requestId"`
	TaskID      string `json:"taskId"`
	ExecutionID string `json:"executionId"`
	NodeID      string `json:"nodeId"`
	LeaseID     string `json:"leaseId"`
}

// syntheticExecutionEventRequest 只允许 Agent 上报状态机定义的事实。
// 事件负载不从 HTTP 透传，避免形成任意日志或命令写入通道。
type syntheticExecutionEventRequest struct {
	EventID       string                  `json:"eventId"`
	LeaseID       string                  `json:"leaseId"`
	LeaseEpoch    int64                   `json:"leaseEpoch"`
	Sequence      int64                   `json:"sequence"`
	Type          agentstate.EventType    `json:"type"`
	NoProcess     bool                    `json:"noProcess"`
	ProcessExited bool                    `json:"processExited"`
	ToolTerminal  agentstate.ToolTerminal `json:"toolTerminal"`
	ResultFacts   agentstate.ResultFacts  `json:"resultFacts"`
}

// syntheticExecutionRenewRequest 只能续期已领取的租约，不能改变任务绑定。
type syntheticExecutionRenewRequest struct {
	RequestID  string `json:"requestId"`
	LeaseID    string `json:"leaseId"`
	LeaseEpoch int64  `json:"leaseEpoch"`
}

// claimSyntheticExecution 在协调器先生成租约，再将同一领取事实写入 SQLite。
// 该 G2 适配不启动子进程；初始 SCHEDULED 事件占用序号一，后续 Agent 事件从二开始。
func (s *Server) claimSyntheticExecution(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.coordinator == nil || s.executions == nil || s.precheckTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_NOT_CONFIGURED", "当前环境尚未配置合成 Agent 协议", false)
		return
	}
	var request syntheticExecutionClaimRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	grant, err := s.coordinator.Claim(agentstate.ClaimRequest{RequestID: request.RequestID, TaskID: request.TaskID, ExecutionID: request.ExecutionID, NodeID: request.NodeID, AgentID: principal.ID, LeaseID: request.LeaseID, LeaseTTL: s.precheckTTL})
	if errors.Is(err, agentstate.ErrClaimIneligible) || errors.Is(err, agentstate.ErrTaskAlreadyClaimed) {
		writeError(w, http.StatusConflict, "EXECUTION_CLAIM_REJECTED", "当前 Agent 无可领取的任务", true)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	scheduledEventID := "scheduled-" + request.ExecutionID
	if _, err := s.coordinator.Append(agentstate.Event{EventID: scheduledEventID, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, Sequence: 1, Type: agentstate.EventScheduled}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_CLAIM_UNAVAILABLE", "任务领取暂时不可用", true)
		return
	}
	err = s.executions.ClaimTask(r.Context(), store.Claim{ExecutionID: grant.ExecutionID, TaskID: request.TaskID, NodeID: request.NodeID, AgentID: principal.ID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, IssuedAt: time.Now().UTC(), ExpiresAt: grant.ExpiresAt, EventID: scheduledEventID, RequestID: request.RequestID})
	if err != nil && !errors.Is(err, store.ErrAlreadyClaimed) {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_CLAIM_UNAVAILABLE", "任务领取暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "grant": grant, "nextEventSequence": 2, "realExecutionEnabled": false})
}

// renewSyntheticExecution 统一以控制面时钟续期，并同步安全租约投影。
func (s *Server) renewSyntheticExecution(w http.ResponseWriter, r *http.Request, principal identity.Principal, executionID string) {
	if s.coordinator == nil || s.executions == nil || s.precheckTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_NOT_CONFIGURED", "当前环境尚未配置合成 Agent 协议", false)
		return
	}
	var request syntheticExecutionRenewRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	snapshot, err := s.coordinator.Snapshot(executionID)
	if err != nil || snapshot.AgentID != principal.ID {
		writeError(w, http.StatusConflict, "EXECUTION_LEASE_REJECTED", "任务租约无效", false)
		return
	}
	grant, err := s.coordinator.Renew(agentstate.RenewRequest{RequestID: request.RequestID, ExecutionID: executionID, LeaseID: request.LeaseID, LeaseEpoch: request.LeaseEpoch, LeaseTTL: s.precheckTTL})
	if err != nil {
		writeError(w, http.StatusConflict, "EXECUTION_LEASE_REJECTED", "任务租约无效", false)
		return
	}
	if err := s.executions.RenewExecutionLease(r.Context(), store.LeaseRenewal{ExecutionID: executionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, AgentID: principal.ID, ExpiresAt: grant.ExpiresAt}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_LEASE_UNAVAILABLE", "任务租约暂时无法续期", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "grant": grant, "realExecutionEnabled": false})
}

// appendSyntheticExecutionEvent 先由状态机判定租约和顺序，再保存不含原始负载的接受事实。
func (s *Server) appendSyntheticExecutionEvent(w http.ResponseWriter, r *http.Request, principal identity.Principal, executionID string) {
	if s.coordinator == nil || s.executions == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_NOT_CONFIGURED", "当前环境尚未配置合成 Agent 协议", false)
		return
	}
	var request syntheticExecutionEventRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	snapshot, err := s.coordinator.Snapshot(executionID)
	if err != nil || snapshot.AgentID != principal.ID {
		writeError(w, http.StatusConflict, "EXECUTION_LEASE_REJECTED", "任务租约无效", false)
		return
	}
	result, err := s.coordinator.Append(agentstate.Event{EventID: request.EventID, ExecutionID: executionID, LeaseID: request.LeaseID, LeaseEpoch: request.LeaseEpoch, Sequence: request.Sequence, Type: request.Type, Facts: agentstate.Facts{NoProcess: request.NoProcess, ProcessExited: request.ProcessExited, ToolTerminal: request.ToolTerminal, ResultFacts: request.ResultFacts}})
	if errors.Is(err, agentstate.ErrEventSequenceReused) || errors.Is(err, agentstate.ErrInvalidStateMutation) || errors.Is(err, agentstate.ErrEventRejected) {
		writeError(w, http.StatusConflict, "EXECUTION_EVENT_REJECTED", "任务事件不符合当前状态", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	if result.Decision == agentstate.EventGap || result.Decision == agentstate.EventStale {
		writeJSON(w, http.StatusConflict, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSequence})
		return
	}
	if result.Decision == agentstate.EventAccepted {
		err = s.executions.AppendExecutionEvent(r.Context(), store.ExecutionEvent{EventID: request.EventID, ExecutionID: executionID, LeaseID: request.LeaseID, LeaseEpoch: request.LeaseEpoch, EventSeq: request.Sequence, EventType: string(request.Type), PayloadJSON: `{"mode":"synthetic"}`, ReceivedAt: time.Now().UTC()})
		if err != nil && !errors.Is(err, store.ErrEventRejected) {
			writeError(w, http.StatusServiceUnavailable, "EXECUTION_EVENT_UNAVAILABLE", "任务事件暂时无法保存", true)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSequence, "state": result.Snapshot.State, "realExecutionEnabled": false})
}

// syntheticLogBatchRequest 将日志批次绑定到当前领取租约，避免任意 Agent 写入其他任务。
type syntheticLogBatchRequest struct {
	LeaseID    string          `json:"leaseId"`
	LeaseEpoch int64           `json:"leaseEpoch"`
	Batch      logstream.Batch `json:"batch"`
}

// syntheticLogGapRequest 只保留序号缺口事实，不保留 Agent 提供的原因文本。
type syntheticLogGapRequest struct {
	LeaseID    string              `json:"leaseId"`
	LeaseEpoch int64               `json:"leaseEpoch"`
	Gap        logstream.GapNotice `json:"gap"`
}

// syntheticLogStore 在未配置持久分段存储时保留 G2 的已脱敏内存投影。
// 配置持久存储后，普通日志批次只走文件 fsync 与 SQLite 索引，避免两套正文产生不一致。
type syntheticLogStore struct {
	mu         sync.Mutex
	ledger     *logstream.BatchLedger
	persistent *logstream.PersistentStore
	byTask     map[string][]logstream.Record
}

func newSyntheticLogStore(ledger *logstream.BatchLedger, persistent *logstream.PersistentStore) *syntheticLogStore {
	if ledger == nil && persistent == nil {
		return nil
	}
	return &syntheticLogStore{ledger: ledger, persistent: persistent, byTask: make(map[string][]logstream.Record)}
}

func (s *syntheticLogStore) appendBatch(ctx context.Context, taskID, executionID string, batch logstream.Batch) (logstream.BatchResult, error) {
	if s.persistent != nil {
		return s.persistent.Append(ctx, executionID, batch)
	}
	if s.ledger == nil {
		return logstream.BatchResult{}, logstream.ErrInvalidInput
	}
	for _, record := range batch.Records {
		redacted, err := (logstream.Policy{Version: batch.PolicyVersion}).Redact(record.Message)
		if err != nil || redacted != record.Message {
			return logstream.BatchResult{}, logstream.ErrPolicyRejected
		}
	}
	result, err := s.ledger.Accept(batch)
	if err != nil || result.Decision != logstream.BatchAccepted {
		return result, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byTask[taskID] = append(s.byTask[taskID], batch.Records...)
	return result, nil
}

func (s *syntheticLogStore) appendGap(ctx context.Context, taskID, executionID string, gap logstream.GapNotice) (logstream.BatchResult, error) {
	if s.persistent != nil {
		return s.persistent.AppendGap(ctx, executionID, gap)
	}
	if s.ledger == nil {
		return logstream.BatchResult{}, logstream.ErrInvalidInput
	}
	result, err := s.ledger.AcceptGap(gap)
	if err != nil || result.Decision != logstream.BatchAccepted {
		return result, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byTask[taskID] = append(s.byTask[taskID], logstream.Record{StreamID: gap.StreamID, SourceKind: gap.SourceKind, SourceEpoch: gap.SourceEpoch, SourceSeq: gap.FirstSeq, Kind: logstream.RecordGap, Message: "日志序号存在缺口", IntegrityCode: gap.ReasonCode, PolicyVersion: gap.PolicyVersion, ParserVersion: gap.ParserVersion, ReceivedAt: time.Now().UTC()})
	return result, nil
}

func (s *syntheticLogStore) records(taskID string) []logstream.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]logstream.Record(nil), s.byTask[taskID]...)
}

func (s *syntheticLogStore) persistentStore() *logstream.PersistentStore {
	if s == nil {
		return nil
	}
	return s.persistent
}

// appendSyntheticLogBatch 拒绝含未脱敏键值的批次，并由批次账本处理重复与序号缺口。
func (s *Server) appendSyntheticLogBatch(w http.ResponseWriter, r *http.Request, principal identity.Principal, executionID string) {
	if s.coordinator == nil || s.logs == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_NOT_CONFIGURED", "当前环境尚未配置合成日志协议", false)
		return
	}
	var request syntheticLogBatchRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	snapshot, err := s.coordinator.Snapshot(executionID)
	if err != nil || snapshot.AgentID != principal.ID || snapshot.LeaseID != request.LeaseID || snapshot.LeaseEpoch != request.LeaseEpoch || request.Batch.StreamID != executionID {
		writeError(w, http.StatusConflict, "EXECUTION_LEASE_REJECTED", "任务租约无效", false)
		return
	}
	result, err := s.logs.appendBatch(r.Context(), snapshot.TaskID, executionID, request.Batch)
	if errors.Is(err, logstream.ErrBatchGap) {
		writeJSON(w, http.StatusConflict, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSeq})
		return
	}
	if errors.Is(err, logstream.ErrPolicyRejected) || errors.Is(err, logstream.ErrInvalidInput) || errors.Is(err, logstream.ErrBatchConflict) {
		writeError(w, http.StatusBadRequest, "LOG_BATCH_REJECTED", "日志批次不符合安全约束", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "LOG_BATCH_UNAVAILABLE", "日志批次暂时无法保存", true)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSeq, "realExecutionEnabled": false})
}

// appendSyntheticLogGap 记录可审查的序号缺口，不接受或展示 Agent 的原因原文。
func (s *Server) appendSyntheticLogGap(w http.ResponseWriter, r *http.Request, principal identity.Principal, executionID string) {
	if s.coordinator == nil || s.logs == nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_PROTOCOL_NOT_CONFIGURED", "当前环境尚未配置合成日志协议", false)
		return
	}
	var request syntheticLogGapRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	snapshot, err := s.coordinator.Snapshot(executionID)
	if err != nil || snapshot.AgentID != principal.ID || snapshot.LeaseID != request.LeaseID || snapshot.LeaseEpoch != request.LeaseEpoch || request.Gap.StreamID != executionID {
		writeError(w, http.StatusConflict, "EXECUTION_LEASE_REJECTED", "任务租约无效", false)
		return
	}
	result, err := s.logs.appendGap(r.Context(), snapshot.TaskID, executionID, request.Gap)
	if errors.Is(err, logstream.ErrBatchGap) {
		writeJSON(w, http.StatusConflict, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSeq})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "LOG_GAP_REJECTED", "日志缺口不符合安全约束", false)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSeq, "realExecutionEnabled": false})
}

// taskLogCursor 将页面游标绑定到单一已授权任务和当前主体。
// 它只保存内部批次位置，不能携带文件路径、SQLite 行号、Agent 序号、配置或秘密引用。
type taskLogCursor struct {
	SubjectDigest string `json:"subjectDigest"`
	TaskDigest    string `json:"taskDigest"`
	SnapshotAt    string `json:"snapshotAt"`
	SnapshotBatch string `json:"snapshotBatch"`
	PositionAt    string `json:"positionAt"`
	PositionBatch string `json:"positionBatch"`
	RecordOffset  int    `json:"recordOffset"`
}

// listSyntheticLogs 在配置持久段存储时读取固定水位页或最后可靠游标之后的增量。
// 未配置时才保留 G2 内存投影，不能把该降级路径描述为可跨重启恢复的日志。
func (s *Server) listSyntheticLogs(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	if s.tasks == nil || s.logs == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置日志查询依赖", false)
		return
	}
	_, err := s.tasks.GetAuthorizedTaskSummary(r.Context(), taskID, principal.ID)
	if errors.Is(err, store.ErrDataSourceNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_QUERY_UNAVAILABLE", "任务暂时不可用", true)
		return
	}
	if persistent := s.logs.persistentStore(); persistent != nil {
		cursorValue := strings.TrimSpace(r.URL.Query().Get("cursor"))
		afterValue := strings.TrimSpace(r.URL.Query().Get("after"))
		if cursorValue != "" && afterValue != "" {
			writeError(w, http.StatusBadRequest, "CURSOR_INVALID", "日志游标无效", false)
			return
		}
		var records []logstream.Record
		var next, last *logstream.PageCursor
		if afterValue != "" {
			after, ok := decodeTaskLogCursor(afterValue, principal.ID, taskID)
			if !ok {
				writeError(w, http.StatusBadRequest, "CURSOR_INVALID", "日志游标无效", false)
				return
			}
			records, last, err = persistent.ReadSince(r.Context(), taskID, after)
		} else {
			var cursor *logstream.PageCursor
			if cursorValue != "" {
				var ok bool
				cursor, ok = decodeTaskLogCursor(cursorValue, principal.ID, taskID)
				if !ok {
					writeError(w, http.StatusBadRequest, "CURSOR_INVALID", "日志游标无效", false)
					return
				}
			}
			records, next, last, err = persistent.ReadPage(r.Context(), taskID, cursor)
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "LOG_QUERY_UNAVAILABLE", "日志暂时不可读取", true)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"requestId":          requestID(w),
			"items":              taskLogResponses(records),
			"nextCursor":         optionalTaskLogCursor(next, principal.ID, taskID),
			"lastReliableCursor": optionalTaskLogCursor(last, principal.ID, taskID),
			"integrity":          "PERSISTED_DOUBLE_REDACTED",
		})
		return
	}
	integrity := "SYNTHETIC_MEMORY"
	if s.realExecutionEnabled {
		integrity = "MEMORY_PROJECTION"
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "items": taskLogResponses(s.logs.records(taskID)), "integrity": integrity, "realExecutionEnabled": s.realExecutionEnabled})
}

// streamTaskLogs 为一个已授权且活动中的任务提供持久日志的单向增量传输。
// SSE 事件只在文件 fsync 与 SQLite 批次登记均成功后发送；连接恢复本身不表示 Agent 端没有缺口。
func (s *Server) streamTaskLogs(w http.ResponseWriter, r *http.Request, principal identity.Principal, taskID string) {
	if s.tasks == nil || s.logs == nil || s.logs.persistentStore() == nil {
		writeError(w, http.StatusServiceUnavailable, "LOG_STREAM_UNAVAILABLE", "当前环境尚未配置持久日志流", true)
		return
	}
	summary, ok := s.loadAuthorizedTaskSummary(w, r, principal, taskID)
	if !ok {
		return
	}
	if summary.State != "STARTING" && summary.State != "RUNNING" {
		writeError(w, http.StatusConflict, "LOG_STREAM_INACTIVE", "当前任务没有活动日志流", false)
		return
	}
	persistent := s.logs.persistentStore()
	afterValue := strings.TrimSpace(r.URL.Query().Get("after"))
	if afterValue == "" {
		afterValue = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	var after *logstream.PageCursor
	if afterValue != "" {
		var cursorOK bool
		after, cursorOK = decodeTaskLogCursor(afterValue, principal.ID, taskID)
		if !cursorOK {
			writeError(w, http.StatusBadRequest, "CURSOR_INVALID", "日志游标无效", false)
			return
		}
	}
	flusher, flushOK := responseFlusher(w)
	if !flushOK {
		writeError(w, http.StatusServiceUnavailable, "LOG_STREAM_UNAVAILABLE", "当前环境不支持日志流", true)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "retry: 2000\n\n")
	flusher.Flush()

	updates := persistent.Subscribe()
	last, err := s.writeTaskLogStreamPage(r.Context(), w, principal, taskID, after)
	if err != nil {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			var pageErr error
			last, pageErr = s.writeTaskLogStreamPage(r.Context(), w, principal, taskID, last)
			if pageErr != nil {
				return
			}
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) writeTaskLogStreamPage(ctx context.Context, w http.ResponseWriter, principal identity.Principal, taskID string, after *logstream.PageCursor) (*logstream.PageCursor, error) {
	// 长连接不能复用建立时的授权结论；权限撤销后直接关闭流，避免继续发送任何新记录。
	if _, err := s.tasks.GetAuthorizedTaskSummary(ctx, taskID, principal.ID); err != nil {
		return after, err
	}
	persistent := s.logs.persistentStore()
	flusher, flushOK := responseFlusher(w)
	if !flushOK {
		return after, errors.New("日志流响应不支持刷新")
	}
	current := after
	for sent := 0; sent < 200; sent++ {
		record, last, found, err := persistent.ReadNextSince(ctx, taskID, current)
		if err != nil {
			return current, err
		}
		if !found {
			break
		}
		payload, marshalErr := json.Marshal(taskLogResponses([]logstream.Record{record})[0])
		if marshalErr != nil {
			return current, marshalErr
		}
		cursor := encodeTaskLogCursor(last, principal.ID, taskID)
		if cursor == "" {
			return current, logstream.ErrStorageCorrupt
		}
		if _, err := fmt.Fprintf(w, "id: %s\nevent: log\ndata: %s\n\n", cursor, payload); err != nil {
			return current, err
		}
		flusher.Flush()
		current = last
	}
	return current, nil
}

// responseFlusher 解开仅附加响应元数据的包装层，保留 SSE 对底层刷新能力的准确判断。
// 包装链异常或超过有限层数时失败关闭，不能把无刷新能力的响应误当作可用日志流。
func responseFlusher(w http.ResponseWriter) (http.Flusher, bool) {
	for depth := 0; depth < 8 && w != nil; depth++ {
		if flusher, ok := w.(http.Flusher); ok {
			return flusher, true
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil, false
		}
		w = unwrapper.Unwrap()
	}
	return nil, false
}

func optionalTaskLogCursor(cursor *logstream.PageCursor, subjectID, taskID string) any {
	if cursor == nil {
		return nil
	}
	encoded := encodeTaskLogCursor(cursor, subjectID, taskID)
	if encoded == "" {
		return nil
	}
	return encoded
}

func decodeTaskLogCursor(value, subjectID, taskID string) (*logstream.PageCursor, bool) {
	if len(value) > 2048 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) > 1024 {
		return nil, false
	}
	var cursor taskLogCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, false
	}
	snapshotAt, snapshotOK := parseTaskLogCursorTime(cursor.SnapshotAt)
	positionAt, positionOK := parseTaskLogCursorTime(cursor.PositionAt)
	if !snapshotOK || !positionOK || cursor.RecordOffset < 0 || !validInternalLogID(cursor.SnapshotBatch) || !validInternalLogID(cursor.PositionBatch) ||
		subtle.ConstantTimeCompare([]byte(cursor.SubjectDigest), []byte(taskLogSubjectDigest(subjectID))) != 1 ||
		subtle.ConstantTimeCompare([]byte(cursor.TaskDigest), []byte(taskLogTaskDigest(taskID))) != 1 {
		return nil, false
	}
	return &logstream.PageCursor{
		Snapshot:     logstream.BatchPosition{ReceivedAt: snapshotAt, BatchID: cursor.SnapshotBatch},
		Position:     logstream.BatchPosition{ReceivedAt: positionAt, BatchID: cursor.PositionBatch},
		RecordOffset: cursor.RecordOffset,
	}, true
}

func encodeTaskLogCursor(cursor *logstream.PageCursor, subjectID, taskID string) string {
	if cursor == nil || cursor.RecordOffset < 0 || cursor.Snapshot.ReceivedAt.IsZero() || cursor.Position.ReceivedAt.IsZero() || !validInternalLogID(cursor.Snapshot.BatchID) || !validInternalLogID(cursor.Position.BatchID) {
		return ""
	}
	raw, err := json.Marshal(taskLogCursor{
		SubjectDigest: taskLogSubjectDigest(subjectID), TaskDigest: taskLogTaskDigest(taskID),
		SnapshotAt: cursor.Snapshot.ReceivedAt.UTC().Format(time.RFC3339Nano), SnapshotBatch: cursor.Snapshot.BatchID,
		PositionAt: cursor.Position.ReceivedAt.UTC().Format(time.RFC3339Nano), PositionBatch: cursor.Position.BatchID,
		RecordOffset: cursor.RecordOffset,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func taskLogSubjectDigest(subjectID string) string {
	digest := sha256.Sum256([]byte("task-log-cursor-v1|subject|" + subjectID))
	return hex.EncodeToString(digest[:])
}

func taskLogTaskDigest(taskID string) string {
	digest := sha256.Sum256([]byte("task-log-cursor-v1|task|" + taskID))
	return hex.EncodeToString(digest[:])
}

func parseTaskLogCursorTime(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed.UTC(), err == nil
}

func validInternalLogID(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, value := range value {
		if (value < '0' || value > '9') && (value < 'a' || value > 'f') {
			return false
		}
	}
	return true
}

// taskLogResponse 是浏览器日志接口的最小脱敏投影。
// 不能直接序列化领域记录，否则 Go 的导出字段会绕开既定的 lowerCamelCase API 契约。
type taskLogResponse struct {
	SourceSeq     int64  `json:"sourceSeq"`
	Kind          string `json:"kind"`
	Message       string `json:"message"`
	IntegrityCode string `json:"integrityCode"`
	ReceivedAt    string `json:"receivedAt"`
}

func taskLogResponses(records []logstream.Record) []taskLogResponse {
	items := make([]taskLogResponse, 0, len(records))
	for _, record := range records {
		items = append(items, taskLogResponse{
			SourceSeq:     record.SourceSeq,
			Kind:          string(record.Kind),
			Message:       record.Message,
			IntegrityCode: record.IntegrityCode,
			ReceivedAt:    record.ReceivedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return items
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
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "grant": map[string]any{"precheckId": grant.PrecheckID, "leaseId": grant.LeaseID, "leaseEpoch": grant.LeaseEpoch, "expiresAt": grant.ExpiresAt.Format(time.RFC3339Nano), "binding": grant.Binding}})
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
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "status": string(completed.State)})
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

// listDataSources 先按对象读取范围过滤，避免通过条目或总数发现无权数据源。
// 已授权列表只额外返回拆分保存的普通业务用户名，不返回 sys、组合身份或任何凭据材料。
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
	items := make([]dataSourceListResponse, 0, len(summaries))
	for _, summary := range summaries {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, summary.DataSourceID) != nil {
			continue
		}
		items = append(items, newDataSourceListResponse(summary))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "items": items})
}

// listExecutionNodes 返回节点管理员范围内的配置与明确的失败关闭状态。
// 当前组件不投影 Agent 环境事实，因此不会把节点声明配置渲染为可接收新任务。
func (s *Server) listExecutionNodes(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.nodeManagement == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置执行节点管理", false)
		return
	}
	nodes, err := s.nodeManagement.ListExecutionNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_QUERY_UNAVAILABLE", "执行节点暂时不可用", true)
		return
	}
	items := make([]executionNodeListResponse, 0, len(nodes))
	for _, node := range nodes {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, node.NodeID) != nil {
			continue
		}
		items = append(items, s.newExecutionNodeListResponse(node))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "items": items})
}

// createExecutionNode 只创建禁用节点记录并生成审计与幂等事实。
// Agent 关联、环境核对和节点启用必须由后续受认证协议写回，不能从浏览器请求推导。
func (s *Server) createExecutionNode(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.nodeManagement == nil || s.csrf == nil || s.roles == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置执行节点创建依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if identity.HasRole(r.Context(), s.roles, principal, identity.RoleNodeAdmin) != nil {
		writeError(w, http.StatusForbidden, "ROLE_REQUIRED", "当前身份不具备执行节点管理能力", false)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	request, ok := decodeExecutionNodeWriteRequest(w, r)
	if !ok {
		return
	}
	if fieldErrors := validateExecutionNodeWrite(request); len(fieldErrors) > 0 {
		writeErrorWithFields(w, http.StatusUnprocessableEntity, "EXECUTION_NODE_FIELDS_INVALID", "执行节点字段不符合要求", false, fieldErrors)
		return
	}
	nodeID := newOpaqueID()
	if nodeID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	result, err := s.nodeManagement.CreateExecutionNode(r.Context(), store.ExecutionNodeCreate{
		NodeID: nodeID, CreatorSubjectID: principal.ID, DisplayName: request.DisplayName,
		NormalizedName: normalizeName(request.DisplayName), Platform: request.Platform, AllowedRoots: request.AllowedRoots,
		ToolHome: request.ToolHome, JavaPath: request.JavaPath,
		RequestID: requestID(w), IdempotencyKey: idempotencyKey, RequestDigest: executionNodeCreateDigest(request), CreatedAt: now,
	})
	if errors.Is(err, store.ErrExecutionNodeNameUnavailable) {
		writeExecutionNodeNameUnavailable(w)
		return
	}
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_CREATE_UNAVAILABLE", "执行节点暂时无法创建", true)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID(w), "id": result.NodeID, "revision": 1, "managementState": "DISABLED", "replayed": result.Replayed})
}

// issueAgentEnrollment 只向已授权节点管理员显示一次新的关联材料。
// 原始材料不进入 SQLite、审计、页面详情或重放响应；若展示响应丢失，管理员必须显式签发新材料。
func (s *Server) issueAgentEnrollment(w http.ResponseWriter, r *http.Request, principal identity.Principal, nodeID string) {
	if s.agentProtocol == nil || s.authorizer == nil || s.csrf == nil || s.enrollmentTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "AGENT_ENROLLMENT_NOT_CONFIGURED", "当前环境尚未配置 Agent 关联", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, nodeID) != nil {
		notFound(w, r)
		return
	}
	if r.ContentLength > 0 {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	material := newOpaqueSecret()
	if len(material) == 0 {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	defer credential.Zero(material)
	enrollmentID := newOpaqueID()
	if enrollmentID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	digest := sha256.Sum256(material)
	err := s.agentProtocol.IssueAgentEnrollment(r.Context(), store.AgentEnrollmentIssue{
		EnrollmentID: enrollmentID, NodeID: nodeID, ActorID: principal.ID, RequestID: requestID(w),
		TokenDigest: digest[:], ExpiresAt: now.Add(s.enrollmentTTL), CreatedAt: now,
	})
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "AGENT_ENROLLMENT_UNAVAILABLE", "Agent 关联材料暂时无法签发", true)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"requestId": requestID(w), "enrollmentId": enrollmentID, "nodeId": nodeID,
		"enrollmentMaterial": string(material), "expiresAt": now.Add(s.enrollmentTTL).Format(time.RFC3339Nano),
		"displayedOnce": true,
	})
}

// getExecutionNode 先验证对象范围，再读取节点配置，避免无权请求通过详情接口枚举节点。
func (s *Server) getExecutionNode(w http.ResponseWriter, r *http.Request, principal identity.Principal, nodeID string) {
	if s.nodeManagement == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置执行节点管理", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, nodeID) != nil {
		notFound(w, r)
		return
	}
	node, err := s.nodeManagement.GetExecutionNode(r.Context(), nodeID)
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_QUERY_UNAVAILABLE", "执行节点暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": s.newExecutionNodeDetailResponse(node)})
}

// updateExecutionNode 只修改浏览器可管理的声明配置，并要求强版本前置条件。
// 更新后不会生成环境检查成功或允许任务选择的事实。
func (s *Server) updateExecutionNode(w http.ResponseWriter, r *http.Request, principal identity.Principal, nodeID string) {
	if s.nodeManagement == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置执行节点更新依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	expectedRevision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的节点版本号", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, nodeID) != nil {
		notFound(w, r)
		return
	}
	request, ok := decodeExecutionNodeWriteRequest(w, r)
	if !ok {
		return
	}
	if fieldErrors := validateExecutionNodeWrite(request); len(fieldErrors) > 0 {
		writeErrorWithFields(w, http.StatusUnprocessableEntity, "EXECUTION_NODE_FIELDS_INVALID", "执行节点字段不符合要求", false, fieldErrors)
		return
	}
	revision, err := s.nodeManagement.UpdateExecutionNode(r.Context(), store.ExecutionNodeUpdate{
		NodeID: nodeID, ActorSubjectID: principal.ID, ExpectedRevision: expectedRevision,
		DisplayName: request.DisplayName, NormalizedName: normalizeName(request.DisplayName), Platform: request.Platform,
		AllowedRoots: request.AllowedRoots, ToolHome: request.ToolHome, JavaPath: request.JavaPath,
		RequestID: requestID(w), UpdatedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "REVISION_CONFLICT", "执行节点已发生变化，请刷新后重试", false)
		return
	}
	if errors.Is(err, store.ErrExecutionNodeNameUnavailable) {
		writeExecutionNodeNameUnavailable(w)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_UPDATE_UNAVAILABLE", "执行节点暂时无法更新", true)
		return
	}
	node, err := s.nodeManagement.GetExecutionNode(r.Context(), nodeID)
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_QUERY_UNAVAILABLE", "执行节点暂时不可用", true)
		return
	}
	if node.Revision != revision {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_UPDATE_UNAVAILABLE", "执行节点更新结果无法核对", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": s.newExecutionNodeDetailResponse(node)})
}

// requestExecutionNodeEnvironmentCheck 仅由节点管理员请求当前 Agent 执行固定本机运行时核验。
// 控制面不主动连接节点，浏览器也不能指定检查内容、路径、命令或工具配置。
func (s *Server) requestExecutionNodeEnvironmentCheck(w http.ResponseWriter, r *http.Request, principal identity.Principal, nodeID string) {
	if s.nodeEnvironment == nil || s.nodeManagement == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置节点环境检查", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if r.ContentLength > 0 {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	expectedRevision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的节点版本号", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, nodeID) != nil {
		notFound(w, r)
		return
	}
	checkID := newOpaqueID()
	if checkID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	result, err := s.nodeEnvironment.RequestExecutionNodeEnvironmentCheck(r.Context(), store.ExecutionNodeEnvironmentCheckRequest{
		NodeID: nodeID, ActorSubjectID: principal.ID, ExpectedRevision: expectedRevision, CheckID: checkID, RequestID: requestID(w), RequestedAt: now,
	})
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "REVISION_CONFLICT", "执行节点已发生变化，请刷新后重试", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "ENVIRONMENT_CHECK_REQUEST_UNAVAILABLE", "节点环境检查暂时无法请求", true)
		return
	}
	node, err := s.nodeManagement.GetExecutionNode(r.Context(), nodeID)
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if err != nil || node.Revision != result.Revision {
		writeError(w, http.StatusServiceUnavailable, "ENVIRONMENT_CHECK_REQUEST_UNAVAILABLE", "节点环境检查结果暂时无法核对", true)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"requestId": requestID(w), "checkId": result.CheckID, "item": s.newExecutionNodeDetailResponse(node)})
}

// deleteExecutionNode 先完成 CSRF、对象范围和版本校验，再由仓储按历史引用选择删除或归档。
// 归档只在没有运行中任务时撤销当前机器身份；响应不披露引用的类型、数量或 Agent 身份。
func (s *Server) deleteExecutionNode(w http.ResponseWriter, r *http.Request, principal identity.Principal, nodeID string) {
	if s.nodeDeleter == nil || s.authorizer == nil || s.csrf == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置执行节点删除依赖", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	expectedRevision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的节点版本号", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, nodeID) != nil {
		notFound(w, r)
		return
	}
	result, err := s.nodeDeleter.DeleteOrArchiveExecutionNode(r.Context(), store.ExecutionNodeDeletion{
		NodeID: nodeID, ActorSubjectID: principal.ID, ExpectedRevision: expectedRevision,
		RequestID: requestID(w), DeletedAt: time.Now().UTC(),
	})
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "EXECUTION_NODE_REVISION_CONFLICT", "执行节点已发生变化，请刷新后重试", false)
		return
	}
	if errors.Is(err, store.ErrExecutionNodeHasRunningTask) {
		writeError(w, http.StatusConflict, "EXECUTION_NODE_RUNNING_TASK", "节点仍有运行任务，暂不能删除或归档", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_DELETE_UNAVAILABLE", "执行节点删除暂时不可用", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId": requestID(w), "id": nodeID, "outcome": result.Outcome, "revision": result.Revision,
		"agentAccessRevoked": result.AgentAccessRevoked,
	})
}

// enableExecutionNode 在当前 Agent 在线、平台匹配、容量可用且固定环境检查通过后启用节点。
// 它不连接数据库、不启动 OBDUMPER，也不能取代任务提交时的路径和数据源预检查。
func (s *Server) enableExecutionNode(w http.ResponseWriter, r *http.Request, principal identity.Principal, nodeID string) {
	if s.nodeEnvironment == nil || s.nodeManagement == nil || s.authorizer == nil || s.csrf == nil || s.heartbeatTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置节点启用", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if r.ContentLength > 0 {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return
	}
	expectedRevision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的节点版本号", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, nodeID) != nil {
		notFound(w, r)
		return
	}
	node, err := s.nodeManagement.GetExecutionNode(r.Context(), nodeID)
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_QUERY_UNAVAILABLE", "执行节点暂时不可用", true)
		return
	}
	response := s.newExecutionNodeListResponse(node)
	if node.Revision != expectedRevision || response.ManagementState != "DISABLED" || response.AgentAssociationStatus != "ASSOCIATED" ||
		response.HeartbeatStatus != "ONLINE" || response.EnvironmentStatus != "NORMAL" || response.CapacityStatus != "AVAILABLE" {
		writeError(w, http.StatusUnprocessableEntity, "NODE_ENABLE_PRECONDITION_FAILED", "节点尚未满足启用条件，请先完成环境检查并保持 Agent 在线", false)
		return
	}
	now := time.Now().UTC()
	revision, err := s.nodeEnvironment.EnableExecutionNode(r.Context(), store.ExecutionNodeEnable{
		NodeID: nodeID, ActorSubjectID: principal.ID, ExpectedRevision: expectedRevision, RequestID: requestID(w),
		EnabledAt: now, OnlineAfter: now.Add(-s.heartbeatTTL),
	})
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "REVISION_CONFLICT", "执行节点已发生变化，请刷新后重试", false)
		return
	}
	if errors.Is(err, store.ErrExecutionNodeEnableRejected) {
		writeError(w, http.StatusUnprocessableEntity, "NODE_ENABLE_PRECONDITION_FAILED", "节点尚未满足启用条件，请先完成环境检查并保持 Agent 在线", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_ENABLE_UNAVAILABLE", "执行节点暂时无法启用", true)
		return
	}
	node, err = s.nodeManagement.GetExecutionNode(r.Context(), nodeID)
	if errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if err != nil || node.Revision != revision {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_ENABLE_UNAVAILABLE", "执行节点启用结果暂时无法核对", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": s.newExecutionNodeDetailResponse(node)})
}

// decodeExecutionNodeWriteRequest 对节点管理写入施加浏览器 JSON 大小和未知字段限制。
func decodeExecutionNodeWriteRequest(w http.ResponseWriter, r *http.Request) (executionNodeWriteRequest, bool) {
	var request executionNodeWriteRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "REQUEST_INVALID", "请求字段无效", false)
		return executionNodeWriteRequest{}, false
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.ToolHome = strings.TrimSpace(request.ToolHome)
	request.JavaPath = strings.TrimSpace(request.JavaPath)
	for index, root := range request.AllowedRoots {
		request.AllowedRoots[index] = strings.TrimSpace(root)
	}
	return request, true
}

// validateExecutionNodeWrite 将可修正的表单错误绑定到对应字段，同时复用仓储的绝对路径与平台规则。
func validateExecutionNodeWrite(request executionNodeWriteRequest) []fieldErrorResponse {
	fieldErrors := make([]fieldErrorResponse, 0, 4)
	if request.DisplayName == "" || len(request.DisplayName) > 200 {
		fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "displayName", Code: "EXECUTION_NODE_NAME_INVALID", Message: "请输入不超过 200 个字符的节点名称"})
	}
	validPlatform := request.Platform == "WINDOWS_AMD64" || request.Platform == "LINUX_AMD64" || request.Platform == "LINUX_ARM64"
	if !validPlatform {
		fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "platform", Code: "EXECUTION_NODE_PLATFORM_INVALID", Message: "请选择受支持的目标平台"})
	}
	if validPlatform && !store.ValidateExecutionNodeConfiguration(request.Platform, request.AllowedRoots) {
		fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "allowedRoots", Code: "EXECUTION_NODE_ROOTS_INVALID", Message: "请为目标平台填写至少一个不重复的绝对允许根目录"})
	}
	if !validPlatform && len(request.AllowedRoots) == 0 {
		fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "allowedRoots", Code: "EXECUTION_NODE_ROOTS_REQUIRED", Message: "请填写至少一个允许根目录"})
	}
	if validPlatform && !store.ValidateExecutionNodeRuntimeConfiguration(request.Platform, request.AllowedRoots, request.ToolHome, request.JavaPath) {
		if request.ToolHome == "" {
			fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "toolHome", Code: "EXECUTION_NODE_TOOL_HOME_REQUIRED", Message: "请填写 OB Loader/Dumper 安装目录"})
		} else {
			fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "toolHome", Code: "EXECUTION_NODE_TOOL_HOME_INVALID", Message: "工具目录必须是与目标平台匹配的绝对路径"})
		}
		if request.JavaPath == "" {
			fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "javaPath", Code: "EXECUTION_NODE_JAVA_PATH_REQUIRED", Message: "请填写工具专用 Java 8 可执行文件路径"})
		} else {
			fieldErrors = append(fieldErrors, fieldErrorResponse{Field: "javaPath", Code: "EXECUTION_NODE_JAVA_PATH_INVALID", Message: "Java 路径必须是与目标平台匹配的绝对路径"})
		}
	}
	return fieldErrors
}

func executionNodeCreateDigest(request executionNodeWriteRequest) string {
	payload, _ := json.Marshal(struct {
		DisplayName  string   `json:"displayName"`
		Platform     string   `json:"platform"`
		AllowedRoots []string `json:"allowedRoots"`
		ToolHome     string   `json:"toolHome"`
		JavaPath     string   `json:"javaPath"`
	}{DisplayName: request.DisplayName, Platform: request.Platform, AllowedRoots: request.AllowedRoots, ToolHome: request.ToolHome, JavaPath: request.JavaPath})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// listExecutionNodeCandidates 只返回调用者可使用的已启用节点。
// 它可服务草稿节点选择和基础连接测试节点选择，但候选不等于在线、空闲或可执行；创建测试时仍由 Store 重新核验 Agent 与事实绑定。
func (s *Server) listExecutionNodeCandidates(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置执行节点候选", false)
		return
	}
	eligibleFor := r.URL.Query().Get("eligibleFor")
	if eligibleFor != "" && eligibleFor != "OBDUMPER_EXPORT" && eligibleFor != "DATA_SOURCE_CONNECTION_TEST" {
		writeError(w, http.StatusBadRequest, "EXECUTION_NODE_FILTER_INVALID", "执行节点筛选条件无效", false)
		return
	}
	if eligibleFor == "DATA_SOURCE_CONNECTION_TEST" {
		s.listDataSourceConnectionTestNodeCandidates(w, r, principal)
		return
	}
	if s.nodeCandidates == nil {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置执行节点候选", false)
		return
	}
	summaries, err := s.nodeCandidates.ListExecutionNodeSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_QUERY_UNAVAILABLE", "执行节点暂时不可用", true)
		return
	}
	items := make([]executionNodeCandidateResponse, 0, len(summaries))
	for _, summary := range summaries {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeUse, summary.NodeID) != nil {
			continue
		}
		items = append(items, executionNodeCandidateResponse{ID: summary.NodeID, DisplayName: summary.DisplayName, Platform: summary.Platform})
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "items": items})
}

// listDataSourceConnectionTestNodeCandidates 只公开当前可安全尝试领取基础连接测试的节点。
// 它不会把节点候选解释为数据库可达或任务可执行；Store 在创建、领取、槽位解析和完成时仍会重新核验事实与资源占用。
func (s *Server) listDataSourceConnectionTestNodeCandidates(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	if s.nodeManagement == nil || s.heartbeatTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "API_DEPENDENCY_NOT_CONFIGURED", "当前环境尚未配置连接测试节点候选", false)
		return
	}
	nodes, err := s.nodeManagement.ListExecutionNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_NODE_QUERY_UNAVAILABLE", "执行节点暂时不可用", true)
		return
	}
	items := make([]executionNodeCandidateResponse, 0, len(nodes))
	now := time.Now().UTC()
	for _, node := range nodes {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeUse, node.NodeID) != nil || !dataSourceConnectionTestNodeCandidateEligible(node, now, s.heartbeatTTL) {
			continue
		}
		items = append(items, executionNodeCandidateResponse{ID: node.NodeID, DisplayName: node.DisplayName, Platform: node.Platform})
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "items": items})
}

// dataSourceConnectionTestNodeCandidateEligible 仅使用已认证 Agent 的当前安全投影筛选候选。
// 它不读取工具、目录、数据库或任务内容，也不能替代后续租约事务中的并发占用复验。
func dataSourceConnectionTestNodeCandidateEligible(node store.ExecutionNode, now time.Time, heartbeatTTL time.Duration) bool {
	if node.ManagementState != "ENABLED" && node.ManagementState != "DISABLED" {
		return false
	}
	if node.Agent == nil || node.Agent.FactsRevision < 1 || node.Agent.LastHeartbeatAt == nil ||
		!now.Before(node.Agent.LastHeartbeatAt.Add(heartbeatTTL)) || node.Agent.CapacityTotal < 1 ||
		node.Agent.CapacityUsed >= node.Agent.CapacityTotal {
		return false
	}
	return executionNodePlatformMatchesAgentFacts(node.Platform, node.Agent.EnvironmentFacts)
}

// dataSourceResponse 承载数据源列表与详情共用的最小脱敏字段。
// 它不包含任何用户名身份字段、sys 账号、组合用户名或凭据材料。
type dataSourceResponse struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"displayName"`
	Environment        string `json:"environment"`
	ConnectionKind     string `json:"connectionKind"`
	CompatibilityMode  string `json:"compatibilityMode"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	ClusterName        string `json:"clusterName"`
	TenantName         string `json:"tenantName"`
	DefaultDatabase    string `json:"defaultDatabase,omitempty"`
	State              string `json:"state"`
	Revision           int64  `json:"revision"`
	CredentialRevision int64  `json:"credentialRevision"`
	// SysCredentialState 只表达 sys 凭据是否可用（AVAILABLE/UNAVAILABLE），不下发账号或任何秘密。
	SysCredentialState string `json:"sysCredentialState"`
	LastTestStatus     string `json:"lastTestStatus,omitempty"`
	LastTestedAt       string `json:"lastTestedAt,omitempty"`
}

// dataSourceListResponse 只用于已通过读取范围校验的数据源列表。
// Username 是拆分保存的普通业务用户名，不是 username@tenant#cluster 组合身份。
type dataSourceListResponse struct {
	dataSourceResponse
	Username string `json:"username"`
}

// dataSourceDetailResponse 只用于单个数据源详情。
// Username 是否出现由详情处理器的同一对象管理范围校验决定，不能由读取范围推定。
type dataSourceDetailResponse struct {
	dataSourceResponse
	Username string `json:"username,omitempty"`
}

// dataSourceConnectionTestResponse 是浏览器轮询连接测试的节点特定安全投影。
// 它不返回 JDBC 元信息、异常文本、连接身份、秘密或安全摘要原文。
// Sys 字段只表达可选的 sys 凭据验证事实（参考 ODC 的 sys 账号验证），与数据库结果相互独立。
type dataSourceConnectionTestResponse struct {
	ID                      string `json:"id"`
	DataSourceID            string `json:"dataSourceId"`
	NodeID                  string `json:"nodeId"`
	NodeFactsRevision       int64  `json:"nodeFactsRevision"`
	Status                  string `json:"status"`
	Code                    string `json:"code,omitempty"`
	VerificationSource      string `json:"verificationSource"`
	RealConnectionVerified  bool   `json:"realConnectionVerified"`
	SysCredentialConfigured bool   `json:"sysCredentialConfigured"`
	SysVerificationStatus   string `json:"sysVerificationStatus,omitempty"`
	SysResultCode           string `json:"sysResultCode,omitempty"`
	CreatedAt               string `json:"createdAt"`
	CompletedAt             string `json:"completedAt,omitempty"`
}

// executionNodeCandidateResponse 只向草稿页面返回节点定位和路径校验所需字段。
type executionNodeCandidateResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Platform    string `json:"platform"`
}

// executionNodeListResponse 保持列表紧凑，只返回管理定位和失败关闭状态。
// 完整允许根目录仅在单节点详情中返回，避免列表泄露不必要的机器路径。
type executionNodeListResponse struct {
	ID                     string                           `json:"id"`
	DisplayName            string                           `json:"displayName"`
	Platform               string                           `json:"platform"`
	ManagementState        string                           `json:"managementState"`
	AgentAssociationStatus string                           `json:"agentAssociationStatus"`
	HeartbeatStatus        string                           `json:"heartbeatStatus"`
	LastHeartbeatAt        *string                          `json:"lastHeartbeatAt"`
	EnvironmentStatus      string                           `json:"environmentStatus"`
	CapacityStatus         string                           `json:"capacityStatus"`
	AcceptsNewTasks        bool                             `json:"acceptsNewTasks"`
	UnavailableReasons     []string                         `json:"unavailableReasons"`
	AgentFacts             *executionNodeAgentFactsResponse `json:"agentFacts,omitempty"`
	Revision               int64                            `json:"revision"`
	UpdatedAt              string                           `json:"updatedAt"`
}

// executionNodeAgentFactsResponse 只投影节点管理员可读取的当前机器摘要。
// 它不包含凭据、关联材料、主机地址、磁盘路径、工具命令或 Agent 原始报文。
type executionNodeAgentFactsResponse struct {
	OS                 string `json:"os"`
	Arch               string `json:"arch"`
	AgentVersion       string `json:"agentVersion"`
	BootID             string `json:"bootId"`
	ObservedAt         string `json:"observedAt"`
	CapacityTotal      int    `json:"capacityTotal"`
	CapacityUsed       int    `json:"capacityUsed"`
	CPUUsagePercent    *int   `json:"cpuUsagePercent,omitempty"`
	MemoryUsagePercent *int   `json:"memoryUsagePercent,omitempty"`
}

// executionNodeDataRootUsageResponse 只在 Agent 上报配置摘要与当前节点声明一致时显示目录空间。
// 目录名称来自浏览器登记配置，而非 Agent 心跳，避免 Agent 将任意本机路径带回控制面。
type executionNodeDataRootUsageResponse struct {
	Root           string `json:"root"`
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
}

// executionNodeDetailResponse 在节点管理员对象范围内补充注册时声明的工具与数据目录。
// 它仍不包含 Agent 凭据、日志、资源采样或伪造的环境检查结果。
type executionNodeDetailResponse struct {
	executionNodeListResponse
	AllowedRoots   []string                             `json:"allowedRoots"`
	ToolHome       string                               `json:"toolHome"`
	JavaPath       string                               `json:"javaPath"`
	DataRootUsages []executionNodeDataRootUsageResponse `json:"dataRootUsages,omitempty"`
	CreatedAt      string                               `json:"createdAt"`
}

func (s *Server) newExecutionNodeListResponse(node store.ExecutionNode) executionNodeListResponse {
	response := executionNodeListResponse{
		ID: node.NodeID, DisplayName: node.DisplayName, Platform: node.Platform, ManagementState: node.ManagementState,
		AgentAssociationStatus: "PENDING", HeartbeatStatus: "NEVER_CONNECTED", LastHeartbeatAt: nil,
		EnvironmentStatus: "NOT_CHECKED", CapacityStatus: "UNKNOWN", AcceptsNewTasks: false,
		Revision: node.Revision, UpdatedAt: node.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	reasons := make([]string, 0, 4)
	switch node.ManagementState {
	case "DISABLED":
		reasons = append(reasons, "NODE_DISABLED")
	case "MAINTENANCE":
		reasons = append(reasons, "NODE_MAINTENANCE")
	}
	if node.Agent == nil {
		response.UnavailableReasons = append(reasons, "AGENT_ASSOCIATION_REQUIRED", "ENVIRONMENT_CHECK_REQUIRED")
		return response
	}
	response.AgentAssociationStatus = "ASSOCIATED"
	agent := node.Agent
	if agent.LastHeartbeatAt == nil {
		response.UnavailableReasons = append(reasons, "HEARTBEAT_REQUIRED", "ENVIRONMENT_CHECK_REQUIRED", "CAPACITY_UNKNOWN")
		return response
	}
	lastHeartbeatAt := agent.LastHeartbeatAt.UTC().Format(time.RFC3339Nano)
	response.LastHeartbeatAt = &lastHeartbeatAt
	if s.heartbeatTTL > 0 && !time.Now().UTC().After(agent.LastHeartbeatAt.Add(s.heartbeatTTL)) {
		response.HeartbeatStatus = "ONLINE"
	} else {
		response.HeartbeatStatus = "OFFLINE"
		reasons = append(reasons, "HEARTBEAT_OFFLINE")
	}
	if agent.CapacityTotal > 0 {
		if agent.CapacityUsed >= agent.CapacityTotal {
			response.CapacityStatus = "BUSY"
			reasons = append(reasons, "CAPACITY_BUSY")
		} else {
			response.CapacityStatus = "AVAILABLE"
		}
	} else {
		reasons = append(reasons, "CAPACITY_UNKNOWN")
	}
	if !agent.EnvironmentFacts.ObservedAt.IsZero() {
		response.AgentFacts = &executionNodeAgentFactsResponse{
			OS: agent.EnvironmentFacts.OperatingSystem, Arch: agent.EnvironmentFacts.Architecture, AgentVersion: agent.EnvironmentFacts.AgentVersion,
			BootID: agent.BootID, ObservedAt: agent.EnvironmentFacts.ObservedAt.UTC().Format(time.RFC3339Nano),
			CapacityTotal: agent.CapacityTotal, CapacityUsed: agent.CapacityUsed,
			CPUUsagePercent: agent.EnvironmentFacts.CPUUsagePercent, MemoryUsagePercent: agent.EnvironmentFacts.MemoryUsagePercent,
		}
	}
	if !executionNodePlatformMatchesAgentFacts(node.Platform, agent.EnvironmentFacts) {
		response.EnvironmentStatus = "ABNORMAL"
		reasons = append(reasons, "ENVIRONMENT_CHECK_ABNORMAL")
	} else if node.ToolHome != "" && node.JavaPath != "" && agent.EnvironmentFacts.RuntimeConfigurationDigest != store.ExecutionNodeRuntimeConfigurationDigest(node) {
		response.EnvironmentStatus = "ABNORMAL"
		reasons = append(reasons, "RUNTIME_CONFIGURATION_MISMATCH")
	} else {
		switch node.EnvironmentCheck.Status {
		case "PASSED":
			if node.EnvironmentCheck.Code == "TOOL_RUNTIME_READY" && node.EnvironmentCheck.FactsRevision == agent.FactsRevision && node.EnvironmentCheck.FactsRevision > 0 && node.EnvironmentCheck.CompletedAt != nil {
				response.EnvironmentStatus = "NORMAL"
			} else {
				response.EnvironmentStatus = "EXPIRED"
				reasons = append(reasons, "ENVIRONMENT_CHECK_EXPIRED")
			}
		case "FAILED":
			response.EnvironmentStatus = "ABNORMAL"
			// 透传具体失败码（TOOL_RUNTIME_INVALID/TOOL_RUNTIME_UNAVAILABLE），
			// 让浏览器能区分工具目录缺失、运行时不可用等可诊断原因，而不是只看到泛化的"环境检查发现异常"。
			if node.EnvironmentCheck.Code != "" {
				reasons = append(reasons, node.EnvironmentCheck.Code)
			} else {
				reasons = append(reasons, "ENVIRONMENT_CHECK_ABNORMAL")
			}
		case "PENDING":
			reasons = append(reasons, "ENVIRONMENT_CHECK_IN_PROGRESS")
		default:
			reasons = append(reasons, "ENVIRONMENT_CHECK_REQUIRED")
		}
	}
	if response.ManagementState == "ENABLED" && response.AgentAssociationStatus == "ASSOCIATED" && response.HeartbeatStatus == "ONLINE" &&
		response.EnvironmentStatus == "NORMAL" && response.CapacityStatus == "AVAILABLE" && len(reasons) == 0 {
		response.AcceptsNewTasks = true
	}
	response.UnavailableReasons = reasons
	return response
}

func executionNodePlatformMatchesAgentFacts(platform string, facts store.AgentEnvironmentFacts) bool {
	switch platform {
	case "WINDOWS_AMD64":
		return facts.OperatingSystem == "WINDOWS" && facts.Architecture == "AMD64"
	case "LINUX_AMD64":
		return facts.OperatingSystem == "LINUX" && facts.Architecture == "AMD64"
	case "LINUX_ARM64":
		return facts.OperatingSystem == "LINUX" && facts.Architecture == "ARM64"
	default:
		return false
	}
}

func (s *Server) newExecutionNodeDetailResponse(node store.ExecutionNode) executionNodeDetailResponse {
	return executionNodeDetailResponse{
		executionNodeListResponse: s.newExecutionNodeListResponse(node),
		AllowedRoots:              append([]string(nil), node.AllowedRoots...),
		ToolHome:                  node.ToolHome,
		JavaPath:                  node.JavaPath,
		DataRootUsages:            executionNodeDataRootUsages(node),
		CreatedAt:                 node.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func executionNodeDataRootUsages(node store.ExecutionNode) []executionNodeDataRootUsageResponse {
	if node.Agent == nil || node.ToolHome == "" || node.JavaPath == "" || node.Agent.EnvironmentFacts.RuntimeConfigurationDigest != store.ExecutionNodeRuntimeConfigurationDigest(node) {
		return nil
	}
	usages := make(map[string]store.AgentDataRootUsage, len(node.Agent.EnvironmentFacts.DataRootUsages))
	for _, usage := range node.Agent.EnvironmentFacts.DataRootUsages {
		usages[usage.RootDigest] = usage
	}
	result := make([]executionNodeDataRootUsageResponse, 0, len(node.AllowedRoots))
	for _, root := range node.AllowedRoots {
		digest := sha256.Sum256([]byte(root))
		usage, found := usages[hex.EncodeToString(digest[:])]
		if !found {
			return nil
		}
		result = append(result, executionNodeDataRootUsageResponse{Root: root, TotalBytes: usage.TotalBytes, AvailableBytes: usage.AvailableBytes})
	}
	return result
}

func newDataSourceResponse(summary store.DataSourceSummary) dataSourceResponse {
	response := dataSourceResponse{
		ID: summary.DataSourceID, DisplayName: summary.DisplayName, Environment: summary.Environment,
		ConnectionKind: summary.ConnectionKind, CompatibilityMode: summary.CompatibilityMode,
		Host: summary.Host, Port: summary.Port, ClusterName: summary.ClusterName, TenantName: summary.TenantName,
		DefaultDatabase: summary.DefaultDatabase, State: summary.State, Revision: summary.Revision,
		CredentialRevision: summary.CredentialRevision, LastTestStatus: summary.LastTestStatus,
	}
	// sys 凭据状态派生（参考 ODC 数据源高级设置）：已配置修订视为可用，否则不可用；账号本身不下发。
	if summary.SysCredentialRevision > 0 {
		response.SysCredentialState = "AVAILABLE"
	} else {
		response.SysCredentialState = "UNAVAILABLE"
	}
	if summary.LastTestedAt != nil {
		response.LastTestedAt = summary.LastTestedAt.UTC().Format(time.RFC3339Nano)
	}
	return response
}

// newDataSourceListResponse 构造已授权列表的普通业务用户名投影。
// 它绝不拼接租户或集群后缀，也不复制 sys 账号或任何凭据材料。
func newDataSourceListResponse(summary store.DataSourceSummary) dataSourceListResponse {
	return dataSourceListResponse{
		dataSourceResponse: newDataSourceResponse(summary),
		Username:           summary.Username,
	}
}

// newDataSourceDetailResponse 在已完成对象范围校验后构造详情投影。
// 该函数只复制拆分保存的业务用户名，绝不组装或返回 sys/租户/集群组合身份。
func newDataSourceDetailResponse(summary store.DataSourceSummary, includeUsername bool) dataSourceDetailResponse {
	response := dataSourceDetailResponse{dataSourceResponse: newDataSourceResponse(summary)}
	if includeUsername {
		response.Username = summary.Username
	}
	return response
}

// newDataSourceConnectionTestResponse 固定限制 G2 合成结果的含义。
// 只有已受控配置的 Agent JDBC 终态才会标为真实连接事实，且仍只代表返回的单个节点。
func newDataSourceConnectionTestResponse(run store.DataSourceConnectionTestRun) dataSourceConnectionTestResponse {
	response := dataSourceConnectionTestResponse{
		ID: run.ConnectionTestID, DataSourceID: run.DataSourceID, NodeID: run.NodeID,
		NodeFactsRevision: run.NodeFactsRevision, Status: run.Status, Code: run.ResultCode,
		VerificationSource: run.VerificationSource, CreatedAt: run.CreatedAt.UTC().Format(time.RFC3339Nano),
		RealConnectionVerified:  run.VerificationSource == "AGENT_JDBC" && run.Status == "SUCCEEDED",
		SysCredentialConfigured: run.SysCredentialRevision > 0,
		SysVerificationStatus:   run.SysVerificationStatus, SysResultCode: run.SysResultCode,
	}
	if !run.CompletedAt.IsZero() {
		response.CompletedAt = run.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return response
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

// withRequestID 在认证、解析或写入前生成唯一请求标识。
// 系统熵源不可用时必须停止请求，不能以固定值、时间戳或计数器继续写入审计事实。
func withRequestID(generate func() (string, error), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value, err := generate()
		if err != nil {
			writeRequestIDUnavailable(w)
			return
		}
		next.ServeHTTP(&requestIDResponseWriter{ResponseWriter: w, value: value}, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type fieldErrorResponse struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// writeErrorWithFields 统一保持错误信封结构，并只返回可安全关联到表单字段的校验信息。
func writeErrorWithFields(w http.ResponseWriter, status int, code, message string, retryable bool, fieldErrors []fieldErrorResponse) {
	if fieldErrors == nil {
		fieldErrors = []fieldErrorResponse{}
	}
	writeJSON(w, status, map[string]any{
		"requestId":   requestID(w),
		"code":        code,
		"message":     message,
		"retryable":   retryable,
		"fieldErrors": fieldErrors,
		"safeDetails": map[string]any{},
	})
}

func writeError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	writeErrorWithFields(w, status, code, message, retryable, nil)
}

// writeRequestIDUnavailable 是请求尚未进入业务边界时的失败关闭响应。
// 此时没有可安全复用的 UUID，因此明确省略 requestId，调用方只能重试整个请求。
func writeRequestIDUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"code":        "REQUEST_ID_UNAVAILABLE",
		"message":     "服务暂时不可用",
		"retryable":   true,
		"fieldErrors": []fieldErrorResponse{},
		"safeDetails": map[string]any{},
	})
}

// writeDataSourceNameUnavailable 只指出可修正的输入字段，不透露冲突记录是否存在、已归档或不可见。
func writeDataSourceNameUnavailable(w http.ResponseWriter) {
	writeErrorWithFields(w, http.StatusUnprocessableEntity, "DATA_SOURCE_NAME_UNAVAILABLE", "数据源名称不可用，请更换后重试", false, []fieldErrorResponse{{
		Field: "displayName", Code: "DATA_SOURCE_NAME_UNAVAILABLE", Message: "数据源名称不可用，请更换后重试",
	}})
}

// writeExecutionNodeNameUnavailable 只定位可修正的名称字段，不泄露冲突节点是否存在、归档或不可见。
func writeExecutionNodeNameUnavailable(w http.ResponseWriter) {
	writeErrorWithFields(w, http.StatusUnprocessableEntity, "EXECUTION_NODE_NAME_UNAVAILABLE", "执行节点名称不可用，请更换后重试", false, []fieldErrorResponse{{
		Field: "displayName", Code: "EXECUTION_NODE_NAME_UNAVAILABLE", Message: "执行节点名称不可用，请更换后重试",
	}})
}

func requestID(w http.ResponseWriter) string {
	if carrier, ok := w.(interface{ RequestID() string }); ok {
		return carrier.RequestID()
	}
	return ""
}
