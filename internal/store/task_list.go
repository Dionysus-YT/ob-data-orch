package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// taskListMaxFilterValues 限制多选筛选值数量，避免查询退化为任意枚举。
const taskListMaxFilterValues = 8

// taskListFilterOptionLimit 是每个筛选维度返回的候选上限。
const taskListFilterOptionLimit = 100

// taskListStateExpression 与详情投影一致：尚无执行记录的已提交任务视为等待调度。
const taskListStateExpression = `COALESCE(e.state, 'WAITING_SCHEDULE')`

// taskListFrom 是列表、计数与候选共用的连接；展示事实缺失时 LEFT JOIN 得到空名称。
const taskListFrom = `
        FROM tasks t
        LEFT JOIN task_executions e ON e.task_id = t.task_id
        LEFT JOIN task_display_facts f ON f.task_id = t.task_id
        LEFT JOIN auth_subjects creator ON creator.subject_id = t.creator_subject_id`

// buildTaskListPredicate 生成授权范围与筛选条件。授权谓词始终位于最前且不可被筛选覆盖；
// 所有用户输入只作为绑定参数，不拼接进 SQL 文本。
func buildTaskListPredicate(input TaskListQuery) (string, []any, error) {
	if strings.TrimSpace(input.SubjectID) == "" {
		return "", nil, errors.New("task list subject is invalid")
	}
	if len(input.Types) > taskListMaxFilterValues || len(input.States) > taskListMaxFilterValues || len(input.Keyword) > 256 {
		return "", nil, errors.New("task list filter is invalid")
	}
	clauses := []string{
		`EXISTS (SELECT 1 FROM auth_subjects subject WHERE subject.subject_id = ? AND subject.account_status = 'ACTIVE')`,
		`(? = 1 OR t.creator_subject_id = ? OR EXISTS (SELECT 1 FROM subject_object_scopes task_scope
            WHERE task_scope.subject_id = ? AND task_scope.scope_type = 'TASK_OPERATE_BY_DATA_SOURCE' AND task_scope.object_id = t.data_source_id))`,
	}
	args := []any{input.SubjectID, input.GlobalRead, input.SubjectID, input.SubjectID}
	add := func(clause string, values ...any) {
		clauses = append(clauses, clause)
		args = append(args, values...)
	}
	if input.TaskID != "" {
		add(`t.task_id = ?`, input.TaskID)
	}
	if input.ParentTaskID != "" {
		add(`t.parent_task_id = ?`, input.ParentTaskID)
	}
	if input.NodeID != "" {
		add(`t.node_id = ?`, input.NodeID)
	}
	if input.OwnedOnly {
		add(`t.creator_subject_id = ?`, input.SubjectID)
	}
	if input.CreatorID != "" {
		add(`t.creator_subject_id = ?`, input.CreatorID)
	}
	if input.DataSourceID != "" {
		add(`t.data_source_id = ?`, input.DataSourceID)
	}
	if input.Environment != "" {
		add(`f.environment = ?`, input.Environment)
	}
	if len(input.Types) > 0 {
		// 当前任务表只承载 OBDUMPER 导出；其他类型筛选应得到空集而不是被忽略。
		values := make([]any, 0, len(input.Types))
		for _, value := range input.Types {
			values = append(values, value)
		}
		add(`'OBDUMPER_EXPORT' IN (`+placeholders(len(values))+`)`, values...)
	}
	if len(input.States) > 0 {
		values := make([]any, 0, len(input.States))
		for _, value := range input.States {
			values = append(values, value)
		}
		add(taskListStateExpression+` IN (`+placeholders(len(values))+`)`, values...)
	}
	if keyword := strings.TrimSpace(input.Keyword); keyword != "" {
		pattern := "%" + escapeLikePattern(keyword) + "%"
		add(`(t.task_id LIKE ? ESCAPE '\' OR COALESCE(json_extract(t.snapshot_json, '$.database'), '') LIKE ? ESCAPE '\'
            OR COALESCE(json_extract(t.snapshot_json, '$.table'), '') LIKE ? ESCAPE '\')`, pattern, pattern, pattern)
	}
	// 时间筛选使用 julianday 比较，避免 RFC 3339 小数秒省略导致的字典序偏差。
	if !input.CreatedFrom.IsZero() {
		add(`julianday(t.submitted_at) >= julianday(?)`, utcText(input.CreatedFrom))
	}
	if !input.CreatedTo.IsZero() {
		add(`julianday(t.submitted_at) < julianday(?)`, utcText(input.CreatedTo))
	}
	if !input.StartedFrom.IsZero() {
		add(`e.started_at IS NOT NULL AND julianday(e.started_at) >= julianday(?)`, utcText(input.StartedFrom))
	}
	if !input.StartedTo.IsZero() {
		add(`e.started_at IS NOT NULL AND julianday(e.started_at) < julianday(?)`, utcText(input.StartedTo))
	}
	return strings.Join(clauses, "\n          AND "), args, nil
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

// escapeLikePattern 转义 LIKE 通配符，关键字只按字面匹配。
func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// CountFilteredTaskSummaries 与列表使用相同授权及筛选，保证分页数字不会泄露筛选外的任务。
func (s *Store) CountFilteredTaskSummaries(ctx context.Context, input TaskListQuery) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("SQLite store is nil")
	}
	predicate, args, err := buildTaskListPredicate(input)
	if err != nil {
		return 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+taskListFrom+` WHERE `+predicate, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count filtered tasks: %w", err)
	}
	return total, nil
}

