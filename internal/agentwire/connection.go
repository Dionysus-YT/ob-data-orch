package agentwire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/credential"
)

// DataSourceConnectionTestClaimNext 是 Agent 原子领取下一条基础连接测试的最小输入。
// Agent 不可指定测试、租约、地址、凭据、SQL、路径或命令，避免本地输入扩大为远程执行能力。
type DataSourceConnectionTestClaimNext struct {
	BootID string
	SentAt time.Time
}

// DataSourceConnectionTestBinding 是控制面签发的基础连接测试不可变安全绑定。
// 它只包含校验租约所需的非敏感摘要与版本，不能表达 JDBC URL、用户名、密码或自由文本结果。
// SysCredentialID/SysCredentialRevision 是可选的 sys 凭据引用（--sys-user/--sys-password），
// 大于 0 表示本次测试需要额外验证 sys 租户连接。
type DataSourceConnectionTestBinding struct {
	ConnectionTestID         string
	DataSourceID             string
	ConnectionConfigDigest   string
	CredentialRevision       int64
	NodeID                   string
	NodeFactsRevision        int64
	SysCredentialID          string
	SysCredentialRevision    int64
	OperationKind            string
	CatalogDatabase          string
	CatalogCompatibilityMode string
	CatalogObjectType        string
	CatalogKeyword           string
}

// DataSourceConnectionTestVerificationSource 区分 G2 合成闭环与节点本地 JDBC 探针。
// 合成结果不得被调用方解释为数据源已连通或可以启用。
type DataSourceConnectionTestVerificationSource string

const (
	// DataSourceConnectionTestG2Synthetic 表示不解析秘密、不启动 Java 的 G2 合成协议结果。
	DataSourceConnectionTestG2Synthetic DataSourceConnectionTestVerificationSource = "G2_SYNTHETIC"
	// DataSourceConnectionTestAgentJDBC 表示固定 JDBC 探针产生的节点本地连接结果。
	DataSourceConnectionTestAgentJDBC DataSourceConnectionTestVerificationSource = "AGENT_JDBC"
)

// DataSourceConnectionTestSecretSlot 是控制面允许 Agent 解析的基础连接测试秘密槽位类型。
// SYS_CONNECTION 只在数据源配置了可选 sys 凭据（--sys-user/--sys-password）且绑定冻结了 sys 引用时可用。
const (
	DataSourceConnectionTestDatabaseSlot DataSourceConnectionTestSecretSlot = "DATABASE_CONNECTION"
	DataSourceConnectionTestSysSlot      DataSourceConnectionTestSecretSlot = "SYS_CONNECTION"
)

// DataSourceConnectionTestSecretSlot 标识基础连接测试可解析的短时秘密槽位。
type DataSourceConnectionTestSecretSlot string

// DataSourceConnectionTestGrant 是控制面签发给当前已认证 Agent 的短租约。
// BindingDigest、租约 epoch 和 VerificationSource 必须在确认、槽位解析与完成时保持一致。
type DataSourceConnectionTestGrant struct {
	ConnectionTestID   string
	LeaseID            string
	LeaseEpoch         int64
	ExpiresAt          time.Time
	Binding            DataSourceConnectionTestBinding
	BindingDigest      string
	VerificationSource DataSourceConnectionTestVerificationSource
}

// DataSourceConnectionTestLeaseAcknowledgement 是 Agent 对已领取基础连接测试租约的确认输入。
// 此操作不解析秘密、不建立网络连接，也不启动 Java。
type DataSourceConnectionTestLeaseAcknowledgement struct {
	BootID           string
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	BindingDigest    string
	SentAt           time.Time
}

// DataSourceConnectionTestSecretSlotRequest 是 Agent 在已确认的有效租约中请求唯一数据库连接槽位的输入。
// 请求体不携带凭据引用或秘密原文，控制面必须按冻结绑定重新校验授权后才可短时返回槽位。
type DataSourceConnectionTestSecretSlotRequest struct {
	BootID           string
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	BindingDigest    string
	SentAt           time.Time
}

