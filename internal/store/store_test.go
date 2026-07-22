package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var testTime = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

func TestOpenConfiguresSchemaAndCreatesVerifiedBackup(t *testing.T) {
	store, databasePath := openTestStore(t)
	seedBaseFixture(t, store)
	if err := store.SubmitTask(context.Background(), validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}

	var foreignKeys, busyTimeout, synchronous int
	var journalMode string
	if err := store.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if err := store.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if err := store.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("read synchronous: %v", err)
	}
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if foreignKeys != 1 || busyTimeout != 5000 || synchronous != 2 || !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("unexpected pragmas foreign_keys=%d busy_timeout=%d synchronous=%d journal_mode=%s", foreignKeys, busyTimeout, synchronous, journalMode)
	}

	backupPath := filepath.Join(filepath.Dir(databasePath), "metadata-backup.db")
	if err := store.Backup(context.Background(), backupPath); err != nil {
		t.Fatalf("Backup(): %v", err)
	}
	if err := store.Backup(context.Background(), backupPath); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Backup() overwrite error = %v", err)
	}
	backup, err := sql.Open("sqlite", sqliteReadOnlyDSN(backupPath))
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = backup.Close() })
	var taskCount int
	if err := backup.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&taskCount); err != nil {
		t.Fatalf("count backup tasks: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("backup task count = %d, want 1", taskCount)
	}
	restored, err := Open(context.Background(), backupPath)
	if err != nil {
		t.Fatalf("open restored backup: %v", err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	assertCount(t, restored.db, "SELECT COUNT(*) FROM tasks", 1)
}

func TestUpdateDraftUsesOptimisticRevisionAndRejectsSecretJSON(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)

	update := DraftUpdate{
		DraftID:           "draft-1",
		ExpectedRevision:  1,
		ConfigJSON:        `{"database":"synthetic_db","table":"synthetic_table"}`,
		ConfigFingerprint: strings.Repeat("b", 64),
		InvalidationJSON:  `{}`,
		UpdatedAt:         testTime.Add(time.Minute),
	}
	revision, err := store.UpdateDraft(context.Background(), update)
	if err != nil || revision != 2 {
		t.Fatalf("UpdateDraft() = %d, %v; want 2, nil", revision, err)
	}
	if _, err := store.UpdateDraft(context.Background(), update); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale UpdateDraft() error = %v, want ErrRevisionConflict", err)
	}

	update.ExpectedRevision = 2
	update.ConfigJSON = `{"password":"synthetic-secret"}`
	if _, err := store.UpdateDraft(context.Background(), update); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret draft JSON error = %v", err)
	}
}

func TestSubmitTaskIsImmutableAllowsEqualFingerprintAndRollsBackAuditFailure(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()

	if err := store.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(task-1): %v", err)
	}
	second := validTaskSubmission("task-2")
	second.RequestID = "request-submit-2"
	if err := store.SubmitTask(ctx, second); err != nil {
		t.Fatalf("SubmitTask(task-2): %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET snapshot_json = '{}' WHERE task_id = 'task-1'`); err == nil {
		t.Fatal("immutable task update unexpectedly succeeded")
	}
	unsafeArgv := validTaskSubmission("task-unsafe")
	unsafeArgv.PlannedArgvJSON = `["--password","not-allowed"]`
	if err := store.SubmitTask(ctx, unsafeArgv); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("unsafe planned argv error = %v", err)
	}

	rollback := validTaskSubmission("task-rollback")
	rollback.RequestID = "request-rollback"
	if _, err := store.db.ExecContext(ctx, `
        INSERT INTO audit_events(
            audit_event_id, actor_type, actor_id, action, object_type, object_id,
            result, request_id, safe_diff_json, occurred_at
        ) VALUES (?, 'SYSTEM', 'test', 'TEST', 'TASK', ?, 'SUCCEEDED', ?, '{}', ?)
    `, auditID(rollback.TaskID, rollback.RequestID), rollback.TaskID, rollback.RequestID, utcText(testTime)); err != nil {
		t.Fatalf("seed conflicting audit event: %v", err)
	}
	if err := store.SubmitTask(ctx, rollback); err == nil {
		t.Fatal("SubmitTask() with conflicting audit event unexpectedly succeeded")
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM tasks", 2)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events", 3)
}

