package main

import (
	"context"
	"errors"
	"net"
	"strconv"
	"syscall"
	"time"

	"ob-data-orch/internal/agentconnectiontest"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

// jdbcConnectionSecretResolver 只允许在已确认的基础连接测试租约内取回唯一数据库槽位。
// 该接口不暴露 Agent 的关联、心跳、领取或任意 HTTP 能力，避免测试运行器扩大协议权限。
type jdbcConnectionSecretResolver interface {
	ResolveDataSourceConnectionTestDatabaseConnection(context.Context, agentwire.DataSourceConnectionTestSecretSlotRequest) (agentwire.DataSourceConnectionTestDatabaseConnectionSlot, error)
	// ResolveDataSourceConnectionTestSysConnection 只解析可选的 sys 凭据槽位（绑定冻结了 sys 引用时）。
	ResolveDataSourceConnectionTestSysConnection(context.Context, agentwire.DataSourceConnectionTestSecretSlotRequest) (agentwire.DataSourceConnectionTestDatabaseConnectionSlot, error)
}

// jdbcConnectionProbe 是固定 JDBC 探针的窄调用边界。
// 生产实现只允许调用已在本租约工作区安装并核验一次的固定探针；测试替身不得改变 Worker 的终态语义。
type jdbcConnectionProbe func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.Request) (jdbcprobe.Result, error)

// jdbcConnectionDial 是 JDBC 前固定 TCP 可达性检查的窄调用边界。
// 它只验证被冻结的主机和端口能否建立 TCP 连接，不读取协议内容，不记录目标地址。
type jdbcConnectionDial func(context.Context, string, string) (net.Conn, error)

// jdbcConnectionTestRunner 只在控制面已签发 AGENT_JDBC 租约且 Worker 已确认租约后运行。
// 它不保存探针元信息、连接地址、用户身份、密码或错误文本；所有短时秘密和私有目录都会在返回前清理。
type jdbcConnectionTestRunner struct {
	resolver      jdbcConnectionSecretResolver
	workspaceRoot string
	runtime       jdbcprobe.Runtime
	bootID        string
	probe         jdbcConnectionProbe
	dial          jdbcConnectionDial
}

// configuredJDBCConnectionTestRunner 在每次已确认租约后读取首次关联固化的本机运行时配置。
// 它不允许连接测试沿用进程环境中的工具路径，关联尚未完成或配置漂移时保持不可用。
type configuredJDBCConnectionTestRunner struct {
	stateStore      *agentwire.StateStore
	lookupEnv       func(string) (string, bool)
	operatingSystem string
	architecture    string
	bootID          string
}

// RunConnectionTest 仅在本机配置可复核时委托给固定 JDBC 探针运行器。
func (r configuredJDBCConnectionTestRunner) RunConnectionTest(ctx context.Context, grant agentwire.DataSourceConnectionTestGrant) agentconnectiontest.Outcome {
	runtimeConfig, _, err := agentRuntimeFromState(r.stateStore, r.lookupEnv, r.operatingSystem, r.architecture)
	if err != nil {
		return jdbcConnectionTestRunner{resolver: r.stateStore, bootID: r.bootID}.RunConnectionTest(ctx, grant)
	}
	jdbcRuntime, err := jdbcprobe.DiscoverRuntime(runtimeConfig.JavaPath, runtimeConfig.ToolHome, runtimeConfig.Environment)
	if err != nil {
		return jdbcConnectionTestRunner{resolver: r.stateStore, bootID: r.bootID}.RunConnectionTest(ctx, grant)
	}
	return jdbcConnectionTestRunner{resolver: r.stateStore, workspaceRoot: runtimeConfig.WorkspaceRoot, runtime: jdbcRuntime, bootID: r.bootID}.RunConnectionTest(ctx, grant)
}

