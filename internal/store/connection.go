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
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/catalogresult"
)

// ExportObjectCatalogClaimTimeout 限制对象查询在 Agent 未领取时的排队时间；领取后的 JDBC 执行仍使用原租约期限。
const ExportObjectCatalogClaimTimeout = 30 * time.Second

// RequestDataSourceConnectionTest 在一个短事务内冻结指定节点 Agent 的基础连接测试意图。
// 它不会建立数据库连接、解析密码或启动 JDBC；这些动作只能由后续持有租约的 Agent 执行。
func (s *Store) RequestDataSourceConnectionTest(ctx context.Context, input DataSourceConnectionTestCreate) (DataSourceConnectionTestCreateResult, error) {
	if input.OperationKind == "" {
		input.OperationKind = "CONNECTION_TEST"
	}
	if err := validateDataSourceConnectionTestCreate(input); err != nil {
		return DataSourceConnectionTestCreateResult{}, err
	}
	result := DataSourceConnectionTestCreateResult{ConnectionTestID: input.ConnectionTestID}
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var digest, existingID string
		err := tx.QueryRowContext(ctx, `
            SELECT request_digest, COALESCE(resource_id, '')
            FROM request_idempotency
            WHERE subject_id = ?
              AND operation = 'CREATE_DATA_SOURCE_CONNECTION_TEST'
              AND idempotency_key = ?
        `, input.CreatorSubjectID, input.IdempotencyKey).Scan(&digest, &existingID)
		if err == nil {
			if digest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			result.ConnectionTestID, result.Replayed = existingID, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read data source connection test idempotency: %w", err)
		}

		var currentRevision int64
		err = tx.QueryRowContext(ctx, `
            SELECT revision
            FROM data_sources
            WHERE data_source_id = ? AND state != 'ARCHIVED'
        `, input.DataSourceID).Scan(&currentRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDataSourceNotFound
		}
		if err != nil {
			return fmt.Errorf("read data source connection test revision: %w", err)
		}
		if currentRevision != input.ExpectedDataSourceRevision {
			return ErrRevisionConflict
		}

		binding, err := readCurrentDataSourceConnectionTestBinding(ctx, tx, input)
		if err != nil {
			return err
		}
		bindingDigest, err := dataSourceConnectionTestBindingDigest(binding)
		if err != nil {
			return err
		}
		binding.BindingDigest = bindingDigest

		insert, err := tx.ExecContext(ctx, `
            INSERT INTO data_source_connection_test_runs(
                connection_test_id, data_source_id, creator_subject_id, connection_config_digest,
                credential_id, credential_revision, node_id, binding_agent_id, node_facts_revision,
                binding_digest, status, verification_source, lease_id, lease_epoch, lease_expires_at,
                result_code, safe_summary_json, created_at, completed_at, valid_until,
                sys_credential_id, sys_credential_revision,
				operation_kind, catalog_database, catalog_compatibility_mode, catalog_object_type, catalog_extended_object_type, catalog_keyword
            )
            SELECT ?, ds.data_source_id, ?, ?, ds.credential_id, ds.current_credential_revision,
                   ?, ?, ?, ?, 'PENDING', ?, NULL, NULL, NULL, NULL, NULL, ?, NULL, ?,
                   ?, ?,
				   ?, ?, ?, ?, ?, ?
            FROM data_sources AS ds
            JOIN credential_revisions AS cr
              ON cr.credential_id = ds.credential_id
             AND cr.revision = ds.current_credential_revision
            JOIN execution_nodes AS n ON n.node_id = ?
            JOIN agents AS a ON a.agent_id = ? AND a.node_id = n.node_id
            JOIN auth_subjects AS subject ON subject.subject_id = ?
            WHERE ds.data_source_id = ?
              AND ds.revision = ?
              AND ds.state != 'ARCHIVED'
              AND cr.status = 'ACTIVE'
              AND n.management_state IN ('ENABLED', 'DISABLED')
              AND a.status = 'ACTIVE'
              AND a.facts_revision = ?
              AND subject.account_status = 'ACTIVE'
        `,
			input.ConnectionTestID, input.CreatorSubjectID, binding.ConnectionConfigDigest,
			binding.NodeID, binding.BindingAgentID, binding.NodeFactsRevision, binding.BindingDigest,
			binding.VerificationSource, utcText(input.CreatedAt), utcText(binding.ValidUntil),
			nullableString(binding.SysCredentialID), nullableInt64(binding.SysCredentialRevision),
			binding.OperationKind, nullableString(binding.CatalogDatabase), nullableString(binding.CatalogCompatibilityMode), storedCatalogObjectType(binding), storedExtendedCatalogObjectType(binding), nullableString(binding.CatalogKeyword),
			binding.NodeID, binding.BindingAgentID, input.CreatorSubjectID, input.DataSourceID,
			input.ExpectedDataSourceRevision, binding.NodeFactsRevision,
		)
		if err != nil {
			return fmt.Errorf("insert data source connection test: %w", err)
		}
		affected, err := insert.RowsAffected()
		if err != nil {
			return fmt.Errorf("read data source connection test insert result: %w", err)
		}
		if affected != 1 {
			return ErrDataSourceConnectionTestInvalid
		}
		if err := insertAudit(ctx, tx, "SUBJECT", input.CreatorSubjectID, "DATA_SOURCE_CONNECTION_TEST_REQUESTED", "DATA_SOURCE_CONNECTION_TEST", input.ConnectionTestID, "SUCCEEDED", input.RequestID, input.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO request_idempotency(
                subject_id, operation, idempotency_key, request_digest, result_status,
                resource_kind, resource_id, response_json, created_at, expires_at
            ) VALUES (?, 'CREATE_DATA_SOURCE_CONNECTION_TEST', ?, ?, 202,
                      'DATA_SOURCE_CONNECTION_TEST', ?, '{}', ?, ?)
        `, input.CreatorSubjectID, input.IdempotencyKey, input.RequestDigest, input.ConnectionTestID,
			utcText(input.CreatedAt), utcText(input.CreatedAt.Add(24*time.Hour))); err != nil {
			return fmt.Errorf("write data source connection test idempotency: %w", err)
		}
		return nil
	})
	return result, err
}

// GetDataSourceConnectionTestRun 返回连接测试的无秘密状态投影。
// 对象访问范围仍由控制面在调用 Store 前校验，归档数据源的历史运行记录不会通过此方法暴露给浏览器。
func (s *Store) GetDataSourceConnectionTestRun(ctx context.Context, connectionTestID string) (DataSourceConnectionTestRun, error) {
	if s == nil || s.db == nil {
		return DataSourceConnectionTestRun{}, errors.New("SQLite store is nil")
	}
	if !validAgentOpaqueValue(connectionTestID, 256) {
		return DataSourceConnectionTestRun{}, ErrDataSourceConnectionTestInvalid
	}
	run, err := readStoredDataSourceConnectionTest(ctx, s.db, connectionTestID)
	if errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
		return DataSourceConnectionTestRun{}, ErrDataSourceConnectionTestInvalid
	}
	if err != nil {
		return DataSourceConnectionTestRun{}, err
	}
	return DataSourceConnectionTestRun{
		ConnectionTestID:       run.Binding.ConnectionTestID,
		DataSourceID:           run.Binding.DataSourceID,
		CreatorSubjectID:       run.CreatorSubjectID,
		ConnectionConfigDigest: run.Binding.ConnectionConfigDigest,
		CredentialID:           run.Binding.CredentialID,
		CredentialRevision:     run.Binding.CredentialRevision,
		NodeID:                 run.Binding.NodeID,
		NodeFactsRevision:      run.Binding.NodeFactsRevision,
		BindingAgentID:         run.Binding.BindingAgentID,
		BindingDigest:          run.Binding.BindingDigest,
		Status:                 run.Status,
		VerificationSource:     run.Binding.VerificationSource,
		ResultCode:             run.ResultCode,
		SafeSummaryJSON:        run.SafeSummaryJSON,
		SysCredentialID:        run.Binding.SysCredentialID,
		SysCredentialRevision:  run.Binding.SysCredentialRevision,
		SysVerificationStatus:  run.SysVerificationStatus,
		SysResultCode:          run.SysResultCode,
		OperationKind:          run.Binding.OperationKind,
		CatalogDatabase:        run.Binding.CatalogDatabase,
		CatalogObjectType:      run.Binding.CatalogObjectType,
		CatalogKeyword:         run.Binding.CatalogKeyword,
		CatalogObjects:         append([]string(nil), run.CatalogObjects...),
		CatalogGroups:          append([]catalogresult.Group(nil), run.CatalogGroups...),
		CatalogTruncated:       run.CatalogTruncated,
		ValidUntil:             run.Binding.ValidUntil,
		CreatedAt:              run.CreatedAt,
		CompletedAt:            run.CompletedAt,
	}, nil
}

// ClaimNextDataSourceConnectionTest 由控制面在单个短事务内领取当前 Agent 的下一条基础连接测试。
// 请求不能携带测试标识，因而 Agent 无法借此探测或选择其他节点、数据源或凭据。
func (s *Store) ClaimNextDataSourceConnectionTest(ctx context.Context, input DataSourceConnectionTestClaimNext) (DataSourceConnectionTestLeaseGrant, bool, error) {
	if err := validateDataSourceConnectionTestClaimNext(input); err != nil {
		return DataSourceConnectionTestLeaseGrant{}, false, err
	}
	var grant DataSourceConnectionTestLeaseGrant
	var found bool
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expireDataSourceConnectionTestsTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, replayed, err := readAgentDataSourceConnectionTestReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if replayed {
			if receipt.Operation != "CLAIM" || receipt.RequestDigest != input.RequestDigest {
				return ErrIdempotencyConflict
			}
			grant, err = replayDataSourceConnectionTestLeaseGrant(ctx, tx, receipt, input.Now)
			if err == nil {
				found = true
			}
			if errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) || errors.Is(err, ErrDataSourceConnectionTestLeaseExpired) {
				outcomeErr = err
				return nil
			}
			return err
		}

		var connectionTestID string
		err = tx.QueryRowContext(ctx, `
            SELECT connection_test_id
            FROM data_source_connection_test_runs
            WHERE status = 'PENDING'
              AND node_id = ?
              AND binding_agent_id = ?
              AND valid_until > ?
            ORDER BY created_at ASC, connection_test_id ASC
            LIMIT 1
        `, input.NodeID, input.AgentID, utcText(input.Now)).Scan(&connectionTestID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find next data source connection test: %w", err)
		}
		grant, outcomeErr = claimDataSourceConnectionTestTx(ctx, tx, connectionTestID, input)
		if outcomeErr == nil {
			found = true
		}
		if errors.Is(outcomeErr, ErrDataSourceConnectionTestLeaseRejected) || errors.Is(outcomeErr, ErrDataSourceConnectionTestLeaseExpired) {
			return nil
		}
		return outcomeErr
	})
	if err != nil {
		return grant, found, err
	}
	return grant, found, outcomeErr
}

// AcknowledgeDataSourceConnectionTest 持久化 Agent 对当前固定连接测试租约的确认。
// 确认不接受任意 Probe 参数，只有后续受控槽位解析和固定 JDBC 探针可以使用该租约。
func (s *Store) AcknowledgeDataSourceConnectionTest(ctx context.Context, input DataSourceConnectionTestAcknowledgement) (DataSourceConnectionTestLeaseGrant, error) {
	if err := validateDataSourceConnectionTestAcknowledgement(input); err != nil {
		return DataSourceConnectionTestLeaseGrant{}, err
	}
	var grant DataSourceConnectionTestLeaseGrant
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expireDataSourceConnectionTestsTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readAgentDataSourceConnectionTestReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if found && (receipt.Operation != "ACKNOWLEDGE" || receipt.RequestDigest != input.RequestDigest ||
			receipt.ConnectionTestID != input.ConnectionTestID || receipt.LeaseID != input.LeaseID ||
			receipt.LeaseEpoch != input.LeaseEpoch || receipt.BindingDigest != input.BindingDigest) {
			return ErrIdempotencyConflict
		}

		run, err := readStoredDataSourceConnectionTest(ctx, tx, input.ConnectionTestID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" {
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		if !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if err := markDataSourceConnectionTestExpiredTx(ctx, tx, run.Binding.ConnectionTestID, input.Now); err != nil {
				return err
			}
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if err := validateOrInvalidateDataSourceConnectionTestBinding(ctx, tx, input.AgentID, run, input.Now); err != nil {
			if errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
				outcomeErr = err
				return nil
			}
			return err
		}
		if found {
			grant = DataSourceConnectionTestLeaseGrant{
				ConnectionTestID: receipt.ConnectionTestID, LeaseID: receipt.LeaseID, LeaseEpoch: receipt.LeaseEpoch,
				ExpiresAt: receipt.ExpiresAt, Binding: run.Binding, Replayed: true,
			}
			return nil
		}
		receipt = agentDataSourceConnectionTestReceipt{
			AgentID: input.AgentID, RequestID: input.RequestID, Operation: "ACKNOWLEDGE", RequestDigest: input.RequestDigest,
			ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: "ACKNOWLEDGED", ExpiresAt: run.LeaseExpiresAt, CreatedAt: input.Now,
		}
		if err := insertAgentDataSourceConnectionTestReceipt(ctx, tx, receipt); err != nil {
			return err
		}
		grant = DataSourceConnectionTestLeaseGrant{
			ConnectionTestID: run.Binding.ConnectionTestID, LeaseID: run.LeaseID, LeaseEpoch: run.LeaseEpoch,
			ExpiresAt: run.LeaseExpiresAt, Binding: run.Binding,
		}
		return nil
	})
	if err != nil {
		return grant, err
	}
	return grant, outcomeErr
}

// ResolveDataSourceConnectionTestDatabaseConnection 在有效且已确认的连接测试租约内返回加密连接材料。
// 该方法只写入无秘密回执和审计意图；实际 AES-GCM 解密必须在事务外的受控控制面服务中短时完成。
func (s *Store) ResolveDataSourceConnectionTestDatabaseConnection(ctx context.Context, input DataSourceConnectionTestSecretResolutionRequest) (EncryptedDataSourceConnectionTestDatabaseConnection, error) {
	if err := validateDataSourceConnectionTestSecretResolutionRequest(input); err != nil {
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, err
	}
	var connection EncryptedDataSourceConnectionTestDatabaseConnection
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expireDataSourceConnectionTestsTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if found && !sameDataSourceConnectionTestSecretResolutionReceipt(receipt, input) {
			return ErrIdempotencyConflict
		}

		run, err := readStoredDataSourceConnectionTest(ctx, tx, input.ConnectionTestID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" {
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		if !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if err := markDataSourceConnectionTestExpiredTx(ctx, tx, run.Binding.ConnectionTestID, input.Now); err != nil {
				return err
			}
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if err := validateOrInvalidateDataSourceConnectionTestBinding(ctx, tx, input.AgentID, run, input.Now); err != nil {
			if errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
				outcomeErr = err
				return nil
			}
			return err
		}
		acknowledged, err := hasDataSourceConnectionTestAcknowledgementReceipt(ctx, tx, input.AgentID, run)
		if err != nil {
			return err
		}
		if !acknowledged {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		connection, err = readEncryptedDataSourceConnectionTestDatabaseConnection(ctx, tx, run)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
		if err := insertDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, dataSourceConnectionTestSecretResolutionReceipt{
			AgentID: input.AgentID, RequestID: input.RequestID, RequestDigest: input.RequestDigest,
			ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: "AUTHORIZED", CreatedAt: input.Now,
		}); err != nil {
			connection.Destroy()
			return err
		}
		if err := insertDataSourceConnectionTestSecretResolutionAudit(ctx, tx, input.AgentID, input.ConnectionTestID, input.RequestID, "DATA_SOURCE_CONNECTION_TEST_SECRET_RESOLVE_REQUESTED", "SUCCEEDED", input.Now); err != nil {
			connection.Destroy()
			return err
		}
		return nil
	})
	if err != nil {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, err
	}
	if outcomeErr != nil {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, outcomeErr
	}
	return connection, nil
}

// ResolveDataSourceConnectionTestSysCredential 在有效且已确认的连接测试租约内返回加密 sys 凭据材料。
// 仅当绑定冻结了 sys 凭据引用（SysCredentialRevision > 0）时可解析；未配置 sys 凭据时失败关闭。
// 该方法只写入无秘密回执和审计意图；实际 AES-GCM 解密必须在事务外的受控控制面服务中短时完成。
func (s *Store) ResolveDataSourceConnectionTestSysCredential(ctx context.Context, input DataSourceConnectionTestSecretResolutionRequest) (EncryptedDataSourceConnectionTestDatabaseConnection, error) {
	if err := validateDataSourceConnectionTestSecretResolutionRequest(input); err != nil {
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, err
	}
	var connection EncryptedDataSourceConnectionTestDatabaseConnection
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expireDataSourceConnectionTestsTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if found && !sameDataSourceConnectionTestSecretResolutionReceipt(receipt, input) {
			return ErrIdempotencyConflict
		}
		run, err := readStoredDataSourceConnectionTest(ctx, tx, input.ConnectionTestID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" {
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		if !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if err := markDataSourceConnectionTestExpiredTx(ctx, tx, run.Binding.ConnectionTestID, input.Now); err != nil {
				return err
			}
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if err := validateOrInvalidateDataSourceConnectionTestBinding(ctx, tx, input.AgentID, run, input.Now); err != nil {
			if errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
				outcomeErr = err
				return nil
			}
			return err
		}
		acknowledged, err := hasDataSourceConnectionTestAcknowledgementReceipt(ctx, tx, input.AgentID, run)
		if err != nil {
			return err
		}
		if !acknowledged {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		// 仅当绑定冻结了 sys 凭据引用时可解析 sys 槽位（可选增强，参考 ODC 的 sys 账号验证）。
		if run.Binding.SysCredentialRevision < 1 {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		connection, err = readEncryptedDataSourceConnectionTestSysCredential(ctx, tx, run)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
		if err := insertDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, dataSourceConnectionTestSecretResolutionReceipt{
			AgentID: input.AgentID, RequestID: input.RequestID, RequestDigest: input.RequestDigest,
			ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: "AUTHORIZED", CreatedAt: input.Now,
		}); err != nil {
			connection.Destroy()
			return err
		}
		if err := insertDataSourceConnectionTestSecretResolutionAudit(ctx, tx, input.AgentID, input.ConnectionTestID, input.RequestID, "DATA_SOURCE_CONNECTION_TEST_SYS_SECRET_RESOLVE_REQUESTED", "SUCCEEDED", input.Now); err != nil {
			connection.Destroy()
			return err
		}
		return nil
	})
	if err != nil {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, err
	}
	if outcomeErr != nil {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, outcomeErr
	}
	return connection, nil
}

// FinishDataSourceConnectionTestSecretResolution 记录短时解密结束后的无秘密结果。
// 即使是重放或解密失败，也会先重新核验当前冻结绑定，避免失效连接测试继续保存为可执行状态。
func (s *Store) FinishDataSourceConnectionTestSecretResolution(ctx context.Context, input DataSourceConnectionTestSecretResolutionOutcome) error {
	if err := validateDataSourceConnectionTestSecretResolutionOutcome(input); err != nil {
		return err
	}
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		receipt, found, err := readDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if !found || !sameDataSourceConnectionTestSecretResolutionReceipt(receipt, DataSourceConnectionTestSecretResolutionRequest{
			AgentID: input.AgentID, ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID,
			LeaseEpoch: input.LeaseEpoch, BindingDigest: input.BindingDigest, RequestID: input.RequestID,
			RequestDigest: input.RequestDigest,
		}) {
			return ErrDataSourceConnectionTestLeaseRejected
		}

		if _, err := expireDataSourceConnectionTestsTx(ctx, tx, input.Now); err != nil {
			return err
		}
		run, err := readStoredDataSourceConnectionTest(ctx, tx, input.ConnectionTestID)
		if err != nil {
			return err
		}
		if run.Status == "EXPIRED" || run.Status != "LEASED" || run.AgentID != input.AgentID ||
			run.LeaseID != input.LeaseID || run.LeaseEpoch != input.LeaseEpoch ||
			run.Binding.BindingDigest != input.BindingDigest || !run.Binding.ValidUntil.After(input.Now) ||
			!run.LeaseExpiresAt.After(input.Now) {
			if receipt.Status != "FAILED" && receipt.Status != "RESOLVED" {
				if err := updateDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "FAILED", input.Now); err != nil {
					return err
				}
				if err := insertDataSourceConnectionTestSecretResolutionAudit(ctx, tx, input.AgentID, input.ConnectionTestID, input.RequestID, "DATA_SOURCE_CONNECTION_TEST_SECRET_RESOLVE_DENIED", "DENIED", input.Now); err != nil {
					return err
				}
			}
			if run.Status == "EXPIRED" || !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
				outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			} else {
				outcomeErr = ErrDataSourceConnectionTestLeaseRejected
			}
			return nil
		}
		if err := validateOrInvalidateDataSourceConnectionTestBinding(ctx, tx, input.AgentID, run, input.Now); err != nil {
			if receipt.Status != "FAILED" && receipt.Status != "RESOLVED" {
				if updateErr := updateDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "FAILED", input.Now); updateErr != nil {
					return updateErr
				}
				if auditErr := insertDataSourceConnectionTestSecretResolutionAudit(ctx, tx, input.AgentID, input.ConnectionTestID, input.RequestID, "DATA_SOURCE_CONNECTION_TEST_SECRET_RESOLVE_DENIED", "DENIED", input.Now); auditErr != nil {
					return auditErr
				}
			}
			if errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
				outcomeErr = err
				return nil
			}
			return err
		}
		acknowledged, err := hasDataSourceConnectionTestAcknowledgementReceipt(ctx, tx, input.AgentID, run)
		if err != nil {
			return err
		}
		if !acknowledged {
			return ErrDataSourceConnectionTestLeaseRejected
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
			if err := updateDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "FAILED", input.Now); err != nil {
				return err
			}
			return insertDataSourceConnectionTestSecretResolutionAudit(ctx, tx, input.AgentID, input.ConnectionTestID, input.RequestID, "DATA_SOURCE_CONNECTION_TEST_SECRET_RESOLVE_FAILED", "FAILED", input.Now)
		}
		if err := updateDataSourceConnectionTestSecretResolutionReceipt(ctx, tx, input.AgentID, input.RequestID, "RESOLVED", input.Now); err != nil {
			return err
		}
		return insertDataSourceConnectionTestSecretResolutionAudit(ctx, tx, input.AgentID, input.ConnectionTestID, input.RequestID, "DATA_SOURCE_CONNECTION_TEST_SECRET_RESOLVED", "SUCCEEDED", input.Now)
	})
	if err != nil {
		return err
	}
	return outcomeErr
}

// CompleteAgentDataSourceConnectionTest 只接受当前、已确认且未过期租约的固定连接测试终态。
// G2 合成成功只完成运行记录；只有未来 AGENT_JDBC 结果才可原子写回数据源的基础连接测试事实。
func (s *Store) CompleteAgentDataSourceConnectionTest(ctx context.Context, input AgentDataSourceConnectionTestCompletion) (DataSourceConnectionTestCompletionResult, error) {
	if err := validateAgentDataSourceConnectionTestCompletion(input); err != nil {
		return DataSourceConnectionTestCompletionResult{}, err
	}
	var result DataSourceConnectionTestCompletionResult
	var outcomeErr error
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := expireDataSourceConnectionTestsTx(ctx, tx, input.Now); err != nil {
			return err
		}
		receipt, found, err := readAgentDataSourceConnectionTestReceipt(ctx, tx, input.AgentID, input.RequestID)
		if err != nil {
			return err
		}
		if found && (receipt.Operation != "COMPLETE" || receipt.RequestDigest != input.RequestDigest ||
			receipt.ConnectionTestID != input.ConnectionTestID || receipt.LeaseID != input.LeaseID ||
			receipt.LeaseEpoch != input.LeaseEpoch || receipt.BindingDigest != input.BindingDigest ||
			(receipt.Status != input.Status && receipt.Status != "EXPIRED")) {
			return ErrIdempotencyConflict
		}

		run, err := readStoredDataSourceConnectionTest(ctx, tx, input.ConnectionTestID)
		if err != nil {
			return err
		}
		if input.VerificationSource != run.Binding.VerificationSource {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		if run.Binding.OperationKind == "EXPORT_OBJECT_CATALOG" {
			if input.SysVerificationStatus != "NOT_CONFIGURED" || input.SysResultCode != "" ||
				!validCatalogResult(input.Status, input.CatalogObjects, input.CatalogGroups, input.CatalogTruncated, run.Binding.CatalogObjectType) {
				return ErrDataSourceConnectionTestLeaseRejected
			}
		} else if len(input.CatalogObjects) > 0 || len(input.CatalogGroups) > 0 || input.CatalogTruncated {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		hasSysCredential := run.Binding.SysCredentialID != "" && run.Binding.SysCredentialRevision > 0
		if hasSysCredential == (input.SysVerificationStatus == "NOT_CONFIGURED") {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		if found {
			if receipt.Status == "EXPIRED" {
				if run.Status != "EXPIRED" {
					return ErrDataSourceConnectionTestLeaseRejected
				}
				result = DataSourceConnectionTestCompletionResult{
					ConnectionTestID: receipt.ConnectionTestID, LeaseID: receipt.LeaseID, LeaseEpoch: receipt.LeaseEpoch,
					BindingDigest: receipt.BindingDigest, Status: "EXPIRED", EvidenceCode: "LEASE_EXPIRED",
					VerificationSource: run.Binding.VerificationSource, SysVerificationStatus: run.SysVerificationStatus,
					SysResultCode: run.SysResultCode, CompletedAt: receipt.CompletedAt, Replayed: true,
				}
				outcomeErr = ErrDataSourceConnectionTestLeaseExpired
				return nil
			}
			if run.Status != receipt.Status || run.AgentID != receipt.AgentID || run.LeaseID != receipt.LeaseID ||
				run.LeaseEpoch != receipt.LeaseEpoch || run.Binding.BindingDigest != receipt.BindingDigest {
				return ErrDataSourceConnectionTestLeaseRejected
			}
			result = DataSourceConnectionTestCompletionResult{
				ConnectionTestID: receipt.ConnectionTestID, LeaseID: receipt.LeaseID, LeaseEpoch: receipt.LeaseEpoch,
				BindingDigest: receipt.BindingDigest, Status: receipt.Status, EvidenceCode: run.ResultCode,
				VerificationSource: run.Binding.VerificationSource, SysVerificationStatus: run.SysVerificationStatus,
				SysResultCode: run.SysResultCode, CompletedAt: receipt.CompletedAt, Replayed: true,
			}
			return nil
		}
		if run.Status == "EXPIRED" {
			result, err = recordExpiredDataSourceConnectionTestCompletion(ctx, tx, input, run)
			if err != nil {
				return err
			}
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if run.Status != "LEASED" || run.AgentID != input.AgentID || run.LeaseID != input.LeaseID ||
			run.LeaseEpoch != input.LeaseEpoch || run.Binding.BindingDigest != input.BindingDigest {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		if !run.Binding.ValidUntil.After(input.Now) || !run.LeaseExpiresAt.After(input.Now) {
			if err := markDataSourceConnectionTestExpiredTx(ctx, tx, run.Binding.ConnectionTestID, input.Now); err != nil {
				return err
			}
			result, err = recordExpiredDataSourceConnectionTestCompletion(ctx, tx, input, run)
			if err != nil {
				return err
			}
			outcomeErr = ErrDataSourceConnectionTestLeaseExpired
			return nil
		}
		if err := validateOrInvalidateDataSourceConnectionTestBinding(ctx, tx, input.AgentID, run, input.Now); err != nil {
			if errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
				outcomeErr = err
				return nil
			}
			return err
		}
		acknowledged, err := hasDataSourceConnectionTestAcknowledgementReceipt(ctx, tx, input.AgentID, run)
		if err != nil {
			return err
		}
		if !acknowledged {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		safeSummaryJSON, err := dataSourceConnectionTestSafeSummaryJSON(input.EvidenceCode, input.VerificationSource)
		if err != nil {
			return err
		}
		var catalogJSON any
		if run.Binding.OperationKind == "EXPORT_OBJECT_CATALOG" && input.Status == "SUCCEEDED" {
			var payload any = input.CatalogObjects
			if run.Binding.CatalogObjectType == "ALL" {
				payload = input.CatalogGroups
			}
			if payload == nil {
				payload = []string{}
			}
			encoded, encodeErr := json.Marshal(payload)
			if encodeErr != nil {
				return encodeErr
			}
			catalogJSON = string(encoded)
		}
		updated, err := tx.ExecContext(ctx, `
            UPDATE data_source_connection_test_runs
            SET status = ?, result_code = ?, safe_summary_json = ?, completed_at = ?,
                sys_verification_status = ?, sys_result_code = ?, catalog_objects_json = ?, catalog_truncated = ?
            WHERE connection_test_id = ? AND status = 'LEASED'
        `, input.Status, input.EvidenceCode, safeSummaryJSON, utcText(input.Now),
			nullableString(input.SysVerificationStatus), nullableString(input.SysResultCode), catalogJSON, input.CatalogTruncated, input.ConnectionTestID)
		if err != nil {
			return fmt.Errorf("complete data source connection test: %w", err)
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("read data source connection test completion result: %w", err)
		}
		if affected != 1 {
			return ErrDataSourceConnectionTestLeaseRejected
		}
		if input.VerificationSource == "AGENT_JDBC" && run.Binding.OperationKind == "CONNECTION_TEST" {
			updatedSource, err := tx.ExecContext(ctx, `
                UPDATE data_sources
                SET last_test_status = ?, last_tested_at = ?, last_test_safe_summary_json = ?,
                    last_test_source = 'AGENT_JDBC'
                WHERE data_source_id = ?
            `, input.Status, utcText(input.Now), safeSummaryJSON, run.Binding.DataSourceID)
			if err != nil {
				return fmt.Errorf("write data source connection test result: %w", err)
			}
			updatedCount, err := updatedSource.RowsAffected()
			if err != nil {
				return fmt.Errorf("read data source connection test result write: %w", err)
			}
			if updatedCount != 1 {
				return ErrDataSourceConnectionTestLeaseRejected
			}
		}
		receipt = agentDataSourceConnectionTestReceipt{
			AgentID: input.AgentID, RequestID: input.RequestID, Operation: "COMPLETE", RequestDigest: input.RequestDigest,
			ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: input.Status, ExpiresAt: run.LeaseExpiresAt,
			CompletedAt: input.Now, CreatedAt: input.Now,
		}
		if err := insertAgentDataSourceConnectionTestReceipt(ctx, tx, receipt); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "DATA_SOURCE_CONNECTION_TEST_COMPLETED", "DATA_SOURCE_CONNECTION_TEST", input.ConnectionTestID, input.Status, input.RequestID, input.Now); err != nil {
			return err
		}
		result = DataSourceConnectionTestCompletionResult{
			ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
			BindingDigest: input.BindingDigest, Status: input.Status, EvidenceCode: input.EvidenceCode,
			VerificationSource: input.VerificationSource, SysVerificationStatus: input.SysVerificationStatus,
			SysResultCode: input.SysResultCode, CompletedAt: input.Now,
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, outcomeErr
}

// ExpireDataSourceConnectionTests 将超时的待领取或已领取测试固定为 EXPIRED。
// 它不会重新排队、续租或改派 Agent，迟到完成也无法将该状态提升为成功。
func (s *Store) ExpireDataSourceConnectionTests(ctx context.Context, now time.Time) (int, error) {
	if now.IsZero() {
		return 0, errors.New("data source connection test expiry time is required")
	}
	var count int
	err := s.withWrite(ctx, func(tx *sql.Tx) error {
		var err error
		count, err = expireDataSourceConnectionTestsTx(ctx, tx, now)
		return err
	})
	return count, err
}

// storedDataSourceConnectionTest 仅供同一短事务内复验租约与冻结绑定使用。
// 它不会作为浏览器响应直接序列化，避免泄露 Agent 回执、密文或内部租约细节。
type storedDataSourceConnectionTest struct {
	Binding               DataSourceConnectionTestBinding
	CreatorSubjectID      string
	AgentID               string
	Status                string
	LeaseID               string
	LeaseEpoch            int64
	LeaseExpiresAt        time.Time
	ResultCode            string
	SafeSummaryJSON       string
	SysVerificationStatus string
	SysResultCode         string
	CatalogObjects        []string
	CatalogGroups         []catalogresult.Group
	CatalogTruncated      bool
	CreatedAt             time.Time
	CompletedAt           time.Time
}

// agentDataSourceConnectionTestReceipt 仅保存 Agent 幂等重放所需的摘要、租约与安全状态投影。
// 它不保存 JDBC 输出、连接地址、用户名、密码或任何可恢复的秘密材料。
type agentDataSourceConnectionTestReceipt struct {
	AgentID          string
	RequestID        string
	Operation        string
	RequestDigest    string
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	BindingDigest    string
	Status           string
	ExpiresAt        time.Time
	CompletedAt      time.Time
	CreatedAt        time.Time
}

// dataSourceConnectionTestSecretResolutionReceipt 是秘密槽位请求的无秘密幂等投影。
// 控制面可以在同一有效租约中重新从密文取得材料，但不能缓存或写入上一次明文响应。
type dataSourceConnectionTestSecretResolutionReceipt struct {
	AgentID          string
	RequestID        string
	RequestDigest    string
	ConnectionTestID string
	LeaseID          string
	LeaseEpoch       int64
	BindingDigest    string
	Status           string
	CreatedAt        time.Time
	CompletedAt      time.Time
}

func readStoredDataSourceConnectionTest(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, connectionTestID string) (storedDataSourceConnectionTest, error) {
	var run storedDataSourceConnectionTest
	var leaseID, resultCode, safeSummaryJSON sql.NullString
	var sysCredentialID, sysVerificationStatus, sysResultCode sql.NullString
	var catalogDatabase, catalogCompatibilityMode, catalogObjectType, catalogExtendedObjectType, catalogKeyword, catalogObjectsJSON sql.NullString
	var catalogTruncated int
	var leaseEpoch sql.NullInt64
	var sysCredentialRevision sql.NullInt64
	var createdAt string
	var leaseExpiresAt, completedAt, validUntil sql.NullString
	err := queryer.QueryRowContext(ctx, `
        SELECT connection_test_id, data_source_id, creator_subject_id, connection_config_digest,
               credential_id, credential_revision, node_id, binding_agent_id, node_facts_revision,
		       binding_digest, status, verification_source, lease_id, lease_epoch,
               lease_expires_at, result_code, safe_summary_json, created_at, completed_at, valid_until,
               sys_credential_id, sys_credential_revision, sys_verification_status, sys_result_code,
		       operation_kind, catalog_database, catalog_compatibility_mode, catalog_object_type, catalog_extended_object_type, catalog_keyword,
               catalog_objects_json, catalog_truncated
        FROM data_source_connection_test_runs
        WHERE connection_test_id = ?
    `, connectionTestID).Scan(
		&run.Binding.ConnectionTestID, &run.Binding.DataSourceID, &run.CreatorSubjectID,
		&run.Binding.ConnectionConfigDigest, &run.Binding.CredentialID, &run.Binding.CredentialRevision,
		&run.Binding.NodeID, &run.Binding.BindingAgentID, &run.Binding.NodeFactsRevision,
		&run.Binding.BindingDigest, &run.Status, &run.Binding.VerificationSource, &leaseID, &leaseEpoch,
		&leaseExpiresAt, &resultCode, &safeSummaryJSON, &createdAt, &completedAt, &validUntil,
		&sysCredentialID, &sysCredentialRevision, &sysVerificationStatus, &sysResultCode,
		&run.Binding.OperationKind, &catalogDatabase, &catalogCompatibilityMode, &catalogObjectType, &catalogExtendedObjectType, &catalogKeyword,
		&catalogObjectsJSON, &catalogTruncated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return storedDataSourceConnectionTest{}, ErrDataSourceConnectionTestLeaseRejected
	}
	if err != nil {
		return storedDataSourceConnectionTest{}, fmt.Errorf("read stored data source connection test: %w", err)
	}
	var parseErr error
	if run.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
		return storedDataSourceConnectionTest{}, fmt.Errorf("parse data source connection test creation: %w", parseErr)
	}
	run.AgentID, run.LeaseID, run.LeaseEpoch = run.Binding.BindingAgentID, leaseID.String, leaseEpoch.Int64
	run.ResultCode, run.SafeSummaryJSON = resultCode.String, safeSummaryJSON.String
	run.Binding.SysCredentialID = sysCredentialID.String
	run.Binding.SysCredentialRevision = sysCredentialRevision.Int64
	run.SysVerificationStatus, run.SysResultCode = sysVerificationStatus.String, sysResultCode.String
	run.Binding.CatalogDatabase, run.Binding.CatalogCompatibilityMode, run.Binding.CatalogObjectType, run.Binding.CatalogKeyword = catalogDatabase.String, catalogCompatibilityMode.String, catalogObjectType.String, catalogKeyword.String
	// 历史 0022 约束只允许 TABLE/VIEW；扩展类型写入 0023 新列，数据库目录仍以空类型表示。
	if catalogExtendedObjectType.Valid {
		run.Binding.CatalogObjectType = catalogExtendedObjectType.String
	} else if run.Binding.OperationKind == "EXPORT_OBJECT_CATALOG" && !catalogObjectType.Valid {
		if run.Binding.CatalogDatabase == "" {
			run.Binding.CatalogObjectType = "DATABASE"
		} else {
			run.Binding.CatalogObjectType = "ALL"
		}
	}
	if catalogObjectsJSON.Valid {
		var target any = &run.CatalogObjects
		if run.Binding.CatalogObjectType == "ALL" {
			target = &run.CatalogGroups
		}
		if err := json.Unmarshal([]byte(catalogObjectsJSON.String), target); err != nil {
			return storedDataSourceConnectionTest{}, fmt.Errorf("parse export object catalog result: %w", err)
		}
	}
	run.CatalogTruncated = catalogTruncated == 1
	if validUntil.Valid {
		if run.Binding.ValidUntil, parseErr = time.Parse(time.RFC3339Nano, validUntil.String); parseErr != nil {
			return storedDataSourceConnectionTest{}, fmt.Errorf("parse data source connection test validity: %w", parseErr)
		}
		run.Binding.ValidUntil = run.Binding.ValidUntil.UTC()
	}
	if leaseExpiresAt.Valid {
		if run.LeaseExpiresAt, parseErr = time.Parse(time.RFC3339Nano, leaseExpiresAt.String); parseErr != nil {
			return storedDataSourceConnectionTest{}, fmt.Errorf("parse data source connection test lease expiry: %w", parseErr)
		}
		run.LeaseExpiresAt = run.LeaseExpiresAt.UTC()
	}
	if completedAt.Valid {
		if run.CompletedAt, parseErr = time.Parse(time.RFC3339Nano, completedAt.String); parseErr != nil {
			return storedDataSourceConnectionTest{}, fmt.Errorf("parse data source connection test completion: %w", parseErr)
		}
		run.CompletedAt = run.CompletedAt.UTC()
	}
	run.CreatedAt = run.CreatedAt.UTC()
	return run, nil
}

// readCurrentDataSourceConnectionTestBinding 只从当前数据库事实构造首次冻结绑定。
// 调用方提交的节点、版本和来源仅作资格条件，连接摘要与凭据修订绝不信任浏览器输入。
func readCurrentDataSourceConnectionTestBinding(ctx context.Context, tx *sql.Tx, input DataSourceConnectionTestCreate) (DataSourceConnectionTestBinding, error) {
	var binding DataSourceConnectionTestBinding
	var connectionKind, compatibilityMode, host, clusterName, tenantName, username, credentialStatus, nodePlatform string
	var lastHeartbeatAt, factsJSON sql.NullString
	var sysCredentialID, sysCredentialStatus string
	var sysCredentialRevision int64
	var port, capacityTotal, capacityUsed int
	err := tx.QueryRowContext(ctx, `
        SELECT ds.connection_kind, ds.compatibility_mode, ds.host, ds.port, ds.cluster_name,
               ds.tenant_name, ds.username, ds.credential_id, ds.current_credential_revision,
               cr.status, n.platform, a.agent_id, a.facts_revision, a.last_heartbeat_at,
               a.capacity_total, a.capacity_used, a.facts_json,
               COALESCE(ds.sys_credential_id, ''), COALESCE(ds.sys_credential_revision, 0),
               COALESCE(scr.status, '')
        FROM data_sources AS ds
        JOIN credential_revisions AS cr
          ON cr.credential_id = ds.credential_id
         AND cr.revision = ds.current_credential_revision
		LEFT JOIN sys_credential_revisions AS scr
		  ON scr.credential_id = ds.sys_credential_id
		 AND scr.revision = ds.sys_credential_revision
		 AND scr.data_source_id = ds.data_source_id
        JOIN execution_nodes AS n ON n.node_id = ?
        JOIN agents AS a ON a.node_id = n.node_id AND a.status = 'ACTIVE'
        JOIN auth_subjects AS subject ON subject.subject_id = ? AND subject.account_status = 'ACTIVE'
        WHERE ds.data_source_id = ?
          AND ds.revision = ?
          AND ds.state != 'ARCHIVED'
          AND (? != 'EXPORT_OBJECT_CATALOG' OR (ds.state = 'ENABLED' AND ds.last_test_status = 'SUCCEEDED' AND ds.last_test_source = 'AGENT_JDBC'))
          AND n.management_state IN ('ENABLED', 'DISABLED')
          AND (? != 'EXPORT_OBJECT_CATALOG' OR n.management_state = 'ENABLED')
          AND a.facts_revision > 0
	`, input.NodeID, input.CreatorSubjectID, input.DataSourceID, input.ExpectedDataSourceRevision, input.OperationKind, input.OperationKind).Scan(
		&connectionKind, &compatibilityMode, &host, &port, &clusterName, &tenantName, &username,
		&binding.CredentialID, &binding.CredentialRevision, &credentialStatus, &nodePlatform,
		&binding.BindingAgentID, &binding.NodeFactsRevision, &lastHeartbeatAt,
		&capacityTotal, &capacityUsed, &factsJSON,
		&sysCredentialID, &sysCredentialRevision, &sysCredentialStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DataSourceConnectionTestBinding{}, ErrDataSourceConnectionTestInvalid
	}
	if err != nil {
		return DataSourceConnectionTestBinding{}, fmt.Errorf("read current data source connection test binding: %w", err)
	}
	if credentialStatus != "ACTIVE" {
		return DataSourceConnectionTestBinding{}, ErrDataSourceConnectionTestInvalid
	}
	if (sysCredentialID == "") != (sysCredentialRevision == 0) || (sysCredentialRevision > 0 && sysCredentialStatus != "ACTIVE") {
		return DataSourceConnectionTestBinding{}, ErrDataSourceConnectionTestInvalid
	}
	if !lastHeartbeatAt.Valid || capacityTotal < 1 || capacityUsed < 0 || capacityUsed >= capacityTotal || !factsJSON.Valid {
		return DataSourceConnectionTestBinding{}, ErrDataSourceConnectionTestInvalid
	}
	heartbeatAt, err := time.Parse(time.RFC3339Nano, lastHeartbeatAt.String)
	if err != nil || heartbeatAt.Before(input.HeartbeatFreshAfter) {
		return DataSourceConnectionTestBinding{}, ErrDataSourceConnectionTestInvalid
	}
	facts, err := decodeAgentEnvironmentFacts(factsJSON.String)
	if err != nil || !dataSourceConnectionTestPlatformMatchesAgentFacts(nodePlatform, facts) {
		return DataSourceConnectionTestBinding{}, ErrDataSourceConnectionTestInvalid
	}
	connectionConfigDigest, err := dataSourceConnectionConfigDigest(
		input.DataSourceID, connectionKind, compatibilityMode, host, port, clusterName, tenantName,
		username, binding.CredentialID, binding.CredentialRevision,
	)
	if err != nil {
		return DataSourceConnectionTestBinding{}, err
	}
	binding = DataSourceConnectionTestBinding{
		ConnectionTestID:       input.ConnectionTestID,
		DataSourceID:           input.DataSourceID,
		ConnectionConfigDigest: connectionConfigDigest,
		CredentialID:           binding.CredentialID,
		CredentialRevision:     binding.CredentialRevision,
		NodeID:                 input.NodeID,
		NodeFactsRevision:      binding.NodeFactsRevision,
		BindingAgentID:         binding.BindingAgentID,
		VerificationSource:     input.VerificationSource,
		OperationKind:          input.OperationKind,
		CatalogDatabase:        input.CatalogDatabase,
		CatalogObjectType:      input.CatalogObjectType,
		CatalogKeyword:         input.CatalogKeyword,
		ValidUntil:             input.ValidUntil.UTC(),
	}
	if input.OperationKind == "EXPORT_OBJECT_CATALOG" {
		binding.CatalogCompatibilityMode = compatibilityMode
	}
	// G2 合成测试不解析真实秘密，因此不能冻结一个自身无法验证的 sys 槽位。
	if input.VerificationSource == "AGENT_JDBC" && input.OperationKind == "CONNECTION_TEST" {
		binding.SysCredentialID = sysCredentialID
		binding.SysCredentialRevision = sysCredentialRevision
	}
	return binding, nil
}

func claimDataSourceConnectionTestTx(ctx context.Context, tx *sql.Tx, connectionTestID string, input DataSourceConnectionTestClaimNext) (DataSourceConnectionTestLeaseGrant, error) {
	run, err := readStoredDataSourceConnectionTest(ctx, tx, connectionTestID)
	if err != nil {
		return DataSourceConnectionTestLeaseGrant{}, err
	}
	if run.Status == "EXPIRED" {
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseExpired
	}
	if !run.Binding.ValidUntil.After(input.Now) {
		if err := markDataSourceConnectionTestExpiredTx(ctx, tx, run.Binding.ConnectionTestID, input.Now); err != nil {
			return DataSourceConnectionTestLeaseGrant{}, err
		}
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseExpired
	}
	if run.Status != "PENDING" || run.Binding.NodeID != input.NodeID || run.Binding.BindingAgentID != input.AgentID {
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseRejected
	}
	if err := validateOrInvalidateDataSourceConnectionTestBinding(ctx, tx, input.AgentID, run, input.Now); err != nil {
		return DataSourceConnectionTestLeaseGrant{}, err
	}
	if err := ensurePrecheckAgentIdle(ctx, tx, input.AgentID); err != nil {
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseRejected
	}
	expiresAt := input.Now.Add(input.LeaseTTL)
	if expiresAt.After(run.Binding.ValidUntil) {
		expiresAt = run.Binding.ValidUntil
	}
	if !expiresAt.After(input.Now) {
		if err := markDataSourceConnectionTestExpiredTx(ctx, tx, run.Binding.ConnectionTestID, input.Now); err != nil {
			return DataSourceConnectionTestLeaseGrant{}, err
		}
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseExpired
	}
	leaseEpoch := int64(1)
	updated, err := tx.ExecContext(ctx, `
		UPDATE data_source_connection_test_runs
		SET status = 'LEASED', lease_id = ?, lease_epoch = ?, lease_expires_at = ?
		WHERE connection_test_id = ? AND status = 'PENDING'
	`, input.LeaseID, leaseEpoch, utcText(expiresAt), run.Binding.ConnectionTestID)
	if err != nil {
		return DataSourceConnectionTestLeaseGrant{}, fmt.Errorf("claim data source connection test: %w", err)
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return DataSourceConnectionTestLeaseGrant{}, fmt.Errorf("read data source connection test claim result: %w", err)
	}
	if affected != 1 {
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseRejected
	}
	receipt := agentDataSourceConnectionTestReceipt{
		AgentID: input.AgentID, RequestID: input.RequestID, Operation: "CLAIM", RequestDigest: input.RequestDigest,
		ConnectionTestID: run.Binding.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: leaseEpoch,
		BindingDigest: run.Binding.BindingDigest, Status: "LEASED", ExpiresAt: expiresAt, CreatedAt: input.Now,
	}
	if err := insertAgentDataSourceConnectionTestReceipt(ctx, tx, receipt); err != nil {
		return DataSourceConnectionTestLeaseGrant{}, err
	}
	return DataSourceConnectionTestLeaseGrant{
		ConnectionTestID: run.Binding.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: leaseEpoch,
		ExpiresAt: expiresAt, Binding: run.Binding,
	}, nil
}

// validateOrInvalidateDataSourceConnectionTestBinding 把当前事实漂移固定为 INVALIDATED。
// 这样旧租约无法在节点、Agent、凭据或连接配置已变化后继续解析槽位或提交通过结果。
func validateOrInvalidateDataSourceConnectionTestBinding(ctx context.Context, tx *sql.Tx, agentID string, run storedDataSourceConnectionTest, now time.Time) error {
	err := validateCurrentDataSourceConnectionTestBinding(ctx, tx, agentID, run.Binding)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
		return err
	}
	if invalidateErr := invalidateDataSourceConnectionTestTx(ctx, tx, run.Binding.ConnectionTestID, now); invalidateErr != nil {
		return invalidateErr
	}
	return ErrDataSourceConnectionTestLeaseRejected
}

func validateCurrentDataSourceConnectionTestBinding(ctx context.Context, tx *sql.Tx, agentID string, binding DataSourceConnectionTestBinding) error {
	if err := validateDataSourceConnectionTestBinding(binding); err != nil {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	if agentID != binding.BindingAgentID {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	var connectionKind, compatibilityMode, host, clusterName, tenantName, username string
	var port int
	var credentialID, credentialStatus, sourceState, nodeState, currentAgentID, agentStatus string
	var lastTestStatus, lastTestSource sql.NullString
	var sysCredentialID, sysCredentialStatus string
	var credentialRevision, factsRevision, sysCredentialRevision int64
	err := tx.QueryRowContext(ctx, `
        SELECT ds.connection_kind, ds.compatibility_mode, ds.host, ds.port, ds.cluster_name,
               ds.tenant_name, ds.username, ds.credential_id, ds.current_credential_revision,
	       ds.state, ds.last_test_status, ds.last_test_source, cr.status, n.management_state, a.agent_id, a.status, a.facts_revision,
		       COALESCE(ds.sys_credential_id, ''), COALESCE(ds.sys_credential_revision, 0),
		       COALESCE(scr.status, '')
        FROM data_sources AS ds
        JOIN credential_revisions AS cr
          ON cr.credential_id = ds.credential_id
         AND cr.revision = ds.current_credential_revision
		LEFT JOIN sys_credential_revisions AS scr
		  ON scr.credential_id = ds.sys_credential_id
		 AND scr.revision = ds.sys_credential_revision
		 AND scr.data_source_id = ds.data_source_id
        JOIN execution_nodes AS n ON n.node_id = ?
        JOIN agents AS a ON a.agent_id = ? AND a.node_id = n.node_id
        WHERE ds.data_source_id = ?
    `, binding.NodeID, binding.BindingAgentID, binding.DataSourceID).Scan(
		&connectionKind, &compatibilityMode, &host, &port, &clusterName, &tenantName, &username,
		&credentialID, &credentialRevision, &sourceState, &lastTestStatus, &lastTestSource, &credentialStatus, &nodeState,
		&currentAgentID, &agentStatus, &factsRevision,
		&sysCredentialID, &sysCredentialRevision, &sysCredentialStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	if err != nil {
		return fmt.Errorf("read current data source connection test binding: %w", err)
	}
	sysBindingMatches := binding.VerificationSource != "AGENT_JDBC" || binding.OperationKind == "EXPORT_OBJECT_CATALOG" ||
		(sysCredentialID == binding.SysCredentialID && sysCredentialRevision == binding.SysCredentialRevision &&
			(sysCredentialRevision == 0 || sysCredentialStatus == "ACTIVE"))
	if sourceState == "ARCHIVED" || credentialStatus != "ACTIVE" || !dataSourceConnectionTestNodeStateAllowed(nodeState) ||
		currentAgentID != binding.BindingAgentID || agentStatus != "ACTIVE" ||
		credentialID != binding.CredentialID || credentialRevision != binding.CredentialRevision ||
		factsRevision != binding.NodeFactsRevision || !sysBindingMatches {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	if binding.OperationKind == "EXPORT_OBJECT_CATALOG" && (sourceState != "ENABLED" || lastTestStatus.String != "SUCCEEDED" || lastTestSource.String != "AGENT_JDBC" || nodeState != "ENABLED") {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	connectionConfigDigest, err := dataSourceConnectionConfigDigest(
		binding.DataSourceID, connectionKind, compatibilityMode, host, port, clusterName, tenantName,
		username, credentialID, credentialRevision,
	)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(connectionConfigDigest), []byte(binding.ConnectionConfigDigest)) != 1 {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	expectedBindingDigest, err := dataSourceConnectionTestBindingDigest(binding)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(expectedBindingDigest), []byte(binding.BindingDigest)) != 1 {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	return nil
}

// dataSourceConnectionTestNodeStateAllowed 将基础连接诊断与任务接收状态分离。
// DISABLED 节点可以在已关联和当前事实约束下执行诊断，但 MAINTENANCE/ARCHIVED 节点必须拒绝；导出候选仍只接受 ENABLED。
func dataSourceConnectionTestNodeStateAllowed(state string) bool {
	return state == "ENABLED" || state == "DISABLED"
}

// dataSourceConnectionTestPlatformMatchesAgentFacts 复用固定三平台映射校验冻结时的机器事实。
// 浏览器声明的平台与 Agent 当前事实不一致时必须失败关闭，不能把跨平台节点用于诊断。
func dataSourceConnectionTestPlatformMatchesAgentFacts(platform string, facts AgentEnvironmentFacts) bool {
	switch platform {
	case "WINDOWS_AMD64":
		return facts.OperatingSystem == "WINDOWS" && facts.Architecture == "AMD64"
	case "LINUX_AMD64":
		return facts.OperatingSystem == "LINUX" && facts.Architecture == "AMD64"
	case "LINUX_ARM64":
		return facts.OperatingSystem == "LINUX" && facts.Architecture == "ARM64"
	default:
		return false
	}
}

func dataSourceConnectionConfigDigest(dataSourceID, connectionKind, compatibilityMode, host string, port int, clusterName, tenantName, username, credentialID string, credentialRevision int64) (string, error) {
	payload, err := json.Marshal(struct {
		DataSourceID       string `json:"dataSourceId"`
		ConnectionKind     string `json:"connectionKind"`
		CompatibilityMode  string `json:"compatibilityMode"`
		Host               string `json:"host"`
		Port               int    `json:"port"`
		ClusterName        string `json:"clusterName"`
		TenantName         string `json:"tenantName"`
		Username           string `json:"username"`
		CredentialID       string `json:"credentialId"`
		CredentialRevision int64  `json:"credentialRevision"`
	}{
		DataSourceID: dataSourceID, ConnectionKind: connectionKind, CompatibilityMode: compatibilityMode,
		Host: host, Port: port, ClusterName: clusterName, TenantName: tenantName, Username: username,
		CredentialID: credentialID, CredentialRevision: credentialRevision,
	})
	if err != nil {
		return "", fmt.Errorf("encode data source connection configuration digest: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func dataSourceConnectionTestBindingDigest(binding DataSourceConnectionTestBinding) (string, error) {
	var catalog any
	type catalogBinding struct {
		Database          string `json:"database"`
		CompatibilityMode string `json:"compatibilityMode"`
		ObjectType        string `json:"objectType"`
		Keyword           string `json:"keyword"`
	}
	if binding.OperationKind == "EXPORT_OBJECT_CATALOG" {
		catalog = catalogBinding{binding.CatalogDatabase, binding.CatalogCompatibilityMode, binding.CatalogObjectType, binding.CatalogKeyword}
	}
	payload, err := json.Marshal(struct {
		BindingAgentID         string `json:"bindingAgentId"`
		ConnectionConfigDigest string `json:"connectionConfigDigest"`
		ConnectionTestID       string `json:"connectionTestId"`
		CredentialID           string `json:"credentialId"`
		CredentialRevision     int64  `json:"credentialRevision"`
		DataSourceID           string `json:"dataSourceId"`
		NodeFactsRevision      int64  `json:"nodeFactsRevision"`
		NodeID                 string `json:"nodeId"`
		SysCredentialID        string `json:"sysCredentialId"`
		SysCredentialRevision  int64  `json:"sysCredentialRevision"`
		ValidUntil             string `json:"validUntil"`
		VerificationSource     string `json:"verificationSource"`
		Catalog                any    `json:"catalog,omitempty"`
	}{
		BindingAgentID: binding.BindingAgentID, ConnectionConfigDigest: binding.ConnectionConfigDigest,
		ConnectionTestID: binding.ConnectionTestID, CredentialID: binding.CredentialID,
		CredentialRevision: binding.CredentialRevision, DataSourceID: binding.DataSourceID,
		NodeFactsRevision: binding.NodeFactsRevision, NodeID: binding.NodeID,
		SysCredentialID: binding.SysCredentialID, SysCredentialRevision: binding.SysCredentialRevision,
		ValidUntil: utcText(binding.ValidUntil), VerificationSource: binding.VerificationSource,
		Catalog: catalog,
	})
	if err != nil {
		return "", fmt.Errorf("encode data source connection test binding digest: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func dataSourceConnectionTestSafeSummaryJSON(evidenceCode, verificationSource string) (string, error) {
	payload, err := json.Marshal(struct {
		EvidenceCode       string `json:"evidenceCode"`
		VerificationSource string `json:"verificationSource"`
	}{EvidenceCode: evidenceCode, VerificationSource: verificationSource})
	if err != nil {
		return "", fmt.Errorf("encode data source connection test safe summary: %w", err)
	}
	return string(payload), nil
}

func readAgentDataSourceConnectionTestReceipt(ctx context.Context, tx *sql.Tx, agentID, requestID string) (agentDataSourceConnectionTestReceipt, bool, error) {
	var receipt agentDataSourceConnectionTestReceipt
	var expiresAt, createdAt string
	var completedAt sql.NullString
	err := tx.QueryRowContext(ctx, `
        SELECT agent_id, request_id, operation, request_digest, connection_test_id, lease_id,
               lease_epoch, binding_digest, status, expires_at, completed_at, created_at
        FROM agent_data_source_connection_test_receipts
        WHERE agent_id = ? AND request_id = ?
    `, agentID, requestID).Scan(
		&receipt.AgentID, &receipt.RequestID, &receipt.Operation, &receipt.RequestDigest,
		&receipt.ConnectionTestID, &receipt.LeaseID, &receipt.LeaseEpoch, &receipt.BindingDigest,
		&receipt.Status, &expiresAt, &completedAt, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return agentDataSourceConnectionTestReceipt{}, false, nil
	}
	if err != nil {
		return agentDataSourceConnectionTestReceipt{}, false, fmt.Errorf("read agent data source connection test receipt: %w", err)
	}
	var parseErr error
	if receipt.ExpiresAt, parseErr = time.Parse(time.RFC3339Nano, expiresAt); parseErr != nil {
		return agentDataSourceConnectionTestReceipt{}, false, fmt.Errorf("parse data source connection test receipt expiry: %w", parseErr)
	}
	if receipt.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
		return agentDataSourceConnectionTestReceipt{}, false, fmt.Errorf("parse data source connection test receipt creation: %w", parseErr)
	}
	if completedAt.Valid {
		if receipt.CompletedAt, parseErr = time.Parse(time.RFC3339Nano, completedAt.String); parseErr != nil {
			return agentDataSourceConnectionTestReceipt{}, false, fmt.Errorf("parse data source connection test receipt completion: %w", parseErr)
		}
		receipt.CompletedAt = receipt.CompletedAt.UTC()
	}
	receipt.ExpiresAt, receipt.CreatedAt = receipt.ExpiresAt.UTC(), receipt.CreatedAt.UTC()
	return receipt, true, nil
}

func insertAgentDataSourceConnectionTestReceipt(ctx context.Context, tx *sql.Tx, receipt agentDataSourceConnectionTestReceipt) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO agent_data_source_connection_test_receipts(
            agent_id, request_id, operation, request_digest, connection_test_id, lease_id,
            lease_epoch, binding_digest, status, expires_at, completed_at, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, receipt.AgentID, receipt.RequestID, receipt.Operation, receipt.RequestDigest, receipt.ConnectionTestID,
		receipt.LeaseID, receipt.LeaseEpoch, receipt.BindingDigest, receipt.Status,
		utcText(receipt.ExpiresAt), nullableTimeText(receipt.CompletedAt), utcText(receipt.CreatedAt))
	if err != nil {
		return fmt.Errorf("write agent data source connection test receipt: %w", err)
	}
	return nil
}

func replayDataSourceConnectionTestLeaseGrant(ctx context.Context, tx *sql.Tx, receipt agentDataSourceConnectionTestReceipt, now time.Time) (DataSourceConnectionTestLeaseGrant, error) {
	run, err := readStoredDataSourceConnectionTest(ctx, tx, receipt.ConnectionTestID)
	if err != nil {
		return DataSourceConnectionTestLeaseGrant{}, err
	}
	if run.Status == "EXPIRED" || !run.Binding.ValidUntil.After(now) || !run.LeaseExpiresAt.After(now) {
		if run.Status != "EXPIRED" {
			if err := markDataSourceConnectionTestExpiredTx(ctx, tx, run.Binding.ConnectionTestID, now); err != nil {
				return DataSourceConnectionTestLeaseGrant{}, err
			}
		}
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseExpired
	}
	if run.Status != "LEASED" || run.AgentID != receipt.AgentID || run.LeaseID != receipt.LeaseID ||
		run.LeaseEpoch != receipt.LeaseEpoch || run.Binding.BindingDigest != receipt.BindingDigest {
		return DataSourceConnectionTestLeaseGrant{}, ErrDataSourceConnectionTestLeaseRejected
	}
	if err := validateOrInvalidateDataSourceConnectionTestBinding(ctx, tx, receipt.AgentID, run, now); err != nil {
		return DataSourceConnectionTestLeaseGrant{}, err
	}
	return DataSourceConnectionTestLeaseGrant{
		ConnectionTestID: receipt.ConnectionTestID, LeaseID: receipt.LeaseID, LeaseEpoch: receipt.LeaseEpoch,
		ExpiresAt: receipt.ExpiresAt, Binding: run.Binding, Replayed: true,
	}, nil
}

func readDataSourceConnectionTestSecretResolutionReceipt(ctx context.Context, tx *sql.Tx, agentID, requestID string) (dataSourceConnectionTestSecretResolutionReceipt, bool, error) {
	var receipt dataSourceConnectionTestSecretResolutionReceipt
	var createdAt string
	var completedAt sql.NullString
	err := tx.QueryRowContext(ctx, `
        SELECT agent_id, request_id, request_digest, connection_test_id, lease_id, lease_epoch,
               binding_digest, status, created_at, completed_at
        FROM agent_data_source_connection_test_secret_resolution_receipts
        WHERE agent_id = ? AND request_id = ?
    `, agentID, requestID).Scan(
		&receipt.AgentID, &receipt.RequestID, &receipt.RequestDigest, &receipt.ConnectionTestID,
		&receipt.LeaseID, &receipt.LeaseEpoch, &receipt.BindingDigest, &receipt.Status, &createdAt, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return dataSourceConnectionTestSecretResolutionReceipt{}, false, nil
	}
	if err != nil {
		return dataSourceConnectionTestSecretResolutionReceipt{}, false, fmt.Errorf("read data source connection test secret resolution receipt: %w", err)
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return dataSourceConnectionTestSecretResolutionReceipt{}, false, fmt.Errorf("parse data source connection test secret resolution creation: %w", err)
	}
	receipt.CreatedAt = parsedCreatedAt.UTC()
	if completedAt.Valid {
		parsedCompletedAt, err := time.Parse(time.RFC3339Nano, completedAt.String)
		if err != nil {
			return dataSourceConnectionTestSecretResolutionReceipt{}, false, fmt.Errorf("parse data source connection test secret resolution completion: %w", err)
		}
		receipt.CompletedAt = parsedCompletedAt.UTC()
	}
	return receipt, true, nil
}

func sameDataSourceConnectionTestSecretResolutionReceipt(receipt dataSourceConnectionTestSecretResolutionReceipt, input DataSourceConnectionTestSecretResolutionRequest) bool {
	return receipt.RequestDigest == input.RequestDigest && receipt.ConnectionTestID == input.ConnectionTestID &&
		receipt.LeaseID == input.LeaseID && receipt.LeaseEpoch == input.LeaseEpoch &&
		receipt.BindingDigest == input.BindingDigest
}

func insertDataSourceConnectionTestSecretResolutionReceipt(ctx context.Context, tx *sql.Tx, receipt dataSourceConnectionTestSecretResolutionReceipt) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO agent_data_source_connection_test_secret_resolution_receipts(
            agent_id, request_id, request_digest, connection_test_id, lease_id, lease_epoch,
            binding_digest, status, created_at, completed_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
    `, receipt.AgentID, receipt.RequestID, receipt.RequestDigest, receipt.ConnectionTestID,
		receipt.LeaseID, receipt.LeaseEpoch, receipt.BindingDigest, receipt.Status, utcText(receipt.CreatedAt))
	if err != nil {
		return fmt.Errorf("write data source connection test secret resolution receipt: %w", err)
	}
	return nil
}

