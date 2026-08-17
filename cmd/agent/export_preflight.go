package main

import (
	"context"
	"errors"
	"time"

	"ob-data-orch/internal/agentconnectiontest"
	"ob-data-orch/internal/agentjdbc"
	"ob-data-orch/internal/agentlocalpreflight"
	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/agentworker"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/jdbcprobe"
)

const exportPreflightMinimumAvailableBytes = 1 << 30

// exportPreflightRunner 只允许主循环领取一条固定预检查，不暴露任务或工具执行能力。
type exportPreflightRunner interface {
	RunNext(context.Context) (agentworker.Outcome, bool, error)
}

// configuredExportPreflightRunner 在每次领取前读取首次关联时固化的本机运行时配置。
// 本机运行时无法复核时仍由固定 Probe 上报安全 UNKNOWN，不能跳过租约或改用进程环境中的工具路径。
type configuredExportPreflightRunner struct {
	stateStore      *agentwire.StateStore
	lookupEnv       func(string) (string, bool)
	operatingSystem string
	architecture    string
	bootID          string
}

// RunNext 只在已关联身份与当前平台一致时构造受控 Worker，并始终保持真实导出关闭。
func (r configuredExportPreflightRunner) RunNext(ctx context.Context) (agentworker.Outcome, bool, error) {
	platform, ok := localCommandPlatform(r.operatingSystem, r.architecture)
	if !ok {
		return agentworker.Outcome{}, false, agentworker.ErrInvalidConfiguration
	}
	worker := &agentworker.Worker{
		Protocol:      r.stateStore,
		ProbeFactory:  r.probeFactory(platform),
		Clock:         time.Now,
		BootID:        r.bootID,
		LocalPlatform: platform,
	}
	return worker.RunNext(ctx)
}

// probeFactory 把 JDBC 槽位解析限制在数据库检查内，并保持目录和空间检查只读本机固定配置。
// EX-I6：存储连通性探测只在显式运行开关开启时装配；本机启动器固定开启并按进程生命周期持续授权；
// 凭据有效性探测固定失败关闭，不解析真实凭据。
func (r configuredExportPreflightRunner) probeFactory(platform commandgen.Platform) agentworker.ProbeFactory {
	runtimeConfig, _, runtimeErr := agentRuntimeFromState(r.stateStore, r.lookupEnv, r.operatingSystem, r.architecture)
	if runtimeErr != nil {
		return func(agentworker.SecretResolver) agentpreflight.Probe {
			return agentlocalpreflight.Probe{
				Runtime:               agentlocalpreflight.RuntimeValidatorFunc(func(context.Context) error { return agentlocalpreflight.ErrToolRuntimeUnavailable }),
				MinimumAvailableBytes: exportPreflightMinimumAvailableBytes,
				AvailableBytes:        agentlocalpreflight.AvailableBytes,
				StorageAuth:           agentlocalpreflight.UnavailableStorageAuthProber{},
			}
		}
	}
	jdbcRuntime, jdbcErr := jdbcprobe.DiscoverRuntime(runtimeConfig.JavaPath, runtimeConfig.ToolHome, runtimeConfig.Environment)
	storageConnectivityEnabled, _ := r.lookupEnv("OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE")
	return func(resolver agentworker.SecretResolver) agentpreflight.Probe {
		probe := agentlocalpreflight.Probe{
			Runtime:               agentlocalpreflight.ToolRuntimeValidator{JavaPath: runtimeConfig.JavaPath, ToolHome: runtimeConfig.ToolHome, Environment: runtimeConfig.Environment, TargetPlatform: platform},
			MinimumAvailableBytes: exportPreflightMinimumAvailableBytes,
			AvailableBytes:        agentlocalpreflight.AvailableBytes,
			// 凭据有效性探测固定失败关闭：真实探测归 EX-V1，本切片绝不解析或发送真实凭据。
			StorageAuth: agentlocalpreflight.UnavailableStorageAuthProber{},
		}
		if storageConnectivityEnabled == "true" {
			// 显式开关开启才执行真实 TCP 探测；直接启动默认 UNKNOWN，本机启动器按运行期持续授权。
			probe.StorageConnectivity = agentlocalpreflight.TCPStorageConnectivityProber{}
		}
		if jdbcErr == nil {
			probe.JDBC = &agentjdbc.PrecheckProbe{WorkspaceRoot: runtimeConfig.WorkspaceRoot, Runtime: jdbcRuntime, Resolver: resolver}
		}
		return probe
	}
}

// connectionAndPrecheckRunner 按同一已确认心跳后的固定顺序领取连接测试和预检查。
// 两类租约相互独立；失效连接测试不能永久饿死预检查，但两个 Worker 仍严格串行，避免并发使用同一 Agent 身份和本机工作区。
type connectionAndPrecheckRunner struct {
	connectionTests connectionTestRunner
	prechecks       exportPreflightRunner
	executions      interface {
		RunNext(context.Context) (bool, error)
	}
}

// RunNext 先处理一条基础连接测试，再尝试一条导出预检查。
// 即使连接测试租约已经失效，也必须尝试预检查并将两个可恢复错误一并交给外层记录，避免旧工作阻断新草稿的固定检查。
func (r connectionAndPrecheckRunner) RunNext(ctx context.Context) (agentconnectiontest.Outcome, bool, error) {
	outcome, connectionFound, connectionErr := r.connectionTests.RunNext(ctx)
	if r.prechecks == nil {
		return outcome, connectionFound, connectionErr
	}
	_, precheckFound, precheckErr := r.prechecks.RunNext(ctx)
	executionFound := false
	var executionErr error
	if r.executions != nil {
		executionFound, executionErr = r.executions.RunNext(ctx)
	}
	return outcome, connectionFound || precheckFound || executionFound, errors.Join(connectionErr, precheckErr, executionErr)
}