func TestClaimTaskIsAtomicAndConcurrent(t *testing.T) {
	primary, databasePath := openTestStore(t)
	seedBaseFixture(t, primary)
	if err := primary.SubmitTask(context.Background(), validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	secondary, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	t.Cleanup(func() { _ = secondary.Close() })

	claims := []Claim{
		validClaim("execution-1", "lease-1", "event-claim-1", "request-claim-1"),
		validClaim("execution-2", "lease-2", "event-claim-2", "request-claim-2"),
	}
	stores := []*Store{primary, secondary}
	results := make(chan error, len(claims))
	var ready sync.WaitGroup
	ready.Add(len(claims))
	start := make(chan struct{})
	for index := range claims {
		go func(index int) {
			ready.Done()
			<-start
			results <- stores[index].ClaimTask(context.Background(), claims[index])
		}(index)
	}
	ready.Wait()
	close(start)

	var succeeded, alreadyClaimed int
	for range claims {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAlreadyClaimed):
			alreadyClaimed++
		default:
			t.Fatalf("ClaimTask() error = %v", err)
		}
	}
	if succeeded != 1 || alreadyClaimed != 1 {
		t.Fatalf("claim outcomes succeeded=%d alreadyClaimed=%d", succeeded, alreadyClaimed)
	}
	assertCount(t, primary.db, "SELECT COUNT(*) FROM task_executions", 1)
	assertCount(t, primary.db, "SELECT COUNT(*) FROM execution_leases", 1)
	assertCount(t, primary.db, "SELECT COUNT(*) FROM execution_events", 1)
	assertCount(t, primary.db, "SELECT COUNT(*) FROM audit_events", 2)
}

func TestAppendExecutionEventRejectsDuplicateAndWrongLease(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if err := store.SubmitTask(ctx, validTaskSubmission("task-1")); err != nil {
		t.Fatalf("SubmitTask(): %v", err)
	}
	claim := validClaim("execution-1", "lease-1", "event-claim-1", "request-claim-1")
	if err := store.ClaimTask(ctx, claim); err != nil {
		t.Fatalf("ClaimTask(): %v", err)
	}

	event := ExecutionEvent{
		EventID:     "event-2",
		ExecutionID: claim.ExecutionID,
		LeaseID:     claim.LeaseID,
		LeaseEpoch:  claim.LeaseEpoch,
		EventSeq:    2,
		EventType:   "PROCESS_STARTED",
		PayloadJSON: `{"pid":42}`,
		ReceivedAt:  testTime.Add(3 * time.Minute),
	}
	if err := store.AppendExecutionEvent(ctx, event); err != nil {
		t.Fatalf("AppendExecutionEvent(): %v", err)
	}
	duplicate := event
	duplicate.EventID = "event-duplicate"
	if err := store.AppendExecutionEvent(ctx, duplicate); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("duplicate event error = %v, want ErrEventRejected", err)
	}
	wrongLease := event
	wrongLease.EventID = "event-wrong-lease"
	wrongLease.EventSeq = 3
	wrongLease.LeaseID = "lease-not-owned"
	if err := store.AppendExecutionEvent(ctx, wrongLease); !errors.Is(err, ErrEventRejected) {
		t.Fatalf("wrong lease event error = %v, want ErrEventRejected", err)
	}
	secretPayload := event
	secretPayload.EventID = "event-secret"
	secretPayload.EventSeq = 3
	secretPayload.PayloadJSON = `{"password":"synthetic-secret"}`
	if err := store.AppendExecutionEvent(ctx, secretPayload); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret payload error = %v", err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM execution_events", 2)
}

