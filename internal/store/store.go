package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/migrate"

	_ "modernc.org/sqlite"
)

const sqlitePragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

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

// ListDataSourceSummaries returns only the API-safe data-source projection.
// Authorization remains a control-plane concern, while this query guarantees
// that the database's encrypted credential material never enters the result.
func (s *Store) ListDataSourceSummaries(ctx context.Context) ([]DataSourceSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("SQLite store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT data_source_id, display_name, environment, connection_kind,
               compatibility_mode, host, port, username,
               COALESCE(default_database, ''), state, revision,
               current_credential_revision, COALESCE(last_test_status, ''),
               last_tested_at, COALESCE(last_test_safe_summary_json, ''), updated_at
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
			&summary.CompatibilityMode, &summary.Host, &summary.Port, &summary.Username,
			&summary.DefaultDatabase, &summary.State, &summary.Revision, &summary.CredentialRevision,
			&summary.LastTestStatus, &lastTestedAt, &summary.LastTestSafeSummaryJSON, &updatedAt,
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

// GetDataSourceSummary returns the same non-sensitive projection as the list
// API for one active source. Archived sources intentionally behave as absent.
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
               compatibility_mode, host, port, username,
               COALESCE(default_database, ''), state, revision,
               current_credential_revision, COALESCE(last_test_status, ''),
               last_tested_at, COALESCE(last_test_safe_summary_json, ''), updated_at
        FROM data_sources
        WHERE data_source_id = ? AND state != 'ARCHIVED'
    `, dataSourceID).Scan(
		&summary.DataSourceID, &summary.DisplayName, &summary.Environment, &summary.ConnectionKind,
		&summary.CompatibilityMode, &summary.Host, &summary.Port, &summary.Username,
		&summary.DefaultDatabase, &summary.State, &summary.Revision, &summary.CredentialRevision,
		&summary.LastTestStatus, &lastTestedAt, &summary.LastTestSafeSummaryJSON, &updatedAt,
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

// CreateDataSource atomically writes the source, its encrypted first credential
// revision, a minimal audit fact, and a 24-hour idempotency result. Any error
// rolls back every record so no usable source can exist without its credential.
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
                compatibility_mode, host, port, username, default_database, credential_id,
                current_credential_revision, state, revision, last_test_status, last_tested_at,
                last_test_safe_summary_json, created_by, created_at, updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'ENABLED', 1, NULL, NULL, NULL, ?, ?, ?)
        `, input.DataSourceID, input.DisplayName, input.NormalizedName, input.Environment,
			input.ConnectionKind, input.CompatibilityMode, input.Host, input.Port, input.Username,
			nullableString(input.DefaultDatabase), input.CredentialID, input.CreatorSubjectID,
			utcText(input.CreatedAt), utcText(input.CreatedAt)); err != nil {
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

// ChangeDataSourceState 在短事务中原子切换启用或禁用状态。
// 同一目标状态会返回当前 revision，且不会重复写入审计事件。
func (s *Store) ChangeDataSourceState(ctx context.Context, input DataSourceStateChange) (DataSourceStateChangeResult, error) {
	if err := validateDataSourceStateChange(input); err != nil {
		return DataSourceStateChangeResult{}, err
	}
	result := DataSourceStateChangeResult{}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var currentState string
		var currentRevision int64
		err := tx.QueryRowContext(ctx, `
            SELECT state, revision
            FROM data_sources
            WHERE data_source_id = ? AND state != 'ARCHIVED'
        `, input.DataSourceID).Scan(&currentState, &currentRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDataSourceNotFound
		}
		if err != nil {
			return fmt.Errorf("read data source state: %w", err)
		}
		if currentState == input.TargetState {
			result.State, result.Revision, result.Replayed = currentState, currentRevision, true
			return nil
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
	return s.withWrite(ctx, func(tx *sql.Tx) error {
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
	})
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
	if input.DataSourceID == "" || input.CredentialID == "" || input.CreatorSubjectID == "" || input.DisplayName == "" || input.NormalizedName == "" || input.Host == "" || input.Username == "" || input.KeyID == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.CreatedAt.IsZero() || input.Port < 1 || input.Port > 65535 {
		return errors.New("data source create identity is invalid")
	}
	if !isSHA256(input.RequestDigest) || len(input.Nonce) == 0 || len(input.Ciphertext) == 0 {
		return errors.New("data source create security material is invalid")
	}
	if !oneOf(input.Environment, "DEVELOPMENT", "TEST", "STAGING", "PRODUCTION") || !oneOf(input.ConnectionKind, "OBSERVER_DIRECT", "ODP", "PUBLIC_CLOUD", "LOGICAL_DATABASE") || !oneOf(input.CompatibilityMode, "MYSQL", "ORACLE", "UNKNOWN") {
		return errors.New("data source create enum is invalid")
	}
	return nil
}

// validateDataSourceStateChange 将状态动作限制为产品已确认的两个可逆状态。
// 归档属于独立删除/归档流程，不能通过此入口触发。
func validateDataSourceStateChange(input DataSourceStateChange) error {
	if input.DataSourceID == "" || input.ActorSubjectID == "" || input.RequestID == "" || input.ChangedAt.IsZero() {
		return errors.New("data source state change identity is invalid")
	}
	if !oneOf(input.TargetState, "ENABLED", "DISABLED") {
		return errors.New("data source state change target is invalid")
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

func isExecutionEventConstraint(err error) bool {
	message := err.Error()
	return strings.Contains(message, "UNIQUE constraint failed: execution_events.execution_id, execution_events.event_seq") ||
		strings.Contains(message, "FOREIGN KEY constraint failed")
}
