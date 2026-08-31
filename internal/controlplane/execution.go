package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/store"
)

const executionLeaseTTL = 2 * time.Minute

// agentExecutionEnvelope 是正式导出受认证机器协议的最小固定信封。
// 任务标识、命令、输出目录和秘密均不能由 Agent 通过该信封自行指定。
type agentExecutionEnvelope struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
}

type agentExecutionClaimRequest struct {
	agentExecutionEnvelope
	Payload struct {
		Capability string `json:"capability"`
	} `json:"payload"`
}

type agentExecutionLeaseRequest struct {
	agentExecutionEnvelope
	Payload struct {
		LeaseID        string `json:"leaseId"`
		LeaseEpoch     int64  `json:"leaseEpoch"`
		EnvelopeDigest string `json:"envelopeDigest"`
	} `json:"payload"`
}

type agentExecutionEventRequest struct {
	agentExecutionEnvelope
	Payload struct {
		LeaseID        string          `json:"leaseId"`
		LeaseEpoch     int64           `json:"leaseEpoch"`
		EnvelopeDigest string          `json:"envelopeDigest"`
		EventID        string          `json:"eventId"`
		EventSeq       int64           `json:"eventSeq"`
		EventType      string          `json:"eventType"`
		Evidence       json.RawMessage `json:"evidence"`
	} `json:"payload"`
}

type agentExecutionLogRequest struct {
	agentExecutionEnvelope
	Payload struct {
		LeaseID        string          `json:"leaseId"`
		LeaseEpoch     int64           `json:"leaseEpoch"`
		EnvelopeDigest string          `json:"envelopeDigest"`
		Batch          logstream.Batch `json:"batch"`
	} `json:"payload"`
}

type agentExecutionLogGapRequest struct {
	agentExecutionEnvelope
	Payload struct {
		LeaseID        string              `json:"leaseId"`
		LeaseEpoch     int64               `json:"leaseEpoch"`
		EnvelopeDigest string              `json:"envelopeDigest"`
		Gap            logstream.GapNotice `json:"gap"`
	} `json:"payload"`
}

// authenticatedExecutionAgent 只在受控真实执行模式中开放执行协议。
// 单独的 Agent 凭据、SQLite 执行仓储和解密器缺少任一项时一律拒绝，不退化到旧合成协议。
func (s *Server) authenticatedExecutionAgent(w http.ResponseWriter, r *http.Request) (store.AgentIdentity, AuthenticatedAgentExecutionStore, bool) {
	executions, ok := s.executions.(AuthenticatedAgentExecutionStore)
	if !s.realExecutionEnabled || !ok || s.agentProtocol == nil || s.decryptor == nil || s.logs == nil || s.heartbeatTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "AGENT_EXECUTION_NOT_CONFIGURED", "当前环境尚未配置受控导出执行", false)
		return store.AgentIdentity{}, nil, false
	}
	credentialDigest, ok := agentCredentialDigest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, nil, false
	}
	machine, err := s.agentProtocol.AuthenticateAgent(r.Context(), credentialDigest)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return store.AgentIdentity{}, nil, false
	}
	return machine, executions, true
}

func validExecutionEnvelope(machine store.AgentIdentity, envelope agentExecutionEnvelope, payloadType string) bool {
	if envelope.ProtocolVersion != agentwire.Version || envelope.ProtocolVersion != machine.ProtocolVersion || envelope.PayloadType != payloadType || envelope.AgentID != machine.AgentID || envelope.NodeID != machine.NodeID || !validAgentPrecheckOpaque(envelope.BootID) || !validAgentPrecheckOpaque(envelope.RequestID) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, envelope.SentAt)
	return err == nil
}

