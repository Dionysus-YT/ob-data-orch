package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func Test数据源连接测试G2合成闭环不写回数据源(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	resetConnectionTestFacts(t, store)
	ctx := context.Background()
	created := requestSyntheticDataSourceConnectionTest(t, store, "connection-test-g2", "G2_SYNTHETIC")
	replayed, err := store.RequestDataSourceConnectionTest(ctx, created)
	if err != nil || !replayed.Replayed || replayed.ConnectionTestID != created.ConnectionTestID {
		t.Fatalf("RequestDataSourceConnectionTest() replay = %#v, %v", replayed, err)
	}
	run, err := store.GetDataSourceConnectionTestRun(ctx, created.ConnectionTestID)
	if err != nil || run.Status != "PENDING" || run.BindingAgentID != "agent-1" || run.NodeFactsRevision != 1 || len(run.ConnectionConfigDigest) != 64 || len(run.BindingDigest) != 64 {
		t.Fatalf("GetDataSourceConnectionTestRun() = %#v, %v", run, err)
	}

	grant := claimAndAcknowledgeDataSourceConnectionTest(t, store, run.ConnectionTestID, testTime.Add(time.Minute))
	completed, err := store.CompleteAgentDataSourceConnectionTest(ctx, AgentDataSourceConnectionTestCompletion{
		AgentID: "agent-1", ConnectionTestID: run.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-complete-g2", RequestDigest: strings.Repeat("c", 64),
		Status: "SUCCEEDED", EvidenceCode: "SYNTHETIC_OK", VerificationSource: "G2_SYNTHETIC",
		SysVerificationStatus: "NOT_CONFIGURED", Now: testTime.Add(3 * time.Minute),
	})
	if err != nil || completed.Status != "SUCCEEDED" || completed.Replayed {
		t.Fatalf("CompleteAgentDataSourceConnectionTest() = %#v, %v", completed, err)
	}
	replayedCompletion, err := store.CompleteAgentDataSourceConnectionTest(ctx, AgentDataSourceConnectionTestCompletion{
		AgentID: "agent-1", ConnectionTestID: run.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-complete-g2", RequestDigest: strings.Repeat("c", 64),
		Status: "SUCCEEDED", EvidenceCode: "SYNTHETIC_OK", VerificationSource: "G2_SYNTHETIC",
		SysVerificationStatus: "NOT_CONFIGURED", Now: testTime.Add(4 * time.Minute),
	})
	if err != nil || !replayedCompletion.Replayed || replayedCompletion.Status != "SUCCEEDED" {
		t.Fatalf("replayed CompleteAgentDataSourceConnectionTest() = %#v, %v", replayedCompletion, err)
	}

	var state, status, testedAt, summary, source string
	if err := store.db.QueryRowContext(ctx, `
        SELECT state, COALESCE(last_test_status, ''), COALESCE(last_tested_at, ''),
               COALESCE(last_test_safe_summary_json, ''), COALESCE(last_test_source, '')
        FROM data_sources WHERE data_source_id = 'source-1'
    `).Scan(&state, &status, &testedAt, &summary, &source); err != nil {
		t.Fatalf("read data source after G2 completion: %v", err)
	}
	if state != "DISABLED" || status != "" || testedAt != "" || summary != "" || source != "" {
		t.Fatalf("G2 completion changed data source facts: state=%q status=%q tested=%q summary=%q source=%q", state, status, testedAt, summary, source)
	}
	if _, err := store.ChangeDataSourceState(ctx, DataSourceStateChange{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", TargetState: "ENABLED", ExpectedRevision: 1,
		RequestID: "connection-test-enable-g2", ChangedAt: testTime.Add(5 * time.Minute),
	}); !errors.Is(err, ErrDataSourceConnectionTestRequired) {
		t.Fatalf("G2 completion unexpectedly enabled source: %v", err)
	}
}

