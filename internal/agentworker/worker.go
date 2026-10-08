// Package agentworker 提供固定 EXPORT_PREFLIGHT 的单次受控编排。
// 它不发现工作、不保存秘密、不启动 OBDUMPER；生产发现和运行时组装必须在单独审查后接入。
package agentworker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/agentjdbc"
	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/outputpath"
)

var (
	// ErrInvalidConfiguration 表示 Worker 依赖或本机身份信息不完整，不能领取预检查。
	ErrInvalidConfiguration = errors.New("预检查 Worker 配置无效")
	// ErrWorkerBusy 表示同一 Agent 进程已有尚未结束的预检查，首版不并行领取。
	ErrWorkerBusy = errors.New("预检查 Worker 正在运行")
	// ErrGrantRejected 表示控制面返回的租约或固定检查上下文不一致。
	ErrGrantRejected = errors.New("预检查租约无效")
	// ErrLeaseExpired 表示本机在继续操作前已观测到短租约截止，不能继续解析或上报。
	ErrLeaseExpired = errors.New("预检查租约已过期")
	// ErrLeaseAcknowledgement 表示确认租约失败，Worker 不会继续运行任何 Probe。
	ErrLeaseAcknowledgement = errors.New("预检查租约确认失败")
	// ErrProbeUnavailable 表示固定检查没有得到可验证的完整报告。
	ErrProbeUnavailable = errors.New("预检查 Probe 不可用")
	// ErrCompletionRejected 表示完整报告未被控制面确认，Worker 不会重跑该租约。
	ErrCompletionRejected = errors.New("预检查结果上报失败")
	// ErrSecretResolution 表示槽位解析上下文无效、过期或控制面拒绝解析。
	ErrSecretResolution = errors.New("预检查秘密槽位不可用")
)

// Protocol 收窄 Worker 可调用的 Agent 协议，避免本地编排获得任意命令或任务执行能力。
type Protocol interface {
	AgentIdentity() (agentwire.AgentIdentity, error)
	ClaimNextPrecheck(context.Context, agentwire.PrecheckClaimNext) (agentwire.PrecheckGrant, bool, error)
	AcknowledgePrecheckLease(context.Context, agentwire.PrecheckLeaseAcknowledgement) error
	ResolvePrecheckDatabaseConnection(context.Context, agentwire.PrecheckSecretSlotRequest) (agentwire.DatabaseConnectionSlot, error)
	ResolvePrecheckStorageCredential(context.Context, agentwire.PrecheckSecretSlotRequest) (agentwire.StorageCredentialSlot, error)
	CompletePrecheck(context.Context, agentwire.PrecheckCompletion) (agentwire.PrecheckState, error)
}

// SecretResolver 将当前已确认租约约束为 JDBC 探针可使用的窄槽位读取能力。
// Probe 不可借此请求其他预检查、租约或数据源的秘密。
type SecretResolver interface {
	ResolveDatabaseConnection(context.Context, agentstate.PrecheckBinding) (agentjdbc.Connection, error)
	ResolveStorageCredential(context.Context, agentstate.PrecheckBinding) (agentpreflight.StorageCredential, error)
}

// ProbeFactory 在租约确认后才创建固定检查 Probe。
// 工厂收到的槽位解析器初始关闭，只有内部包装器进入数据库检查时才会放行，不能缓存或跨租约复用。
type ProbeFactory func(SecretResolver) agentpreflight.Probe

// Outcome 是一次已被控制面确认的固定预检查结果。
// Report 只包含固定检查状态和稳定证据码，不含对象、路径、连接或秘密。
type Outcome struct {
	State  agentwire.PrecheckState
	Report agentpreflight.Report
}

// Worker 串行执行一个受控的 EXPORT_PREFLIGHT 租约。
// Agent 与节点标识只能从已关联的受保护身份状态读取，调用方不能通过本地配置覆盖。
type Worker struct {
	Protocol      Protocol
	ProbeFactory  ProbeFactory
	Clock         func() time.Time
	BootID        string
	LocalPlatform commandgen.Platform

	mu      sync.Mutex
	running bool
}