func updateDataSourceConnectionTestSecretResolutionReceipt(ctx context.Context, tx *sql.Tx, agentID, requestID, status string, completedAt time.Time) error {
	updated, err := tx.ExecContext(ctx, `
        UPDATE agent_data_source_connection_test_secret_resolution_receipts
        SET status = ?, completed_at = ?
        WHERE agent_id = ? AND request_id = ?
    `, status, utcText(completedAt), agentID, requestID)
	if err != nil {
		return fmt.Errorf("complete data source connection test secret resolution receipt: %w", err)
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("read data source connection test secret resolution update: %w", err)
	}
	if affected != 1 {
		return ErrDataSourceConnectionTestLeaseRejected
	}
	return nil
}

func readEncryptedDataSourceConnectionTestDatabaseConnection(ctx context.Context, tx *sql.Tx, run storedDataSourceConnectionTest) (EncryptedDataSourceConnectionTestDatabaseConnection, error) {
	var connection EncryptedDataSourceConnectionTestDatabaseConnection
	var connectionKind, username, tenantName, clusterName string
	err := tx.QueryRowContext(ctx, `
        SELECT ds.host, ds.port, ds.connection_kind, ds.username, ds.tenant_name, ds.cluster_name,
               cr.key_id, cr.nonce, cr.ciphertext,
               subject.subject_id
        FROM data_sources AS ds
        JOIN credential_revisions AS cr
          ON cr.credential_id = ds.credential_id
         AND cr.revision = ds.current_credential_revision
        JOIN auth_subjects AS subject ON subject.subject_id = ?
        WHERE ds.data_source_id = ?
          AND ds.credential_id = ?
          AND ds.current_credential_revision = ?
          AND ds.state != 'ARCHIVED'
          AND cr.status = 'ACTIVE'
          AND subject.account_status = 'ACTIVE'
    `, run.CreatorSubjectID, run.Binding.DataSourceID, run.Binding.CredentialID, run.Binding.CredentialRevision).Scan(
		&connection.Host, &connection.Port, &connectionKind, &username, &tenantName, &clusterName,
		&connection.KeyID, &connection.Nonce, &connection.Ciphertext, &connection.OwnerSubjectID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, ErrDataSourceConnectionTestLeaseRejected
	}
	if err != nil {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, fmt.Errorf("read encrypted data source connection test database connection: %w", err)
	}
	if connectionKind != "ODP" {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, ErrDataSourceConnectionTestLeaseRejected
	}
	var identityOK bool
	connection.Username, identityOK = composePrivateODPJDBCIdentity(username, tenantName, clusterName)
	if !identityOK {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, ErrDataSourceConnectionTestLeaseRejected
	}
	connection.DataSourceID = run.Binding.DataSourceID
	connection.NodeID = run.Binding.NodeID
	connection.CredentialID = run.Binding.CredentialID
	connection.Revision = run.Binding.CredentialRevision
	if connection.Host == "" || connection.Port < 1 || connection.Port > 65535 || len(connection.Username) == 0 ||
		connection.OwnerSubjectID != run.CreatorSubjectID || connection.KeyID == "" || len(connection.Nonce) == 0 || len(connection.Ciphertext) == 0 {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, ErrDataSourceConnectionTestLeaseRejected
	}
	return connection, nil
}

