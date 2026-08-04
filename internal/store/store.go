package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/migrate"
	"ob-data-orch/internal/outputpath"
	"ob-data-orch/internal/precheckcontract"

	_ "modernc.org/sqlite"
)

const sqlitePragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var windowsNativeAbsolutePathPattern = regexp.MustCompile(`^[A-Za-z]:\\`)
var fixedPrecheckChecks = precheckcontract.FixedChecks()

// f3LogIndexPlaceholderRetention 仅满足现有段索引的非空保留时间列。
// F3 尚无系统设置和物理清理实现，不能把这个占位值当成已确认的产品保留策略或据此删除日志。
const f3LogIndexPlaceholderRetention = 30 * 24 * time.Hour

// Store owns only database access. It has no network, credential resolution,
// filesystem logging, agent long polling, or command execution behavior.
type Store struct {
	db      *sql.DB
	writeMu sync.Mutex
}

func Open(ctx context.Context, databasePath string) (*Store, error) {
	if strings.TrimSpace(databasePath) == "" {
		return nil, errors.New("database path is required")
	}
	absPath, err := filepath.Abs(databasePath)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	parent := filepath.Dir(absPath)
	info, err := os.Stat(parent)
	if err != nil {
		return nil, fmt.Errorf("database directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("database parent is not a directory")
	}

	db, err := sql.Open("sqlite", sqliteDSN(absPath, ""))
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	store := &Store{db: db}
	if err := store.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ListDataSourceSummaries 仅返回可安全暴露给 API 的数据源投影。
// 授权仍由控制面负责；该查询保证数据库中的加密凭据材料不会进入结果。
func (s *Store) ListDataSourceSummaries(ctx context.Context) ([]DataSourceSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("SQLite store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT data_source_id, display_name, environment, connection_kind,
               compatibility_mode, host, port, cluster_name, tenant_name, username,
               COALESCE(default_database, ''), state, revision,
               current_credential_revision, COALESCE(last_test_status, ''),
		       last_tested_at, COALESCE(last_test_safe_summary_json, ''), COALESCE(last_test_source, ''), updated_at
        FROM data_sources
        WHERE state != 'ARCHIVED'
        ORDER BY normalized_name ASC
    `)
	if err != nil {
		return nil, fmt.Errorf("list data source summaries: %w", err)
	}
	defer rows.Close()
	var summaries []DataSourceSummary
	for rows.Next() {
		var summary DataSourceSummary
		var lastTestedAt sql.NullString
		var updatedAt string
		if err := rows.Scan(
			&summary.DataSourceID, &summary.DisplayName, &summary.Environment, &summary.ConnectionKind,
			&summary.CompatibilityMode, &summary.Host, &summary.Port, &summary.ClusterName, &summary.TenantName, &summary.Username,
			&summary.DefaultDatabase, &summary.State, &summary.Revision, &summary.CredentialRevision,
			&summary.LastTestStatus, &lastTestedAt, &summary.LastTestSafeSummaryJSON, &summary.LastTestSource, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan data source summary: %w", err)
		}
		parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse data source update time: %w", err)
		}
		summary.UpdatedAt = parsedUpdatedAt.UTC()
		if lastTestedAt.Valid {
			parsedLastTestedAt, err := time.Parse(time.RFC3339Nano, lastTestedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse data source test time: %w", err)
			}
			summary.LastTestedAt = &parsedLastTestedAt
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate data source summaries: %w", err)
	}
	return summaries, nil
}

// GetDataSourceSummary 为一个活动数据源返回与列表 API 相同的非敏感投影。
// 已归档数据源故意表现为不存在。
func (s *Store) GetDataSourceSummary(ctx context.Context, dataSourceID string) (DataSourceSummary, error) {
	if s == nil || s.db == nil {
		return DataSourceSummary{}, errors.New("SQLite store is nil")
	}
	if strings.TrimSpace(dataSourceID) == "" {
		return DataSourceSummary{}, ErrDataSourceNotFound
	}
	var summary DataSourceSummary
	var lastTestedAt sql.NullString
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
        SELECT data_source_id, display_name, environment, connection_kind,
               compatibility_mode, host, port, cluster_name, tenant_name, username,
               COALESCE(default_database, ''), state, revision,
               current_credential_revision, COALESCE(last_test_status, ''),
		       last_tested_at, COALESCE(last_test_safe_summary_json, ''), COALESCE(last_test_source, ''), updated_at
        FROM data_sources
        WHERE data_source_id = ? AND state != 'ARCHIVED'
    `, dataSourceID).Scan(
		&summary.DataSourceID, &summary.DisplayName, &summary.Environment, &summary.ConnectionKind,
		&summary.CompatibilityMode, &summary.Host, &summary.Port, &summary.ClusterName, &summary.TenantName, &summary.Username,
		&summary.DefaultDatabase, &summary.State, &summary.Revision, &summary.CredentialRevision,
		&summary.LastTestStatus, &lastTestedAt, &summary.LastTestSafeSummaryJSON, &summary.LastTestSource, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DataSourceSummary{}, ErrDataSourceNotFound
	}
	if err != nil {
		return DataSourceSummary{}, fmt.Errorf("get data source summary: %w", err)
	}
	parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return DataSourceSummary{}, fmt.Errorf("parse data source update time: %w", err)
	}
	summary.UpdatedAt = parsedUpdatedAt.UTC()
	if lastTestedAt.Valid {
		parsedLastTestedAt, err := time.Parse(time.RFC3339Nano, lastTestedAt.String)
		if err != nil {
			return DataSourceSummary{}, fmt.Errorf("parse data source test time: %w", err)
		}
		summary.LastTestedAt = &parsedLastTestedAt
	}
	return summary, nil
}

// ListExecutionNodeSummaries 仅返回当前启用节点的最小安全投影。
// 节点对象授权由控制面逐项执行，仓储层不拥有浏览器身份或权限范围。
func (s *Store) ListExecutionNodeSummaries(ctx context.Context) ([]ExecutionNodeSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("SQLite store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT node_id, display_name, platform
        FROM execution_nodes
        WHERE management_state = 'ENABLED'
        ORDER BY normalized_name ASC
    `)
	if err != nil {
		return nil, fmt.Errorf("list execution node summaries: %w", err)
	}
	defer rows.Close()
	var summaries []ExecutionNodeSummary
	for rows.Next() {
		var summary ExecutionNodeSummary
		if err := rows.Scan(&summary.NodeID, &summary.DisplayName, &summary.Platform); err != nil {
			return nil, fmt.Errorf("scan execution node summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate execution node summaries: %w", err)
	}
	return summaries, nil
}

// ListExecutionNodes 返回节点管理员可见的配置投影。
// Agent、工具、心跳、环境和资源事实尚未从此查询推导，避免把静态配置误作机器已验证状态。
func (s *Store) ListExecutionNodes(ctx context.Context) ([]ExecutionNode, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("SQLite store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.node_id, n.display_name, n.platform, n.management_state, n.allowed_roots_json, n.tool_home, n.java_path,
		       COALESCE(n.environment_check_id, ''), n.environment_check_status, COALESCE(n.environment_check_code, ''),
		       COALESCE(n.environment_check_facts_revision, 0), n.environment_check_requested_at, n.environment_check_completed_at,
		       n.revision, n.created_at, n.updated_at,
		       a.agent_id, a.protocol_version, a.boot_id, a.last_heartbeat_at,
		       a.facts_revision, a.capacity_total, a.capacity_used, a.facts_json
		FROM execution_nodes AS n
		LEFT JOIN agents AS a ON a.node_id = n.node_id AND a.status = 'ACTIVE'
		WHERE n.management_state != 'ARCHIVED'
		ORDER BY n.normalized_name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list execution nodes: %w", err)
	}
	defer rows.Close()
	nodes := make([]ExecutionNode, 0)
	for rows.Next() {
		node, err := scanExecutionNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate execution nodes: %w", err)
	}
	return nodes, nil
}

// GetExecutionNode 返回单个未归档节点的受控配置投影。
// 调用方仍必须在读取前完成对象范围校验，仓储层不拥有浏览器身份。
func (s *Store) GetExecutionNode(ctx context.Context, nodeID string) (ExecutionNode, error) {
	if s == nil || s.db == nil {
		return ExecutionNode{}, errors.New("SQLite store is nil")
	}
	node, err := scanExecutionNode(s.db.QueryRowContext(ctx, `
		SELECT n.node_id, n.display_name, n.platform, n.management_state, n.allowed_roots_json, n.tool_home, n.java_path,
		       COALESCE(n.environment_check_id, ''), n.environment_check_status, COALESCE(n.environment_check_code, ''),
		       COALESCE(n.environment_check_facts_revision, 0), n.environment_check_requested_at, n.environment_check_completed_at,
		       n.revision, n.created_at, n.updated_at,
		       a.agent_id, a.protocol_version, a.boot_id, a.last_heartbeat_at,
		       a.facts_revision, a.capacity_total, a.capacity_used, a.facts_json
		FROM execution_nodes AS n
		LEFT JOIN agents AS a ON a.node_id = n.node_id AND a.status = 'ACTIVE'
		WHERE n.node_id = ? AND n.management_state != 'ARCHIVED'
	`, nodeID))
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionNode{}, ErrExecutionNodeNotFound
	}
	if err != nil {
		return ExecutionNode{}, fmt.Errorf("get execution node: %w", err)
	}
	return node, nil
}

// CreateExecutionNode 原子写入默认禁用的节点、审计和幂等结果。
// 新节点没有 Agent 关联或环境事实时绝不能写成已启用，防止浏览器配置绕过节点准入。
func (s *Store) CreateExecutionNode(ctx context.Context, input ExecutionNodeCreate) (ExecutionNodeCreateResult, error) {
	if err := validateExecutionNodeCreate(input); err != nil {
		return ExecutionNodeCreateResult{}, err
	}
	allowedRootsJSON, err := json.Marshal(input.AllowedRoots)
	if err != nil {
		return ExecutionNodeCreateResult{}, fmt.Errorf("encode execution node roots: %w", err)
	}
	result := ExecutionNodeCreateResult{NodeID: input.NodeID}
	err = s.withWrite(ctx, func(tx *sql.Tx) error {
		var existingDigest, existingID string
		err := tx.QueryRowContext(ctx, `
            SELECT request_digest, COALESCE(resource_id, '')
            FROM request_idempotency
            WHERE subject_id = ? AND operation = 'CREATE_EXECUTION_NODE' AND idempotency_key = ?
        `, input.CreatorSubjectID, input.IdempotencyKey).Scan(&existingDigest, &existingID)
		if err == nil {
			if existingDigest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			result.NodeID, result.Replayed = existingID, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read execution node idempotency: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO execution_nodes(
                node_id, display_name, normalized_name, platform, management_state,
				allowed_roots_json, tool_home, java_path, tool_config_ref, revision, created_by, created_at, updated_at
            ) VALUES (?, ?, ?, ?, 'DISABLED', ?, ?, ?, NULL, 1, ?, ?, ?)
		`, input.NodeID, input.DisplayName, input.NormalizedName, input.Platform, string(allowedRootsJSON), input.ToolHome, input.JavaPath,
			input.CreatorSubjectID, utcText(input.CreatedAt), utcText(input.CreatedAt)); err != nil {
			if isExecutionNodeNameConstraint(err) {
				return ErrExecutionNodeNameUnavailable
			}
			return fmt.Errorf("insert execution node: %w", err)
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.CreatorSubjectID, "EXECUTION_NODE_CREATED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO request_idempotency(
                subject_id, operation, idempotency_key, request_digest, result_status,
                resource_kind, resource_id, response_json, created_at, expires_at
            ) VALUES (?, 'CREATE_EXECUTION_NODE', ?, ?, 201, 'EXECUTION_NODE', ?, '{}', ?, ?)
        `, input.CreatorSubjectID, input.IdempotencyKey, input.RequestDigest, input.NodeID,
			utcText(input.CreatedAt), utcText(input.CreatedAt.Add(24*time.Hour))); err != nil {
			return fmt.Errorf("write execution node idempotency: %w", err)
		}
		return nil
	})
	return result, err
}

// UpdateExecutionNode 只更新管理员声明的节点配置与审计事实。
// 平台或允许根目录变化会使先前固定环境结论失效，并将节点退回禁用状态，防止旧事实覆盖新声明。
func (s *Store) UpdateExecutionNode(ctx context.Context, input ExecutionNodeUpdate) (int64, error) {
	if err := validateExecutionNodeUpdate(input); err != nil {
		return 0, err
	}
	allowedRootsJSON, err := json.Marshal(input.AllowedRoots)
	if err != nil {
		return 0, fmt.Errorf("encode execution node roots: %w", err)
	}
	var revision int64
	err = s.withWrite(ctx, func(tx *sql.Tx) error {
		update, err := tx.ExecContext(ctx, `
            UPDATE execution_nodes
			SET display_name = ?, normalized_name = ?, platform = ?, allowed_roots_json = ?, tool_home = ?, java_path = ?,
                management_state = 'DISABLED', environment_check_id = NULL, environment_check_status = 'NOT_CHECKED',
                environment_check_code = NULL, environment_check_facts_revision = NULL,
                environment_check_requested_at = NULL, environment_check_completed_at = NULL,
                revision = revision + 1, updated_at = ?
            WHERE node_id = ? AND management_state != 'ARCHIVED' AND revision = ?
		`, input.DisplayName, input.NormalizedName, input.Platform, string(allowedRootsJSON), input.ToolHome, input.JavaPath,
			utcText(input.UpdatedAt), input.NodeID, input.ExpectedRevision)
		if err != nil {
			if isExecutionNodeNameConstraint(err) {
				return ErrExecutionNodeNameUnavailable
			}
			return fmt.Errorf("update execution node: %w", err)
		}
		affected, err := update.RowsAffected()
		if err != nil {
			return fmt.Errorf("read execution node update result: %w", err)
		}
		if affected != 1 {
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM execution_nodes WHERE node_id = ? AND management_state != 'ARCHIVED'`, input.NodeID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
				return ErrExecutionNodeNotFound
			} else if err != nil {
				return fmt.Errorf("read execution node update target: %w", err)
			}
			return ErrRevisionConflict
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "EXECUTION_NODE_UPDATED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.UpdatedAt); err != nil {
			return err
		}
		revision = input.ExpectedRevision + 1
		return nil
	})
	return revision, err
}

// RequestExecutionNodeEnvironmentCheck 创建一条由当前 Agent 主动领取的固定本机运行时检查。
// 它不会把路径、命令、工具配置或 Agent 身份从浏览器传入节点侧。
func (s *Store) RequestExecutionNodeEnvironmentCheck(ctx context.Context, input ExecutionNodeEnvironmentCheckRequest) (ExecutionNodeEnvironmentCheckRequestResult, error) {
	if err := validateExecutionNodeEnvironmentCheckRequest(input); err != nil {
		return ExecutionNodeEnvironmentCheckRequestResult{}, err
	}
	result := ExecutionNodeEnvironmentCheckRequestResult{CheckID: input.CheckID}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		update, err := tx.ExecContext(ctx, `
			UPDATE execution_nodes
			SET management_state = 'DISABLED', environment_check_id = ?, environment_check_status = 'PENDING',
				environment_check_code = NULL, environment_check_facts_revision = NULL,
				environment_check_requested_at = ?, environment_check_completed_at = NULL,
				revision = revision + 1, updated_at = ?
			WHERE node_id = ? AND management_state != 'ARCHIVED' AND revision = ?
		`, input.CheckID, utcText(input.RequestedAt), utcText(input.RequestedAt), input.NodeID, input.ExpectedRevision)
		if err != nil {
			return fmt.Errorf("request execution node environment check: %w", err)
		}
		affected, err := update.RowsAffected()
		if err != nil {
			return fmt.Errorf("read execution node environment check result: %w", err)
		}
		if affected != 1 {
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM execution_nodes WHERE node_id = ? AND management_state != 'ARCHIVED'`, input.NodeID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
				return ErrExecutionNodeNotFound
			} else if err != nil {
				return fmt.Errorf("read execution node environment check target: %w", err)
			}
			return ErrRevisionConflict
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.RequestedAt); err != nil {
			return err
		}
		result.Revision = input.ExpectedRevision + 1
		return nil
	})
	return result, err
}

// EnsureCurrentExecutionNodeEnvironmentCheck 在 Agent 启动标识或固定运行时事实变化后自动续接检查。
// 此路径仅重置环境结论，故意不更改管理启用状态；首次启用仍需管理员在检查通过后显式确认。
func (s *Store) EnsureCurrentExecutionNodeEnvironmentCheck(ctx context.Context, input ExecutionNodeEnvironmentCheckRefresh) (bool, error) {
	if err := validateExecutionNodeEnvironmentCheckRefresh(input); err != nil {
		return false, err
	}
	queued := false
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		update, err := tx.ExecContext(ctx, `
			UPDATE execution_nodes
			SET environment_check_id = ?, environment_check_status = 'PENDING', environment_check_code = NULL,
				environment_check_facts_revision = ?, environment_check_requested_at = ?, environment_check_completed_at = NULL,
				revision = revision + 1, updated_at = ?
			WHERE node_id = ? AND management_state != 'ARCHIVED'
			  AND COALESCE(environment_check_facts_revision, 0) != ?
			  AND EXISTS (
				SELECT 1 FROM agents
				WHERE agents.agent_id = ? AND agents.node_id = execution_nodes.node_id
				  AND agents.status = 'ACTIVE' AND agents.facts_revision = ?
			  )
		`, input.CheckID, input.FactsRevision, utcText(input.RequestedAt), utcText(input.RequestedAt),
			input.NodeID, input.FactsRevision, input.AgentID, input.FactsRevision)
		if err != nil {
			return fmt.Errorf("refresh execution node environment check: %w", err)
		}
		affected, err := update.RowsAffected()
		if err != nil {
			return fmt.Errorf("read execution node environment refresh result: %w", err)
		}
		if affected == 0 {
			return nil
		}
		if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.RequestedAt); err != nil {
			return err
		}
		queued = true
		return nil
	})
	return queued, err
}

// GetPendingExecutionNodeEnvironmentCheck 只向当前已关联的受认证 Agent 返回本节点待处理检查标识。
// 返回值不能表达路径、命令、秘密或可变检查项，Agent 必须使用本机固定实现完成核验。
func (s *Store) GetPendingExecutionNodeEnvironmentCheck(ctx context.Context, agentID, nodeID string) (PendingExecutionNodeEnvironmentCheck, bool, error) {
	if !validAgentOpaqueValue(agentID, 256) || !validAgentOpaqueValue(nodeID, 256) {
		return PendingExecutionNodeEnvironmentCheck{}, false, errors.New("execution node environment check identity is invalid")
	}
	var checkID string
	err := s.db.QueryRowContext(ctx, `
		SELECT n.environment_check_id
		FROM execution_nodes AS n
		JOIN agents AS a ON a.node_id = n.node_id AND a.agent_id = ? AND a.status = 'ACTIVE'
		WHERE n.node_id = ? AND n.management_state != 'ARCHIVED'
		  AND n.environment_check_status = 'PENDING' AND n.environment_check_id IS NOT NULL
	`, agentID, nodeID).Scan(&checkID)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingExecutionNodeEnvironmentCheck{}, false, nil
	}
	if err != nil {
		return PendingExecutionNodeEnvironmentCheck{}, false, fmt.Errorf("get pending execution node environment check: %w", err)
	}
	return PendingExecutionNodeEnvironmentCheck{CheckID: checkID}, true, nil
}

// CompleteExecutionNodeEnvironmentCheck 只接受当前 Agent 对唯一固定运行时检查的稳定安全结论。
// Agent 机器事实版本变化或节点检查被重新发起时，旧回执不会覆盖当前节点状态。
func (s *Store) CompleteExecutionNodeEnvironmentCheck(ctx context.Context, input AgentExecutionNodeEnvironmentCheckCompletion) error {
	if err := validateAgentExecutionNodeEnvironmentCheckCompletion(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		var currentStatus, currentCode string
		var currentRevision int64
		readErr := tx.QueryRowContext(ctx, `
			SELECT n.environment_check_status, COALESCE(n.environment_check_code, ''), COALESCE(n.environment_check_facts_revision, 0)
			FROM execution_nodes AS n
			JOIN agents AS a ON a.node_id = n.node_id AND a.agent_id = ? AND a.status = 'ACTIVE'
			WHERE n.node_id = ? AND n.environment_check_id = ? AND n.management_state != 'ARCHIVED'
		`, input.AgentID, input.NodeID, input.CheckID).Scan(&currentStatus, &currentCode, &currentRevision)
		if errors.Is(readErr, sql.ErrNoRows) {
			return ErrExecutionNodeEnvironmentCheckRequired
		}
		if readErr != nil {
			return fmt.Errorf("read execution node environment check completion: %w", readErr)
		}
		if currentStatus == input.Status && currentCode == input.Code && currentRevision == input.FactsRevision {
			return nil
		}
		if currentStatus != "PENDING" {
			return ErrExecutionNodeEnvironmentCheckRequired
		}
		update, err := tx.ExecContext(ctx, `
			UPDATE execution_nodes
			SET environment_check_status = ?, environment_check_code = ?, environment_check_facts_revision = ?,
				environment_check_completed_at = ?, revision = revision + 1, updated_at = ?
			WHERE node_id = ? AND environment_check_id = ? AND environment_check_status = 'PENDING'
			  AND EXISTS (
				SELECT 1 FROM agents WHERE agents.agent_id = ? AND agents.node_id = execution_nodes.node_id
				AND agents.status = 'ACTIVE' AND agents.facts_revision = ?
			  )
		`, input.Status, input.Code, input.FactsRevision, utcText(input.CompletedAt), utcText(input.CompletedAt), input.NodeID, input.CheckID, input.AgentID, input.FactsRevision)
		if err != nil {
			return fmt.Errorf("complete execution node environment check: %w", err)
		}
		affected, err := update.RowsAffected()
		if err != nil {
			return fmt.Errorf("read execution node environment check completion result: %w", err)
		}
		if affected != 1 {
			return ErrExecutionNodeEnvironmentCheckRequired
		}
		auditResult := "FAILED"
		if input.Status == "PASSED" {
			auditResult = "SUCCEEDED"
		}
		// 环境检查协议使用 PASSED/FAILED，审计表使用统一的 SUCCEEDED/FAILED/DENIED 枚举，不能直接混写。
		return insertAudit(ctx, tx, "AGENT", input.AgentID, "EXECUTION_NODE_ENVIRONMENT_CHECK_COMPLETED", "EXECUTION_NODE", input.NodeID, auditResult, input.RequestID, input.CompletedAt)
	})
}

// EnableExecutionNode 在当前 Agent、固定检查事实和在线窗口均成立后启用节点。
// 该方法不启动任务或工具；任务级路径和数据源预检查仍是后续独立门禁。
func (s *Store) EnableExecutionNode(ctx context.Context, input ExecutionNodeEnable) (int64, error) {
	if err := validateExecutionNodeEnable(input); err != nil {
		return 0, err
	}
	var revision int64
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		update, err := tx.ExecContext(ctx, `
			UPDATE execution_nodes
			SET management_state = 'ENABLED', revision = revision + 1, updated_at = ?
			WHERE node_id = ? AND management_state = 'DISABLED' AND revision = ?
			  AND environment_check_status = 'PASSED' AND environment_check_code = 'TOOL_RUNTIME_READY'
			  AND EXISTS (
				SELECT 1 FROM agents
				WHERE agents.node_id = execution_nodes.node_id AND agents.status = 'ACTIVE'
				  AND agents.last_heartbeat_at IS NOT NULL AND agents.last_heartbeat_at >= ?
				  AND agents.facts_revision = execution_nodes.environment_check_facts_revision
			  )
		`, utcText(input.EnabledAt), input.NodeID, input.ExpectedRevision, utcText(input.OnlineAfter))
		if err != nil {
			return fmt.Errorf("enable execution node: %w", err)
		}
		affected, err := update.RowsAffected()
		if err != nil {
			return fmt.Errorf("read execution node enable result: %w", err)
		}
		if affected != 1 {
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM execution_nodes WHERE node_id = ? AND management_state != 'ARCHIVED'`, input.NodeID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
				return ErrExecutionNodeNotFound
			} else if err != nil {
				return fmt.Errorf("read execution node enable target: %w", err)
			}
			var currentRevision int64
			if err := tx.QueryRowContext(ctx, `SELECT revision FROM execution_nodes WHERE node_id = ?`, input.NodeID).Scan(&currentRevision); err != nil {
				return fmt.Errorf("read execution node enable revision: %w", err)
			}
			if currentRevision != input.ExpectedRevision {
				return ErrRevisionConflict
			}
			return ErrExecutionNodeEnableRejected
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "EXECUTION_NODE_ENABLED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.EnabledAt); err != nil {
			return err
		}
		revision = input.ExpectedRevision + 1
		return nil
	})
	return revision, err
}

// DeleteOrArchiveExecutionNode 在一个短事务内执行节点的删除语义。
// Agent 关联、草稿、预检查、任务、执行记录或连接测试任一存在时只能归档，以保留历史快照与外键；
// 有运行中任务时拒绝任何处置，避免归档导致 Agent 失去继续上报所需的机器身份。
// 无引用时才允许物理删除节点。审计事实与创建幂等记录始终保留，避免延迟重试重建已删除节点。
func (s *Store) DeleteOrArchiveExecutionNode(ctx context.Context, input ExecutionNodeDeletion) (ExecutionNodeDeletionResult, error) {
	if err := validateExecutionNodeDeletion(input); err != nil {
		return ExecutionNodeDeletionResult{}, err
	}
	result := ExecutionNodeDeletionResult{}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var currentRevision int64
		err := tx.QueryRowContext(ctx, `
			SELECT revision
			FROM execution_nodes
			WHERE node_id = ? AND management_state != 'ARCHIVED'
		`, input.NodeID).Scan(&currentRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrExecutionNodeNotFound
		}
		if err != nil {
			return fmt.Errorf("read execution node deletion state: %w", err)
		}
		if currentRevision != input.ExpectedRevision {
			return ErrRevisionConflict
		}
		var runningTaskCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM task_executions
			WHERE node_id = ? AND state IN ('STARTING', 'RUNNING', 'CANCELLING')
		`, input.NodeID).Scan(&runningTaskCount); err != nil {
			return fmt.Errorf("count running execution node tasks: %w", err)
		}
		if runningTaskCount > 0 {
			return ErrExecutionNodeHasRunningTask
		}

		var referenceCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT
				(SELECT COUNT(*) FROM agent_enrollment_tokens WHERE node_id = ?) +
				(SELECT COUNT(*) FROM agents WHERE node_id = ?) +
				(SELECT COUNT(*) FROM export_drafts WHERE node_id = ?) +
				(SELECT COUNT(*) FROM precheck_runs WHERE node_id = ?) +
				(SELECT COUNT(*) FROM tasks WHERE node_id = ?) +
				(SELECT COUNT(*) FROM task_executions WHERE node_id = ?) +
				(SELECT COUNT(*) FROM data_source_connection_test_runs WHERE node_id = ?)
		`, input.NodeID, input.NodeID, input.NodeID, input.NodeID, input.NodeID, input.NodeID, input.NodeID).Scan(&referenceCount); err != nil {
			return fmt.Errorf("count execution node references: %w", err)
		}

		if referenceCount > 0 {
			update, err := tx.ExecContext(ctx, `
				UPDATE execution_nodes
				SET management_state = 'ARCHIVED', revision = revision + 1, updated_at = ?
				WHERE node_id = ? AND management_state != 'ARCHIVED' AND revision = ?
			`, utcText(input.DeletedAt), input.NodeID, currentRevision)
			if err != nil {
				return fmt.Errorf("archive referenced execution node: %w", err)
			}
			affected, err := update.RowsAffected()
			if err != nil {
				return fmt.Errorf("read execution node archive result: %w", err)
			}
			if affected != 1 {
				return ErrRevisionConflict
			}
			if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "EXECUTION_NODE_ARCHIVED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.DeletedAt); err != nil {
				return err
			}
			revoked, err := revokeArchivedExecutionNodeAgentAccess(ctx, tx, input)
			if err != nil {
				return err
			}
			result.Outcome, result.Revision, result.AgentAccessRevoked = "ARCHIVED", currentRevision+1, revoked
			return nil
		}

		deleted, err := tx.ExecContext(ctx, `DELETE FROM execution_nodes WHERE node_id = ? AND revision = ?`, input.NodeID, currentRevision)
		if err != nil {
			return fmt.Errorf("delete execution node: %w", err)
		}
		affected, err := deleted.RowsAffected()
		if err != nil {
			return fmt.Errorf("read execution node deletion result: %w", err)
		}
		if affected != 1 {
			return ErrRevisionConflict
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "EXECUTION_NODE_DELETED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.DeletedAt); err != nil {
			return err
		}
		result.Outcome = "DELETED"
		return nil
	})
	return result, err
}

// revokeArchivedExecutionNodeAgentAccess 仅在节点归档且没有运行中任务时撤销仍有效的机器身份和一次性材料。
// 历史 Agent 行保留为可审计引用，后续认证因状态已撤销和节点已归档而失败关闭。
func revokeArchivedExecutionNodeAgentAccess(ctx context.Context, tx *sql.Tx, input ExecutionNodeDeletion) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT agent_id FROM agents WHERE node_id = ? AND status = 'ACTIVE'`, input.NodeID)
	if err != nil {
		return false, fmt.Errorf("list archived node active agents: %w", err)
	}
	defer rows.Close()
	var agentIDs []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return false, fmt.Errorf("scan archived node active agent: %w", err)
		}
		agentIDs = append(agentIDs, agentID)
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate archived node active agents: %w", err)
	}
	if len(agentIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE agents
			SET status = 'REVOKED', revoked_at = ?
			WHERE node_id = ? AND status = 'ACTIVE'
		`, utcText(input.DeletedAt), input.NodeID); err != nil {
			return false, fmt.Errorf("revoke archived node agents: %w", err)
		}
		for _, agentID := range agentIDs {
			if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "AGENT_REVOKED", "AGENT", agentID, "SUCCEEDED", input.RequestID, input.DeletedAt); err != nil {
				return false, err
			}
		}
	}
	activeEnrollments, err := tx.ExecContext(ctx, `
		UPDATE agent_enrollment_tokens
		SET status = 'REVOKED'
		WHERE node_id = ? AND status = 'ACTIVE'
	`, input.NodeID)
	if err != nil {
		return false, fmt.Errorf("revoke archived node enrollments: %w", err)
	}
	revokedEnrollments, err := activeEnrollments.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read archived node enrollment revocation result: %w", err)
	}
	// 节点归档审计已在同一事务中记录关联材料撤销；同一节点和请求不能重复写审计主键。
	return len(agentIDs) > 0 || revokedEnrollments > 0, nil
}