// ListTaskSummaries 在 SQLite 查询中裁剪授权范围，并按提交时间和任务 ID 倒序返回。
// 返回值最多为 Limit+1 条，供 HTTP 层生成主体与筛选绑定的下一页游标。
func (s *Store) ListTaskSummaries(ctx context.Context, input TaskListQuery) ([]TaskListItem, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("SQLite store is nil")
	}
	if input.Limit < 1 || input.Limit > 100 || (input.BeforeSubmitted.IsZero() != (strings.TrimSpace(input.BeforeTaskID) == "")) {
		return nil, errors.New("task list query is invalid")
	}
	predicate, args, err := buildTaskListPredicate(input)
	if err != nil {
		return nil, err
	}
	if !input.BeforeSubmitted.IsZero() {
		before := utcText(input.BeforeSubmitted)
		predicate += "\n          AND (t.submitted_at < ? OR (t.submitted_at = ? AND t.task_id < ?))"
		args = append(args, before, before, input.BeforeTaskID)
	}
	// 可写资格独立于读取范围：全局只读管理员不能借列表获得写操作入口。
	selectArgs := []any{input.SubjectID, input.SubjectID}
	rows, err := s.db.QueryContext(ctx, `
        SELECT t.task_id, t.creator_subject_id, t.data_source_id, t.node_id, 'OBDUMPER_EXPORT',
               COALESCE(json_extract(t.snapshot_json, '$.database'), ''),
               COALESCE(json_extract(t.snapshot_json, '$.table'), ''),
               COALESCE(json_extract(t.snapshot_json, '$.scopeKind'), ''),
               t.snapshot_version,
               `+taskListStateExpression+`, COALESCE(e.reconciliation_required, 0),
               t.submitted_at, COALESCE(e.started_at, ''), COALESCE(e.finished_at, ''),
               COALESCE(e.updated_at, t.submitted_at),
               f.data_source_name, f.environment, f.node_name, creator.display_name,
               CASE WHEN e.cancellation_requested_at IS NOT NULL THEN 1 ELSE 0 END,
               CASE WHEN json_extract(e.result_summary_json, '$.checkpointPresent') = 1 THEN 1 ELSE 0 END,
               COALESCE(t.derivation_kind, ''),
               CASE WHEN EXISTS (SELECT 1 FROM data_sources current_ds WHERE current_ds.data_source_id = t.data_source_id) THEN 0 ELSE 1 END,
               CASE WHEN EXISTS (SELECT 1 FROM execution_nodes current_node WHERE current_node.node_id = t.node_id) THEN 0 ELSE 1 END,
               CASE WHEN t.creator_subject_id = ? OR EXISTS (SELECT 1 FROM subject_object_scopes op_scope
                   WHERE op_scope.subject_id = ? AND op_scope.scope_type = 'TASK_OPERATE_BY_DATA_SOURCE' AND op_scope.object_id = t.data_source_id)
                   THEN 1 ELSE 0 END
        `+taskListFrom+`
        WHERE `+predicate+`
        ORDER BY t.submitted_at DESC, t.task_id DESC
        LIMIT ?`, append(append(selectArgs, args...), input.Limit+1)...)
	if err != nil {
		return nil, fmt.Errorf("list authorized tasks: %w", err)
	}
	defer rows.Close()
	items := make([]TaskListItem, 0, input.Limit+1)
	for rows.Next() {
		var item TaskListItem
		var reconciliation, cancellation, checkpoint, sourceDeleted, nodeDeleted, operable int
		var submittedAt, startedAt, finishedAt, updatedAt string
		var dataSourceName, environment, nodeName, creatorName sql.NullString
		if err := rows.Scan(&item.TaskID, &item.CreatorSubjectID, &item.DataSourceID, &item.NodeID,
			&item.TaskType, &item.Database, &item.Table, &item.ScopeKind, &item.SnapshotVersion,
			&item.State, &reconciliation, &submittedAt, &startedAt, &finishedAt, &updatedAt,
			&dataSourceName, &environment, &nodeName, &creatorName,
			&cancellation, &checkpoint, &item.DerivationKind, &sourceDeleted, &nodeDeleted, &operable); err != nil {
			return nil, fmt.Errorf("scan authorized task: %w", err)
		}
		item.ReconciliationRequired = reconciliation == 1
		item.CancellationRequested = cancellation == 1
		item.CheckpointPresent = checkpoint == 1
		item.Operable = operable == 1
		item.DataSourceDeleted, item.NodeDeleted = sourceDeleted == 1, nodeDeleted == 1
		item.DataSourceName, item.Environment, item.NodeName, item.CreatorName = dataSourceName.String, environment.String, nodeName.String, creatorName.String
		if item.SubmittedAt, err = parseTaskListTime(submittedAt, true); err != nil {
			return nil, fmt.Errorf("parse task submitted time: %w", err)
		}
		if item.StartedAt, err = parseTaskListTime(startedAt, false); err != nil {
			return nil, fmt.Errorf("parse task started time: %w", err)
		}
		if item.FinishedAt, err = parseTaskListTime(finishedAt, false); err != nil {
			return nil, fmt.Errorf("parse task finished time: %w", err)
		}
		if item.UpdatedAt, err = parseTaskListTime(updatedAt, true); err != nil {
			return nil, fmt.Errorf("parse task updated time: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorized tasks: %w", err)
	}
	return items, nil
}

