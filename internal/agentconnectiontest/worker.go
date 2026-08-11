// Package agentconnectiontest 提供节点侧基础连接测试的受控编排。
// 它与导出预检查隔离，不检查对象、工具、路径或空间，也不提供任意命令、SQL 或文件访问能力。
package agentconnectiontest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/agentwire"
)

var (
	// ErrInvalidConfiguration 表示 Worker 的本机依赖或受保护身份信息不完整。
	ErrInvalidConfiguration = errors.New("基础连接测试 Worker 配置无效")
	// ErrWorkerBusy 表示同一 Agent 已在处理一条基础连接测试，G2 不允许并行领取。
	ErrWorkerBusy = errors.New("基础连接测试 Worker 正在运行")
	// ErrGrantRejected 表示控制面返回的租约、冻结绑定或验证来源不符合当前 Worker 的受控范围。
	ErrGrantRejected = errors.New("基础连接测试租约无效")
	// ErrLeaseExpired 表示本机在继续操作前观测到租约已截止。
	ErrLeaseExpired = errors.New("基础连接测试租约已过期")
	// ErrLeaseAcknowledgement 表示确认租约失败，Worker 不会继续回写结果。
	ErrLeaseAcknowledgement = errors.New("基础连接测试租约确认失败")
	// ErrCompletionRejected 表示控制面未确认固定终态，Worker 不会重跑同一租约。
	ErrCompletionRejected = errors.New("基础连接测试结果上报失败")
)

// Protocol 收窄基础连接测试 Worker 可调用的机器协议。
// JDBC 专用槽位解析只由显式注入的 JDBCRunner 在已确认租约后使用，不能混入领取或心跳路径。
type Protocol interface {
	AgentIdentity() (agentwire.AgentIdentity, error)
	ClaimNextDataSourceConnectionTest(context.Context, agentwire.DataSourceConnectionTestClaimNext) (agentwire.DataSourceConnectionTestGrant, bool, error)
	AcknowledgeDataSourceConnectionTestLease(context.Context, agentwire.DataSourceConnectionTestLeaseAcknowledgement) error
	CompleteDataSourceConnectionTest(context.Context, agentwire.DataSourceConnectionTestCompletion) (agentwire.DataSourceConnectionTestStatus, error)
}

// JDBCRunner 只处理已经确认的 AGENT_JDBC 租约。
// 实现必须使用固定 JDBC 探针和短时数据库槽位，且只能返回三种受控终态。
type JDBCRunner interface {
	RunConnectionTest(context.Context, agentwire.DataSourceConnectionTestGrant) Outcome
}

// Outcome 是控制面已确认的基础连接测试终态安全投影。
// 只有 AGENT_JDBC 的 DATABASE_CONNECTED 才表示固定节点已建立 JDBC 连接；它仍不代表导出可执行。
// Sys 字段只表达可选的 sys 凭据验证事实（参考 ODC 的 sys 账号验证），与数据库结果相互独立。
type Outcome struct {
	ConnectionTestID      string
	Status                agentwire.DataSourceConnectionTestStatus
	EvidenceCode          string
	VerificationSource    agentwire.DataSourceConnectionTestVerificationSource
	SysVerificationStatus agentwire.DataSourceConnectionTestSysVerificationStatus
	SysEvidenceCode       string
}

// Worker 串行处理一条由当前已认证 Agent 领取的基础连接测试租约。
// 默认只有 G2 合成租约可运行；显式配置 JDBCRunner 后才允许在确认租约后解析一次短时槽位并调用固定探针。
type Worker struct {
	Protocol   Protocol
	JDBCRunner JDBCRunner
	Clock      func() time.Time
	BootID     string

	mu      sync.Mutex
	running bool
}

