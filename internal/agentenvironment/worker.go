// Package agentenvironment 运行节点启用前唯一允许的固定本机环境核验。
// 它不启动 OBDUMPER、不连接数据库，也不接受路径、命令、SQL 或其他远程操作参数。
package agentenvironment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/agentlocalpreflight"
	"ob-data-orch/internal/agentwire"
)

var (
	// ErrInvalidConfiguration 表示本机身份、时钟或固定运行时核验器缺失。
	ErrInvalidConfiguration = errors.New("节点环境检查 Worker 配置无效")
	// ErrWorkerBusy 表示同一 Agent 进程正在回传一条固定环境检查。
	ErrWorkerBusy = errors.New("节点环境检查 Worker 正在运行")
	// ErrCompletionRejected 表示控制面没有确认固定检查结果。
	ErrCompletionRejected = errors.New("节点环境检查结果上报失败")
)

// Protocol 将 Worker 限制为读取受保护身份和回传一条固定稳定结果。
// 它不提供领取任务、解析秘密、启动命令或访问任意本机文件的能力。
type Protocol interface {
	AgentIdentity() (agentwire.AgentIdentity, error)
	CompleteExecutionNodeEnvironmentCheck(context.Context, string, string, int64, string, string, time.Time) error
}

// Worker 在心跳返回明确检查标识后运行固定工具运行时核验。
// Agent 只使用本机管理员配置的 Java、Connector/J 和 OBDUMPER 布局，控制面不会下发路径。
type Worker struct {
	Protocol Protocol
	Runtime  agentlocalpreflight.RuntimeValidator
	Clock    func() time.Time
	BootID   string

	mu      sync.Mutex
	running bool
}

// Run 将控制面心跳返回的单个检查标识绑定到当前 Agent 身份和事实版本。
// 空标识表示当前没有待处理工作；运行时异常会转换为稳定失败码，不向控制面泄露本机细节。
func (w *Worker) Run(ctx context.Context, checkID string, factsRevision int64) (bool, error) {
	if strings.TrimSpace(checkID) == "" {
		return false, nil
	}
	if w == nil || !w.enter() {
		return false, ErrWorkerBusy
	}
	defer w.leave()
	if w.Protocol == nil || w.Runtime == nil || w.Clock == nil || !validOpaque(w.BootID, 256) || !validOpaque(checkID, 256) || strings.ContainsAny(checkID, "/\\?#:") || factsRevision < 1 {
		return false, ErrInvalidConfiguration
	}
	identity, err := w.Protocol.AgentIdentity()
	if err != nil || !validOpaque(identity.AgentID, 256) || !validOpaque(identity.NodeID, 256) {
		return false, ErrInvalidConfiguration
	}
	status, code := runtimeResult(ctx, w.Runtime)
	if err := w.Protocol.CompleteExecutionNodeEnvironmentCheck(ctx, w.BootID, checkID, factsRevision, status, code, w.Clock().UTC()); err != nil {
		// 下层错误只允许是稳定机器协议类别或 HTTP 状态，不应包含本机路径、命令或运行时原始错误。
		return true, fmt.Errorf("%w: %v", ErrCompletionRejected, err)
	}
	return true, nil
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

func runtimeResult(ctx context.Context, runtime agentlocalpreflight.RuntimeValidator) (string, string) {
	if ctx == nil || ctx.Err() != nil || runtime == nil {
		return "FAILED", "TOOL_RUNTIME_UNAVAILABLE"
	}
	err := runtime.Validate(ctx)
	if err == nil {
		return "PASSED", "TOOL_RUNTIME_READY"
	}
	if errors.Is(err, agentlocalpreflight.ErrToolRuntimeInvalid) {
		return "FAILED", "TOOL_RUNTIME_INVALID"
	}
	return "FAILED", "TOOL_RUNTIME_UNAVAILABLE"
}

func validOpaque(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) && !strings.ContainsRune(value, 0) && !strings.ContainsAny(value, "\r\n")
}