// RunNext 按 claim-next、acknowledge、固定 Probe、complete 的唯一顺序运行一条预检查。
// 没有工作时返回 found=false；任何校验、确认、租约、Probe 或上报异常都会停止后续动作，不自动重试或重新领取。
func (w *Worker) RunNext(ctx context.Context) (Outcome, bool, error) {
	if !w.enter() {
		return Outcome{}, false, ErrWorkerBusy
	}
	defer w.leave()
	if !w.validConfiguration() {
		return Outcome{}, false, ErrInvalidConfiguration
	}
	identity, err := w.Protocol.AgentIdentity()
	if err != nil || !validOpaque(identity.AgentID, 256) || !validOpaque(identity.NodeID, 256) {
		return Outcome{}, false, ErrInvalidConfiguration
	}

	grant, found, err := w.Protocol.ClaimNextPrecheck(ctx, agentwire.PrecheckClaimNext{
		BootID: w.BootID,
		SentAt: w.now(),
	})
	if err != nil {
		return Outcome{}, false, ErrGrantRejected
	}
	if !found {
		return Outcome{}, false, nil
	}
	grant = cloneGrant(grant)
	if !validGrant(identity, w.LocalPlatform, grant) || agentpreflight.ValidateRequest(requestForGrant(identity.AgentID, grant)) != nil {
		return Outcome{}, true, ErrGrantRejected
	}
	if w.expired(grant) {
		return Outcome{}, true, ErrLeaseExpired
	}

	if err := w.Protocol.AcknowledgePrecheckLease(ctx, acknowledgement(w.BootID, grant, w.now())); err != nil {
		return Outcome{}, true, ErrLeaseAcknowledgement
	}
	if w.expired(grant) {
		return Outcome{}, true, ErrLeaseExpired
	}

	resolver := &leaseSecretResolver{
		protocol: w.Protocol,
		clock:    w.Clock,
		bootID:   w.BootID,
		grant:    grant,
		active:   true,
	}
	// 槽位初始保持关闭，即使工厂错误地提前调用解析器也不会发起控制面请求。
	// 只有固定包装器进入相应检查的调用范围时才附加内部许可；resolver 分别限制数据库和存储槽位各解析一次，并会在 RunNext 返回时立即失效。
	defer resolver.close()
	probe := secretGatedProbe{delegate: w.ProbeFactory(resolver), resolver: resolver}
	request := requestForGrant(identity.AgentID, grant)
	report, err := agentpreflight.Run(ctx, request, probe)
	if err != nil || agentpreflight.ValidateReportFor(kindForGrant(grant), report) != nil {
		report = failedReport(kindForGrant(grant), grant.PrecheckID)
	}
	if w.expired(grant) {
		return Outcome{}, true, ErrLeaseExpired
	}

	state, err := w.Protocol.CompletePrecheck(ctx, completion(w.BootID, grant, report, w.now()))
	if err != nil || !expectedState(report, state) {
		return Outcome{}, true, ErrCompletionRejected
	}
	return Outcome{State: state, Report: report}, true, nil
}

func (w *Worker) enter() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return false
	}
	w.running = true
	return true
}

func (w *Worker) leave() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.running = false
}

func (w *Worker) validConfiguration() bool {
	return w.Protocol != nil && w.ProbeFactory != nil && w.Clock != nil && validOpaque(w.BootID, 256) && validPlatform(w.LocalPlatform)
}

func (w *Worker) now() time.Time {
	return w.Clock().UTC()
}

func (w *Worker) expired(grant agentwire.PrecheckGrant) bool {
	return !w.now().Before(grant.ExpiresAt)
}

func acknowledgement(bootID string, grant agentwire.PrecheckGrant, sentAt time.Time) agentwire.PrecheckLeaseAcknowledgement {
	return agentwire.PrecheckLeaseAcknowledgement{
		BootID:        bootID,
		PrecheckID:    grant.PrecheckID,
		LeaseID:       grant.LeaseID,
		LeaseEpoch:    grant.LeaseEpoch,
		BindingDigest: grant.BindingDigest,
		SentAt:        sentAt,
	}
}

