package agentenvironment

import (
	"context"
	"testing"
	"time"

	"ob-data-orch/internal/agentlocalpreflight"
	"ob-data-orch/internal/agentwire"
)

func TestWorker只回传固定运行时结果(t *testing.T) {
	t.Parallel()
	protocol := &recordingProtocol{}
	worker := Worker{
		Protocol: protocol, Runtime: agentlocalpreflight.RuntimeValidatorFunc(func(context.Context) error { return nil }),
		Clock: func() time.Time { return time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC) }, BootID: "boot-1",
	}
	ran, err := worker.Run(context.Background(), "check-1", 3)
	if err != nil || !ran {
		t.Fatalf("Run() = %v, %v", ran, err)
	}
	if protocol.bootID != "boot-1" || protocol.checkID != "check-1" || protocol.factsRevision != 3 || protocol.status != "PASSED" || protocol.code != "TOOL_RUNTIME_READY" {
		t.Fatalf("completion = %#v", protocol)
	}
}

func TestWorker将运行时异常收敛为稳定失败码(t *testing.T) {
	t.Parallel()
	protocol := &recordingProtocol{}
	worker := Worker{
		Protocol: protocol, Runtime: agentlocalpreflight.RuntimeValidatorFunc(func(context.Context) error { return agentlocalpreflight.ErrToolRuntimeInvalid }),
		Clock: func() time.Time { return time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC) }, BootID: "boot-1",
	}
	_, err := worker.Run(context.Background(), "check-1", 3)
	if err != nil || protocol.status != "FAILED" || protocol.code != "TOOL_RUNTIME_INVALID" {
		t.Fatalf("Run() err=%v completion=%#v", err, protocol)
	}
}

func TestWorker拒绝空检查标识且不回传(t *testing.T) {
	t.Parallel()
	protocol := &recordingProtocol{}
	worker := Worker{Protocol: protocol, Runtime: agentlocalpreflight.RuntimeValidatorFunc(func(context.Context) error { return nil }), Clock: time.Now, BootID: "boot-1"}
	ran, err := worker.Run(context.Background(), "", 1)
	if err != nil || ran || protocol.calls != 0 {
		t.Fatalf("Run() = %v, %v, calls=%d", ran, err, protocol.calls)
	}
}

type recordingProtocol struct {
	bootID        string
	checkID       string
	factsRevision int64
	status        string
	code          string
	calls         int
	err           error
}

func (p *recordingProtocol) AgentIdentity() (agentwire.AgentIdentity, error) {
	if p.err != nil {
		return agentwire.AgentIdentity{}, p.err
	}
	return agentwire.AgentIdentity{AgentID: "agent-1", NodeID: "node-1"}, nil
}

func (p *recordingProtocol) CompleteExecutionNodeEnvironmentCheck(_ context.Context, bootID, checkID string, factsRevision int64, status, code string, _ time.Time) error {
	p.calls++
	p.bootID, p.checkID, p.factsRevision, p.status, p.code = bootID, checkID, factsRevision, status, code
	return p.err
}
