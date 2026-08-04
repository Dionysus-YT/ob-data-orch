package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"ob-data-orch/migrations"

	_ "modernc.org/sqlite"
)

func TestApplyCreatesStrictSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)

	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply() first run: %v", err)
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply() idempotent run: %v", err)
	}

	var tableCount, strictCount int
	err := db.QueryRowContext(ctx, `
        SELECT COUNT(*), COALESCE(SUM(strict), 0)
        FROM pragma_table_list
        WHERE schema = 'main' AND type = 'table' AND name NOT LIKE 'sqlite_%'
    `).Scan(&tableCount, &strictCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 26 || strictCount != 26 {
		t.Fatalf("schema tables = %d, strict tables = %d; want 26 and 26", tableCount, strictCount)
	}

	var migrationCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 13 {
		t.Fatalf("migration count = %d, want 13", migrationCount)
	}
	for _, table := range []string{
		"data_source_connection_test_runs",
		"agent_data_source_connection_test_receipts",
		"agent_data_source_connection_test_secret_resolution_receipts",
	} {
		var strict int
		if err := db.QueryRowContext(ctx, `
            SELECT strict
            FROM pragma_table_list
            WHERE schema = 'main' AND name = ?
        `, table).Scan(&strict); err != nil || strict != 1 {
			t.Fatalf("connection-test table %s strict=%d err=%v, want 1 and nil", table, strict, err)
		}
	}

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign key check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key check returned a violation")
	}
}