// DataSourceConnectionTestDatabaseConnectionSlot 是 Agent 内存中的短时数据库连接输入。
// Username 必须已经是控制面按已确认规则冻结的完整 JDBC 身份，Agent 不组合用户、租户或集群；Username 和 Password 使用完后必须调用 Destroy，绝不能进入状态文件、日志、命令行或环境变量。
type DataSourceConnectionTestDatabaseConnectionSlot struct {
	Host     string
	Port     int
	Username []byte
	Password []byte
}

// Destroy 尽力清零基础连接测试短时槽位中的秘密字节。
func (s *DataSourceConnectionTestDatabaseConnectionSlot) Destroy() {
	if s == nil {
		return
	}
	credential.Zero(s.Username)
	credential.Zero(s.Password)
	s.Username = nil
	s.Password = nil
}

// DataSourceConnectionTestStatus 是控制面允许 Agent 回写的基础连接测试终态。
// 它不接受排队、运行中或任意调用方声明的成功状态。
type DataSourceConnectionTestStatus string

const (
	// DataSourceConnectionTestSucceeded 表示固定 JDBC 连接成功或 G2 合成闭环成功。
	DataSourceConnectionTestSucceeded DataSourceConnectionTestStatus = "SUCCEEDED"
	// DataSourceConnectionTestFailed 表示固定 JDBC 已确认无法建立连接。
	DataSourceConnectionTestFailed DataSourceConnectionTestStatus = "FAILED"
	// DataSourceConnectionTestUnknown 表示固定 JDBC 连接事实不可用，不能被解释为失败或成功。
	DataSourceConnectionTestUnknown DataSourceConnectionTestStatus = "UNKNOWN"
)

// DataSourceConnectionTestCompletion 是 Agent 回写当前租约的固定无秘密结果。
// EvidenceCode 与 VerificationSource 受枚举约束，禁止上传 JDBC 异常、URL、用户名或其他自由文本。
// SysVerificationStatus/SysEvidenceCode 只表达可选的 sys 凭据验证事实，与数据库结果相互独立。
type DataSourceConnectionTestCompletion struct {
	BootID                string
	ConnectionTestID      string
	LeaseID               string
	LeaseEpoch            int64
	BindingDigest         string
	Status                DataSourceConnectionTestStatus
	EvidenceCode          string
	VerificationSource    DataSourceConnectionTestVerificationSource
	SysVerificationStatus DataSourceConnectionTestSysVerificationStatus
	SysEvidenceCode       string
	CatalogObjects        []string
	CatalogTruncated      bool
	SentAt                time.Time
}

// DataSourceConnectionTestSysVerificationStatus 是可选 sys 凭据验证的受控终态。
// NOT_CONFIGURED 表示数据源未配置 sys 凭据；SUCCEEDED 表示 sys 租户认证成功；
// FAILED 表示已确认无法连接；UNKNOWN 表示事实不可用，不能被解释为成功或失败。
type DataSourceConnectionTestSysVerificationStatus string

const (
	DataSourceConnectionTestSysNotConfigured DataSourceConnectionTestSysVerificationStatus = "NOT_CONFIGURED"
	DataSourceConnectionTestSysSucceeded     DataSourceConnectionTestSysVerificationStatus = "SUCCEEDED"
	DataSourceConnectionTestSysFailed        DataSourceConnectionTestSysVerificationStatus = "FAILED"
	DataSourceConnectionTestSysUnknown       DataSourceConnectionTestSysVerificationStatus = "UNKNOWN"
)

type dataSourceConnectionTestEnvelope struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
}

type dataSourceConnectionTestClaimNextRequest struct {
	dataSourceConnectionTestEnvelope
	Payload dataSourceConnectionTestClaimNextPayload `json:"payload"`
}

type dataSourceConnectionTestClaimNextPayload struct {
	Capability string `json:"capability"`
}

type dataSourceConnectionTestLeaseAcknowledgementRequest struct {
	dataSourceConnectionTestEnvelope
	Payload dataSourceConnectionTestLeasePayload `json:"payload"`
}

type dataSourceConnectionTestLeasePayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
}

type dataSourceConnectionTestSecretSlotRequest struct {
	dataSourceConnectionTestEnvelope
	Payload dataSourceConnectionTestSecretSlotPayload `json:"payload"`
}

type dataSourceConnectionTestSecretSlotPayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
	Slot          string `json:"slot"`
}

type dataSourceConnectionTestCompletionRequest struct {
	dataSourceConnectionTestEnvelope
	Payload dataSourceConnectionTestCompletionPayload `json:"payload"`
}

type dataSourceConnectionTestCompletionPayload struct {
	LeaseID               string                                        `json:"leaseId"`
	LeaseEpoch            int64                                         `json:"leaseEpoch"`
	BindingDigest         string                                        `json:"bindingDigest"`
	Status                DataSourceConnectionTestStatus                `json:"status"`
	EvidenceCode          string                                        `json:"evidenceCode"`
	SysVerificationStatus DataSourceConnectionTestSysVerificationStatus `json:"sysVerificationStatus"`
	SysEvidenceCode       string                                        `json:"sysEvidenceCode"`
	CatalogObjects        []string                                      `json:"catalogObjects,omitempty"`
	CatalogTruncated      bool                                          `json:"catalogTruncated,omitempty"`
}

type dataSourceConnectionTestBindingPayload struct {
	ConnectionTestID         string `json:"connectionTestId"`
	DataSourceID             string `json:"dataSourceId"`
	ConnectionConfigDigest   string `json:"connectionConfigDigest"`
	CredentialRevision       int64  `json:"credentialRevision"`
	NodeID                   string `json:"nodeId"`
	NodeFactsRevision        int64  `json:"nodeFactsRevision"`
	SysCredentialID          string `json:"sysCredentialId,omitempty"`
	SysCredentialRevision    int64  `json:"sysCredentialRevision,omitempty"`
	OperationKind            string `json:"operationKind,omitempty"`
	CatalogDatabase          string `json:"catalogDatabase,omitempty"`
	CatalogCompatibilityMode string `json:"catalogCompatibilityMode,omitempty"`
	CatalogObjectType        string `json:"catalogObjectType,omitempty"`
	CatalogKeyword           string `json:"catalogKeyword,omitempty"`
}

type dataSourceConnectionTestClaimResponsePayload struct {
	ConnectionTestID     string                                     `json:"connectionTestId"`
	LeaseID              string                                     `json:"leaseId"`
	LeaseEpoch           int64                                      `json:"leaseEpoch"`
	ExpiresAt            string                                     `json:"expiresAt"`
	Binding              dataSourceConnectionTestBindingPayload     `json:"binding"`
	BindingDigest        string                                     `json:"bindingDigest"`
	VerificationSource   DataSourceConnectionTestVerificationSource `json:"verificationSource"`
	RealExecutionEnabled *bool                                      `json:"realExecutionEnabled"`
}

type dataSourceConnectionTestLeaseAcknowledgementResponsePayload struct {
	RealExecutionEnabled *bool `json:"realExecutionEnabled"`
}

type dataSourceConnectionTestSecretSlotResponsePayload struct {
	AgentRequestID   string `json:"agentRequestId"`
	ConnectionTestID string `json:"connectionTestId"`
	LeaseID          string `json:"leaseId"`
	LeaseEpoch       int64  `json:"leaseEpoch"`
	BindingDigest    string `json:"bindingDigest"`
	Slot             string `json:"slot"`
	Connection       struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username []byte `json:"username"`
		Password []byte `json:"password"`
	} `json:"connection"`
	RealExecutionEnabled *bool `json:"realExecutionEnabled"`
}

type dataSourceConnectionTestCompletionResponsePayload struct {
	Status                DataSourceConnectionTestStatus                `json:"status"`
	EvidenceCode          string                                        `json:"evidenceCode"`
	VerificationSource    DataSourceConnectionTestVerificationSource    `json:"verificationSource"`
	SysVerificationStatus DataSourceConnectionTestSysVerificationStatus `json:"sysVerificationStatus"`
	SysEvidenceCode       string                                        `json:"sysEvidenceCode"`
	RealExecutionEnabled  *bool                                         `json:"realExecutionEnabled"`
}