// claimNextAuthenticatedExecution 由服务端选择任务并生成 execution/lease 标识。
// 请求不接受 taskId、命令、路径或凭据，避免把该入口扩展为远程执行通道。
func (s *Server) claimNextAuthenticatedExecution(w http.ResponseWriter, r *http.Request) {
	machine, executions, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionClaimRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_CLAIM_NEXT") || request.Payload.Capability != "OBDUMPER_EXPORT" {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	executionID, err := identifier.NewUUIDV4()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_CLAIM_UNAVAILABLE", "任务领取暂时不可用", true)
		return
	}
	leaseID, err := identifier.NewUUIDV4()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_CLAIM_UNAVAILABLE", "任务领取暂时不可用", true)
		return
	}
	scheduledEventID, err := identifier.NewUUIDV4()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_CLAIM_UNAVAILABLE", "任务领取暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	grant, found, err := executions.ClaimNextExecution(r.Context(), store.ExecutionClaimNext{
		AgentID: machine.AgentID, NodeID: machine.NodeID, ExecutionID: executionID, LeaseID: leaseID,
		ScheduledEventID: scheduledEventID, RequestID: request.RequestID, LeaseTTL: executionLeaseTTL,
		HeartbeatFreshAfter: now.Add(-s.heartbeatTTL), Now: now,
	})
	if errors.Is(err, store.ErrClaimIneligible) || errors.Is(err, store.ErrPrecheckLeaseRejected) {
		writeError(w, http.StatusConflict, "EXECUTION_CLAIM_REJECTED", "当前任务不满足执行条件", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_CLAIM_UNAVAILABLE", "任务领取暂时不可用", true)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeExecutionResponse(w, "EXECUTION_CLAIMED", map[string]any{
		"taskId": grant.TaskID, "executionId": grant.ExecutionID, "leaseId": grant.LeaseID,
		"leaseEpoch": grant.LeaseEpoch, "expiresAt": grant.ExpiresAt.Format(time.RFC3339Nano),
		"envelopeDigest": grant.EnvelopeDigest, "argv": grant.PlannedArgv,
		"toolVersion": grant.ToolVersion, "metadataVersion": grant.MetadataVersion,
		"capabilityVersion": grant.CapabilityVersion, "realExecutionEnabled": true,
	})
}

func (s *Server) acknowledgeAuthenticatedExecution(w http.ResponseWriter, r *http.Request, executionID string) {
	machine, executions, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionLeaseRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_ACKNOWLEDGE_LEASE") || !validExecutionPathID(executionID) || !validExecutionLeasePayload(request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	_, err := executions.AppendAuthenticatedExecutionEvent(r.Context(), machine.AgentID, store.ExecutionEvent{
		EventID: request.RequestID + "-ack", ExecutionID: executionID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
		EventSeq: 2, EventType: "LEASE_ACKNOWLEDGED", PayloadJSON: "{}", ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		writeExecutionStoreError(w, err)
		return
	}
	writeExecutionResponse(w, "EXECUTION_LEASE_ACKNOWLEDGED", map[string]any{"realExecutionEnabled": true})
}

// renewAuthenticatedExecution 仅按控制面时钟续期当前机器已经领取的执行租约。
// 续期请求不会触发子进程、重新选择任务或重新返回数据库秘密。
func (s *Server) renewAuthenticatedExecution(w http.ResponseWriter, r *http.Request, executionID string) {
	machine, executions, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionLeaseRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_RENEW_LEASE") || !validExecutionPathID(executionID) || !validExecutionLeasePayload(request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(executionLeaseTTL)
	if err := executions.RenewExecutionLease(r.Context(), store.LeaseRenewal{ExecutionID: executionID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch, AgentID: machine.AgentID, ExpiresAt: expiresAt}); err != nil {
		writeExecutionStoreError(w, err)
		return
	}
	writeExecutionResponse(w, "EXECUTION_LEASE_RENEWED", map[string]any{"executionId": executionID, "leaseId": request.Payload.LeaseID, "leaseEpoch": request.Payload.LeaseEpoch, "envelopeDigest": request.Payload.EnvelopeDigest, "expiresAt": expiresAt.Format(time.RFC3339Nano), "realExecutionEnabled": true})
}

