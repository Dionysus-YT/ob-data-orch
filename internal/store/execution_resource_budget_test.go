package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/executionbudget"
)

func seedBudgetNode(t *testing.T, s *Store, memory uint64) {
	t.Helper()
	seedBaseFixture(t, s)
	node, err := s.GetExecutionNode(context.Background(), "node-1")
	if err != nil {
		t.Fatal(err)
	}
	factsJSON, err := encodeAgentEnvironmentFacts(AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: testTime, MemoryTotalBytes: memory, MemoryAvailableBytes: memory, RuntimeConfigurationDigest: ExecutionNodeRuntimeConfigurationDigest(node)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE agents SET facts_json=?, execution_worker_limit=64 WHERE agent_id='agent-1'`, factsJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE execution_nodes SET environment_check_status='PASSED', environment_check_code='TOOL_RUNTIME_READY', environment_check_facts_revision=1, environment_check_completed_at=? WHERE node_id='node-1'`, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
}

func budgetClaim(id string) ExecutionClaimNext {
	return ExecutionClaimNext{AgentID: "agent-1", NodeID: "node-1", ExecutionID: "execution-" + id, LeaseID: "lease-" + id, ScheduledEventID: "scheduled-" + id, RequestID: "request-" + id, RequestDigest: strings.Repeat("a", 64), LeaseTTL: time.Minute, HeartbeatFreshAfter: testTime.Add(-time.Minute), Now: testTime.Add(3 * time.Minute)}
}

func submitBudgetTask(t *testing.T, s *Store, id, heap string) {
	t.Helper()
	task := validTaskSubmission(id)
	if heap != "" {
		task.PlannedArgvJSON = strings.TrimSuffix(task.PlannedArgvJSON, "]") + `,"--mem","` + heap + `"]`
	}
	if err := s.SubmitTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
}

func Test预算原子领取与同请求重放不重复分配(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 16*executionbudget.GiB)
	for i := 0; i < 3; i++ {
		submitBudgetTask(t, s, fmt.Sprintf("task-%d", i), "8G")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimed := 0
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, found, err := s.ClaimNextExecution(context.Background(), budgetClaim(fmt.Sprint(id)))
			if err != nil {
				t.Error(err)
				return
			}
			if found {
				mu.Lock()
				claimed++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if claimed != 1 {
		t.Fatalf("超额并行领取: %d", claimed)
	}
	var requestID string
	if err := s.db.QueryRow(`SELECT request_id FROM execution_claim_receipts`).Scan(&requestID); err != nil {
		t.Fatal(err)
	}
	replay := budgetClaim(strings.TrimPrefix(requestID, "request-"))
	replay.ExecutionID = "execution-retry"
	if _, found, err := s.ClaimNextExecution(context.Background(), replay); err != nil || !found {
		t.Fatalf("重放失败: %v", err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM execution_resource_allocations`).Scan(&count); err != nil || count != 1 {
		t.Fatal("重放创建第二份分配")
	}
	replay.RequestDigest = strings.Repeat("b", 64)
	if _, _, err := s.ClaimNextExecution(context.Background(), replay); err != ErrIdempotencyConflict {
		t.Fatal("不同请求摘要未拒绝")
	}
	for _, statement := range []string{`UPDATE execution_resource_allocations SET budget_bytes=1`, `DELETE FROM execution_resource_allocations`, `UPDATE execution_claim_receipts SET request_digest='changed'`, `DELETE FROM execution_claim_receipts`} {
		if _, err := s.db.Exec(statement); err == nil {
			t.Fatal("冻结分配或回执可以被改写")
		}
	}
}

func Test过期租约保留预算而已确认退出释放(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 16*executionbudget.GiB)
	for i := 0; i < 3; i++ {
		submitBudgetTask(t, s, fmt.Sprintf("task-%d", i), "")
	}
	for i := 0; i < 2; i++ {
		if _, found, err := s.ClaimNextExecution(context.Background(), budgetClaim(fmt.Sprint(i))); err != nil || !found {
			t.Fatal("预算内领取失败", err)
		}
	}
	usage, err := s.ListNodeBudgetUsages(context.Background())
	if err != nil || usage["node-1"].ReservedBytes != 10*executionbudget.GiB || usage["node-1"].UsedBytes != 0 {
		t.Fatal("待启动预算错误", usage, err)
	}
	// 使用仓储接受路径验证运行段迁移和退出释放，不依赖前端伪造状态。
	appendEvent := func(seq int64, typ, payload string) {
		t.Helper()
		if _, err := s.AppendAuthenticatedExecutionEvent(context.Background(), "agent-1", ExecutionEvent{EventID: fmt.Sprintf("event-%d", seq), ExecutionID: "execution-0", LeaseID: "lease-0", LeaseEpoch: 1, EventSeq: seq, EventType: typ, PayloadJSON: payload, ReceivedAt: testTime.Add(3 * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent(2, "LEASE_ACKNOWLEDGED", "{}")
	appendEvent(3, "PROCESS_STARTED", `{"pid":42,"startedAt":"2026-01-02T03:07:05Z","executableDigest":"`+strings.Repeat("a", 64)+`","bootId":"boot-1"}`)
	usage, err = s.ListNodeBudgetUsages(context.Background())
	if err != nil || usage["node-1"].UsedBytes != 5*executionbudget.GiB || usage["node-1"].ReservedBytes != 5*executionbudget.GiB {
		t.Fatal("运行预算重复或缺失", usage, err)
	}
	appendEvent(4, "PROCESS_EXITED", `{"exitCode":0}`)
	usage, err = s.ListNodeBudgetUsages(context.Background())
	if err != nil || usage["node-1"].UsedBytes != 0 || usage["node-1"].ReservedBytes != 5*executionbudget.GiB {
		t.Fatal("已退出进程未释放预算", usage, err)
	}
	if err := s.withWrite(context.Background(), func(tx *sql.Tx) error {
		return expireExecutionLeasesTx(context.Background(), tx, testTime.Add(5*time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
	usage, err = s.ListNodeBudgetUsages(context.Background())
	if err != nil || usage["node-1"].ReservedBytes != 5*executionbudget.GiB {
		t.Fatal("租约过期错误释放预算", usage, err)
	}
	if err := s.withWrite(context.Background(), func(tx *sql.Tx) error { return ensurePrecheckAgentIdle(context.Background(), tx, "agent-1") }); err != ErrPrecheckLeaseRejected {
		t.Fatal("待核对期间领取了预检查", err)
	}
}

func Test未知物理内存不得从槽位推算额度(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 16*executionbudget.GiB)
	submitBudgetTask(t, s, "task-unknown", "")
	// 已提交任务也必须复核最新物理事实；不能凭此前成功的静态检查领取执行。
	if _, err := s.db.Exec(`UPDATE agents SET facts_json=NULL WHERE agent_id='agent-1'`); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.ClaimNextExecution(context.Background(), budgetClaim("unknown")); err != nil || found {
		t.Fatal("未知内存仍分配任务", err)
	}
}

func Test节点任务聚合完整且不泄露其他账号(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 64*executionbudget.GiB)
	for i := 0; i < 24; i++ {
		submitBudgetTask(t, s, fmt.Sprintf("task-%d", i), "")
	}
	if _, err := s.db.Exec(`INSERT INTO auth_subjects VALUES ('subject-other','external-other','Synthetic Other','ACTIVE',NULL,?,?)`, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	other := validTaskSubmission("task-24")
	other.CreatorSubjectID, other.AuditActorID = "subject-other", "subject-other"
	if err := s.SubmitTask(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.ClaimNextExecution(context.Background(), budgetClaim("stats")); err != nil || !found {
		t.Fatal(err)
	}
	summaries, err := s.ListOwnedNodeTaskSummaries(context.Background(), "subject-1")
	if err != nil || summaries["node-1"].Running != 1 || summaries["node-1"].Waiting != 23 {
		t.Fatal("聚合不完整或泄露", summaries, err)
	}
	query := TaskListQuery{SubjectID: "subject-1", NodeID: "node-1", OwnedOnly: true, Limit: 10}
	count, err := s.CountFilteredTaskSummaries(context.Background(), query)
	if err != nil || count != 24 {
		t.Fatal("筛选计数错误", count, err)
	}
	items, err := s.ListTaskSummaries(context.Background(), query)
	if err != nil || len(items) != 11 {
		t.Fatal("筛选破坏分页", len(items), err)
	}
	query.NodeID = "unknown-node"
	if count, err := s.CountFilteredTaskSummaries(context.Background(), query); err != nil || count != 0 {
		t.Fatal("未知节点泄露任务", count, err)
	}
	query.NodeID, query.OwnedOnly, query.GlobalRead = "node-1", false, true
	if count, err := s.CountFilteredTaskSummaries(context.Background(), query); err != nil || count != 25 {
		t.Fatal("全局聚合没有包含其他账号", count, err)
	}
	all, err := s.ListNodeTaskSummaries(context.Background(), "subject-1", true)
	if err != nil || all["node-1"].Waiting != 24 {
		t.Fatal("全局节点聚合不完整", all, err)
	}
	query.Limit = 50
	if items, err := s.ListTaskSummaries(context.Background(), query); err != nil || len(items) != 25 {
		t.Fatal("全局节点列表与计数不一致", len(items), err)
	}
	if _, err := s.GetAuthorizedTaskSummary(context.Background(), "task-24", "subject-1"); err != ErrDataSourceNotFound {
		t.Fatal("普通范围读到了其他账号任务", err)
	}
	if detail, err := s.GetGlobalTaskSummary(context.Background(), "task-24", "subject-1"); err != nil || detail.CreatorSubjectID != "subject-other" {
		t.Fatal("全局任务详情未接入", detail, err)
	}
	if _, err := s.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-1'`); err != nil {
		t.Fatal(err)
	}
	summaries, err = s.ListOwnedNodeTaskSummaries(context.Background(), "subject-1")
	if err != nil || len(summaries) != 0 {
		t.Fatal("身份撤销后仍泄露计数", summaries, err)
	}
	if _, err := s.GetGlobalTaskSummary(context.Background(), "task-24", "subject-1"); err != ErrDataSourceNotFound {
		t.Fatal("已停用全局主体读取任务", err)
	}
}

func Test动态占用与物理采样不使既有检查失效(t *testing.T) {
	s, _ := openTestStore(t)
	seedBudgetNode(t, s, 16*executionbudget.GiB)
	node, err := s.GetExecutionNode(context.Background(), "node-1")
	if err != nil {
		t.Fatal(err)
	}
	facts := node.Agent.EnvironmentFacts
	facts.ObservedAt = testTime.Add(time.Minute)
	facts.MemoryAvailableBytes = 8 * executionbudget.GiB
	revision, err := s.RecordAgentHeartbeat(context.Background(), AgentHeartbeat{AgentID: "agent-1", NodeID: "node-1", ProtocolVersion: "agent-v1", BootID: "boot-1", RequestID: "dynamic-heartbeat", ObservedAt: facts.ObservedAt, ReceivedAt: facts.ObservedAt, CapacityTotal: 32, CapacityUsed: 2, Facts: facts})
	if err != nil || revision != 1 {
		t.Fatal("动态占用使预检查绑定失效", revision, err)
	}
	node, err = s.GetExecutionNode(context.Background(), "node-1")
	if err != nil || node.Agent.CapacityUsed != 2 || node.Agent.CapacityTotal != 32 || node.EnvironmentCheck.FactsRevision != 1 {
		t.Fatal("真实并行事实未保存", node, err)
	}
}

func Test取消释放须符合启动与进程树证据(t *testing.T) {
	for _, kind := range []string{"not-started", "confirmed-tree", "uncertain-tree", "contradictory-no-process"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := openTestStore(t)
			seedBudgetNode(t, s, 16*executionbudget.GiB)
			submitBudgetTask(t, s, "task-0", "")
			if _, found, err := s.ClaimNextExecution(context.Background(), budgetClaim("0")); err != nil || !found {
				t.Fatal("领取失败", err)
			}
			appendEvent := func(seq int64, typ, payload string) error {
				_, err := s.AppendAuthenticatedExecutionEvent(context.Background(), "agent-1", ExecutionEvent{EventID: fmt.Sprintf("cancel-event-%d", seq), ExecutionID: "execution-0", LeaseID: "lease-0", LeaseEpoch: 1, EventSeq: seq, EventType: typ, PayloadJSON: payload, ReceivedAt: testTime.Add(3 * time.Minute)})
				return err
			}
			if err := appendEvent(2, "LEASE_ACKNOWLEDGED", "{}"); err != nil {
				t.Fatal(err)
			}
			if kind == "not-started" {
				if err := appendEvent(3, "PROCESS_CANCELLED", `{"noProcess":true,"terminationCode":"NOT_STARTED","treeObserved":true}`); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := appendEvent(3, "PROCESS_STARTED", `{"pid":123,"startedAt":"2026-07-22T00:00:00Z","executableDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bootId":"boot-1"}`); err != nil {
					t.Fatal(err)
				}
				if kind == "contradictory-no-process" {
					if err := appendEvent(4, "PROCESS_CANCELLED", `{"noProcess":true,"terminationCode":"NOT_STARTED","treeObserved":true}`); err != ErrEventRejected {
						t.Fatal("已启动仍接受无进程取消", err)
					}
				} else {
					payload := `{"noProcess":false,"terminationCode":"PROCESS_TREE_TERMINATED","treeObserved":true}`
					if kind == "uncertain-tree" {
						payload = `{"noProcess":false,"terminationCode":"CANCEL_FAILED","treeObserved":false}`
					}
					if err := appendEvent(4, "PROCESS_CANCELLED", payload); err != nil {
						t.Fatal(err)
					}
					err := appendEvent(5, "PROCESS_EXITED", `{"exitCode":1}`)
					if kind == "uncertain-tree" {
						if err != ErrEventRejected {
							t.Fatal("待核对终态继续接受主进程退出", err)
						}
					} else if err != nil {
						t.Fatal(err)
					}
				}
			}
			usages, err := s.ListNodeBudgetUsages(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			usage := usages["node-1"]
			switch kind {
			case "not-started", "confirmed-tree":
				if usage.UsedBytes+usage.ReservedBytes != 0 {
					t.Fatal("可信退出未释放", usage)
				}
			case "uncertain-tree":
				if usage.ReservedBytes != 5*executionbudget.GiB || usage.UsedBytes != 0 {
					t.Fatal("进程树待核对被释放", usage)
				}
			case "contradictory-no-process":
				if usage.UsedBytes != 5*executionbudget.GiB {
					t.Fatal("矛盾事实改变占用", usage)
				}
			}
		})
	}
}