// ClaimNextDataSourceConnectionTest 以已关联的受保护机器身份原子领取下一条基础连接测试。
// 无工作时 found 为 false；任何身份、TLS、响应或绑定异常均失败关闭且不会持久化租约。
func (s *StateStore) ClaimNextDataSourceConnectionTest(ctx context.Context, input DataSourceConnectionTestClaimNext) (DataSourceConnectionTestGrant, bool, error) {
	state, found, err := s.loadState()
	if err != nil {
		return DataSourceConnectionTestGrant{}, false, err
	}
	if !found {
		return DataSourceConnectionTestGrant{}, false, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return DataSourceConnectionTestGrant{}, false, ErrEnrollmentPending
	}
	if !validDataSourceConnectionTestClaimNext(input) {
		return DataSourceConnectionTestGrant{}, false, ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return DataSourceConnectionTestGrant{}, false, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return DataSourceConnectionTestGrant{}, false, ErrIdentityUnavailable
	}
	return client.claimNextDataSourceConnectionTest(ctx, &state, requestID, input)
}

// AcknowledgeDataSourceConnectionTestLease 确认控制面签发的基础连接测试租约。
// 确认失败时调用方不得解析槽位或发起 JDBC 探针。
func (s *StateStore) AcknowledgeDataSourceConnectionTestLease(ctx context.Context, input DataSourceConnectionTestLeaseAcknowledgement) error {
	state, found, err := s.loadState()
	if err != nil {
		return err
	}
	if !found {
		return ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return ErrEnrollmentPending
	}
	if !validDataSourceConnectionTestLeaseAcknowledgement(input) {
		return ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return ErrIdentityUnavailable
	}
	return client.acknowledgeDataSourceConnectionTestLease(ctx, &state, requestID, input)
}

// ResolveDataSourceConnectionTestDatabaseConnection 在已确认的当前租约中解析唯一数据库连接槽位。
// 响应中的用户名和密码只留在调用方内存；StateStore 不缓存、不持久化也不记录它们。
func (s *StateStore) ResolveDataSourceConnectionTestDatabaseConnection(ctx context.Context, input DataSourceConnectionTestSecretSlotRequest) (DataSourceConnectionTestDatabaseConnectionSlot, error) {
	state, found, err := s.loadState()
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, err
	}
	if !found {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrEnrollmentPending
	}
	if !validDataSourceConnectionTestSecretSlotRequest(input) {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	return client.resolveDataSourceConnectionTestDatabaseConnection(ctx, &state, requestID, input)
}

// ResolveDataSourceConnectionTestSysConnection 在已确认的当前租约中解析可选的 sys 凭据槽位。
// 响应中的用户名和密码只留在调用方内存；StateStore 不缓存、不持久化也不记录它们。
// 数据源未配置 sys 凭据时返回 ErrProtocolRejected（Agent 不得在绑定未冻结 sys 引用时请求）。
func (s *StateStore) ResolveDataSourceConnectionTestSysConnection(ctx context.Context, input DataSourceConnectionTestSecretSlotRequest) (DataSourceConnectionTestDatabaseConnectionSlot, error) {
	state, found, err := s.loadState()
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, err
	}
	if !found {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrEnrollmentPending
	}
	if !validDataSourceConnectionTestSecretSlotRequest(input) {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	return client.resolveDataSourceConnectionTestSecretSlot(ctx, &state, requestID, input, DataSourceConnectionTestSysSlot)
}

// CompleteDataSourceConnectionTest 回写当前基础连接测试租约的固定无秘密终态。
// 控制面响应必须精确回显状态、证据码和来源，防止客户端把不同节点或不同模式的结果混写。
func (s *StateStore) CompleteDataSourceConnectionTest(ctx context.Context, input DataSourceConnectionTestCompletion) (DataSourceConnectionTestStatus, error) {
	state, found, err := s.loadState()
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return "", ErrEnrollmentPending
	}
	if !validDataSourceConnectionTestCompletion(input) {
		return "", ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return "", ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return "", ErrIdentityUnavailable
	}
	return client.completeDataSourceConnectionTest(ctx, &state, requestID, input)
}

