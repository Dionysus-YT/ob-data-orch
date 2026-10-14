package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/nodefacts"
)

func Test心跳诊断往返持久化且动态执行队列不改变检查版本(t *testing.T) {
	s, path := openTestStore(t)
	seedBaseFixture(t, s)
	observed, started := testTime.Add(time.Minute), testTime
	facts := AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: observed, StartedAt: &started,
		RuntimeVersions: &nodefacts.RuntimeVersions{Java: &nodefacts.InstalledVersion{Version: "1.8.0_491", Source: "JAVA_RELEASE"}}, RuntimeConfigurationDigest: strings.Repeat("a", 64),
		ExecutionObservation: &nodefacts.ExecutionObservation{Active: []nodefacts.ActiveExecution{{ExecutionID: "synthetic-execution", LeaseID: "synthetic-lease", LeaseEpoch: 1, LastEventSeq: 3}}}, LogQueue: &nodefacts.LogQueue{PendingBatches: 1, PendingBytes: 512, LastUploadResult: "UNKNOWN"}}
	input := AgentHeartbeat{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "synthetic-boot", RequestID: "diagnostic-1", ObservedAt: observed, ReceivedAt: observed, CapacityTotal: 1, Facts: facts}
	revision, err := s.RecordAgentHeartbeat(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.RequestID = "diagnostic-2"
	input.Facts.ExecutionObservation.Active[0].LastEventSeq = 4
	input.Facts.LogQueue.PendingBytes = 1024
	if next, err := s.RecordAgentHeartbeat(context.Background(), input); err != nil || next != revision {
		t.Fatal("动态诊断错误地使检查失效", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	node, err := reopened.GetExecutionNode(context.Background(), "node-1")
	if err != nil || node.Agent.EnvironmentFacts.StartedAt == nil || node.Agent.EnvironmentFacts.ExecutionObservation.Active[0].LastEventSeq != 4 || node.Agent.EnvironmentFacts.LogQueue.PendingBytes != 1024 {
		t.Fatal("控制面重启丢失心跳诊断", err)
	}
	input.RequestID = "diagnostic-3"
	input.Facts.RuntimeVersions.Java.Version = "1.8.0_492"
	if next, err := reopened.RecordAgentHeartbeat(context.Background(), input); err != nil || next != revision+1 {
		t.Fatal("安装版本变化未失效固定检查", err)
	}
}

func Test活动执行读取按账号节点Agent和当前租约过滤(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 32<<30)
	submitBudgetTask(t, s, "task-observation", "1G")
	if _, found, err := s.ClaimNextExecution(context.Background(), budgetClaim("observation")); err != nil || !found {
		t.Fatal("合成领取失败", err)
	}
	read := func(subject, node, agent string, global bool) []NodeExecutionBinding {
		t.Helper()
		items, err := s.ListAuthorizedNodeExecutionBindings(context.Background(), node, agent, subject, global)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	items := read("subject-1", "node-1", "agent-1", false)
	if len(items) != 1 || items[0].LeaseID != "lease-observation" || items[0].LeaseEpoch != 1 {
		t.Fatal("授权租约没有投影")
	}
	for _, scope := range []struct{ subject, node, agent string }{{"unknown", "node-1", "agent-1"}, {"subject-1", "unknown", "agent-1"}, {"subject-1", "node-1", "unknown"}} {
		if len(read(scope.subject, scope.node, scope.agent, false)) != 0 {
			t.Fatal("账号或绑定扩大了读取")
		}
	}
	if _, err := s.db.Exec(`INSERT INTO auth_subjects VALUES ('subject-other','external-other','Synthetic Other','ACTIVE',NULL,?,?)`, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	if len(read("subject-other", "node-1", "agent-1", false)) != 0 {
		t.Fatal("节点管理范围泄露其他账号租约")
	}
	if len(read("subject-other", "node-1", "agent-1", true)) != 1 {
		t.Fatal("可信全局读取没有投影")
	}
	if _, err := s.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-other'`); err != nil {
		t.Fatal(err)
	}
	if len(read("subject-other", "node-1", "agent-1", true)) != 0 {
		t.Fatal("失效账号保留全局读取")
	}
}
