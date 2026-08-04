package agentwire

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/outputpath"
)

// PrecheckClaimNext 是 Agent 请求服务端原子领取下一条固定 EXPORT_PREFLIGHT 的最小输入。
// Agent 不能提供预检查或租约标识，避免命令行、本地环境或浏览器把任意工作注入 Worker。
type PrecheckClaimNext struct {
	BootID string
	SentAt time.Time
}

// PrecheckGrant 是控制面签发的不可变预检查租约。
// BindingDigest 与 CheckSet 必须在确认租约及提交结果时原样关联，避免检查范围或绑定被替换。
type PrecheckGrant struct {
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	ExpiresAt     time.Time
	Binding       agentstate.PrecheckBinding
	BindingDigest string
	CheckSet      []agentpreflight.CheckID
	Context       PrecheckExecutionContext
}

// PrecheckExecutionContext 是控制面从冻结草稿和节点声明派生的固定本地检查输入。
// Agent 必须按原样使用它，不能通过命令行、本地环境或浏览器请求覆盖数据库对象、路径或根目录。
type PrecheckExecutionContext struct {
	CompatibilityMode string
	Database          string
	Table             string
	OutputPath        string
	LogPath           string
	SkipCheckDir      bool
	TargetPlatform    commandgen.Platform
	AllowedRoots      []string
}

// PrecheckLeaseAcknowledgement 是 Agent 对一个已领取预检查租约的固定确认输入。
// 它只确认控制面签发的摘要和 epoch，不允许调用方重新提交或修改绑定内容。
type PrecheckLeaseAcknowledgement struct {
	BootID        string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	SentAt        time.Time
}

// PrecheckCompletion 是 Agent 上报固定检查清单结果的最小输入。
// Report 必须完整匹配控制面的固定检查集；Succeeded 只由报告本身派生，不能作为独立网络字段传输。
type PrecheckCompletion struct {
	BootID        string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	SentAt        time.Time
	Report        agentpreflight.Report
}

// PrecheckSecretSlotRequest 是 Agent 在已确认预检查租约中解析数据库连接槽位的最小输入。
// 槽位值不出现在该请求中；控制面只依据绑定的 Agent、租约、epoch 和摘要重新读取当前材料。
type PrecheckSecretSlotRequest struct {
	BootID        string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	SentAt        time.Time
}

// DatabaseConnectionSlot 是 Agent 只在内存中使用的短时数据库连接槽位。
// Username 和 Password 必须由调用方在 JDBC 探针返回后立即清零，不能放入状态文件、日志或环境变量。
type DatabaseConnectionSlot struct {
	Host     string
	Port     int
	Username []byte
	Password []byte
}

// Destroy 尽力清除短时数据库连接凭据，减少它们在普通 Agent 逻辑中继续存活的机会。
func (s *DatabaseConnectionSlot) Destroy() {
	if s == nil {
		return
	}
	credential.Zero(s.Username)
	credential.Zero(s.Password)
	s.Username = nil
	s.Password = nil
}

// PrecheckState 表示控制面确认的预检查终态。
// 当前协议只接受完整成功或失败，超时、未知或中间状态不能由 Agent complete 响应伪造。
type PrecheckState string

const (
	// PrecheckSucceeded 表示所有固定检查均通过。
	PrecheckSucceeded PrecheckState = "SUCCEEDED"
	// PrecheckFailed 表示至少一个固定检查未通过或事实不可用。
	PrecheckFailed PrecheckState = "FAILED"
)

type precheckClaimNextRequest struct {
	ProtocolVersion string                   `json:"protocolVersion"`
	AgentID         string                   `json:"agentId"`
	NodeID          string                   `json:"nodeId"`
	BootID          string                   `json:"bootId"`
	RequestID       string                   `json:"requestId"`
	SentAt          string                   `json:"sentAt"`
	PayloadType     string                   `json:"payloadType"`
	Payload         precheckClaimNextPayload `json:"payload"`
}

type precheckClaimNextPayload struct {
	Capability string `json:"capability"`
}

