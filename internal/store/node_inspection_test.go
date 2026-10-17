package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/nodefacts"
)

func completeInspectionFixture(digest, checkID string, allowed bool, now time.Time) *nodefacts.RuntimeInspection {
	report := &nodefacts.RuntimeInspection{CheckID: checkID, RuntimeConfigurationDigest: digest, ObservedAt: now, VersionProbeAllowed: allowed, DirectLoad: &nodefacts.DirectLoadConfiguration{Status: "MISSING"}}
	for _, kind := range nodefacts.InspectionKinds() {
		item := nodefacts.InspectionItem{Kind: kind, Status: "PASSED", Code: "READY", Required: kind == "JAVA_RUNTIME" || kind == "OBDUMPER_RUNTIME" || kind == "CONNECTOR" || kind == "WORKSPACE" || kind == "LOG_QUEUE" || kind == "RESOURCES" || kind == "RUNTIME_LAYOUT"}
		if kind == "JAVA_RUNTIME" {
			item.Version = "1.8.0_491"
		}
		if kind == "OBDUMPER_RUNTIME" || kind == "OBLOADER_RUNTIME" {
			item.Version = "4.3.5-RELEASE"
		}
		if !allowed && item.Version != "" {
			item.Version = ""
			item.Status, item.Code = "UNKNOWN", "VERSION_PROBE_NOT_REQUESTED"
		}
		report.Items = append(report.Items, item)
	}
	return report
}

func Test固定版本授权绑定检查且完整回执原子保存并由心跳保留(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	node, err := s.GetExecutionNode(ctx, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	now := testTime.Add(time.Minute)
	digest := ExecutionNodeRuntimeConfigurationDigest(node)
	heartbeat := AgentHeartbeat{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "boot-1", RequestID: "inspection-heartbeat", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, RuntimeConfigurationDigest: digest}}
	revision, err := s.RecordAgentHeartbeat(ctx, heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	node, _ = s.GetExecutionNode(ctx, "node-1")
	_, err = s.RequestExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRequest{NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: node.Revision, CheckID: "manual-check", RequestID: "manual-request", RequestedAt: now, VersionProbeAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	// 复现真实领取顺序：人工请求后先记录心跳并核对自动续接，再向 Agent 返回待处理检查。
	heartbeat.RequestID = "manual-claim-heartbeat"
	heartbeat.ObservedAt = now.Add(time.Second)
	heartbeat.ReceivedAt, heartbeat.Facts.ObservedAt = heartbeat.ObservedAt, heartbeat.ObservedAt
	if next, err := s.RecordAgentHeartbeat(ctx, heartbeat); err != nil || next != revision {
		t.Fatal("同一运行环境的领取心跳改变了事实绑定", err)
	}
	if queued, err := s.EnsureCurrentExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRefresh{NodeID: "node-1", AgentID: "agent-1", CheckID: "automatic-claim-check", RequestID: heartbeat.RequestID, FactsRevision: revision, RequestedAt: heartbeat.ReceivedAt}); err != nil || queued {
		t.Fatal("领取心跳把人工版本检查替换为未授权的自动检查", err)
	}
	pending, found, err := s.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-1", "node-1")
	if err != nil || !found || pending.CheckID != "manual-check" || !pending.VersionProbeAllowed {
		t.Fatal("手动固定查询授权没有持久绑定")
	}
	report := completeInspectionFixture(digest, "manual-check", true, now)
	completion := AgentExecutionNodeEnvironmentCheckCompletion{NodeID: "node-1", AgentID: "agent-1", CheckID: "manual-check", FactsRevision: revision, Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "manual-completion", CompletedAt: now}
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, completion); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
		t.Fatal("只有布局的旧回执冒充实际版本通过")
	}
	completion.Inspection = report
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, completion); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, completion); err != nil {
		t.Fatal("相同完整回执不能幂等重放", err)
	}
	changed := *report
	changed.ObservedAt = now.Add(time.Second)
	completion.Inspection = &changed
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, completion); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
		t.Fatal("不同回执覆盖已确认结果")
	}
	heartbeat.RequestID = "inspection-next-heartbeat"
	heartbeat.ObservedAt = now.Add(time.Second)
	heartbeat.Facts.ObservedAt = heartbeat.ObservedAt
	heartbeat.ReceivedAt = heartbeat.ObservedAt
	if next, err := s.RecordAgentHeartbeat(ctx, heartbeat); err != nil || next != revision {
		t.Fatal(err)
	}
	node, err = s.GetExecutionNode(ctx, "node-1")
	if err != nil || node.Agent.EnvironmentFacts.RuntimeInspection == nil {
		t.Fatal("独立心跳清除了已确认检查证据")
	}
	activity, err := s.ReadNodeActivity(ctx, "node-1", "subject-1", "manual-check")
	if err != nil || activity.CheckSource != "SUBJECT" || len(activity.Operations) == 0 || len(activity.Events) == 0 {
		t.Fatal("节点事件与操作来源没有进入读取链")
	}
	encoded, _ := json.Marshal(activity)
	if strings.Contains(string(encoded), "subject-1") || strings.Contains(string(encoded), "safe_diff") {
		t.Fatal("活动摘要携带账号或原始差异")
	}
	for index := range 50 {
		_, err := s.db.Exec(`INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at)
			VALUES(?,'SUBJECT','subject-1','EXECUTION_NODE_UPDATED','EXECUTION_NODE','node-1','SUCCEEDED',?,'{}',?)`, fmt.Sprintf("recent-update-%d", index), fmt.Sprintf("recent-request-%d", index), utcText(now.Add(time.Duration(index+1)*time.Second)))
		if err != nil {
			t.Fatal(err)
		}
	}
	activity, err = s.ReadNodeActivity(ctx, "node-1", "subject-1", "manual-check")
	if err != nil || activity.CheckSource != "SUBJECT" || len(activity.Operations) != 5 || len(activity.Events) == 0 {
		t.Fatal("近期大量编辑挤掉检查来源或已保存节点事件")
	}
	if _, err := s.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-1'`); err != nil {
		t.Fatal(err)
	}
	activity, err = s.ReadNodeActivity(ctx, "node-1", "subject-1", "manual-check")
	if err != nil || len(activity.Events) != 0 || len(activity.Operations) != 0 {
		t.Fatal("失效账号仍读取节点活动")
	}
}