// readEncryptedDataSourceConnectionTestSysCredential 从冻结的 sys 凭据引用读取加密材料并组装 sys 租户身份。
// sys 身份由平台组装；集群为空时为 sysUser@sys，有集群时为 sysUser@sys#cluster（参考 ODC：账号勿填 @sys#集群 后缀）。
func readEncryptedDataSourceConnectionTestSysCredential(ctx context.Context, tx *sql.Tx, run storedDataSourceConnectionTest) (EncryptedDataSourceConnectionTestDatabaseConnection, error) {
	var connection EncryptedDataSourceConnectionTestDatabaseConnection
	var sysUser, clusterName string
	err := tx.QueryRowContext(ctx, `
        SELECT ds.host, ds.port, ds.cluster_name, ds.sys_user,
               scr.key_id, scr.nonce, scr.ciphertext,
               subject.subject_id
        FROM data_sources AS ds
        JOIN sys_credential_revisions AS scr
          ON scr.credential_id = ds.sys_credential_id
         AND scr.data_source_id = ds.data_source_id
         AND scr.revision = ds.sys_credential_revision
        JOIN auth_subjects AS subject ON subject.subject_id = ?
        WHERE ds.data_source_id = ?
          AND ds.sys_credential_id = ?
          AND ds.sys_credential_revision = ?
          AND ds.state != 'ARCHIVED'
          AND scr.status = 'ACTIVE'
          AND subject.account_status = 'ACTIVE'
    `, run.CreatorSubjectID, run.Binding.DataSourceID, run.Binding.SysCredentialID, run.Binding.SysCredentialRevision).Scan(
		&connection.Host, &connection.Port, &clusterName, &sysUser,
		&connection.KeyID, &connection.Nonce, &connection.Ciphertext, &connection.OwnerSubjectID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, ErrDataSourceConnectionTestLeaseRejected
	}
	if err != nil {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, fmt.Errorf("read encrypted data source connection test sys credential: %w", err)
	}
	var identityOK bool
	connection.Username, identityOK = composePrivateODPJDBCIdentity(sysUser, "sys", clusterName)
	if !identityOK {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, ErrDataSourceConnectionTestLeaseRejected
	}
	connection.DataSourceID = run.Binding.DataSourceID
	connection.NodeID = run.Binding.NodeID
	connection.CredentialID = run.Binding.SysCredentialID
	connection.Revision = run.Binding.SysCredentialRevision
	if connection.Host == "" || connection.Port < 1 || connection.Port > 65535 || len(connection.Username) == 0 ||
		connection.OwnerSubjectID != run.CreatorSubjectID || connection.KeyID == "" || len(connection.Nonce) == 0 || len(connection.Ciphertext) == 0 {
		connection.Destroy()
		return EncryptedDataSourceConnectionTestDatabaseConnection{}, ErrDataSourceConnectionTestLeaseRejected
	}
	return connection, nil
}