// IssueAgentEnrollment 为一个未归档节点签发一次性关联材料的摘要记录。
// 原始材料不写入 SQLite、审计或幂等记录；重复签发会撤销该节点尚未消费的旧材料。
func (s *Store) IssueAgentEnrollment(ctx context.Context, input AgentEnrollmentIssue) error {
	if err := validateAgentEnrollmentIssue(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		var managementState string
		err := tx.QueryRowContext(ctx, `
            SELECT management_state
            FROM execution_nodes
            WHERE node_id = ?
        `, input.NodeID).Scan(&managementState)
		if errors.Is(err, sql.ErrNoRows) || managementState == "ARCHIVED" {
			return ErrExecutionNodeNotFound
		}
		if err != nil {
			return fmt.Errorf("read enrollment node: %w", err)
		}
		replacedRows, err := tx.QueryContext(ctx, `
            SELECT agent_id
            FROM agents
            WHERE node_id = ? AND status = 'ACTIVE'
        `, input.NodeID)
		if err != nil {
			return fmt.Errorf("list agents replaced by enrollment: %w", err)
		}
		var replacedAgentIDs []string
		for replacedRows.Next() {
			var agentID string
			if err := replacedRows.Scan(&agentID); err != nil {
				replacedRows.Close()
				return fmt.Errorf("scan agent replaced by enrollment: %w", err)
			}
			replacedAgentIDs = append(replacedAgentIDs, agentID)
		}
		if err := replacedRows.Err(); err != nil {
			replacedRows.Close()
			return fmt.Errorf("iterate agents replaced by enrollment: %w", err)
		}
		if err := replacedRows.Close(); err != nil {
			return fmt.Errorf("close agents replaced by enrollment: %w", err)
		}
		if len(replacedAgentIDs) > 0 {
			if _, err := tx.ExecContext(ctx, `
                UPDATE agents
                SET status = 'REPLACED', revoked_at = ?
                WHERE node_id = ? AND status = 'ACTIVE'
            `, utcText(input.CreatedAt), input.NodeID); err != nil {
				return fmt.Errorf("replace prior active agents: %w", err)
			}
			for _, agentID := range replacedAgentIDs {
				if err := insertAudit(ctx, tx, "SUBJECT", input.ActorID, "AGENT_REPLACED", "AGENT", agentID, "SUCCEEDED", input.RequestID, input.CreatedAt); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `
            UPDATE agent_enrollment_tokens
            SET status = 'REVOKED'
            WHERE node_id = ? AND status = 'ACTIVE'
        `, input.NodeID); err != nil {
			return fmt.Errorf("revoke prior active enrollments: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO agent_enrollment_tokens(
                enrollment_id, node_id, token_digest, status, expires_at, consumed_at,
                consumed_by_agent_id, created_by, created_at
            ) VALUES (?, ?, ?, 'ACTIVE', ?, NULL, NULL, ?, ?)
        `, input.EnrollmentID, input.NodeID, input.TokenDigest, utcText(input.ExpiresAt), input.ActorID, utcText(input.CreatedAt)); err != nil {
			return fmt.Errorf("insert agent enrollment: %w", err)
		}
		return insertAudit(ctx, tx, "SUBJECT", input.ActorID, "AGENT_ENROLLMENT_ISSUED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.CreatedAt)
	})
}

// ExchangeAgentEnrollment 原子消费一次性关联材料并建立不可变的 Agent 与节点绑定。
// 已消费材料只允许同一 Agent 使用同一机器凭据安全重试，不能再次关联其他机器。
func (s *Store) ExchangeAgentEnrollment(ctx context.Context, input AgentEnrollmentExchange) (AgentEnrollmentResult, error) {
	if err := validateAgentEnrollmentExchange(input); err != nil {
		return AgentEnrollmentResult{}, err
	}
	result := AgentEnrollmentResult{}
	expired := false
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var storedNodeID, status, expiresAt, consumedByAgentID string
		var tokenDigest []byte
		err := tx.QueryRowContext(ctx, `
            SELECT node_id, token_digest, status, expires_at, COALESCE(consumed_by_agent_id, '')
            FROM agent_enrollment_tokens
            WHERE enrollment_id = ?
        `, input.EnrollmentID).Scan(&storedNodeID, &tokenDigest, &status, &expiresAt, &consumedByAgentID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEnrollmentRejected
		}
		if err != nil {
			return fmt.Errorf("read agent enrollment: %w", err)
		}
		if storedNodeID != input.NodeID || subtle.ConstantTimeCompare(tokenDigest, input.EnrollmentMaterialDigest) != 1 {
			return ErrEnrollmentRejected
		}
		if status == "CONSUMED" {
			if consumedByAgentID != input.AgentID {
				return ErrEnrollmentRejected
			}
			identity, identityErr := authenticateAgentDigest(ctx, tx, input.CredentialDigest)
			if identityErr != nil || identity.AgentID != input.AgentID || identity.NodeID != input.NodeID {
				return ErrEnrollmentRejected
			}
			runtimeConfiguration, runtimeErr := loadAgentEnrollmentRuntimeConfiguration(ctx, tx, input.NodeID)
			if runtimeErr != nil {
				return runtimeErr
			}
			result = AgentEnrollmentResult{AgentID: input.AgentID, NodeID: input.NodeID, Replayed: true, RuntimeConfiguration: runtimeConfiguration}
			return nil
		}
		if status != "ACTIVE" {
			return ErrEnrollmentRejected
		}
		expires, parseErr := time.Parse(time.RFC3339Nano, expiresAt)
		if parseErr != nil {
			return fmt.Errorf("parse enrollment expiry: %w", parseErr)
		}
		if !input.ExchangedAt.Before(expires.UTC()) {
			if _, updateErr := tx.ExecContext(ctx, `
                UPDATE agent_enrollment_tokens
                SET status = 'EXPIRED'
                WHERE enrollment_id = ? AND status = 'ACTIVE'
			`, input.EnrollmentID); updateErr != nil {
				return fmt.Errorf("expire agent enrollment: %w", updateErr)
			}
			// 过期状态必须先随短事务提交；直接返回拒绝错误会被 withWrite 回滚。
			expired = true
			return nil
		}
		runtimeConfiguration, runtimeErr := loadAgentEnrollmentRuntimeConfiguration(ctx, tx, input.NodeID)
		if runtimeErr != nil {
			return runtimeErr
		}
		var activeAgentID string
		err = tx.QueryRowContext(ctx, `
            SELECT agent_id
            FROM agents
            WHERE node_id = ? AND status = 'ACTIVE'
        `, input.NodeID).Scan(&activeAgentID)
		if err == nil {
			return ErrAgentAlreadyAssociated
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read active node agent: %w", err)
		}
		var existingAgentID string
		err = tx.QueryRowContext(ctx, `SELECT agent_id FROM agents WHERE agent_id = ?`, input.AgentID).Scan(&existingAgentID)
		if err == nil {
			return ErrEnrollmentRejected
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read existing agent: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO agents(
                agent_id, node_id, credential_digest, credential_revision, status,
                protocol_version, boot_id, last_heartbeat_at, capacity_total, capacity_used,
                facts_json, facts_revision, created_at, revoked_at
            ) VALUES (?, ?, ?, 1, 'ACTIVE', ?, NULL, NULL, 1, 0, NULL, 0, ?, NULL)
        `, input.AgentID, input.NodeID, input.CredentialDigest, input.ProtocolVersion, utcText(input.ExchangedAt)); err != nil {
			return fmt.Errorf("insert enrolled agent: %w", err)
		}
		consumed, err := tx.ExecContext(ctx, `
            UPDATE agent_enrollment_tokens
            SET status = 'CONSUMED', consumed_at = ?, consumed_by_agent_id = ?
            WHERE enrollment_id = ? AND status = 'ACTIVE'
        `, utcText(input.ExchangedAt), input.AgentID, input.EnrollmentID)
		if err != nil {
			return fmt.Errorf("consume agent enrollment: %w", err)
		}
		count, err := consumed.RowsAffected()
		if err != nil {
			return fmt.Errorf("read agent enrollment consumption: %w", err)
		}
		if count != 1 {
			return ErrEnrollmentRejected
		}
		if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "AGENT_ENROLLED", "EXECUTION_NODE", input.NodeID, "SUCCEEDED", input.RequestID, input.ExchangedAt); err != nil {
			return err
		}
		result = AgentEnrollmentResult{AgentID: input.AgentID, NodeID: input.NodeID, RuntimeConfiguration: runtimeConfiguration}
		return nil
	})
	if err != nil {
		return AgentEnrollmentResult{}, err
	}
	if expired {
		return AgentEnrollmentResult{}, ErrEnrollmentRejected
	}
	return result, err
}

// loadAgentEnrollmentRuntimeConfiguration 只在有效关联材料已验证后读取该节点的固定本机配置。
// 配置不允许为空或跨平台，避免 Agent 关联成功后退化到环境变量、PATH 或控制面动态下发路径。
func loadAgentEnrollmentRuntimeConfiguration(ctx context.Context, tx *sql.Tx, nodeID string) (AgentEnrollmentRuntimeConfiguration, error) {
	var configuration AgentEnrollmentRuntimeConfiguration
	var allowedRootsJSON, managementState string
	err := tx.QueryRowContext(ctx, `
		SELECT platform, allowed_roots_json, tool_home, java_path, revision, management_state
		FROM execution_nodes
		WHERE node_id = ?
	`, nodeID).Scan(&configuration.Platform, &allowedRootsJSON, &configuration.ToolHome, &configuration.JavaPath, &configuration.Revision, &managementState)
	if errors.Is(err, sql.ErrNoRows) || managementState == "ARCHIVED" {
		return AgentEnrollmentRuntimeConfiguration{}, ErrEnrollmentRejected
	}
	if err != nil {
		return AgentEnrollmentRuntimeConfiguration{}, fmt.Errorf("read enrollment target node: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedRootsJSON), &configuration.AllowedRoots); err != nil ||
		!ValidateExecutionNodeRuntimeConfiguration(configuration.Platform, configuration.AllowedRoots, configuration.ToolHome, configuration.JavaPath) {
		return AgentEnrollmentRuntimeConfiguration{}, ErrEnrollmentRejected
	}
	configuration.Digest = executionNodeRuntimeConfigurationDigest(configuration.Platform, configuration.ToolHome, configuration.JavaPath, configuration.AllowedRoots)
	if configuration.Digest == "" {
		return AgentEnrollmentRuntimeConfiguration{}, ErrEnrollmentRejected
	}
	return configuration, nil
}

// AuthenticateAgent 只校验机器凭据摘要与当前有效绑定，不接受浏览器身份或请求体声明替代。
func (s *Store) AuthenticateAgent(ctx context.Context, credentialDigest []byte) (AgentIdentity, error) {
	if s == nil || s.db == nil || len(credentialDigest) != sha256.Size {
		return AgentIdentity{}, ErrAgentAuthenticationFailed
	}
	return authenticateAgentDigest(ctx, s.db, credentialDigest)
}

func authenticateAgentDigest(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, credentialDigest []byte) (AgentIdentity, error) {
	var identity AgentIdentity
	var storedDigest []byte
	err := queryer.QueryRowContext(ctx, `
        SELECT a.agent_id, a.node_id, a.protocol_version, a.credential_digest
        FROM agents AS a
        JOIN execution_nodes AS n ON n.node_id = a.node_id
        WHERE a.status = 'ACTIVE' AND n.management_state != 'ARCHIVED'
          AND a.credential_digest = ?
    `, credentialDigest).Scan(&identity.AgentID, &identity.NodeID, &identity.ProtocolVersion, &storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentIdentity{}, ErrAgentAuthenticationFailed
	}
	if err != nil {
		return AgentIdentity{}, fmt.Errorf("read agent identity: %w", err)
	}
	if subtle.ConstantTimeCompare(storedDigest, credentialDigest) != 1 {
		return AgentIdentity{}, ErrAgentAuthenticationFailed
	}
	return identity, nil
}

// RecordAgentHeartbeat 只更新当前 Agent 在线与环境事实，不产生环境检查成功或任务可执行结论。
// 控制面接收时间是在线状态的唯一权威时间，Agent 观测时间仅用于展示事实采样时刻。
func (s *Store) RecordAgentHeartbeat(ctx context.Context, input AgentHeartbeat) (int64, error) {
	if err := validateAgentHeartbeat(input); err != nil {
		return 0, err
	}
	factsJSON, err := encodeAgentEnvironmentFacts(input.Facts)
	if err != nil {
		return 0, err
	}
	requestDigest, err := agentHeartbeatRequestDigest(input, factsJSON)
	if err != nil {
		return 0, err
	}
	var factsRevision int64
	err = s.withWrite(ctx, func(tx *sql.Tx) error {
		var previousRequestID, previousRequestDigest, previousBootID, previousFactsJSON sql.NullString
		var previousCapacityTotal, previousCapacityUsed int
		readErr := tx.QueryRowContext(ctx, `
            SELECT facts_revision, last_heartbeat_request_id, last_heartbeat_request_digest,
                   boot_id, capacity_total, capacity_used, facts_json
            FROM agents
            WHERE agent_id = ? AND node_id = ? AND status = 'ACTIVE' AND protocol_version = ?
              AND EXISTS (
                  SELECT 1 FROM execution_nodes
                  WHERE execution_nodes.node_id = agents.node_id
                    AND execution_nodes.management_state != 'ARCHIVED'
              )
		`, input.AgentID, input.NodeID, input.ProtocolVersion).Scan(
			&factsRevision, &previousRequestID, &previousRequestDigest, &previousBootID,
			&previousCapacityTotal, &previousCapacityUsed, &previousFactsJSON,
		)
		if errors.Is(readErr, sql.ErrNoRows) {
			return ErrAgentHeartbeatRejected
		}
		if readErr != nil {
			return fmt.Errorf("read prior agent heartbeat: %w", readErr)
		}
		if previousRequestID.Valid && previousRequestID.String == input.RequestID {
			if previousRequestDigest.Valid && subtle.ConstantTimeCompare([]byte(previousRequestDigest.String), []byte(requestDigest)) == 1 {
				return nil
			}
			return ErrAgentHeartbeatConflict
		}
		bootChanged := previousBootID.Valid && previousBootID.String != input.BootID
		factsChanged := true
		if previousFactsJSON.Valid {
			previousFacts, decodeErr := decodeAgentEnvironmentFacts(previousFactsJSON.String)
			if decodeErr != nil {
				return fmt.Errorf("decode prior agent environment facts: %w", decodeErr)
			}
			factsChanged = !sameAgentEnvironmentFacts(previousFacts, input.Facts)
		}
		factsRevisionIncrement := 0
		if !previousBootID.Valid || previousBootID.String != input.BootID ||
			previousCapacityTotal != input.CapacityTotal || previousCapacityUsed != input.CapacityUsed ||
			factsChanged {
			factsRevisionIncrement = 1
		}
		update, updateErr := tx.ExecContext(ctx, `
            UPDATE agents
            SET boot_id = ?, last_heartbeat_at = ?, capacity_total = ?, capacity_used = ?,
                facts_json = ?, facts_revision = facts_revision + ?,
                last_heartbeat_request_id = ?, last_heartbeat_request_digest = ?
            WHERE agent_id = ? AND node_id = ? AND status = 'ACTIVE' AND protocol_version = ?
              AND EXISTS (
                  SELECT 1 FROM execution_nodes
                  WHERE execution_nodes.node_id = agents.node_id
                    AND execution_nodes.management_state != 'ARCHIVED'
              )
		`, input.BootID, utcText(input.ReceivedAt), input.CapacityTotal, input.CapacityUsed,
			factsJSON, factsRevisionIncrement, input.RequestID, requestDigest, input.AgentID, input.NodeID, input.ProtocolVersion)
		if updateErr != nil {
			return fmt.Errorf("record agent heartbeat: %w", updateErr)
		}
		count, updateErr := update.RowsAffected()
		if updateErr != nil {
			return fmt.Errorf("read agent heartbeat result: %w", updateErr)
		}
		if count != 1 {
			return ErrAgentHeartbeatRejected
		}
		if bootChanged {
			// 同一机器身份出现新启动标识时，旧进程与旧 Worker 是否仍受监管均不可证明。
			// 保留任务持久状态和租约，先附加核对标识，禁止页面把陈旧“运行中”误作可信执行。
			if err := markAgentRestartReconciliationRequiredTx(ctx, tx, input.AgentID, input.ReceivedAt, input.RequestID); err != nil {
				return err
			}
		}
		if updateErr := tx.QueryRowContext(ctx, `
            SELECT facts_revision FROM agents WHERE agent_id = ?
        `, input.AgentID).Scan(&factsRevision); updateErr != nil {
			return fmt.Errorf("read agent facts revision: %w", updateErr)
		}
		return nil
	})
	return factsRevision, err
}

// sameAgentEnvironmentFacts 只比较会改变固定运行时与冻结绑定含义的机器事实。
// CPU、内存和目录空间用于展示，采样值持续变化不能使已完成的环境检查或预检查失效。
func sameAgentEnvironmentFacts(left, right AgentEnvironmentFacts) bool {
	return left.OperatingSystem == right.OperatingSystem &&
		left.Architecture == right.Architecture &&
		left.AgentVersion == right.AgentVersion &&
		left.RuntimeConfigurationDigest == right.RuntimeConfigurationDigest
}

// agentHeartbeatRequestDigest 只从 Agent 主动上报的稳定字段计算摘要。
// 控制面接收时间不参与摘要，确保网络响应丢失后的同一请求重发不会推进环境事实版本。
func agentHeartbeatRequestDigest(input AgentHeartbeat, factsJSON string) (string, error) {
	payload, err := json.Marshal(struct {
		AgentID         string `json:"agentId"`
		NodeID          string `json:"nodeId"`
		ProtocolVersion string `json:"protocolVersion"`
		BootID          string `json:"bootId"`
		RequestID       string `json:"requestId"`
		ObservedAt      string `json:"observedAt"`
		CapacityTotal   int    `json:"capacityTotal"`
		CapacityUsed    int    `json:"capacityUsed"`
		FactsJSON       string `json:"factsJson"`
	}{
		AgentID: input.AgentID, NodeID: input.NodeID, ProtocolVersion: input.ProtocolVersion, BootID: input.BootID,
		RequestID: input.RequestID, ObservedAt: input.ObservedAt.UTC().Format(time.RFC3339Nano),
		CapacityTotal: input.CapacityTotal, CapacityUsed: input.CapacityUsed, FactsJSON: factsJSON,
	})
	if err != nil {
		return "", fmt.Errorf("encode agent heartbeat request digest: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// scanExecutionNode 统一解析 SQLite 中的声明配置与受认证 Agent 当前事实。
// Agent 事实解析失败时拒绝返回不完整投影，避免把损坏数据误显示为机器已关联或环境可用。
func scanExecutionNode(row interface{ Scan(...any) error }) (ExecutionNode, error) {
	var node ExecutionNode
	var allowedRootsJSON, createdAt, updatedAt string
	var environmentCheckRequestedAt, environmentCheckCompletedAt sql.NullString
	var agentID, protocolVersion, bootID, lastHeartbeatAt, factsJSON sql.NullString
	var factsRevision, capacityTotal, capacityUsed sql.NullInt64
	if err := row.Scan(&node.NodeID, &node.DisplayName, &node.Platform, &node.ManagementState, &allowedRootsJSON, &node.ToolHome, &node.JavaPath,
		&node.EnvironmentCheck.CheckID, &node.EnvironmentCheck.Status, &node.EnvironmentCheck.Code,
		&node.EnvironmentCheck.FactsRevision, &environmentCheckRequestedAt, &environmentCheckCompletedAt,
		&node.Revision, &createdAt, &updatedAt,
		&agentID, &protocolVersion, &bootID, &lastHeartbeatAt,
		&factsRevision, &capacityTotal, &capacityUsed, &factsJSON); err != nil {
		return ExecutionNode{}, err
	}
	if err := json.Unmarshal([]byte(allowedRootsJSON), &node.AllowedRoots); err != nil {
		return ExecutionNode{}, fmt.Errorf("decode execution node roots: %w", err)
	}
	var err error
	node.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ExecutionNode{}, fmt.Errorf("parse execution node creation time: %w", err)
	}
	node.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ExecutionNode{}, fmt.Errorf("parse execution node update time: %w", err)
	}
	if environmentCheckRequestedAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339Nano, environmentCheckRequestedAt.String)
		if parseErr != nil {
			return ExecutionNode{}, fmt.Errorf("parse execution node environment check request time: %w", parseErr)
		}
		node.EnvironmentCheck.RequestedAt = &parsed
	}
	if environmentCheckCompletedAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339Nano, environmentCheckCompletedAt.String)
		if parseErr != nil {
			return ExecutionNode{}, fmt.Errorf("parse execution node environment check completion time: %w", parseErr)
		}
		node.EnvironmentCheck.CompletedAt = &parsed
	}
	if agentID.Valid {
		agent := &ExecutionNodeAgent{AgentID: agentID.String, ProtocolVersion: protocolVersion.String, BootID: bootID.String}
		if factsRevision.Valid {
			agent.FactsRevision = factsRevision.Int64
		}
		if capacityTotal.Valid {
			agent.CapacityTotal = int(capacityTotal.Int64)
		}
		if capacityUsed.Valid {
			agent.CapacityUsed = int(capacityUsed.Int64)
		}
		if lastHeartbeatAt.Valid {
			parsedHeartbeatAt, parseErr := time.Parse(time.RFC3339Nano, lastHeartbeatAt.String)
			if parseErr != nil {
				return ExecutionNode{}, fmt.Errorf("parse agent heartbeat time: %w", parseErr)
			}
			agent.LastHeartbeatAt = &parsedHeartbeatAt
		}
		if factsJSON.Valid {
			facts, parseErr := decodeAgentEnvironmentFacts(factsJSON.String)
			if parseErr != nil {
				return ExecutionNode{}, parseErr
			}
			agent.EnvironmentFacts = facts
		}
		node.Agent = agent
	}
	return node, nil
}

type persistedAgentEnvironmentFacts struct {
	OperatingSystem            string               `json:"operatingSystem"`
	Architecture               string               `json:"architecture"`
	AgentVersion               string               `json:"agentVersion"`
	ObservedAt                 string               `json:"observedAt"`
	CPUUsagePercent            *int                 `json:"cpuUsagePercent,omitempty"`
	MemoryUsagePercent         *int                 `json:"memoryUsagePercent,omitempty"`
	RuntimeConfigurationDigest string               `json:"runtimeConfigurationDigest,omitempty"`
	DataRootUsages             []AgentDataRootUsage `json:"dataRootUsages,omitempty"`
}

func encodeAgentEnvironmentFacts(facts AgentEnvironmentFacts) (string, error) {
	payload, err := json.Marshal(persistedAgentEnvironmentFacts{
		OperatingSystem: facts.OperatingSystem, Architecture: facts.Architecture, AgentVersion: facts.AgentVersion,
		ObservedAt: facts.ObservedAt.UTC().Format(time.RFC3339Nano), CPUUsagePercent: facts.CPUUsagePercent,
		MemoryUsagePercent: facts.MemoryUsagePercent, RuntimeConfigurationDigest: facts.RuntimeConfigurationDigest,
		DataRootUsages: append([]AgentDataRootUsage(nil), facts.DataRootUsages...),
	})
	if err != nil {
		return "", fmt.Errorf("encode agent environment facts: %w", err)
	}
	return string(payload), nil
}

func decodeAgentEnvironmentFacts(raw string) (AgentEnvironmentFacts, error) {
	var payload persistedAgentEnvironmentFacts
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return AgentEnvironmentFacts{}, fmt.Errorf("decode agent environment facts: %w", err)
	}
	observedAt, err := time.Parse(time.RFC3339Nano, payload.ObservedAt)
	if err != nil {
		return AgentEnvironmentFacts{}, fmt.Errorf("parse agent fact observation time: %w", err)
	}
	facts := AgentEnvironmentFacts{
		OperatingSystem: payload.OperatingSystem, Architecture: payload.Architecture, AgentVersion: payload.AgentVersion,
		ObservedAt: observedAt.UTC(), CPUUsagePercent: payload.CPUUsagePercent, MemoryUsagePercent: payload.MemoryUsagePercent,
		RuntimeConfigurationDigest: payload.RuntimeConfigurationDigest, DataRootUsages: append([]AgentDataRootUsage(nil), payload.DataRootUsages...),
	}
	if err := validateAgentEnvironmentFacts(facts); err != nil {
		return AgentEnvironmentFacts{}, fmt.Errorf("invalid stored agent environment facts: %w", err)
	}
	return facts, nil
}

// CreateDataSource 原子写入默认禁用的数据源、其首个加密凭据修订、最小审计事实与 24 小时幂等结果。
// 任一错误都会回滚全部记录，以避免存在没有凭据或未经连接测试即可使用的数据源。
func (s *Store) CreateDataSource(ctx context.Context, input DataSourceCreate) (DataSourceCreateResult, error) {
	if err := validateDataSourceCreate(input); err != nil {
		return DataSourceCreateResult{}, err
	}
	result := DataSourceCreateResult{DataSourceID: input.DataSourceID}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var existingDigest, existingID string
		err := tx.QueryRowContext(ctx, `
            SELECT request_digest, COALESCE(resource_id, '')
            FROM request_idempotency
            WHERE subject_id = ? AND operation = 'CREATE_DATA_SOURCE' AND idempotency_key = ?
        `, input.CreatorSubjectID, input.IdempotencyKey).Scan(&existingDigest, &existingID)
		if err == nil {
			if existingDigest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			result.DataSourceID, result.Replayed = existingID, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read data source idempotency: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO data_sources(
                data_source_id, display_name, normalized_name, environment, connection_kind,
                compatibility_mode, host, port, cluster_name, tenant_name, username, default_database, credential_id,
                current_credential_revision, state, revision, last_test_status, last_tested_at,
                last_test_safe_summary_json, created_by, created_at, updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'DISABLED', 1, NULL, NULL, NULL, ?, ?, ?)
		`, input.DataSourceID, input.DisplayName, input.NormalizedName, input.Environment,
			input.ConnectionKind, input.CompatibilityMode, input.Host, input.Port, input.ClusterName, input.TenantName, input.Username,
			nullableString(input.DefaultDatabase), input.CredentialID, input.CreatorSubjectID,
			utcText(input.CreatedAt), utcText(input.CreatedAt)); err != nil {
			if isDataSourceNameConstraint(err) {
				return ErrDataSourceNameUnavailable
			}
			return fmt.Errorf("insert data source: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO credential_revisions(
                credential_id, revision, data_source_id, secret_type, key_id, nonce,
                ciphertext, aad_json, status, created_at, retired_at
            ) VALUES (?, 1, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)
        `, input.CredentialID, input.DataSourceID, input.KeyID, input.Nonce, input.Ciphertext, utcText(input.CreatedAt)); err != nil {
			return fmt.Errorf("insert encrypted credential revision: %w", err)
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.CreatorSubjectID, "DATA_SOURCE_CREATED", "DATA_SOURCE", input.DataSourceID, "SUCCEEDED", input.RequestID, input.CreatedAt); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
            INSERT INTO request_idempotency(
                subject_id, operation, idempotency_key, request_digest, result_status,
                resource_kind, resource_id, response_json, created_at, expires_at
            ) VALUES (?, 'CREATE_DATA_SOURCE', ?, ?, 201, 'DATA_SOURCE', ?, '{}', ?, ?)
        `, input.CreatorSubjectID, input.IdempotencyKey, input.RequestDigest, input.DataSourceID,
			utcText(input.CreatedAt), utcText(input.CreatedAt.Add(24*time.Hour)))
		if err != nil {
			return fmt.Errorf("write data source idempotency: %w", err)
		}
		return nil
	})
	return result, err
}

// EnsureAuthSubject 建立或更新已认证身份的最小投影，使外键引用的业务写入不会依赖隐式播种数据。
// 同一内部主体不得绑定到不同外部身份，遇到此类映射冲突时必须失败关闭。
func (s *Store) EnsureAuthSubject(ctx context.Context, input AuthSubject) error {
	if err := validateAuthSubject(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		var externalSubject string
		err := tx.QueryRowContext(ctx, `SELECT external_subject FROM auth_subjects WHERE subject_id = ?`, input.SubjectID).Scan(&externalSubject)
		switch {
		case err == nil:
			if externalSubject != input.ExternalSubject {
				return errors.New("auth subject mapping conflicts with stored identity")
			}
			if _, err := tx.ExecContext(ctx, `
                UPDATE auth_subjects
                SET display_name = ?, account_status = ?, directory_revision = ?, updated_at = ?
                WHERE subject_id = ?
            `, input.DisplayName, input.AccountStatus, nullableString(input.DirectoryRevision), utcText(input.UpdatedAt), input.SubjectID); err != nil {
				return fmt.Errorf("update auth subject: %w", err)
			}
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("read auth subject: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO auth_subjects(subject_id, external_subject, display_name, account_status, directory_revision, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?, ?, ?)
        `, input.SubjectID, input.ExternalSubject, input.DisplayName, input.AccountStatus, nullableString(input.DirectoryRevision), utcText(input.CreatedAt), utcText(input.UpdatedAt)); err != nil {
			return fmt.Errorf("insert auth subject: %w", err)
		}
		return nil
	})
}