func Test数据源连接测试允许禁用诊断节点但拒绝维护节点(t *testing.T) {
	t.Run("禁用节点", func(t *testing.T) {
		store, _ := openTestStore(t)
		seedBaseFixture(t, store)
		if _, err := store.db.Exec(`UPDATE execution_nodes SET management_state = 'DISABLED' WHERE node_id = 'node-1'`); err != nil {
			t.Fatalf("disable execution node: %v", err)
		}
		created := requestSyntheticDataSourceConnectionTest(t, store, "connection-test-disabled-node", "G2_SYNTHETIC")
		claimAndAcknowledgeDataSourceConnectionTest(t, store, created.ConnectionTestID, testTime.Add(time.Minute))
	})

	t.Run("维护节点", func(t *testing.T) {
		store, _ := openTestStore(t)
		seedBaseFixture(t, store)
		prepareConnectionTestAgentFacts(t, store)
		if _, err := store.db.Exec(`UPDATE execution_nodes SET management_state = 'MAINTENANCE' WHERE node_id = 'node-1'`); err != nil {
			t.Fatalf("put execution node into maintenance: %v", err)
		}
		input := syntheticDataSourceConnectionTestInput("connection-test-maintenance-node", "G2_SYNTHETIC")
		if _, err := store.RequestDataSourceConnectionTest(context.Background(), input); !errors.Is(err, ErrDataSourceConnectionTestInvalid) {
			t.Fatalf("RequestDataSourceConnectionTest() error = %v, want invalid", err)
		}
	})
}

func Test数据源连接测试首次冻结复验当前机器事实(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*Store)
	}{
		{
			name: "心跳过期",
			mutate: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE agents SET last_heartbeat_at = ? WHERE agent_id = 'agent-1'`, utcText(testTime.Add(-10*time.Minute))); err != nil {
					t.Fatalf("expire agent heartbeat: %v", err)
				}
			},
		},
		{
			name: "容量占满",
			mutate: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE agents SET capacity_used = capacity_total WHERE agent_id = 'agent-1'`); err != nil {
					t.Fatalf("fill agent capacity: %v", err)
				}
			},
		},
		{
			name: "平台不匹配",
			mutate: func(store *Store) {
				factsJSON, err := encodeAgentEnvironmentFacts(AgentEnvironmentFacts{
					OperatingSystem: "LINUX", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: testTime,
				})
				if err != nil {
					t.Fatalf("encode mismatched agent facts: %v", err)
				}
				if _, err := store.db.Exec(`UPDATE agents SET facts_json = ? WHERE agent_id = 'agent-1'`, factsJSON); err != nil {
					t.Fatalf("mismatch agent platform: %v", err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store, _ := openTestStore(t)
			seedBaseFixture(t, store)
			prepareConnectionTestAgentFacts(t, store)
			testCase.mutate(store)
			input := syntheticDataSourceConnectionTestInput("connection-test-current-facts-"+testCase.name, "G2_SYNTHETIC")
			if _, err := store.RequestDataSourceConnectionTest(context.Background(), input); !errors.Is(err, ErrDataSourceConnectionTestInvalid) {
				t.Fatalf("RequestDataSourceConnectionTest() error = %v, want invalid", err)
			}
		})
	}
}

func Test数据源连接测试漂移会失效并拒绝领取(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(*Store)
	}{
		{
			name: "Agent事实版本变化",
			apply: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE agents SET facts_revision = 2 WHERE agent_id = 'agent-1'`); err != nil {
					t.Fatalf("update agent facts: %v", err)
				}
			},
		},
		{
			name: "连接配置变化",
			apply: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE data_sources SET host = '127.0.0.9' WHERE data_source_id = 'source-1'`); err != nil {
					t.Fatalf("update data source connection configuration: %v", err)
				}
			},
		},
		{
			name: "节点进入维护",
			apply: func(store *Store) {
				if _, err := store.db.Exec(`UPDATE execution_nodes SET management_state = 'MAINTENANCE' WHERE node_id = 'node-1'`); err != nil {
					t.Fatalf("put execution node into maintenance: %v", err)
				}
			},
		},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			store, _ := openTestStore(t)
			seedBaseFixture(t, store)
			created := requestSyntheticDataSourceConnectionTest(t, store, "connection-test-drift-"+strings.ReplaceAll(mutate.name, "", "x"), "G2_SYNTHETIC")
			mutate.apply(store)
			_, found, err := store.ClaimNextDataSourceConnectionTest(context.Background(), DataSourceConnectionTestClaimNext{
				AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-drift", RequestID: "connection-test-claim-drift",
				RequestDigest: strings.Repeat("d", 64), LeaseTTL: time.Minute, Now: testTime.Add(time.Minute),
			})
			if !errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) || found {
				t.Fatalf("ClaimNextDataSourceConnectionTest() found=%t error=%v, want false and lease rejection", found, err)
			}
			run, err := store.GetDataSourceConnectionTestRun(context.Background(), created.ConnectionTestID)
			if err != nil || run.Status != "INVALIDATED" || !run.CompletedAt.Equal(testTime.Add(time.Minute)) {
				t.Fatalf("invalidated run = %#v, %v", run, err)
			}
			assertCount(t, store.db, `SELECT COUNT(*) FROM agent_data_source_connection_test_receipts`, 0)
		})
	}
}