func hasDataSourceConnectionTestAcknowledgementReceipt(ctx context.Context, tx *sql.Tx, agentID string, run storedDataSourceConnectionTest) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `
        SELECT COUNT(*)
        FROM agent_data_source_connection_test_receipts
        WHERE agent_id = ? AND operation = 'ACKNOWLEDGE' AND connection_test_id = ?
          AND lease_id = ? AND lease_epoch = ? AND binding_digest = ? AND status = 'ACKNOWLEDGED'
    `, agentID, run.Binding.ConnectionTestID, run.LeaseID, run.LeaseEpoch, run.Binding.BindingDigest).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("read data source connection test acknowledgement receipt: %w", err)
	}
	return count == 1, nil
}

func recordExpiredDataSourceConnectionTestCompletion(ctx context.Context, tx *sql.Tx, input AgentDataSourceConnectionTestCompletion, run storedDataSourceConnectionTest) (DataSourceConnectionTestCompletionResult, error) {
	if run.AgentID != input.AgentID || run.LeaseID != input.LeaseID || run.LeaseEpoch != input.LeaseEpoch ||
		run.Binding.BindingDigest != input.BindingDigest || run.LeaseExpiresAt.IsZero() {
		return DataSourceConnectionTestCompletionResult{}, ErrDataSourceConnectionTestLeaseRejected
	}
	acknowledged, err := hasDataSourceConnectionTestAcknowledgementReceipt(ctx, tx, input.AgentID, run)
	if err != nil {
		return DataSourceConnectionTestCompletionResult{}, err
	}
	if !acknowledged {
		return DataSourceConnectionTestCompletionResult{}, ErrDataSourceConnectionTestLeaseRejected
	}
	receipt := agentDataSourceConnectionTestReceipt{
		AgentID: input.AgentID, RequestID: input.RequestID, Operation: "COMPLETE", RequestDigest: input.RequestDigest,
		ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
		BindingDigest: input.BindingDigest, Status: "EXPIRED", ExpiresAt: run.LeaseExpiresAt,
		CompletedAt: input.Now, CreatedAt: input.Now,
	}
	if err := insertAgentDataSourceConnectionTestReceipt(ctx, tx, receipt); err != nil {
		return DataSourceConnectionTestCompletionResult{}, err
	}
	if err := insertAudit(ctx, tx, "AGENT", input.AgentID, "DATA_SOURCE_CONNECTION_TEST_COMPLETION_EXPIRED", "DATA_SOURCE_CONNECTION_TEST", input.ConnectionTestID, "FAILED", input.RequestID, input.Now); err != nil {
		return DataSourceConnectionTestCompletionResult{}, err
	}
	return DataSourceConnectionTestCompletionResult{
		ConnectionTestID: input.ConnectionTestID, LeaseID: input.LeaseID, LeaseEpoch: input.LeaseEpoch,
		BindingDigest: input.BindingDigest, Status: "EXPIRED", EvidenceCode: "LEASE_EXPIRED",
		VerificationSource: run.Binding.VerificationSource, CompletedAt: input.Now,
	}, nil
}

