package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test持续授权使重启后的自动完整复验通过且保留管理意图(t *testing.T) {
	for _, management := range []string{"ENABLED", "DISABLED", "MAINTENANCE"} {
		t.Run(management, func(t *testing.T) {
			s, _ := openTestStore(t)
			seedBaseFixture(t, s)
			if _, err := s.db.Exec(`UPDATE agents SET environment_inspection_authorized=0 WHERE agent_id='agent-1'`); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			node, err := s.GetExecutionNode(ctx, "node-1")
			if err != nil {
				t.Fatal(err)
			}
			now := testTime.Add(time.Minute)
			digest := ExecutionNodeRuntimeConfigurationDigest(node)
			heartbeat := AgentHeartbeat{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "before-restart", RequestID: "before-heartbeat", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, RuntimeConfigurationDigest: digest}}
			revision, err := s.RecordAgentHeartbeat(ctx, heartbeat)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`UPDATE execution_nodes SET management_state=? WHERE node_id='node-1'`, management); err != nil {
				t.Fatal(err)
			}
			node, _ = s.GetExecutionNode(ctx, "node-1")
			if _, err := s.RequestExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRequest{NodeID: node.NodeID, ActorSubjectID: "subject-1", ExpectedRevision: node.Revision, CheckID: "before-check", RequestID: "before-request", RequestedAt: now, VersionProbeAllowed: true}); err != nil {
				t.Fatal(err)
			}
			node, _ = s.GetExecutionNode(ctx, "node-1")
			if node.ManagementState != management || !node.Agent.EnvironmentInspectionAuthorized {
				t.Fatal("手动检查改变管理意图或没有持久授权")
			}
			if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, AgentExecutionNodeEnvironmentCheckCompletion{NodeID: node.NodeID, AgentID: "agent-1", CheckID: "before-check", FactsRevision: revision, Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "before-completion", CompletedAt: now, Inspection: completeInspectionFixture(digest, "before-check", true, now)}); err != nil {
				t.Fatal(err)
			}
			heartbeat.BootID, heartbeat.RequestID = "after-restart", "after-heartbeat"
			heartbeat.ObservedAt = now.Add(time.Second)
			heartbeat.ReceivedAt, heartbeat.Facts.ObservedAt = heartbeat.ObservedAt, heartbeat.ObservedAt
			next, err := s.RecordAgentHeartbeat(ctx, heartbeat)
			if err != nil || next == revision {
				t.Fatal("重启没有更新事实版本", err)
			}
			node, _ = s.GetExecutionNode(ctx, "node-1")
			if node.Agent.EnvironmentFacts.RuntimeInspection != nil {
				t.Fatal("重启沿用旧的检查证据")
			}
			if queued, err := s.EnsureCurrentExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRefresh{NodeID: node.NodeID, AgentID: "agent-1", CheckID: "after-check", RequestID: heartbeat.RequestID, FactsRevision: next, RequestedAt: heartbeat.ReceivedAt}); err != nil || !queued {
				t.Fatal("重启没有排队复验", err)
			}
			pending, found, err := s.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-1", node.NodeID)
			if err != nil || !found || !pending.VersionProbeAllowed || pending.CheckID != "after-check" {
				t.Fatal("持续授权没有进入新检查", err)
			}
			if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, AgentExecutionNodeEnvironmentCheckCompletion{NodeID: node.NodeID, AgentID: "agent-1", CheckID: pending.CheckID, FactsRevision: next, Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "after-completion", CompletedAt: heartbeat.ReceivedAt, Inspection: completeInspectionFixture(digest, pending.CheckID, true, heartbeat.ReceivedAt)}); err != nil {
				t.Fatal(err)
			}
			node, err = s.GetExecutionNode(ctx, node.NodeID)
			if err != nil || node.ManagementState != management || node.EnvironmentCheck.Status != "PASSED" || node.EnvironmentCheck.FactsRevision != next {
				t.Fatal("自动复验没有恢复当前证据或改变管理意图", err)
			}
			var allowed bool
			if err := s.db.QueryRow(`SELECT json_extract(safe_diff_json,'$.versionProbeAllowed') FROM audit_events WHERE action='EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED' AND object_id='node-1'`).Scan(&allowed); err != nil || !allowed {
				t.Fatal("自动复验没有保存授权快照", err)
			}
		})
	}
}

