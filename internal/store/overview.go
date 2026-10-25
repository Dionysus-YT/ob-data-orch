package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ob-data-orch/internal/overview"
)

// ErrOverviewScopeDenied 不区分身份不存在与已停用，也不返回任何对象或计数。
var ErrOverviewScopeDenied = errors.New("overview scope is unavailable")

// OverviewFacts 只在控制面内部传递；节点必须再经现有逐对象授权和安全投影后才能展示。
type OverviewFacts struct {
	// 权限范围受限与查询失败分别投影，避免撤权后把不可见任务显示为零。
	TasksRestricted   bool
	MyTasksRestricted bool
	Metrics           *overview.Metrics
	Trend             *[]overview.Bucket
	MyTasks           *[]TaskListItem
	Failures          *[]TaskListItem
	Nodes             *[]ExecutionNode
	Budgets           map[string]NodeBudgetUsage
}

// ReadOverviewNodeIDs 只读取活动主体所需的节点标识供控制面授权，不读取名称、环境或预算正文。
// 无权限节点的损坏事实不得通过首页分区错误泄露存在性。
func (s *Store) ReadOverviewNodeIDs(ctx context.Context, subjectID string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("SQLite store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT node_id FROM execution_nodes WHERE management_state != 'ARCHIVED'
		AND EXISTS (SELECT 1 FROM auth_subjects WHERE subject_id=? AND account_status='ACTIVE') ORDER BY node_id`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReadOverviewFacts 在同一短只读事务中读取首页事实，不分页截断统计，不解析秘密或访问执行节点。
// 分区查询失败使用 nil 表达不可用；主体核验、事务或取消失败则整份快照失败关闭。
func (s *Store) ReadOverviewFacts(ctx context.Context, query overview.Query) (OverviewFacts, error) {
	if s == nil || s.db == nil {
		return OverviewFacts{}, errors.New("SQLite store is nil")
	}
	verified, err := overview.NewQuery(query.SubjectID, query.GlobalRead, query.Now, query.Range)
	if err != nil {
		return OverviewFacts{}, err
	}
	verified.NodeIDs, verified.NodeReadUnavailable = query.NodeIDs, query.NodeReadUnavailable
	query = verified
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return OverviewFacts{}, fmt.Errorf("begin overview snapshot: %w", err)
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM auth_subjects WHERE subject_id = ? AND account_status = 'ACTIVE'`, query.SubjectID).Scan(&active); errors.Is(err, sql.ErrNoRows) {
		return OverviewFacts{}, ErrOverviewScopeDenied
	} else if err != nil {
		return OverviewFacts{}, fmt.Errorf("read overview identity: %w", err)
	}
	var result OverviewFacts
	var managed bool
	if err := tx.QueryRowContext(ctx, "SELECT "+managedPermissionExists).Scan(&managed); err != nil {
		return OverviewFacts{}, err
	}
	if managed {
		profile, err := permissionProfile(ctx, tx, query.SubjectID)
		if err != nil {
			return OverviewFacts{}, err
		}
		query.GlobalRead = query.GlobalRead && initialGlobalTaskRead(profile)
		result.MyTasksRestricted = !profile.Assignment.CanCreateTask()
		operatorScope := false
		for _, object := range profile.Assignment.Scopes {
			if object.Type == "TASK_OPERATE_BY_DATA_SOURCE" && profile.Assignment.HasScope(object.Type, object.ObjectID) {
				operatorScope = true
				break
			}
		}
		result.TasksRestricted = result.MyTasksRestricted && !operatorScope && !query.GlobalRead
	}
	scope := TaskListQuery{SubjectID: query.SubjectID, GlobalRead: query.GlobalRead, Limit: overview.ListLimit}
	predicate, args, err := buildTaskListPredicate(scope)
	if err != nil {
		return OverviewFacts{}, err
	}
	var metrics overview.Metrics
	metricArgs := []any{utcText(query.TodayFrom), utcText(query.TodayTo), utcText(query.TodayFrom), utcText(query.TodayTo)}
	err = tx.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN `+taskListStateExpression+`='RUNNING' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN `+taskListStateExpression+`='WAITING_SCHEDULE' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN e.state='SUCCEEDED' AND julianday(e.finished_at)>=julianday(?) AND julianday(e.finished_at)<julianday(?) THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN e.state='FAILED' AND julianday(e.finished_at)>=julianday(?) AND julianday(e.finished_at)<julianday(?) THEN 1 ELSE 0 END),0)
		`+taskListFrom+` WHERE `+predicate, append(metricArgs, args...)...).Scan(&metrics.Running, &metrics.Waiting, &metrics.TodaySucceeded, &metrics.TodayFailed)
	if err == nil {
		result.Metrics = &metrics
	}
	if buckets, readErr := readOverviewTrend(ctx, tx, query, predicate, args); readErr == nil {
		result.Trend = &buckets
	}
	mine := scope
	mine.OwnedOnly, mine.GlobalRead, mine.overviewRecentFrom = true, false, query.Now.Add(-24*time.Hour)
	if tasks, readErr := listTaskSummaries(ctx, tx, mine, true); readErr == nil {
		if len(tasks) > overview.ListLimit {
			tasks = tasks[:overview.ListLimit]
		}
		result.MyTasks = &tasks
	}
	failed := scope
	failed.States, failed.FinishedFrom, failed.FinishedTo = []string{"FAILED"}, query.Now.Add(-24*time.Hour), query.Now
	if tasks, readErr := listTaskSummaries(ctx, tx, failed, true); readErr == nil {
		if len(tasks) > overview.ListLimit {
			tasks = tasks[:overview.ListLimit]
		}
		result.Failures = &tasks
	}
	if !query.NodeReadUnavailable {
		if len(query.NodeIDs) == 0 {
			nodes := []ExecutionNode{}
			result.Nodes, result.Budgets = &nodes, map[string]NodeBudgetUsage{}
		} else if nodes, readErr := queryExecutionNodes(ctx, tx, &query.NodeIDs); readErr == nil {
			if budgets, budgetErr := queryNodeBudgetUsages(ctx, tx, &query.NodeIDs); budgetErr == nil {
				result.Nodes, result.Budgets = &nodes, budgets
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return OverviewFacts{}, err
	}
	if result.TasksRestricted {
		result.Metrics, result.Trend, result.Failures = nil, nil, nil
	}
	if result.MyTasksRestricted {
		result.MyTasks = nil
	}
	if err := tx.Commit(); err != nil {
		return OverviewFacts{}, fmt.Errorf("commit overview snapshot: %w", err)
	}
	return result, nil
}

// readOverviewTrend 使用精确半开区间，与任务列表 FinishedFrom/To 共用终态时间口径。
func readOverviewTrend(ctx context.Context, tx *sql.Tx, query overview.Query, predicate string, args []any) ([]overview.Bucket, error) {
	buckets := append([]overview.Bucket{}, query.Buckets...)
	format := "%Y-%m-%dT%H:00:00Z"
	if query.Range == "7d" {
		format = "%Y-%m-%dT00:00:00Z"
	}
	values := append([]any{format}, args...)
	values = append(values, utcText(query.From), utcText(query.To))
	// SQLite 只扫描一次授权终态集合，按固定 UTC 桶聚合；不逐桶重新读取整张任务表。
	rows, err := tx.QueryContext(ctx, `SELECT strftime(?,e.finished_at), COUNT(*),
			COALESCE(SUM(CASE WHEN e.state='SUCCEEDED' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN e.state='FAILED' THEN 1 ELSE 0 END),0)
			`+taskListFrom+` WHERE `+predicate+` AND e.state IN ('SUCCEEDED','FAILED','CANCELLED')
			AND julianday(e.finished_at)>=julianday(?) AND julianday(e.finished_at)<julianday(?) GROUP BY 1`, values...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	indices := map[string]int{}
	for index, bucket := range buckets {
		key := bucket.From.Truncate(time.Hour).Format(time.RFC3339)
		if query.Range == "7d" {
			key = bucket.From.Format("2006-01-02T00:00:00Z")
		}
		indices[key] = index
	}
	for rows.Next() {
		var key string
		var total, succeeded, failed int
		if err := rows.Scan(&key, &total, &succeeded, &failed); err != nil {
			return nil, err
		}
		index, exists := indices[key]
		if !exists {
			return nil, errors.New("overview bucket is outside the snapshot")
		}
		buckets[index].Total, buckets[index].Succeeded, buckets[index].Failed = total, succeeded, failed
	}
	return buckets, rows.Err()
}