// expireDataSourceConnectionTestsTx 使用单条写入取得 SQLite 写锁，再只为真实状态转换追加过期审计。
// 它不先读取候选记录，避免多个 Store 在延迟读事务升级写锁时相互冲突。
func expireDataSourceConnectionTestsTx(ctx context.Context, tx *sql.Tx, now time.Time) (int, error) {
	rows, err := tx.QueryContext(ctx, `
        UPDATE data_source_connection_test_runs
        SET status = 'EXPIRED', result_code = 'LEASE_EXPIRED',
            completed_at = COALESCE(completed_at, ?)
        WHERE status IN ('PENDING', 'LEASED')
          AND (
              valid_until IS NULL OR valid_until <= ?
              OR (status = 'LEASED' AND (lease_expires_at IS NULL OR lease_expires_at <= ?))
              OR (status = 'PENDING' AND operation_kind = 'EXPORT_OBJECT_CATALOG' AND created_at <= ?)
          )
        RETURNING connection_test_id
    `, utcText(now), utcText(now), utcText(now), utcText(now.Add(-ExportObjectCatalogClaimTimeout)))
	if err != nil {
		return 0, fmt.Errorf("expire data source connection tests: %w", err)
	}
	var connectionTestIDs []string
	for rows.Next() {
		var connectionTestID string
		if err := rows.Scan(&connectionTestID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan expired data source connection test result: %w", err)
		}
		connectionTestIDs = append(connectionTestIDs, connectionTestID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate expired data source connection test results: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close expired data source connection tests: %w", err)
	}
	// 对象名仅供短时向导选择；到期后清空持久化结果，不保留目录历史。
	if _, err := tx.ExecContext(ctx, `
		UPDATE data_source_connection_test_runs
		SET catalog_objects_json = NULL, catalog_truncated = 0
		WHERE operation_kind = 'EXPORT_OBJECT_CATALOG' AND valid_until <= ? AND catalog_objects_json IS NOT NULL
	`, utcText(now)); err != nil {
		return 0, fmt.Errorf("clear expired export object catalog: %w", err)
	}
	for _, connectionTestID := range connectionTestIDs {
		if err := insertDataSourceConnectionTestExpiryAudit(ctx, tx, connectionTestID, now); err != nil {
			return 0, err
		}
	}
	return len(connectionTestIDs), nil
}

func markDataSourceConnectionTestExpiredTx(ctx context.Context, tx *sql.Tx, connectionTestID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
        UPDATE data_source_connection_test_runs
        SET status = 'EXPIRED', result_code = 'LEASE_EXPIRED',
            completed_at = COALESCE(completed_at, ?)
        WHERE connection_test_id = ? AND status IN ('PENDING', 'LEASED')
    `, utcText(now), connectionTestID)
	if err != nil {
		return fmt.Errorf("mark data source connection test expired: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read marked data source connection test expiry result: %w", err)
	}
	if count == 1 {
		return insertDataSourceConnectionTestExpiryAudit(ctx, tx, connectionTestID, now)
	}
	return nil
}

func invalidateDataSourceConnectionTestTx(ctx context.Context, tx *sql.Tx, connectionTestID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
        UPDATE data_source_connection_test_runs
        SET status = 'INVALIDATED', result_code = 'BINDING_INVALIDATED',
            completed_at = COALESCE(completed_at, ?)
        WHERE connection_test_id = ? AND status IN ('PENDING', 'LEASED')
    `, utcText(now), connectionTestID)
	if err != nil {
		return fmt.Errorf("invalidate data source connection test: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read data source connection test invalidation result: %w", err)
	}
	if count == 1 {
		return insertAudit(ctx, tx, "SYSTEM", "control-plane", "DATA_SOURCE_CONNECTION_TEST_INVALIDATED", "DATA_SOURCE_CONNECTION_TEST", connectionTestID, "FAILED", "data-source-connection-test-invalidated-"+connectionTestID, now)
	}
	return nil
}

