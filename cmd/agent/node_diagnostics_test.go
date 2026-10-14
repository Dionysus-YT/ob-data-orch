package main

import (
	"context"
	"testing"
	"time"

	"ob-data-orch/internal/nodefacts"
)

type observedConnectionRunner struct{ connectionTestRunnerStub }

func (r observedConnectionRunner) ExecutionObservation() *nodefacts.ExecutionObservation {
	return &nodefacts.ExecutionObservation{Active: []nodefacts.ActiveExecution{{ExecutionID: "synthetic-execution", LeaseID: "synthetic-lease", LeaseEpoch: 1, LastEventSeq: 3}}}
}
func (r observedConnectionRunner) LogQueueStats() *nodefacts.LogQueue {
	return &nodefacts.LogQueue{PendingBatches: 2, PendingBytes: 1024, LastUploadResult: "UNKNOWN"}
}

func TestAgent主循环将进程和执行队列采集接入心跳(t *testing.T) {
	calls := []string{}
	heartbeat := &heartbeatProtocolStub{calls: &calls}
	runner := observedConnectionRunner{connectionTestRunnerStub{calls: &calls}}
	if err := runAgentProtocolLoop(context.Background(), heartbeat, &runner, "synthetic-boot", "WINDOWS", "AMD64", time.Minute, time.Second, true, testLogger()); err != nil {
		t.Fatal(err)
	}
	facts := heartbeat.input.Facts
	if facts.StartedAt == nil || !facts.StartedAt.Equal(agentStartedAt) || facts.StartedAt.After(facts.ObservedAt) || facts.ExecutionObservation.Active[0].LastEventSeq != 3 || facts.LogQueue.PendingBatches != 2 {
		t.Fatal("装配层没有采集真实诊断端口")
	}
}
