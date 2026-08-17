package agentwire

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/logstream"
)

// ExecutionClaimNext 是 Agent 请求当前节点下一条固定 OBDUMPER_EXPORT 的最小输入。
// 请求没有 taskId、参数、路径或凭据字段，避免本地调用方把协议扩展成任意执行。
type ExecutionClaimNext struct {
	BootID string
	SentAt time.Time
}

// ExecutionGrant 是服务端签发的不可变导出信封。
// Argv 只含控制面已冻结的非密码令牌，密码另由当前 execution 租约解析。
type ExecutionGrant struct {
	TaskID         string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	ExpiresAt      time.Time
	EnvelopeDigest string
	Argv           []string
}

// ExecutionLeaseAcknowledgement 表示 Agent 已校验领取到的固定任务信封。
type ExecutionLeaseAcknowledgement struct {
	BootID         string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	SentAt         time.Time
}

// ExecutionLeaseRenewal 只请求延长当前 execution 的既有租约。
// Agent 不能借此切换任务、节点、参数或秘密槽位。
type ExecutionLeaseRenewal struct {
	BootID         string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	SentAt         time.Time
}

// ExecutionSecretSlotRequest 只在已确认的 execution 租约内请求数据库连接槽位。
type ExecutionSecretSlotRequest struct {
	BootID         string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	SentAt         time.Time
}

// ExecutionEvent 是 Agent 采集的固定进程或结果事实。
// Evidence 只能由 Worker 使用固定字段构造，不能承载命令、路径、SQL、密码或任意日志文本。
type ExecutionEvent struct {
	BootID         string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	EventID        string
	EventSeq       int64
	EventType      string
	Evidence       map[string]any
	SentAt         time.Time
}

// ExecutionLogBatch 是已经完成 Agent 第一层脱敏的单个日志批次。
// 该结构不接受原始字节、任意文件路径或未脱敏日志来源。
type ExecutionLogBatch struct {
	BootID         string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	Batch          logstream.Batch
	SentAt         time.Time
}

// ExecutionLogGap 是 Agent 在无法持久化已脱敏正文时上报的连续来源缺口。
// 它只含来源位置和固定原因码，不能携带被丢弃的正文、路径、命令或原始错误。
type ExecutionLogGap struct {
	BootID         string
	ExecutionID    string
	LeaseID        string
	LeaseEpoch     int64
	EnvelopeDigest string
	Gap            logstream.GapNotice
	SentAt         time.Time
}

type executionClaimNextRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
	Payload         struct {
		Capability string `json:"capability"`
	} `json:"payload"`
}

type executionLeaseRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
	Payload         struct {
		LeaseID        string `json:"leaseId"`
		LeaseEpoch     int64  `json:"leaseEpoch"`
		EnvelopeDigest string `json:"envelopeDigest"`
	} `json:"payload"`
}

type executionEventRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
	Payload         struct {
		LeaseID        string         `json:"leaseId"`
		LeaseEpoch     int64          `json:"leaseEpoch"`
		EnvelopeDigest string         `json:"envelopeDigest"`
		EventID        string         `json:"eventId"`
		EventSeq       int64          `json:"eventSeq"`
		EventType      string         `json:"eventType"`
		Evidence       map[string]any `json:"evidence"`
	} `json:"payload"`
}

type executionLogRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
	Payload         struct {
		LeaseID        string          `json:"leaseId"`
		LeaseEpoch     int64           `json:"leaseEpoch"`
		EnvelopeDigest string          `json:"envelopeDigest"`
		Batch          logstream.Batch `json:"batch"`
	} `json:"payload"`
}

type executionLogGapRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
	Payload         struct {
		LeaseID        string              `json:"leaseId"`
		LeaseEpoch     int64               `json:"leaseEpoch"`
		EnvelopeDigest string              `json:"envelopeDigest"`
		Gap            logstream.GapNotice `json:"gap"`
	} `json:"payload"`
}