func insertDataSourceConnectionTestExpiryAudit(ctx context.Context, tx *sql.Tx, connectionTestID string, now time.Time) error {
	return insertAudit(ctx, tx, "SYSTEM", "control-plane", "DATA_SOURCE_CONNECTION_TEST_EXPIRED", "DATA_SOURCE_CONNECTION_TEST", connectionTestID, "FAILED", "data-source-connection-test-expiry-"+connectionTestID, now)
}

// insertDataSourceConnectionTestSecretResolutionAudit 为同一槽位请求保留意图和结果两类无秘密审计事实。
// 审计标识追加动作后缀，避免普通对象请求去重规则覆盖前一条事实。
func insertDataSourceConnectionTestSecretResolutionAudit(ctx context.Context, tx *sql.Tx, agentID, connectionTestID, requestID, action, result string, occurredAt time.Time) error {
	safeDiff, err := json.Marshal(map[string]string{"objectId": connectionTestID})
	if err != nil {
		return fmt.Errorf("encode data source connection test secret audit diff: %w", err)
	}
	auditEventID := auditID(connectionTestID, requestID) + "-" + action
	_, err = tx.ExecContext(ctx, `
        INSERT INTO audit_events(
            audit_event_id, actor_type, actor_id, action, object_type, object_id,
            result, request_id, safe_diff_json, occurred_at
        ) VALUES (?, 'AGENT', ?, ?, 'DATA_SOURCE_CONNECTION_TEST', ?, ?, ?, ?, ?)
    `, auditEventID, agentID, action, connectionTestID, result, requestID, string(safeDiff), utcText(occurredAt))
	if err != nil {
		return fmt.Errorf("append data source connection test secret resolution audit: %w", err)
	}
	return nil
}