func TestApplyUpgradesLegacyPrechecksToPersistentLeaseSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)
	legacy := fstest.MapFS{}
	for _, name := range []string{
		"0001_initial.sql",
		"0002_add_data_source_odc_identity.sql",
		"0003_disable_unverified_data_sources.sql",
		"0004_cleanup_unreferenced_archived_data_sources.sql",
		"0005_add_agent_facts_revision.sql",
		"0006_add_agent_heartbeat_idempotency.sql",
	} {
		contents, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatalf("read legacy migration %s: %v", name, err)
		}
		legacy[name] = &fstest.MapFile{Data: contents}
	}
	if err := ApplyFS(ctx, db, legacy); err != nil {
		t.Fatalf("ApplyFS() legacy schema: %v", err)
	}
	insertLegacyPrecheckFixture(t, db)
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply() persistent precheck migration: %v", err)
	}
	var factsRevision int64
	var bindingDigest, bindingAgentID sql.NullString
	if err := db.QueryRowContext(ctx, `
        SELECT node_facts_revision, binding_digest, binding_agent_id
        FROM precheck_runs WHERE precheck_id = 'precheck-legacy'
    `).Scan(&factsRevision, &bindingDigest, &bindingAgentID); err != nil {
		t.Fatalf("read upgraded precheck: %v", err)
	}
	if factsRevision != 0 || bindingDigest.Valid || bindingAgentID.Valid {
		t.Fatalf("upgraded historical precheck = revision=%d digest=%q agent=%q; want 0, NULL, NULL", factsRevision, bindingDigest.String, bindingAgentID.String)
	}
	var strict int
	if err := db.QueryRowContext(ctx, `SELECT strict FROM pragma_table_list WHERE schema = 'main' AND name = 'agent_precheck_receipts'`).Scan(&strict); err != nil || strict != 1 {
		t.Fatalf("agent_precheck_receipts strict=%d err=%v", strict, err)
	}
	digest := strings.Repeat("a", 64)
	if _, err := db.ExecContext(ctx, `
        INSERT INTO agent_precheck_receipts(
            agent_id, request_id, operation, request_digest, precheck_id, lease_id,
            lease_epoch, binding_digest, status, expires_at, completed_at, created_at
        ) VALUES ('agent-legacy', 'receipt-legacy-1', 'CLAIM', ?, 'precheck-legacy', 'lease-legacy',
                  1, ?, 'LEASED', '2026-01-01T00:01:00Z', NULL, '2026-01-01T00:00:00Z')
    `, digest, digest); err != nil {
		t.Fatalf("insert persistent precheck receipt: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
        INSERT INTO agent_precheck_receipts(
            agent_id, request_id, operation, request_digest, precheck_id, lease_id,
            lease_epoch, binding_digest, status, expires_at, completed_at, created_at
        ) VALUES ('agent-legacy', 'receipt-legacy-1', 'ACKNOWLEDGE', ?, 'precheck-legacy', 'lease-legacy',
                  1, ?, 'ACKNOWLEDGED', '2026-01-01T00:01:00Z', NULL, '2026-01-01T00:00:00Z')
    `, digest, digest); err == nil {
		t.Fatal("agent_precheck_receipts accepted a request ID reused across operations")
	}
}

func TestApplyDisablesUnverifiedLegacyDataSources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)
	initial, err := migrations.Files.ReadFile("0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	odcIdentity, err := migrations.Files.ReadFile("0002_add_data_source_odc_identity.sql")
	if err != nil {
		t.Fatalf("read ODC identity migration: %v", err)
	}
	legacy := fstest.MapFS{
		"0001_initial.sql":                      &fstest.MapFile{Data: initial},
		"0002_add_data_source_odc_identity.sql": &fstest.MapFile{Data: odcIdentity},
	}
	if err := ApplyFS(ctx, db, legacy); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO auth_subjects VALUES ('subject-1', 'external-1', 'Synthetic User', 'ACTIVE', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert migration subject: %v", err)
	}
	for _, fixture := range []struct {
		id, status string
	}{
		{id: "source-unverified", status: ""},
		{id: "source-verified", status: "SUCCEEDED"},
	} {
		if _, err := db.ExecContext(ctx, `
            INSERT INTO data_sources(
                data_source_id, display_name, normalized_name, environment, connection_kind,
                compatibility_mode, host, port, username, default_database, credential_id,
                current_credential_revision, state, revision, last_test_status, last_tested_at,
                last_test_safe_summary_json, created_by, created_at, updated_at, cluster_name, tenant_name
            ) VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', '127.0.0.1', 2881, 'synthetic_user', 'synthetic_db', ?, 1,
                      'ENABLED', 1, NULLIF(?, ''), NULL, NULL, 'subject-1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z',
                      'synthetic-cluster', 'synthetic-tenant')
        `, fixture.id, fixture.id, fixture.id, "credential-"+fixture.id, fixture.status); err != nil {
			t.Fatalf("insert legacy data source %s: %v", fixture.id, err)
		}
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("apply connection-test migration: %v", err)
	}
	states := make(map[string]string)
	rows, err := db.QueryContext(ctx, `SELECT data_source_id, state FROM data_sources`)
	if err != nil {
		t.Fatalf("read migrated states: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			t.Fatalf("scan migrated state: %v", err)
		}
		states[id] = state
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated states: %v", err)
	}
	if states["source-unverified"] != "DISABLED" || states["source-verified"] != "ENABLED" {
		t.Fatalf("migrated states = %#v", states)
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_DISABLED_UNVERIFIED'`).Scan(&auditCount); err != nil {
		t.Fatalf("count migration audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("migration audit events = %d, want 1", auditCount)
	}
}

