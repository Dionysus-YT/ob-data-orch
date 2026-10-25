package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ob-data-orch/internal/overview"
)

func seedOverviewTask(t *testing.T, database *Store, id, subject, state string, at time.Time) {
	t.Helper()
	input := validTaskSubmission(id)
	input.CreatorSubjectID, input.AuditActorID, input.RequestID = subject, subject, "request-"+id
	input.SnapshotJSON = `{"database":"synthetic","table":"` + id + `"}`
	if err := database.SubmitTask(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if state == "WAITING_SCHEDULE" {
		return
	}
	var finished any
	if state == "SUCCEEDED" || state == "FAILED" || state == "CANCELLED" {
		finished = utcText(at)
	}
	_, err := database.db.Exec(`INSERT INTO task_executions(execution_id,task_id,node_id,agent_id,state,revision,reconciliation_required,created_at,started_at,finished_at,updated_at) VALUES(?,?, 'node-1','agent-1',?,1,0,?,?,?,?)`, "execution-"+id, id, state, utcText(at.Add(-time.Minute)), utcText(at.Add(-time.Minute)), finished, utcText(at))
	if err != nil {
		t.Fatal(err)
	}
}

func Test首页聚合与终态下钻共用授权和半开时间(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	now := testTime.Add(30 * time.Hour)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, subject := range []string{"subject-other", "subject-delegated", "subject-empty"} {
		if err := database.EnsureAuthSubject(ctx, AuthSubject{SubjectID: subject, ExternalSubject: subject, DisplayName: "合成主体", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
			t.Fatal(err)
		}
	}
	for _, state := range []string{"WAITING_SCHEDULE", "STARTING", "RUNNING", "CANCELLING", "SUCCEEDED", "FAILED", "CANCELLED"} {
		seedOverviewTask(t, database, state, "subject-1", state, day.Add(time.Minute))
	}
	seedOverviewTask(t, database, "before-today", "subject-1", "FAILED", day.Add(-time.Millisecond))
	seedOverviewTask(t, database, "other-failure", "subject-other", "FAILED", day.Add(time.Hour))
	seedOverviewTask(t, database, "at-window-end", "subject-1", "CANCELLED", now)
	query, _ := overview.NewQuery("subject-1", false, now, "24h")
	facts, err := database.ReadOverviewFacts(ctx, query)
	if err != nil || facts.Metrics == nil || facts.Trend == nil || facts.MyTasks == nil || facts.Failures == nil || facts.Nodes == nil {
		t.Fatalf("快照不完整：%#v %v", facts, err)
	}
	if *facts.Metrics != (overview.Metrics{Running: 1, Waiting: 1, TodaySucceeded: 1, TodayFailed: 1}) {
		t.Fatalf("精确状态或自然日口径错误：%#v", facts.Metrics)
	}
	total, success, failed := 0, 0, 0
	for _, bucket := range *facts.Trend {
		total += bucket.Total
		success += bucket.Succeeded
		failed += bucket.Failed
	}
	if total != 4 || success != 1 || failed != 2 {
		t.Fatalf("终态总数应包含取消且排除终点：%d/%d/%d", total, success, failed)
	}
	if len(*facts.MyTasks) != 5 || (*facts.MyTasks)[0].State != "FAILED" {
		t.Fatal("本人重点列表没有分组和限量")
	}
	for _, task := range *facts.MyTasks {
		if task.CreatorSubjectID != "subject-1" {
			t.Fatal("本人列表包含他人任务")
		}
	}
	filtered := TaskListQuery{SubjectID: "subject-1", Limit: 10, FinishedFrom: query.From, FinishedTo: query.To}
	items, err := database.ListTaskSummaries(ctx, filtered)
	if err != nil || len(items) != total {
		t.Fatalf("图表与下钻范围不一致：%d %v", len(items), err)
	}
	count, err := database.CountFilteredTaskSummaries(ctx, filtered)
	if err != nil || count != total {
		t.Fatal("列表与计数不一致")
	}
	q, _ := overview.NewQuery("subject-empty", false, now, "7d")
	empty, err := database.ReadOverviewFacts(ctx, q)
	if err != nil || empty.Metrics == nil || *empty.Metrics != (overview.Metrics{}) || len(*empty.MyTasks) != 0 {
		t.Fatal("空范围泄露他人统计")
	}
	q, _ = overview.NewQuery("subject-delegated", false, now, "24h")
	if _, err := database.db.Exec(`INSERT INTO subject_object_scopes(subject_id,scope_type,object_id,granted_by,granted_at) VALUES(?,'TASK_OPERATE_BY_DATA_SOURCE','source-1','subject-1',?)`, q.SubjectID, utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	delegated, err := database.ReadOverviewFacts(ctx, q)
	if err != nil || delegated.Metrics.TodayFailed != 2 || len(*delegated.MyTasks) != 0 {
		t.Fatal("对象授权范围或本人范围混淆")
	}
	if _, err := database.db.Exec(`DELETE FROM subject_object_scopes WHERE subject_id=?`, q.SubjectID); err != nil {
		t.Fatal(err)
	}
	revoked, err := database.ReadOverviewFacts(ctx, q)
	if err != nil || revoked.Metrics.TodayFailed != 0 {
		t.Fatal("授权撤销后仍返回旧统计")
	}
}

func Test首页身份失效和分区损坏失败关闭(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	query, _ := overview.NewQuery("missing", false, testTime, "24h")
	if _, err := database.ReadOverviewFacts(context.Background(), query); !errors.Is(err, ErrOverviewScopeDenied) {
		t.Fatal("未知主体未拒绝")
	}
	query.SubjectID = "subject-1"
	// 只损坏隔离数据库的展示事实依赖，证明不会把读取失败转换成零值。
	if _, err := database.db.Exec(`ALTER TABLE task_display_facts RENAME TO synthetic_missing_display_facts`); err != nil {
		t.Fatal(err)
	}
	facts, err := database.ReadOverviewFacts(context.Background(), query)
	if err != nil || facts.Metrics != nil || facts.MyTasks != nil || facts.Nodes == nil {
		t.Fatalf("分区失败没有隔离：%#v %v", facts, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := database.ReadOverviewFacts(ctx, query); err == nil {
		t.Fatal("取消请求没有终止读取")
	}
	if _, err := database.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='subject-1'`); err != nil {
		t.Fatal(fmt.Errorf("停用合成主体：%w", err))
	}
	if _, err := database.ReadOverviewFacts(context.Background(), query); !errors.Is(err, ErrOverviewScopeDenied) {
		t.Fatal("停用主体未拒绝")
	}
}

func Test首页不读取范围外节点事实且坏数据不泄露存在性(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	ids, err := database.ReadOverviewNodeIDs(ctx, "subject-1")
	if err != nil || len(ids) != 1 || ids[0] != "node-1" {
		t.Fatal("授权前标识查询不正确")
	}
	query, _ := overview.NewQuery("subject-1", false, testTime, "24h")
	if _, err := database.db.Exec(`UPDATE agents SET facts_json='{}' WHERE agent_id='agent-1'`); err != nil {
		t.Fatal(err)
	}
	hidden, err := database.ReadOverviewFacts(ctx, query)
	if err != nil || hidden.Metrics == nil || hidden.Nodes == nil || len(*hidden.Nodes) != 0 {
		t.Fatal("隐藏节点的损坏事实影响了受限分区")
	}
	query.NodeIDs = []string{"node-1"}
	visible, err := database.ReadOverviewFacts(ctx, query)
	if err != nil || visible.Metrics == nil || visible.Nodes != nil {
		t.Fatal("有权坏节点没有作为独立不可用分区")
	}
	query.NodeReadUnavailable = true
	if facts, err := database.ReadOverviewFacts(ctx, query); err != nil || facts.Nodes != nil || facts.Metrics == nil {
		t.Fatal("节点授权元数据失败没有隔离")
	}
}