type precheckLeaseAcknowledgementRequest struct {
	ProtocolVersion string                              `json:"protocolVersion"`
	AgentID         string                              `json:"agentId"`
	NodeID          string                              `json:"nodeId"`
	BootID          string                              `json:"bootId"`
	RequestID       string                              `json:"requestId"`
	SentAt          string                              `json:"sentAt"`
	PayloadType     string                              `json:"payloadType"`
	Payload         precheckLeaseAcknowledgementPayload `json:"payload"`
}

type precheckLeaseAcknowledgementPayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
}

type precheckSecretSlotRequest struct {
	ProtocolVersion string                           `json:"protocolVersion"`
	AgentID         string                           `json:"agentId"`
	NodeID          string                           `json:"nodeId"`
	BootID          string                           `json:"bootId"`
	RequestID       string                           `json:"requestId"`
	SentAt          string                           `json:"sentAt"`
	PayloadType     string                           `json:"payloadType"`
	Payload         precheckSecretSlotRequestPayload `json:"payload"`
}

type precheckSecretSlotRequestPayload struct {
	LeaseID       string `json:"leaseId"`
	LeaseEpoch    int64  `json:"leaseEpoch"`
	BindingDigest string `json:"bindingDigest"`
	Slot          string `json:"slot"`
}

type precheckCompletionRequest struct {
	ProtocolVersion string                    `json:"protocolVersion"`
	AgentID         string                    `json:"agentId"`
	NodeID          string                    `json:"nodeId"`
	BootID          string                    `json:"bootId"`
	RequestID       string                    `json:"requestId"`
	SentAt          string                    `json:"sentAt"`
	PayloadType     string                    `json:"payloadType"`
	Payload         precheckCompletionPayload `json:"payload"`
}

type precheckCompletionPayload struct {
	LeaseID       string                  `json:"leaseId"`
	LeaseEpoch    int64                   `json:"leaseEpoch"`
	BindingDigest string                  `json:"bindingDigest"`
	Results       []precheckResultPayload `json:"results"`
}

type precheckResultPayload struct {
	Check        agentpreflight.CheckID `json:"check"`
	Status       agentpreflight.Status  `json:"status"`
	EvidenceCode string                 `json:"evidenceCode"`
}

type precheckBindingPayload struct {
	PrecheckID         string `json:"precheckId"`
	NodeID             string `json:"nodeId"`
	DraftRevision      int64  `json:"draftRevision"`
	ConfigFingerprint  string `json:"configFingerprint"`
	CredentialRevision int64  `json:"credentialRevision"`
	NodeFactsVersion   int64  `json:"nodeFactsVersion"`
}

type precheckExecutionContextPayload struct {
	CompatibilityMode string   `json:"compatibilityMode"`
	Database          string   `json:"database"`
	Table             string   `json:"table"`
	OutputPath        string   `json:"outputPath"`
	LogPath           string   `json:"logPath"`
	SkipCheckDir      bool     `json:"skipCheckDir"`
	TargetPlatform    string   `json:"targetPlatform"`
	AllowedRoots      []string `json:"allowedRoots"`
}

type precheckClaimResponsePayload struct {
	PrecheckID           string                          `json:"precheckId"`
	LeaseID              string                          `json:"leaseId"`
	LeaseEpoch           int64                           `json:"leaseEpoch"`
	ExpiresAt            string                          `json:"expiresAt"`
	Binding              precheckBindingPayload          `json:"binding"`
	BindingDigest        string                          `json:"bindingDigest"`
	CheckSet             []agentpreflight.CheckID        `json:"checkSet"`
	ExecutionContext     precheckExecutionContextPayload `json:"executionContext"`
	RealExecutionEnabled *bool                           `json:"realExecutionEnabled"`
}

type precheckLeaseAcknowledgementResponsePayload struct {
	RealExecutionEnabled *bool `json:"realExecutionEnabled"`
}

type precheckSecretSlotResponsePayload struct {
	AgentRequestID string `json:"agentRequestId"`
	PrecheckID     string `json:"precheckId"`
	LeaseID        string `json:"leaseId"`
	LeaseEpoch     int64  `json:"leaseEpoch"`
	BindingDigest  string `json:"bindingDigest"`
	Slot           string `json:"slot"`
	Connection     struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username []byte `json:"username"`
		Password []byte `json:"password"`
	} `json:"connection"`
	RealExecutionEnabled *bool `json:"realExecutionEnabled"`
}