// ChangeDataSourceState 在短事务中原子切换启用或禁用状态。
// 启用必须有当前成功连接测试事实；同一目标状态会返回当前 revision，且不会重复写入审计事件。
func (s *Store) ChangeDataSourceState(ctx context.Context, input DataSourceStateChange) (DataSourceStateChangeResult, error) {
	if err := validateDataSourceStateChange(input); err != nil {
		return DataSourceStateChangeResult{}, err
	}
	result := DataSourceStateChangeResult{}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var currentState, lastTestStatus, lastTestSource string
		var currentRevision int64
		err := tx.QueryRowContext(ctx, `
            SELECT state, revision, COALESCE(last_test_status, ''), COALESCE(last_test_source, '')
            FROM data_sources
            WHERE data_source_id = ? AND state != 'ARCHIVED'
		`, input.DataSourceID).Scan(&currentState, &currentRevision, &lastTestStatus, &lastTestSource)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDataSourceNotFound
		}
		if err != nil {
			return fmt.Errorf("read data source state: %w", err)
		}
		if currentState == input.TargetState {
			if input.TargetState == "ENABLED" && (lastTestStatus != "SUCCEEDED" || lastTestSource != "AGENT_JDBC") {
				return ErrDataSourceConnectionTestRequired
			}
			result.State, result.Revision, result.Replayed = currentState, currentRevision, true
			return nil
		}
		if currentRevision != input.ExpectedRevision {
			return ErrRevisionConflict
		}
		if input.TargetState == "ENABLED" && (lastTestStatus != "SUCCEEDED" || lastTestSource != "AGENT_JDBC") {
			return ErrDataSourceConnectionTestRequired
		}
		update, err := tx.ExecContext(ctx, `
            UPDATE data_sources
            SET state = ?, revision = revision + 1, updated_at = ?
            WHERE data_source_id = ? AND revision = ?
        `, input.TargetState, utcText(input.ChangedAt), input.DataSourceID, currentRevision)
		if err != nil {
			return fmt.Errorf("change data source state: %w", err)
		}
		affected, err := update.RowsAffected()
		if err != nil {
			return fmt.Errorf("read data source state result: %w", err)
		}
		if affected != 1 {
			return ErrRevisionConflict
		}
		action := "DATA_SOURCE_DISABLED"
		if input.TargetState == "ENABLED" {
			action = "DATA_SOURCE_ENABLED"
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, action, "DATA_SOURCE", input.DataSourceID, "SUCCEEDED", input.RequestID, input.ChangedAt); err != nil {
			return err
		}
		result.State, result.Revision = input.TargetState, currentRevision+1
		return nil
	})
	return result, err
}

// DeleteOrArchiveDataSource 在一个短事务内执行已确认的删除语义。
// 任一历史草稿、预检查或任务引用都保留数据源及其凭据并只归档；
// 无引用时才删除数据源拥有的凭据修订和数据源记录，审计事实与创建幂等记录始终保留。
// 已删除资源的创建幂等记录仍需保留至少 24 小时，避免延迟重试错误地重新创建数据源。
func (s *Store) DeleteOrArchiveDataSource(ctx context.Context, input DataSourceDeletion) (DataSourceDeletionResult, error) {
	if err := validateDataSourceDeletion(input); err != nil {
		return DataSourceDeletionResult{}, err
	}
	result := DataSourceDeletionResult{}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var currentRevision int64
		err := tx.QueryRowContext(ctx, `
            SELECT revision
            FROM data_sources
            WHERE data_source_id = ? AND state != 'ARCHIVED'
        `, input.DataSourceID).Scan(&currentRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDataSourceNotFound
		}
		if err != nil {
			return fmt.Errorf("read data source deletion state: %w", err)
		}
		if currentRevision != input.ExpectedRevision {
			return ErrRevisionConflict
		}

		var referenceCount int
		if err := tx.QueryRowContext(ctx, `
            SELECT
                (SELECT COUNT(*) FROM export_drafts WHERE data_source_id = ?) +
				(SELECT COUNT(*) FROM precheck_runs WHERE data_source_id = ?) +
				(SELECT COUNT(*) FROM data_source_connection_test_runs WHERE data_source_id = ?) +
				(SELECT COUNT(*) FROM tasks WHERE data_source_id = ?)
		`, input.DataSourceID, input.DataSourceID, input.DataSourceID, input.DataSourceID).Scan(&referenceCount); err != nil {
			return fmt.Errorf("count data source references: %w", err)
		}

		if referenceCount > 0 {
			update, err := tx.ExecContext(ctx, `
                UPDATE data_sources
                SET state = 'ARCHIVED', revision = revision + 1, updated_at = ?
                WHERE data_source_id = ? AND revision = ?
            `, utcText(input.DeletedAt), input.DataSourceID, currentRevision)
			if err != nil {
				return fmt.Errorf("archive referenced data source: %w", err)
			}
			affected, err := update.RowsAffected()
			if err != nil {
				return fmt.Errorf("read data source archive result: %w", err)
			}
			if affected != 1 {
				return ErrRevisionConflict
			}
			if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "DATA_SOURCE_ARCHIVED", "DATA_SOURCE", input.DataSourceID, "SUCCEEDED", input.RequestID, input.DeletedAt); err != nil {
				return err
			}
			result.Outcome, result.Revision = "ARCHIVED", currentRevision+1
			return nil
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM credential_revisions WHERE data_source_id = ?`, input.DataSourceID); err != nil {
			return fmt.Errorf("delete data source credentials: %w", err)
		}
		deleted, err := tx.ExecContext(ctx, `DELETE FROM data_sources WHERE data_source_id = ? AND revision = ?`, input.DataSourceID, currentRevision)
		if err != nil {
			return fmt.Errorf("delete data source: %w", err)
		}
		affected, err := deleted.RowsAffected()
		if err != nil {
			return fmt.Errorf("read data source deletion result: %w", err)
		}
		if affected != 1 {
			return ErrRevisionConflict
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "DATA_SOURCE_DELETED", "DATA_SOURCE", input.DataSourceID, "SUCCEEDED", input.RequestID, input.DeletedAt); err != nil {
			return err
		}
		result.Outcome = "DELETED"
		return nil
	})
	return result, err
}

// GetDataSourceCredentialReference 只为已授权的写路径提供当前凭据引用。
// 返回值不包含密文、nonce 或任何可恢复的秘密材料。
func (s *Store) GetDataSourceCredentialReference(ctx context.Context, dataSourceID string) (DataSourceCredentialReference, error) {
	if s == nil || s.db == nil {
		return DataSourceCredentialReference{}, errors.New("SQLite store is nil")
	}
	var reference DataSourceCredentialReference
	err := s.db.QueryRowContext(ctx, `
        SELECT credential_id, current_credential_revision
        FROM data_sources
        WHERE data_source_id = ? AND state != 'ARCHIVED'
    `, dataSourceID).Scan(&reference.CredentialID, &reference.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return DataSourceCredentialReference{}, ErrDataSourceNotFound
	}
	if err != nil {
		return DataSourceCredentialReference{}, fmt.Errorf("read data source credential reference: %w", err)
	}
	return reference, nil
}

// UpdateDataSource 同时更新连接字段、可选凭据 revision 与审计事实。
// 密码轮换与普通字段更新共用 revision 条件，不能形成部分成功的连接配置；
// 任一参与连接测试的字段变化都会在同一事务中使测试事实失效并强制禁用数据源。
func (s *Store) UpdateDataSource(ctx context.Context, input DataSourceUpdate) (DataSourceUpdateResult, error) {
	if err := validateDataSourceUpdate(input); err != nil {
		return DataSourceUpdateResult{}, err
	}
	result := DataSourceUpdateResult{}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var credentialID, currentConnectionKind, currentCompatibilityMode, currentHost, currentClusterName, currentTenantName, currentUsername, currentDefaultDatabase string
		var credentialRevision int64
		var currentPort int
		err := tx.QueryRowContext(ctx, `
            SELECT credential_id, current_credential_revision, connection_kind, compatibility_mode,
                   host, port, cluster_name, tenant_name, username, COALESCE(default_database, '')
            FROM data_sources
            WHERE data_source_id = ? AND state != 'ARCHIVED' AND revision = ?
		`, input.DataSourceID, input.ExpectedRevision).Scan(
			&credentialID, &credentialRevision, &currentConnectionKind, &currentCompatibilityMode,
			&currentHost, &currentPort, &currentClusterName, &currentTenantName, &currentUsername, &currentDefaultDatabase,
		)
		if errors.Is(err, sql.ErrNoRows) {
			var exists int
			if checkErr := tx.QueryRowContext(ctx, `SELECT 1 FROM data_sources WHERE data_source_id = ? AND state != 'ARCHIVED'`, input.DataSourceID).Scan(&exists); errors.Is(checkErr, sql.ErrNoRows) {
				return ErrDataSourceNotFound
			} else if checkErr != nil {
				return fmt.Errorf("check data source update target: %w", checkErr)
			}
			return ErrRevisionConflict
		}
		if err != nil {
			return fmt.Errorf("read data source update target: %w", err)
		}
		connectionTestInvalidated := input.Password != nil || input.ConnectionKind != currentConnectionKind || input.CompatibilityMode != currentCompatibilityMode || input.Host != currentHost || input.Port != currentPort || input.ClusterName != currentClusterName || input.TenantName != currentTenantName || input.Username != currentUsername
		newCredentialRevision := credentialRevision
		if input.Password != nil {
			if input.Password.CredentialID != credentialID || input.Password.Revision != credentialRevision+1 {
				return errors.New("data source password revision does not match current credential")
			}
			if _, err := tx.ExecContext(ctx, `
                INSERT INTO credential_revisions(
                    credential_id, revision, data_source_id, secret_type, key_id, nonce,
                    ciphertext, aad_json, status, created_at, retired_at
                ) VALUES (?, ?, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)
            `, input.Password.CredentialID, input.Password.Revision, input.DataSourceID,
				input.Password.KeyID, input.Password.Nonce, input.Password.Ciphertext, utcText(input.UpdatedAt)); err != nil {
				return fmt.Errorf("insert rotated encrypted credential: %w", err)
			}
			retired, err := tx.ExecContext(ctx, `
                UPDATE credential_revisions
                SET status = 'SUPERSEDED', retired_at = ?
                WHERE credential_id = ? AND revision = ? AND status = 'ACTIVE'
			`, utcText(input.UpdatedAt), credentialID, credentialRevision)
			if err != nil {
				return fmt.Errorf("retire prior credential revision: %w", err)
			}
			retiredCount, err := retired.RowsAffected()
			if err != nil {
				return fmt.Errorf("read prior credential retirement result: %w", err)
			}
			if retiredCount != 1 {
				return errors.New("current credential revision is not active")
			}
			newCredentialRevision = input.Password.Revision
		}
		if _, err := tx.ExecContext(ctx, `
            UPDATE data_sources
            SET display_name = ?, normalized_name = ?, environment = ?, connection_kind = ?,
                compatibility_mode = ?, host = ?, port = ?, cluster_name = ?, tenant_name = ?, username = ?, default_database = ?,
                current_credential_revision = ?,
                state = CASE WHEN ? THEN 'DISABLED' ELSE state END,
                revision = revision + 1,
				last_test_status = CASE WHEN ? THEN NULL ELSE last_test_status END,
				last_tested_at = CASE WHEN ? THEN NULL ELSE last_tested_at END,
				last_test_safe_summary_json = CASE WHEN ? THEN NULL ELSE last_test_safe_summary_json END,
				last_test_source = CASE WHEN ? THEN NULL ELSE last_test_source END,
				updated_at = ?
            WHERE data_source_id = ? AND revision = ?
		`, input.DisplayName, input.NormalizedName, input.Environment, input.ConnectionKind,
			input.CompatibilityMode, input.Host, input.Port, input.ClusterName, input.TenantName, input.Username, nullableString(input.DefaultDatabase),
			newCredentialRevision, connectionTestInvalidated, connectionTestInvalidated, connectionTestInvalidated, connectionTestInvalidated, connectionTestInvalidated,
			utcText(input.UpdatedAt), input.DataSourceID, input.ExpectedRevision); err != nil {
			if isDataSourceNameConstraint(err) {
				return ErrDataSourceNameUnavailable
			}
			return fmt.Errorf("update data source: %w", err)
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.ActorSubjectID, "DATA_SOURCE_UPDATED", "DATA_SOURCE", input.DataSourceID, "SUCCEEDED", input.RequestID, input.UpdatedAt); err != nil {
			return err
		}
		result.Revision, result.CredentialRevision, result.ConnectionTestInvalidated = input.ExpectedRevision+1, newCredentialRevision, connectionTestInvalidated
		return nil
	})
	return result, err
}

// RevokeLocalSyntheticDataSourceTestFacts 仅供显式本机 MVP 启动时撤销旧版未标记的合成连接测试事实。
// 它绝不清除 AGENT_JDBC 的节点实测事实；撤销范围只限于历史合成遗留结果，避免重启改变真实测试结论。
func (s *Store) RevokeLocalSyntheticDataSourceTestFacts(ctx context.Context, revokedAt time.Time) (int, error) {
	if revokedAt.IsZero() {
		return 0, errors.New("local synthetic test revocation time is required")
	}
	var revoked int
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
            SELECT data_source_id
            FROM data_sources
            WHERE state != 'ARCHIVED'
              AND last_test_source IS NULL
              AND (
                  last_test_status IS NOT NULL
                  OR last_tested_at IS NOT NULL
                  OR last_test_safe_summary_json IS NOT NULL
              )
        `)
		if err != nil {
			return fmt.Errorf("list local synthetic test facts: %w", err)
		}
		ids := make([]string, 0)
		for rows.Next() {
			var dataSourceID string
			if err := rows.Scan(&dataSourceID); err != nil {
				rows.Close()
				return fmt.Errorf("scan local synthetic test fact: %w", err)
			}
			ids = append(ids, dataSourceID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate local synthetic test facts: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close local synthetic test facts: %w", err)
		}

		requestID := fmt.Sprintf("local-mvp-revoke-synthetic-tests-%d", revokedAt.UTC().UnixNano())
		for _, dataSourceID := range ids {
			if _, err := tx.ExecContext(ctx, `
                UPDATE data_sources
                SET state = 'DISABLED', revision = revision + 1,
					last_test_status = NULL, last_tested_at = NULL,
					last_test_safe_summary_json = NULL, last_test_source = NULL, updated_at = ?
                WHERE data_source_id = ?
            `, utcText(revokedAt), dataSourceID); err != nil {
				return fmt.Errorf("revoke local synthetic test fact: %w", err)
			}
			if err := insertAudit(ctx, tx, "SYSTEM", "local-mvp", "DATA_SOURCE_LOCAL_SYNTHETIC_TEST_REVOKED", "DATA_SOURCE", dataSourceID, "SUCCEEDED", requestID, revokedAt); err != nil {
				return err
			}
			revoked++
		}
		return nil
	})
	return revoked, err
}

// CreateExportDraft 原子保存首条切片草稿、审计和创建幂等结果。
// 数据源必须启用且当前连接配置已有成功测试事实，归档、禁用或未验证数据源不能形成新的导出草稿。
func (s *Store) CreateExportDraft(ctx context.Context, input ExportDraftCreate) (ExportDraftCreateResult, error) {
	if err := validateExportDraftCreate(input); err != nil {
		return ExportDraftCreateResult{}, err
	}
	result := ExportDraftCreateResult{DraftID: input.DraftID}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var digest, existingID string
		err := tx.QueryRowContext(ctx, `
            SELECT request_digest, COALESCE(resource_id, '')
            FROM request_idempotency
            WHERE subject_id = ? AND operation = 'CREATE_EXPORT_DRAFT' AND idempotency_key = ?
        `, input.OwnerSubjectID, input.IdempotencyKey).Scan(&digest, &existingID)
		if err == nil {
			if digest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			result.DraftID, result.Replayed = existingID, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read export draft idempotency: %w", err)
		}
		insert, err := tx.ExecContext(ctx, `
            INSERT INTO export_drafts(
                draft_id, owner_subject_id, data_source_id, node_id, revision,
                tool_version, metadata_version, capability_version, config_json,
                config_fingerprint, invalidation_json, created_at, updated_at
            )
            SELECT ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?
            WHERE EXISTS (
                SELECT 1 FROM data_sources
                WHERE data_source_id = ? AND state = 'ENABLED'
                  AND last_test_status = 'SUCCEEDED' AND last_test_source = 'AGENT_JDBC'
            )
              AND EXISTS (SELECT 1 FROM execution_nodes WHERE node_id = ? AND management_state = 'ENABLED')
        `, input.DraftID, input.OwnerSubjectID, input.DataSourceID, input.NodeID,
			input.ToolVersion, input.MetadataVersion, input.CapabilityVersion, input.ConfigJSON,
			input.ConfigFingerprint, input.InvalidationJSON, utcText(input.CreatedAt), utcText(input.UpdatedAt),
			input.DataSourceID, input.NodeID)
		if err != nil {
			return fmt.Errorf("insert export draft: %w", err)
		}
		affected, err := insert.RowsAffected()
		if err != nil {
			return fmt.Errorf("read export draft insert result: %w", err)
		}
		if affected != 1 {
			return ErrDataSourceNotFound
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.OwnerSubjectID, "EXPORT_DRAFT_CREATED", "EXPORT_DRAFT", input.DraftID, "SUCCEEDED", input.RequestID, input.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO request_idempotency(
                subject_id, operation, idempotency_key, request_digest, result_status,
                resource_kind, resource_id, response_json, created_at, expires_at
            ) VALUES (?, 'CREATE_EXPORT_DRAFT', ?, ?, 201, 'EXPORT_DRAFT', ?, '{}', ?, ?)
        `, input.OwnerSubjectID, input.IdempotencyKey, input.RequestDigest, input.DraftID,
			utcText(input.CreatedAt), utcText(input.CreatedAt.Add(24*time.Hour))); err != nil {
			return fmt.Errorf("write export draft idempotency: %w", err)
		}
		return nil
	})
	return result, err
}

// GetExportDraft 返回草稿配置与版本事实；对象范围校验由控制面完成。
func (s *Store) GetExportDraft(ctx context.Context, draftID string) (ExportDraft, error) {
	if s == nil || s.db == nil {
		return ExportDraft{}, errors.New("SQLite store is nil")
	}
	var draft ExportDraft
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
        SELECT draft_id, owner_subject_id, data_source_id, COALESCE(node_id, ''), revision,
               tool_version, metadata_version, capability_version, config_json,
               COALESCE(config_fingerprint, ''), invalidation_json, created_at, updated_at
        FROM export_drafts WHERE draft_id = ?
    `, draftID).Scan(&draft.DraftID, &draft.OwnerSubjectID, &draft.DataSourceID, &draft.NodeID, &draft.Revision,
		&draft.ToolVersion, &draft.MetadataVersion, &draft.CapabilityVersion, &draft.ConfigJSON,
		&draft.ConfigFingerprint, &draft.InvalidationJSON, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ExportDraft{}, ErrDataSourceNotFound
	}
	if err != nil {
		return ExportDraft{}, fmt.Errorf("read export draft: %w", err)
	}
	var parseErr error
	if draft.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
		return ExportDraft{}, fmt.Errorf("parse export draft create time: %w", parseErr)
	}
	if draft.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt); parseErr != nil {
		return ExportDraft{}, fmt.Errorf("parse export draft update time: %w", parseErr)
	}
	draft.CreatedAt, draft.UpdatedAt = draft.CreatedAt.UTC(), draft.UpdatedAt.UTC()
	return draft, nil
}

