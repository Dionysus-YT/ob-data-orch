package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"ob-data-orch/internal/agentexecution"
	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/nodefacts"
	"sort"
)

// exportExecutionRunner 沿用历史名称，每次只领取一条显式能力的冻结任务。
// 它不暴露任意命令、SQL 或路径参数。
type exportExecutionRunner interface {
	RunNext(context.Context) (agentexecution.Outcome, bool, error)
}

// configuredExportExecutionRunner 每次领取前都从已关联状态重新读取本机 Java、工具和数据目录白名单。
// 配置缺失、平台不符或目录漂移时 Worker 只上报启动拒绝，不会退化到 PATH 或环境变量路径。
type configuredExportExecutionRunner struct {
	normalImportEnabled bool
	directLoadEnabled   bool
	claimMu             sync.Mutex
	claimPosition       int
	stateStore          *agentwire.StateStore
	lookupEnv           func(string) (string, bool)
	operatingSystem     string
	architecture        string
	bootID              string
	queueMu             sync.Mutex
	queue               *agentlogqueue.Queue
	queueRoot           string
	workersMu           sync.Mutex
	workers             map[*agentexecution.Worker]struct{}
}

func (r *configuredExportExecutionRunner) RunNext(ctx context.Context) (agentexecution.Outcome, bool, error) {
	if r.stateStore == nil {
		return agentexecution.Outcome{}, false, agentexecution.ErrInvalidConfiguration
	}
	release := r.stateStore.HoldRuntimeConfiguration()
	defer release()
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
	queue, err := r.sharedQueue(queueRoot)
	if err != nil {
		return agentexecution.Outcome{}, false, agentexecution.ErrInvalidConfiguration
	}
	worker := &agentexecution.Worker{
		Protocol: r.stateStore,
		Runtime: agentexecution.Runtime{
			TargetPlatform: configuration.Platform, JavaPath: runtimeConfig.JavaPath, ToolHome: runtimeConfig.ToolHome, WorkspaceRoot: runtimeConfig.WorkspaceRoot, LogQueueRoot: queueRoot, SharedLogQueue: queue, LogQueueObserver: queueValidationObserver(queueRoot),
			Environment: runtimeConfig.Environment, AllowedRoots: configuration.AllowedRoots,
		},
		Clock: time.Now, BootID: r.bootID,
	}
	// 在同一节点预算内轮询显式开启的能力；未开启的旁路不会被导出或普通导入代领。
	r.claimMu.Lock()
	types := []string{"OBDUMPER_EXPORT"}
	if r.normalImportEnabled {
		types = append(types, importdomain.TaskType)
	}
	if r.directLoadEnabled {
		types = append(types, directload.TaskType)
	}
	worker.TaskType = types[r.claimPosition%len(types)]
	r.claimPosition = (r.claimPosition + 1) % len(types)
	r.claimMu.Unlock()
	r.workersMu.Lock()
	if r.workers == nil {
		r.workers = make(map[*agentexecution.Worker]struct{})
	}
	r.workers[worker] = struct{}{}
	r.workersMu.Unlock()
	defer func() { r.workersMu.Lock(); delete(r.workers, worker); r.workersMu.Unlock() }()
	return worker.RunNext(ctx)
}

// ExecutionObservation 从真实 Worker 采集执行和本地事件位置，不复制控制面任务状态。
func (r *configuredExportExecutionRunner) ExecutionObservation() *nodefacts.ExecutionObservation {
	if r == nil {
		return nil
	}
	observation := &nodefacts.ExecutionObservation{Active: make([]nodefacts.ActiveExecution, 0)}
	r.workersMu.Lock()
	defer r.workersMu.Unlock()
	for worker := range r.workers {
		if active, found := worker.ActiveExecution(); found {
			observation.Active = append(observation.Active, active)
		}
	}
	sort.Slice(observation.Active, func(i, j int) bool { return observation.Active[i].ExecutionID < observation.Active[j].ExecutionID })
	return observation
}

// LogQueueStats 复用执行器的唯一持久队列，不能产生另一份容量缓存或读取正文。
func (r *configuredExportExecutionRunner) LogQueueStats() *nodefacts.LogQueue {
	if r == nil || r.stateStore == nil {
		return nil
	}
	root, err := r.stateStore.LogQueueDirectory()
	if err != nil {
		return nil
	}
	queue, err := r.sharedQueue(root)
	if err != nil {
		return nil
	}
	stats, err := queue.Stats()
	if err != nil {
		return nil
	}
	return stats
}

// sharedQueue 一个 Agent 只持有一个队列实例；目录漂移时失败关闭，不并存不同容量缓存。
func (r *configuredExportExecutionRunner) sharedQueue(root string) (*agentlogqueue.Queue, error) {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	if r.queue != nil {
		if r.queueRoot != root {
			return nil, agentexecution.ErrInvalidConfiguration
		}
		return r.queue, nil
	}
	queue, err := agentlogqueue.Open(root, agentlogqueue.DefaultMaxBytes)
	if err != nil {
		return nil, err
	}
	r.queue, r.queueRoot = queue, root
	return queue, nil
}

// asynchronousExportExecutionRunner 让主循环在真实工具运行期间继续心跳。
// 后台数受本机资源上界约束，真正的共享预算由控制面领取事务裁决；空策略保留单 Worker。
type asynchronousExportExecutionRunner struct {
	delegate exportExecutionRunner
	logger   *slog.Logger
	mu       sync.Mutex
	running  bool
	active   int
	capacity func() int
}

func (r *asynchronousExportExecutionRunner) RunNext(ctx context.Context) (bool, error) {
	if r == nil || r.delegate == nil || ctx == nil {
		return false, errors.New("export execution runner is unavailable")
	}
	r.mu.Lock()
	limit := 1
	if r.capacity != nil {
		limit = r.capacity()
	}
	if ctx.Err() != nil || r.active >= limit {
		r.mu.Unlock()
		return false, nil
	}
	r.running = true
	r.active++
	r.mu.Unlock()
	go func() {
		_, _, err := r.delegate.RunNext(ctx)
		if err != nil && r.logger != nil {
			r.logger.Warn("agent export execution worker attempt failed", "error", err)
		}
		r.mu.Lock()
		r.active--
		r.running = r.active > 0
		r.mu.Unlock()
	}()
	return true, nil
}

// ActiveWorkers 供心跳读取实际后台数，不从数据库状态或任务列表猜测。
func (r *asynchronousExportExecutionRunner) ActiveWorkers() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// ExecutionObservation 保留委托执行器的事实所有权；缺采集端口时保持未知。
func (r *asynchronousExportExecutionRunner) ExecutionObservation() *nodefacts.ExecutionObservation {
	if r == nil {
		return nil
	}
	if observer, ok := r.delegate.(interface {
		ExecutionObservation() *nodefacts.ExecutionObservation
	}); ok {
		return observer.ExecutionObservation()
	}
	return nil
}

// LogQueueStats 保留唯一共享队列的统计所有权。
func (r *asynchronousExportExecutionRunner) LogQueueStats() *nodefacts.LogQueue {
	if r == nil {
		return nil
	}
	if observer, ok := r.delegate.(interface{ LogQueueStats() *nodefacts.LogQueue }); ok {
		return observer.LogQueueStats()
	}
	return nil
}