// RunConnectionTest 在单个私有工作区中解析一次槽位并调用固定 JDBC 探针。
// 只有明确的 JDBC 连接拒绝会映射为 FAILED；运行时、槽位、探针或清理异常一律映射为 UNKNOWN。
func (r jdbcConnectionTestRunner) RunConnectionTest(ctx context.Context, grant agentwire.DataSourceConnectionTestGrant) (outcome agentconnectiontest.Outcome) {
	outcome = agentconnectiontest.Outcome{
		ConnectionTestID:      grant.ConnectionTestID,
		Status:                agentwire.DataSourceConnectionTestUnknown,
		EvidenceCode:          "DATABASE_CONNECTION_UNAVAILABLE",
		VerificationSource:    agentwire.DataSourceConnectionTestAgentJDBC,
		SysVerificationStatus: agentwire.DataSourceConnectionTestSysNotConfigured,
	}
	if grant.Binding.SysCredentialRevision > 0 {
		// 绑定要求验证 sys 时，任何早退都必须报告未知，不能伪装成未配置。
		outcome.SysVerificationStatus = agentwire.DataSourceConnectionTestSysUnknown
		outcome.SysEvidenceCode = "SYS_CONNECTION_UNAVAILABLE"
	}
	if r.resolver == nil || r.workspaceRoot == "" || r.bootID == "" || grant.VerificationSource != agentwire.DataSourceConnectionTestAgentJDBC {
		return outcome
	}

	workspace, err := credential.CreateWorkspace(r.workspaceRoot, "connection-test-"+grant.ConnectionTestID)
	if err != nil {
		return outcome
	}
	defer func() {
		if cleanupErr := workspace.Cleanup(); cleanupErr != nil {
			outcome.Status = agentwire.DataSourceConnectionTestUnknown
			outcome.EvidenceCode = "DATABASE_CONNECTION_UNAVAILABLE"
		}
	}()

	slot, err := r.resolver.ResolveDataSourceConnectionTestDatabaseConnection(ctx, agentwire.DataSourceConnectionTestSecretSlotRequest{
		BootID:           r.bootID,
		ConnectionTestID: grant.ConnectionTestID,
		LeaseID:          grant.LeaseID,
		LeaseEpoch:       grant.LeaseEpoch,
		BindingDigest:    grant.BindingDigest,
		SentAt:           time.Now().UTC(),
	})
	if err != nil {
		return outcome
	}
	defer slot.Destroy()

	dial := r.dial
	if dial == nil {
		dialer := net.Dialer{Timeout: 5 * time.Second}
		dial = dialer.DialContext
	}
	connection, err := dial(ctx, "tcp", net.JoinHostPort(slot.Host, strconv.Itoa(slot.Port)))
	if err != nil {
		outcome.Status = agentwire.DataSourceConnectionTestFailed
		outcome.EvidenceCode = jdbcConnectionDialFailureCode(err)
		return outcome
	}
	if connection == nil {
		return outcome
	}
	if err := connection.Close(); err != nil {
		return outcome
	}

	probe := r.probe
	if probe == nil {
		probePath, probeDigest, installErr := jdbcprobe.Install(workspace)
		if installErr != nil {
			return outcome
		}
		preparedRuntime := r.runtime
		preparedRuntime.ProbePath = probePath
		preparedRuntime.ProbeSHA256 = probeDigest
		probe = func(probeContext context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, request jdbcprobe.Request) (jdbcprobe.Result, error) {
			return jdbcprobe.TestConnection(probeContext, preparedRuntime, request)
		}
	}
	_, err = probe(ctx, workspace, r.runtime, jdbcprobe.Request{
		Host: slot.Host, Port: slot.Port, Username: slot.Username, Password: slot.Password,
	})
	if err == nil {
		outcome.Status = agentwire.DataSourceConnectionTestSucceeded
		outcome.EvidenceCode = "DATABASE_CONNECTED"
	} else if errors.Is(err, jdbcprobe.ErrConnectionFailed) {
		outcome.Status = agentwire.DataSourceConnectionTestFailed
		outcome.EvidenceCode = "DATABASE_CONNECTION_FAILED"
	} else {
		return outcome
	}
	// 可选的 sys 凭据验证（参考 ODC 的 sys 账号验证）：绑定冻结了 sys 引用时才解析 sys 槽位。
	// sys 结果与数据库结果相互独立，不改变数据库终态。
	outcome.SysVerificationStatus, outcome.SysEvidenceCode = r.runSysVerification(ctx, grant, workspace, probe)
	return outcome
}