// CreatePrecheck 冻结草稿 revision、指纹、数据源凭据 revision 和节点绑定。
// 该事务只排队固定检查，不领取租约、不启动进程，也不创建执行任务。
func (s *Store) CreatePrecheck(ctx context.Context, input PrecheckCreate) (PrecheckCreateResult, error) {
	if err := validatePrecheckCreate(input); err != nil {
		return PrecheckCreateResult{}, err
	}
	result := PrecheckCreateResult{PrecheckID: input.PrecheckID}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var digest, existingID string
		err := tx.QueryRowContext(ctx, `SELECT request_digest, COALESCE(resource_id, '') FROM request_idempotency WHERE subject_id = ? AND operation = 'CREATE_EXPORT_PRECHECK' AND idempotency_key = ?`, input.CreatorSubjectID, input.IdempotencyKey).Scan(&digest, &existingID)
		if err == nil {
			if digest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			result.PrecheckID, result.Replayed = existingID, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read precheck idempotency: %w", err)
		}
		var bindingAgentID string
		var nodeFactsRevision int64
		err = tx.QueryRowContext(ctx, `
            SELECT a.agent_id, a.facts_revision
            FROM execution_nodes AS n
			JOIN agents AS a ON a.node_id = n.node_id AND a.status = 'ACTIVE'
			WHERE n.node_id = ? AND n.management_state = 'ENABLED' AND a.facts_revision > 0
        `, input.NodeID).Scan(&bindingAgentID, &nodeFactsRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPrecheckInvalid
		}
		if err != nil {
			return fmt.Errorf("read precheck node facts: %w", err)
		}
		binding := PrecheckBinding{
			PrecheckID: input.PrecheckID, DraftID: input.DraftID, DraftRevision: input.DraftRevision,
			ConfigFingerprint: input.ConfigFingerprint, DataSourceID: input.DataSourceID,
			CredentialID: input.CredentialID, CredentialRevision: input.CredentialRevision,
			NodeID: input.NodeID, NodeFactsRevision: nodeFactsRevision, BindingAgentID: bindingAgentID, ValidUntil: input.ValidUntil,
		}
		bindingDigest, err := precheckBindingDigest(binding)
		if err != nil {
			return err
		}
		insert, err := tx.ExecContext(ctx, `
            INSERT INTO precheck_runs(
                precheck_id, draft_id, draft_revision, config_fingerprint, data_source_id,
                credential_id, credential_revision, node_id, agent_id, status, lease_id,
                lease_epoch, lease_expires_at, result_json, integrity_status, valid_until,
				created_at, completed_at, node_facts_revision, binding_digest, binding_agent_id
            )
            SELECT ?, d.draft_id, d.revision, d.config_fingerprint, d.data_source_id,
                   ds.credential_id, ds.current_credential_revision, d.node_id, NULL, 'PENDING', NULL,
				   NULL, NULL, NULL, 'UNKNOWN', ?, ?, NULL, ?, ?, ?
            FROM export_drafts d JOIN data_sources ds ON ds.data_source_id = d.data_source_id
            WHERE d.draft_id = ? AND d.revision = ? AND d.config_fingerprint = ?
              AND d.data_source_id = ? AND d.node_id = ?
              AND ds.credential_id = ? AND ds.current_credential_revision = ?
              AND ds.state = 'ENABLED' AND ds.last_test_status = 'SUCCEEDED'
              AND ds.last_test_source = 'AGENT_JDBC'
		`, input.PrecheckID, utcText(input.ValidUntil), utcText(input.CreatedAt), nodeFactsRevision, bindingDigest, bindingAgentID,
			input.DraftID, input.DraftRevision, input.ConfigFingerprint, input.DataSourceID, input.NodeID,
			input.CredentialID, input.CredentialRevision)
		if err != nil {
			return fmt.Errorf("insert export precheck: %w", err)
		}
		affected, err := insert.RowsAffected()
		if err != nil {
			return fmt.Errorf("read precheck insert result: %w", err)
		}
		if affected != 1 {
			return ErrPrecheckInvalid
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.CreatorSubjectID, "EXPORT_PRECHECK_CREATED", "PRECHECK", input.PrecheckID, "SUCCEEDED", input.RequestID, input.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id, operation, idempotency_key, request_digest, result_status, resource_kind, resource_id, response_json, created_at, expires_at) VALUES (?, 'CREATE_EXPORT_PRECHECK', ?, ?, 202, 'PRECHECK', ?, '{}', ?, ?)`, input.CreatorSubjectID, input.IdempotencyKey, input.RequestDigest, input.PrecheckID, utcText(input.CreatedAt), utcText(input.CreatedAt.Add(24*time.Hour))); err != nil {
			return fmt.Errorf("write precheck idempotency: %w", err)
		}
		return nil
	})
	return result, err
}

// GetPrecheckRun 返回预检查的非敏感状态；权限与草稿所有权由控制面校验。
func (s *Store) GetPrecheckRun(ctx context.Context, precheckID string) (PrecheckRun, error) {
	if s == nil || s.db == nil {
		return PrecheckRun{}, errors.New("SQLite store is nil")
	}
	var run PrecheckRun
	var validUntil, createdAt, resultJSON string
	err := s.db.QueryRowContext(ctx, `
        SELECT precheck_id, draft_id, draft_revision, config_fingerprint, data_source_id,
               credential_id, credential_revision, node_id, node_facts_revision,
               COALESCE(binding_agent_id, ''), COALESCE(binding_digest, ''), status, integrity_status, COALESCE(result_json, ''), valid_until, created_at
        FROM precheck_runs
        WHERE precheck_id = ?
    `, precheckID).Scan(
		&run.PrecheckID, &run.DraftID, &run.DraftRevision, &run.ConfigFingerprint, &run.DataSourceID,
		&run.CredentialID, &run.CredentialRevision, &run.NodeID, &run.NodeFactsRevision,
		&run.BindingAgentID, &run.BindingDigest, &run.Status, &run.IntegrityStatus, &resultJSON, &validUntil, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PrecheckRun{}, ErrDataSourceNotFound
	}
	if err != nil {
		return PrecheckRun{}, fmt.Errorf("read precheck run: %w", err)
	}
	if run.ValidUntil, err = time.Parse(time.RFC3339Nano, validUntil); err != nil {
		return PrecheckRun{}, fmt.Errorf("parse precheck expiry: %w", err)
	}
	if run.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return PrecheckRun{}, fmt.Errorf("parse precheck create time: %w", err)
	}
	run.ValidUntil, run.CreatedAt = run.ValidUntil.UTC(), run.CreatedAt.UTC()
	run.Results = decodePrecheckResults(resultJSON)
	return run, nil
}

// CompletePrecheck 仅兼容已由旧合成协调器校验的 PENDING 预检查完成事实。
// 正式持久化租约路径必须使用 CompleteAgentPrecheck，避免绕过 Agent、租约和绑定摘要校验。
func (s *Store) CompletePrecheck(ctx context.Context, input PrecheckCompletion) error {
	if err := validatePrecheckCompletion(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		status := "FAILED"
		if input.Succeeded {
			status = "SUCCEEDED"
		}
		updated, err := tx.ExecContext(ctx, `UPDATE precheck_runs SET status = ?, result_json = ?, integrity_status = ?, completed_at = ? WHERE precheck_id = ? AND status = 'PENDING'`, status, input.ResultJSON, input.IntegrityStatus, utcText(input.CompletedAt), input.PrecheckID)
		if err != nil {
			return fmt.Errorf("complete precheck: %w", err)
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("read precheck completion result: %w", err)
		}
		if affected != 1 {
			return ErrPrecheckInvalid
		}
		return nil
	})
}

// ClaimPrecheck 原子领取指定预检查的短租约，并冻结当前 Agent 的环境事实版本。
// 该方法仅保留给既有受控合成适配；受认证 Agent 必须使用 ClaimNextPrecheck，不能提交预检查标识。
func (s *Store) ClaimPrecheck(ctx context.Context, input PrecheckClaim) (PrecheckLeaseGrant, error) {
	if err := validatePrecheckClaim(input); err != nil {
		return PrecheckLeaseGrant{}, err
	}
	var grant PrecheckLeaseGrant
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expirePrechecksTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readAgentPrecheckReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if found {
			if receipt.Operation != "CLAIM" || receipt.RequestDigest != input.RequestDigest || receipt.PrecheckID != input.PrecheckID || receipt.LeaseID != input.LeaseID {
				return ErrIdempotencyConflict
			}
			grant, err = replayPrecheckLeaseGrant(ctx, tx, receipt)
			return err
		}
		grant, outcomeErr = claimPrecheckTx(ctx, tx, input)
		if errors.Is(outcomeErr, ErrPrecheckLeaseExpired) {
			return nil
		}
		return outcomeErr
	})
	if err != nil {
		return grant, err
	}
	return grant, outcomeErr
}

// ClaimNextPrecheck 由控制面在单个短事务内选择并领取当前 Agent 的下一条固定预检查。
// 请求不接受预检查标识；没有候选时返回 found=false，且不写入回执或租约。
func (s *Store) ClaimNextPrecheck(ctx context.Context, input PrecheckClaimNext) (PrecheckLeaseGrant, bool, error) {
	if err := validatePrecheckClaimNext(input); err != nil {
		return PrecheckLeaseGrant{}, false, err
	}
	var grant PrecheckLeaseGrant
	var claimed bool
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expirePrechecksTx(ctx, tx, input.Now); err != nil {
			return err
		}
		if err := expireExecutionLeasesTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, replayed, err := readAgentPrecheckReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if replayed {
			if receipt.Operation != "CLAIM" || receipt.RequestDigest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			grant, err = replayPrecheckLeaseGrant(ctx, tx, receipt)
			if err == nil {
				claimed = true
			}
			return err
		}

		var precheckID string
		err = tx.QueryRowContext(ctx, `
            SELECT precheck_id
            FROM precheck_runs
            WHERE status = 'PENDING'
              AND node_id = ?
              AND binding_agent_id = ?
              AND valid_until > ?
            ORDER BY created_at ASC, precheck_id ASC
            LIMIT 1
        `, input.NodeID, input.AgentID, utcText(input.Now)).Scan(&precheckID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find next precheck: %w", err)
		}
		claim := PrecheckClaim{
			AgentID: input.AgentID, NodeID: input.NodeID, PrecheckID: precheckID, LeaseID: input.LeaseID,
			RequestID: input.RequestID, RequestDigest: input.RequestDigest, LeaseTTL: input.LeaseTTL, Now: input.Now,
		}
		grant, outcomeErr = claimPrecheckTx(ctx, tx, claim)
		if errors.Is(outcomeErr, ErrPrecheckLeaseExpired) {
			return nil
		}
		if outcomeErr == nil {
			claimed = true
		}
		return outcomeErr
	})
	if err != nil {
		return grant, claimed, err
	}
	return grant, claimed, outcomeErr
}

// claimPrecheckTx 在已经取得 SQLite 写事务后复验绑定并写入租约与回执。
// 任何状态或事实漂移均失败关闭，调用方只对已标记过期的结果提交事务以保存过期事实。
func claimPrecheckTx(ctx context.Context, tx *sql.Tx, input PrecheckClaim) (PrecheckLeaseGrant, error) {
	run, err := readStoredPrecheck(ctx, tx, input.PrecheckID)
	if err != nil {
		return PrecheckLeaseGrant{}, err
	}
	if run.Status == "EXPIRED" {
		return PrecheckLeaseGrant{}, ErrPrecheckLeaseExpired
	}
	if !run.Binding.ValidUntil.After(input.Now) {
		if err := markPrecheckExpiredTx(ctx, tx, run.Binding.PrecheckID, input.Now); err != nil {
			return PrecheckLeaseGrant{}, err
		}
		return PrecheckLeaseGrant{}, ErrPrecheckLeaseExpired
	}
	if run.Status != "PENDING" || run.Binding.NodeID != input.NodeID {
		return PrecheckLeaseGrant{}, ErrPrecheckLeaseRejected
	}
	if err := validateCurrentPrecheckBinding(ctx, tx, input.AgentID, run.Binding); err != nil {
		return PrecheckLeaseGrant{}, err
	}
	executionContext, err := readPrecheckExecutionContext(ctx, tx, run.Binding)
	if err != nil {
		return PrecheckLeaseGrant{}, err
	}
	if err := ensurePrecheckAgentIdle(ctx, tx, input.AgentID); err != nil {
		return PrecheckLeaseGrant{}, err
	}
	expiresAt := input.Now.Add(input.LeaseTTL)
	if expiresAt.After(run.Binding.ValidUntil) {
		expiresAt = run.Binding.ValidUntil
	}
	if !expiresAt.After(input.Now) {
		if err := markPrecheckExpiredTx(ctx, tx, run.Binding.PrecheckID, input.Now); err != nil {
			return PrecheckLeaseGrant{}, err
		}
		return PrecheckLeaseGrant{}, ErrPrecheckLeaseExpired
	}
	leaseEpoch := int64(1)
	updated, err := tx.ExecContext(ctx, `
        UPDATE precheck_runs
        SET agent_id = ?, status = 'LEASED', lease_id = ?, lease_epoch = ?, lease_expires_at = ?
        WHERE precheck_id = ? AND status = 'PENDING'
    `, input.AgentID, input.LeaseID, leaseEpoch, utcText(expiresAt), input.PrecheckID)
	if err != nil {
		return PrecheckLeaseGrant{}, fmt.Errorf("claim export precheck: %w", err)
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return PrecheckLeaseGrant{}, fmt.Errorf("read precheck claim result: %w", err)
	}
	if affected != 1 {
		return PrecheckLeaseGrant{}, ErrPrecheckLeaseRejected
	}
	receipt := agentPrecheckReceipt{
		AgentID: input.AgentID, RequestID: input.RequestID, Operation: "CLAIM", RequestDigest: input.RequestDigest,
		PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: leaseEpoch,
		BindingDigest: run.Binding.BindingDigest, Status: "LEASED", ExpiresAt: expiresAt, CreatedAt: input.Now,
	}
	if err := insertAgentPrecheckReceipt(ctx, tx, receipt); err != nil {
		return PrecheckLeaseGrant{}, err
	}
	return PrecheckLeaseGrant{
		PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: leaseEpoch,
		ExpiresAt: expiresAt, Binding: run.Binding, ExecutionContext: executionContext,
	}, nil
}

// AcknowledgePrecheck 持久化 Agent 对固定检查清单和当前租约的确认。
// 确认只记录受控回执；它不接受结果、秘密或任何额外运行参数。
func (s *Store) AcknowledgePrecheck(ctx context.Context, input PrecheckAcknowledgement) (PrecheckLeaseGrant, error) {
	if err := validatePrecheckAcknowledgement(input); err != nil {
		return PrecheckLeaseGrant{}, err
	}
	var grant PrecheckLeaseGrant
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expirePrechecksTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readAgentPrecheckReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if found {
			if receipt.Operation != "ACKNOWLEDGE" || receipt.RequestDigest != input.RequestDigest || receipt.PrecheckID != input.PrecheckID ||
				receipt.LeaseID != input.LeaseID || receipt.LeaseEpoch != input.LeaseEpoch ||
				receipt.BindingDigest != input.BindingDigest {
				return ErrIdempotencyConflict
			}
			grant, err = replayPrecheckLeaseGrant(ctx, tx, receipt)
			return err
		}

		run, err := readStoredPrecheck(ctx, tx, input.PrecheckID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" {
			outcomeErr = ErrPrecheckLeaseExpired
			return nil
		}
		if !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if err := markPrecheckExpiredTx(ctx, tx, run.Binding.PrecheckID, input.Now); err != nil {
				return err
			}
			outcomeErr = ErrPrecheckLeaseExpired
			return nil
		}
		if run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest {
			return ErrPrecheckLeaseRejected
		}
		if err := validateCurrentPrecheckBinding(ctx, tx, input.AgentID, run.Binding); err != nil {
			return err
		}
		executionContext, err := readPrecheckExecutionContext(ctx, tx, run.Binding)
		if err != nil {
			return err
		}
		receipt = agentPrecheckReceipt{
			AgentID: input.AgentID, RequestID: input.RequestID, Operation: "ACKNOWLEDGE", RequestDigest: input.RequestDigest,
			PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: "ACKNOWLEDGED", ExpiresAt: run.LeaseExpiresAt, CreatedAt: input.Now,
		}
		if err := insertAgentPrecheckReceipt(ctx, tx, receipt); err != nil {
			return err
		}
		grant = PrecheckLeaseGrant{
			PrecheckID: run.Binding.PrecheckID, LeaseID: run.LeaseID, LeaseEpoch: run.LeaseEpoch,
			ExpiresAt: run.LeaseExpiresAt, Binding: run.Binding, ExecutionContext: executionContext,
		}
		return nil
	})
	if err != nil {
		return grant, err
	}
	return grant, outcomeErr
}

// ResolvePrecheckDatabaseConnection 在有效、已确认的预检查租约内返回仍加密的数据库连接材料。
// 该方法只保存无秘密请求回执和审计意图；AES-GCM 解密必须在事务外由受控控制面服务短时完成。
func (s *Store) ResolvePrecheckDatabaseConnection(ctx context.Context, input PrecheckSecretResolutionRequest) (EncryptedPrecheckDatabaseConnection, error) {
	if err := validatePrecheckSecretResolutionRequest(input); err != nil {
		return EncryptedPrecheckDatabaseConnection{}, err
	}
	var connection EncryptedPrecheckDatabaseConnection
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expirePrechecksTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readPrecheckSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if found && !samePrecheckSecretResolutionReceipt(receipt, input) {
			return ErrIdempotencyConflict
		}
		run, err := readStoredPrecheck(ctx, tx, input.PrecheckID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" {
			outcomeErr = ErrPrecheckLeaseExpired
			return nil
		}
		if run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest {
			return ErrPrecheckLeaseRejected
		}
		if !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if err := markPrecheckExpiredTx(ctx, tx, run.Binding.PrecheckID, input.Now); err != nil {
				return err
			}
			outcomeErr = ErrPrecheckLeaseExpired
			return nil
		}
		if err := validateCurrentPrecheckBinding(ctx, tx, input.AgentID, run.Binding); err != nil {
			return err
		}
		acknowledged, err := hasPrecheckAcknowledgementReceipt(ctx, tx, input.AgentID, run)
		if err != nil {
			return err
		}
		if !acknowledged {
			return ErrPrecheckLeaseRejected
		}
		connection, err = readEncryptedPrecheckDatabaseConnection(ctx, tx, run.Binding)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
		if err := insertPrecheckSecretResolutionReceipt(ctx, tx, precheckSecretResolutionReceipt{
			AgentID: input.AgentID, RequestID: input.RequestID, RequestDigest: input.RequestDigest,
			PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: "AUTHORIZED", CreatedAt: input.Now,
		}); err != nil {
			connection.Destroy()
			return err
		}
		if err := insertPrecheckSecretResolutionAudit(ctx, tx, input.AgentID, input.PrecheckID, input.RequestID, "EXPORT_PRECHECK_SECRET_RESOLVE_REQUESTED", "SUCCEEDED", input.Now); err != nil {
			connection.Destroy()
			return err
		}
		return nil
	})
	if err != nil {
		connection.Destroy()
		return EncryptedPrecheckDatabaseConnection{}, err
	}
	if outcomeErr != nil {
		connection.Destroy()
		return EncryptedPrecheckDatabaseConnection{}, outcomeErr
	}
	return connection, nil
}

// FinishPrecheckSecretResolution 记录短时解密的安全结果，并在成功写出前重验当前绑定。
// 它不接收或保存任何明文；同一 requestId 的重试只会重新从密文取得材料，不会缓存先前响应。
func (s *Store) FinishPrecheckSecretResolution(ctx context.Context, input PrecheckSecretResolutionOutcome) error {
	if err := validatePrecheckSecretResolutionOutcome(input); err != nil {
		return err
	}
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		receipt, found, err := readPrecheckSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if !found || !samePrecheckSecretResolutionReceipt(receipt, PrecheckSecretResolutionRequest{
			AgentID: input.AgentID, PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, RequestID: input.RequestID, RequestDigest: input.RequestDigest,
		}) {
			return ErrPrecheckLeaseRejected
		}
		if receipt.Status == "RESOLVED" {
			if input.Succeeded {
				return nil
			}
			return ErrIdempotencyConflict
		}
		if !input.Succeeded {
			if receipt.Status == "FAILED" {
				return nil
			}
			if err := updatePrecheckSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "FAILED", input.Now); err != nil {
				return err
			}
			return insertPrecheckSecretResolutionAudit(ctx, tx, input.AgentID, input.PrecheckID, input.RequestID, "EXPORT_PRECHECK_SECRET_RESOLVE_FAILED", "FAILED", input.Now)
		}
		if _, err := expirePrechecksTx(ctx, tx, input.Now); err != nil {
			return err
		}
		run, err := readStoredPrecheck(ctx, tx, input.PrecheckID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" || run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest ||
			!run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if receipt.Status != "FAILED" {
				if err := updatePrecheckSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "FAILED", input.Now); err != nil {
					return err
				}
				if err := insertPrecheckSecretResolutionAudit(ctx, tx, input.AgentID, input.PrecheckID, input.RequestID, "EXPORT_PRECHECK_SECRET_RESOLVE_DENIED", "DENIED", input.Now); err != nil {
					return err
				}
			}
			if run.Status == "EXPIRED" || !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
				outcomeErr = ErrPrecheckLeaseExpired
			} else {
				outcomeErr = ErrPrecheckLeaseRejected
			}
			return nil
		}
		if err := validateCurrentPrecheckBinding(ctx, tx, input.AgentID, run.Binding); err != nil {
			if receipt.Status != "FAILED" {
				if updateErr := updatePrecheckSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "FAILED", input.Now); updateErr != nil {
					return updateErr
				}
				if auditErr := insertPrecheckSecretResolutionAudit(ctx, tx, input.AgentID, input.PrecheckID, input.RequestID, "EXPORT_PRECHECK_SECRET_RESOLVE_DENIED", "DENIED", input.Now); auditErr != nil {
					return auditErr
				}
			}
			outcomeErr = ErrPrecheckLeaseRejected
			return nil
		}
		acknowledged, err := hasPrecheckAcknowledgementReceipt(ctx, tx, input.AgentID, run)
		if err != nil {
			return err
		}
		if !acknowledged {
			return ErrPrecheckLeaseRejected
		}
		if err := updatePrecheckSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "RESOLVED", input.Now); err != nil {
			return err
		}
		return insertPrecheckSecretResolutionAudit(ctx, tx, input.AgentID, input.PrecheckID, input.RequestID, "EXPORT_PRECHECK_SECRET_RESOLVED", "SUCCEEDED", input.Now)
	})
	if err != nil {
		return err
	}
	return outcomeErr
}

// CompleteAgentPrecheck 只接受当前、未过期租约对应的完整受控检查结果。
// 超时会持久化为 EXPIRED 和 INCOMPLETE，不能被迟到结果覆盖。
func (s *Store) CompleteAgentPrecheck(ctx context.Context, input AgentPrecheckCompletion) (PrecheckCompletionResult, error) {
	if err := validateAgentPrecheckCompletion(input); err != nil {
		return PrecheckCompletionResult{}, err
	}
	resultJSON, succeeded, err := encodeAgentPrecheckResults(input.Results)
	if err != nil {
		return PrecheckCompletionResult{}, err
	}
	var result PrecheckCompletionResult
	var outcomeErr error
	err = s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expirePrechecksTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readAgentPrecheckReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		status := "FAILED"
		if succeeded {
			status = "SUCCEEDED"
		}
		if found {
			if receipt.Operation != "COMPLETE" || receipt.RequestDigest != input.RequestDigest || receipt.PrecheckID != input.PrecheckID ||
				receipt.LeaseID != input.LeaseID || receipt.LeaseEpoch != input.LeaseEpoch ||
				receipt.BindingDigest != input.BindingDigest || (receipt.Status != status && receipt.Status != "EXPIRED") {
				return ErrIdempotencyConflict
			}
			run, err := readStoredPrecheck(ctx, tx, receipt.PrecheckID)
			if err != nil {
				return err
			}
			if run.Binding.BindingDigest != receipt.BindingDigest {
				return ErrPrecheckLeaseRejected
			}
			integrityStatus := "COMPLETE"
			if receipt.Status == "EXPIRED" {
				integrityStatus, outcomeErr = "INCOMPLETE", ErrPrecheckLeaseExpired
			}
			result = PrecheckCompletionResult{
				PrecheckID: run.Binding.PrecheckID, LeaseID: receipt.LeaseID, LeaseEpoch: receipt.LeaseEpoch,
				BindingDigest: receipt.BindingDigest, Status: receipt.Status, IntegrityStatus: integrityStatus,
				CompletedAt: receipt.CompletedAt, Replayed: true,
			}
			return nil
		}

		run, err := readStoredPrecheck(ctx, tx, input.PrecheckID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" {
			var recordErr error
			result, recordErr = recordExpiredPrecheckCompletion(ctx, tx, input, run)
			if recordErr != nil {
				if errors.Is(recordErr, ErrPrecheckLeaseRejected) {
					outcomeErr = recordErr
					return nil
				}
				return recordErr
			}
			outcomeErr = ErrPrecheckLeaseExpired
			return nil
		}
		if run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest {
			return ErrPrecheckLeaseRejected
		}
		if !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if err := markPrecheckExpiredTx(ctx, tx, run.Binding.PrecheckID, input.Now); err != nil {
				return err
			}
			result, err = recordExpiredPrecheckCompletion(ctx, tx, input, run)
			if err != nil {
				if errors.Is(err, ErrPrecheckLeaseRejected) {
					outcomeErr = err
					return nil
				}
				return err
			}
			outcomeErr = ErrPrecheckLeaseExpired
			return nil
		}
		if err := validateCurrentPrecheckBinding(ctx, tx, input.AgentID, run.Binding); err != nil {
			return err
		}
		acknowledged, err := hasPrecheckAcknowledgementReceipt(ctx, tx, input.AgentID, run)
		if err != nil {
			return err
		}
		if !acknowledged {
			return ErrPrecheckLeaseRejected
		}
		updated, err := tx.ExecContext(ctx, `
            UPDATE precheck_runs
            SET status = ?, result_json = ?, integrity_status = 'COMPLETE', completed_at = ?
            WHERE precheck_id = ? AND status = 'LEASED' AND agent_id = ?
              AND lease_id = ? AND lease_epoch = ? AND binding_digest = ?
		`, status, resultJSON, utcText(input.Now), input.PrecheckID, input.AgentID,
			input.LeaseID, input.LeaseEpoch, input.BindingDigest)
		if err != nil {
			return fmt.Errorf("complete agent precheck: %w", err)
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("read agent precheck completion result: %w", err)
		}
		if affected != 1 {
			return ErrPrecheckLeaseRejected
		}
		receipt = agentPrecheckReceipt{
			AgentID: input.AgentID, RequestID: input.RequestID, Operation: "COMPLETE", RequestDigest: input.RequestDigest,
			PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: status, ExpiresAt: run.LeaseExpiresAt,
			CompletedAt: input.Now, CreatedAt: input.Now,
		}
		if err := insertAgentPrecheckReceipt(ctx, tx, receipt); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "EXPORT_PRECHECK_COMPLETED", "PRECHECK", input.PrecheckID, status, input.RequestID, input.Now); err != nil {
			return err
		}
		result = PrecheckCompletionResult{
			PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: status, IntegrityStatus: "COMPLETE", CompletedAt: input.Now,
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, outcomeErr
}

// ExpirePrechecks 把过期的待领取或已领取预检查固定为不可提交状态。
// 它绝不将记录重新放回 PENDING，也不会创建任务或再次分配租约。
func (s *Store) ExpirePrechecks(ctx context.Context, now time.Time) (int, error) {
	if now.IsZero() {
		return 0, errors.New("precheck expiry time is required")
	}
	var count int
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var err error
		count, err = expirePrechecksTx(ctx, tx, now)
		return err
	})
	return count, err
}

// storedPrecheck 只用于同一短事务中的租约判断，避免把内部租约事实暴露给浏览器读取模型。
type storedPrecheck struct {
	Binding         PrecheckBinding
	AgentID         string
	Status          string
	LeaseID         string
	LeaseEpoch      int64
	LeaseExpiresAt  time.Time
	IntegrityStatus string
	CompletedAt     time.Time
}

// agentPrecheckReceipt 仅保存 Agent 幂等重放所需的摘要和租约投影。
// 它不保存检查结果正文、路径、SQL、命令或任何秘密材料。
type agentPrecheckReceipt struct {
	AgentID       string
	RequestID     string
	Operation     string
	RequestDigest string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	Status        string
	ExpiresAt     time.Time
	CompletedAt   time.Time
	CreatedAt     time.Time
}

// precheckSecretResolutionReceipt 只保存秘密槽位请求的无秘密幂等投影。
// 控制面可在同一有效租约内重新从密文解密，但绝不将先前的明文响应写入该表。
type precheckSecretResolutionReceipt struct {
	AgentID       string
	RequestID     string
	RequestDigest string
	PrecheckID    string
	LeaseID       string
	LeaseEpoch    int64
	BindingDigest string
	Status        string
	CreatedAt     time.Time
	CompletedAt   time.Time
}

func readStoredPrecheck(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, precheckID string) (storedPrecheck, error) {
	var run storedPrecheck
	var bindingDigest, bindingAgentID, leaseID, agentID sql.NullString
	var leaseEpoch sql.NullInt64
	var validUntil, leaseExpiresAt, completedAt sql.NullString
	err := queryer.QueryRowContext(ctx, `
        SELECT precheck_id, draft_id, draft_revision, config_fingerprint, data_source_id,
               credential_id, credential_revision, node_id, node_facts_revision,
               binding_digest, binding_agent_id, agent_id, status, lease_id, lease_epoch, lease_expires_at,
               integrity_status, valid_until, completed_at
        FROM precheck_runs
        WHERE precheck_id = ?
    `, precheckID).Scan(
		&run.Binding.PrecheckID, &run.Binding.DraftID, &run.Binding.DraftRevision,
		&run.Binding.ConfigFingerprint, &run.Binding.DataSourceID, &run.Binding.CredentialID,
		&run.Binding.CredentialRevision, &run.Binding.NodeID, &run.Binding.NodeFactsRevision,
		&bindingDigest, &bindingAgentID, &agentID, &run.Status, &leaseID, &leaseEpoch, &leaseExpiresAt,
		&run.IntegrityStatus, &validUntil, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return storedPrecheck{}, ErrPrecheckLeaseRejected
	}
	if err != nil {
		return storedPrecheck{}, fmt.Errorf("read stored precheck: %w", err)
	}
	run.Binding.BindingDigest, run.Binding.BindingAgentID = bindingDigest.String, bindingAgentID.String
	run.AgentID, run.LeaseID, run.LeaseEpoch = agentID.String, leaseID.String, leaseEpoch.Int64
	var parseErr error
	if validUntil.Valid {
		run.Binding.ValidUntil, parseErr = time.Parse(time.RFC3339Nano, validUntil.String)
		if parseErr != nil {
			return storedPrecheck{}, fmt.Errorf("parse stored precheck validity: %w", parseErr)
		}
		run.Binding.ValidUntil = run.Binding.ValidUntil.UTC()
	}
	if leaseExpiresAt.Valid {
		run.LeaseExpiresAt, parseErr = time.Parse(time.RFC3339Nano, leaseExpiresAt.String)
		if parseErr != nil {
			return storedPrecheck{}, fmt.Errorf("parse stored precheck lease expiry: %w", parseErr)
		}
		run.LeaseExpiresAt = run.LeaseExpiresAt.UTC()
	}
	if completedAt.Valid {
		run.CompletedAt, parseErr = time.Parse(time.RFC3339Nano, completedAt.String)
		if parseErr != nil {
			return storedPrecheck{}, fmt.Errorf("parse stored precheck completion: %w", parseErr)
		}
		run.CompletedAt = run.CompletedAt.UTC()
	}
	return run, nil
}

func readAgentPrecheckReceipt(ctx context.Context, tx *sql.Tx, agentID, requestID string) (agentPrecheckReceipt, bool, error) {
	var receipt agentPrecheckReceipt
	var completedAt sql.NullString
	var expiresAt, createdAt string
	err := tx.QueryRowContext(ctx, `
        SELECT agent_id, request_id, operation, request_digest, precheck_id, lease_id,
               lease_epoch, binding_digest, status, expires_at, completed_at, created_at
        FROM agent_precheck_receipts
        WHERE agent_id = ? AND request_id = ?
    `, agentID, requestID).Scan(
		&receipt.AgentID, &receipt.RequestID, &receipt.Operation, &receipt.RequestDigest, &receipt.PrecheckID,
		&receipt.LeaseID, &receipt.LeaseEpoch, &receipt.BindingDigest, &receipt.Status, &expiresAt, &completedAt, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return agentPrecheckReceipt{}, false, nil
	}
	if err != nil {
		return agentPrecheckReceipt{}, false, fmt.Errorf("read agent precheck receipt: %w", err)
	}
	var parseErr error
	if receipt.ExpiresAt, parseErr = time.Parse(time.RFC3339Nano, expiresAt); parseErr != nil {
		return agentPrecheckReceipt{}, false, fmt.Errorf("parse agent precheck receipt expiry: %w", parseErr)
	}
	if receipt.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
		return agentPrecheckReceipt{}, false, fmt.Errorf("parse agent precheck receipt creation: %w", parseErr)
	}
	if completedAt.Valid {
		if receipt.CompletedAt, parseErr = time.Parse(time.RFC3339Nano, completedAt.String); parseErr != nil {
			return agentPrecheckReceipt{}, false, fmt.Errorf("parse agent precheck receipt completion: %w", parseErr)
		}
		receipt.CompletedAt = receipt.CompletedAt.UTC()
	}
	receipt.ExpiresAt, receipt.CreatedAt = receipt.ExpiresAt.UTC(), receipt.CreatedAt.UTC()
	return receipt, true, nil
}

func readPrecheckSecretResolutionReceipt(ctx context.Context, tx *sql.Tx, agentID, requestID string) (precheckSecretResolutionReceipt, bool, error) {
	var receipt precheckSecretResolutionReceipt
	var completedAt sql.NullString
	var createdAt string
	err := tx.QueryRowContext(ctx, `
        SELECT agent_id, request_id, request_digest, precheck_id, lease_id, lease_epoch,
               binding_digest, status, created_at, completed_at
        FROM agent_precheck_secret_resolution_receipts
        WHERE agent_id = ? AND request_id = ?
    `, agentID, requestID).Scan(
		&receipt.AgentID, &receipt.RequestID, &receipt.RequestDigest, &receipt.PrecheckID, &receipt.LeaseID,
		&receipt.LeaseEpoch, &receipt.BindingDigest, &receipt.Status, &createdAt, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return precheckSecretResolutionReceipt{}, false, nil
	}
	if err != nil {
		return precheckSecretResolutionReceipt{}, false, fmt.Errorf("read precheck secret resolution receipt: %w", err)
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return precheckSecretResolutionReceipt{}, false, fmt.Errorf("parse precheck secret resolution creation: %w", err)
	}
	receipt.CreatedAt = parsedCreatedAt.UTC()
	if completedAt.Valid {
		parsedCompletedAt, err := time.Parse(time.RFC3339Nano, completedAt.String)
		if err != nil {
			return precheckSecretResolutionReceipt{}, false, fmt.Errorf("parse precheck secret resolution completion: %w", err)
		}
		receipt.CompletedAt = parsedCompletedAt.UTC()
	}
	return receipt, true, nil
}

func samePrecheckSecretResolutionReceipt(receipt precheckSecretResolutionReceipt, input PrecheckSecretResolutionRequest) bool {
	return receipt.RequestDigest == input.RequestDigest && receipt.PrecheckID == input.PrecheckID &&
		receipt.LeaseID == input.LeaseID && receipt.LeaseEpoch == input.LeaseEpoch &&
		receipt.BindingDigest == input.BindingDigest
}

func insertPrecheckSecretResolutionReceipt(ctx context.Context, tx *sql.Tx, receipt precheckSecretResolutionReceipt) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO agent_precheck_secret_resolution_receipts(
            agent_id, request_id, request_digest, precheck_id, lease_id, lease_epoch,
            binding_digest, status, created_at, completed_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
    `, receipt.AgentID, receipt.RequestID, receipt.RequestDigest, receipt.PrecheckID, receipt.LeaseID,
		receipt.LeaseEpoch, receipt.BindingDigest, receipt.Status, utcText(receipt.CreatedAt))
	if err != nil {
		return fmt.Errorf("write precheck secret resolution receipt: %w", err)
	}
	return nil
}

func updatePrecheckSecretResolutionReceipt(ctx context.Context, tx *sql.Tx, agentID, requestID, status string, completedAt time.Time) error {
	updated, err := tx.ExecContext(ctx, `
        UPDATE agent_precheck_secret_resolution_receipts
        SET status = ?, completed_at = ?
        WHERE agent_id = ? AND request_id = ?
    `, status, utcText(completedAt), agentID, requestID)
	if err != nil {
		return fmt.Errorf("complete precheck secret resolution receipt: %w", err)
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("read precheck secret resolution update: %w", err)
	}
	if affected != 1 {
		return ErrPrecheckLeaseRejected
	}
	return nil
}

func readEncryptedPrecheckDatabaseConnection(ctx context.Context, tx *sql.Tx, binding PrecheckBinding) (EncryptedPrecheckDatabaseConnection, error) {
	var connection EncryptedPrecheckDatabaseConnection
	var connectionKind, username, tenantName, clusterName string
	err := tx.QueryRowContext(ctx, `
        SELECT ds.host, ds.port, ds.connection_kind, ds.username, ds.tenant_name, ds.cluster_name,
               cr.key_id, cr.nonce, cr.ciphertext,
               d.owner_subject_id, d.node_id
        FROM export_drafts AS d
        JOIN data_sources AS ds ON ds.data_source_id = d.data_source_id
        JOIN credential_revisions AS cr
          ON cr.credential_id = ds.credential_id
         AND cr.revision = ds.current_credential_revision
        WHERE d.draft_id = ? AND d.revision = ? AND d.config_fingerprint = ?
          AND d.data_source_id = ? AND d.node_id = ?
          AND ds.credential_id = ?
          AND ds.current_credential_revision = ? AND ds.state = 'ENABLED'
		  AND ds.last_test_status = 'SUCCEEDED' AND ds.last_test_source = 'AGENT_JDBC'
          AND cr.status = 'ACTIVE'
	`, binding.DraftID, binding.DraftRevision, binding.ConfigFingerprint, binding.DataSourceID, binding.NodeID,
		binding.CredentialID, binding.CredentialRevision).Scan(
		&connection.Host, &connection.Port, &connectionKind, &username, &tenantName, &clusterName,
		&connection.KeyID, &connection.Nonce, &connection.Ciphertext, &connection.OwnerSubjectID, &connection.NodeID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EncryptedPrecheckDatabaseConnection{}, ErrPrecheckLeaseRejected
	}
	if err != nil {
		connection.Destroy()
		return EncryptedPrecheckDatabaseConnection{}, fmt.Errorf("read encrypted precheck database connection: %w", err)
	}
	if connectionKind != "ODP" {
		connection.Destroy()
		return EncryptedPrecheckDatabaseConnection{}, ErrPrecheckLeaseRejected
	}
	var identityOK bool
	connection.Username, identityOK = composePrivateODPJDBCIdentity(username, tenantName, clusterName)
	if !identityOK {
		connection.Destroy()
		return EncryptedPrecheckDatabaseConnection{}, ErrPrecheckLeaseRejected
	}
	connection.DataSourceID, connection.CredentialID, connection.Revision = binding.DataSourceID, binding.CredentialID, binding.CredentialRevision
	if connection.Host == "" || connection.Port < 1 || connection.Port > 65535 || len(connection.Username) == 0 ||
		strings.TrimSpace(connection.OwnerSubjectID) == "" || connection.NodeID != binding.NodeID ||
		connection.KeyID == "" || len(connection.Nonce) == 0 || len(connection.Ciphertext) == 0 {
		connection.Destroy()
		return EncryptedPrecheckDatabaseConnection{}, ErrPrecheckLeaseRejected
	}
	return connection, nil
}