func TestListDataSourceSummariesExcludesCredentialMaterial(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
        INSERT INTO data_sources VALUES (?, ?, ?, 'TEST', 'OBSERVER_DIRECT', 'MYSQL', ?, 2882, ?, ?, ?, 1, 'ARCHIVED', 1, NULL, NULL, NULL, ?, ?, ?)
    `, "source-archived", "Archived", "archived", "127.0.0.2", "synthetic_user", "synthetic_db", "credential-archived", "subject-1", utcText(testTime), utcText(testTime)); err != nil {
		t.Fatalf("seed archived data source: %v", err)
	}
	summaries, err := store.ListDataSourceSummaries(ctx)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("ListDataSourceSummaries() = %#v, %v", summaries, err)
	}
	summary := summaries[0]
	if summary.DataSourceID != "source-1" || summary.CredentialRevision != 1 || summary.UpdatedAt.IsZero() {
		t.Fatalf("unexpected data source summary: %#v", summary)
	}
	detail, err := store.GetDataSourceSummary(ctx, "source-1")
	if err != nil || detail.DataSourceID != summary.DataSourceID {
		t.Fatalf("GetDataSourceSummary(source-1) = %#v, %v", detail, err)
	}
	if _, err := store.GetDataSourceSummary(ctx, "source-archived"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("GetDataSourceSummary(source-archived) error = %v", err)
	}
	serialized, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, forbidden := range []string{"key-1", "nonce", "ciphertext", "synthetic-secret"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("summary contains credential material %q: %s", forbidden, serialized)
		}
	}
}

func TestCreateDataSourceAtomicallyPersistsEncryptedCredentialAuditAndIdempotency(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	input := DataSourceCreate{
		DataSourceID: "source-create", CredentialID: "credential-create", CreatorSubjectID: "subject-1",
		DisplayName: "Created Source", NormalizedName: "created-source", Environment: "TEST",
		ConnectionKind: "OBSERVER_DIRECT", CompatibilityMode: "MYSQL", Host: "127.0.0.3", Port: 2881,
		Username: "synthetic-user", DefaultDatabase: "synthetic_db", KeyID: "key-create",
		Nonce: []byte{1, 2, 3}, Ciphertext: []byte{4, 5, 6}, RequestID: "request-create-1",
		IdempotencyKey: "idempotency-create-1", RequestDigest: testFingerprint, CreatedAt: testTime,
	}
	created, err := store.CreateDataSource(context.Background(), input)
	if err != nil || created.DataSourceID != input.DataSourceID || created.Replayed {
		t.Fatalf("CreateDataSource() = %#v, %v", created, err)
	}
	replay, err := store.CreateDataSource(context.Background(), input)
	if err != nil || !replay.Replayed || replay.DataSourceID != input.DataSourceID {
		t.Fatalf("replayed CreateDataSource() = %#v, %v", replay, err)
	}
	conflict := input
	conflict.RequestDigest = strings.Repeat("b", 64)
	if _, err := store.CreateDataSource(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting CreateDataSource() error = %v", err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM data_sources", 2)
	assertCount(t, store.db, "SELECT COUNT(*) FROM credential_revisions WHERE credential_id = 'credential-create'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_CREATED'", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM request_idempotency WHERE operation = 'CREATE_DATA_SOURCE'", 1)
}

func TestChangeDataSourceStateIsAtomicAndIdempotent(t *testing.T) {
	store, _ := openTestStore(t)
	seedBaseFixture(t, store)
	input := DataSourceStateChange{
		DataSourceID: "source-1", ActorSubjectID: "subject-1", TargetState: "DISABLED",
		ExpectedRevision: 1, RequestID: "request-disable-1", ChangedAt: testTime.Add(time.Minute),
	}
	changed, err := store.ChangeDataSourceState(context.Background(), input)
	if err != nil || changed.State != "DISABLED" || changed.Revision != 2 || changed.Replayed {
		t.Fatalf("ChangeDataSourceState() = %#v, %v", changed, err)
	}
	replayed, err := store.ChangeDataSourceState(context.Background(), input)
	if err != nil || !replayed.Replayed || replayed.Revision != 2 {
		t.Fatalf("replayed ChangeDataSourceState() = %#v, %v", replayed, err)
	}
	var state string
	var revision int64
	if err := store.db.QueryRow("SELECT state, revision FROM data_sources WHERE data_source_id = 'source-1'").Scan(&state, &revision); err != nil {
		t.Fatalf("read changed data source: %v", err)
	}
	if state != "DISABLED" || revision != 2 {
		t.Fatalf("stored state=%s revision=%d", state, revision)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_DISABLED'", 1)
	missing := input
	missing.DataSourceID = "source-missing"
	missing.RequestID = "request-disable-missing"
	if _, err := store.ChangeDataSourceState(context.Background(), missing); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("missing ChangeDataSourceState() error = %v", err)
	}
	stale := input
	stale.TargetState = "ENABLED"
	stale.RequestID = "request-enable-stale"
	if _, err := store.ChangeDataSourceState(context.Background(), stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale ChangeDataSourceState() error = %v", err)
	}
}

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metadata.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func seedBaseFixture(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO auth_subjects VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, []any{"subject-1", "external-1", "Synthetic User", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO data_sources VALUES (?, ?, ?, 'TEST', 'OBSERVER_DIRECT', 'MYSQL', ?, 2881, ?, ?, ?, 1, 'ENABLED', 1, NULL, NULL, NULL, ?, ?, ?)`, []any{"source-1", "Synthetic Source", "synthetic source", "127.0.0.1", "synthetic_user@synthetic_tenant", "synthetic_db", "credential-1", "subject-1", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO credential_revisions VALUES (?, 1, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)`, []any{"credential-1", "source-1", "key-1", []byte{1, 2, 3}, []byte{4, 5, 6}, utcText(testTime)}},
		{`INSERT INTO execution_nodes VALUES (?, ?, ?, 'WINDOWS_AMD64', 'ENABLED', '[]', NULL, 1, ?, ?, ?)`, []any{"node-1", "Synthetic Node", "synthetic node", "subject-1", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO agents VALUES (?, ?, ?, 1, 'ACTIVE', ?, ?, ?, 1, 0, '{}', ?, NULL)`, []any{"agent-1", "node-1", []byte{7, 8, 9}, "agent-v1", "boot-1", utcText(testTime), utcText(testTime)}},
		{`INSERT INTO export_drafts VALUES (?, ?, ?, ?, 1, ?, ?, ?, '{}', ?, '{}', ?, ?)`, []any{"draft-1", "subject-1", "source-1", "node-1", "4.3.5-RELEASE", "obdumper-4.3.5-slice-v2", "export-direct-single-table-csv-v1", testFingerprint, utcText(testTime), utcText(testTime)}},
		{`INSERT INTO precheck_runs VALUES (?, ?, 1, ?, ?, ?, 1, ?, ?, 'SUCCEEDED', NULL, NULL, NULL, '{}', 'COMPLETE', ?, ?, ?)`, []any{"precheck-1", "draft-1", testFingerprint, "source-1", "credential-1", "node-1", "agent-1", utcText(testTime.Add(10 * time.Minute)), utcText(testTime), utcText(testTime)}},
	}
	for index, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed statement %d: %v", index+1, err)
		}
	}
}

