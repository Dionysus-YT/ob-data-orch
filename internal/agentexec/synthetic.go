package agentexec

import (
	"context"
	"errors"
	"regexp"
	"time"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/credential"
)

var (
	ErrSyntheticToolUnavailable = errors.New("假工具不可用")
	ErrSyntheticStartRejected   = errors.New("假工具明确拒绝启动")
	ErrProcessEvidenceInvalid   = errors.New("进程证据无效，必须核对")
)

var executableDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// SyntheticLaunch 是 G2 假工具可接受的唯一启动输入。
// 它不包含 argv、环境、秘密、安全材料路径或任意可执行文件信息。
type SyntheticLaunch struct {
	Intent StartIntent
	BootID string
}

// ProcessIdentity 是实际启动后必须取得的最小无秘密进程身份。
// 假工具使用合成 PID，但字段约束与后续真实适配的恢复核对要求一致。
type ProcessIdentity struct {
	PID              int
	StartedAt        time.Time
	ExecutableDigest string
	BootID           string
}

// Observation 是假工具返回的受控终态事实，不允许以任意文本推导成功或失败。
type Observation struct {
	ProcessExited bool
	ToolTerminal  agentstate.ToolTerminal
	ResultFacts   agentstate.ResultFacts
}

// EventFact 是后续 Agent 协议适配可顺序编号的无秘密事实。
type EventFact struct {
	Type  agentstate.EventType
	Facts agentstate.Facts
}

// RunResult 记录本地适配观察到的进程身份、终态事实和待上报事件。
// ReconciliationRequired 为真时绝不提供足以形成产品终态的完整事实。
type RunResult struct {
	Process                *ProcessIdentity
	Observation            Observation
	Events                 []EventFact
	ReconciliationRequired bool
}

// SyntheticTool 是 G2 合成工具端口。它没有命令或路径参数，不能成为任意进程启动器。
type SyntheticTool interface {
	Start(context.Context, SyntheticLaunch) (SyntheticProcess, error)
}

// SyntheticProcess 表示已由假工具声明启动的合成进程。
type SyntheticProcess interface {
	Identity() ProcessIdentity
	Wait(context.Context) (Observation, error)
}

// RunSynthetic 先持久化启动意图，再调用假工具并形成可核对的事实。
// 中断、未知或不完整证据只产生核对事件，绝不伪造成功或失败终态。
func RunSynthetic(ctx context.Context, workspace credential.Workspace, launch SyntheticLaunch, tool SyntheticTool) (RunResult, error) {
	if tool == nil || !opaqueIDPattern.MatchString(launch.BootID) {
		return RunResult{}, ErrSyntheticToolUnavailable
	}
	if err := WriteStartIntent(workspace, launch.Intent); err != nil {
		return RunResult{}, err
	}
	process, err := tool.Start(ctx, launch)
	if err != nil {
		if errors.Is(err, ErrSyntheticStartRejected) {
			return RunResult{Events: []EventFact{{Type: agentstate.EventStartRejected, Facts: agentstate.Facts{NoProcess: true}}}}, nil
		}
		return reconciliationResult(nil, Observation{}), ErrSyntheticToolUnavailable
	}
	if process == nil {
		return reconciliationResult(nil, Observation{}), ErrSyntheticToolUnavailable
	}
	identity := process.Identity()
	if err := validateProcessIdentity(identity, launch.BootID); err != nil {
		return reconciliationResult(nil, Observation{}), ErrProcessEvidenceInvalid
	}
	result := RunResult{Process: &identity, Events: []EventFact{{Type: agentstate.EventProcessStarted}}}
	observation, err := process.Wait(ctx)
	if err != nil {
		return reconciliationResult(&identity, Observation{}), ErrSyntheticToolUnavailable
	}
	result.Observation = observation
	if !observation.ProcessExited {
		return reconciliationResult(&identity, observation), nil
	}
	result.Events = append(result.Events, EventFact{Type: agentstate.EventProcessExited, Facts: agentstate.Facts{ProcessExited: true}})
	if validToolTerminal(observation.ToolTerminal) && observation.ToolTerminal != agentstate.ToolUnknown {
		result.Events = append(result.Events, EventFact{Type: agentstate.EventToolTerminal, Facts: agentstate.Facts{ToolTerminal: observation.ToolTerminal}})
	}
	if validResultFacts(observation.ResultFacts) && observation.ResultFacts != agentstate.ResultUnknown {
		result.Events = append(result.Events, EventFact{Type: agentstate.EventResultFacts, Facts: agentstate.Facts{ResultFacts: observation.ResultFacts}})
	}
	if validToolTerminal(observation.ToolTerminal) && validResultFacts(observation.ResultFacts) && (observation.ToolTerminal == agentstate.ToolSuccess && observation.ResultFacts == agentstate.ResultVerified || observation.ToolTerminal == agentstate.ToolFailure || observation.ResultFacts == agentstate.ResultFailed) {
		return result, nil
	}
	result.ReconciliationRequired = true
	result.Events = append(result.Events, EventFact{Type: agentstate.EventLocalRecoveryDone})
	return result, nil
}

func reconciliationResult(identity *ProcessIdentity, observation Observation) RunResult {
	result := RunResult{Process: identity, Observation: observation, ReconciliationRequired: true}
	if identity != nil {
		result.Events = append(result.Events, EventFact{Type: agentstate.EventProcessStarted})
	}
	result.Events = append(result.Events, EventFact{Type: agentstate.EventLocalRecoveryDone})
	return result
}

func validateProcessIdentity(identity ProcessIdentity, bootID string) error {
	if identity.PID < 1 || identity.StartedAt.IsZero() || identity.BootID != bootID || !executableDigestPattern.MatchString(identity.ExecutableDigest) {
		return ErrProcessEvidenceInvalid
	}
	return nil
}

func validToolTerminal(value agentstate.ToolTerminal) bool {
	return value == agentstate.ToolUnknown || value == agentstate.ToolSuccess || value == agentstate.ToolFailure
}

func validResultFacts(value agentstate.ResultFacts) bool {
	return value == agentstate.ResultUnknown || value == agentstate.ResultVerified || value == agentstate.ResultFailed
}