type precheckCompletionResponsePayload struct {
	Status               PrecheckState `json:"status"`
	RealExecutionEnabled *bool         `json:"realExecutionEnabled"`
}

// ClaimNextPrecheck 以已关联机器身份请求服务端原子领取下一条固定预检查。
// 待关联身份、非 HTTPS 状态或任一不可信响应均失败关闭；无工作返回 found=false，且不会在本地保存候选或租约。
func (s *StateStore) ClaimNextPrecheck(ctx context.Context, input PrecheckClaimNext) (PrecheckGrant, bool, error) {
	state, found, err := s.loadState()
	if err != nil {
		return PrecheckGrant{}, false, err
	}
	if !found {
		return PrecheckGrant{}, false, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return PrecheckGrant{}, false, ErrEnrollmentPending
	}
	if !validPrecheckClaimNext(input) {
		return PrecheckGrant{}, false, ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return PrecheckGrant{}, false, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return PrecheckGrant{}, false, ErrIdentityUnavailable
	}
	return client.claimNextPrecheck(ctx, &state, requestID, input)
}

// AcknowledgePrecheckLease 确认控制面签发的固定检查集、绑定摘要和短租约。
// Agent 不在此路径请求秘密或执行任何检查，确认失败时不得继续完成预检查。
func (s *StateStore) AcknowledgePrecheckLease(ctx context.Context, input PrecheckLeaseAcknowledgement) error {
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
	if !validPrecheckLeaseAcknowledgement(input) {
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
	return client.acknowledgePrecheckLease(ctx, &state, requestID, input)
}

// ResolvePrecheckDatabaseConnection 在有效、已确认租约内取得唯一数据库连接槽位。
// 同一次网络重试复用 requestId，但 StateStore 不缓存也不持久化响应中的用户名或密码。
func (s *StateStore) ResolvePrecheckDatabaseConnection(ctx context.Context, input PrecheckSecretSlotRequest) (DatabaseConnectionSlot, error) {
	state, found, err := s.loadState()
	if err != nil {
		return DatabaseConnectionSlot{}, err
	}
	if !found {
		return DatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return DatabaseConnectionSlot{}, ErrEnrollmentPending
	}
	if !validPrecheckSecretSlotRequest(input) {
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return DatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return DatabaseConnectionSlot{}, ErrIdentityUnavailable
	}
	return client.resolvePrecheckDatabaseConnection(ctx, &state, requestID, input)
}

// CompletePrecheck 上报一个已确认租约的完整固定检查报告。
// 它不会传输成功布尔值、命令、SQL、路径或秘密，控制面只可从结构化结果推导最终状态。
func (s *StateStore) CompletePrecheck(ctx context.Context, input PrecheckCompletion) (PrecheckState, error) {
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
	if !validPrecheckCompletion(input) {
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
	return client.completePrecheck(ctx, &state, requestID, input)
}

func (c *httpsClient) claimNextPrecheck(ctx context.Context, state *identityState, requestID string, input PrecheckClaimNext) (PrecheckGrant, bool, error) {
	request := precheckClaimNextRequest{
		ProtocolVersion: state.ProtocolVersion,
		AgentID:         state.AgentID,
		NodeID:          state.NodeID,
		BootID:          input.BootID,
		RequestID:       requestID,
		SentAt:          input.SentAt.UTC().Format(time.RFC3339Nano),
		PayloadType:     "EXPORT_PREFLIGHT_CLAIM_NEXT",
		Payload:         precheckClaimNextPayload{Capability: string(agentpreflight.CapabilityExportPreflight)},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return PrecheckGrant{}, false, ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	responseBody, status, err := c.postWithRetry(ctx, "/agent/v1/prechecks:claim-next", requestBody, state.MachineCredential)
	if err != nil {
		return PrecheckGrant{}, false, err
	}
	defer credential.Zero(responseBody)
	if status == http.StatusNoContent {
		if len(responseBody) != 0 {
			return PrecheckGrant{}, false, ErrProtocolRejected
		}
		return PrecheckGrant{}, false, nil
	}
	if status != http.StatusOK {
		return PrecheckGrant{}, false, precheckHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "PRECHECK_CLAIMED")
	if err != nil {
		return PrecheckGrant{}, false, ErrProtocolRejected
	}
	var payload precheckClaimResponsePayload
	// 真实执行开关必须显式返回以防协议降级，但不会改变固定预检查的安全边界；预检查仍不启动 OBDUMPER。
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.RealExecutionEnabled == nil {
		return PrecheckGrant{}, false, ErrProtocolRejected
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	if err != nil {
		return PrecheckGrant{}, false, ErrProtocolRejected
	}
	grant := PrecheckGrant{
		PrecheckID:    payload.PrecheckID,
		LeaseID:       payload.LeaseID,
		LeaseEpoch:    payload.LeaseEpoch,
		ExpiresAt:     expiresAt,
		Binding:       payload.Binding.toBinding(),
		BindingDigest: payload.BindingDigest,
		CheckSet:      append([]agentpreflight.CheckID(nil), payload.CheckSet...),
		Context:       payload.ExecutionContext.toContext(),
	}
	if !validPrecheckGrant(grant, state.NodeID) {
		return PrecheckGrant{}, false, ErrProtocolRejected
	}
	return grant, true, nil
}

func (c *httpsClient) resolvePrecheckDatabaseConnection(ctx context.Context, state *identityState, requestID string, input PrecheckSecretSlotRequest) (DatabaseConnectionSlot, error) {
	request := precheckSecretSlotRequest{
		ProtocolVersion: state.ProtocolVersion,
		AgentID:         state.AgentID,
		NodeID:          state.NodeID,
		BootID:          input.BootID,
		RequestID:       requestID,
		SentAt:          input.SentAt.UTC().Format(time.RFC3339Nano),
		PayloadType:     "EXPORT_PREFLIGHT_RESOLVE_SECRET_SLOTS",
		Payload: precheckSecretSlotRequestPayload{
			LeaseID:       input.LeaseID,
			LeaseEpoch:    input.LeaseEpoch,
			BindingDigest: input.BindingDigest,
			Slot:          "DATABASE_CONNECTION",
		},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	endpoint := "/agent/v1/prechecks/" + input.PrecheckID + "/secret-slots:resolve"
	responseBody, status, err := c.postWithRetry(ctx, endpoint, requestBody, state.MachineCredential)
	if err != nil {
		return DatabaseConnectionSlot{}, err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		return DatabaseConnectionSlot{}, precheckHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "PRECHECK_SECRET_SLOTS_RESOLVED")
	if err != nil {
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	var payload precheckSecretSlotResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.AgentRequestID != requestID || payload.PrecheckID != input.PrecheckID ||
		payload.LeaseID != input.LeaseID || payload.LeaseEpoch != input.LeaseEpoch || payload.BindingDigest != input.BindingDigest ||
		payload.Slot != "DATABASE_CONNECTION" || payload.RealExecutionEnabled == nil {
		payload.destroy()
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	connection := DatabaseConnectionSlot{
		Host:     payload.Connection.Host,
		Port:     payload.Connection.Port,
		Username: payload.Connection.Username,
		Password: payload.Connection.Password,
	}
	payload.Connection.Username = nil
	payload.Connection.Password = nil
	if !validDatabaseConnectionSlot(connection) {
		connection.Destroy()
		return DatabaseConnectionSlot{}, ErrProtocolRejected
	}
	return connection, nil
}

func (c *httpsClient) acknowledgePrecheckLease(ctx context.Context, state *identityState, requestID string, input PrecheckLeaseAcknowledgement) error {
	request := precheckLeaseAcknowledgementRequest{
		ProtocolVersion: state.ProtocolVersion,
		AgentID:         state.AgentID,
		NodeID:          state.NodeID,
		BootID:          input.BootID,
		RequestID:       requestID,
		SentAt:          input.SentAt.UTC().Format(time.RFC3339Nano),
		PayloadType:     "EXPORT_PREFLIGHT_ACKNOWLEDGE_LEASE",
		Payload: precheckLeaseAcknowledgementPayload{
			LeaseID:       input.LeaseID,
			LeaseEpoch:    input.LeaseEpoch,
			BindingDigest: input.BindingDigest,
		},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	endpoint := "/agent/v1/prechecks/" + input.PrecheckID + ":acknowledge-lease"
	responseBody, status, err := c.postWithRetry(ctx, endpoint, requestBody, state.MachineCredential)
	if err != nil {
		return err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		return precheckHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "PRECHECK_LEASE_ACKNOWLEDGED")
	if err != nil {
		return ErrProtocolRejected
	}
	var payload precheckLeaseAcknowledgementResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.RealExecutionEnabled == nil {
		return ErrProtocolRejected
	}
	return nil
}

func (c *httpsClient) completePrecheck(ctx context.Context, state *identityState, requestID string, input PrecheckCompletion) (PrecheckState, error) {
	request := precheckCompletionRequest{
		ProtocolVersion: state.ProtocolVersion,
		AgentID:         state.AgentID,
		NodeID:          state.NodeID,
		BootID:          input.BootID,
		RequestID:       requestID,
		SentAt:          input.SentAt.UTC().Format(time.RFC3339Nano),
		PayloadType:     "EXPORT_PREFLIGHT_COMPLETE",
		Payload: precheckCompletionPayload{
			LeaseID:       input.LeaseID,
			LeaseEpoch:    input.LeaseEpoch,
			BindingDigest: input.BindingDigest,
			Results:       precheckResultPayloads(input.Report.Results),
		},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	endpoint := "/agent/v1/prechecks/" + input.PrecheckID + ":complete"
	responseBody, status, err := c.postWithRetry(ctx, endpoint, requestBody, state.MachineCredential)
	if err != nil {
		return "", err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		return "", precheckHTTPError(status)
	}
	envelope, err := decodeResponseEnvelope(responseBody, "PRECHECK_COMPLETED")
	if err != nil {
		return "", ErrProtocolRejected
	}
	var payload precheckCompletionResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.RealExecutionEnabled == nil || !validPrecheckState(payload.Status) {
		return "", ErrProtocolRejected
	}
	if (payload.Status == PrecheckSucceeded) != input.Report.Succeeded {
		return "", ErrProtocolRejected
	}
	return payload.Status, nil
}

func (p precheckBindingPayload) toBinding() agentstate.PrecheckBinding {
	return agentstate.PrecheckBinding{
		PrecheckID:         p.PrecheckID,
		NodeID:             p.NodeID,
		DraftRevision:      p.DraftRevision,
		ConfigFingerprint:  p.ConfigFingerprint,
		CredentialRevision: p.CredentialRevision,
		NodeFactsVersion:   p.NodeFactsVersion,
	}
}

func (p precheckExecutionContextPayload) toContext() PrecheckExecutionContext {
	return PrecheckExecutionContext{
		CompatibilityMode: p.CompatibilityMode,
		Database:          p.Database,
		Table:             p.Table,
		OutputPath:        p.OutputPath,
		LogPath:           p.LogPath,
		SkipCheckDir:      p.SkipCheckDir,
		TargetPlatform:    commandgen.Platform(p.TargetPlatform),
		AllowedRoots:      append([]string(nil), p.AllowedRoots...),
	}
}

func (p *precheckSecretSlotResponsePayload) destroy() {
	if p == nil {
		return
	}
	credential.Zero(p.Connection.Username)
	credential.Zero(p.Connection.Password)
	p.Connection.Username = nil
	p.Connection.Password = nil
}

func precheckResultPayloads(results []agentpreflight.Result) []precheckResultPayload {
	payloads := make([]precheckResultPayload, 0, len(results))
	for _, result := range results {
		payloads = append(payloads, precheckResultPayload{Check: result.Check, Status: result.Status, EvidenceCode: result.EvidenceCode})
	}
	return payloads
}

func validPrecheckClaimNext(input PrecheckClaimNext) bool {
	return validOpaqueValue(input.BootID, 256) && !input.SentAt.IsZero()
}

func validPrecheckLeaseAcknowledgement(input PrecheckLeaseAcknowledgement) bool {
	return validOpaqueValue(input.BootID, 256) && validPrecheckPathID(input.PrecheckID) && validOpaqueValue(input.LeaseID, 256) && input.LeaseEpoch > 0 && validOpaqueValue(input.BindingDigest, 256) && !input.SentAt.IsZero()
}

func validPrecheckSecretSlotRequest(input PrecheckSecretSlotRequest) bool {
	return validOpaqueValue(input.BootID, 256) && validPrecheckPathID(input.PrecheckID) && validOpaqueValue(input.LeaseID, 256) &&
		input.LeaseEpoch > 0 && validOpaqueValue(input.BindingDigest, 256) && !input.SentAt.IsZero()
}

func validPrecheckCompletion(input PrecheckCompletion) bool {
	if !validOpaqueValue(input.BootID, 256) || !validPrecheckPathID(input.PrecheckID) || !validOpaqueValue(input.LeaseID, 256) || input.LeaseEpoch < 1 || !validOpaqueValue(input.BindingDigest, 256) || input.SentAt.IsZero() || input.Report.PrecheckID != input.PrecheckID {
		return false
	}
	return agentpreflight.ValidateReport(input.Report) == nil
}

func validPrecheckGrant(grant PrecheckGrant, nodeID string) bool {
	if !validOpaqueValue(grant.PrecheckID, 256) || !validOpaqueValue(grant.LeaseID, 256) || grant.LeaseEpoch < 1 || grant.ExpiresAt.IsZero() || !validOpaqueValue(grant.BindingDigest, 256) || !validPrecheckBinding(grant.Binding) || !validPrecheckCheckSet(grant.CheckSet) {
		return false
	}
	return grant.Binding.PrecheckID == grant.PrecheckID && grant.Binding.NodeID == nodeID && validPrecheckExecutionContext(grant.Context)
}

func validPrecheckExecutionContext(context PrecheckExecutionContext) bool {
	if (context.CompatibilityMode != "MYSQL" && context.CompatibilityMode != "ORACLE") || !validOpaqueValue(context.Database, 256) || !validOpaqueValue(context.Table, 256) || !validExportOutputPath(context.TargetPlatform, context.OutputPath) || (context.LogPath != "" && !validExportOutputPath(context.TargetPlatform, context.LogPath)) || len(context.AllowedRoots) == 0 || len(context.AllowedRoots) > 32 {
		return false
	}
	for _, root := range context.AllowedRoots {
		if !outputpath.IsAllowedRootPath(string(context.TargetPlatform), root) {
			return false
		}
	}
	return true
}

// validExportOutputPath 只接受控制面已声明目标平台可由 OBDUMPER 消费的导出目录。
// Windows 命令路径必须保持 /E:/ 形式，不能在 Agent 协议中退化为反斜杠格式。
func validExportOutputPath(platform commandgen.Platform, value string) bool {
	return outputpath.IsExportOutputPath(string(platform), value)
}

func validDatabaseConnectionSlot(connection DatabaseConnectionSlot) bool {
	if connection.Port < 1 || connection.Port > 65535 || len(connection.Host) == 0 || len(connection.Host) > 253 ||
		len(connection.Username) == 0 || len(connection.Username) > 256 || len(connection.Password) == 0 || len(connection.Password) > 4096 {
		return false
	}
	for _, value := range []byte(connection.Host) {
		if !(value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '.' || value == '-') {
			return false
		}
	}
	return !containsForbiddenSecretByte(connection.Username) && !containsForbiddenSecretByte(connection.Password)
}

func containsForbiddenSecretByte(value []byte) bool {
	for _, item := range value {
		if item == '\r' || item == '\n' || item == 0 {
			return true
		}
	}
	return false
}

func validPrecheckBinding(binding agentstate.PrecheckBinding) bool {
	return validOpaqueValue(binding.PrecheckID, 256) && validOpaqueValue(binding.NodeID, 256) && binding.DraftRevision > 0 && validOpaqueValue(binding.ConfigFingerprint, 256) && binding.CredentialRevision > 0 && binding.NodeFactsVersion > 0
}

func validPrecheckCheckSet(checkSet []agentpreflight.CheckID) bool {
	fixedChecks := agentpreflight.FixedChecks()
	if len(checkSet) != len(fixedChecks) {
		return false
	}
	for index, check := range fixedChecks {
		if checkSet[index] != check {
			return false
		}
	}
	return true
}

func validPrecheckState(state PrecheckState) bool {
	return state == PrecheckSucceeded || state == PrecheckFailed
}

func validPrecheckPathID(value string) bool {
	return validOpaqueValue(value, 256) && !strings.ContainsAny(value, "/\\?#")
}

func precheckHTTPError(status int) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ErrAgentAuthenticationDenied
	}
	return ErrProtocolRejected
}