func validTaskSubmission(taskID string) TaskSubmission {
	return TaskSubmission{
		TaskID:                 taskID,
		CreatorSubjectID:       "subject-1",
		AuditActorID:           "subject-1",
		DataSourceID:           "source-1",
		NodeID:                 "node-1",
		PrecheckID:             "precheck-1",
		CredentialID:           "credential-1",
		CredentialRevision:     1,
		ConfigFingerprint:      testFingerprint,
		ToolVersion:            "4.3.5-RELEASE",
		MetadataVersion:        "obdumper-4.3.5-slice-v2",
		CapabilityVersion:      "export-direct-single-table-csv-v1",
		SnapshotJSON:           `{"credentialReference":{"credentialId":"credential-1","revision":1}}`,
		PlannedArgvJSON:        `["--host","127.0.0.1","--port","2881","--user","synthetic_user@synthetic_tenant","--database","synthetic_db","--table","synthetic_table","--csv","--file-path","E:\\tmp\\output"]`,
		PlannedCommandRedacted: `obdumper --host 127.0.0.1 --port 2881 --user ****** --database synthetic_db --table synthetic_table --csv --file-path E:\tmp\output`,
		RequestID:              "request-submit-1",
		SubmittedAt:            testTime.Add(2 * time.Minute),
	}
}

func validClaim(executionID, leaseID, eventID, requestID string) Claim {
	return Claim{
		ExecutionID: executionID,
		TaskID:      "task-1",
		NodeID:      "node-1",
		AgentID:     "agent-1",
		LeaseID:     leaseID,
		LeaseEpoch:  1,
		IssuedAt:    testTime.Add(3 * time.Minute),
		ExpiresAt:   testTime.Add(8 * time.Minute),
		EventID:     eventID,
		RequestID:   requestID,
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("count query %q = %d, want %d", query, got, want)
	}
}
