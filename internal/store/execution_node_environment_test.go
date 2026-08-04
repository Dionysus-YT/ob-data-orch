package store

import (
	"context"
	"testing"
	"time"
)

func Test执行节点固定环境检查通过后才能启用(t *testing.T) {
	t.Parallel()
	store, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	seedExecutionNodeEnvironmentFixture(t, store, now)

	factsRevision, err := store.RecordAgentHeartbeat(ctx, AgentHeartbeat{
		AgentID: "agent-environment-1", NodeID: "node-environment-1", ProtocolVersion: "agent-v1", BootID: "boot-environment-1",
		RequestID: "heartbeat-environment-1", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: now},
	})
	if err != nil || factsRevision != 1 {
		t.Fatalf("RecordAgentHeartbeat() = %d, %v, want 1, nil", factsRevision, err)
	}
	requested, err := store.RequestExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRequest{
		NodeID: "node-environment-1", ActorSubjectID: "subject-environment-1", ExpectedRevision: 1,
		CheckID: "check-environment-1", RequestID: "request-environment-1", RequestedAt: now.Add(time.Second),
	})
	if err != nil || requested != (ExecutionNodeEnvironmentCheckRequestResult{CheckID: "check-environment-1", Revision: 2}) {
		t.Fatalf("RequestExecutionNodeEnvironmentCheck() = %#v, %v", requested, err)
	}
	pending, found, err := store.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-environment-1", "node-environment-1")
	if err != nil || !found || pending.CheckID != "check-environment-1" {
		t.Fatalf("GetPendingExecutionNodeEnvironmentCheck() = %#v, %v, %v", pending, found, err)
	}
	if _, err := store.EnableExecutionNode(ctx, ExecutionNodeEnable{
		NodeID: "node-environment-1", ActorSubjectID: "subject-environment-1", ExpectedRevision: 2,
		RequestID: "enable-before-check", EnabledAt: now.Add(2 * time.Second), OnlineAfter: now.Add(-time.Minute),
	}); err != ErrExecutionNodeEnableRejected {
		t.Fatalf("EnableExecutionNode() before completion = %v, want ErrExecutionNodeEnableRejected", err)
	}
	if err := store.CompleteExecutionNodeEnvironmentCheck(ctx, AgentExecutionNodeEnvironmentCheckCompletion{
		NodeID: "node-environment-1", AgentID: "agent-environment-1", CheckID: "check-environment-1", FactsRevision: factsRevision,
		Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "complete-environment-1", CompletedAt: now.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("CompleteExecutionNodeEnvironmentCheck() error = %v", err)
	}
	revision, err := store.EnableExecutionNode(ctx, ExecutionNodeEnable{
		NodeID: "node-environment-1", ActorSubjectID: "subject-environment-1", ExpectedRevision: 3,
		RequestID: "enable-environment-1", EnabledAt: now.Add(4 * time.Second), OnlineAfter: now.Add(-time.Minute),
	})
	if err != nil || revision != 4 {
		t.Fatalf("EnableExecutionNode() = %d, %v, want 4, nil", revision, err)
	}
	node, err := store.GetExecutionNode(ctx, "node-environment-1")
	if err != nil || node.ManagementState != "ENABLED" || node.EnvironmentCheck.Status != "PASSED" || node.EnvironmentCheck.Code != "TOOL_RUNTIME_READY" || node.EnvironmentCheck.FactsRevision != factsRevision {
		t.Fatalf("GetExecutionNode() = %#v, %v", node, err)
	}
}

func Test执行节点环境事实变化会自动重新排队且保留启用状态(t *testing.T) {
	t.Parallel()
	store, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.July, 31, 1, 0, 0, 0, time.UTC)
	seedExecutionNodeEnvironmentFixture(t, store, now)

	factsRevision, err := store.RecordAgentHeartbeat(ctx, AgentHeartbeat{
		AgentID: "agent-environment-1", NodeID: "node-environment-1", ProtocolVersion: "agent-v1", BootID: "boot-environment-1",
		RequestID: "heartbeat-refresh-1", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: now},
	})
	if err != nil || factsRevision != 1 {
		t.Fatalf("首个 RecordAgentHeartbeat() = %d, %v", factsRevision, err)
	}
	queued, err := store.EnsureCurrentExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRefresh{
		NodeID: "node-environment-1", AgentID: "agent-environment-1", CheckID: "check-refresh-1", RequestID: "heartbeat-refresh-1", FactsRevision: factsRevision, RequestedAt: now,
	})
	if err != nil || !queued {
		t.Fatalf("EnsureCurrentExecutionNodeEnvironmentCheck() = %v, %v, want true, nil", queued, err)
	}
	if err := store.CompleteExecutionNodeEnvironmentCheck(ctx, AgentExecutionNodeEnvironmentCheckCompletion{
		NodeID: "node-environment-1", AgentID: "agent-environment-1", CheckID: "check-refresh-1", FactsRevision: factsRevision,
		Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "complete-refresh-1", CompletedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("完成首个自动环境检查失败: %v", err)
	}
	if _, err := store.EnableExecutionNode(ctx, ExecutionNodeEnable{
		NodeID: "node-environment-1", ActorSubjectID: "subject-environment-1", ExpectedRevision: 3,
		RequestID: "enable-refresh-1", EnabledAt: now.Add(2 * time.Second), OnlineAfter: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("EnableExecutionNode() error = %v", err)
	}

	factsRevision, err = store.RecordAgentHeartbeat(ctx, AgentHeartbeat{
		AgentID: "agent-environment-1", NodeID: "node-environment-1", ProtocolVersion: "agent-v1", BootID: "boot-environment-2",
		RequestID: "heartbeat-refresh-2", ObservedAt: now.Add(3 * time.Second), ReceivedAt: now.Add(3 * time.Second), CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: now.Add(3 * time.Second)},
	})
	if err != nil || factsRevision != 2 {
		t.Fatalf("重启后 RecordAgentHeartbeat() = %d, %v, want 2, nil", factsRevision, err)
	}
	queued, err = store.EnsureCurrentExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRefresh{
		NodeID: "node-environment-1", AgentID: "agent-environment-1", CheckID: "check-refresh-2", RequestID: "heartbeat-refresh-2", FactsRevision: factsRevision, RequestedAt: now.Add(3 * time.Second),
	})
	if err != nil || !queued {
		t.Fatalf("重启后 EnsureCurrentExecutionNodeEnvironmentCheck() = %v, %v, want true, nil", queued, err)
	}
	node, err := store.GetExecutionNode(ctx, "node-environment-1")
	if err != nil || node.ManagementState != "ENABLED" || node.EnvironmentCheck.Status != "PENDING" || node.EnvironmentCheck.FactsRevision != factsRevision {
		t.Fatalf("重启后的节点环境投影 = %#v, %v", node, err)
	}
	pending, found, err := store.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-environment-1", "node-environment-1")
	if err != nil || !found || pending.CheckID != "check-refresh-2" {
		t.Fatalf("重启后待处理环境检查 = %#v, %v, %v", pending, found, err)
	}
}