// runSysVerification 在数据库验证完成后额外验证可选的 sys 租户连接。
// 槽位解析失败、TCP 或探针异常一律映射为 UNKNOWN；只有明确拒绝映射为 FAILED。
func (r jdbcConnectionTestRunner) runSysVerification(ctx context.Context, grant agentwire.DataSourceConnectionTestGrant, workspace credential.Workspace, probe jdbcConnectionProbe) (agentwire.DataSourceConnectionTestSysVerificationStatus, string) {
	if grant.Binding.SysCredentialRevision < 1 {
		return agentwire.DataSourceConnectionTestSysNotConfigured, ""
	}
	sysSlot, err := r.resolver.ResolveDataSourceConnectionTestSysConnection(ctx, agentwire.DataSourceConnectionTestSecretSlotRequest{
		BootID:           r.bootID,
		ConnectionTestID: grant.ConnectionTestID,
		LeaseID:          grant.LeaseID,
		LeaseEpoch:       grant.LeaseEpoch,
		BindingDigest:    grant.BindingDigest,
		SentAt:           time.Now().UTC(),
	})
	if err != nil {
		return agentwire.DataSourceConnectionTestSysUnknown, "SYS_CONNECTION_UNAVAILABLE"
	}
	defer sysSlot.Destroy()

	dial := r.dial
	if dial == nil {
		dialer := net.Dialer{Timeout: 5 * time.Second}
		dial = dialer.DialContext
	}
	connection, err := dial(ctx, "tcp", net.JoinHostPort(sysSlot.Host, strconv.Itoa(sysSlot.Port)))
	if err != nil {
		return agentwire.DataSourceConnectionTestSysFailed, jdbcSysConnectionDialFailureCode(err)
	}
	if connection == nil {
		return agentwire.DataSourceConnectionTestSysUnknown, "SYS_CONNECTION_UNAVAILABLE"
	}
	if err := connection.Close(); err != nil {
		return agentwire.DataSourceConnectionTestSysUnknown, "SYS_CONNECTION_UNAVAILABLE"
	}

	_, err = probe(ctx, workspace, r.runtime, jdbcprobe.Request{
		Host: sysSlot.Host, Port: sysSlot.Port, Username: sysSlot.Username, Password: sysSlot.Password,
	})
	if err == nil {
		return agentwire.DataSourceConnectionTestSysSucceeded, "SYS_CONNECTED"
	}
	if errors.Is(err, jdbcprobe.ErrConnectionFailed) {
		return agentwire.DataSourceConnectionTestSysFailed, "SYS_CONNECTION_FAILED"
	}
	return agentwire.DataSourceConnectionTestSysUnknown, "SYS_CONNECTION_UNAVAILABLE"
}

// jdbcSysConnectionDialFailureCode 与数据库版一致，只把 TCP 建连错误折叠为 SYS_ 前缀的受控证据码。
func jdbcSysConnectionDialFailureCode(err error) string {
	code := jdbcConnectionDialFailureCode(err)
	switch code {
	case "DATABASE_HOST_UNRESOLVABLE":
		return "SYS_HOST_UNRESOLVABLE"
	case "DATABASE_TCP_REFUSED":
		return "SYS_TCP_REFUSED"
	case "DATABASE_TCP_TIMEOUT":
		return "SYS_TCP_TIMEOUT"
	default:
		return "SYS_TCP_UNREACHABLE"
	}
}

// jdbcConnectionDialFailureCode 只将底层 TCP 建连错误折叠为有限的无秘密证据码。
// 它不能回传目标地址、系统错误文本或网络栈细节，避免连接诊断扩大为网络探测接口。
func jdbcConnectionDialFailureCode(err error) string {
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return "DATABASE_HOST_UNRESOLVABLE"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "DATABASE_TCP_REFUSED"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "DATABASE_TCP_TIMEOUT"
	}
	return "DATABASE_TCP_UNREACHABLE"
}