func TestApplyPhysicallyDeletesUnreferencedArchivedDataSources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)
	initial, err := migrations.Files.ReadFile("0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	odcIdentity, err := migrations.Files.ReadFile("0002_add_data_source_odc_identity.sql")
	if err != nil {
		t.Fatalf("read ODC identity migration: %v", err)
	}
	disableUnverified, err := migrations.Files.ReadFile("0003_disable_unverified_data_sources.sql")
	if err != nil {
		t.Fatalf("read connection-test migration: %v", err)
	}
	legacy := fstest.MapFS{
		"0001_initial.sql":                         &fstest.MapFile{Data: initial},
		"0002_add_data_source_odc_identity.sql":    &fstest.MapFile{Data: odcIdentity},
		"0003_disable_unverified_data_sources.sql": &fstest.MapFile{Data: disableUnverified},
	}
	if err := ApplyFS(ctx, db, legacy); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO auth_subjects VALUES ('subject-1', 'external-1', 'Synthetic User', 'ACTIVE', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert migration subject: %v", err)
	}
	for _, fixture := range []struct {
		id, credentialID string
	}{
		{id: "source-archived-unused", credentialID: "credential-archived-unused"},
		{id: "source-archived-referenced", credentialID: "credential-archived-referenced"},
	} {
		if _, err := db.ExecContext(ctx, `
            INSERT INTO data_sources(
                data_source_id, display_name, normalized_name, environment, connection_kind,
                compatibility_mode, host, port, username, default_database, credential_id,
                current_credential_revision, state, revision, last_test_status, last_tested_at,
                last_test_safe_summary_json, created_by, created_at, updated_at, cluster_name, tenant_name
            ) VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', '127.0.0.1', 2881, 'synthetic_user', 'synthetic_db', ?, 1,
                      'ARCHIVED', 1, NULL, NULL, NULL, 'subject-1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z',
                      'synthetic-cluster', 'synthetic-tenant')
        `, fixture.id, fixture.id, fixture.id, fixture.credentialID); err != nil {
			t.Fatalf("insert archived data source %s: %v", fixture.id, err)
		}
		if _, err := db.ExecContext(ctx, `
            INSERT INTO credential_revisions(
                credential_id, revision, data_source_id, secret_type, key_id, nonce,
                ciphertext, aad_json, status, created_at, retired_at
            ) VALUES (?, 1, ?, 'DATABASE_PASSWORD', 'key-1', X'010203', X'040506', '{}', 'ACTIVE', '2026-01-01T00:00:00Z', NULL)
        `, fixture.credentialID, fixture.id); err != nil {
			t.Fatalf("insert archived credential %s: %v", fixture.id, err)
		}
		if _, err := db.ExecContext(ctx, `
            INSERT INTO request_idempotency(
                subject_id, operation, idempotency_key, request_digest, result_status,
                resource_kind, resource_id, response_json, created_at, expires_at
            ) VALUES ('subject-1', 'CREATE_DATA_SOURCE', ?, ?, 201, 'DATA_SOURCE', ?, '{}', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')
        `, "idempotency-"+fixture.id, strings.Repeat("a", 64), fixture.id); err != nil {
			t.Fatalf("insert archived idempotency %s: %v", fixture.id, err)
		}
	}
	if _, err := db.ExecContext(ctx, `
        INSERT INTO export_drafts(
            draft_id, owner_subject_id, data_source_id, node_id, revision,
            tool_version, metadata_version, capability_version, config_json,
            config_fingerprint, invalidation_json, created_at, updated_at
        ) VALUES ('draft-archived-reference', 'subject-1', 'source-archived-referenced', NULL, 1,
                  '4.3.5-RELEASE', 'metadata-v1', 'capability-v1', '{}', NULL, '{}',
                  '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
    `); err != nil {
		t.Fatalf("insert archived source reference: %v", err)
	}

	if err := Apply(ctx, db); err != nil {
		t.Fatalf("apply archived cleanup migration: %v", err)
	}
	for _, query := range []string{
		"SELECT COUNT(*) FROM data_sources WHERE data_source_id = 'source-archived-unused'",
		"SELECT COUNT(*) FROM credential_revisions WHERE data_source_id = 'source-archived-unused'",
		"SELECT COUNT(*) FROM request_idempotency WHERE resource_id = 'source-archived-unused'",
	} {
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cleanup query %q = %d, %v; want 0", query, count, err)
		}
	}
	var state string
	if err := db.QueryRowContext(ctx, `SELECT state FROM data_sources WHERE data_source_id = 'source-archived-referenced'`).Scan(&state); err != nil || state != "ARCHIVED" {
		t.Fatalf("referenced archived source state = %q, %v", state, err)
	}
	var remainingCredential, remainingIdempotency, deletionAudit int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM credential_revisions WHERE data_source_id = 'source-archived-referenced'`).Scan(&remainingCredential); err != nil {
		t.Fatalf("count referenced credential: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_idempotency WHERE resource_id = 'source-archived-referenced'`).Scan(&remainingIdempotency); err != nil {
		t.Fatalf("count referenced idempotency: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE action = 'DATA_SOURCE_DELETED' AND actor_id = 'migration-0004'`).Scan(&deletionAudit); err != nil {
		t.Fatalf("count cleanup audit: %v", err)
	}
	if remainingCredential != 1 || remainingIdempotency != 1 || deletionAudit != 1 {
		t.Fatalf("referenced preservation credential=%d idempotency=%d cleanupAudit=%d", remainingCredential, remainingIdempotency, deletionAudit)
	}
}