func Test固定复验授权撤销或身份失效后拒绝旧许可回执(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	node, _ := s.GetExecutionNode(ctx, "node-1")
	digest := ExecutionNodeRuntimeConfigurationDigest(node)
	now := testTime.Add(time.Minute)
	revision, err := s.RecordAgentHeartbeat(ctx, AgentHeartbeat{AgentID: "agent-1", NodeID: node.NodeID, ProtocolVersion: "agent-v1", BootID: "authorized-boot", RequestID: "authorized-heartbeat", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, RuntimeConfigurationDigest: digest}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRequest{NodeID: node.NodeID, ActorSubjectID: "subject-1", ExpectedRevision: node.Revision, CheckID: "authorized-check", RequestID: "authorized-request", RequestedAt: now, VersionProbeAllowed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE agents SET environment_inspection_authorized=0 WHERE agent_id='agent-1'`); err != nil {
		t.Fatal(err)
	}
	pending, found, err := s.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-1", node.NodeID)
	if err != nil || !found || pending.VersionProbeAllowed {
		t.Fatal("撤销后仍下发旧许可", err)
	}
	completion := AgentExecutionNodeEnvironmentCheckCompletion{NodeID: node.NodeID, AgentID: "agent-1", CheckID: pending.CheckID, FactsRevision: revision, Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "late-completion", CompletedAt: now, Inspection: completeInspectionFixture(digest, pending.CheckID, true, now)}
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, completion); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
		t.Fatal("撤销后旧许可回执覆盖当前状态", err)
	}
	for _, status := range []string{"REVOKED", "REPLACED"} {
		if _, err := s.db.Exec(`UPDATE agents SET status=?,environment_inspection_authorized=1 WHERE agent_id='agent-1'`, status); err != nil {
			t.Fatal(err)
		}
		if _, found, err := s.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-1", node.NodeID); err != nil || found {
			t.Fatal("失效机器仍能领取检查", err)
		}
		if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, completion); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
			t.Fatal("失效机器仍能完成检查", err)
		}
	}
}

func Test关联只传递本次材料授权且重放不能恢复被撤销的授权(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "旧范围", true: "固定复验授权"}[allowed], func(t *testing.T) {
			s, _ := openTestStore(t)
			seedBaseFixture(t, s)
			ctx := context.Background()
			if _, err := s.db.Exec(`UPDATE execution_nodes SET management_state='ENABLED',environment_check_id='prior-agent-check',environment_check_status='PASSED',environment_check_code='TOOL_RUNTIME_READY',environment_check_facts_revision=1 WHERE node_id='node-1'`); err != nil {
				t.Fatal(err)
			}
			digest := make([]byte, 32)
			if err := s.IssueAgentEnrollment(ctx, AgentEnrollmentIssue{EnrollmentID: "new-enrollment", NodeID: "node-1", ActorID: "subject-1", RequestID: "new-issue", TokenDigest: digest, ExpiresAt: testTime.Add(time.Hour), CreatedAt: testTime, EnvironmentInspectionAuthorized: allowed}); err != nil {
				t.Fatal(err)
			}
			exchange := AgentEnrollmentExchange{EnrollmentID: "new-enrollment", NodeID: "node-1", AgentID: "new-agent", RequestID: "new-exchange", ProtocolVersion: "agent-v1", EnrollmentMaterialDigest: digest, CredentialDigest: append([]byte{1}, make([]byte, 31)...), ExchangedAt: testTime.Add(time.Minute)}
			result, err := s.ExchangeAgentEnrollment(ctx, exchange)
			if err != nil {
				t.Fatal(err)
			}
			node, err := s.GetExecutionNode(ctx, "node-1")
			if err != nil || node.Agent.AgentID != "new-agent" || node.Agent.EnvironmentInspectionAuthorized != allowed {
				t.Fatal("关联从旧身份继承了范围或丢失新授权", err)
			}
			if node.EnvironmentCheck.Status != "NOT_CHECKED" || node.EnvironmentCheck.CheckID != "" || node.EnvironmentCheck.FactsRevision != 0 || node.ManagementState != "ENABLED" || result.RuntimeConfiguration.Revision != node.Revision {
				t.Fatal("新身份沿用旧检查或改变管理意图")
			}
			now := exchange.ExchangedAt.Add(time.Second)
			runtimeDigest := ExecutionNodeRuntimeConfigurationDigest(node)
			revision, err := s.RecordAgentHeartbeat(ctx, AgentHeartbeat{AgentID: exchange.AgentID, NodeID: node.NodeID, ProtocolVersion: "agent-v1", BootID: "new-agent-boot", RequestID: "new-agent-heartbeat", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, RuntimeConfigurationDigest: runtimeDigest}})
			if err != nil || revision != 1 {
				t.Fatal("新身份首份事实版本错误", err)
			}
			if queued, err := s.EnsureCurrentExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRefresh{NodeID: node.NodeID, AgentID: exchange.AgentID, CheckID: "new-agent-check", RequestID: "new-agent-refresh", FactsRevision: revision, RequestedAt: now}); err != nil || !queued {
				t.Fatal("新身份事实同号时未重新排队检查", err)
			}
			pending, found, err := s.GetPendingExecutionNodeEnvironmentCheck(ctx, exchange.AgentID, node.NodeID)
			if err != nil || !found || pending.CheckID != "new-agent-check" || pending.VersionProbeAllowed != allowed {
				t.Fatal("新检查没有绑定新身份的授权", err)
			}
			beforeReplay, _ := s.GetExecutionNode(ctx, node.NodeID)
			if _, err := s.db.Exec(`UPDATE agents SET environment_inspection_authorized=0 WHERE agent_id='new-agent'`); err != nil {
				t.Fatal(err)
			}
			if result, err := s.ExchangeAgentEnrollment(ctx, exchange); err != nil || !result.Replayed {
				t.Fatal("同一关联不能安全重放", err)
			}
			node, _ = s.GetExecutionNode(ctx, "node-1")
			if node.Agent.EnvironmentInspectionAuthorized || node.EnvironmentCheck.CheckID != beforeReplay.EnvironmentCheck.CheckID || node.EnvironmentCheck.Status != beforeReplay.EnvironmentCheck.Status || node.Revision != beforeReplay.Revision {
				t.Fatal("重放重新授予已经撤销的权限或清除了新身份检查")
			}
		})
	}
}

func Test失效主体不能授予持续复验且失败事务不改变节点(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	if _, err := s.db.Exec(`UPDATE agents SET environment_inspection_authorized=0 WHERE agent_id='agent-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-1'`); err != nil {
		t.Fatal(err)
	}
	before, _ := s.GetExecutionNode(context.Background(), "node-1")
	if _, err := s.RequestExecutionNodeEnvironmentCheck(context.Background(), ExecutionNodeEnvironmentCheckRequest{NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: before.Revision, CheckID: "inactive-check", RequestID: "inactive-request", RequestedAt: testTime, VersionProbeAllowed: true}); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
		t.Fatal("失效主体仍授予持续授权", err)
	}
	after, _ := s.GetExecutionNode(context.Background(), "node-1")
	if after.Agent.EnvironmentInspectionAuthorized || after.Revision != before.Revision || after.EnvironmentCheck.CheckID != before.EnvironmentCheck.CheckID {
		t.Fatal("失败的授权事务留下节点或授权变更")
	}
	if err := s.IssueAgentEnrollment(context.Background(), AgentEnrollmentIssue{EnrollmentID: "inactive-enrollment", NodeID: "node-1", ActorID: "subject-1", RequestID: "inactive-issue", TokenDigest: make([]byte, 32), ExpiresAt: testTime.Add(time.Hour), CreatedAt: testTime, EnvironmentInspectionAuthorized: true}); !errors.Is(err, ErrEnrollmentRejected) {
		t.Fatal("失效主体仍签发持续授权材料", err)
	}
}