func hasPrecheckAcknowledgementReceipt(ctx context.Context, tx *sql.Tx, agentID string, run storedPrecheck) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM agent_precheck_receipts
        WHERE agent_id = ? AND operation = 'ACKNOWLEDGE' AND precheck_id = ?
          AND lease_id = ? AND lease_epoch = ? AND binding_digest = ? AND status = 'ACKNOWLEDGED'
    `, agentID, run.Binding.PrecheckID, run.LeaseID, run.LeaseEpoch, run.Binding.BindingDigest).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("read precheck acknowledgement receipt: %w", err)
	}
	return count == 1, nil
}

// recordExpiredPrecheckCompletion 为已确认但迟到的完成请求同事务保存回执和审计。
// 只有原 Agent、原租约和原绑定可以获得该结果，避免过期回执成为越权探测通道。
func recordExpiredPrecheckCompletion(ctx context.Context, tx *sql.Tx, input AgentPrecheckCompletion, run storedPrecheck) (PrecheckCompletionResult, error) {
	if run.AgentID != input.AgentID || run.LeaseID != input.LeaseID || run.LeaseEpoch != input.LeaseEpoch ||
		run.Binding.BindingDigest != input.BindingDigest || run.LeaseExpiresAt.IsZero() {
		return PrecheckCompletionResult{}, ErrPrecheckLeaseRejected
	}
	acknowledged, err := hasPrecheckAcknowledgementReceipt(ctx, tx, input.AgentID, run)
	if err != nil {
		return PrecheckCompletionResult{}, err
	}
	if !acknowledged {
		return PrecheckCompletionResult{}, ErrPrecheckLeaseRejected
	}
	receipt := agentPrecheckReceipt{
		AgentID: input.AgentID, RequestID: input.RequestID, Operation: "COMPLETE", RequestDigest: input.RequestDigest,
		PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
		BindingDigest: input.BindingDigest, Status: "EXPIRED", ExpiresAt: run.LeaseExpiresAt,
		CompletedAt: input.Now, CreatedAt: input.Now,
	}
	if err := insertAgentPrecheckReceipt(ctx, tx, receipt); err != nil {
		return PrecheckCompletionResult{}, err
	}
	if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "EXPORT_PRECHECK_COMPLETION_EXPIRED", "PRECHECK", input.PrecheckID, "FAILED", input.RequestID, input.Now); err != nil {
		return PrecheckCompletionResult{}, err
	}
	return PrecheckCompletionResult{
		PrecheckID: input.PrecheckID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
		BindingDigest: input.BindingDigest, Status: "EXPIRED", IntegrityStatus: "INCOMPLETE", CompletedAt: input.Now,
	}, nil
}

func insertAgentPrecheckReceipt(ctx context.Context, tx *sql.Tx, receipt agentPrecheckReceipt) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO agent_precheck_receipts(
            agent_id, request_id, operation, request_digest, precheck_id, lease_id,
            lease_epoch, binding_digest, status, expires_at, completed_at, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, receipt.AgentID, receipt.RequestID, receipt.Operation, receipt.RequestDigest, receipt.PrecheckID,
		receipt.LeaseID, receipt.LeaseEpoch, receipt.BindingDigest, receipt.Status,
		utcText(receipt.ExpiresAt), nullableTimeText(receipt.CompletedAt), utcText(receipt.CreatedAt))
	if err != nil {
		return fmt.Errorf("write agent precheck receipt: %w", err)
	}
	return nil
}

func replayPrecheckLeaseGrant(ctx context.Context, tx *sql.Tx, receipt agentPrecheckReceipt) (PrecheckLeaseGrant, error) {
	run, err := readStoredPrecheck(ctx, tx, receipt.PrecheckID)
	if err != nil {
		return PrecheckLeaseGrant{}, err
	}
	if run.Binding.BindingDigest != receipt.BindingDigest {
		return PrecheckLeaseGrant{}, ErrPrecheckLeaseRejected
	}
	executionContext, err := readPrecheckExecutionContext(ctx, tx, run.Binding)
	if err != nil {
		return PrecheckLeaseGrant{}, err
	}
	return PrecheckLeaseGrant{
		PrecheckID: receipt.PrecheckID, LeaseID: receipt.LeaseID, LeaseEpoch: receipt.LeaseEpoch,
		ExpiresAt: receipt.ExpiresAt, Binding: run.Binding, ExecutionContext: executionContext, Replayed: true,
	}, nil
}

func validateCurrentPrecheckBinding(ctx context.Context, tx *sql.Tx, agentID string, binding PrecheckBinding) error {
	if err := validatePrecheckBinding(binding); err != nil {
		return ErrPrecheckLeaseRejected
	}
	var factsRevision int64
	err := tx.QueryRowContext(ctx, `
        SELECT a.facts_revision
        FROM agents AS a
        JOIN execution_nodes AS n ON n.node_id = a.node_id
        WHERE a.agent_id = ? AND a.node_id = ? AND a.status = 'ACTIVE'
          AND n.management_state = 'ENABLED'
    `, agentID, binding.NodeID).Scan(&factsRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPrecheckLeaseRejected
	}
	if err != nil {
		return fmt.Errorf("read current precheck agent facts: %w", err)
	}
	if agentID != binding.BindingAgentID || factsRevision != binding.NodeFactsRevision {
		return ErrPrecheckLeaseRejected
	}
	var draftRevision, credentialRevision int64
	var configFingerprint, dataSourceID, draftNodeID, credentialID, dataSourceState, lastTestStatus, lastTestSource, credentialStatus, nodeState string
	err = tx.QueryRowContext(ctx, `
        SELECT d.revision, d.config_fingerprint, d.data_source_id, d.node_id,
               ds.credential_id, ds.current_credential_revision, ds.state,
               COALESCE(ds.last_test_status, ''), COALESCE(ds.last_test_source, ''),
               cr.status, n.management_state
        FROM export_drafts AS d
        JOIN data_sources AS ds ON ds.data_source_id = d.data_source_id
        JOIN credential_revisions AS cr
          ON cr.credential_id = ds.credential_id
         AND cr.revision = ds.current_credential_revision
        JOIN execution_nodes AS n ON n.node_id = d.node_id
        WHERE d.draft_id = ?
    `, binding.DraftID).Scan(
		&draftRevision, &configFingerprint, &dataSourceID, &draftNodeID,
		&credentialID, &credentialRevision, &dataSourceState,
		&lastTestStatus, &lastTestSource, &credentialStatus, &nodeState,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPrecheckLeaseRejected
	}
	if err != nil {
		return fmt.Errorf("read current precheck data source binding: %w", err)
	}
	if draftRevision != binding.DraftRevision || configFingerprint != binding.ConfigFingerprint ||
		dataSourceID != binding.DataSourceID || draftNodeID != binding.NodeID ||
		credentialID != binding.CredentialID || credentialRevision != binding.CredentialRevision ||
		dataSourceState != "ENABLED" || lastTestStatus != "SUCCEEDED" || lastTestSource != "AGENT_JDBC" ||
		credentialStatus != "ACTIVE" || nodeState != "ENABLED" {
		return ErrPrecheckLeaseRejected
	}
	expectedDigest, err := precheckBindingDigest(binding)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(expectedDigest), []byte(binding.BindingDigest)) != 1 {
		return ErrPrecheckLeaseRejected
	}
	return nil
}

// readPrecheckExecutionContext 只从仍与冻结绑定一致的草稿和节点读取固定本地检查输入。
// Agent 不会从浏览器、命令行或秘密槽位请求中获得这些字段，因此无法借此扩展检查范围。
func readPrecheckExecutionContext(ctx context.Context, tx *sql.Tx, binding PrecheckBinding) (PrecheckExecutionContext, error) {
	var configJSON, compatibilityMode, platform, allowedRootsJSON string
	err := tx.QueryRowContext(ctx, `
        SELECT d.config_json, s.compatibility_mode, n.platform, n.allowed_roots_json
        FROM export_drafts AS d
        JOIN data_sources AS s ON s.data_source_id = d.data_source_id
        JOIN execution_nodes AS n ON n.node_id = d.node_id
        WHERE d.draft_id = ? AND d.revision = ? AND d.config_fingerprint = ?
          AND d.data_source_id = ? AND d.node_id = ? AND n.management_state = 'ENABLED'
    `, binding.DraftID, binding.DraftRevision, binding.ConfigFingerprint, binding.DataSourceID, binding.NodeID).Scan(
		&configJSON, &compatibilityMode, &platform, &allowedRootsJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	if err != nil {
		return PrecheckExecutionContext{}, fmt.Errorf("read precheck execution context: %w", err)
	}
	var config struct {
		Database     string `json:"database"`
		Table        string `json:"table"`
		Format       string `json:"format"`
		FilePath     string `json:"filePath"`
		LogPath      string `json:"logPath"`
		SkipCheckDir bool   `json:"skipCheckDir"`
	}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil || (compatibilityMode != "MYSQL" && compatibilityMode != "ORACLE") || config.Format != "CSV" ||
		!validPrecheckObjectName(config.Database) || !validPrecheckObjectName(config.Table) ||
		!validPrecheckOutputPath(platform, config.FilePath) || (config.LogPath != "" && !validPrecheckOutputPath(platform, config.LogPath)) {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	var allowedRoots []string
	if err := json.Unmarshal([]byte(allowedRootsJSON), &allowedRoots); err != nil || !ValidateExecutionNodeConfiguration(platform, allowedRoots) {
		return PrecheckExecutionContext{}, ErrPrecheckLeaseRejected
	}
	return PrecheckExecutionContext{
		CompatibilityMode: compatibilityMode,
		Database:          config.Database,
		Table:             config.Table,
		OutputPath:        config.FilePath,
		LogPath:           config.LogPath,
		SkipCheckDir:      config.SkipCheckDir,
		TargetPlatform:    platform,
		AllowedRoots:      append([]string(nil), allowedRoots...),
	}, nil
}

func validPrecheckObjectName(value string) bool {
	return len(value) > 0 && len(value) <= 256 && value == strings.TrimSpace(value) && !strings.ContainsRune(value, '\x00') && !strings.ContainsAny(value, "\r\n")
}

func validPrecheckOutputPath(platform, value string) bool {
	if len(value) == 0 || len(value) > 4096 || value != strings.TrimSpace(value) || strings.ContainsRune(value, '\x00') || strings.ContainsAny(value, "\r\n") {
		return false
	}
	if platform == "WINDOWS_AMD64" {
		return outputpath.IsExportOutputPath(platform, value)
	}
	return (platform == "LINUX_AMD64" || platform == "LINUX_ARM64") && strings.HasPrefix(value, "/")
}

// ensurePrecheckAgentIdle 阻止单容量 Agent 在持有正式执行、预检查或连接测试租约时继续领取。
// 三类工作都会短时占用同一受控容量，不能借由并行检查或连接测试绕过执行租约的排他边界。
func ensurePrecheckAgentIdle(ctx context.Context, tx *sql.Tx, agentID string) error {
	var activeLeaseCount int
	err := tx.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM execution_leases
        WHERE agent_id = ?
          AND status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE')
    `, agentID).Scan(&activeLeaseCount)
	if err != nil {
		return fmt.Errorf("read Agent execution capacity: %w", err)
	}
	if activeLeaseCount != 0 {
		return ErrPrecheckLeaseRejected
	}
	var activePrecheckCount int
	err = tx.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM precheck_runs
        WHERE agent_id = ?
          AND status = 'LEASED'
    `, agentID).Scan(&activePrecheckCount)
	if err != nil {
		return fmt.Errorf("read Agent precheck capacity: %w", err)
	}
	if activePrecheckCount != 0 {
		return ErrPrecheckLeaseRejected
	}
	var activeConnectionTestCount int
	err = tx.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM data_source_connection_test_runs
        WHERE binding_agent_id = ? AND status = 'LEASED'
    `, agentID).Scan(&activeConnectionTestCount)
	if err != nil {
		return fmt.Errorf("read Agent data source connection test capacity: %w", err)
	}
	if activeConnectionTestCount != 0 {
		return ErrPrecheckLeaseRejected
	}
	return nil
}

// expireExecutionLeasesTx 用控制面时钟收口失效的正式执行租约。
// 租约一旦过期，Agent 不能再补写进程或结果事实；任务会失败并标记需要人工核对，避免陈旧 ISSUED 租约永久占满单容量节点。
func expireExecutionLeasesTx(ctx context.Context, tx *sql.Tx, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `
        UPDATE execution_leases
        SET status = 'EXPIRED', released_at = COALESCE(released_at, ?)
        WHERE status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE')
          AND expires_at <= ?
        RETURNING execution_id
    `, utcText(now), utcText(now))
	if err != nil {
		return fmt.Errorf("expire execution leases: %w", err)
	}
	defer rows.Close()
	executionIDs := make([]string, 0)
	for rows.Next() {
		var executionID string
		if err := rows.Scan(&executionID); err != nil {
			return fmt.Errorf("read expired execution lease: %w", err)
		}
		executionIDs = append(executionIDs, executionID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate expired execution leases: %w", err)
	}
	for _, executionID := range executionIDs {
		updated, err := tx.ExecContext(ctx, `
            UPDATE task_executions
            SET state = 'FAILED', reconciliation_required = 1, revision = revision + 1,
                finished_at = COALESCE(finished_at, ?), updated_at = ?
            WHERE execution_id = ? AND state IN ('STARTING', 'RUNNING', 'CANCELLING')
        `, utcText(now), utcText(now), executionID)
		if err != nil {
			return fmt.Errorf("fail expired execution: %w", err)
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("read expired execution update: %w", err)
		}
		if count == 1 {
			if err := insertAudit(ctx, tx, "SYSTEM", "control-plane", "TASK_EXECUTION_LEASE_EXPIRED", "TASK_EXECUTION", executionID, "FAILED", "execution-lease-expiry-"+executionID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// markAgentRestartReconciliationRequiredTx 将仍由旧 Agent 进程持有的活跃执行标为待核对。
// 启动标识变化只能证明旧监管进程不再连续，不能证明工具已经结束，因此不得在这里伪造终态、释放租约或重新派发任务。
func markAgentRestartReconciliationRequiredTx(ctx context.Context, tx *sql.Tx, agentID string, occurredAt time.Time, requestID string) error {
	rows, err := tx.QueryContext(ctx, `
        UPDATE task_executions AS execution
        SET reconciliation_required = 1, revision = revision + 1, updated_at = ?
        WHERE execution.agent_id = ?
          AND execution.state IN ('STARTING', 'RUNNING', 'CANCELLING')
          AND execution.reconciliation_required = 0
          AND EXISTS (
              SELECT 1
              FROM execution_leases AS lease
              WHERE lease.execution_id = execution.execution_id
                AND lease.agent_id = ?
                AND lease.status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE')
          )
        RETURNING execution_id
    `, utcText(occurredAt), agentID, agentID)
	if err != nil {
		return fmt.Errorf("mark restarted agent executions for reconciliation: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var executionID string
		if err := rows.Scan(&executionID); err != nil {
			return fmt.Errorf("read restarted agent reconciliation execution: %w", err)
		}
		if err := insertAudit(ctx, tx, "AGENT", agentID, "TASK_EXECUTION_RECONCILIATION_REQUIRED", "TASK_EXECUTION", executionID, "SUCCEEDED", requestID+"-"+executionID, occurredAt); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate restarted agent reconciliation executions: %w", err)
	}
	return nil
}

// expirePrechecksTx 以单条写入取得 SQLite 写锁，并只为实际状态转换写入过期审计。
// 不能先读取再更新，否则两个 Store 的延迟读事务会在升级写锁时相互冲突。
func expirePrechecksTx(ctx context.Context, tx *sql.Tx, now time.Time) (int, error) {
	rows, err := tx.QueryContext(ctx, `
        UPDATE precheck_runs
        SET status = 'EXPIRED', integrity_status = 'INCOMPLETE',
            completed_at = COALESCE(completed_at, ?)
        WHERE status IN ('PENDING', 'LEASED')
          AND (
              valid_until IS NULL OR valid_until <= ?
              OR (status = 'LEASED' AND (lease_expires_at IS NULL OR lease_expires_at <= ?))
          )
        RETURNING precheck_id
    `, utcText(now), utcText(now), utcText(now))
	if err != nil {
		return 0, fmt.Errorf("expire prechecks: %w", err)
	}
	var precheckIDs []string
	for rows.Next() {
		var precheckID string
		if err := rows.Scan(&precheckID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan expired precheck result: %w", err)
		}
		precheckIDs = append(precheckIDs, precheckID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate expired precheck results: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close expired prechecks: %w", err)
	}
	for _, precheckID := range precheckIDs {
		if err := insertPrecheckExpiryAudit(ctx, tx, precheckID, now); err != nil {
			return 0, err
		}
	}
	return len(precheckIDs), nil
}

func markPrecheckExpiredTx(ctx context.Context, tx *sql.Tx, precheckID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
        UPDATE precheck_runs
        SET status = 'EXPIRED', integrity_status = 'INCOMPLETE',
            completed_at = COALESCE(completed_at, ?)
        WHERE precheck_id = ? AND status IN ('PENDING', 'LEASED')
    `, utcText(now), precheckID)
	if err != nil {
		return fmt.Errorf("mark precheck expired: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read marked precheck expiry result: %w", err)
	}
	if count == 1 {
		return insertPrecheckExpiryAudit(ctx, tx, precheckID, now)
	}
	return nil
}

// insertPrecheckExpiryAudit 将控制面时钟导致的过期作为失败关闭事实持久化。
// 预检查只能从 PENDING 或 LEASED 转入 EXPIRED 一次，因此稳定请求标识不会覆盖其他审计记录。
func insertPrecheckExpiryAudit(ctx context.Context, tx *sql.Tx, precheckID string, now time.Time) error {
	return insertAudit(ctx, tx, "SYSTEM", "control-plane", "EXPORT_PRECHECK_EXPIRED", "PRECHECK", precheckID, "FAILED", "precheck-expiry-"+precheckID, now)
}

func precheckBindingDigest(binding PrecheckBinding) (string, error) {
	payload, err := json.Marshal(struct {
		BindingAgentID     string `json:"bindingAgentId"`
		PrecheckID         string `json:"precheckId"`
		DraftID            string `json:"draftId"`
		DraftRevision      int64  `json:"draftRevision"`
		ConfigFingerprint  string `json:"configFingerprint"`
		DataSourceID       string `json:"dataSourceId"`
		CredentialID       string `json:"credentialId"`
		CredentialRevision int64  `json:"credentialRevision"`
		NodeID             string `json:"nodeId"`
		NodeFactsRevision  int64  `json:"nodeFactsRevision"`
		ValidUntil         string `json:"validUntil"`
	}{
		BindingAgentID: binding.BindingAgentID, PrecheckID: binding.PrecheckID, DraftID: binding.DraftID,
		DraftRevision: binding.DraftRevision, ConfigFingerprint: binding.ConfigFingerprint,
		DataSourceID: binding.DataSourceID, CredentialID: binding.CredentialID,
		CredentialRevision: binding.CredentialRevision, NodeID: binding.NodeID,
		NodeFactsRevision: binding.NodeFactsRevision, ValidUntil: utcText(binding.ValidUntil),
	})
	if err != nil {
		return "", fmt.Errorf("encode precheck binding digest: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// encodeAgentPrecheckResults 固化固定检查清单的顺序和证据码，避免存储层接受任意 JSON 结果。
func encodeAgentPrecheckResults(results []PrecheckCheckResult) (string, bool, error) {
	payload, err := json.Marshal(struct {
		Checks []PrecheckCheckResult `json:"checks"`
	}{Checks: results})
	if err != nil {
		return "", false, fmt.Errorf("encode agent precheck results: %w", err)
	}
	succeeded := true
	for _, result := range results {
		if result.Status != "PASSED" {
			succeeded = false
			break
		}
	}
	return string(payload), succeeded, nil
}

// decodePrecheckResults 仅从已持久化的固定检查 JSON 还原安全结果投影。
// 旧合成记录、损坏 JSON 或任意非固定检查都只返回空结果，不能把未校验内容暴露给浏览器。
func decodePrecheckResults(value string) []PrecheckCheckResult {
	var payload struct {
		Checks []PrecheckCheckResult `json:"checks"`
	}
	if value == "" || json.Unmarshal([]byte(value), &payload) != nil || len(payload.Checks) != len(fixedPrecheckChecks) {
		return nil
	}
	for index, expected := range fixedPrecheckChecks {
		result := payload.Checks[index]
		if result.Check != expected || !precheckcontract.ValidResult(result.Check, result.Status, result.EvidenceCode) {
			return nil
		}
	}
	return append([]PrecheckCheckResult(nil), payload.Checks...)
}

func nullableTimeText(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return utcText(value)
}

func (s *Store) initialize(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("SQLite store is nil")
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping SQLite database: %w", err)
	}
	if err := migrate.Apply(ctx, s.db); err != nil {
		return fmt.Errorf("initialize SQLite schema: %w", err)
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("read SQLite foreign key setting: %w", err)
	}
	if foreignKeys != 1 {
		return errors.New("SQLite foreign_keys must remain enabled")
	}
	return nil
}

func (s *Store) UpdateDraft(ctx context.Context, input DraftUpdate) (int64, error) {
	if err := validateDraftUpdate(input); err != nil {
		return 0, err
	}
	var revision int64
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
            UPDATE export_drafts
            SET revision = revision + 1,
                config_json = ?,
                config_fingerprint = ?,
                invalidation_json = ?,
                updated_at = ?
            WHERE draft_id = ? AND revision = ?
        `, input.ConfigJSON, input.ConfigFingerprint, input.InvalidationJSON, utcText(input.UpdatedAt), input.DraftID, input.ExpectedRevision)
		if err != nil {
			return fmt.Errorf("update export draft: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read export draft update result: %w", err)
		}
		if affected != 1 {
			return ErrRevisionConflict
		}
		revision = input.ExpectedRevision + 1
		return nil
	})
	return revision, err
}

func (s *Store) SubmitTask(ctx context.Context, input TaskSubmission) error {
	if err := validateTaskSubmission(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error { return s.submitTaskTx(ctx, tx, input) })
}

// SubmitTaskIdempotent 在同一事务中写入不可变任务、审计和幂等结果。
func (s *Store) SubmitTaskIdempotent(ctx context.Context, input TaskSubmission, idempotencyKey, requestDigest string) (TaskSubmissionResult, error) {
	if err := validateTaskSubmission(input); err != nil {
		return TaskSubmissionResult{}, err
	}
	if len(idempotencyKey) < 16 || !isSHA256(requestDigest) {
		return TaskSubmissionResult{}, errors.New("task submission idempotency is invalid")
	}
	result := TaskSubmissionResult{TaskID: input.TaskID}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var digest, existingID string
		err := tx.QueryRowContext(ctx, `SELECT request_digest, COALESCE(resource_id, '') FROM request_idempotency WHERE subject_id = ? AND operation = 'SUBMIT_EXPORT_DRAFT' AND idempotency_key = ?`, input.CreatorSubjectID, idempotencyKey).Scan(&digest, &existingID)
		if err == nil {
			if digest != requestDigest {
				return ErrIdempotencyConflict
			}
			result.TaskID, result.Replayed = existingID, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read task submission idempotency: %w", err)
		}
		if err := s.submitTaskTx(ctx, tx, input); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO request_idempotency(subject_id, operation, idempotency_key, request_digest, result_status, resource_kind, resource_id, response_json, created_at, expires_at) VALUES (?, 'SUBMIT_EXPORT_DRAFT', ?, ?, 201, 'TASK', ?, '{}', ?, ?)`, input.CreatorSubjectID, idempotencyKey, requestDigest, input.TaskID, utcText(input.SubmittedAt), utcText(input.SubmittedAt.Add(24*time.Hour))); err != nil {
			return fmt.Errorf("write task submission idempotency: %w", err)
		}
		return nil
	})
	return result, err
}

// submitTaskTx 保持原有任务冻结条件，供普通和幂等提交共用。
func (s *Store) submitTaskTx(ctx context.Context, tx *sql.Tx, input TaskSubmission) error {
	precheck, err := readStoredPrecheck(ctx, tx, input.PrecheckID)
	if errors.Is(err, ErrPrecheckLeaseRejected) {
		return ErrPrecheckInvalid
	}
	if err != nil {
		return err
	}
	if err := validateCurrentPrecheckBinding(ctx, tx, precheck.Binding.BindingAgentID, precheck.Binding); err != nil {
		if errors.Is(err, ErrPrecheckLeaseRejected) {
			return ErrPrecheckInvalid
		}
		return err
	}
	result, err := tx.ExecContext(ctx, `
            INSERT INTO tasks(
                task_id, creator_subject_id, data_source_id, node_id, precheck_id,
                credential_id, credential_revision, config_fingerprint, tool_version,
                metadata_version, capability_version, snapshot_json, planned_argv_json,
                planned_command_redacted, submitted_at
            )
            SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
            WHERE EXISTS (
                SELECT 1
                FROM precheck_runs
                WHERE precheck_id = ?
                  AND status = 'SUCCEEDED'
                  AND integrity_status = 'COMPLETE'
                  AND data_source_id = ?
                  AND node_id = ?
                  AND credential_id = ?
                  AND credential_revision = ?
                  AND config_fingerprint = ?
                  AND valid_until >= ?
			  AND node_facts_revision > 0
			  AND binding_agent_id IS NOT NULL
			  AND binding_digest IS NOT NULL
			  AND length(binding_digest) = 64
			  AND binding_digest NOT GLOB '*[^0-9a-f]*'
			  AND EXISTS (
			      SELECT 1
			      FROM agents AS binding_agent
			      JOIN execution_nodes AS binding_node ON binding_node.node_id = binding_agent.node_id
			      WHERE binding_agent.agent_id = precheck_runs.binding_agent_id
			        AND binding_agent.node_id = precheck_runs.node_id
			        AND binding_agent.status = 'ACTIVE'
			        AND binding_agent.facts_revision = precheck_runs.node_facts_revision
			        AND binding_node.management_state = 'ENABLED'
			  )
            )
        `,
		input.TaskID, input.CreatorSubjectID, input.DataSourceID, input.NodeID, input.PrecheckID,
		input.CredentialID, input.CredentialRevision, input.ConfigFingerprint, input.ToolVersion,
		input.MetadataVersion, input.CapabilityVersion, input.SnapshotJSON, input.PlannedArgvJSON,
		input.PlannedCommandRedacted, utcText(input.SubmittedAt),
		input.PrecheckID, input.DataSourceID, input.NodeID, input.CredentialID,
		input.CredentialRevision, input.ConfigFingerprint, utcText(input.SubmittedAt),
	)
	if err != nil {
		return fmt.Errorf("insert immutable task: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read task submission result: %w", err)
	}
	if affected != 1 {
		return ErrPrecheckInvalid
	}
	if err := insertAudit(ctx, tx, "SUBJECT", input.AuditActorID, "TASK_SUBMITTED", "TASK", input.TaskID, "SUCCEEDED", input.RequestID, input.SubmittedAt); err != nil {
		return err
	}
	return nil
}

// GetTaskSummary 返回任务的冻结非敏感投影；控制面必须在调用后校验创建者范围。
func (s *Store) GetTaskSummary(ctx context.Context, taskID string) (TaskSummary, error) {
	if s == nil || s.db == nil {
		return TaskSummary{}, errors.New("SQLite store is nil")
	}
	var summary TaskSummary
	var submittedAt, startedAt, finishedAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT t.task_id, t.creator_subject_id, t.data_source_id, t.node_id, t.precheck_id,
		       t.config_fingerprint, t.tool_version, t.metadata_version, t.capability_version,
		       COALESCE(json_extract(t.snapshot_json, '$.database'), ''),
		       COALESCE(json_extract(t.snapshot_json, '$.table'), ''),
		       COALESCE(json_extract(t.snapshot_json, '$.format'), ''),
		       t.planned_command_redacted, COALESCE(e.state, 'WAITING_SCHEDULE'),
		       COALESCE(e.execution_id, ''), COALESCE(e.reconciliation_required, 0),
		       t.submitted_at, COALESCE(e.started_at, ''), COALESCE(e.finished_at, ''),
		       COALESCE(e.updated_at, t.submitted_at)
        FROM tasks t
        LEFT JOIN task_executions e ON e.task_id = t.task_id
        WHERE t.task_id = ?
    `, taskID).Scan(&summary.TaskID, &summary.CreatorSubjectID, &summary.DataSourceID, &summary.NodeID,
		&summary.PrecheckID, &summary.ConfigFingerprint, &summary.ToolVersion, &summary.MetadataVersion,
		&summary.CapabilityVersion, &summary.Database, &summary.Table, &summary.Format,
		&summary.PlannedCommandRedacted, &summary.State, &summary.ExecutionID, &summary.ReconciliationRequired,
		&submittedAt, &startedAt, &finishedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskSummary{}, ErrDataSourceNotFound
	}
	if err != nil {
		return TaskSummary{}, fmt.Errorf("read task summary: %w", err)
	}
	if summary.SubmittedAt, err = parseTaskListTime(submittedAt, true); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task submission time: %w", err)
	}
	if summary.StartedAt, err = parseTaskListTime(startedAt, false); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task start time: %w", err)
	}
	if summary.FinishedAt, err = parseTaskListTime(finishedAt, false); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task finish time: %w", err)
	}
	if summary.UpdatedAt, err = parseTaskListTime(updatedAt, true); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task update time: %w", err)
	}
	return summary, nil
}