// validateDataSourceConnectionTestCreate 约束浏览器只可请求当前数据源版本上的受控测试意图。
func validateDataSourceConnectionTestCreate(input DataSourceConnectionTestCreate) error {
	if !validAgentOpaqueValue(input.ConnectionTestID, 256) || !validAgentOpaqueValue(input.DataSourceID, 256) ||
		!validAgentOpaqueValue(input.CreatorSubjectID, 256) || input.ExpectedDataSourceRevision < 1 ||
		!validAgentOpaqueValue(input.NodeID, 256) || !oneOf(input.VerificationSource, "G2_SYNTHETIC", "AGENT_JDBC") ||
		!validAgentOpaqueValue(input.RequestID, 256) || !validAgentOpaqueValue(input.IdempotencyKey, 256) ||
		!isSHA256(input.RequestDigest) || input.CreatedAt.IsZero() || input.HeartbeatFreshAfter.IsZero() ||
		input.HeartbeatFreshAfter.After(input.CreatedAt) || input.ValidUntil.IsZero() || !input.ValidUntil.After(input.CreatedAt) {
		return ErrDataSourceConnectionTestInvalid
	}
	if input.OperationKind == "EXPORT_OBJECT_CATALOG" {
		if input.VerificationSource != "AGENT_JDBC" || !validCatalogScope(input.CatalogDatabase, input.CatalogObjectType) ||
			!validCatalogText(input.CatalogKeyword, 100, true) {
			return ErrDataSourceConnectionTestInvalid
		}
	} else if input.OperationKind != "CONNECTION_TEST" || input.CatalogDatabase != "" || input.CatalogObjectType != "" || input.CatalogKeyword != "" {
		return ErrDataSourceConnectionTestInvalid
	}
	return nil
}