func Test数据源连接测试Agent回执重放和冲突失败关闭(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	created := requestSyntheticDataSourceConnectionTest(t, store, "connection-test-receipts", "G2_SYNTHETIC")
	claimInput := DataSourceConnectionTestClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-receipts", RequestID: "connection-test-claim-receipts",
		RequestDigest: strings.Repeat("4", 64), LeaseTTL: 3 * time.Minute, Now: testTime.Add(time.Minute),
	}
	grant, found, err := store.ClaimNextDataSourceConnectionTest(ctx, claimInput)
	if err != nil || !found {
		t.Fatalf("ClaimNextDataSourceConnectionTest() = %#v, %t, %v", grant, found, err)
	}
	replayedGrant, found, err := store.ClaimNextDataSourceConnectionTest(ctx, claimInput)
	if err != nil || !found || !replayedGrant.Replayed || replayedGrant.LeaseID != grant.LeaseID {
		t.Fatalf("replayed ClaimNextDataSourceConnectionTest() = %#v, %t, %v", replayedGrant, found, err)
	}
	conflictingClaim := claimInput
	conflictingClaim.RequestDigest = strings.Repeat("5", 64)
	if _, _, err := store.ClaimNextDataSourceConnectionTest(ctx, conflictingClaim); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting ClaimNextDataSourceConnectionTest() error = %v, want idempotency conflict", err)
	}

	ack := DataSourceConnectionTestAcknowledgement{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-ack-receipts", RequestDigest: strings.Repeat("6", 64), Now: testTime.Add(2 * time.Minute),
	}
	acknowledged, err := store.AcknowledgeDataSourceConnectionTest(ctx, ack)
	if err != nil || acknowledged.Replayed {
		t.Fatalf("AcknowledgeDataSourceConnectionTest() = %#v, %v", acknowledged, err)
	}
	replayedAck, err := store.AcknowledgeDataSourceConnectionTest(ctx, ack)
	if err != nil || !replayedAck.Replayed || replayedAck.LeaseID != grant.LeaseID {
		t.Fatalf("replayed AcknowledgeDataSourceConnectionTest() = %#v, %v", replayedAck, err)
	}
	conflictingAck := ack
	conflictingAck.RequestDigest = strings.Repeat("7", 64)
	if _, err := store.AcknowledgeDataSourceConnectionTest(ctx, conflictingAck); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting AcknowledgeDataSourceConnectionTest() error = %v, want idempotency conflict", err)
	}
}