func TestTasksAreImmutable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	insertSyntheticTaskFixture(t, db)

	if _, err := db.ExecContext(ctx, `UPDATE tasks SET planned_command_redacted = 'changed' WHERE task_id = 'task-1'`); err == nil {
		t.Fatal("updating an immutable task unexpectedly succeeded")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM tasks WHERE task_id = 'task-1'`); err == nil {
		t.Fatal("deleting an immutable task unexpectedly succeeded")
	}
}

func TestApplyRejectsChangedChecksum(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDatabase(t)
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	secondMigration, err := migrations.Files.ReadFile("0002_add_data_source_odc_identity.sql")
	if err != nil {
		t.Fatalf("read second migration: %v", err)
	}
	thirdMigration, err := migrations.Files.ReadFile("0003_disable_unverified_data_sources.sql")
	if err != nil {
		t.Fatalf("read third migration: %v", err)
	}
	fourthMigration, err := migrations.Files.ReadFile("0004_cleanup_unreferenced_archived_data_sources.sql")
	if err != nil {
		t.Fatalf("read fourth migration: %v", err)
	}
	fifthMigration, err := migrations.Files.ReadFile("0005_add_agent_facts_revision.sql")
	if err != nil {
		t.Fatalf("read fifth migration: %v", err)
	}
	sixthMigration, err := migrations.Files.ReadFile("0006_add_agent_heartbeat_idempotency.sql")
	if err != nil {
		t.Fatalf("read sixth migration: %v", err)
	}
	seventhMigration, err := migrations.Files.ReadFile("0007_persist_precheck_leases.sql")
	if err != nil {
		t.Fatalf("read seventh migration: %v", err)
	}
	eighthMigration, err := migrations.Files.ReadFile("0008_add_precheck_secret_resolution_receipts.sql")
	if err != nil {
		t.Fatalf("read eighth migration: %v", err)
	}
	ninthMigration, err := migrations.Files.ReadFile("0009_add_data_source_connection_test_runs.sql")
	if err != nil {
		t.Fatalf("read ninth migration: %v", err)
	}
	tenthMigration, err := migrations.Files.ReadFile("0010_add_execution_node_environment_checks.sql")
	if err != nil {
		t.Fatalf("read tenth migration: %v", err)
	}
	eleventhMigration, err := migrations.Files.ReadFile("0011_add_execution_node_runtime_configuration.sql")
	if err != nil {
		t.Fatalf("read eleventh migration: %v", err)
	}
	twelfthMigration, err := migrations.Files.ReadFile("0012_add_execution_secret_resolution_receipts.sql")
	if err != nil {
		t.Fatalf("read twelfth migration: %v", err)
	}
	thirteenthMigration, err := migrations.Files.ReadFile("0013_add_log_batch_projection_fields.sql")
	if err != nil {
		t.Fatalf("read thirteenth migration: %v", err)
	}
	tampered := fstest.MapFS{
		"0001_initial.sql":                                    &fstest.MapFile{Data: []byte("CREATE TABLE tampered(value TEXT) STRICT;")},
		"0002_add_data_source_odc_identity.sql":               &fstest.MapFile{Data: secondMigration},
		"0003_disable_unverified_data_sources.sql":            &fstest.MapFile{Data: thirdMigration},
		"0004_cleanup_unreferenced_archived_data_sources.sql": &fstest.MapFile{Data: fourthMigration},
		"0005_add_agent_facts_revision.sql":                   &fstest.MapFile{Data: fifthMigration},
		"0006_add_agent_heartbeat_idempotency.sql":            &fstest.MapFile{Data: sixthMigration},
		"0007_persist_precheck_leases.sql":                    &fstest.MapFile{Data: seventhMigration},
		"0008_add_precheck_secret_resolution_receipts.sql":    &fstest.MapFile{Data: eighthMigration},
		"0009_add_data_source_connection_test_runs.sql":       &fstest.MapFile{Data: ninthMigration},
		"0010_add_execution_node_environment_checks.sql":      &fstest.MapFile{Data: tenthMigration},
		"0011_add_execution_node_runtime_configuration.sql":   &fstest.MapFile{Data: eleventhMigration},
		"0012_add_execution_secret_resolution_receipts.sql":   &fstest.MapFile{Data: twelfthMigration},
		"0013_add_log_batch_projection_fields.sql":            &fstest.MapFile{Data: thirteenthMigration},
	}
	if err := ApplyFS(ctx, db, tampered); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Apply() error = %v, want checksum mismatch", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = checksum WHERE version = 1`); err == nil {
		t.Fatal("updating immutable migration history unexpectedly succeeded")
	}
}