// pollAuthenticatedExecutionControl 返回当前租约绑定的固定取消事实，不暴露浏览器请求原文。
func (s *Server) pollAuthenticatedExecutionControl(w http.ResponseWriter, r *http.Request, executionID string) {
	machine, executions, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionLeaseRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_POLL_CONTROL") || !validExecutionPathID(executionID) || !validExecutionLeasePayload(request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	control, err := executions.PollExecutionControl(r.Context(), store.ExecutionControlPoll{
		AgentID: machine.AgentID, ExecutionID: executionID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
		EnvelopeDigest: request.Payload.EnvelopeDigest, Now: time.Now().UTC(),
	})
	if err != nil {
		writeExecutionStoreError(w, err)
		return
	}
	payload := map[string]any{
		"executionId": executionID, "leaseId": request.Payload.LeaseID, "leaseEpoch": request.Payload.LeaseEpoch,
		"envelopeDigest": request.Payload.EnvelopeDigest, "cancelRequested": control.CancelRequested,
		"realExecutionEnabled": true,
	}
	if control.CancellationRequestID != "" {
		payload["cancellationRequestId"] = control.CancellationRequestID
	}
	if !control.CancellationDeadline.IsZero() {
		payload["cancellationDeadline"] = control.CancellationDeadline.Format(time.RFC3339Nano)
	}
	writeExecutionResponse(w, "EXECUTION_CONTROL", payload)
}