func completion(bootID string, grant agentwire.PrecheckGrant, report agentpreflight.Report, sentAt time.Time) agentwire.PrecheckCompletion {
	return agentwire.PrecheckCompletion{
		BootID:        bootID,
		PrecheckID:    grant.PrecheckID,
		LeaseID:       grant.LeaseID,
		LeaseEpoch:    grant.LeaseEpoch,
		BindingDigest: grant.BindingDigest,
		SentAt:        sentAt,
		Report:        report,
	}
}

func requestForGrant(agentID string, grant agentwire.PrecheckGrant) agentpreflight.Request {
	request := agentpreflight.Request{
		Capability:        agentpreflight.CapabilityExportPreflight,
		PrecheckID:        grant.PrecheckID,
		NodeID:            grant.Binding.NodeID,
		AgentID:           agentID,
		LeaseID:           grant.LeaseID,
		LeaseEpoch:        grant.LeaseEpoch,
		Binding:           grant.Binding,
		CompatibilityMode: grant.Context.CompatibilityMode,
		Database:          grant.Context.Database,
		Objects:           append([]string(nil), grant.Context.Objects...),
		ObjectTypes:       append([]string(nil), grant.Context.ObjectTypes...),
		ContentKind:       grant.Context.ContentKind,
		TargetPlatform:    grant.Context.TargetPlatform,
		OutputPath:        grant.Context.OutputPath,
		LogPath:           grant.Context.LogPath,
		SkipCheckDir:      grant.Context.SkipCheckDir,
		AllowedRoots:      append([]string(nil), grant.Context.AllowedRoots...),
		OutputKind:        agentpreflight.OutputKind(grant.Context.OutputKind),
	}
	if grant.Context.StorageTarget != nil {
		request.StorageTarget = &agentpreflight.StorageTarget{
			Provider: grant.Context.StorageTarget.Provider, URI: grant.Context.StorageTarget.URI,
			Endpoint: grant.Context.StorageTarget.Endpoint, TmpPath: grant.Context.StorageTarget.TmpPath,
		}
	}
	return request
}

// kindForGrant 把租约上下文的输出类型归一化；缺省保持本地语义。
func kindForGrant(grant agentwire.PrecheckGrant) agentpreflight.OutputKind {
	if grant.Context.OutputKind == "" {
		return agentpreflight.OutputKindLocal
	}
	return agentpreflight.OutputKind(grant.Context.OutputKind)
}

func validGrant(identity agentwire.AgentIdentity, localPlatform commandgen.Platform, grant agentwire.PrecheckGrant) bool {
	if !validPathIdentifier(grant.PrecheckID) || !validOpaque(grant.LeaseID, 256) || grant.Binding.NodeID != identity.NodeID || grant.Context.TargetPlatform != localPlatform || grant.LeaseEpoch < 1 || grant.ExpiresAt.IsZero() ||
		!validOpaque(grant.BindingDigest, 256) || !validBinding(grant.Binding) || !checkSetForKind(kindForGrant(grant), grant.CheckSet) || !validExecutionContext(grant.Context) {
		return false
	}
	return grant.Binding.PrecheckID == grant.PrecheckID
}

func validBinding(binding agentstate.PrecheckBinding) bool {
	return validPathIdentifier(binding.PrecheckID) && validOpaque(binding.NodeID, 256) && binding.DraftRevision > 0 &&
		validOpaque(binding.ConfigFingerprint, 256) && binding.CredentialRevision > 0 && binding.NodeFactsVersion > 0
}

// checkSetForKind 校验控制面下发的检查清单与输出类型形态一致（本地六项或存储六项）。
func checkSetForKind(kind agentpreflight.OutputKind, checkSet []agentpreflight.CheckID) bool {
	expected := agentpreflight.ChecksForOutputKind(kind)
	if len(checkSet) != len(expected) {
		return false
	}
	for index, check := range expected {
		if checkSet[index] != check {
			return false
		}
	}
	return true
}