// GetAuthorizedTaskSummary 只在当前主体拥有任务或具备对应数据源任务范围时返回详情投影。
// 无权对象与不存在对象使用同一错误，避免调用方泄露任务存在性。
func (s *Store) GetAuthorizedTaskSummary(ctx context.Context, taskID, subjectID string) (TaskSummary, error) {
	if s == nil || s.db == nil {
		return TaskSummary{}, errors.New("SQLite store is nil")
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(subjectID) == "" {
		return TaskSummary{}, ErrDataSourceNotFound
	}
	var summary TaskSummary
	var submittedAt, startedAt, finishedAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT t.task_id, t.creator_subject_id, t.data_source_id, t.node_id, t.precheck_id,
		       t.config_fingerprint, t.tool_version, t.metadata_version, t.capability_version,
		       COALESCE(json_extract(t.snapshot_json, '$.database'), ''),
		       COALESCE(json_extract(t.snapshot_json, '$.table'), ''),
		       COALESCE(json_extract(t.snapshot_json, '$.format'), ''),
		       t.planned_command_redacted, COALESCE(e.state, 'WAITING_SCHEDULE'),
		       COALESCE(e.execution_id, ''), COALESCE(e.reconciliation_required, 0),
		       t.submitted_at, COALESCE(e.started_at, ''), COALESCE(e.finished_at, ''),
		       COALESCE(e.updated_at, t.submitted_at)
        FROM tasks t
        LEFT JOIN task_executions e ON e.task_id = t.task_id
        WHERE t.task_id = ?
          AND EXISTS (
              SELECT 1 FROM auth_subjects subject
              WHERE subject.subject_id = ? AND subject.account_status = 'ACTIVE'
          )
          AND (
              t.creator_subject_id = ?
              OR EXISTS (
                  SELECT 1 FROM subject_object_scopes task_scope
                  WHERE task_scope.subject_id = ?
                    AND task_scope.scope_type = 'TASK_OPERATE_BY_DATA_SOURCE'
                    AND task_scope.object_id = t.data_source_id
              )
          )
    `, taskID, subjectID, subjectID, subjectID).Scan(&summary.TaskID, &summary.CreatorSubjectID, &summary.DataSourceID, &summary.NodeID,
		&summary.PrecheckID, &summary.ConfigFingerprint, &summary.ToolVersion, &summary.MetadataVersion,
		&summary.CapabilityVersion, &summary.Database, &summary.Table, &summary.Format,
		&summary.PlannedCommandRedacted, &summary.State, &summary.ExecutionID, &summary.ReconciliationRequired,
		&submittedAt, &startedAt, &finishedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskSummary{}, ErrDataSourceNotFound
	}
	if err != nil {
		return TaskSummary{}, fmt.Errorf("read authorized task summary: %w", err)
	}
	if summary.SubmittedAt, err = parseTaskListTime(submittedAt, true); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task submission time: %w", err)
	}
	if summary.StartedAt, err = parseTaskListTime(startedAt, false); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task start time: %w", err)
	}
	if summary.FinishedAt, err = parseTaskListTime(finishedAt, false); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task finish time: %w", err)
	}
	if summary.UpdatedAt, err = parseTaskListTime(updatedAt, true); err != nil {
		return TaskSummary{}, fmt.Errorf("parse task update time: %w", err)
	}
	return summary, nil
}

// CountAuthorizedTaskSummaries 只统计当前主体在任务列表可读取范围内的任务。
// 调用方只能据此派生授权范围的页数，不能将原始总数作为全局任务数量返回。
func (s *Store) CountAuthorizedTaskSummaries(ctx context.Context, subjectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("SQLite store is nil")
	}
	if strings.TrimSpace(subjectID) == "" {
		return 0, errors.New("task count subject is invalid")
	}
	var total int
	err := s.db.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM tasks t
        WHERE EXISTS (
              SELECT 1 FROM auth_subjects subject
              WHERE subject.subject_id = ? AND subject.account_status = 'ACTIVE'
          )
          AND (
              t.creator_subject_id = ?
              OR EXISTS (
                  SELECT 1 FROM subject_object_scopes task_scope
                  WHERE task_scope.subject_id = ?
                    AND task_scope.scope_type = 'TASK_OPERATE_BY_DATA_SOURCE'
                    AND task_scope.object_id = t.data_source_id
              )
          )
    `, subjectID, subjectID, subjectID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count authorized tasks: %w", err)
	}
	return total, nil
}

// ListTaskSummaries 在 SQLite 查询中裁剪授权范围，并按提交时间和任务 ID 倒序返回。
// 返回值最多为 Limit+1 条，供 HTTP 层生成主体绑定的下一页游标。
func (s *Store) ListTaskSummaries(ctx context.Context, input TaskListQuery) ([]TaskListItem, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("SQLite store is nil")
	}
	if strings.TrimSpace(input.SubjectID) == "" || input.Limit < 1 || input.Limit > 100 ||
		(input.BeforeSubmitted.IsZero() != (strings.TrimSpace(input.BeforeTaskID) == "")) {
		return nil, errors.New("task list query is invalid")
	}
	before := ""
	if !input.BeforeSubmitted.IsZero() {
		before = utcText(input.BeforeSubmitted)
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT t.task_id, t.creator_subject_id, t.data_source_id, t.node_id,
               'OBDUMPER_EXPORT',
               COALESCE(json_extract(t.snapshot_json, '$.database'), ''),
               COALESCE(json_extract(t.snapshot_json, '$.table'), ''),
               COALESCE(e.state, 'WAITING_SCHEDULE'), COALESCE(e.reconciliation_required, 0),
               t.submitted_at, COALESCE(e.started_at, ''), COALESCE(e.finished_at, ''),
               COALESCE(e.updated_at, t.submitted_at)
        FROM tasks t
        LEFT JOIN task_executions e ON e.task_id = t.task_id
        WHERE EXISTS (
              SELECT 1 FROM auth_subjects subject
              WHERE subject.subject_id = ? AND subject.account_status = 'ACTIVE'
          )
          AND (
              t.creator_subject_id = ?
              OR EXISTS (
                  SELECT 1 FROM subject_object_scopes task_scope
                  WHERE task_scope.subject_id = ?
                    AND task_scope.scope_type = 'TASK_OPERATE_BY_DATA_SOURCE'
                    AND task_scope.object_id = t.data_source_id
              )
          )
          AND (? = '' OR t.submitted_at < ? OR (t.submitted_at = ? AND t.task_id < ?))
        ORDER BY t.submitted_at DESC, t.task_id DESC
        LIMIT ?
    `, input.SubjectID, input.SubjectID, input.SubjectID, before, before, before, input.BeforeTaskID, input.Limit+1)
	if err != nil {
		return nil, fmt.Errorf("list authorized tasks: %w", err)
	}
	defer rows.Close()
	items := make([]TaskListItem, 0, input.Limit+1)
	for rows.Next() {
		var item TaskListItem
		var reconciliation int
		var submittedAt, startedAt, finishedAt, updatedAt string
		if err := rows.Scan(&item.TaskID, &item.CreatorSubjectID, &item.DataSourceID, &item.NodeID,
			&item.TaskType, &item.Database, &item.Table, &item.State, &reconciliation,
			&submittedAt, &startedAt, &finishedAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan authorized task: %w", err)
		}
		item.ReconciliationRequired = reconciliation == 1
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

func parseTaskListTime(value string, required bool) (time.Time, error) {
	if value == "" && !required {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func (s *Store) ClaimTask(ctx context.Context, input Claim) error {
	if err := validateClaim(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
            INSERT INTO task_executions(
                execution_id, task_id, node_id, agent_id, state, revision,
                reconciliation_required, process_evidence_json, result_summary_json,
                created_at, started_at, finished_at, updated_at
            )
            SELECT ?, task_id, node_id, ?, 'STARTING', 1, 0, NULL, NULL, ?, NULL, NULL, ?
            FROM tasks
            WHERE task_id = ?
              AND node_id = ?
              AND EXISTS (
                SELECT 1 FROM agents
                WHERE agent_id = ? AND node_id = tasks.node_id AND status = 'ACTIVE'
              )
        `, input.ExecutionID, input.AgentID, utcText(input.IssuedAt), utcText(input.IssuedAt), input.TaskID, input.NodeID, input.AgentID)
		if err != nil {
			if isTaskExecutionUnique(err) {
				return ErrAlreadyClaimed
			}
			return fmt.Errorf("create task execution: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read task claim result: %w", err)
		}
		if affected != 1 {
			return ErrClaimIneligible
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO execution_leases(
                lease_id, execution_id, lease_epoch, agent_id, status, issued_at,
                acknowledged_at, expires_at, released_at
            ) VALUES (?, ?, ?, ?, 'ISSUED', ?, NULL, ?, NULL)
        `, input.LeaseID, input.ExecutionID, input.LeaseEpoch, input.AgentID, utcText(input.IssuedAt), utcText(input.ExpiresAt)); err != nil {
			return fmt.Errorf("create execution lease: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO execution_events(
                event_id, execution_id, lease_id, lease_epoch, event_seq, event_type,
                payload_json, received_at, accepted, rejection_code
            ) VALUES (?, ?, ?, ?, 1, 'SCHEDULED', '{}', ?, 1, NULL)
        `, input.EventID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, utcText(input.IssuedAt)); err != nil {
			return fmt.Errorf("append initial execution event: %w", err)
		}
		return insertAudit(ctx, tx, "AGENT", input.AgentID, "TASK_CLAIMED", "TASK_EXECUTION", input.ExecutionID, "SUCCEEDED", input.RequestID, input.IssuedAt)
	})
}

// ClaimNextExecution 在单个 SQLite 短事务内选择当前 Agent 唯一可领取的已提交任务并创建租约。
// 任务、节点、数据源、凭据、预检查、环境事实和容量均由服务端重新核验；Agent 无法指定任务或参数。
func (s *Store) ClaimNextExecution(ctx context.Context, input ExecutionClaimNext) (ExecutionLeaseGrant, bool, error) {
	if err := validateExecutionClaimNext(input); err != nil {
		return ExecutionLeaseGrant{}, false, err
	}
	var grant ExecutionLeaseGrant
	var found bool
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if err := expireExecutionLeasesTx(ctx, tx, input.Now); err != nil {
			return err
		}
		if err := ensurePrecheckAgentIdle(ctx, tx, input.AgentID); err != nil {
			if errors.Is(err, ErrPrecheckLeaseRejected) {
				return nil
			}
			return err
		}
		var plannedArgvJSON string
		err := tx.QueryRowContext(ctx, `
            SELECT t.task_id, t.planned_argv_json, t.config_fingerprint, t.tool_version,
                   t.metadata_version, t.capability_version
            FROM tasks AS t
            JOIN precheck_runs AS p ON p.precheck_id = t.precheck_id
            JOIN data_sources AS ds ON ds.data_source_id = t.data_source_id
            JOIN credential_revisions AS cr
              ON cr.credential_id = t.credential_id AND cr.revision = t.credential_revision
            JOIN auth_subjects AS owner ON owner.subject_id = t.creator_subject_id
            JOIN execution_nodes AS n ON n.node_id = t.node_id
            JOIN agents AS a ON a.agent_id = ? AND a.node_id = t.node_id AND a.status = 'ACTIVE'
            WHERE t.node_id = ?
			  AND NOT EXISTS (SELECT 1 FROM task_executions AS existing WHERE existing.task_id = t.task_id)
			  AND n.management_state = 'ENABLED'
			  AND n.environment_check_status = 'PASSED' AND n.environment_check_code = 'TOOL_RUNTIME_READY'
			  AND n.environment_check_facts_revision = a.facts_revision
			  AND a.last_heartbeat_at IS NOT NULL AND a.last_heartbeat_at >= ?
              AND a.capacity_total = 1 AND a.capacity_used = 0
              AND p.status = 'SUCCEEDED' AND p.integrity_status = 'COMPLETE' AND p.valid_until > ?
              AND p.binding_agent_id = a.agent_id AND p.node_facts_revision = a.facts_revision
              AND ds.state = 'ENABLED' AND ds.last_test_status = 'SUCCEEDED' AND ds.last_test_source = 'AGENT_JDBC'
              AND cr.status = 'ACTIVE' AND owner.account_status = 'ACTIVE'
            ORDER BY t.submitted_at ASC
            LIMIT 1
        `, input.AgentID, input.NodeID, utcText(input.HeartbeatFreshAfter), utcText(input.Now)).Scan(
			&grant.TaskID, &plannedArgvJSON, &grant.ConfigFingerprint, &grant.ToolVersion,
			&grant.MetadataVersion, &grant.CapabilityVersion,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read next execution task: %w", err)
		}
		if err := json.Unmarshal([]byte(plannedArgvJSON), &grant.PlannedArgv); err != nil || !validExecutionArgv(grant.PlannedArgv) {
			return ErrClaimIneligible
		}
		digest, err := executionEnvelopeDigest(grant.TaskID, input.NodeID, grant.ConfigFingerprint, grant.ToolVersion, grant.MetadataVersion, grant.CapabilityVersion, grant.PlannedArgv)
		if err != nil {
			return err
		}
		grant.EnvelopeDigest = digest
		grant.ExecutionID = input.ExecutionID
		grant.LeaseID = input.LeaseID
		grant.LeaseEpoch = 1
		grant.ExpiresAt = input.Now.Add(input.LeaseTTL).UTC()
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO task_executions(
                execution_id, task_id, node_id, agent_id, state, revision,
                reconciliation_required, process_evidence_json, result_summary_json,
                created_at, started_at, finished_at, updated_at
            ) VALUES (?, ?, ?, ?, 'STARTING', 1, 0, NULL, NULL, ?, NULL, NULL, ?)
        `, grant.ExecutionID, grant.TaskID, input.NodeID, input.AgentID, utcText(input.Now), utcText(input.Now)); err != nil {
			if isTaskExecutionUnique(err) {
				return nil
			}
			return fmt.Errorf("create next task execution: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO execution_leases(
                lease_id, execution_id, lease_epoch, agent_id, status, issued_at,
                acknowledged_at, expires_at, released_at
            ) VALUES (?, ?, 1, ?, 'ISSUED', ?, NULL, ?, NULL)
        `, grant.LeaseID, grant.ExecutionID, input.AgentID, utcText(input.Now), utcText(grant.ExpiresAt)); err != nil {
			return fmt.Errorf("create next execution lease: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO execution_events(
                event_id, execution_id, lease_id, lease_epoch, event_seq, event_type,
                payload_json, received_at, accepted, rejection_code
            ) VALUES (?, ?, ?, 1, 1, 'SCHEDULED', '{}', ?, 1, NULL)
        `, input.ScheduledEventID, grant.ExecutionID, grant.LeaseID, utcText(input.Now)); err != nil {
			return fmt.Errorf("append initial execution event: %w", err)
		}
		if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "TASK_CLAIMED", "TASK_EXECUTION", grant.ExecutionID, "SUCCEEDED", input.RequestID, input.Now); err != nil {
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		return ExecutionLeaseGrant{}, false, err
	}
	if !found {
		return ExecutionLeaseGrant{}, false, nil
	}
	grant.PlannedArgv = append([]string(nil), grant.PlannedArgv...)
	return grant, true, nil
}

// ResolveExecutionDatabaseConnection 只在当前 execution 的有效租约内返回加密数据库材料。
// 明文解密不在 SQLite 事务内发生；调用方必须在写出 HTTPS 响应前调用 FinishExecutionSecretResolution。
func (s *Store) ResolveExecutionDatabaseConnection(ctx context.Context, input ExecutionSecretResolutionRequest) (EncryptedExecutionDatabaseConnection, error) {
	if err := validateExecutionSecretResolutionRequest(input); err != nil {
		return EncryptedExecutionDatabaseConnection{}, err
	}
	var connection EncryptedExecutionDatabaseConnection
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var receiptDigest string
		var receiptStatus string
		err := tx.QueryRowContext(ctx, `
            SELECT request_digest, status
            FROM execution_secret_resolution_receipts
            WHERE agent_id = ? AND request_id = ?
        `, input.AgentID, input.RequestID).Scan(&receiptDigest, &receiptStatus)
		if err == nil && receiptDigest != input.RequestDigest {
			return ErrIdempotencyConflict
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read execution secret receipt: %w", err)
		}
		var username, tenantName, clusterName, argvJSON string
		var leaseExpiresAt string
		err = tx.QueryRowContext(ctx, `
            SELECT ds.host, ds.port, ds.username, ds.tenant_name, ds.cluster_name,
                   t.data_source_id, t.creator_subject_id, e.node_id, t.credential_id, t.credential_revision,
                   cr.key_id, cr.nonce, cr.ciphertext, t.planned_argv_json, l.expires_at
            FROM task_executions AS e
            JOIN tasks AS t ON t.task_id = e.task_id
            JOIN execution_leases AS l
              ON l.execution_id = e.execution_id AND l.lease_id = ? AND l.lease_epoch = ?
            JOIN agents AS a ON a.agent_id = l.agent_id AND a.node_id = e.node_id AND a.status = 'ACTIVE'
            JOIN execution_nodes AS n ON n.node_id = e.node_id
            JOIN data_sources AS ds ON ds.data_source_id = t.data_source_id
            JOIN credential_revisions AS cr ON cr.credential_id = t.credential_id AND cr.revision = t.credential_revision
            JOIN auth_subjects AS owner ON owner.subject_id = t.creator_subject_id
            WHERE e.execution_id = ? AND e.agent_id = ? AND l.agent_id = ?
              AND l.status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE') AND l.expires_at > ?
              AND n.management_state = 'ENABLED' AND ds.state = 'ENABLED' AND cr.status = 'ACTIVE'
              AND owner.account_status = 'ACTIVE'
        `, input.LeaseID, input.LeaseEpoch, input.ExecutionID, input.AgentID, input.AgentID, utcText(input.Now)).Scan(
			&connection.Host, &connection.Port, &username, &tenantName, &clusterName,
			&connection.DataSourceID, &connection.OwnerSubjectID, &connection.NodeID, &connection.CredentialID, &connection.Revision,
			&connection.KeyID, &connection.Nonce, &connection.Ciphertext, &argvJSON, &leaseExpiresAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEventRejected
		}
		if err != nil {
			return fmt.Errorf("read execution database connection: %w", err)
		}
		var argv []string
		if err := json.Unmarshal([]byte(argvJSON), &argv); err != nil || !validExecutionArgv(argv) {
			return ErrEventRejected
		}
		digest, err := executionEnvelopeDigestForStoredTask(ctx, tx, input.ExecutionID, argv)
		if err != nil || subtle.ConstantTimeCompare([]byte(digest), []byte(input.EnvelopeDigest)) != 1 {
			return ErrEventRejected
		}
		if _, err := time.Parse(time.RFC3339Nano, leaseExpiresAt); err != nil {
			return ErrEventRejected
		}
		connection.Username, _ = composePrivateODPJDBCIdentity(username, tenantName, clusterName)
		if len(connection.Username) == 0 {
			return ErrEventRejected
		}
		if errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if receiptStatus == "" {
			if _, err := tx.ExecContext(ctx, `
                INSERT INTO execution_secret_resolution_receipts(
                    agent_id, request_id, request_digest, execution_id, lease_id, lease_epoch,
                    envelope_digest, status, created_at, completed_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, 'AUTHORIZED', ?, NULL)
            `, input.AgentID, input.RequestID, input.RequestDigest, input.ExecutionID, input.LeaseID, input.LeaseEpoch,
				input.EnvelopeDigest, utcText(input.Now)); err != nil {
				connection.Destroy()
				return fmt.Errorf("create execution secret receipt: %w", err)
			}
			if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "EXECUTION_SECRET_RESOLVE_REQUESTED", "TASK_EXECUTION", input.ExecutionID, "SUCCEEDED", input.RequestID, input.Now); err != nil {
				connection.Destroy()
				return err
			}
		}
		return nil
	})
	if err != nil {
		connection.Destroy()
		return EncryptedExecutionDatabaseConnection{}, err
	}
	return connection, nil
}

// FinishExecutionSecretResolution 保存无秘密槽位解析结果，并在成功前再次确认租约仍有效。
func (s *Store) FinishExecutionSecretResolution(ctx context.Context, input ExecutionSecretResolutionOutcome) error {
	if err := validateExecutionSecretResolutionOutcome(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `
            SELECT status FROM execution_secret_resolution_receipts
            WHERE agent_id = ? AND request_id = ? AND request_digest = ? AND execution_id = ?
              AND lease_id = ? AND lease_epoch = ? AND envelope_digest = ?
        `, input.AgentID, input.RequestID, input.RequestDigest, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EnvelopeDigest).Scan(&status); errors.Is(err, sql.ErrNoRows) {
			return ErrEventRejected
		} else if err != nil {
			return fmt.Errorf("read execution secret completion: %w", err)
		}
		if status == "RESOLVED" {
			if input.Succeeded {
				return nil
			}
			return ErrIdempotencyConflict
		}
		if status == "FAILED" {
			if !input.Succeeded {
				return nil
			}
			return ErrIdempotencyConflict
		}
		result := "FAILED"
		auditResult := "FAILED"
		action := "EXECUTION_SECRET_RESOLVE_FAILED"
		if input.Succeeded {
			var valid int
			err := tx.QueryRowContext(ctx, `
                SELECT 1 FROM execution_leases
                WHERE execution_id = ? AND lease_id = ? AND lease_epoch = ? AND agent_id = ?
                  AND status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE') AND expires_at > ?
            `, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.AgentID, utcText(input.Now)).Scan(&valid)
			if err != nil {
				return ErrEventRejected
			}
			result, auditResult, action = "RESOLVED", "SUCCEEDED", "EXECUTION_SECRET_RESOLVED"
		}
		if _, err := tx.ExecContext(ctx, `
            UPDATE execution_secret_resolution_receipts SET status = ?, completed_at = ?
            WHERE agent_id = ? AND request_id = ?
        `, result, utcText(input.Now), input.AgentID, input.RequestID); err != nil {
			return fmt.Errorf("complete execution secret receipt: %w", err)
		}
		return insertAudit(ctx, tx, "AGENT", input.AgentID, action, "TASK_EXECUTION", input.ExecutionID, auditResult, input.RequestID+"-result", input.Now)
	})
}

// RenewExecutionLease 仅续期仍由同一 Agent 持有的当前 epoch 租约。
func (s *Store) RenewExecutionLease(ctx context.Context, input LeaseRenewal) error {
	if input.ExecutionID == "" || input.LeaseID == "" || input.AgentID == "" || input.LeaseEpoch < 1 || input.ExpiresAt.IsZero() {
		return errors.New("execution lease renewal is invalid")
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		updated, err := tx.ExecContext(ctx, `
            UPDATE execution_leases
            SET expires_at = ?
            WHERE execution_id = ? AND lease_id = ? AND lease_epoch = ? AND agent_id = ?
              AND status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE')
        `, utcText(input.ExpiresAt), input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.AgentID)
		if err != nil {
			return fmt.Errorf("renew execution lease: %w", err)
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("read execution lease renewal result: %w", err)
		}
		if affected != 1 {
			return ErrEventRejected
		}
		return nil
	})
}

func (s *Store) AppendExecutionEvent(ctx context.Context, input ExecutionEvent) error {
	if err := validateExecutionEvent(input); err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO execution_events(
                event_id, execution_id, lease_id, lease_epoch, event_seq, event_type,
                payload_json, received_at, accepted, rejection_code
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, NULL)
        `, input.EventID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EventSeq,
			input.EventType, input.PayloadJSON, utcText(input.ReceivedAt)); err != nil {
			if isExecutionEventConstraint(err) {
				return ErrEventRejected
			}
			return fmt.Errorf("append execution event: %w", err)
		}
		return nil
	})
}

// AppendAuthenticatedExecutionEvent 以当前机器租约、连续序号和固定事实类型持久化正式执行事件。
// Agent 只能上报进程和结果事实；SQLite 根据证据组合投影任务状态，不能接受任意状态写入。
func (s *Store) AppendAuthenticatedExecutionEvent(ctx context.Context, agentID string, input ExecutionEvent) (string, error) {
	if strings.TrimSpace(agentID) == "" || validateExecutionEvent(input) != nil || !validProjectedExecutionEvent(input) {
		return "", ErrEventRejected
	}
	state := ""
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var existing ExecutionEvent
		err := tx.QueryRowContext(ctx, `
            SELECT event_id, execution_id, lease_id, lease_epoch, event_seq, event_type, payload_json, received_at
            FROM execution_events WHERE event_id = ?
        `, input.EventID).Scan(&existing.EventID, &existing.ExecutionID, &existing.LeaseID, &existing.LeaseEpoch, &existing.EventSeq, &existing.EventType, &existing.PayloadJSON, new(string))
		if err == nil {
			if existing.ExecutionID == input.ExecutionID && existing.LeaseID == input.LeaseID && existing.LeaseEpoch == input.LeaseEpoch && existing.EventSeq == input.EventSeq && existing.EventType == input.EventType && existing.PayloadJSON == input.PayloadJSON {
				return tx.QueryRowContext(ctx, `SELECT state FROM task_executions WHERE execution_id = ?`, input.ExecutionID).Scan(&state)
			}
			return ErrEventRejected
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read execution event replay: %w", err)
		}
		var currentState string
		var nextSequence int64
		err = tx.QueryRowContext(ctx, `
            SELECT e.state, COALESCE((SELECT MAX(event_seq) + 1 FROM execution_events WHERE execution_id = e.execution_id), 1)
            FROM task_executions AS e
            JOIN execution_leases AS l ON l.execution_id = e.execution_id AND l.lease_id = ? AND l.lease_epoch = ?
            WHERE e.execution_id = ? AND e.agent_id = ? AND l.agent_id = ?
              AND l.status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE') AND l.expires_at > ?
        `, input.LeaseID, input.LeaseEpoch, input.ExecutionID, agentID, agentID, utcText(input.ReceivedAt)).Scan(&currentState, &nextSequence)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEventRejected
		}
		if err != nil {
			return fmt.Errorf("read current execution lease: %w", err)
		}
		if input.EventSeq != nextSequence {
			return ErrEventRejected
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO execution_events(
                event_id, execution_id, lease_id, lease_epoch, event_seq, event_type,
                payload_json, received_at, accepted, rejection_code
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, NULL)
        `, input.EventID, input.ExecutionID, input.LeaseID, input.LeaseEpoch, input.EventSeq, input.EventType, input.PayloadJSON, utcText(input.ReceivedAt)); err != nil {
			return fmt.Errorf("append authenticated execution event: %w", err)
		}
		state = currentState
		switch input.EventType {
		case "LEASE_ACKNOWLEDGED":
			if currentState != "STARTING" {
				return ErrEventRejected
			}
			if _, err := tx.ExecContext(ctx, `UPDATE execution_leases SET status = 'ACKNOWLEDGED', acknowledged_at = ? WHERE execution_id = ? AND lease_id = ? AND lease_epoch = ?`, utcText(input.ReceivedAt), input.ExecutionID, input.LeaseID, input.LeaseEpoch); err != nil {
				return fmt.Errorf("acknowledge execution lease: %w", err)
			}
		case "START_REJECTED":
			if currentState != "STARTING" {
				return ErrEventRejected
			}
			state = "FAILED"
		case "PROCESS_STARTED":
			if currentState != "STARTING" {
				return ErrEventRejected
			}
			state = "RUNNING"
			if _, err := tx.ExecContext(ctx, `UPDATE execution_leases SET status = 'ACTIVE' WHERE execution_id = ? AND lease_id = ? AND lease_epoch = ?`, input.ExecutionID, input.LeaseID, input.LeaseEpoch); err != nil {
				return fmt.Errorf("activate execution lease: %w", err)
			}
		case "TOOL_TERMINAL_OBSERVED":
			if executionEventTerminal(input.PayloadJSON) == "FAILED" && executionHasEvent(ctx, tx, input.ExecutionID, "PROCESS_EXITED") {
				state = "FAILED"
			}
		case "RESULT_FACTS_OBSERVED":
			result := executionEventResult(input.PayloadJSON)
			if result == "FAILED" && executionHasEvent(ctx, tx, input.ExecutionID, "PROCESS_EXITED") {
				state = "FAILED"
			}
			if result == "VERIFIED" && executionHasEvent(ctx, tx, input.ExecutionID, "PROCESS_EXITED") && executionHasTerminal(ctx, tx, input.ExecutionID, "SUCCEEDED") {
				state = "SUCCEEDED"
			}
		}
		if state != currentState {
			finishedAt := any(nil)
			if state == "SUCCEEDED" || state == "FAILED" {
				finishedAt = utcText(input.ReceivedAt)
			}
			if _, err := tx.ExecContext(ctx, `
                UPDATE task_executions
                SET state = ?, revision = revision + 1, finished_at = COALESCE(?, finished_at),
                    started_at = CASE WHEN ? = 'RUNNING' THEN COALESCE(started_at, ?) ELSE started_at END,
                    updated_at = ?
                WHERE execution_id = ?
            `, state, finishedAt, state, utcText(input.ReceivedAt), utcText(input.ReceivedAt), input.ExecutionID); err != nil {
				return fmt.Errorf("project execution state: %w", err)
			}
			if state == "SUCCEEDED" || state == "FAILED" {
				if _, err := tx.ExecContext(ctx, `UPDATE execution_leases SET status = 'RELEASED', released_at = ? WHERE execution_id = ? AND lease_id = ? AND lease_epoch = ?`, utcText(input.ReceivedAt), input.ExecutionID, input.LeaseID, input.LeaseEpoch); err != nil {
					return fmt.Errorf("release terminal execution lease: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return state, nil
}

// ExecutionLeaseOwned 仅为日志上传复验当前 execution 租约归属。
// 它不读取任务参数、凭据或日志正文，避免日志 API 获得额外执行能力。
func (s *Store) ExecutionLeaseOwned(ctx context.Context, agentID, executionID, leaseID string, leaseEpoch int64, now time.Time) bool {
	if s == nil || s.db == nil || agentID == "" || executionID == "" || leaseID == "" || leaseEpoch < 1 || now.IsZero() {
		return false
	}
	var found int
	err := s.db.QueryRowContext(ctx, `
        SELECT 1 FROM execution_leases AS l
        JOIN task_executions AS e ON e.execution_id = l.execution_id
        WHERE l.execution_id = ? AND l.lease_id = ? AND l.lease_epoch = ? AND l.agent_id = ?
          AND e.agent_id = ? AND l.status IN ('ISSUED', 'ACKNOWLEDGED', 'ACTIVE') AND l.expires_at > ?
    `, executionID, leaseID, leaseEpoch, agentID, agentID, utcText(now)).Scan(&found)
	return err == nil && found == 1
}

// ExecutionTaskID 返回日志投影所需的任务关联，不返回任务快照、参数或任何凭据材料。
func (s *Store) ExecutionTaskID(ctx context.Context, executionID string) (string, error) {
	if s == nil || s.db == nil || executionID == "" {
		return "", errors.New("execution task lookup is invalid")
	}
	var taskID string
	err := s.db.QueryRowContext(ctx, `SELECT task_id FROM task_executions WHERE execution_id = ?`, executionID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrDataSourceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read execution task id: %w", err)
	}
	return taskID, nil
}

// PrepareLogAppend 在文件追加前核对批次序号并确保存在一个空的活动段索引。
// 它不登记任何正文或批次；调用方必须先 fsync JSONL，再通过 CommitLogAppend 写入短事务。
func (s *Store) PrepareLogAppend(ctx context.Context, input logstream.IndexedBatch) (logstream.AppendPlan, error) {
	if s == nil || s.db == nil || !validLogBatchIndex(input) {
		return logstream.AppendPlan{}, logstream.ErrInvalidInput
	}
	plan := logstream.AppendPlan{}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var expected int64
		var sourceKind, sourceRef string
		err := tx.QueryRowContext(ctx, `SELECT expected_sequence, source_kind, source_ref FROM log_streams WHERE stream_id = ?`, input.StreamID).Scan(&expected, &sourceKind, &sourceRef)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `
                INSERT INTO log_streams(
                    stream_id, execution_id, source_kind, source_ref, stream_epoch,
                    expected_sequence, collection_status, integrity_status, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, 1, 'OPEN', 'COMPLETE', ?, ?)
            `, input.StreamID, input.ExecutionID, input.SourceKind, input.SourceRef, input.SourceEpoch, utcText(input.ReceivedAt), utcText(input.ReceivedAt)); err != nil {
				return fmt.Errorf("create log stream: %w", err)
			}
			expected = 1
		case err != nil:
			return fmt.Errorf("read log stream: %w", err)
		case sourceKind != input.SourceKind || sourceRef != input.SourceRef:
			return logstream.ErrStorageCorrupt
		}

		lastDigest, err := logStreamLastDigest(ctx, tx, input.StreamID)
		if err != nil {
			return err
		}
		var existingDigest string
		var existingLast int64
		err = tx.QueryRowContext(ctx, `
            SELECT batch_digest, last_sequence FROM log_batches
            WHERE stream_id = ? AND stream_epoch = ? AND first_sequence = ?
        `, input.StreamID, input.SourceEpoch, input.FirstSequence).Scan(&existingDigest, &existingLast)
		if err == nil {
			if existingDigest != input.Digest || existingLast != input.LastSequence {
				return logstream.ErrBatchConflict
			}
			plan = logstream.AppendPlan{Decision: logstream.BatchDuplicate, ExpectedSequence: expected, LastDigest: lastDigest}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read indexed log batch: %w", err)
		}
		if input.FirstSequence != expected || (input.GapReasonCode == "" && input.PreviousDigest != lastDigest) {
			plan = logstream.AppendPlan{Decision: logstream.BatchNeedGap, ExpectedSequence: expected, LastDigest: lastDigest}
			return nil
		}

		var segmentID string
		var segmentOrdinal, byteLength int64
		err = tx.QueryRowContext(ctx, `
            SELECT segment_id, segment_ordinal, byte_length
            FROM log_segments WHERE stream_id = ? AND state = 'OPEN'
            ORDER BY segment_ordinal DESC LIMIT 1
        `, input.StreamID).Scan(&segmentID, &segmentOrdinal, &byteLength)
		if errors.Is(err, sql.ErrNoRows) {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(segment_ordinal), -1) + 1 FROM log_segments WHERE stream_id = ?`, input.StreamID).Scan(&segmentOrdinal); err != nil {
				return fmt.Errorf("read next log segment ordinal: %w", err)
			}
			segmentID = logSegmentID(input.StreamID, segmentOrdinal)
			if _, err := tx.ExecContext(ctx, `
                INSERT INTO log_segments(
                    segment_id, stream_id, segment_ordinal, storage_key, first_sequence,
                    last_sequence, byte_length, record_count, content_digest, state, retain_until,
                    created_at, sealed_at
			) VALUES (?, ?, ?, ?, ?, NULL, 0, 0, NULL, 'OPEN', ?, ?, NULL)
			`, segmentID, input.StreamID, segmentOrdinal, segmentID+".open", input.FirstSequence, utcText(input.ReceivedAt.Add(f3LogIndexPlaceholderRetention)), utcText(input.ReceivedAt)); err != nil {
				return fmt.Errorf("create log segment: %w", err)
			}
			byteLength = 0
		} else if err != nil {
			return fmt.Errorf("read active log segment: %w", err)
		}
		plan = logstream.AppendPlan{Decision: logstream.BatchAccepted, ExpectedSequence: expected, LastDigest: lastDigest, SegmentID: segmentID, SegmentOrdinal: segmentOrdinal, SegmentByteLength: byteLength}
		return nil
	})
	if err != nil {
		return logstream.AppendPlan{}, err
	}
	return plan, nil
}