// resolveAuthenticatedExecutionSecret 只为已领取、已确认的 execution 返回唯一数据库密码槽位。
// 该响应不写 Agent 状态文件，且用户名与密码在 HTTP 编码后立即清零。
func (s *Server) resolveAuthenticatedExecutionSecret(w http.ResponseWriter, r *http.Request, executionID string) {
	machine, executions, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionLeaseRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_RESOLVE_SECRET_SLOTS") || !validExecutionPathID(executionID) || !validExecutionLeasePayload(request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest) {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	now := time.Now().UTC()
	requestDigest := executionRequestDigest("RESOLVE_SECRET", request.agentExecutionEnvelope, executionID, request.Payload)
	encrypted, err := executions.ResolveExecutionDatabaseConnection(r.Context(), store.ExecutionSecretResolutionRequest{
		AgentID: machine.AgentID, ExecutionID: executionID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
		EnvelopeDigest: request.Payload.EnvelopeDigest, RequestID: request.RequestID, RequestDigest: requestDigest, Now: now,
	})
	if err != nil {
		writeExecutionStoreError(w, err)
		return
	}
	defer encrypted.Destroy()
	finish := func(succeeded bool) error {
		return executions.FinishExecutionSecretResolution(r.Context(), store.ExecutionSecretResolutionOutcome{
			AgentID: machine.AgentID, ExecutionID: executionID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
			EnvelopeDigest: request.Payload.EnvelopeDigest, RequestID: request.RequestID, RequestDigest: requestDigest, Succeeded: succeeded, Now: time.Now().UTC(),
		})
	}
	owner := identity.Principal{Type: identity.BrowserPrincipal, ID: encrypted.OwnerSubjectID}
	if identity.Validate(owner, identity.BrowserPrincipal) != nil || s.authorizer == nil || identity.Can(r.Context(), s.authorizer, owner, identity.ScopeDataSourceRead, encrypted.DataSourceID) != nil || identity.Can(r.Context(), s.authorizer, owner, identity.ScopeNodeUse, encrypted.NodeID) != nil {
		_ = finish(false)
		writeExecutionStoreError(w, store.ErrEventRejected)
		return
	}
	plaintext, err := s.decryptor.Decrypt(credential.Envelope{
		FormatVersion: credential.FormatVersion, KeyID: encrypted.KeyID,
		Reference: credential.Reference{CredentialID: encrypted.CredentialID, Revision: encrypted.Revision, SecretType: credential.DatabasePassword, DataSourceID: encrypted.DataSourceID},
		Nonce:     encrypted.Nonce, Ciphertext: encrypted.Ciphertext,
	})
	if err != nil {
		_ = finish(false)
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_SECRET_RESOLUTION_UNAVAILABLE", "导出秘密槽位暂时不可用", true)
		return
	}
	defer credential.Zero(plaintext)
	// EX-I6 存储凭据槽位（2026-08-14）：同一解析请求内附带对象存储任务的凭据。
	// 本地输出任务返回空结构，不附带 storageCredential 段。
	encryptedStorage, err := executions.ResolveExecutionStorageCredential(r.Context(), store.ExecutionSecretResolutionRequest{
		AgentID: machine.AgentID, ExecutionID: executionID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
		EnvelopeDigest: request.Payload.EnvelopeDigest, RequestID: request.RequestID, RequestDigest: requestDigest, Now: now,
	})
	if err != nil {
		_ = finish(false)
		writeExecutionStoreError(w, err)
		return
	}
	defer encryptedStorage.Destroy()
	var storagePayload any
	if encryptedStorage.StorageCredentialID != "" {
		accessKeyPlaintext, err := s.decryptor.Decrypt(credential.Envelope{
			FormatVersion: credential.FormatVersion, KeyID: encryptedStorage.AccessKeyKeyID,
			Reference: credential.Reference{CredentialID: encryptedStorage.AccessKeyCredentialID, Revision: encryptedStorage.Revision, SecretType: credential.StorageAccessKey, DataSourceID: encryptedStorage.StorageCredentialID},
			Nonce:     encryptedStorage.AccessKeyNonce, Ciphertext: encryptedStorage.AccessKeyCiphertext,
		})
		if err != nil {
			_ = finish(false)
			writeError(w, http.StatusServiceUnavailable, "EXECUTION_SECRET_RESOLUTION_UNAVAILABLE", "导出秘密槽位暂时不可用", true)
			return
		}
		defer credential.Zero(accessKeyPlaintext)
		secretKeyPlaintext, err := s.decryptor.Decrypt(credential.Envelope{
			FormatVersion: credential.FormatVersion, KeyID: encryptedStorage.SecretKeyKeyID,
			Reference: credential.Reference{CredentialID: encryptedStorage.SecretKeyCredentialID, Revision: encryptedStorage.Revision, SecretType: credential.StorageSecretKey, DataSourceID: encryptedStorage.StorageCredentialID},
			Nonce:     encryptedStorage.SecretKeyNonce, Ciphertext: encryptedStorage.SecretKeyCiphertext,
		})
		if err != nil {
			_ = finish(false)
			writeError(w, http.StatusServiceUnavailable, "EXECUTION_SECRET_RESOLUTION_UNAVAILABLE", "导出秘密槽位暂时不可用", true)
			return
		}
		defer credential.Zero(secretKeyPlaintext)
		storagePayload = map[string]any{
			"provider": encryptedStorage.Provider, "accessKey": accessKeyPlaintext, "secretKey": secretKeyPlaintext,
		}
	}
	if err := finish(true); err != nil {
		writeExecutionStoreError(w, err)
		return
	}
	writeExecutionResponse(w, "EXECUTION_SECRET_SLOTS_RESOLVED", map[string]any{
		"agentRequestId": request.RequestID, "executionId": executionID, "leaseId": request.Payload.LeaseID,
		"leaseEpoch": request.Payload.LeaseEpoch, "envelopeDigest": request.Payload.EnvelopeDigest,
		"connection":           map[string]any{"host": encrypted.Host, "port": encrypted.Port, "username": encrypted.Username, "password": plaintext},
		"storageCredential":    storagePayload,
		"realExecutionEnabled": true,
	})
}