func validExecutionContext(executionContext agentwire.PrecheckExecutionContext) bool {
	if (executionContext.CompatibilityMode != "MYSQL" && executionContext.CompatibilityMode != "ORACLE") || !validOpaque(executionContext.Database, 256) || (executionContext.ContentKind != "DATA_ONLY" && executionContext.ContentKind != "DDL_ONLY" && executionContext.ContentKind != "DDL_AND_DATA") || len(executionContext.AllowedRoots) == 0 || len(executionContext.AllowedRoots) > 32 {
		return false
	}
	// EX-I6：缺省输出类型保持本地语义；对象存储输出必须携带受控存储目标段。
	kind := executionContext.OutputKind
	if kind == "" {
		kind = agentpreflight.OutputKindLocal
	}
	if kind.IsStorageOutput() {
		if executionContext.StorageTarget == nil || executionContext.StorageTarget.Provider != string(kind) ||
			!validOpaque(executionContext.OutputPath, 4096) || executionContext.LogPath != "" {
			return false
		}
		if executionContext.StorageTarget.Endpoint != "" && !validOpaque(executionContext.StorageTarget.Endpoint, 253) {
			return false
		}
		if executionContext.StorageTarget.TmpPath != "" && !validExportOutputPath(executionContext.TargetPlatform, executionContext.StorageTarget.TmpPath) {
			return false
		}
	} else {
		if executionContext.StorageTarget != nil || !validExportOutputPath(executionContext.TargetPlatform, executionContext.OutputPath) || (executionContext.LogPath != "" && !validExportOutputPath(executionContext.TargetPlatform, executionContext.LogPath)) {
			return false
		}
	}
	for _, object := range executionContext.Objects {
		if !validOpaque(object, 256) {
			return false
		}
	}
	for _, root := range executionContext.AllowedRoots {
		if !outputpath.IsAllowedRootPath(string(executionContext.TargetPlatform), root) {
			return false
		}
	}
	return true
}

func validExportOutputPath(platform commandgen.Platform, value string) bool {
	return outputpath.IsExportOutputPath(string(platform), value)
}

func validPlatform(platform commandgen.Platform) bool {
	return platform == commandgen.PlatformWindowsAMD64 || platform == commandgen.PlatformLinuxAMD64 || platform == commandgen.PlatformLinuxARM64
}

func validOpaque(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) && !strings.ContainsRune(value, 0) && !strings.ContainsAny(value, "\r\n")
}

func validPathIdentifier(value string) bool {
	return validOpaque(value, 256) && !strings.ContainsAny(value, "/\\?#")
}

func expectedState(report agentpreflight.Report, state agentwire.PrecheckState) bool {
	if report.Succeeded {
		return state == agentwire.PrecheckSucceeded
	}
	return state == agentwire.PrecheckFailed
}

func cloneGrant(grant agentwire.PrecheckGrant) agentwire.PrecheckGrant {
	grant.CheckSet = append([]agentpreflight.CheckID(nil), grant.CheckSet...)
	grant.Context.AllowedRoots = append([]string(nil), grant.Context.AllowedRoots...)
	return grant
}

// leaseSecretResolver 将一次可用的预检查租约封装为不可复用的槽位解析器。
// 即使 Probe 保留接口引用，RunNext 返回后 active 也会关闭，从而拒绝延迟解析。
type leaseSecretResolver struct {
	protocol Protocol
	clock    func() time.Time
	bootID   string
	grant    agentwire.PrecheckGrant

	mu           sync.Mutex
	active       bool
	permit       *secretResolvePermit
	databaseUsed bool
	storageUsed  bool
}

// secretResolvePermit 只由 Worker 在固定秘密相关检查的同步调用范围内创建。
// ProbeFactory 没有此令牌，因此不能在本机前置检查前或其后自行解析槽位。
type secretResolvePermit struct {
	check agentpreflight.CheckID
}

type secretResolvePermitContextKey struct{}

// secretGatedProbe 只在固定数据库或存储认证检查开始时放行本次租约的对应槽位解析器。
// 本机前置检查、对象检查和存储连通性检查不能借由保留的解析器提前触发控制面秘密操作。
type secretGatedProbe struct {
	delegate agentpreflight.Probe
	resolver *leaseSecretResolver
}

