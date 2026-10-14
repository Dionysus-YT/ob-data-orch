package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/executionbudget"
	"ob-data-orch/internal/precheckcontract"
)

func Test新提交拒绝旧清单及未知资源事实但保留历史读取(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		s, _ := openTestStore(t)
		seedBaseFixture(t, s)
		if legacy {
			checks := []PrecheckCheckResult{}
			for _, name := range precheckcontract.LegacyChecks(false) {
				checks = append(checks, PrecheckCheckResult{Check: name, Status: "PASSED", EvidenceCode: "SYNTHETIC_OK"})
			}
			payload, _ := json.Marshal(map[string]any{"checks": checks})
			if _, err := s.db.Exec(`UPDATE precheck_runs SET result_json=? WHERE precheck_id='precheck-1'`, string(payload)); err != nil {
				t.Fatal(err)
			}
			run, err := s.GetPrecheckRun(context.Background(), "precheck-1")
			if err != nil || len(run.Results) != 6 {
				t.Fatal("历史报告不可读取", err)
			}
		} else if _, err := s.db.Exec(`UPDATE agents SET facts_json=NULL WHERE agent_id='agent-1'`); err != nil {
			t.Fatal(err)
		}
		if err := s.SubmitTask(context.Background(), validTaskSubmission("task-proof-rejected")); !errors.Is(err, ErrPrecheckInvalid) {
			t.Fatal("不完整证明被用于新提交", err)
		}
		assertCount(t, s.db, "SELECT COUNT(*) FROM tasks", 0)
	}
}

func Test资源回执不得伪造控制面资格(t *testing.T) {
	for _, requirement := range []*executionbudget.PreflightRequirement{nil, {}, {HeapBytes: 4 << 30, RequiredBytes: 5 << 30, TotalBudgetBytes: 3 << 30}} {
		if matchesPrecheckResource(requirement, passedPrecheckResults()) {
			t.Fatal("伪造通过绕过资源政策")
		}
	}
	requirement := executionbudget.Preflight("", 16<<30, executionbudget.Policy{})
	if !matchesPrecheckResource(&requirement, passedPrecheckResults()) {
		t.Fatal("符合可信事实的资源报告被拒绝")
	}
	unknown := passedPrecheckResults()
	unknown[3] = PrecheckCheckResult{Check: "NODE_RESOURCE", Status: "UNKNOWN", EvidenceCode: "NODE_RESOURCE_UNAVAILABLE"}
	if !matchesPrecheckResource(&requirement, unknown) {
		t.Fatal("已知额度不能阻止探针异常报告未知并结束失败检查")
	}
}

func Test指定列分区必须取得对应固定证据且不能伪造资源通过(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	config := `{"database":"synthetic_db","scopeKind":"SPECIFIED","table":"synthetic_table","contentKind":"DATA_ONLY","format":"CSV","filePath":"/E:/tmp/output","config":{"objectScope":{"objectTypes":["TABLE"],"expressions":[{"name":"synthetic_table"}]},"outputConfig":{"outputKind":"LOCAL"},"filterConfig":{"includeColumnNames":["id"],"partition":"sp0"}}}`
	if _, err := s.db.Exec(`UPDATE export_drafts SET config_version='v6',config_json=? WHERE draft_id='draft-1'`, config); err != nil {
		t.Fatal(err)
	}
	run := createClaimablePrecheck(t, s, "precheck-selection")
	now := testTime.Add(time.Minute)
	grant, err := s.ClaimPrecheck(context.Background(), PrecheckClaim{AgentID: "agent-1", NodeID: "node-1", PrecheckID: run.PrecheckID, LeaseID: "lease-selection", RequestID: "claim-selection", RequestDigest: strings.Repeat("b", 64), LeaseTTL: 2 * time.Minute, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(grant.ExecutionContext.IncludeColumnNames) != 1 || len(grant.ExecutionContext.Partitions) != 1 {
		t.Fatal("冻结选择未传入上下文")
	}
	_, err = s.AcknowledgePrecheck(context.Background(), PrecheckAcknowledgement{AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "ack-selection", RequestDigest: strings.Repeat("c", 64), Now: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	completion := AgentPrecheckCompletion{AgentID: "agent-1", PrecheckID: run.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "complete-selection", RequestDigest: strings.Repeat("d", 64), Results: passedPrecheckResults(), Now: now.Add(2 * time.Second)}
	if _, err := s.CompleteAgentPrecheck(context.Background(), completion); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatal("普通对象通过替代了选择证据", err)
	}
	completion.Results[1].EvidenceCode = "OBJECT_SELECTION_VERIFIED"
	if _, err := s.db.Exec(`UPDATE agents SET facts_json=NULL WHERE agent_id='agent-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteAgentPrecheck(context.Background(), completion); !errors.Is(err, ErrPrecheckLeaseRejected) {
		t.Fatal("未知节点资源被上报为通过", err)
	}
	completion.Results[3] = PrecheckCheckResult{Check: "NODE_RESOURCE", Status: "UNKNOWN", EvidenceCode: "NODE_RESOURCE_UNAVAILABLE"}
	completed, err := s.CompleteAgentPrecheck(context.Background(), completion)
	if err != nil || completed.Status != "FAILED" {
		t.Fatal("未知资源未完整记录并阻断", err)
	}
}