func Test数据源连接测试槽位解析需要确认且可安全重试(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	created := requestSyntheticDataSourceConnectionTest(t, store, "connection-test-secret", "G2_SYNTHETIC")
	claim, found, err := store.ClaimNextDataSourceConnectionTest(ctx, DataSourceConnectionTestClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-secret", RequestID: "connection-test-claim-secret",
		RequestDigest: strings.Repeat("e", 64), LeaseTTL: 3 * time.Minute, Now: testTime.Add(time.Minute),
	})
	if err != nil || !found {
		t.Fatalf("ClaimNextDataSourceConnectionTest() = %#v, %t, %v", claim, found, err)
	}
	request := DataSourceConnectionTestSecretResolutionRequest{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
		BindingDigest: claim.Binding.BindingDigest, RequestID: "connection-test-secret-request", RequestDigest: strings.Repeat("f", 64), Now: testTime.Add(2 * time.Minute),
	}
	if _, err := store.ResolveDataSourceConnectionTestDatabaseConnection(ctx, request); !errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
		t.Fatalf("ResolveDataSourceConnectionTestDatabaseConnection() without ack error = %v", err)
	}
	if _, err := store.AcknowledgeDataSourceConnectionTest(ctx, DataSourceConnectionTestAcknowledgement{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
		BindingDigest: claim.Binding.BindingDigest, RequestID: "connection-test-ack-secret", RequestDigest: strings.Repeat("1", 64), Now: testTime.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("AcknowledgeDataSourceConnectionTest() = %v", err)
	}
	connection, err := store.ResolveDataSourceConnectionTestDatabaseConnection(ctx, request)
	if err != nil || connection.NodeID != "node-1" || connection.OwnerSubjectID != "subject-1" || len(connection.Ciphertext) == 0 ||
		!bytes.Equal(connection.Username, []byte("synthetic_user@synthetic-tenant#synthetic-cluster")) {
		t.Fatalf("ResolveDataSourceConnectionTestDatabaseConnection() = %#v, %v", connection, err)
	}
	connection.Destroy()
	if len(connection.Username) != 0 || len(connection.Ciphertext) != 0 || len(connection.Nonce) != 0 {
		t.Fatal("Destroy() did not clear connection test slot buffers")
	}
	if err := store.FinishDataSourceConnectionTestSecretResolution(ctx, DataSourceConnectionTestSecretResolutionOutcome{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
		BindingDigest: claim.Binding.BindingDigest, RequestID: request.RequestID, RequestDigest: request.RequestDigest, Succeeded: false, Now: testTime.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("FinishDataSourceConnectionTestSecretResolution(false) = %v", err)
	}
	connection, err = store.ResolveDataSourceConnectionTestDatabaseConnection(ctx, request)
	if err != nil {
		t.Fatalf("retry ResolveDataSourceConnectionTestDatabaseConnection() = %v", err)
	}
	connection.Destroy()
	if err := store.FinishDataSourceConnectionTestSecretResolution(ctx, DataSourceConnectionTestSecretResolutionOutcome{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
		BindingDigest: claim.Binding.BindingDigest, RequestID: request.RequestID, RequestDigest: request.RequestDigest, Succeeded: true, Now: testTime.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("FinishDataSourceConnectionTestSecretResolution(true) = %v", err)
	}
	var receiptStatus string
	if err := store.db.QueryRow(`
        SELECT status FROM agent_data_source_connection_test_secret_resolution_receipts
        WHERE agent_id = 'agent-1' AND request_id = 'connection-test-secret-request'
    `).Scan(&receiptStatus); err != nil || receiptStatus != "RESOLVED" {
		t.Fatalf("secret resolution receipt = %q, %v", receiptStatus, err)
	}
}

func Test数据源连接测试AgentJDBC写回与过期失败关闭(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	resetConnectionTestFacts(t, store)
	ctx := context.Background()
	created := requestSyntheticDataSourceConnectionTest(t, store, "connection-test-jdbc", "AGENT_JDBC")
	grant := claimAndAcknowledgeDataSourceConnectionTest(t, store, created.ConnectionTestID, testTime.Add(time.Minute))
	completed, err := store.CompleteAgentDataSourceConnectionTest(ctx, AgentDataSourceConnectionTestCompletion{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-complete-jdbc", RequestDigest: strings.Repeat("2", 64),
		Status: "SUCCEEDED", EvidenceCode: "DATABASE_CONNECTED", VerificationSource: "AGENT_JDBC",
		SysVerificationStatus: "NOT_CONFIGURED", Now: testTime.Add(3 * time.Minute),
	})
	if err != nil || completed.Status != "SUCCEEDED" {
		t.Fatalf("CompleteAgentDataSourceConnectionTest(AGENT_JDBC) = %#v, %v", completed, err)
	}
	var status, source, summary string
	var testedAt sql.NullString
	if err := store.db.QueryRow(`
        SELECT COALESCE(last_test_status, ''), COALESCE(last_test_source, ''),
               COALESCE(last_test_safe_summary_json, ''), last_tested_at
        FROM data_sources WHERE data_source_id = 'source-1'
    `).Scan(&status, &source, &summary, &testedAt); err != nil {
		t.Fatalf("read AGENT_JDBC writeback: %v", err)
	}
	if status != "SUCCEEDED" || source != "AGENT_JDBC" || !testedAt.Valid || !strings.Contains(summary, "DATABASE_CONNECTED") {
		t.Fatalf("AGENT_JDBC writeback status=%q source=%q tested=%q summary=%q", status, source, testedAt.String, summary)
	}
	created = requestSyntheticDataSourceConnectionTest(t, store, "connection-test-expired", "G2_SYNTHETIC")
	grant = claimAndAcknowledgeDataSourceConnectionTest(t, store, created.ConnectionTestID, testTime.Add(5*time.Minute))
	expiredAt := grant.ExpiresAt
	expired, err := store.CompleteAgentDataSourceConnectionTest(ctx, AgentDataSourceConnectionTestCompletion{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-complete-expired", RequestDigest: strings.Repeat("3", 64),
		Status: "SUCCEEDED", EvidenceCode: "SYNTHETIC_OK", VerificationSource: "G2_SYNTHETIC",
		SysVerificationStatus: "NOT_CONFIGURED", Now: expiredAt,
	})
	if !errors.Is(err, ErrDataSourceConnectionTestLeaseExpired) || expired.Status != "EXPIRED" {
		t.Fatalf("late CompleteAgentDataSourceConnectionTest() = %#v, %v", expired, err)
	}
	lateRun, err := store.GetDataSourceConnectionTestRun(ctx, created.ConnectionTestID)
	if err != nil || lateRun.Status != "EXPIRED" {
		t.Fatalf("expired connection test run = %#v, %v", lateRun, err)
	}
	if _, err := store.ChangeDataSourceState(ctx, DataSourceStateChange{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", TargetState: "ENABLED", ExpectedRevision: 1,
		RequestID: "connection-test-enable-jdbc", ChangedAt: testTime.Add(9 * time.Minute),
	}); err != nil {
		t.Fatalf("ChangeDataSourceState() after AGENT_JDBC success = %v", err)
	}
}

func Test数据源连接测试引用使数据源归档(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	requestSyntheticDataSourceConnectionTest(t, store, "connection-test-source-history", "G2_SYNTHETIC")
	result, err := store.ArchiveDataSource(context.Background(), DataSourceArchive{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		RequestID: "connection-test-delete-source", ArchivedAt: testTime.Add(time.Minute),
	})
	if err != nil || result.Outcome != "ARCHIVED" {
		t.Fatalf("ArchiveDataSource() = %#v, %v", result, err)
	}
	assertCount(t, store.db, `SELECT COUNT(*) FROM data_source_connection_test_runs WHERE data_source_id = 'source-1'`, 1)
}

func requestSyntheticDataSourceConnectionTest(t *testing.T, store *Store, connectionTestID, verificationSource string) DataSourceConnectionTestCreate {
	t.Helper()
	prepareConnectionTestAgentFacts(t, store)
	input := syntheticDataSourceConnectionTestInput(connectionTestID, verificationSource)
	if _, err := store.RequestDataSourceConnectionTest(context.Background(), input); err != nil {
		t.Fatalf("RequestDataSourceConnectionTest(%q) = %v", connectionTestID, err)
	}
	return input
}

func syntheticDataSourceConnectionTestInput(connectionTestID, verificationSource string) DataSourceConnectionTestCreate {
	return DataSourceConnectionTestCreate{
		ConnectionTestID: connectionTestID, DataSourceID: "source-1", CreatorSubjectID: "subject-1", ExpectedDataSourceRevision: 1,
		NodeID: "node-1", VerificationSource: verificationSource, RequestID: "connection-test-request-" + connectionTestID,
		IdempotencyKey: "connection-test-idempotency-" + connectionTestID, RequestDigest: strings.Repeat("a", 64),
		CreatedAt: testTime, HeartbeatFreshAfter: testTime.Add(-5 * time.Minute), ValidUntil: testTime.Add(10 * time.Minute),
	}
}

func prepareConnectionTestAgentFacts(t *testing.T, store *Store) {
	t.Helper()
	factsJSON, err := encodeAgentEnvironmentFacts(AgentEnvironmentFacts{
		OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "agent-test-v1", ObservedAt: testTime,
	})
	if err != nil {
		t.Fatalf("encode connection test agent facts: %v", err)
	}
	if _, err := store.db.Exec(`
        UPDATE agents
        SET last_heartbeat_at = ?, capacity_total = 1, capacity_used = 0, facts_json = ?
        WHERE agent_id = 'agent-1'
    `, utcText(testTime), factsJSON); err != nil {
		t.Fatalf("prepare connection test agent facts: %v", err)
	}
}

func claimAndAcknowledgeDataSourceConnectionTest(t *testing.T, store *Store, connectionTestID string, now time.Time) DataSourceConnectionTestLeaseGrant {
	t.Helper()
	claim, found, err := store.ClaimNextDataSourceConnectionTest(context.Background(), DataSourceConnectionTestClaimNext{
		AgentID: "agent-1", NodeID: "node-1", LeaseID: "lease-" + connectionTestID, RequestID: "connection-test-claim-" + connectionTestID,
		RequestDigest: strings.Repeat("b", 64), LeaseTTL: 3 * time.Minute, Now: now,
	})
	if err != nil || !found || claim.ConnectionTestID != connectionTestID {
		t.Fatalf("ClaimNextDataSourceConnectionTest(%q) = %#v, %t, %v", connectionTestID, claim, found, err)
	}
	if _, err := store.AcknowledgeDataSourceConnectionTest(context.Background(), DataSourceConnectionTestAcknowledgement{
		AgentID: "agent-1", ConnectionTestID: connectionTestID, LeaseID: claim.LeaseID, LeaseEpoch: claim.LeaseEpoch,
		BindingDigest: claim.Binding.BindingDigest, RequestID: "connection-test-ack-" + connectionTestID,
		RequestDigest: strings.Repeat("9", 64), Now: now,
	}); err != nil {
		t.Fatalf("AcknowledgeDataSourceConnectionTest(%q) = %v", connectionTestID, err)
	}
	return claim
}

func resetConnectionTestFacts(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`
        UPDATE data_sources
        SET state = 'DISABLED', last_test_status = NULL, last_tested_at = NULL,
            last_test_safe_summary_json = NULL, last_test_source = NULL
        WHERE data_source_id = 'source-1'
    `); err != nil {
		t.Fatalf("reset data source connection test facts: %v", err)
	}
}

// Test数据源连接测试sys凭据槽位冻结解析与结果回写 验证可选的 sys 凭据（参考 ODC 的 sys 账号验证）：
// 配置 sys 凭据后连接测试冻结 sys 引用，Agent 可解析 SYS_CONNECTION 槽位并回写独立 sys 结果。
func Test数据源连接测试sys凭据槽位冻结解析与结果回写(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	// 给 source-1 配置可选的 sys 凭据（修订 1）。
	if _, err := store.UpdateDataSource(ctx, DataSourceUpdate{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 1,
		DisplayName: "Synthetic Source", NormalizedName: "synthetic source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic_user",
		SysUser:     "root",
		SysPassword: &EncryptedDataSourcePassword{CredentialID: "sys-credential-1", Revision: 1, KeyID: "key-1", Nonce: []byte{7}, Ciphertext: []byte{8}},
		RequestID:   "request-sys-test", UpdatedAt: testTime,
	}); err != nil {
		t.Fatalf("UpdateDataSource(sys) = %v", err)
	}
	// 创建连接测试并领取：绑定必须冻结 sys 引用（数据源已更新为修订 2）。
	prepareConnectionTestAgentFacts(t, store)
	sysTestInput := syntheticDataSourceConnectionTestInput("connection-test-sys", "AGENT_JDBC")
	sysTestInput.ExpectedDataSourceRevision = 2
	if _, err := store.RequestDataSourceConnectionTest(ctx, sysTestInput); err != nil {
		t.Fatalf("RequestDataSourceConnectionTest(sys) = %v", err)
	}
	created := sysTestInput
	grant := claimAndAcknowledgeDataSourceConnectionTest(t, store, created.ConnectionTestID, testTime.Add(time.Minute))
	if grant.Binding.SysCredentialID != "sys-credential-1" || grant.Binding.SysCredentialRevision != 1 {
		t.Fatalf("connection test binding lacks sys reference: %#v", grant.Binding)
	}
	// 解析 SYS_CONNECTION 槽位：返回加密 sys 材料（不能有明文）。
	encrypted, err := store.ResolveDataSourceConnectionTestSysCredential(ctx, DataSourceConnectionTestSecretResolutionRequest{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-sys-resolve", RequestDigest: strings.Repeat("d", 64), Now: testTime.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ResolveDataSourceConnectionTestSysCredential() = %v", err)
	}
	defer encrypted.Destroy()
	if encrypted.CredentialID != "sys-credential-1" || encrypted.Revision != 1 || encrypted.OwnerSubjectID != "subject-1" ||
		string(encrypted.Username) != "root@sys#synthetic-cluster" || len(encrypted.Ciphertext) == 0 {
		t.Fatalf("sys credential resolution = %#v", encrypted)
	}
	if err := store.FinishDataSourceConnectionTestSecretResolution(ctx, DataSourceConnectionTestSecretResolutionOutcome{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-sys-resolve", RequestDigest: strings.Repeat("d", 64), Succeeded: true, Now: testTime.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("FinishDataSourceConnectionTestSecretResolution() = %v", err)
	}
	// 回写数据库成功 + sys 验证成功（相互独立）。
	if _, err := store.CompleteAgentDataSourceConnectionTest(ctx, AgentDataSourceConnectionTestCompletion{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-sys-mismatch", RequestDigest: strings.Repeat("a", 64),
		Status: "SUCCEEDED", EvidenceCode: "DATABASE_CONNECTED", VerificationSource: "AGENT_JDBC",
		SysVerificationStatus: "NOT_CONFIGURED", Now: testTime.Add(3 * time.Minute),
	}); !errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
		t.Fatalf("configured sys completion mismatch error = %v, want ErrDataSourceConnectionTestLeaseRejected", err)
	}
	completed, err := store.CompleteAgentDataSourceConnectionTest(ctx, AgentDataSourceConnectionTestCompletion{
		AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch,
		BindingDigest: grant.Binding.BindingDigest, RequestID: "connection-test-sys-complete", RequestDigest: strings.Repeat("e", 64),
		Status: "SUCCEEDED", EvidenceCode: "DATABASE_CONNECTED", VerificationSource: "AGENT_JDBC",
		SysVerificationStatus: "SUCCEEDED", SysResultCode: "SYS_CONNECTED", Now: testTime.Add(3 * time.Minute),
	})
	if err != nil || completed.Status != "SUCCEEDED" {
		t.Fatalf("CompleteAgentDataSourceConnectionTest(sys) = %#v, %v", completed, err)
	}
	run, err := store.GetDataSourceConnectionTestRun(ctx, created.ConnectionTestID)
	if err != nil || run.SysVerificationStatus != "SUCCEEDED" || run.SysResultCode != "SYS_CONNECTED" || run.SysCredentialRevision != 1 {
		t.Fatalf("connection test run sys projection = %#v, %v", run, err)
	}
	// 领取后轮换 sys 凭据会使冻结绑定失效，旧租约不能再提交结果。
	staleInput := syntheticDataSourceConnectionTestInput("connection-test-sys-stale", "AGENT_JDBC")
	staleInput.ExpectedDataSourceRevision = 2
	staleInput.CreatedAt = testTime.Add(4 * time.Minute)
	staleInput.ValidUntil = testTime.Add(20 * time.Minute)
	if _, err := store.RequestDataSourceConnectionTest(ctx, staleInput); err != nil {
		t.Fatalf("RequestDataSourceConnectionTest(stale sys) = %v", err)
	}
	staleGrant := claimAndAcknowledgeDataSourceConnectionTest(t, store, staleInput.ConnectionTestID, testTime.Add(5*time.Minute))
	if _, err := store.UpdateDataSource(ctx, DataSourceUpdate{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", ExpectedRevision: 2,
		DisplayName: "Synthetic Source", NormalizedName: "synthetic source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.1", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic_user",
		SysUser:     "root",
		SysPassword: &EncryptedDataSourcePassword{CredentialID: "sys-credential-1", Revision: 2, KeyID: "key-1", Nonce: []byte{9}, Ciphertext: []byte{10}},
		RequestID:   "request-sys-rotate-stale", UpdatedAt: testTime.Add(6 * time.Minute),
	}); err != nil {
		t.Fatalf("UpdateDataSource(rotate stale sys) = %v", err)
	}
	if _, err := store.CompleteAgentDataSourceConnectionTest(ctx, AgentDataSourceConnectionTestCompletion{
		AgentID: "agent-1", ConnectionTestID: staleInput.ConnectionTestID, LeaseID: staleGrant.LeaseID, LeaseEpoch: staleGrant.LeaseEpoch,
		BindingDigest: staleGrant.Binding.BindingDigest, RequestID: "connection-test-sys-stale-complete", RequestDigest: strings.Repeat("b", 64),
		Status: "SUCCEEDED", EvidenceCode: "DATABASE_CONNECTED", VerificationSource: "AGENT_JDBC",
		SysVerificationStatus: "SUCCEEDED", SysResultCode: "SYS_CONNECTED", Now: testTime.Add(7 * time.Minute),
	}); !errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
		t.Fatalf("stale sys binding completion error = %v, want ErrDataSourceConnectionTestLeaseRejected", err)
	}
	staleRun, err := store.GetDataSourceConnectionTestRun(ctx, staleInput.ConnectionTestID)
	if err != nil || staleRun.Status != "INVALIDATED" {
		t.Fatalf("stale sys binding run = %#v, %v", staleRun, err)
	}
	// 未配置 sys 凭据的数据源：绑定不冻结 sys 引用，解析 SYS_CONNECTION 槽位必须失败关闭。
	plainSource := DataSourceCreate{
		DataSourceID: "source-plain", CredentialID: "credential-plain", CreatorSubjectID: "subject-1",
		DisplayName: "Plain Source", NormalizedName: "plain source", Environment: "TEST",
		ConnectionKind: "ODP", CompatibilityMode: "MYSQL", Host: "127.0.0.5", Port: 2881,
		ClusterName: "synthetic-cluster", TenantName: "synthetic-tenant", Username: "synthetic-user", KeyID: "key-plain",
		Nonce: []byte{1}, Ciphertext: []byte{2}, RequestID: "request-plain",
		IdempotencyKey: "idempotency-plain", RequestDigest: testFingerprint, CreatedAt: testTime,
	}
	if _, err := store.CreateDataSource(ctx, plainSource); err != nil {
		t.Fatalf("CreateDataSource(plain) = %v", err)
	}
	prepareConnectionTestAgentFacts(t, store)
	plainInput := syntheticDataSourceConnectionTestInput("connection-test-plain", "AGENT_JDBC")
	plainInput.DataSourceID = "source-plain"
	if _, err := store.RequestDataSourceConnectionTest(ctx, plainInput); err != nil {
		t.Fatalf("RequestDataSourceConnectionTest(plain) = %v", err)
	}
	plain := plainInput
	plainGrant := claimAndAcknowledgeDataSourceConnectionTest(t, store, plain.ConnectionTestID, testTime.Add(6*time.Minute))
	if plainGrant.Binding.SysCredentialRevision != 0 {
		t.Fatalf("plain binding unexpectedly has sys reference: %#v", plainGrant.Binding)
	}
	if _, err := store.ResolveDataSourceConnectionTestSysCredential(ctx, DataSourceConnectionTestSecretResolutionRequest{
		AgentID: "agent-1", ConnectionTestID: plain.ConnectionTestID, LeaseID: plainGrant.LeaseID, LeaseEpoch: plainGrant.LeaseEpoch,
		BindingDigest: plainGrant.Binding.BindingDigest, RequestID: "connection-test-plain-resolve", RequestDigest: strings.Repeat("f", 64), Now: testTime.Add(7 * time.Minute),
	}); !errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
		t.Fatalf("sys resolution without sys reference error = %v, want ErrDataSourceConnectionTestLeaseRejected", err)
	}
}