func (p secretGatedProbe) Probe(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	if ctx == nil || p.delegate == nil || p.resolver == nil {
		return agentpreflight.Result{}, agentpreflight.ErrInvalidRequest
	}
	if check == agentpreflight.CheckDatabaseConnectivity || check == agentpreflight.CheckStorageAuth {
		permit := &secretResolvePermit{check: check}
		p.resolver.allow(permit)
		defer p.resolver.disallow(permit)
		ctx = context.WithValue(ctx, secretResolvePermitContextKey{}, permit)
	}
	return p.delegate.Probe(ctx, check, request)
}

func (r *leaseSecretResolver) allow(permit *secretResolvePermit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active && permit != nil {
		r.permit = permit
	}
}

func (r *leaseSecretResolver) disallow(permit *secretResolvePermit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.permit == permit {
		r.permit = nil
	}
}

func (r *leaseSecretResolver) ResolveDatabaseConnection(ctx context.Context, binding agentstate.PrecheckBinding) (agentjdbc.Connection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx == nil {
		return agentjdbc.Connection{}, ErrSecretResolution
	}
	permit, _ := ctx.Value(secretResolvePermitContextKey{}).(*secretResolvePermit)
	if !r.active || permit == nil || r.permit != permit || permit.check != agentpreflight.CheckDatabaseConnectivity || r.databaseUsed || !sameBinding(binding, r.grant.Binding) || !r.clock().UTC().Before(r.grant.ExpiresAt) {
		return agentjdbc.Connection{}, ErrSecretResolution
	}
	// 在发起网络请求前先占用唯一槽位，失败也不允许重试解析，避免扩大秘密暴露次数。
	r.databaseUsed = true
	slot, err := r.protocol.ResolvePrecheckDatabaseConnection(ctx, agentwire.PrecheckSecretSlotRequest{
		BootID:        r.bootID,
		PrecheckID:    r.grant.PrecheckID,
		LeaseID:       r.grant.LeaseID,
		LeaseEpoch:    r.grant.LeaseEpoch,
		BindingDigest: r.grant.BindingDigest,
		SentAt:        r.clock().UTC(),
	})
	if err != nil || !validConnectionSlot(slot) {
		slot.Destroy()
		return agentjdbc.Connection{}, ErrSecretResolution
	}
	connection := agentjdbc.Connection{
		Host:     slot.Host,
		Port:     slot.Port,
		Username: slot.Username,
		Password: slot.Password,
	}
	// 所有权转移给 JDBC Probe；slot.Destroy 不能在这里清零同一底层字节切片。
	slot.Username = nil
	slot.Password = nil
	return connection, nil
}

// ResolveStorageCredential 只在 STORAGE_AUTH 的同步调用范围内解析一次当前预检查冻结的存储凭据。
// 存储连通性、数据库和对象检查均不能调用它；返回的字节所有权转移给认证探测器，并由其立即清零。
func (r *leaseSecretResolver) ResolveStorageCredential(ctx context.Context, binding agentstate.PrecheckBinding) (agentpreflight.StorageCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx == nil {
		return agentpreflight.StorageCredential{}, ErrSecretResolution
	}
	permit, _ := ctx.Value(secretResolvePermitContextKey{}).(*secretResolvePermit)
	if !r.active || permit == nil || r.permit != permit || permit.check != agentpreflight.CheckStorageAuth || r.storageUsed ||
		!r.grant.Context.OutputKind.IsStorageOutput() || r.grant.Context.StorageTarget == nil || !sameBinding(binding, r.grant.Binding) || !r.clock().UTC().Before(r.grant.ExpiresAt) {
		return agentpreflight.StorageCredential{}, ErrSecretResolution
	}
	// 在发起网络请求前先占用唯一存储槽位，失败也不重复解析，避免扩大密钥暴露次数。
	r.storageUsed = true
	slot, err := r.protocol.ResolvePrecheckStorageCredential(ctx, agentwire.PrecheckSecretSlotRequest{
		BootID:        r.bootID,
		PrecheckID:    r.grant.PrecheckID,
		LeaseID:       r.grant.LeaseID,
		LeaseEpoch:    r.grant.LeaseEpoch,
		BindingDigest: r.grant.BindingDigest,
		SentAt:        r.clock().UTC(),
	})
	if err != nil || !validStorageCredentialSlot(slot) || slot.Provider != r.grant.Context.StorageTarget.Provider {
		slot.Destroy()
		return agentpreflight.StorageCredential{}, ErrSecretResolution
	}
	storageCredential := agentpreflight.StorageCredential{
		Provider:  slot.Provider,
		AccessKey: slot.AccessKey,
		SecretKey: slot.SecretKey,
	}
	// 所有权转移给 STORAGE_AUTH 探测器；slot.Destroy 不能清零同一底层字节切片。
	slot.AccessKey = nil
	slot.SecretKey = nil
	return storageCredential, nil
}

