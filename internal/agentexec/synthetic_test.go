package agentexec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/credential"
)

func Test假工具成功需要进程工具与结果三类事实(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := scriptedTool{process: scriptedProcess{identity: validProcess(), observation: Observation{ProcessExited: true, ToolTerminal: agentstate.ToolSuccess, ResultFacts: agentstate.ResultVerified}}}
	result, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool)
	if err != nil || result.ReconciliationRequired {
		t.Fatalf("RunSynthetic() = %#v, %v", result, err)
	}
	want := []agentstate.EventType{agentstate.EventProcessStarted, agentstate.EventProcessExited, agentstate.EventToolTerminal, agentstate.EventResultFacts}
	if got := eventTypes(result.Events); !reflect.DeepEqual(got, want) {
		t.Fatalf("事件 = %#v，期望 %#v", got, want)
	}
	snapshot := projectEvents(t, result)
	if snapshot.State != agentstate.StateSucceeded || snapshot.ReconciliationRequired {
		t.Fatalf("成功投影 = %#v", snapshot)
	}
}

func Test假工具失败保留完整失败事实(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := scriptedTool{process: scriptedProcess{identity: validProcess(), observation: Observation{ProcessExited: true, ToolTerminal: agentstate.ToolFailure, ResultFacts: agentstate.ResultFailed}}}
	result, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool)
	if err != nil || result.ReconciliationRequired || result.Observation.ToolTerminal != agentstate.ToolFailure {
		t.Fatalf("RunSynthetic() = %#v, %v", result, err)
	}
	if snapshot := projectEvents(t, result); snapshot.State != agentstate.StateFailed || snapshot.ReconciliationRequired {
		t.Fatalf("失败投影 = %#v", snapshot)
	}
}

func Test假工具中断不伪造终态(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := scriptedTool{process: scriptedProcess{identity: validProcess(), observation: Observation{ProcessExited: false, ToolTerminal: agentstate.ToolUnknown, ResultFacts: agentstate.ResultUnknown}}}
	result, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool)
	if err != nil || !result.ReconciliationRequired {
		t.Fatalf("RunSynthetic() = %#v, %v", result, err)
	}
	want := []agentstate.EventType{agentstate.EventProcessStarted, agentstate.EventLocalRecoveryDone}
	if got := eventTypes(result.Events); !reflect.DeepEqual(got, want) {
		t.Fatalf("中断事件 = %#v，期望 %#v", got, want)
	}
	if snapshot := projectEvents(t, result); snapshot.State != agentstate.StateRunning || !snapshot.ReconciliationRequired {
		t.Fatalf("中断投影 = %#v", snapshot)
	}
}

func Test假工具启动失败只报告无进程事实(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := scriptedTool{startError: ErrSyntheticStartRejected}
	result, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool)
	if err != nil {
		t.Fatalf("RunSynthetic() 错误 = %v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Type != agentstate.EventStartRejected || !result.Events[0].Facts.NoProcess {
		t.Fatalf("启动失败事件 = %#v", result.Events)
	}
	if _, found, readErr := ReadStartIntent(workspace); readErr != nil || !found {
		t.Fatalf("启动失败后意图 = found:%t err:%v", found, readErr)
	}
}

func Test未知启动错误不伪造无进程事实(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := scriptedTool{startError: errors.New("synthetic-secret-not-allowed")}
	result, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool)
	if !errors.Is(err, ErrSyntheticToolUnavailable) || !result.ReconciliationRequired {
		t.Fatalf("RunSynthetic() = %#v, %v", result, err)
	}
	if got := eventTypes(result.Events); !reflect.DeepEqual(got, []agentstate.EventType{agentstate.EventLocalRecoveryDone}) {
		t.Fatalf("未知启动错误事件 = %#v", got)
	}
}