func TestApplyRejectsDisabledForeignKeys(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Apply(context.Background(), db); err == nil || !strings.Contains(err.Error(), "foreign_keys") {
		t.Fatalf("Apply() error = %v, want foreign_keys failure", err)
	}
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "metadata.db"))
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertSyntheticTaskFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	fingerprint := strings.Repeat("a", 64)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO auth_subjects VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, []any{"subject-1", "external-1", "Synthetic User", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO data_sources VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', ?, 2881, ?, ?, ?, 1, 'ENABLED', 1, NULL, NULL, NULL, ?, ?, ?, 'synthetic-cluster', 'synthetic-tenant', NULL)`, []any{"source-1", "Synthetic Source", "synthetic source", "127.0.0.1", "synthetic_user", "synthetic_db", "credential-1", "subject-1", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO credential_revisions VALUES (?, 1, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)`, []any{"credential-1", "source-1", "key-1", []byte{1, 2, 3}, []byte{4, 5, 6}, "2026-01-01T00:00:00Z"}},
		{`INSERT INTO execution_nodes(node_id, display_name, normalized_name, platform, management_state, allowed_roots_json, tool_config_ref, revision, created_by, created_at, updated_at) VALUES (?, ?, ?, 'WINDOWS_AMD64', 'ENABLED', '[]', NULL, 1, ?, ?, ?)`, []any{"node-1", "Synthetic Node", "synthetic node", "subject-1", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO export_drafts VALUES (?, ?, ?, ?, 1, ?, ?, ?, '{}', ?, '[]', ?, ?)`, []any{"draft-1", "subject-1", "source-1", "node-1", "4.3.5-RELEASE", "obdumper-4.3.5-slice-v3", "export-odp-single-table-csv-v1", fingerprint, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO precheck_runs(
            precheck_id, draft_id, draft_revision, config_fingerprint, data_source_id,
            credential_id, credential_revision, node_id, agent_id, status, lease_id,
            lease_epoch, lease_expires_at, result_json, integrity_status, valid_until,
            created_at, completed_at, node_facts_revision, binding_digest, binding_agent_id
        ) VALUES (?, ?, 1, ?, ?, ?, 1, ?, NULL, 'SUCCEEDED', NULL, NULL, NULL, '{}', 'COMPLETE', ?, ?, ?, 0, NULL, NULL)`, []any{"precheck-1", "draft-1", fingerprint, "source-1", "credential-1", "node-1", "2026-01-01T01:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:01:00Z"}},
		{`INSERT INTO tasks VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, '{}', '[]', ?, ?)`, []any{"task-1", "subject-1", "source-1", "node-1", "precheck-1", "credential-1", fingerprint, "4.3.5-RELEASE", "obdumper-4.3.5-slice-v3", "export-odp-single-table-csv-v1", "obdumper --user ******", "2026-01-01T00:02:00Z"}},
	}
	for index, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("insert synthetic fixture statement %d: %v", index+1, err)
		}
	}
}

func insertLegacyPrecheckFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	fingerprint := strings.Repeat("a", 64)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO auth_subjects VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, []any{"subject-legacy", "external-legacy", "Synthetic User", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO data_sources VALUES (?, ?, ?, 'TEST', 'ODP', 'MYSQL', ?, 2881, ?, ?, ?, 1, 'ENABLED', 1, 'SUCCEEDED', ?, '{}', ?, ?, ?, 'synthetic-cluster', 'synthetic-tenant')`, []any{"source-legacy", "Synthetic Source", "synthetic source legacy", "127.0.0.1", "synthetic_user", "synthetic_db", "credential-legacy", "2026-01-01T00:00:00Z", "subject-legacy", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO credential_revisions VALUES (?, 1, ?, 'DATABASE_PASSWORD', ?, ?, ?, '{}', 'ACTIVE', ?, NULL)`, []any{"credential-legacy", "source-legacy", "key-legacy", []byte{1, 2, 3}, []byte{4, 5, 6}, "2026-01-01T00:00:00Z"}},
		{`INSERT INTO execution_nodes(node_id, display_name, normalized_name, platform, management_state, allowed_roots_json, tool_config_ref, revision, created_by, created_at, updated_at) VALUES (?, ?, ?, 'WINDOWS_AMD64', 'ENABLED', '[]', NULL, 1, ?, ?, ?)`, []any{"node-legacy", "Synthetic Node", "synthetic node legacy", "subject-legacy", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO agents(
            agent_id, node_id, credential_digest, credential_revision, status, protocol_version,
            boot_id, last_heartbeat_at, capacity_total, capacity_used, facts_json, facts_revision,
            created_at, revoked_at, last_heartbeat_request_id, last_heartbeat_request_digest
        ) VALUES (?, ?, X'010203', 1, 'ACTIVE', 'agent-v1', 'boot-legacy', '2026-01-01T00:00:00Z',
                  1, 0, NULL, 0, '2026-01-01T00:00:00Z', NULL, NULL, NULL)`, []any{"agent-legacy", "node-legacy"}},
		{`INSERT INTO export_drafts VALUES (?, ?, ?, ?, 1, ?, ?, ?, '{}', ?, '{}', ?, ?)`, []any{"draft-legacy", "subject-legacy", "source-legacy", "node-legacy", "4.3.5-RELEASE", "metadata-legacy", "capability-legacy", fingerprint, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO precheck_runs(
            precheck_id, draft_id, draft_revision, config_fingerprint, data_source_id,
            credential_id, credential_revision, node_id, agent_id, status, lease_id,
            lease_epoch, lease_expires_at, result_json, integrity_status, valid_until,
            created_at, completed_at
        ) VALUES (?, ?, 1, ?, ?, ?, 1, ?, NULL, 'SUCCEEDED', NULL, NULL, NULL, '{}', 'COMPLETE', ?, ?, ?)`, []any{"precheck-legacy", "draft-legacy", fingerprint, "source-legacy", "credential-legacy", "node-legacy", "2026-01-01T01:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:01:00Z"}},
	}
	for index, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("insert legacy precheck fixture statement %d: %v", index+1, err)
		}
	}
}