func (s *Server) appendAuthenticatedExecutionEvent(w http.ResponseWriter, r *http.Request, executionID string) {
	machine, executions, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionEventRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_APPEND_EVENT") || !validExecutionPathID(executionID) || !validExecutionLeasePayload(request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest) || !validAgentPrecheckOpaque(request.Payload.EventID) || request.Payload.EventSeq < 3 || len(request.Payload.Evidence) == 0 {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	var evidence any
	if json.Unmarshal(request.Payload.Evidence, &evidence) != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	state, err := executions.AppendAuthenticatedExecutionEvent(r.Context(), machine.AgentID, store.ExecutionEvent{
		EventID: request.Payload.EventID, ExecutionID: executionID, LeaseID: request.Payload.LeaseID, LeaseEpoch: request.Payload.LeaseEpoch,
		EventSeq: request.Payload.EventSeq, EventType: request.Payload.EventType, PayloadJSON: string(payload), ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		writeExecutionStoreError(w, err)
		return
	}
	writeExecutionResponse(w, "EXECUTION_EVENT_ACCEPTED", map[string]any{"state": state, "realExecutionEnabled": true})
}

// appendAuthenticatedExecutionLog 接收已完成第一层脱敏的日志批次，并由控制面执行键值泄露拦截和序号核对。
// 终态后只接受同一已释放租约的已持久化补传，不能借日志端点恢复执行或改写任务状态。
func (s *Server) appendAuthenticatedExecutionLog(w http.ResponseWriter, r *http.Request, executionID string) {
	machine, _, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionLogRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_APPEND_LOG") || !validExecutionPathID(executionID) || !validExecutionLeasePayload(request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest) || request.Payload.Batch.StreamID != executionID {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	sealed, err := logstream.SealBatch(request.Payload.Batch)
	if err != nil || request.Payload.Batch.Digest != sealed.Digest {
		writeError(w, http.StatusBadRequest, "LOG_BATCH_REJECTED", "日志批次不符合安全约束", false)
		return
	}
	request.Payload.Batch = sealed
	database, databaseOK := s.executions.(*store.Store)
	if !databaseOK {
		writeError(w, http.StatusServiceUnavailable, "LOG_BATCH_UNAVAILABLE", "日志批次暂时不可用", true)
		return
	}
	taskID, err := database.AuthorizeExecutionLogAppend(r.Context(), machine.AgentID, executionID, request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest, time.Now().UTC())
	if err != nil || taskID == "" {
		writeExecutionStoreError(w, err)
		return
	}
	result, err := s.logs.appendBatch(r.Context(), taskID, executionID, request.Payload.Batch)
	if errors.Is(err, logstream.ErrBatchGap) {
		writeJSON(w, http.StatusConflict, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSeq})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "LOG_BATCH_REJECTED", "日志批次不符合安全约束", false)
		return
	}
	writeExecutionResponse(w, "EXECUTION_LOG_ACCEPTED", executionLogConfirmationPayload(sealed, result))
}