// ListTaskFilterOptions 只从当前主体可读任务中提取候选值，候选名称使用提交时冻结的展示事实。
// 名称缺失（对象已删除且无冻结名称）时以标识兜底，不反查当前配置。
func (s *Store) ListTaskFilterOptions(ctx context.Context, input TaskListQuery) (TaskListFilterOptions, error) {
	if s == nil || s.db == nil {
		return TaskListFilterOptions{}, errors.New("SQLite store is nil")
	}
	scope := TaskListQuery{SubjectID: input.SubjectID, GlobalRead: input.GlobalRead, OwnedOnly: input.OwnedOnly}
	predicate, args, err := buildTaskListPredicate(scope)
	if err != nil {
		return TaskListFilterOptions{}, err
	}
	var options TaskListFilterOptions
	read := func(idExpr, labelExpr string) ([]TaskListFilterOption, error) {
		rows, err := s.db.QueryContext(ctx, `SELECT `+idExpr+`, MAX(`+labelExpr+`) `+taskListFrom+`
            WHERE `+predicate+` GROUP BY `+idExpr+` ORDER BY MAX(`+labelExpr+`), `+idExpr+` LIMIT ?`,
			append(append([]any{}, args...), taskListFilterOptionLimit)...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		values := []TaskListFilterOption{}
		for rows.Next() {
			var option TaskListFilterOption
			var label sql.NullString
			if err := rows.Scan(&option.ID, &label); err != nil {
				return nil, err
			}
			option.Label = label.String
			values = append(values, option)
		}
		return values, rows.Err()
	}
	if options.DataSources, err = read("t.data_source_id", "f.data_source_name"); err != nil {
		return TaskListFilterOptions{}, fmt.Errorf("read task data source options: %w", err)
	}
	if options.Nodes, err = read("t.node_id", "f.node_name"); err != nil {
		return TaskListFilterOptions{}, fmt.Errorf("read task node options: %w", err)
	}
	if options.Creators, err = read("t.creator_subject_id", "creator.display_name"); err != nil {
		return TaskListFilterOptions{}, fmt.Errorf("read task creator options: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT f.environment `+taskListFrom+`
        WHERE `+predicate+` AND f.environment IS NOT NULL ORDER BY f.environment`, args...)
	if err != nil {
		return TaskListFilterOptions{}, fmt.Errorf("read task environment options: %w", err)
	}
	defer rows.Close()
	options.Environments = []string{}
	for rows.Next() {
		var environment string
		if err := rows.Scan(&environment); err != nil {
			return TaskListFilterOptions{}, err
		}
		options.Environments = append(options.Environments, environment)
	}
	return options, rows.Err()
}

// insertTaskDisplayFacts 在提交事务内冻结当前数据源与节点的展示名称。
// 只读取名称与环境，不复制连接、路径或凭据；对象不存在时保存空值。
func insertTaskDisplayFacts(ctx context.Context, tx *sql.Tx, taskID, dataSourceID, nodeID, capturedAt string) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO task_display_facts(task_id, data_source_name, environment, node_name, captured_at)
        SELECT ?, (SELECT display_name FROM data_sources WHERE data_source_id = ?),
               (SELECT environment FROM data_sources WHERE data_source_id = ?),
               (SELECT display_name FROM execution_nodes WHERE node_id = ?), ?`,
		taskID, dataSourceID, dataSourceID, nodeID, capturedAt)
	if err != nil {
		return fmt.Errorf("freeze task display facts: %w", err)
	}
	return nil
}