func Test执行节点环境检查拒绝过期事实版本(t *testing.T) {
	t.Parallel()
	store, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.July, 28, 8, 10, 0, 0, time.UTC)
	seedExecutionNodeEnvironmentFixture(t, store, now)
	_, err := store.RecordAgentHeartbeat(ctx, AgentHeartbeat{
		AgentID: "agent-environment-1", NodeID: "node-environment-1", ProtocolVersion: "agent-v1", BootID: "boot-environment-1",
		RequestID: "heartbeat-environment-2", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: now},
	})
	if err != nil {
		t.Fatalf("RecordAgentHeartbeat() error = %v", err)
	}
	_, err = store.RequestExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRequest{
		NodeID: "node-environment-1", ActorSubjectID: "subject-environment-1", ExpectedRevision: 1,
		CheckID: "check-environment-2", RequestID: "request-environment-2", RequestedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("RequestExecutionNodeEnvironmentCheck() error = %v", err)
	}
	_, err = store.RecordAgentHeartbeat(ctx, AgentHeartbeat{
		AgentID: "agent-environment-1", NodeID: "node-environment-1", ProtocolVersion: "agent-v1", BootID: "boot-environment-2",
		RequestID: "heartbeat-environment-3", ObservedAt: now.Add(2 * time.Second), ReceivedAt: now.Add(2 * time.Second), CapacityTotal: 1, CapacityUsed: 0,
		Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: now.Add(2 * time.Second)},
	})
	if err != nil {
		t.Fatalf("RecordAgentHeartbeat() drift error = %v", err)
	}
	if err := store.CompleteExecutionNodeEnvironmentCheck(ctx, AgentExecutionNodeEnvironmentCheckCompletion{
		NodeID: "node-environment-1", AgentID: "agent-environment-1", CheckID: "check-environment-2", FactsRevision: 1,
		Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "complete-environment-2", CompletedAt: now.Add(3 * time.Second),
	}); err != ErrExecutionNodeEnvironmentCheckRequired {
		t.Fatalf("CompleteExecutionNodeEnvironmentCheck() = %v, want ErrExecutionNodeEnvironmentCheckRequired", err)
	}
}

func seedExecutionNodeEnvironmentFixture(t *testing.T, store *Store, now time.Time) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO auth_subjects(subject_id, external_subject, display_name, account_status, directory_revision, created_at, updated_at) VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, []any{"subject-environment-1", "external-environment-1", "Environment User", utcText(now), utcText(now)}},
		{`INSERT INTO execution_nodes(node_id, display_name, normalized_name, platform, management_state, allowed_roots_json, tool_config_ref, revision, created_by, created_at, updated_at) VALUES (?, ?, ?, 'WINDOWS_AMD64', 'DISABLED', '["E:\\environment"]', NULL, 1, ?, ?, ?)`, []any{"node-environment-1", "Environment Node", "environment node", "subject-environment-1", utcText(now), utcText(now)}},
		{`INSERT INTO agents(agent_id, node_id, credential_digest, credential_revision, status, protocol_version, boot_id, last_heartbeat_at, capacity_total, capacity_used, facts_json, facts_revision, created_at, revoked_at) VALUES (?, ?, X'010203', 1, 'ACTIVE', 'agent-v1', NULL, NULL, 1, 0, NULL, 0, ?, NULL)`, []any{"agent-environment-1", "node-environment-1", utcText(now)}},
	}
	for index, statement := range statements {
		if _, err := store.db.ExecContext(context.Background(), statement.query, statement.args...); err != nil {
			t.Fatalf("seed statement %d: %v", index+1, err)
		}
	}
}