func Test假工具等待中断不伪造终态(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := scriptedTool{process: scriptedProcess{identity: validProcess(), waitError: errors.New("synthetic-secret-not-allowed")}}
	result, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool)
	if !errors.Is(err, ErrSyntheticToolUnavailable) || !result.ReconciliationRequired {
		t.Fatalf("RunSynthetic() = %#v, %v", result, err)
	}
	if got := eventTypes(result.Events); !reflect.DeepEqual(got, []agentstate.EventType{agentstate.EventProcessStarted, agentstate.EventLocalRecoveryDone}) {
		t.Fatalf("等待中断事件 = %#v", got)
	}
}

func Test假工具证据缺失进入核对而不重复启动(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	process := validProcess()
	process.ExecutableDigest = "not-a-digest"
	tool := scriptedTool{process: scriptedProcess{identity: process}}
	result, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool)
	if !errors.Is(err, ErrProcessEvidenceInvalid) || !result.ReconciliationRequired {
		t.Fatalf("RunSynthetic() = %#v, %v", result, err)
	}
	if _, err := RunSynthetic(context.Background(), workspace, validLaunch(), tool); !errors.Is(err, ErrStartIntentExists) {
		t.Fatalf("重复启动错误 = %v，期望 %v", err, ErrStartIntentExists)
	}
}

func cleanupWorkspace(t *testing.T, workspace credential.Workspace) {
	t.Helper()
	if err := RemoveStartIntent(workspace); err != nil {
		t.Fatalf("RemoveStartIntent() 错误 = %v", err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatalf("Cleanup() 错误 = %v", err)
	}
}

func validLaunch() SyntheticLaunch {
	return SyntheticLaunch{Intent: validStartIntent(), BootID: "boot-1"}
}

func validProcess() ProcessIdentity {
	return ProcessIdentity{PID: 1001, StartedAt: time.Date(2026, 7, 22, 9, 1, 0, 0, time.UTC), ExecutableDigest: strings.Repeat("b", 64), BootID: "boot-1"}
}

func eventTypes(events []EventFact) []agentstate.EventType {
	result := make([]agentstate.EventType, 0, len(events))
	for _, event := range events {
		result = append(result, event.Type)
	}
	return result
}

func projectEvents(t *testing.T, result RunResult) agentstate.Snapshot {
	t.Helper()
	clock := fixedClock{now: time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)}
	coordinator, err := agentstate.NewCoordinator(clock)
	if err != nil {
		t.Fatalf("NewCoordinator() 错误 = %v", err)
	}
	if err := coordinator.Schedule(agentstate.TaskSchedule{TaskID: "task-1", NodeID: "node-1"}); err != nil {
		t.Fatalf("Schedule() 错误 = %v", err)
	}
	grant, err := coordinator.Claim(agentstate.ClaimRequest{RequestID: "claim-1", TaskID: "task-1", ExecutionID: "execution-1", NodeID: "node-1", AgentID: "agent-1", LeaseID: "lease-1", LeaseTTL: time.Minute})
	if err != nil {
		t.Fatalf("Claim() 错误 = %v", err)
	}
	for index, fact := range result.Events {
		if _, err := coordinator.Append(agentstate.Event{EventID: "event-" + string(rune('1'+index)), ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, Sequence: int64(index + 1), Type: fact.Type, Facts: fact.Facts}); err != nil {
			t.Fatalf("Append(%d) 错误 = %v", index, err)
		}
	}
	snapshot, err := coordinator.Snapshot(grant.ExecutionID)
	if err != nil {
		t.Fatalf("Snapshot() 错误 = %v", err)
	}
	return snapshot
}

type fixedClock struct {
	now time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.now
}

type scriptedTool struct {
	process    scriptedProcess
	startError error
}

func (tool scriptedTool) Start(context.Context, SyntheticLaunch) (SyntheticProcess, error) {
	if tool.startError != nil {
		return nil, tool.startError
	}
	return tool.process, nil
}

type scriptedProcess struct {
	identity    ProcessIdentity
	observation Observation
	waitError   error
}

func (process scriptedProcess) Identity() ProcessIdentity {
	return process.identity
}

func (process scriptedProcess) Wait(context.Context) (Observation, error) {
	return process.observation, process.waitError
}
