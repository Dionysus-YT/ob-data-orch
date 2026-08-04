package main

import (
	"context"
	"errors"
	"testing"

	"ob-data-orch/internal/agentworker"
)

type exportPreflightRunnerStub struct {
	calls *[]string
	found bool
	err   error
}

func (s *exportPreflightRunnerStub) RunNext(context.Context) (agentworker.Outcome, bool, error) {
	*s.calls = append(*s.calls, "export-preflight")
	return agentworker.Outcome{}, s.found, s.err
}

func TestConnectionAndPrecheckRunner按固定顺序领取两类工作(t *testing.T) {
	calls := make([]string, 0, 2)
	runner := connectionAndPrecheckRunner{
		connectionTests: &connectionTestRunnerStub{calls: &calls, found: true},
		prechecks:       &exportPreflightRunnerStub{calls: &calls, found: true},
	}

	_, found, err := runner.RunNext(context.Background())

	if err != nil || !found || !sameAgentCalls(calls, []string{"connection-test", "export-preflight"}) {
		t.Fatalf("RunNext() found=%t err=%v calls=%#v", found, err, calls)
	}
}

func TestConnectionAndPrecheckRunner连接测试失败时仍尝试预检查(t *testing.T) {
	calls := make([]string, 0, 2)
	runner := connectionAndPrecheckRunner{
		connectionTests: &connectionTestRunnerStub{calls: &calls, err: errors.New("synthetic connection test failure")},
		prechecks:       &exportPreflightRunnerStub{calls: &calls, found: true},
	}

	_, found, err := runner.RunNext(context.Background())

	if err == nil || !found || !sameAgentCalls(calls, []string{"connection-test", "export-preflight"}) {
		t.Fatalf("RunNext() found=%t err=%v calls=%#v", found, err, calls)
	}
}