func Test人工检查绑定事实漂移后拒绝新旧回执且新检查复核持续授权(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	node, err := s.GetExecutionNode(ctx, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	now := testTime.Add(time.Minute)
	digest := ExecutionNodeRuntimeConfigurationDigest(node)
	heartbeat := AgentHeartbeat{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "boot-1", RequestID: "manual-bound-heartbeat", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, RuntimeConfigurationDigest: digest}}
	revision, err := s.RecordAgentHeartbeat(ctx, heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRequest{NodeID: "node-1", ActorSubjectID: "subject-1", ExpectedRevision: node.Revision, CheckID: "manual-bound-check", RequestID: "manual-bound-request", RequestedAt: now, VersionProbeAllowed: true}); err != nil {
		t.Fatal(err)
	}
	heartbeat.BootID, heartbeat.RequestID = "boot-2", "manual-bound-restart"
	heartbeat.ObservedAt = now.Add(time.Second)
	heartbeat.ReceivedAt, heartbeat.Facts.ObservedAt = heartbeat.ObservedAt, heartbeat.ObservedAt
	next, err := s.RecordAgentHeartbeat(ctx, heartbeat)
	if err != nil || next == revision {
		t.Fatal("重启没有更新绑定事实", err)
	}
	// 即使自动续接尚未提交，旧人工检查也不能把新机器事实当作原授权范围的回执。
	for _, completionRevision := range []int64{revision, next} {
		input := AgentExecutionNodeEnvironmentCheckCompletion{NodeID: "node-1", AgentID: "agent-1", CheckID: "manual-bound-check", FactsRevision: completionRevision, Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "manual-bound-completion", CompletedAt: heartbeat.ReceivedAt, Inspection: completeInspectionFixture(digest, "manual-bound-check", true, heartbeat.ReceivedAt)}
		if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, input); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
			t.Fatal("事实漂移后人工检查接受了新旧回执", err)
		}
	}
	if queued, err := s.EnsureCurrentExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRefresh{NodeID: "node-1", AgentID: "agent-1", CheckID: "auto-restart-check", RequestID: heartbeat.RequestID, FactsRevision: next, RequestedAt: heartbeat.ReceivedAt}); err != nil || !queued {
		t.Fatal("事实漂移没有重新排队自动检查", err)
	}
	pending, found, err := s.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-1", "node-1")
	if err != nil || !found || pending.CheckID != "auto-restart-check" || !pending.VersionProbeAllowed {
		t.Fatal("自动续接未复核当前机器的持续检查授权", err)
	}
}

func Test未授权身份的自动检查拒绝伪造与迟到回执(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	if _, err := s.db.Exec(`UPDATE agents SET environment_inspection_authorized=0 WHERE agent_id='agent-1'`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	node, _ := s.GetExecutionNode(ctx, "node-1")
	now := testTime.Add(time.Minute)
	digest := ExecutionNodeRuntimeConfigurationDigest(node)
	revision, err := s.RecordAgentHeartbeat(ctx, AgentHeartbeat{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "boot-1", RequestID: "auto-heartbeat", ObservedAt: now, ReceivedAt: now, CapacityTotal: 1, Facts: AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, RuntimeConfigurationDigest: digest}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureCurrentExecutionNodeEnvironmentCheck(ctx, ExecutionNodeEnvironmentCheckRefresh{NodeID: "node-1", AgentID: "agent-1", CheckID: "auto-check", RequestID: "auto-request", FactsRevision: revision, RequestedAt: now}); err != nil {
		t.Fatal(err)
	}
	pending, found, err := s.GetPendingExecutionNodeEnvironmentCheck(ctx, "agent-1", "node-1")
	if err != nil || !found || pending.VersionProbeAllowed {
		t.Fatal("自动检查得到未授予的启动权限")
	}
	report := completeInspectionFixture(digest, "auto-check", true, now)
	input := AgentExecutionNodeEnvironmentCheckCompletion{NodeID: "node-1", AgentID: "agent-1", CheckID: "auto-check", FactsRevision: revision, Status: "PASSED", Code: "TOOL_RUNTIME_READY", RequestID: "auto-completion", CompletedAt: now, Inspection: report}
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, input); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
		t.Fatal("自报授权被当成服务端许可")
	}
	input.Inspection = completeInspectionFixture(digest, "auto-check", false, now)
	input.Status, input.Code = nodefacts.InspectionResult(input.Inspection)
	input.Inspection.RuntimeConfigurationDigest = strings.Repeat("b", 64)
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, input); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
		t.Fatal("配置漂移回执通过")
	}
	input.Inspection.RuntimeConfigurationDigest = digest
	input.FactsRevision++
	if err := s.CompleteExecutionNodeEnvironmentCheck(ctx, input); !errors.Is(err, ErrExecutionNodeEnvironmentCheckRequired) {
		t.Fatal("迟到事实版本回执通过")
	}
}