// RunNext 按 claim-next、acknowledge、固定结果、complete 的唯一顺序处理一条基础连接测试。
// 无工作时 found 为 false；租约、身份、确认或终态不可信时停止后续操作，不自动重试或重新领取。
func (w *Worker) RunNext(ctx context.Context) (Outcome, bool, error) {
	if w == nil {
		return Outcome{}, false, ErrInvalidConfiguration
	}
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

	grant, found, err := w.Protocol.ClaimNextDataSourceConnectionTest(ctx, agentwire.DataSourceConnectionTestClaimNext{
		BootID: w.BootID,
		SentAt: w.now(),
	})
	if err != nil {
		return Outcome{}, false, ErrGrantRejected
	}
	if !found {
		return Outcome{}, false, nil
	}
	if !w.validGrant(identity, grant) {
		return Outcome{}, true, ErrGrantRejected
	}
	if w.expired(grant) {
		return Outcome{}, true, ErrLeaseExpired
	}
	if err := w.Protocol.AcknowledgeDataSourceConnectionTestLease(ctx, acknowledgement(w.BootID, grant, w.now())); err != nil {
		return Outcome{}, true, ErrLeaseAcknowledgement
	}
	if w.expired(grant) {
		return Outcome{}, true, ErrLeaseExpired
	}

	outcome, ok := w.testOutcome(ctx, grant)
	if !ok {
		return Outcome{}, true, ErrGrantRejected
	}
	state, err := w.Protocol.CompleteDataSourceConnectionTest(ctx, completion(w.BootID, grant, outcome, w.now()))
	if err != nil || state != outcome.Status {
		return Outcome{}, true, ErrCompletionRejected
	}
	return outcome, true, nil
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
	return w != nil && w.Protocol != nil && w.Clock != nil && validOpaque(w.BootID, 256)
}

// validGrant 先校验所有来源共享的冻结绑定，再按来源收窄允许的本机能力。
// AGENT_JDBC 没有显式运行器时必须拒绝，避免控制面开关与 Agent 本机配置不一致时解析秘密。
func (w *Worker) validGrant(identity agentwire.AgentIdentity, grant agentwire.DataSourceConnectionTestGrant) bool {
	binding := grant.Binding
	validBinding := validPathID(grant.ConnectionTestID) && validOpaque(grant.LeaseID, 256) && grant.LeaseEpoch > 0 && !grant.ExpiresAt.IsZero() &&
		validSHA256Digest(grant.BindingDigest) && validPathID(binding.ConnectionTestID) && binding.ConnectionTestID == grant.ConnectionTestID &&
		validOpaque(binding.DataSourceID, 256) && validSHA256Digest(binding.ConnectionConfigDigest) && binding.CredentialRevision > 0 &&
		validOpaque(binding.NodeID, 256) && binding.NodeID == identity.NodeID && binding.NodeFactsRevision > 0 &&
		((binding.SysCredentialID == "" && binding.SysCredentialRevision == 0) ||
			(validOpaque(binding.SysCredentialID, 256) && binding.SysCredentialRevision > 0))
	if !validBinding {
		return false
	}
	switch grant.VerificationSource {
	case agentwire.DataSourceConnectionTestG2Synthetic:
		return true
	case agentwire.DataSourceConnectionTestAgentJDBC:
		return w.JDBCRunner != nil
	default:
		return false
	}
}

// testOutcome 将来源固定到唯一可接受的结果集合，避免运行器将异常文本或任意结果写入控制面。
func (w *Worker) testOutcome(ctx context.Context, grant agentwire.DataSourceConnectionTestGrant) (Outcome, bool) {
	switch grant.VerificationSource {
	case agentwire.DataSourceConnectionTestG2Synthetic:
		return Outcome{
			ConnectionTestID: grant.ConnectionTestID, Status: agentwire.DataSourceConnectionTestSucceeded,
			EvidenceCode: "SYNTHETIC_OK", VerificationSource: agentwire.DataSourceConnectionTestG2Synthetic,
			SysVerificationStatus: agentwire.DataSourceConnectionTestSysNotConfigured,
		}, true
	case agentwire.DataSourceConnectionTestAgentJDBC:
		outcome := w.JDBCRunner.RunConnectionTest(ctx, grant)
		return outcome, validJDBCOutcome(grant, outcome)
	default:
		return Outcome{}, false
	}
}