func (r *leaseSecretResolver) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = false
}

func sameBinding(left, right agentstate.PrecheckBinding) bool {
	return left.PrecheckID == right.PrecheckID && left.NodeID == right.NodeID && left.DraftRevision == right.DraftRevision &&
		left.ConfigFingerprint == right.ConfigFingerprint && left.CredentialRevision == right.CredentialRevision && left.NodeFactsVersion == right.NodeFactsVersion
}

func validConnectionSlot(slot agentwire.DatabaseConnectionSlot) bool {
	if slot.Port < 1 || slot.Port > 65535 || len(slot.Host) == 0 || len(slot.Host) > 253 || len(slot.Username) == 0 || len(slot.Username) > 256 || len(slot.Password) == 0 || len(slot.Password) > 4096 {
		return false
	}
	for _, value := range []byte(slot.Host) {
		if !(value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '.' || value == '-') {
			return false
		}
	}
	return !containsForbiddenSecretByte(slot.Username) && !containsForbiddenSecretByte(slot.Password)
}

// validStorageCredentialSlot 只接受已知提供方与不含控制字符的完整密钥对。
// Worker 在交给本机认证探测器前复核一次，避免协议层校验漂移时扩大秘密消费范围。
func validStorageCredentialSlot(slot agentwire.StorageCredentialSlot) bool {
	if slot.Provider != "OSS" && slot.Provider != "S3" && slot.Provider != "COS" && slot.Provider != "OBS" ||
		len(slot.AccessKey) == 0 || len(slot.AccessKey) > 4096 || len(slot.SecretKey) == 0 || len(slot.SecretKey) > 4096 {
		return false
	}
	return !containsForbiddenSecretByte(slot.AccessKey) && !containsForbiddenSecretByte(slot.SecretKey)
}

func containsForbiddenSecretByte(value []byte) bool {
	for _, item := range value {
		if item == '\r' || item == '\n' || item == 0 {
			return true
		}
	}
	return false
}

func failedReport(kind agentpreflight.OutputKind, precheckID string) agentpreflight.Report {
	results := make([]agentpreflight.Result, 0, len(agentpreflight.ChecksForOutputKind(kind)))
	for _, check := range agentpreflight.ChecksForOutputKind(kind) {
		results = append(results, agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: failureEvidenceCode(check)})
	}
	return agentpreflight.Report{PrecheckID: precheckID, Results: results}
}

func failureEvidenceCode(check agentpreflight.CheckID) string {
	switch check {
	case agentpreflight.CheckDatabaseConnectivity:
		return "DATABASE_CONNECTION_UNAVAILABLE"
	case agentpreflight.CheckObjectAccess:
		return "OBJECT_ACCESS_UNAVAILABLE"
	case agentpreflight.CheckToolEnvironment:
		return "TOOL_RUNTIME_UNAVAILABLE"
	case agentpreflight.CheckOutputPath, agentpreflight.CheckOutputEmpty:
		return "OUTPUT_PATH_UNAVAILABLE"
	case agentpreflight.CheckAvailableSpace:
		return "OUTPUT_SPACE_UNAVAILABLE"
	case agentpreflight.CheckStorageConnectivity:
		return "STORAGE_CONNECTIVITY_UNAVAILABLE"
	case agentpreflight.CheckStorageAuth:
		return "STORAGE_AUTH_UNAVAILABLE"
	default:
		return "SYNTHETIC_OK"
	}
}