func validateDataSourceConnectionTestBinding(binding DataSourceConnectionTestBinding) error {
	if !validAgentOpaqueValue(binding.ConnectionTestID, 256) || !validAgentOpaqueValue(binding.DataSourceID, 256) ||
		!isSHA256(binding.ConnectionConfigDigest) || !validAgentOpaqueValue(binding.CredentialID, 256) ||
		binding.CredentialRevision < 1 || !validAgentOpaqueValue(binding.NodeID, 256) ||
		binding.NodeFactsRevision < 1 || !validAgentOpaqueValue(binding.BindingAgentID, 256) ||
		!isSHA256(binding.BindingDigest) || !oneOf(binding.VerificationSource, "G2_SYNTHETIC", "AGENT_JDBC") ||
		binding.ValidUntil.IsZero() {
		return ErrDataSourceConnectionTestInvalid
	}
	if (binding.SysCredentialID == "") != (binding.SysCredentialRevision == 0) {
		return ErrDataSourceConnectionTestInvalid
	}
	if binding.OperationKind == "EXPORT_OBJECT_CATALOG" {
		if binding.VerificationSource != "AGENT_JDBC" || binding.SysCredentialID != "" ||
			!validCatalogScope(binding.CatalogDatabase, binding.CatalogObjectType) || !oneOf(binding.CatalogCompatibilityMode, "MYSQL", "ORACLE") ||
			!validCatalogText(binding.CatalogKeyword, 100, true) {
			return ErrDataSourceConnectionTestInvalid
		}
	} else if (binding.OperationKind != "" && binding.OperationKind != "CONNECTION_TEST") || binding.CatalogDatabase != "" || binding.CatalogCompatibilityMode != "" || binding.CatalogObjectType != "" || binding.CatalogKeyword != "" {
		return ErrDataSourceConnectionTestInvalid
	}
	return nil
}

// validCatalogScope 限制数据库目录查询只能使用空数据库名；对象查询仍必须绑定明确数据库。
func validCatalogScope(database, objectType string) bool {
	if objectType == "DATABASE" {
		return database == ""
	}
	return oneOf(objectType, "ALL", "TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE") && validCatalogText(database, 256, false)
}

// storedCatalogObjectType 将数据库目录及批量对象目录编码为空对象类型；两者通过数据库名区分。
func storedCatalogObjectType(binding DataSourceConnectionTestBinding) any {
	if binding.OperationKind == "EXPORT_OBJECT_CATALOG" && oneOf(binding.CatalogObjectType, "DATABASE", "ALL", "FUNCTION", "PROCEDURE", "SEQUENCE") {
		return nil
	}
	return nullableString(binding.CatalogObjectType)
}

// storedExtendedCatalogObjectType 只把新增 DDL 对象类型写入 0023 新列，保留历史 TABLE/VIEW 编码。
func storedExtendedCatalogObjectType(binding DataSourceConnectionTestBinding) any {
	if binding.OperationKind == "EXPORT_OBJECT_CATALOG" && oneOf(binding.CatalogObjectType, "FUNCTION", "PROCEDURE", "SEQUENCE") {
		return binding.CatalogObjectType
	}
	return nil
}

// validCatalogText 将目录筛选限制为短 UTF-8 文本；JDBC 模式转义由固定探针执行。
func validCatalogText(value string, maximum int, allowEmpty bool) bool {
	if (!allowEmpty && value == "") || len(value) > maximum || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

// validCatalogResult 限定 Agent 可回传的对象名集合；失败与未知不得携带可发现对象。
func validCatalogResult(status string, objects []string, groups []catalogresult.Group, truncated bool, objectType string) bool {
	if status != "SUCCEEDED" {
		return len(objects) == 0 && len(groups) == 0 && !truncated
	}
	if objectType == "ALL" {
		return len(objects) == 0 && !truncated && catalogresult.ValidGroups(groups)
	}
	if len(groups) != 0 {
		return false
	}
	if (objectType == "DATABASE" && len(objects) > 100) || (objectType != "DATABASE" && truncated) {
		return false
	}
	seen := make(map[string]bool, len(objects))
	for _, name := range objects {
		if !validCatalogText(name, 256, false) || strings.ContainsAny(name, "*,") || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func validateDataSourceConnectionTestClaimNext(input DataSourceConnectionTestClaimNext) error {
	if !validAgentOpaqueValue(input.AgentID, 256) || !validAgentOpaqueValue(input.NodeID, 256) ||
		!validAgentOpaqueValue(input.LeaseID, 256) || !validAgentOpaqueValue(input.RequestID, 256) ||
		!isSHA256(input.RequestDigest) || input.LeaseTTL <= 0 || input.Now.IsZero() {
		return ErrDataSourceConnectionTestInvalid
	}
	return nil
}

func validateDataSourceConnectionTestAcknowledgement(input DataSourceConnectionTestAcknowledgement) error {
	return validateDataSourceConnectionTestLeaseInput(
		input.AgentID, input.ConnectionTestID, input.LeaseID, input.LeaseEpoch,
		input.BindingDigest, input.RequestID, input.RequestDigest, input.Now,
	)
}

func validateDataSourceConnectionTestSecretResolutionRequest(input DataSourceConnectionTestSecretResolutionRequest) error {
	return validateDataSourceConnectionTestLeaseInput(
		input.AgentID, input.ConnectionTestID, input.LeaseID, input.LeaseEpoch,
		input.BindingDigest, input.RequestID, input.RequestDigest, input.Now,
	)
}

func validateDataSourceConnectionTestSecretResolutionOutcome(input DataSourceConnectionTestSecretResolutionOutcome) error {
	return validateDataSourceConnectionTestLeaseInput(
		input.AgentID, input.ConnectionTestID, input.LeaseID, input.LeaseEpoch,
		input.BindingDigest, input.RequestID, input.RequestDigest, input.Now,
	)
}

func validateDataSourceConnectionTestLeaseInput(agentID, connectionTestID, leaseID string, leaseEpoch int64, bindingDigest, requestID, requestDigest string, now time.Time) error {
	if !validAgentOpaqueValue(agentID, 256) || !validAgentOpaqueValue(connectionTestID, 256) ||
		!validAgentOpaqueValue(leaseID, 256) || leaseEpoch < 1 || !isSHA256(bindingDigest) ||
		!validAgentOpaqueValue(requestID, 256) || !isSHA256(requestDigest) || now.IsZero() {
		return ErrDataSourceConnectionTestInvalid
	}
	return nil
}

func validateAgentDataSourceConnectionTestCompletion(input AgentDataSourceConnectionTestCompletion) error {
	if err := validateDataSourceConnectionTestLeaseInput(
		input.AgentID, input.ConnectionTestID, input.LeaseID, input.LeaseEpoch,
		input.BindingDigest, input.RequestID, input.RequestDigest, input.Now,
	); err != nil {
		return err
	}
	if !validDataSourceConnectionTestResultCode(input.EvidenceCode) {
		return ErrDataSourceConnectionTestInvalid
	}
	switch input.VerificationSource {
	case "G2_SYNTHETIC":
		if input.Status != "SUCCEEDED" || input.EvidenceCode != "SYNTHETIC_OK" {
			return ErrDataSourceConnectionTestInvalid
		}
	case "AGENT_JDBC":
		if !validAgentJDBCConnectionTestOutcome(input.Status, input.EvidenceCode) {
			return ErrDataSourceConnectionTestInvalid
		}
	default:
		return ErrDataSourceConnectionTestInvalid
	}
	// 可选的 sys 凭据验证结果只允许受控枚举与证据码（镜像数据库结果，SYS_ 前缀）。
	if !validAgentJDBCSysVerificationOutcome(input.VerificationSource, input.SysVerificationStatus, input.SysResultCode) {
		return ErrDataSourceConnectionTestInvalid
	}
	return nil
}

// validAgentJDBCSysVerificationOutcome 校验可选的 sys 凭据验证结果（与数据库结果相互独立）。
func validAgentJDBCSysVerificationOutcome(verificationSource, status, resultCode string) bool {
	if verificationSource != "AGENT_JDBC" {
		return status == "NOT_CONFIGURED" && resultCode == ""
	}
	switch status {
	case "NOT_CONFIGURED":
		return resultCode == ""
	case "SUCCEEDED":
		return resultCode == "SYS_CONNECTED"
	case "FAILED":
		return resultCode == "SYS_HOST_UNRESOLVABLE" || resultCode == "SYS_TCP_REFUSED" ||
			resultCode == "SYS_TCP_TIMEOUT" || resultCode == "SYS_TCP_UNREACHABLE" || resultCode == "SYS_CONNECTION_FAILED"
	case "UNKNOWN":
		return resultCode == "SYS_CONNECTION_UNAVAILABLE"
	default:
		return false
	}
}

func validAgentJDBCConnectionTestOutcome(status, evidenceCode string) bool {
	switch status {
	case "SUCCEEDED":
		return evidenceCode == "DATABASE_CONNECTED"
	case "FAILED":
		return evidenceCode == "DATABASE_HOST_UNRESOLVABLE" || evidenceCode == "DATABASE_TCP_REFUSED" ||
			evidenceCode == "DATABASE_TCP_TIMEOUT" || evidenceCode == "DATABASE_TCP_UNREACHABLE" || evidenceCode == "DATABASE_CONNECTION_FAILED"
	case "UNKNOWN":
		return evidenceCode == "DATABASE_CONNECTION_UNAVAILABLE"
	default:
		return false
	}
}

func validDataSourceConnectionTestResultCode(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, item := range []byte(value) {
		if !(item >= 'A' && item <= 'Z' || item >= '0' && item <= '9' || item == '_') {
			return false
		}
	}
	return true
}