// executionLogAcceptedPayload 是控制面对单个日志批次的确认回执。
// Agent 只在回执完整回显已发送批次的位置和摘要时释放本地副本，避免延迟、重复或错位响应误确认其他批次。
type executionLogAcceptedPayload struct {
	Decision             logstream.BatchDecision `json:"decision"`
	ExpectedSequence     int64                   `json:"expectedSequence"`
	StreamID             string                  `json:"streamId"`
	SourceEpoch          int64                   `json:"sourceEpoch"`
	FirstSequence        int64                   `json:"firstSequence"`
	LastSequence         int64                   `json:"lastSequence"`
	BatchDigest          string                  `json:"batchDigest"`
	RealExecutionEnabled bool                    `json:"realExecutionEnabled"`
}

type executionLogGapAcceptedPayload struct {
	Decision             logstream.BatchDecision `json:"decision"`
	ExpectedSequence     int64                   `json:"expectedSequence"`
	StreamID             string                  `json:"streamId"`
	SourceEpoch          int64                   `json:"sourceEpoch"`
	FirstSequence        int64                   `json:"firstSequence"`
	LastSequence         int64                   `json:"lastSequence"`
	GapDigest            string                  `json:"gapDigest"`
	RealExecutionEnabled bool                    `json:"realExecutionEnabled"`
}

type executionClaimPayload struct {
	TaskID               string   `json:"taskId"`
	ExecutionID          string   `json:"executionId"`
	LeaseID              string   `json:"leaseId"`
	LeaseEpoch           int64    `json:"leaseEpoch"`
	ExpiresAt            string   `json:"expiresAt"`
	EnvelopeDigest       string   `json:"envelopeDigest"`
	Argv                 []string `json:"argv"`
	ToolVersion          string   `json:"toolVersion"`
	MetadataVersion      string   `json:"metadataVersion"`
	CapabilityVersion    string   `json:"capabilityVersion"`
	RealExecutionEnabled *bool    `json:"realExecutionEnabled"`
}

type executionSecretPayload struct {
	AgentRequestID string `json:"agentRequestId"`
	ExecutionID    string `json:"executionId"`
	LeaseID        string `json:"leaseId"`
	LeaseEpoch     int64  `json:"leaseEpoch"`
	EnvelopeDigest string `json:"envelopeDigest"`
	Connection     struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username []byte `json:"username"`
		Password []byte `json:"password"`
	} `json:"connection"`
	// StorageCredential 是 EX-I6 对象存储任务的短时凭据段（本地输出任务缺省）。
	StorageCredential *struct {
		Provider  string `json:"provider"`
		AccessKey []byte `json:"accessKey"`
		SecretKey []byte `json:"secretKey"`
	} `json:"storageCredential"`
	RealExecutionEnabled *bool `json:"realExecutionEnabled"`
}

type executionLeaseRenewedPayload struct {
	ExecutionID          string `json:"executionId"`
	LeaseID              string `json:"leaseId"`
	LeaseEpoch           int64  `json:"leaseEpoch"`
	EnvelopeDigest       string `json:"envelopeDigest"`
	ExpiresAt            string `json:"expiresAt"`
	RealExecutionEnabled *bool  `json:"realExecutionEnabled"`
}

// ClaimNextExecution 仅从已关联状态调用受认证领取端点；无工作返回 found=false。
func (s *StateStore) ClaimNextExecution(ctx context.Context, input ExecutionClaimNext) (ExecutionGrant, bool, error) {
	state, found, err := s.loadState()
	if err != nil || !found || state.pendingEnrollment() || !validExecutionClaimNext(input) {
		if err != nil {
			return ExecutionGrant{}, false, err
		}
		return ExecutionGrant{}, false, ErrIdentityUnavailable
	}
	defer state.destroy()
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ExecutionGrant{}, false, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return ExecutionGrant{}, false, ErrIdentityUnavailable
	}
	return client.claimNextExecution(ctx, &state, requestID, input)
}

// AcknowledgeExecutionLease 只确认已经领取的信封，不触发凭据解析或进程启动。
func (s *StateStore) AcknowledgeExecutionLease(ctx context.Context, input ExecutionLeaseAcknowledgement) error {
	return s.withExecutionState(ctx, input.BootID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, input.SentAt, "OBDUMPER_EXPORT_ACKNOWLEDGE_LEASE", func(client *httpsClient, state *identityState, requestID string, request executionLeaseRequest) error {
		body, err := json.Marshal(request)
		if err != nil {
			return ErrProtocolRejected
		}
		defer credential.Zero(body)
		response, status, err := client.postWithRetry(ctx, "/agent/v1/executions/"+input.ExecutionID+":acknowledge-lease", body, state.MachineCredential)
		if err != nil {
			return err
		}
		defer credential.Zero(response)
		if status != http.StatusOK {
			return executionHTTPError(status)
		}
		if _, err := decodeResponseEnvelope(response, "EXECUTION_LEASE_ACKNOWLEDGED"); err != nil {
			return ErrProtocolRejected
		}
		return nil
	})
}