func (c *httpsClient) claimNextDataSourceConnectionTest(ctx context.Context, state *identityState, requestID string, input DataSourceConnectionTestClaimNext) (DataSourceConnectionTestGrant, bool, error) {
	request := dataSourceConnectionTestClaimNextRequest{
		dataSourceConnectionTestEnvelope: dataSourceConnectionTestEnvelope{
			ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID,
			BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano),
			PayloadType: "DATA_SOURCE_CONNECTION_TEST_CLAIM_NEXT",
		},
		Payload: dataSourceConnectionTestClaimNextPayload{Capability: "DATA_SOURCE_CONNECTION_TEST"},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return DataSourceConnectionTestGrant{}, false, ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	responseBody, status, err := c.postWithRetry(ctx, "/agent/v1/data-source-connection-tests:claim-next", requestBody, state.MachineCredential)
	if err != nil {
		return DataSourceConnectionTestGrant{}, false, err
	}
	defer credential.Zero(responseBody)
	if status == http.StatusNoContent {
		if len(responseBody) != 0 {
			return DataSourceConnectionTestGrant{}, false, ErrProtocolRejected
		}
		return DataSourceConnectionTestGrant{}, false, nil
	}
	if status != http.StatusOK {
		return DataSourceConnectionTestGrant{}, false, dataSourceConnectionTestHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "DATA_SOURCE_CONNECTION_TEST_CLAIMED")
	if err != nil {
		return DataSourceConnectionTestGrant{}, false, ErrProtocolRejected
	}
	var payload dataSourceConnectionTestClaimResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.RealExecutionEnabled == nil || *payload.RealExecutionEnabled {
		return DataSourceConnectionTestGrant{}, false, ErrProtocolRejected
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	if err != nil {
		return DataSourceConnectionTestGrant{}, false, ErrProtocolRejected
	}
	grant := DataSourceConnectionTestGrant{
		ConnectionTestID: payload.ConnectionTestID, LeaseID: payload.LeaseID, LeaseEpoch: payload.LeaseEpoch,
		ExpiresAt: expiresAt, Binding: payload.Binding.toBinding(), BindingDigest: payload.BindingDigest,
		VerificationSource: payload.VerificationSource,
	}
	if !validDataSourceConnectionTestGrant(grant, state.NodeID) {
		return DataSourceConnectionTestGrant{}, false, ErrProtocolRejected
	}
	return grant, true, nil
}

func (c *httpsClient) acknowledgeDataSourceConnectionTestLease(ctx context.Context, state *identityState, requestID string, input DataSourceConnectionTestLeaseAcknowledgement) error {
	request := dataSourceConnectionTestLeaseAcknowledgementRequest{
		dataSourceConnectionTestEnvelope: dataSourceConnectionTestEnvelope{
			ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID,
			BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano),
			PayloadType: "DATA_SOURCE_CONNECTION_TEST_ACKNOWLEDGE_LEASE",
		},
		Payload: dataSourceConnectionTestLeasePayload{LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	endpoint := "/agent/v1/data-source-connection-tests/" + input.ConnectionTestID + ":acknowledge-lease"
	responseBody, status, err := c.postWithRetry(ctx, endpoint, requestBody, state.MachineCredential)
	if err != nil {
		return err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		return dataSourceConnectionTestHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "DATA_SOURCE_CONNECTION_TEST_LEASE_ACKNOWLEDGED")
	if err != nil {
		return ErrProtocolRejected
	}
	var payload dataSourceConnectionTestLeaseAcknowledgementResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.RealExecutionEnabled == nil || *payload.RealExecutionEnabled {
		return ErrProtocolRejected
	}
	return nil
}

func (c *httpsClient) resolveDataSourceConnectionTestDatabaseConnection(ctx context.Context, state *identityState, requestID string, input DataSourceConnectionTestSecretSlotRequest) (DataSourceConnectionTestDatabaseConnectionSlot, error) {
	return c.resolveDataSourceConnectionTestSecretSlot(ctx, state, requestID, input, DataSourceConnectionTestDatabaseSlot)
}

func (c *httpsClient) resolveDataSourceConnectionTestSecretSlot(ctx context.Context, state *identityState, requestID string, input DataSourceConnectionTestSecretSlotRequest, slot DataSourceConnectionTestSecretSlot) (DataSourceConnectionTestDatabaseConnectionSlot, error) {
	request := dataSourceConnectionTestSecretSlotRequest{
		dataSourceConnectionTestEnvelope: dataSourceConnectionTestEnvelope{
			ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID,
			BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano),
			PayloadType: "DATA_SOURCE_CONNECTION_TEST_RESOLVE_SECRET_SLOTS",
		},
		Payload: dataSourceConnectionTestSecretSlotPayload{
			LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest, Slot: string(slot),
		},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	endpoint := "/agent/v1/data-source-connection-tests/" + input.ConnectionTestID + "/secret-slots:resolve"
	responseBody, status, err := c.postWithRetry(ctx, endpoint, requestBody, state.MachineCredential)
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, dataSourceConnectionTestHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "DATA_SOURCE_CONNECTION_TEST_SECRET_SLOTS_RESOLVED")
	if err != nil {
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrProtocolRejected
	}
	var payload dataSourceConnectionTestSecretSlotResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.AgentRequestID != requestID ||
		payload.ConnectionTestID != input.ConnectionTestID || payload.LeaseID != input.LeaseID || payload.LeaseEpoch != input.LeaseEpoch ||
		payload.BindingDigest != input.BindingDigest || payload.Slot != string(slot) || payload.RealExecutionEnabled == nil || *payload.RealExecutionEnabled {
		payload.destroy()
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrProtocolRejected
	}
	connection := DataSourceConnectionTestDatabaseConnectionSlot{
		Host: payload.Connection.Host, Port: payload.Connection.Port, Username: payload.Connection.Username, Password: payload.Connection.Password,
	}
	payload.Connection.Username = nil
	payload.Connection.Password = nil
	if !validDataSourceConnectionTestDatabaseConnectionSlot(connection) {
		connection.Destroy()
		return DataSourceConnectionTestDatabaseConnectionSlot{}, ErrProtocolRejected
	}
	return connection, nil
}

func (c *httpsClient) completeDataSourceConnectionTest(ctx context.Context, state *identityState, requestID string, input DataSourceConnectionTestCompletion) (DataSourceConnectionTestStatus, error) {
	request := dataSourceConnectionTestCompletionRequest{
		dataSourceConnectionTestEnvelope: dataSourceConnectionTestEnvelope{
			ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID,
			BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano),
			PayloadType: "DATA_SOURCE_CONNECTION_TEST_COMPLETE",
		},
		Payload: dataSourceConnectionTestCompletionPayload{
			LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest,
			Status: input.Status, EvidenceCode: input.EvidenceCode,
			SysVerificationStatus: input.SysVerificationStatus, SysEvidenceCode: input.SysEvidenceCode,
			CatalogObjects: input.CatalogObjects, CatalogTruncated: input.CatalogTruncated,
		},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	endpoint := "/agent/v1/data-source-connection-tests/" + input.ConnectionTestID + ":complete"
	responseBody, status, err := c.postWithRetry(ctx, endpoint, requestBody, state.MachineCredential)
	if err != nil {
		return "", err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		return "", dataSourceConnectionTestHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "DATA_SOURCE_CONNECTION_TEST_COMPLETED")
	if err != nil {
		return "", ErrProtocolRejected
	}
	var payload dataSourceConnectionTestCompletionResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.RealExecutionEnabled == nil || *payload.RealExecutionEnabled ||
		payload.Status != input.Status || payload.EvidenceCode != input.EvidenceCode || payload.VerificationSource != input.VerificationSource ||
		payload.SysVerificationStatus != input.SysVerificationStatus || payload.SysEvidenceCode != input.SysEvidenceCode ||
		!validDataSourceConnectionTestOutcome(payload.VerificationSource, payload.Status, payload.EvidenceCode) {
		return "", ErrProtocolRejected
	}
	return payload.Status, nil
}

func (p dataSourceConnectionTestBindingPayload) toBinding() DataSourceConnectionTestBinding {
	return DataSourceConnectionTestBinding{
		ConnectionTestID: p.ConnectionTestID, DataSourceID: p.DataSourceID, ConnectionConfigDigest: p.ConnectionConfigDigest,
		CredentialRevision: p.CredentialRevision, NodeID: p.NodeID, NodeFactsRevision: p.NodeFactsRevision,
		SysCredentialID: p.SysCredentialID, SysCredentialRevision: p.SysCredentialRevision,
		OperationKind: p.OperationKind, CatalogDatabase: p.CatalogDatabase, CatalogCompatibilityMode: p.CatalogCompatibilityMode,
		CatalogObjectType: p.CatalogObjectType, CatalogKeyword: p.CatalogKeyword,
	}
}

func (p *dataSourceConnectionTestSecretSlotResponsePayload) destroy() {
	if p == nil {
		return
	}
	credential.Zero(p.Connection.Username)
	credential.Zero(p.Connection.Password)
	p.Connection.Username = nil
	p.Connection.Password = nil
}

func validDataSourceConnectionTestClaimNext(input DataSourceConnectionTestClaimNext) bool {
	return validOpaqueValue(input.BootID, 256) && !input.SentAt.IsZero()
}

func validDataSourceConnectionTestLeaseAcknowledgement(input DataSourceConnectionTestLeaseAcknowledgement) bool {
	return validOpaqueValue(input.BootID, 256) && validDataSourceConnectionTestPathID(input.ConnectionTestID) &&
		validOpaqueValue(input.LeaseID, 256) && input.LeaseEpoch > 0 && validDataSourceConnectionTestDigest(input.BindingDigest) && !input.SentAt.IsZero()
}

func validDataSourceConnectionTestSecretSlotRequest(input DataSourceConnectionTestSecretSlotRequest) bool {
	return validOpaqueValue(input.BootID, 256) && validDataSourceConnectionTestPathID(input.ConnectionTestID) &&
		validOpaqueValue(input.LeaseID, 256) && input.LeaseEpoch > 0 && validDataSourceConnectionTestDigest(input.BindingDigest) && !input.SentAt.IsZero()
}

func validDataSourceConnectionTestCompletion(input DataSourceConnectionTestCompletion) bool {
	return validOpaqueValue(input.BootID, 256) && validDataSourceConnectionTestPathID(input.ConnectionTestID) &&
		validOpaqueValue(input.LeaseID, 256) && input.LeaseEpoch > 0 && validDataSourceConnectionTestDigest(input.BindingDigest) &&
		!input.SentAt.IsZero() && validDataSourceConnectionTestOutcome(input.VerificationSource, input.Status, input.EvidenceCode) &&
		validDataSourceConnectionTestSysOutcomeWithSource(input.VerificationSource, input.SysVerificationStatus, input.SysEvidenceCode)
}

// validDataSourceConnectionTestSysOutcomeWithSource 在来源维度上校验 sys 验证结果（G2 必须 NOT_CONFIGURED）。
func validDataSourceConnectionTestSysOutcomeWithSource(source DataSourceConnectionTestVerificationSource, status DataSourceConnectionTestSysVerificationStatus, evidenceCode string) bool {
	if source == DataSourceConnectionTestG2Synthetic {
		return status == DataSourceConnectionTestSysNotConfigured && evidenceCode == ""
	}
	if source != DataSourceConnectionTestAgentJDBC {
		return false
	}
	return ValidDataSourceConnectionTestSysOutcome(status, evidenceCode)
}

// ValidDataSourceConnectionTestSysOutcome 校验可选的 sys 凭据验证结果只使用受控枚举与证据码。
func ValidDataSourceConnectionTestSysOutcome(status DataSourceConnectionTestSysVerificationStatus, evidenceCode string) bool {
	return (status == DataSourceConnectionTestSysNotConfigured && evidenceCode == "") ||
		(status == DataSourceConnectionTestSysSucceeded && evidenceCode == "SYS_CONNECTED") ||
		(status == DataSourceConnectionTestSysFailed && evidenceCode == "SYS_HOST_UNRESOLVABLE") ||
		(status == DataSourceConnectionTestSysFailed && evidenceCode == "SYS_TCP_REFUSED") ||
		(status == DataSourceConnectionTestSysFailed && evidenceCode == "SYS_TCP_TIMEOUT") ||
		(status == DataSourceConnectionTestSysFailed && evidenceCode == "SYS_TCP_UNREACHABLE") ||
		(status == DataSourceConnectionTestSysFailed && evidenceCode == "SYS_CONNECTION_FAILED") ||
		(status == DataSourceConnectionTestSysUnknown && evidenceCode == "SYS_CONNECTION_UNAVAILABLE")
}

func validDataSourceConnectionTestGrant(grant DataSourceConnectionTestGrant, nodeID string) bool {
	return validDataSourceConnectionTestPathID(grant.ConnectionTestID) && validOpaqueValue(grant.LeaseID, 256) && grant.LeaseEpoch > 0 &&
		!grant.ExpiresAt.IsZero() && validDataSourceConnectionTestDigest(grant.BindingDigest) && validDataSourceConnectionTestBinding(grant.Binding) &&
		validDataSourceConnectionTestVerificationSource(grant.VerificationSource) && grant.Binding.ConnectionTestID == grant.ConnectionTestID && grant.Binding.NodeID == nodeID
}

func validDataSourceConnectionTestBinding(binding DataSourceConnectionTestBinding) bool {
	return validDataSourceConnectionTestPathID(binding.ConnectionTestID) && validOpaqueValue(binding.DataSourceID, 256) &&
		validDataSourceConnectionTestDigest(binding.ConnectionConfigDigest) && binding.CredentialRevision > 0 &&
		validOpaqueValue(binding.NodeID, 256) && binding.NodeFactsRevision > 0
}

func validDataSourceConnectionTestVerificationSource(source DataSourceConnectionTestVerificationSource) bool {
	return source == DataSourceConnectionTestG2Synthetic || source == DataSourceConnectionTestAgentJDBC
}

func validDataSourceConnectionTestOutcome(source DataSourceConnectionTestVerificationSource, status DataSourceConnectionTestStatus, evidenceCode string) bool {
	if source == DataSourceConnectionTestG2Synthetic {
		return status == DataSourceConnectionTestSucceeded && evidenceCode == "SYNTHETIC_OK"
	}
	if source != DataSourceConnectionTestAgentJDBC {
		return false
	}
	return (status == DataSourceConnectionTestSucceeded && evidenceCode == "DATABASE_CONNECTED") ||
		(status == DataSourceConnectionTestFailed && evidenceCode == "DATABASE_HOST_UNRESOLVABLE") ||
		(status == DataSourceConnectionTestFailed && evidenceCode == "DATABASE_TCP_REFUSED") ||
		(status == DataSourceConnectionTestFailed && evidenceCode == "DATABASE_TCP_TIMEOUT") ||
		(status == DataSourceConnectionTestFailed && evidenceCode == "DATABASE_TCP_UNREACHABLE") ||
		(status == DataSourceConnectionTestFailed && evidenceCode == "DATABASE_CONNECTION_FAILED") ||
		(status == DataSourceConnectionTestUnknown && evidenceCode == "DATABASE_CONNECTION_UNAVAILABLE")
}

func validDataSourceConnectionTestDatabaseConnectionSlot(slot DataSourceConnectionTestDatabaseConnectionSlot) bool {
	if slot.Port < 1 || slot.Port > 65535 || len(slot.Host) == 0 || len(slot.Host) > 253 ||
		len(slot.Username) == 0 || len(slot.Username) > 256 || len(slot.Password) == 0 || len(slot.Password) > 4096 {
		return false
	}
	for _, value := range []byte(slot.Host) {
		if !(value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '.' || value == '-') {
			return false
		}
	}
	return !containsForbiddenDataSourceConnectionTestSecretByte(slot.Username) && !containsForbiddenDataSourceConnectionTestSecretByte(slot.Password)
}

func containsForbiddenDataSourceConnectionTestSecretByte(value []byte) bool {
	for _, item := range value {
		if item == '\r' || item == '\n' || item == 0 {
			return true
		}
	}
	return false
}

func validDataSourceConnectionTestPathID(value string) bool {
	return validOpaqueValue(value, 256) && !strings.ContainsAny(value, "/\\?#:")
}

func validDataSourceConnectionTestDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func dataSourceConnectionTestHTTPError(status int) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ErrAgentAuthenticationDenied
	}
	return ErrProtocolRejected
}
