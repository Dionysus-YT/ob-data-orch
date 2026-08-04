package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"ob-data-orch/internal/agentexecution"
	"ob-data-orch/internal/agentwire"
)

// exportExecutionRunner 只允许主循环调度一条已冻结的 OBDUMPER_EXPORT。
// 它不暴露任意命令、SQL 或路径参数。
type exportExecutionRunner interface {
	RunNext(context.Context) (agentexecution.Outcome, bool, error)
}

// configuredExportExecutionRunner 每次领取前都从已关联状态重新读取本机 Java、工具和数据目录白名单。
// 配置缺失、平台不符或目录漂移时 Worker 只上报启动拒绝，不会退化到 PATH 或环境变量路径。
type configuredExportExecutionRunner struct {
	stateStore      *agentwire.StateStore
	lookupEnv       func(string) (string, bool)
	operatingSystem string
	architecture    string
	bootID          string
}

func (r configuredExportExecutionRunner) RunNext(ctx context.Context) (agentexecution.Outcome, bool, error) {
	if r.operatingSystem != "WINDOWS" || r.architecture != "AMD64" {
		return agentexecution.Outcome{}, false, agentexecution.ErrInvalidConfiguration
	}
	runtimeConfig, _, err := agentRuntimeFromState(r.stateStore, r.lookupEnv, r.operatingSystem, r.architecture)
	if err != nil {
		return agentexecution.Outcome{}, false, agentexecution.ErrInvalidConfiguration
	}
	configuration, err := r.stateStore.RuntimeConfiguration()
	if err != nil || len(configuration.AllowedRoots) == 0 {
		return agentexecution.Outcome{}, false, agentexecution.ErrInvalidConfiguration
	}
	queueRoot, err := r.stateStore.LogQueueDirectory()
	if err != nil {
		return agentexecution.Outcome{}, false, agentexecution.ErrInvalidConfiguration
	}
	worker := &agentexecution.Worker{
		Protocol: r.stateStore,
		Runtime: agentexecution.Runtime{
			TargetPlatform: configuration.Platform, JavaPath: runtimeConfig.JavaPath, ToolHome: runtimeConfig.ToolHome, WorkspaceRoot: runtimeConfig.WorkspaceRoot, LogQueueRoot: queueRoot, LogQueueObserver: queueValidationObserver(queueRoot),
			Environment: runtimeConfig.Environment, AllowedRoots: configuration.AllowedRoots,
		},
		Clock: time.Now, BootID: r.bootID,
	}
	return worker.RunNext(ctx)
}

// asynchronousExportExecutionRunner 让主循环在真实工具运行期间继续心跳。
// 同一 Agent 始终只保留一个后台 Worker；网络或启动错误只阻断该任务，不能并发领取第二条任务。
type asynchronousExportExecutionRunner struct {
	delegate exportExecutionRunner
	logger   *slog.Logger
	mu       sync.Mutex
	running  bool
}

func (r *asynchronousExportExecutionRunner) RunNext(ctx context.Context) (bool, error) {
	if r == nil || r.delegate == nil || ctx == nil {
		return false, errors.New("export execution runner is unavailable")
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return false, nil
	}
	r.running = true
	r.mu.Unlock()
	go func() {
		_, _, err := r.delegate.RunNext(ctx)
		if err != nil && r.logger != nil {
			r.logger.Warn("agent export execution worker attempt failed", "error", err)
		}
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()
	return true, nil
}