// RenewExecutionLease 使用控制面时钟续期；Agent 只接受同一 execution、lease 与摘要的回执。
func (s *StateStore) RenewExecutionLease(ctx context.Context, input ExecutionLeaseRenewal) error {
	return s.withExecutionState(ctx, input.BootID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, input.SentAt, "OBDUMPER_EXPORT_RENEW_LEASE", func(client *httpsClient, state *identityState, requestID string, request executionLeaseRequest) error {
		body, err := json.Marshal(request)
		if err != nil {
			return ErrProtocolRejected
		}
		defer credential.Zero(body)
		response, status, err := client.postWithRetry(ctx, "/agent/v1/executions/"+input.ExecutionID+":renew-lease", body, state.MachineCredential)
		if err != nil {
			return err
		}
		defer credential.Zero(response)
		if status != http.StatusOK {
			return executionHTTPError(status)
		}
		envelope, err := decodeResponseEnvelope(response, "EXECUTION_LEASE_RENEWED")
		if err != nil {
			return ErrProtocolRejected
		}
		var payload executionLeaseRenewedPayload
		if decodeStrictJSON(envelope.Payload, &payload) != nil || payload.RealExecutionEnabled == nil || !*payload.RealExecutionEnabled || payload.ExecutionID != input.ExecutionID || payload.LeaseID != input.LeaseID || payload.LeaseEpoch != input.LeaseEpoch || payload.EnvelopeDigest != input.EnvelopeDigest {
			return ErrProtocolRejected
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
		if err != nil || !expiresAt.After(input.SentAt) {
			return ErrProtocolRejected
		}
		return nil
	})
}

// ResolveExecutionDatabaseConnection 在有效 execution 租约中取得短时数据库连接槽位。
func (s *StateStore) ResolveExecutionDatabaseConnection(ctx context.Context, input ExecutionSecretSlotRequest) (DatabaseConnectionSlot, error) {
	if !validExecutionLeaseInput(input.BootID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, input.SentAt) {
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	state, found, err := s.loadState()
	if err != nil || !found || state.pendingEnrollment() {
		if err != nil {
			return DatabaseConnectionSlot{}, err
		}
		return DatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	defer state.destroy()
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return DatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return DatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	request := executionLeaseRequest{ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID, BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano), PayloadType: "OBDUMPER_EXPORT_RESOLVE_SECRET_SLOTS"}
	request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest = input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest
	body, err := json.Marshal(request)
	if err != nil {
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	defer credential.Zero(body)
	response, status, err := client.postWithRetry(ctx, "/agent/v1/executions/"+input.ExecutionID+"/secret-slots:resolve", body, state.MachineCredential)
	if err != nil {
		return DatabaseConnectionSlot{}, err
	}
	defer credential.Zero(response)
	if status != http.StatusOK {
		return DatabaseConnectionSlot{}, executionHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(response, "EXECUTION_SECRET_SLOTS_RESOLVED")
	if err != nil {
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	var payload executionSecretPayload
	if decodeStrictJSON(envelope.Payload, &payload) != nil || payload.RealExecutionEnabled == nil || !*payload.RealExecutionEnabled || payload.AgentRequestID != requestID || payload.ExecutionID != input.ExecutionID || payload.LeaseID != input.LeaseID || payload.LeaseEpoch != input.LeaseEpoch || payload.EnvelopeDigest != input.EnvelopeDigest {
		payload.destroy()
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	connection := DatabaseConnectionSlot{Host: payload.Connection.Host, Port: payload.Connection.Port, Username: payload.Connection.Username, Password: payload.Connection.Password}
	payload.Connection.Username, payload.Connection.Password = nil, nil
	// EX-I6：对象存储任务附带短时存储凭据段；本地输出任务缺省跳过。
	if payload.StorageCredential != nil {
		connection.StorageCredential = &StorageCredentialSlot{Provider: payload.StorageCredential.Provider, AccessKey: payload.StorageCredential.AccessKey, SecretKey: payload.StorageCredential.SecretKey}
		payload.StorageCredential.AccessKey, payload.StorageCredential.SecretKey = nil, nil
	}
	if !validDatabaseConnectionSlot(connection) {
		connection.Destroy()
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	return connection, nil
}

// AppendExecutionEvent 上报一个连续、固定类型的执行事实。
func (s *StateStore) AppendExecutionEvent(ctx context.Context, input ExecutionEvent) error {
	if !validExecutionEvent(input) {
		return ErrProtocolRejected
	}
	state, found, err := s.loadState()
	if err != nil || !found || state.pendingEnrollment() {
		if err != nil {
			return err
		}
		return ErrIdentityUnavailable
	}
	defer state.destroy()
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return ErrIdentityUnavailable
	}
	request := executionEventRequest{ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID, BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano), PayloadType: "OBDUMPER_EXPORT_APPEND_EVENT"}
	request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest = input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest
	request.Payload.EventID, request.Payload.EventSeq, request.Payload.EventType, request.Payload.Evidence = input.EventID, input.EventSeq, input.EventType, input.Evidence
	body, err := json.Marshal(request)
	if err != nil {
		return ErrProtocolRejected
	}
	defer credential.Zero(body)
	response, status, err := client.postWithRetry(ctx, "/agent/v1/executions/"+input.ExecutionID+":events:append", body, state.MachineCredential)
	if err != nil {
		return err
	}
	defer credential.Zero(response)
	if status != http.StatusOK {
		return executionHTTPError(status)
	}
	if _, err := decodeResponseEnvelope(response, "EXECUTION_EVENT_ACCEPTED"); err != nil {
		return ErrProtocolRejected
	}
	return nil
}

// AppendExecutionLog 上报已经密封且完成第一层脱敏的日志批次。
func (s *StateStore) AppendExecutionLog(ctx context.Context, input ExecutionLogBatch) error {
	if !validExecutionLeaseInput(input.BootID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, input.SentAt) || input.Batch.StreamID != input.ExecutionID {
		return ErrProtocolRejected
	}
	sealed, err := logstream.SealBatch(input.Batch)
	if err != nil {
		return ErrProtocolRejected
	}
	state, found, err := s.loadState()
	if err != nil || !found || state.pendingEnrollment() {
		if err != nil {
			return err
		}
		return ErrIdentityUnavailable
	}
	defer state.destroy()
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return ErrIdentityUnavailable
	}
	request := executionLogRequest{ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID, BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano), PayloadType: "OBDUMPER_EXPORT_APPEND_LOG"}
	request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest, request.Payload.Batch = input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, sealed
	body, err := json.Marshal(request)
	if err != nil {
		return ErrProtocolRejected
	}
	defer credential.Zero(body)
	response, status, err := client.postWithRetry(ctx, "/agent/v1/executions/"+input.ExecutionID+":logs:append", body, state.MachineCredential)
	if err != nil {
		return err
	}
	defer credential.Zero(response)
	if status != http.StatusOK {
		return executionHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(response, "EXECUTION_LOG_ACCEPTED")
	if err != nil {
		return ErrProtocolRejected
	}
	var confirmation executionLogAcceptedPayload
	if decodeStrictJSON(envelope.Payload, &confirmation) != nil || !validExecutionLogConfirmation(confirmation, sealed) {
		return ErrProtocolRejected
	}
	return nil
}

// AppendExecutionLogGap 上报已经持久化在 Agent 私有队列中的无正文日志缺口。
// 只有控制面确认同一来源范围与缺口摘要后，调用方才可删除本地缺口条目。
func (s *StateStore) AppendExecutionLogGap(ctx context.Context, input ExecutionLogGap) error {
	if !validExecutionLeaseInput(input.BootID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, input.SentAt) || input.Gap.StreamID != input.ExecutionID {
		return ErrProtocolRejected
	}
	gapDigest, err := logstream.GapDigest(input.Gap)
	if err != nil {
		return ErrProtocolRejected
	}
	state, found, err := s.loadState()
	if err != nil || !found || state.pendingEnrollment() {
		if err != nil {
			return err
		}
		return ErrIdentityUnavailable
	}
	defer state.destroy()
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return ErrIdentityUnavailable
	}
	request := executionLogGapRequest{ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID, BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano), PayloadType: "OBDUMPER_EXPORT_APPEND_LOG_GAP"}
	request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest, request.Payload.Gap = input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, input.Gap
	body, err := json.Marshal(request)
	if err != nil {
		return ErrProtocolRejected
	}
	defer credential.Zero(body)
	response, status, err := client.postWithRetry(ctx, "/agent/v1/executions/"+input.ExecutionID+":logs:gap", body, state.MachineCredential)
	if err != nil {
		return err
	}
	defer credential.Zero(response)
	if status != http.StatusOK {
		return executionHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(response, "EXECUTION_LOG_GAP_ACCEPTED")
	if err != nil {
		return ErrProtocolRejected
	}
	var confirmation executionLogGapAcceptedPayload
	if decodeStrictJSON(envelope.Payload, &confirmation) != nil || !validExecutionLogGapConfirmation(confirmation, input.Gap, gapDigest) {
		return ErrProtocolRejected
	}
	return nil
}

func (s *StateStore) withExecutionState(ctx context.Context, bootID, executionID, leaseID string, leaseEpoch int64, digest string, sentAt time.Time, payloadType string, operation func(*httpsClient, *identityState, string, executionLeaseRequest) error) error {
	if !validExecutionLeaseInput(bootID, executionID, leaseID, leaseEpoch, digest, sentAt) || operation == nil {
		return ErrProtocolRejected
	}
	state, found, err := s.loadState()
	if err != nil || !found || state.pendingEnrollment() {
		if err != nil {
			return err
		}
		return ErrIdentityUnavailable
	}
	defer state.destroy()
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return ErrIdentityUnavailable
	}
	request := executionLeaseRequest{ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID, BootID: bootID, RequestID: requestID, SentAt: sentAt.UTC().Format(time.RFC3339Nano), PayloadType: payloadType}
	request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest = leaseID, leaseEpoch, digest
	return operation(client, &state, requestID, request)
}

func (c *httpsClient) claimNextExecution(ctx context.Context, state *identityState, requestID string, input ExecutionClaimNext) (ExecutionGrant, bool, error) {
	request := executionClaimNextRequest{ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID, BootID: input.BootID, RequestID: requestID, SentAt: input.SentAt.UTC().Format(time.RFC3339Nano), PayloadType: "OBDUMPER_EXPORT_CLAIM_NEXT"}
	request.Payload.Capability = "OBDUMPER_EXPORT"
	body, err := json.Marshal(request)
	if err != nil {
		return ExecutionGrant{}, false, ErrProtocolRejected
	}
	defer credential.Zero(body)
	response, status, err := c.postWithRetry(ctx, "/agent/v1/executions:claim-next", body, state.MachineCredential)
	if err != nil {
		return ExecutionGrant{}, false, err
	}
	defer credential.Zero(response)
	if status == http.StatusNoContent {
		if len(response) != 0 {
			return ExecutionGrant{}, false, ErrProtocolRejected
		}
		return ExecutionGrant{}, false, nil
	}
	if status != http.StatusOK {
		return ExecutionGrant{}, false, executionHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(response, "EXECUTION_CLAIMED")
	if err != nil {
		return ExecutionGrant{}, false, ErrProtocolRejected
	}
	var payload executionClaimPayload
	if decodeStrictJSON(envelope.Payload, &payload) != nil || payload.RealExecutionEnabled == nil || !*payload.RealExecutionEnabled {
		return ExecutionGrant{}, false, ErrProtocolRejected
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	if err != nil {
		return ExecutionGrant{}, false, ErrProtocolRejected
	}
	grant := ExecutionGrant{TaskID: payload.TaskID, ExecutionID: payload.ExecutionID, LeaseID: payload.LeaseID, LeaseEpoch: payload.LeaseEpoch, ExpiresAt: expiresAt, EnvelopeDigest: payload.EnvelopeDigest, Argv: append([]string(nil), payload.Argv...)}
	if !validExecutionGrant(grant) {
		return ExecutionGrant{}, false, ErrProtocolRejected
	}
	return grant, true, nil
}

func (p *executionSecretPayload) destroy() {
	if p == nil {
		return
	}
	credential.Zero(p.Connection.Username)
	credential.Zero(p.Connection.Password)
	p.Connection.Username, p.Connection.Password = nil, nil
}

func validExecutionClaimNext(input ExecutionClaimNext) bool {
	return validOpaqueValue(input.BootID, 256) && !input.SentAt.IsZero()
}
func validExecutionLeaseInput(bootID, executionID, leaseID string, leaseEpoch int64, digest string, sentAt time.Time) bool {
	return validOpaqueValue(bootID, 256) && validOpaqueValue(executionID, 256) && !strings.ContainsAny(executionID, "/\\?#:") && validOpaqueValue(leaseID, 256) && leaseEpoch > 0 && validExecutionDigest(digest) && !sentAt.IsZero()
}
func validExecutionGrant(grant ExecutionGrant) bool {
	return validExecutionLeaseInput("boot", grant.ExecutionID, grant.LeaseID, grant.LeaseEpoch, grant.EnvelopeDigest, grant.ExpiresAt) && validOpaqueValue(grant.TaskID, 256) && !strings.ContainsAny(grant.TaskID, "/\\?#:") && grant.ExpiresAt.After(time.Now().UTC().Add(-5*time.Minute)) && validExecutionArgv(grant.Argv)
}
func validExecutionDigest(value string) bool {
	return len(value) == 64 && value == strings.ToLower(value) && validSHA256Hex(value)
}
func validSHA256Hex(value string) bool {
	for _, item := range value {
		if !(item >= '0' && item <= '9' || item >= 'a' && item <= 'f') {
			return false
		}
	}
	return true
}
func validExecutionArgv(argv []string) bool {
	if len(argv) == 0 || len(argv) > 32 {
		return false
	}
	for _, item := range argv {
		if item == "" || len(item) > 4096 || strings.ContainsRune(item, 0) || strings.ContainsAny(item, "\r\n") || item == "--password" || strings.HasPrefix(item, "--password=") || strings.HasPrefix(item, "-p") {
			return false
		}
	}
	return true
}
func validExecutionEvent(input ExecutionEvent) bool {
	if !validExecutionLeaseInput(input.BootID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest, input.SentAt) || !validOpaqueValue(input.EventID, 256) || input.EventSeq < 3 || input.Evidence == nil {
		return false
	}
	return input.EventType == "START_REJECTED" || input.EventType == "PROCESS_STARTED" || input.EventType == "PROCESS_EXITED" || input.EventType == "TOOL_TERMINAL_OBSERVED" || input.EventType == "RESULT_FACTS_OBSERVED"
}

func validExecutionLogConfirmation(confirmation executionLogAcceptedPayload, batch logstream.Batch) bool {
	if confirmation.Decision != logstream.BatchAccepted && confirmation.Decision != logstream.BatchDuplicate {
		return false
	}
	return confirmation.RealExecutionEnabled &&
		confirmation.ExpectedSequence == batch.LastSeq+1 &&
		confirmation.StreamID == batch.StreamID &&
		confirmation.SourceEpoch == batch.SourceEpoch &&
		confirmation.FirstSequence == batch.FirstSeq &&
		confirmation.LastSequence == batch.LastSeq &&
		confirmation.BatchDigest == batch.Digest
}

func validExecutionLogGapConfirmation(confirmation executionLogGapAcceptedPayload, gap logstream.GapNotice, gapDigest string) bool {
	if confirmation.Decision != logstream.BatchAccepted && confirmation.Decision != logstream.BatchDuplicate {
		return false
	}
	return confirmation.RealExecutionEnabled &&
		confirmation.ExpectedSequence == gap.LastSeq+1 &&
		confirmation.StreamID == gap.StreamID &&
		confirmation.SourceEpoch == gap.SourceEpoch &&
		confirmation.FirstSequence == gap.FirstSeq &&
		confirmation.LastSequence == gap.LastSeq &&
		confirmation.GapDigest == gapDigest
}

func executionHTTPError(status int) error {
	if status >= 500 {
		return ErrControlPlaneUnavailable
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ErrAgentAuthenticationDenied
	}
	return ErrProtocolRejected
}