func (w *Worker) now() time.Time {
	return w.Clock().UTC()
}

func (w *Worker) expired(grant agentwire.DataSourceConnectionTestGrant) bool {
	return !w.now().Before(grant.ExpiresAt)
}

func acknowledgement(bootID string, grant agentwire.DataSourceConnectionTestGrant, sentAt time.Time) agentwire.DataSourceConnectionTestLeaseAcknowledgement {
	return agentwire.DataSourceConnectionTestLeaseAcknowledgement{
		BootID: bootID, ConnectionTestID: grant.ConnectionTestID, LeaseID: grant.LeaseID,
		LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.BindingDigest, SentAt: sentAt,
	}
}

func completion(bootID string, grant agentwire.DataSourceConnectionTestGrant, outcome Outcome, sentAt time.Time) agentwire.DataSourceConnectionTestCompletion {
	return agentwire.DataSourceConnectionTestCompletion{
		BootID: bootID, ConnectionTestID: grant.ConnectionTestID, LeaseID: grant.LeaseID,
		LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.BindingDigest, Status: outcome.Status,
		EvidenceCode: outcome.EvidenceCode, VerificationSource: outcome.VerificationSource,
		SysVerificationStatus: outcome.SysVerificationStatus, SysEvidenceCode: outcome.SysEvidenceCode, SentAt: sentAt,
	}
}

func validJDBCOutcome(grant agentwire.DataSourceConnectionTestGrant, outcome Outcome) bool {
	if outcome.ConnectionTestID != grant.ConnectionTestID || outcome.VerificationSource != agentwire.DataSourceConnectionTestAgentJDBC {
		return false
	}
	// 可选的 sys 凭据验证结果必须匹配绑定冻结状态：配置了 sys 引用时必须给出受控终态，否则必须为 NOT_CONFIGURED。
	hasSys := grant.Binding.SysCredentialRevision > 0
	sysOutcomeValid := hasSys == (outcome.SysVerificationStatus != agentwire.DataSourceConnectionTestSysNotConfigured) &&
		outcome.SysVerificationStatus != "" && agentwire.ValidDataSourceConnectionTestSysOutcome(outcome.SysVerificationStatus, outcome.SysEvidenceCode)
	if !sysOutcomeValid {
		return false
	}
	return (outcome.Status == agentwire.DataSourceConnectionTestSucceeded && outcome.EvidenceCode == "DATABASE_CONNECTED") ||
		(outcome.Status == agentwire.DataSourceConnectionTestFailed && outcome.EvidenceCode == "DATABASE_HOST_UNRESOLVABLE") ||
		(outcome.Status == agentwire.DataSourceConnectionTestFailed && outcome.EvidenceCode == "DATABASE_TCP_REFUSED") ||
		(outcome.Status == agentwire.DataSourceConnectionTestFailed && outcome.EvidenceCode == "DATABASE_TCP_TIMEOUT") ||
		(outcome.Status == agentwire.DataSourceConnectionTestFailed && outcome.EvidenceCode == "DATABASE_TCP_UNREACHABLE") ||
		(outcome.Status == agentwire.DataSourceConnectionTestFailed && outcome.EvidenceCode == "DATABASE_CONNECTION_FAILED") ||
		(outcome.Status == agentwire.DataSourceConnectionTestUnknown && outcome.EvidenceCode == "DATABASE_CONNECTION_UNAVAILABLE")
}

func validOpaque(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) && !strings.ContainsRune(value, 0) && !strings.ContainsAny(value, "\r\n")
}

func validPathID(value string) bool {
	return validOpaque(value, 256) && !strings.ContainsAny(value, "/\\?#:")
}

func validSHA256Digest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, item := range value {
		if !(item >= 'a' && item <= 'f' || item >= '0' && item <= '9') {
			return false
		}
	}
	return true
}