// CommitLogAppend 在段文件已经 fsync 后登记范围、摘要和偏移。
// 写入事务重新核对当前水位，避免任何未登记的文件尾部被确认或读出。
func (s *Store) CommitLogAppend(ctx context.Context, input logstream.IndexedBatch) (logstream.BatchResult, error) {
	if s == nil || s.db == nil || !validLogBatchIndex(input) || input.SegmentID == "" || input.SegmentOffsetEnd <= input.SegmentOffsetStart {
		return logstream.BatchResult{}, logstream.ErrInvalidInput
	}
	gapSummary, err := logBatchGapSummary(input.GapReasonCode)
	if err != nil {
		return logstream.BatchResult{}, logstream.ErrInvalidInput
	}
	result := logstream.BatchResult{}
	err = s.withWrite(ctx, func(tx *sql.Tx) error {
		var existingDigest string
		var existingLast int64
		err := tx.QueryRowContext(ctx, `
            SELECT batch_digest, last_sequence FROM log_batches
            WHERE stream_id = ? AND stream_epoch = ? AND first_sequence = ?
        `, input.StreamID, input.SourceEpoch, input.FirstSequence).Scan(&existingDigest, &existingLast)
		if err == nil {
			if existingDigest != input.Digest || existingLast != input.LastSequence {
				return logstream.ErrBatchConflict
			}
			expected, lastDigest, err := logStreamWatermark(ctx, tx, input.StreamID)
			if err != nil {
				return err
			}
			result = logstream.BatchResult{Decision: logstream.BatchDuplicate, ExpectedSeq: expected, LastDigest: lastDigest}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read committed log batch: %w", err)
		}
		expected, lastDigest, err := logStreamWatermark(ctx, tx, input.StreamID)
		if err != nil {
			return err
		}
		if input.FirstSequence != expected || (input.GapReasonCode == "" && input.PreviousDigest != lastDigest) {
			return logstream.ErrBatchGap
		}
		var byteLength int64
		err = tx.QueryRowContext(ctx, `SELECT byte_length FROM log_segments WHERE segment_id = ? AND stream_id = ? AND state = 'OPEN'`, input.SegmentID, input.StreamID).Scan(&byteLength)
		if errors.Is(err, sql.ErrNoRows) || byteLength != input.SegmentOffsetStart {
			return logstream.ErrStorageCorrupt
		}
		if err != nil {
			return fmt.Errorf("read log segment offset: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO log_batches(
                batch_id, stream_id, stream_epoch, first_sequence, last_sequence, batch_digest,
                segment_id, segment_offset_start, segment_offset_end, gap_summary_json, received_at,
                policy_version, parser_version, record_count
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        `, input.BatchID, input.StreamID, input.SourceEpoch, input.FirstSequence, input.LastSequence, input.Digest,
			input.SegmentID, input.SegmentOffsetStart, input.SegmentOffsetEnd, gapSummary, utcText(input.ReceivedAt), input.PolicyVersion, input.ParserVersion, input.RecordCount); err != nil {
			return fmt.Errorf("index log batch: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
            UPDATE log_segments SET last_sequence = ?, byte_length = ?, record_count = record_count + ? WHERE segment_id = ?
        `, input.LastSequence, input.SegmentOffsetEnd, input.RecordCount, input.SegmentID); err != nil {
			return fmt.Errorf("advance log segment: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE log_streams
			SET expected_sequence = ?, integrity_status = CASE WHEN ? <> '' THEN 'GAPPED' ELSE integrity_status END, updated_at = ?
			WHERE stream_id = ?
		`, input.LastSequence+1, input.GapReasonCode, utcText(input.ReceivedAt), input.StreamID); err != nil {
			return fmt.Errorf("advance log stream: %w", err)
		}
		resultDigest := input.Digest
		if input.GapReasonCode != "" {
			resultDigest = lastDigest
		}
		result = logstream.BatchResult{Decision: logstream.BatchAccepted, ExpectedSeq: input.LastSequence + 1, LastDigest: resultDigest}
		return nil
	})
	if err != nil {
		return logstream.BatchResult{}, err
	}
	return result, nil
}

// LatestTaskLogPosition 返回当前任务最后已确认批次的内部排序位置，不读取日志正文。
func (s *Store) LatestTaskLogPosition(ctx context.Context, taskID string) (logstream.BatchPosition, bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(taskID) == "" {
		return logstream.BatchPosition{}, false, logstream.ErrInvalidInput
	}
	var batchID, receivedAt string
	err := s.db.QueryRowContext(ctx, `
        SELECT b.batch_id, b.received_at
        FROM task_executions AS execution
        JOIN log_streams AS stream ON stream.execution_id = execution.execution_id
        JOIN log_batches AS b ON b.stream_id = stream.stream_id
        WHERE execution.task_id = ?
        ORDER BY b.received_at DESC, b.batch_id DESC LIMIT 1
    `, taskID).Scan(&batchID, &receivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return logstream.BatchPosition{}, false, nil
	}
	if err != nil {
		return logstream.BatchPosition{}, false, fmt.Errorf("read task log watermark: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, receivedAt)
	if err != nil {
		return logstream.BatchPosition{}, false, logstream.ErrStorageCorrupt
	}
	return logstream.BatchPosition{BatchID: batchID, ReceivedAt: parsed.UTC()}, true, nil
}

// NextTaskLogBatch 只返回固定水位内下一批的索引，正文仍由分段文件读取。
func (s *Store) NextTaskLogBatch(ctx context.Context, taskID string, snapshot, after logstream.BatchPosition, includeAfter bool) (logstream.IndexedBatch, bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(taskID) == "" || snapshot.BatchID == "" || snapshot.ReceivedAt.IsZero() {
		return logstream.IndexedBatch{}, false, logstream.ErrInvalidInput
	}
	query := `
		SELECT b.batch_id, stream.execution_id, stream.stream_id, stream.source_kind, stream.source_ref,
		       b.stream_epoch, b.first_sequence, b.last_sequence, b.batch_digest,
		       COALESCE((
				SELECT prior.batch_digest FROM log_batches AS prior
				WHERE prior.stream_id = b.stream_id AND prior.gap_summary_json IS NULL AND prior.last_sequence < b.first_sequence
				ORDER BY prior.last_sequence DESC LIMIT 1
			), ''), COALESCE(b.gap_summary_json, ''),
               b.policy_version, b.parser_version, b.segment_id, segment.segment_ordinal,
		       segment.storage_key, segment.state, segment.byte_length, b.segment_offset_start, b.segment_offset_end, b.record_count, b.received_at
        FROM task_executions AS execution
        JOIN log_streams AS stream ON stream.execution_id = execution.execution_id
        JOIN log_batches AS b ON b.stream_id = stream.stream_id
        JOIN log_segments AS segment ON segment.segment_id = b.segment_id
        WHERE execution.task_id = ?
          AND (b.received_at < ? OR (b.received_at = ? AND b.batch_id <= ?))
		  AND (? = '' OR b.received_at > ? OR (b.received_at = ? AND b.batch_id ` + logBatchPositionOperator(includeAfter) + ` ?))
        ORDER BY b.received_at ASC, b.batch_id ASC LIMIT 1
	`
	var batch logstream.IndexedBatch
	var receivedAt, gapSummary string
	err := s.db.QueryRowContext(ctx, query, taskID, utcText(snapshot.ReceivedAt), utcText(snapshot.ReceivedAt), snapshot.BatchID,
		after.BatchID, utcText(after.ReceivedAt), utcText(after.ReceivedAt), after.BatchID).Scan(
		&batch.BatchID, &batch.ExecutionID, &batch.StreamID, &batch.SourceKind, &batch.SourceRef,
		&batch.SourceEpoch, &batch.FirstSequence, &batch.LastSequence, &batch.Digest,
		&batch.PreviousDigest, &gapSummary,
		&batch.PolicyVersion, &batch.ParserVersion, &batch.SegmentID, &batch.SegmentOrdinal,
		&batch.SegmentStorageKey, &batch.SegmentState, &batch.SegmentByteLength, &batch.SegmentOffsetStart, &batch.SegmentOffsetEnd, &batch.RecordCount, &receivedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return logstream.IndexedBatch{}, false, nil
	}
	if err != nil {
		return logstream.IndexedBatch{}, false, fmt.Errorf("read indexed task log batch: %w", err)
	}
	if batch.GapReasonCode, err = parseLogGapSummary(gapSummary); err != nil {
		return logstream.IndexedBatch{}, false, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, receivedAt)
	if err != nil {
		return logstream.IndexedBatch{}, false, logstream.ErrStorageCorrupt
	}
	batch.ReceivedAt = parsed.UTC()
	return batch, true, nil
}

// ListLogSegmentBatches 返回恢复时复核已登记正文摘要所需的批次索引。
// 它只面向内部段 ID，不能按浏览器或 Agent 提供的路径、任务标识读取。
func (s *Store) ListLogSegmentBatches(ctx context.Context, segmentID string) ([]logstream.IndexedBatch, error) {
	if s == nil || s.db == nil || !sha256Pattern.MatchString(segmentID) {
		return nil, logstream.ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.batch_id, stream.execution_id, stream.stream_id, stream.source_kind, stream.source_ref,
		       b.stream_epoch, b.first_sequence, b.last_sequence, b.batch_digest,
		       COALESCE((
				SELECT prior.batch_digest FROM log_batches AS prior
				WHERE prior.stream_id = b.stream_id AND prior.gap_summary_json IS NULL AND prior.last_sequence < b.first_sequence
				ORDER BY prior.last_sequence DESC LIMIT 1
			), ''), COALESCE(b.gap_summary_json, ''),
		       b.policy_version, b.parser_version, b.segment_id, segment.segment_ordinal,
		       segment.storage_key, segment.state, segment.byte_length, b.segment_offset_start, b.segment_offset_end, b.record_count, b.received_at
		FROM log_batches AS b
		JOIN log_streams AS stream ON stream.stream_id = b.stream_id
		JOIN log_segments AS segment ON segment.segment_id = b.segment_id
		WHERE b.segment_id = ?
		ORDER BY b.segment_offset_start ASC, b.batch_id ASC
	`, segmentID)
	if err != nil {
		return nil, fmt.Errorf("list indexed log segment batches: %w", err)
	}
	defer rows.Close()
	batches := make([]logstream.IndexedBatch, 0)
	for rows.Next() {
		var batch logstream.IndexedBatch
		var receivedAt, gapSummary string
		if err := rows.Scan(
			&batch.BatchID, &batch.ExecutionID, &batch.StreamID, &batch.SourceKind, &batch.SourceRef,
			&batch.SourceEpoch, &batch.FirstSequence, &batch.LastSequence, &batch.Digest,
			&batch.PreviousDigest, &gapSummary,
			&batch.PolicyVersion, &batch.ParserVersion, &batch.SegmentID, &batch.SegmentOrdinal,
			&batch.SegmentStorageKey, &batch.SegmentState, &batch.SegmentByteLength, &batch.SegmentOffsetStart, &batch.SegmentOffsetEnd, &batch.RecordCount, &receivedAt,
		); err != nil {
			return nil, fmt.Errorf("scan indexed log segment batch: %w", err)
		}
		if batch.GapReasonCode, err = parseLogGapSummary(gapSummary); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, receivedAt)
		if err != nil {
			return nil, logstream.ErrStorageCorrupt
		}
		batch.ReceivedAt = parsed.UTC()
		batches = append(batches, batch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed log segment batches: %w", err)
	}
	return batches, nil
}

// ListOpenLogSegments 返回启动恢复所需的活动段元数据，不返回日志正文或本机绝对路径。
func (s *Store) ListOpenLogSegments(ctx context.Context) ([]logstream.IndexedSegment, error) {
	if s == nil || s.db == nil {
		return nil, logstream.ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT segment_id, storage_key, byte_length, state
		FROM log_segments
		WHERE state = 'OPEN'
		ORDER BY stream_id ASC, segment_ordinal ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list open log segments: %w", err)
	}
	defer rows.Close()
	segments := make([]logstream.IndexedSegment, 0)
	for rows.Next() {
		var segment logstream.IndexedSegment
		if err := rows.Scan(&segment.SegmentID, &segment.StorageKey, &segment.ByteLength, &segment.State); err != nil {
			return nil, fmt.Errorf("scan open log segment: %w", err)
		}
		if !validOpenLogSegment(segment) {
			return nil, logstream.ErrStorageCorrupt
		}
		segments = append(segments, segment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate open log segments: %w", err)
	}
	return segments, nil
}

// SealLogSegment 将已完成原子改名的活动段登记为不可变段。
// 文件长度和摘要必须与 SQLite 当前水位一致，避免把未索引尾部或错误段标记为可读。
func (s *Store) SealLogSegment(ctx context.Context, segment logstream.IndexedSegment, digest string, sealedAt time.Time) error {
	if s == nil || s.db == nil || !validOpenLogSegment(segment) || !sha256Pattern.MatchString(digest) || sealedAt.IsZero() {
		return logstream.ErrInvalidInput
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		var storageKey, state, existingDigest string
		var byteLength int64
		err := tx.QueryRowContext(ctx, `
			SELECT storage_key, byte_length, state, COALESCE(content_digest, '')
			FROM log_segments WHERE segment_id = ?
		`, segment.SegmentID).Scan(&storageKey, &byteLength, &state, &existingDigest)
		if errors.Is(err, sql.ErrNoRows) {
			return logstream.ErrStorageCorrupt
		}
		if err != nil {
			return fmt.Errorf("read log segment for sealing: %w", err)
		}
		if state == "SEALED" {
			if storageKey != segment.SegmentID+".jsonl" || byteLength != segment.ByteLength || existingDigest != digest {
				return logstream.ErrStorageCorrupt
			}
			return nil
		}
		if state != "OPEN" || storageKey != segment.StorageKey || byteLength != segment.ByteLength {
			return logstream.ErrStorageCorrupt
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE log_segments
			SET storage_key = ?, content_digest = ?, state = 'SEALED', sealed_at = ?
			WHERE segment_id = ? AND state = 'OPEN'
		`, segment.SegmentID+".jsonl", digest, utcText(sealedAt), segment.SegmentID); err != nil {
			return fmt.Errorf("seal log segment index: %w", err)
		}
		return nil
	})
}

func logBatchPositionOperator(includeAfter bool) string {
	if includeAfter {
		return ">="
	}
	return ">"
}

func validLogBatchIndex(input logstream.IndexedBatch) bool {
	return input.BatchID != "" && input.ExecutionID != "" && input.StreamID != "" && input.SourceRef != "" &&
		(input.SourceKind == "STDOUT" || input.SourceKind == "STDERR" || input.SourceKind == "TOOL_FILE" || input.SourceKind == "AGENT_EVENT") &&
		input.SourceEpoch > 0 && input.FirstSequence > 0 && input.LastSequence >= input.FirstSequence &&
		sha256Pattern.MatchString(input.Digest) && input.PolicyVersion != "" && input.ParserVersion != "" && input.RecordCount > 0 && !input.ReceivedAt.IsZero() &&
		(input.GapReasonCode == "" || validLogGapReasonCode(input.GapReasonCode))
}

func validOpenLogSegment(segment logstream.IndexedSegment) bool {
	return sha256Pattern.MatchString(segment.SegmentID) && segment.StorageKey == segment.SegmentID+".open" && segment.ByteLength >= 0 && segment.State == "OPEN"
}

func validLogGapReasonCode(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func logBatchGapSummary(reasonCode string) (any, error) {
	if reasonCode == "" {
		return nil, nil
	}
	if !validLogGapReasonCode(reasonCode) {
		return nil, logstream.ErrInvalidInput
	}
	value, err := json.Marshal(struct {
		ReasonCode string `json:"reasonCode"`
	}{ReasonCode: reasonCode})
	if err != nil {
		return nil, err
	}
	return string(value), nil
}

func parseLogGapSummary(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	var summary struct {
		ReasonCode string `json:"reasonCode"`
	}
	if err := json.Unmarshal([]byte(value), &summary); err != nil || !validLogGapReasonCode(summary.ReasonCode) {
		return "", logstream.ErrStorageCorrupt
	}
	return summary.ReasonCode, nil
}

func logStreamWatermark(ctx context.Context, tx *sql.Tx, streamID string) (int64, string, error) {
	var expected int64
	if err := tx.QueryRowContext(ctx, `SELECT expected_sequence FROM log_streams WHERE stream_id = ?`, streamID).Scan(&expected); err != nil {
		return 0, "", fmt.Errorf("read log stream watermark: %w", err)
	}
	digest, err := logStreamLastDigest(ctx, tx, streamID)
	return expected, digest, err
}

func logStreamLastDigest(ctx context.Context, tx *sql.Tx, streamID string) (string, error) {
	var digest string
	err := tx.QueryRowContext(ctx, `SELECT batch_digest FROM log_batches WHERE stream_id = ? AND gap_summary_json IS NULL ORDER BY last_sequence DESC LIMIT 1`, streamID).Scan(&digest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read last log digest: %w", err)
	}
	return digest, nil
}

func logSegmentID(streamID string, ordinal int64) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("log-segment|%s|%d", streamID, ordinal)))
	return hex.EncodeToString(digest[:])
}

func validProjectedExecutionEvent(input ExecutionEvent) bool {
	var payload map[string]any
	if json.Unmarshal([]byte(input.PayloadJSON), &payload) != nil {
		return false
	}
	switch input.EventType {
	case "LEASE_ACKNOWLEDGED", "PROCESS_EXITED":
		return len(payload) == 0 || (input.EventType == "PROCESS_EXITED" && len(payload) == 1 && validExecutionExitCode(payload["exitCode"]))
	case "START_REJECTED":
		return len(payload) == 1 && payload["noProcess"] == true
	case "PROCESS_STARTED":
		return len(payload) == 4 && validExecutionPID(payload["pid"]) && validExecutionTime(payload["startedAt"]) && validExecutionDigest(payload["executableDigest"]) && validExecutionOpaque(payload["bootId"])
	case "TOOL_TERMINAL_OBSERVED":
		return len(payload) == 1 && (payload["terminal"] == "SUCCEEDED" || payload["terminal"] == "FAILED")
	case "RESULT_FACTS_OBSERVED":
		return len(payload) == 3 && (payload["result"] == "VERIFIED" || payload["result"] == "FAILED") && validExecutionCount(payload["fileCount"]) && validExecutionBytes(payload["totalBytes"])
	default:
		return false
	}
}

func validExecutionPID(value any) bool {
	number, ok := value.(float64)
	return ok && number >= 1 && number == float64(int64(number))
}
func validExecutionExitCode(value any) bool {
	number, ok := value.(float64)
	return ok && number >= -1 && number <= 255 && number == float64(int64(number))
}
func validExecutionCount(value any) bool {
	number, ok := value.(float64)
	return ok && number >= 0 && number <= 1_000_000 && number == float64(int64(number))
}
func validExecutionBytes(value any) bool {
	number, ok := value.(float64)
	return ok && number >= 0 && number <= float64(^uint64(0)) && number == float64(uint64(number))
}
func validExecutionTime(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, text)
	return err == nil
}
func validExecutionDigest(value any) bool { text, ok := value.(string); return ok && isSHA256(text) }
func validExecutionOpaque(value any) bool {
	text, ok := value.(string)
	return ok && validAgentOpaqueValue(text, 256)
}

func executionHasEvent(ctx context.Context, tx *sql.Tx, executionID, eventType string) bool {
	var found int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM execution_events WHERE execution_id = ? AND event_type = ?`, executionID, eventType).Scan(&found)
	return err == nil && found == 1
}

func executionHasTerminal(ctx context.Context, tx *sql.Tx, executionID, terminal string) bool {
	var payload string
	err := tx.QueryRowContext(ctx, `SELECT payload_json FROM execution_events WHERE execution_id = ? AND event_type = 'TOOL_TERMINAL_OBSERVED' ORDER BY event_seq DESC LIMIT 1`, executionID).Scan(&payload)
	return err == nil && executionEventTerminal(payload) == terminal
}

func executionEventTerminal(payload string) string {
	var value struct {
		Terminal string `json:"terminal"`
	}
	_ = json.Unmarshal([]byte(payload), &value)
	return value.Terminal
}

func executionEventResult(payload string) string {
	var value struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal([]byte(payload), &value)
	return value.Result
}

// Backup creates a new, consistent SQLite snapshot using VACUUM INTO. It
// refuses to overwrite an existing file and does not include root-key material.
func (s *Store) Backup(ctx context.Context, destination string) error {
	if s == nil || s.db == nil {
		return errors.New("SQLite store is nil")
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("backup destination is required")
	}
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve backup destination: %w", err)
	}
	if _, err := os.Stat(absDestination); err == nil {
		return errors.New("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination: %w", err)
	}
	if info, err := os.Stat(filepath.Dir(absDestination)); err != nil {
		return fmt.Errorf("backup directory is unavailable: %w", err)
	} else if !info.IsDir() {
		return errors.New("backup parent is not a directory")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var literal string
	if err := s.db.QueryRowContext(ctx, "SELECT quote(?)", absDestination).Scan(&literal); err != nil {
		return fmt.Errorf("quote backup destination: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO "+literal); err != nil {
		return fmt.Errorf("create SQLite backup: %w", err)
	}
	return verifyBackup(ctx, absDestination)
}

func (s *Store) withWrite(ctx context.Context, operation func(*sql.Tx) error) error {
	if s == nil || s.db == nil {
		return errors.New("SQLite store is nil")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite write transaction: %w", err)
	}
	defer tx.Rollback()
	if err := operation(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SQLite write transaction: %w", err)
	}
	return nil
}

func insertAudit(ctx context.Context, tx *sql.Tx, actorType, actorID, action, objectType, objectID, result, requestID string, occurredAt time.Time) error {
	safeDiff, err := json.Marshal(map[string]string{"objectId": objectID})
	if err != nil {
		return fmt.Errorf("encode audit diff: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO audit_events(
            audit_event_id, actor_type, actor_id, action, object_type, object_id,
            result, request_id, safe_diff_json, occurred_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, auditID(objectID, requestID), actorType, actorID, action, objectType, objectID, result, requestID, string(safeDiff), utcText(occurredAt)); err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

// insertPrecheckSecretResolutionAudit 为同一请求的意图和结果使用不同不可变审计标识。
// 普通审计标识按对象和请求去重；槽位解析需要同时保留两类无秘密事实，不能互相覆盖。
func insertPrecheckSecretResolutionAudit(ctx context.Context, tx *sql.Tx, agentID, precheckID, requestID, action, result string, occurredAt time.Time) error {
	safeDiff, err := json.Marshal(map[string]string{"objectId": precheckID})
	if err != nil {
		return fmt.Errorf("encode precheck secret audit diff: %w", err)
	}
	auditEventID := auditID(precheckID, requestID) + "-" + strings.ToLower(action)
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO audit_events(
            audit_event_id, actor_type, actor_id, action, object_type, object_id,
            result, request_id, safe_diff_json, occurred_at
        ) VALUES (?, 'AGENT', ?, ?, 'PRECHECK', ?, ?, ?, ?, ?)
    `, auditEventID, agentID, action, precheckID, result, requestID, string(safeDiff), utcText(occurredAt)); err != nil {
		return fmt.Errorf("append precheck secret resolution audit: %w", err)
	}
	return nil
}

func validateDraftUpdate(input DraftUpdate) error {
	if input.DraftID == "" || input.ExpectedRevision < 1 || input.ConfigFingerprint == "" || input.UpdatedAt.IsZero() {
		return errors.New("draft update identity is invalid")
	}
	if !isSHA256(input.ConfigFingerprint) {
		return errors.New("draft configuration fingerprint is invalid")
	}
	if err := validateSafeObjectJSON(input.ConfigJSON); err != nil {
		return fmt.Errorf("draft configuration is invalid: %w", err)
	}
	if err := validateSafeObjectJSON(input.InvalidationJSON); err != nil {
		return fmt.Errorf("draft invalidation is invalid: %w", err)
	}
	return nil
}

func validateDataSourceCreate(input DataSourceCreate) error {
	if input.DataSourceID == "" || input.CredentialID == "" || input.CreatorSubjectID == "" || input.DisplayName == "" || input.NormalizedName == "" || input.Host == "" || input.ClusterName == "" || input.TenantName == "" || input.Username == "" || input.KeyID == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.CreatedAt.IsZero() || input.Port < 1 || input.Port > 65535 {
		return errors.New("data source create identity is invalid")
	}
	if !isSHA256(input.RequestDigest) || len(input.Nonce) == 0 || len(input.Ciphertext) == 0 {
		return errors.New("data source create security material is invalid")
	}
	if !oneOf(input.Environment, "DEVELOPMENT", "TEST", "STAGING", "PRODUCTION") || input.ConnectionKind != "ODP" || !oneOf(input.CompatibilityMode, "MYSQL", "ORACLE") || (input.CompatibilityMode == "ORACLE" && input.DefaultDatabase != "") {
		return errors.New("data source create enum is invalid")
	}
	return nil
}

func validateAuthSubject(input AuthSubject) error {
	if strings.TrimSpace(input.SubjectID) == "" || strings.TrimSpace(input.ExternalSubject) == "" || strings.TrimSpace(input.DisplayName) == "" || input.CreatedAt.IsZero() || input.UpdatedAt.IsZero() {
		return errors.New("auth subject identity is invalid")
	}
	if !oneOf(input.AccountStatus, "ACTIVE", "DISABLED", "UNKNOWN") {
		return errors.New("auth subject status is invalid")
	}
	return nil
}

// validateAgentEnrollmentIssue 防止将原始关联材料或无效时钟写入持久化边界。
func validateAgentEnrollmentIssue(input AgentEnrollmentIssue) error {
	if !validAgentOpaqueValue(input.EnrollmentID, 256) || !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.ActorID, 256) || !validAgentOpaqueValue(input.RequestID, 256) || len(input.TokenDigest) != sha256.Size || input.CreatedAt.IsZero() || input.ExpiresAt.IsZero() || !input.ExpiresAt.After(input.CreatedAt) {
		return errors.New("agent enrollment issue is invalid")
	}
	return nil
}

// validateAgentEnrollmentExchange 确保关联交换只使用短时摘要和固定协议版本。
func validateAgentEnrollmentExchange(input AgentEnrollmentExchange) error {
	if !validAgentOpaqueValue(input.EnrollmentID, 256) || !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.RequestID, 256) || !validAgentProtocolVersion(input.ProtocolVersion) || len(input.EnrollmentMaterialDigest) != sha256.Size || len(input.CredentialDigest) != sha256.Size || input.ExchangedAt.IsZero() {
		return errors.New("agent enrollment exchange is invalid")
	}
	return nil
}

// validateAgentHeartbeat 限制 Agent 可写的当前事实，避免心跳退化为任意 JSON 写入通道。
func validateAgentHeartbeat(input AgentHeartbeat) error {
	if !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.BootID, 256) || !validAgentOpaqueValue(input.RequestID, 256) || !validAgentProtocolVersion(input.ProtocolVersion) || input.ObservedAt.IsZero() || input.ReceivedAt.IsZero() || input.CapacityTotal != 1 || input.CapacityUsed < 0 || input.CapacityUsed > input.CapacityTotal {
		return errors.New("agent heartbeat is invalid")
	}
	if !input.Facts.ObservedAt.Equal(input.ObservedAt) {
		return errors.New("agent heartbeat fact time does not match observation time")
	}
	return validateAgentEnvironmentFacts(input.Facts)
}

func validateAgentEnvironmentFacts(facts AgentEnvironmentFacts) error {
	if !oneOf(facts.OperatingSystem, "WINDOWS", "LINUX") || !oneOf(facts.Architecture, "AMD64", "ARM64") || !validAgentOpaqueValue(facts.AgentVersion, 128) || facts.ObservedAt.IsZero() {
		return errors.New("agent environment facts are invalid")
	}
	for _, percentage := range []*int{facts.CPUUsagePercent, facts.MemoryUsagePercent} {
		if percentage != nil && (*percentage < 0 || *percentage > 100) {
			return errors.New("agent resource percentage is invalid")
		}
	}
	if facts.RuntimeConfigurationDigest != "" && !isSHA256(facts.RuntimeConfigurationDigest) {
		return errors.New("agent runtime configuration digest is invalid")
	}
	if len(facts.DataRootUsages) > 32 {
		return errors.New("agent data root usages exceed limit")
	}
	seenRoots := make(map[string]struct{}, len(facts.DataRootUsages))
	for _, usage := range facts.DataRootUsages {
		if !isSHA256(usage.RootDigest) || usage.AvailableBytes > usage.TotalBytes {
			return errors.New("agent data root usage is invalid")
		}
		if _, exists := seenRoots[usage.RootDigest]; exists {
			return errors.New("agent data root usage is duplicated")
		}
		seenRoots[usage.RootDigest] = struct{}{}
	}
	return nil
}

// ExecutionNodeRuntimeConfigurationDigest 对注册页面声明的本机配置生成稳定摘要。
// 摘要只用于关联后配置漂移检测和空间采样映射，不替代 Agent 对真实路径的本机核验。
func ExecutionNodeRuntimeConfigurationDigest(node ExecutionNode) string {
	return executionNodeRuntimeConfigurationDigest(node.Platform, node.ToolHome, node.JavaPath, node.AllowedRoots)
}

func executionNodeRuntimeConfigurationDigest(platform, toolHome, javaPath string, allowedRoots []string) string {
	payload, err := json.Marshal(struct {
		Platform     string   `json:"platform"`
		ToolHome     string   `json:"toolHome"`
		JavaPath     string   `json:"javaPath"`
		AllowedRoots []string `json:"allowedRoots"`
	}{Platform: platform, ToolHome: toolHome, JavaPath: javaPath, AllowedRoots: allowedRoots})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func validAgentProtocolVersion(value string) bool {
	return validAgentOpaqueValue(value, 64)
}

func validAgentOpaqueValue(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) && !strings.ContainsRune(value, '\x00') && !strings.ContainsAny(value, "\r\n")
}

// validateExecutionNodeCreate 约束节点配置只能表达受控路由信息。
// 平台与根目录在 Agent 上报前只是管理员声明，仍需严格限制为受支持的平台和绝对路径。
func validateExecutionNodeCreate(input ExecutionNodeCreate) error {
	if input.NodeID == "" || input.CreatorSubjectID == "" || input.DisplayName == "" || input.NormalizedName == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.CreatedAt.IsZero() {
		return errors.New("execution node create identity is invalid")
	}
	if !isSHA256(input.RequestDigest) || !ValidateExecutionNodeRuntimeConfiguration(input.Platform, input.AllowedRoots, input.ToolHome, input.JavaPath) {
		return errors.New("execution node create configuration is invalid")
	}
	return nil
}

// validateExecutionNodeUpdate 复用创建时的平台与根目录约束，并补充乐观锁和审计主体。
func validateExecutionNodeUpdate(input ExecutionNodeUpdate) error {
	if input.NodeID == "" || input.ActorSubjectID == "" || input.ExpectedRevision < 1 || input.DisplayName == "" || input.NormalizedName == "" || input.RequestID == "" || input.UpdatedAt.IsZero() {
		return errors.New("execution node update identity is invalid")
	}
	if !ValidateExecutionNodeRuntimeConfiguration(input.Platform, input.AllowedRoots, input.ToolHome, input.JavaPath) {
		return errors.New("execution node update configuration is invalid")
	}
	return nil
}

// validateExecutionNodeEnvironmentCheckRequest 将浏览器动作限制为一次不可扩展的固定检查请求。
func validateExecutionNodeEnvironmentCheckRequest(input ExecutionNodeEnvironmentCheckRequest) error {
	if !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.ActorSubjectID, 256) ||
		!validAgentOpaqueValue(input.CheckID, 256) || !validAgentOpaqueValue(input.RequestID, 256) ||
		input.ExpectedRevision < 1 || input.RequestedAt.IsZero() {
		return errors.New("execution node environment check request is invalid")
	}
	return nil
}

// validateExecutionNodeEnvironmentCheckRefresh 限制自动续接请求只能绑定刚完成认证的当前 Agent 事实。
func validateExecutionNodeEnvironmentCheckRefresh(input ExecutionNodeEnvironmentCheckRefresh) error {
	if !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.AgentID, 256) ||
		!validAgentOpaqueValue(input.CheckID, 256) || !validAgentOpaqueValue(input.RequestID, 256) ||
		input.FactsRevision < 1 || input.RequestedAt.IsZero() {
		return errors.New("execution node environment check refresh is invalid")
	}
	return nil
}

// validateAgentExecutionNodeEnvironmentCheckCompletion 只允许三种固定稳定证据码。
func validateAgentExecutionNodeEnvironmentCheckCompletion(input AgentExecutionNodeEnvironmentCheckCompletion) error {
	if !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.AgentID, 256) ||
		!validAgentOpaqueValue(input.CheckID, 256) || !validAgentOpaqueValue(input.RequestID, 256) ||
		input.FactsRevision < 1 || input.CompletedAt.IsZero() {
		return errors.New("execution node environment check completion identity is invalid")
	}
	if input.Status == "PASSED" && input.Code == "TOOL_RUNTIME_READY" {
		return nil
	}
	if input.Status == "FAILED" && oneOf(input.Code, "TOOL_RUNTIME_INVALID", "TOOL_RUNTIME_UNAVAILABLE") {
		return nil
	}
	return errors.New("execution node environment check completion result is invalid")
}

// validateExecutionNodeEnable 约束启用动作必须带版本、审计主体和控制面在线窗口。
func validateExecutionNodeEnable(input ExecutionNodeEnable) error {
	if !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.ActorSubjectID, 256) ||
		!validAgentOpaqueValue(input.RequestID, 256) || input.ExpectedRevision < 1 || input.EnabledAt.IsZero() ||
		input.OnlineAfter.IsZero() || input.OnlineAfter.After(input.EnabledAt) {
		return errors.New("execution node enable is invalid")
	}
	return nil
}

// validateExecutionNodeDeletion 保证删除或归档经过对象版本、审计主体和明确时钟。
func validateExecutionNodeDeletion(input ExecutionNodeDeletion) error {
	if !validAgentOpaqueValue(input.NodeID, 256) || !validAgentOpaqueValue(input.ActorSubjectID, 256) ||
		!validAgentOpaqueValue(input.RequestID, 256) || input.ExpectedRevision < 1 || input.DeletedAt.IsZero() {
		return errors.New("execution node deletion identity is invalid")
	}
	return nil
}

// ValidateExecutionNodeConfiguration 只接受首条切片支持的平台及其对应的本机导出目录白名单。
// Windows 白名单兼容既有反斜杠根目录和 /E:/ 正斜杠盘符格式；控制面保存原文，不转换为 URI 或跨平台路径。
func ValidateExecutionNodeConfiguration(platform string, allowedRoots []string) bool {
	if !oneOf(platform, "WINDOWS_AMD64", "LINUX_AMD64", "LINUX_ARM64") || len(allowedRoots) == 0 || len(allowedRoots) > 32 {
		return false
	}
	seen := make(map[string]struct{}, len(allowedRoots))
	for _, root := range allowedRoots {
		if len(root) == 0 || len(root) > 4096 || root != strings.TrimSpace(root) || strings.ContainsRune(root, '\x00') || strings.ContainsAny(root, "\r\n") {
			return false
		}
		if platform == "WINDOWS_AMD64" {
			if !outputpath.IsAllowedRootPath(platform, root) {
				return false
			}
		} else if !strings.HasPrefix(root, "/") {
			return false
		}
		key := root
		if platform == "WINDOWS_AMD64" {
			key = strings.ToLower(root)
		}
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

// ValidateExecutionNodeRuntimeConfiguration 校验浏览器登记的工具、Java 和导出数据目录声明。
// 这里只能验证路径格式；存在性、可执行性、可写性和空间必须由首次关联后的目标 Agent 复核。
func ValidateExecutionNodeRuntimeConfiguration(platform string, allowedRoots []string, toolHome, javaPath string) bool {
	if !ValidateExecutionNodeConfiguration(platform, allowedRoots) {
		return false
	}
	return validExecutionNodeLocalPath(platform, toolHome) && validExecutionNodeLocalPath(platform, javaPath)
}

func validExecutionNodeLocalPath(platform, value string) bool {
	if len(value) == 0 || len(value) > 4096 || value != strings.TrimSpace(value) || strings.ContainsRune(value, '\x00') || strings.ContainsAny(value, "\r\n") {
		return false
	}
	if platform == "WINDOWS_AMD64" {
		return windowsNativeAbsolutePathPattern.MatchString(value)
	}
	return strings.HasPrefix(value, "/")
}

// validateDataSourceStateChange 将状态动作限制为已确认的启停状态。
func validateDataSourceStateChange(input DataSourceStateChange) error {
	if input.DataSourceID == "" || input.ActorSubjectID == "" || input.RequestID == "" || input.ChangedAt.IsZero() || input.ExpectedRevision < 1 {
		return errors.New("data source state change identity is invalid")
	}
	if !oneOf(input.TargetState, "ENABLED", "DISABLED") {
		return errors.New("data source state change target is invalid")
	}
	return nil
}

// validateDataSourceDeletion 保证删除和归档都经过对象版本、审计主体和明确时钟。
func validateDataSourceDeletion(input DataSourceDeletion) error {
	if input.DataSourceID == "" || input.ActorSubjectID == "" || input.RequestID == "" || input.DeletedAt.IsZero() || input.ExpectedRevision < 1 {
		return errors.New("data source deletion identity is invalid")
	}
	return nil
}

// validateDataSourceUpdate 复用创建时的连接枚举约束，并额外验证轮换材料。
func validateDataSourceUpdate(input DataSourceUpdate) error {
	if input.DataSourceID == "" || input.ActorSubjectID == "" || input.ExpectedRevision < 1 || input.DisplayName == "" || input.NormalizedName == "" || input.Host == "" || input.ClusterName == "" || input.TenantName == "" || input.Username == "" || input.RequestID == "" || input.UpdatedAt.IsZero() || input.Port < 1 || input.Port > 65535 {
		return errors.New("data source update identity is invalid")
	}
	if !oneOf(input.Environment, "DEVELOPMENT", "TEST", "STAGING", "PRODUCTION") || input.ConnectionKind != "ODP" || !oneOf(input.CompatibilityMode, "MYSQL", "ORACLE") || (input.CompatibilityMode == "ORACLE" && input.DefaultDatabase != "") {
		return errors.New("data source update enum is invalid")
	}
	if input.Password != nil && (input.Password.CredentialID == "" || input.Password.Revision < 2 || input.Password.KeyID == "" || len(input.Password.Nonce) == 0 || len(input.Password.Ciphertext) == 0) {
		return errors.New("data source rotated credential is invalid")
	}
	return nil
}

// validateExportDraftCreate 拒绝秘密字段和未完成的生成指纹，确保草稿只保存
// 可重算的非敏感配置。
func validateExportDraftCreate(input ExportDraftCreate) error {
	if input.DraftID == "" || input.OwnerSubjectID == "" || input.DataSourceID == "" || input.NodeID == "" || input.ToolVersion == "" || input.MetadataVersion == "" || input.CapabilityVersion == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.CreatedAt.IsZero() || input.UpdatedAt.IsZero() {
		return errors.New("export draft create identity is invalid")
	}
	if !isSHA256(input.ConfigFingerprint) || !isSHA256(input.RequestDigest) {
		return errors.New("export draft create fingerprint is invalid")
	}
	if err := validateSafeObjectJSON(input.ConfigJSON); err != nil {
		return fmt.Errorf("export draft configuration is invalid: %w", err)
	}
	if err := validateSafeObjectJSON(input.InvalidationJSON); err != nil {
		return fmt.Errorf("export draft invalidation is invalid: %w", err)
	}
	return nil
}

// validatePrecheckCreate 保证预检查只能绑定到一个已生成的草稿版本与配置指纹。
func validatePrecheckCreate(input PrecheckCreate) error {
	if input.PrecheckID == "" || input.DraftID == "" || input.DraftRevision < 1 || input.DataSourceID == "" || input.CredentialID == "" || input.CredentialRevision < 1 || input.NodeID == "" || input.CreatorSubjectID == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.CreatedAt.IsZero() || input.ValidUntil.IsZero() || !input.ValidUntil.After(input.CreatedAt) {
		return errors.New("precheck create identity is invalid")
	}
	if !isSHA256(input.ConfigFingerprint) || !isSHA256(input.RequestDigest) {
		return errors.New("precheck create fingerprint is invalid")
	}
	return nil
}

// validatePrecheckBinding 限制领取路径只处理创建事务冻结的非敏感版本绑定。
// nodeFactsRevision 为零或摘要缺失的历史记录不能进入持久化租约流程。
func validatePrecheckBinding(binding PrecheckBinding) error {
	if !validAgentOpaqueValue(binding.PrecheckID, 256) || !validAgentOpaqueValue(binding.DraftID, 256) ||
		binding.DraftRevision < 1 || !isSHA256(binding.ConfigFingerprint) ||
		!validAgentOpaqueValue(binding.DataSourceID, 256) || !validAgentOpaqueValue(binding.CredentialID, 256) ||
		binding.CredentialRevision < 1 || !validAgentOpaqueValue(binding.NodeID, 256) ||
		binding.NodeFactsRevision < 1 || !validAgentOpaqueValue(binding.BindingAgentID, 256) ||
		!isSHA256(binding.BindingDigest) || binding.ValidUntil.IsZero() {
		return errors.New("precheck binding is invalid")
	}
	return nil
}

// validatePrecheckClaim 确保领取时间和租约期限只能由控制面调用方明确提供。
func validatePrecheckClaim(input PrecheckClaim) error {
	if !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.NodeID, 256) ||
		!validAgentOpaqueValue(input.PrecheckID, 256) || !validAgentOpaqueValue(input.LeaseID, 256) ||
		!validAgentOpaqueValue(input.RequestID, 256) || !isSHA256(input.RequestDigest) ||
		input.LeaseTTL <= 0 || input.Now.IsZero() {
		return errors.New("precheck claim is invalid")
	}
	return nil
}

// validatePrecheckClaimNext 只校验服务端已经生成的租约与 Agent 信封字段。
// 预检查标识必须由事务内查询得出，不能被调用方带入此入口。
func validatePrecheckClaimNext(input PrecheckClaimNext) error {
	if !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.NodeID, 256) ||
		!validAgentOpaqueValue(input.LeaseID, 256) || !validAgentOpaqueValue(input.RequestID, 256) ||
		!isSHA256(input.RequestDigest) || input.LeaseTTL <= 0 || input.Now.IsZero() {
		return errors.New("next precheck claim is invalid")
	}
	return nil
}

// validatePrecheckAcknowledgement 只允许确认当前租约和控制面签发的冻结摘要。
func validatePrecheckAcknowledgement(input PrecheckAcknowledgement) error {
	if !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.PrecheckID, 256) ||
		!validAgentOpaqueValue(input.LeaseID, 256) || input.LeaseEpoch < 1 ||
		!isSHA256(input.BindingDigest) || !validAgentOpaqueValue(input.RequestID, 256) ||
		!isSHA256(input.RequestDigest) || input.Now.IsZero() {
		return errors.New("precheck acknowledgement is invalid")
	}
	return nil
}

// validatePrecheckSecretResolutionRequest 限制秘密槽位请求只能复用当前预检查租约的非秘密标识。
// 秘密原值、凭据引用、路径、SQL 和命令都不属于该请求模型。
func validatePrecheckSecretResolutionRequest(input PrecheckSecretResolutionRequest) error {
	if !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.PrecheckID, 256) ||
		!validAgentOpaqueValue(input.LeaseID, 256) || input.LeaseEpoch < 1 ||
		!isSHA256(input.BindingDigest) || !validAgentOpaqueValue(input.RequestID, 256) ||
		!isSHA256(input.RequestDigest) || input.Now.IsZero() {
		return errors.New("precheck secret resolution request is invalid")
	}
	return nil
}

// validatePrecheckSecretResolutionOutcome 复用请求校验，并要求调用方明确记录短时解密是否成功。
// 它不接受任何明文字段，避免结果记录被误扩展为秘密传输通道。
func validatePrecheckSecretResolutionOutcome(input PrecheckSecretResolutionOutcome) error {
	return validatePrecheckSecretResolutionRequest(PrecheckSecretResolutionRequest{
		AgentID: input.AgentID, PrecheckID: input.PrecheckID, LeaseID: input.LeaseID,
		LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest, RequestID: input.RequestID,
		RequestDigest: input.RequestDigest, Now: input.Now,
	})
}

// validatePrecheckCompletion 只允许保存安全对象形式的检查摘要。
func validatePrecheckCompletion(input PrecheckCompletion) error {
	if input.PrecheckID == "" || input.CompletedAt.IsZero() || !oneOf(input.IntegrityStatus, "COMPLETE", "INCOMPLETE") {
		return errors.New("precheck completion identity is invalid")
	}
	if !input.Succeeded && input.IntegrityStatus != "COMPLETE" {
		return errors.New("failed precheck completion must be complete")
	}
	if input.Succeeded && input.IntegrityStatus != "COMPLETE" {
		return errors.New("successful precheck completion must be complete")
	}
	if err := validateSafeObjectJSON(input.ResultJSON); err != nil {
		return fmt.Errorf("precheck completion result is invalid: %w", err)
	}
	return nil
}

// validateAgentPrecheckCompletion 拒绝不完整、可携带自由文本或缺少租约绑定的正式完成请求。
func validateAgentPrecheckCompletion(input AgentPrecheckCompletion) error {
	if !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.PrecheckID, 256) ||
		!validAgentOpaqueValue(input.LeaseID, 256) || input.LeaseEpoch < 1 ||
		!isSHA256(input.BindingDigest) || !validAgentOpaqueValue(input.RequestID, 256) ||
		!isSHA256(input.RequestDigest) || input.Now.IsZero() || len(input.Results) != len(fixedPrecheckChecks) {
		return errors.New("agent precheck completion is invalid")
	}
	for index, check := range fixedPrecheckChecks {
		result := input.Results[index]
		if result.Check != check || !oneOf(result.Status, "PASSED", "FAILED", "UNKNOWN") || !precheckcontract.ValidResult(result.Check, result.Status, result.EvidenceCode) {
			return errors.New("agent precheck completion results are invalid")
		}
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func validateTaskSubmission(input TaskSubmission) error {
	if input.TaskID == "" || input.CreatorSubjectID == "" || input.AuditActorID == "" || input.DataSourceID == "" || input.NodeID == "" || input.PrecheckID == "" || input.CredentialID == "" || input.CredentialRevision < 1 || input.ToolVersion == "" || input.MetadataVersion == "" || input.CapabilityVersion == "" || input.RequestID == "" || input.SubmittedAt.IsZero() {
		return errors.New("task submission identity is invalid")
	}
	if !isSHA256(input.ConfigFingerprint) {
		return errors.New("task configuration fingerprint is invalid")
	}
	if err := validateSafeObjectJSON(input.SnapshotJSON); err != nil {
		return fmt.Errorf("task snapshot is invalid: %w", err)
	}
	if err := validateArgvJSON(input.PlannedArgvJSON); err != nil {
		return fmt.Errorf("planned argv is invalid: %w", err)
	}
	if strings.TrimSpace(input.PlannedCommandRedacted) == "" || strings.Contains(input.PlannedCommandRedacted, "--password") {
		return errors.New("planned command must be redacted")
	}
	return nil
}

func validateClaim(input Claim) error {
	if input.ExecutionID == "" || input.TaskID == "" || input.NodeID == "" || input.AgentID == "" || input.LeaseID == "" || input.EventID == "" || input.RequestID == "" || input.LeaseEpoch < 1 || input.IssuedAt.IsZero() || input.ExpiresAt.IsZero() || !input.ExpiresAt.After(input.IssuedAt) {
		return errors.New("task claim identity is invalid")
	}
	return nil
}

func validateExecutionClaimNext(input ExecutionClaimNext) error {
	if input.AgentID == "" || input.NodeID == "" || input.ExecutionID == "" || input.LeaseID == "" || input.ScheduledEventID == "" || input.RequestID == "" || input.LeaseTTL <= 0 || input.HeartbeatFreshAfter.IsZero() || input.Now.IsZero() {
		return errors.New("execution claim-next identity is invalid")
	}
	return nil
}

func validateExecutionSecretResolutionRequest(input ExecutionSecretResolutionRequest) error {
	if input.AgentID == "" || input.ExecutionID == "" || input.LeaseID == "" || input.LeaseEpoch < 1 || !isSHA256(input.EnvelopeDigest) || input.RequestID == "" || !isSHA256(input.RequestDigest) || input.Now.IsZero() {
		return errors.New("execution secret resolution request is invalid")
	}
	return nil
}

func validateExecutionSecretResolutionOutcome(input ExecutionSecretResolutionOutcome) error {
	if input.AgentID == "" || input.ExecutionID == "" || input.LeaseID == "" || input.LeaseEpoch < 1 || !isSHA256(input.EnvelopeDigest) || input.RequestID == "" || !isSHA256(input.RequestDigest) || input.Now.IsZero() {
		return errors.New("execution secret resolution outcome is invalid")
	}
	return nil
}

func validExecutionArgv(argv []string) bool {
	if len(argv) == 0 || len(argv) > 32 {
		return false
	}
	for _, value := range argv {
		if value == "" || len(value) > 4096 || strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") || value == "--password" || strings.HasPrefix(value, "--password=") || strings.HasPrefix(value, "-p") {
			return false
		}
	}
	return true
}

func executionEnvelopeDigest(taskID, nodeID, configFingerprint, toolVersion, metadataVersion, capabilityVersion string, argv []string) (string, error) {
	payload, err := json.Marshal(struct {
		TaskID            string   `json:"taskId"`
		NodeID            string   `json:"nodeId"`
		ConfigFingerprint string   `json:"configFingerprint"`
		ToolVersion       string   `json:"toolVersion"`
		MetadataVersion   string   `json:"metadataVersion"`
		CapabilityVersion string   `json:"capabilityVersion"`
		Argv              []string `json:"argv"`
	}{TaskID: taskID, NodeID: nodeID, ConfigFingerprint: configFingerprint, ToolVersion: toolVersion, MetadataVersion: metadataVersion, CapabilityVersion: capabilityVersion, Argv: argv})
	if err != nil {
		return "", fmt.Errorf("encode execution envelope: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// executionEnvelopeDigestForStoredTask 只从当前 execution 关联的不可变任务读取信封组成部分。
// 秘密解析前再次计算摘要，避免 Agent 用旧任务、旧节点或被替换的参数请求槽位。
func executionEnvelopeDigestForStoredTask(ctx context.Context, tx *sql.Tx, executionID string, argv []string) (string, error) {
	var taskID, nodeID, configFingerprint, toolVersion, metadataVersion, capabilityVersion string
	err := tx.QueryRowContext(ctx, `
        SELECT t.task_id, e.node_id, t.config_fingerprint, t.tool_version, t.metadata_version, t.capability_version
        FROM task_executions AS e
        JOIN tasks AS t ON t.task_id = e.task_id
        WHERE e.execution_id = ?
    `, executionID).Scan(&taskID, &nodeID, &configFingerprint, &toolVersion, &metadataVersion, &capabilityVersion)
	if err != nil {
		return "", err
	}
	return executionEnvelopeDigest(taskID, nodeID, configFingerprint, toolVersion, metadataVersion, capabilityVersion, argv)
}

func validateExecutionEvent(input ExecutionEvent) error {
	if input.EventID == "" || input.ExecutionID == "" || input.LeaseID == "" || input.LeaseEpoch < 1 || input.EventSeq < 1 || strings.TrimSpace(input.EventType) == "" || input.ReceivedAt.IsZero() {
		return errors.New("execution event identity is invalid")
	}
	if err := validateSafeObjectJSON(input.PayloadJSON); err != nil {
		return fmt.Errorf("execution event payload is invalid: %w", err)
	}
	return nil
}

func validateSafeObjectJSON(raw string) error {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return errors.New("must be valid JSON")
	}
	if _, isObject := value.(map[string]any); !isObject {
		return errors.New("must be a JSON object")
	}
	if containsSecretValueKey(value) {
		return errors.New("must not contain a secret value")
	}
	return nil
}

func validateArgvJSON(raw string) error {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return errors.New("must be a JSON string array")
	}
	if values == nil {
		return errors.New("must be a JSON string array")
	}
	for _, value := range values {
		if value == "" || value == "--password" {
			return errors.New("contains a forbidden command token")
		}
	}
	return nil
}

func containsSecretValueKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			switch strings.ToLower(key) {
			case "password", "passwordvalue", "plaintext", "secretvalue", "credentialvalue", "tokenvalue":
				return true
			}
			if containsSecretValueKey(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsSecretValueKey(nested) {
				return true
			}
		}
	}
	return false
}

func verifyBackup(ctx context.Context, backupPath string) error {
	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(backupPath))
	if err != nil {
		return fmt.Errorf("open SQLite backup: %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("check SQLite backup: %w", err)
	}
	if !strings.EqualFold(result, "ok") {
		return errors.New("SQLite backup integrity check failed")
	}
	var migrationCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		return fmt.Errorf("read SQLite backup migration history: %w", err)
	}
	if migrationCount == 0 {
		return errors.New("SQLite backup has no migration history")
	}
	return nil
}

func sqliteDSN(path, mode string) string {
	options := sqlitePragmas
	if mode != "" {
		options = mode + "&" + options
	}
	return "file:" + filepath.ToSlash(path) + "?" + options
}

func sqliteReadOnlyDSN(path string) string {
	return "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
}

func isSHA256(value string) bool {
	return sha256Pattern.MatchString(value)
}

func utcText(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func auditID(objectID, requestID string) string {
	return "audit-" + objectID + "-" + requestID
}

func isTaskExecutionUnique(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed: task_executions.task_id")
}

func isDataSourceNameConstraint(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed: data_sources.normalized_name")
}

func isExecutionNodeNameConstraint(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed: execution_nodes.normalized_name")
}

func isExecutionEventConstraint(err error) bool {
	message := err.Error()
	return strings.Contains(message, "UNIQUE constraint failed: execution_events.execution_id, execution_events.event_seq") ||
		strings.Contains(message, "FOREIGN KEY constraint failed")
}