// appendAuthenticatedExecutionLogGap 接收 Agent 私有队列在容量达到上限时形成的无正文缺口。
// 该入口只推进已验证的来源序号，不能接受日志正文、任意原因文本或跨租约缺口。
func (s *Server) appendAuthenticatedExecutionLogGap(w http.ResponseWriter, r *http.Request, executionID string) {
	machine, _, ok := s.authenticatedExecutionAgent(w, r)
	if !ok {
		return
	}
	var request agentExecutionLogGapRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if !validExecutionEnvelope(machine, request.agentExecutionEnvelope, "OBDUMPER_EXPORT_APPEND_LOG_GAP") || !validExecutionPathID(executionID) || !validExecutionLeasePayload(request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest) || request.Payload.Gap.StreamID != executionID {
		writeError(w, http.StatusBadRequest, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	gapDigest, err := logstream.GapDigest(request.Payload.Gap)
	if err != nil {
		writeError(w, http.StatusBadRequest, "LOG_GAP_REJECTED", "日志缺口不符合安全约束", false)
		return
	}
	database, databaseOK := s.executions.(*store.Store)
	if !databaseOK {
		writeError(w, http.StatusServiceUnavailable, "LOG_GAP_UNAVAILABLE", "日志缺口暂时无法保存", true)
		return
	}
	taskID, err := database.AuthorizeExecutionLogAppend(r.Context(), machine.AgentID, executionID, request.Payload.LeaseID, request.Payload.LeaseEpoch, request.Payload.EnvelopeDigest, time.Now().UTC())
	if err != nil || taskID == "" {
		writeExecutionStoreError(w, err)
		return
	}
	result, err := s.logs.appendGap(r.Context(), taskID, executionID, request.Payload.Gap)
	if errors.Is(err, logstream.ErrBatchGap) {
		writeJSON(w, http.StatusConflict, map[string]any{"requestId": requestID(w), "decision": result.Decision, "expectedSequence": result.ExpectedSeq})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "LOG_GAP_REJECTED", "日志缺口不符合安全约束", false)
		return
	}
	writeExecutionResponse(w, "EXECUTION_LOG_GAP_ACCEPTED", executionLogGapConfirmationPayload(request.Payload.Gap, gapDigest, result))
}

// executionLogConfirmationPayload 仅回显当前已确认批次的安全定位字段。
// Agent 必须逐项核对后才可删除私有队列副本，不能把其他请求的延迟或重复回执当作确认。
func executionLogConfirmationPayload(batch logstream.Batch, result logstream.BatchResult) map[string]any {
	return map[string]any{
		"decision": result.Decision, "expectedSequence": result.ExpectedSeq,
		"streamId": batch.StreamID, "sourceEpoch": batch.SourceEpoch,
		"firstSequence": batch.FirstSeq, "lastSequence": batch.LastSeq,
		"batchDigest": batch.Digest, "realExecutionEnabled": true,
	}
}

// executionLogGapConfirmationPayload 回显已确认缺口的安全来源范围与摘要。
// Agent 只能据此删除同一私有缺口文件，不能把普通批次回执混用为缺口确认。
func executionLogGapConfirmationPayload(gap logstream.GapNotice, gapDigest string, result logstream.BatchResult) map[string]any {
	return map[string]any{
		"decision": result.Decision, "expectedSequence": result.ExpectedSeq,
		"streamId": gap.StreamID, "sourceEpoch": gap.SourceEpoch,
		"firstSequence": gap.FirstSeq, "lastSequence": gap.LastSeq,
		"gapDigest": gapDigest, "realExecutionEnabled": true,
	}
}

func validExecutionPathID(value string) bool { return validAgentPrecheckPathID(value) }

func parseAuthenticatedExecutionAction(path string) (string, string, bool) {
	const prefix = "/agent/v1/executions/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	value := strings.TrimPrefix(path, prefix)
	for suffix, action := range map[string]string{
		":acknowledge-lease":    "acknowledge-lease",
		":renew-lease":          "renew-lease",
		":poll-control":         "poll-control",
		"/secret-slots:resolve": "resolve-secret-slots",
		":events:append":        "append-events",
		":logs:append":          "append-logs",
		":logs:gap":             "append-log-gap",
	} {
		if strings.HasSuffix(value, suffix) {
			executionID := strings.TrimSuffix(value, suffix)
			return executionID, action, validExecutionPathID(executionID)
		}
	}
	return "", "", false
}

func validExecutionLeasePayload(leaseID string, epoch int64, digest string) bool {
	return validAgentPrecheckOpaque(leaseID) && epoch > 0 && validAgentPrecheckDigest(digest)
}

func executionRequestDigest(action string, envelope agentExecutionEnvelope, executionID string, payload any) string {
	content, _ := json.Marshal(struct {
		Action    string `json:"action"`
		Agent     string `json:"agent"`
		Node      string `json:"node"`
		Boot      string `json:"boot"`
		Request   string `json:"request"`
		Execution string `json:"execution"`
		Payload   any    `json:"payload"`
	}{action, envelope.AgentID, envelope.NodeID, envelope.BootID, envelope.RequestID, executionID, payload})
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func writeExecutionResponse(w http.ResponseWriter, status string, payload any) {
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": status, "payload": payload})
}

func writeExecutionStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "EXECUTION_REQUEST_CONFLICT", "执行请求与此前请求冲突", false)
	case errors.Is(err, store.ErrEventRejected), errors.Is(err, store.ErrClaimIneligible), errors.Is(err, store.ErrPrecheckLeaseRejected):
		writeError(w, http.StatusConflict, "EXECUTION_LEASE_REJECTED", "任务租约无效或已过期", false)
	default:
		writeError(w, http.StatusServiceUnavailable, "EXECUTION_PROTOCOL_UNAVAILABLE", "执行协议暂时不可用", true)
	}
}
