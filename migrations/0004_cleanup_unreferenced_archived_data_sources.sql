INSERT INTO audit_events(
    audit_event_id, actor_type, actor_id, action, object_type, object_id,
    result, request_id, safe_diff_json, occurred_at
)
SELECT
    'migration-0004-delete-' || data_source_id,
    'SYSTEM',
    'migration-0004',
    'DATA_SOURCE_DELETED',
    'DATA_SOURCE',
    data_source_id,
    'SUCCEEDED',
    'migration-0004',
    '{"reason":"unreferenced_archived_cleanup"}',
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
FROM data_sources
WHERE state = 'ARCHIVED'
  AND NOT EXISTS (SELECT 1 FROM export_drafts WHERE export_drafts.data_source_id = data_sources.data_source_id)
  AND NOT EXISTS (SELECT 1 FROM precheck_runs WHERE precheck_runs.data_source_id = data_sources.data_source_id)
  AND NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.data_source_id = data_sources.data_source_id);

DELETE FROM credential_revisions
WHERE data_source_id IN (
    SELECT data_source_id
    FROM data_sources
    WHERE state = 'ARCHIVED'
      AND NOT EXISTS (SELECT 1 FROM export_drafts WHERE export_drafts.data_source_id = data_sources.data_source_id)
      AND NOT EXISTS (SELECT 1 FROM precheck_runs WHERE precheck_runs.data_source_id = data_sources.data_source_id)
      AND NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.data_source_id = data_sources.data_source_id)
);

DELETE FROM request_idempotency
WHERE resource_kind = 'DATA_SOURCE'
  AND resource_id IN (
      SELECT data_source_id
      FROM data_sources
      WHERE state = 'ARCHIVED'
        AND NOT EXISTS (SELECT 1 FROM export_drafts WHERE export_drafts.data_source_id = data_sources.data_source_id)
        AND NOT EXISTS (SELECT 1 FROM precheck_runs WHERE precheck_runs.data_source_id = data_sources.data_source_id)
        AND NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.data_source_id = data_sources.data_source_id)
  );

DELETE FROM data_sources
WHERE state = 'ARCHIVED'
  AND NOT EXISTS (SELECT 1 FROM export_drafts WHERE export_drafts.data_source_id = data_sources.data_source_id)
  AND NOT EXISTS (SELECT 1 FROM precheck_runs WHERE precheck_runs.data_source_id = data_sources.data_source_id)
  AND NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.data_source_id = data_sources.data_source_id);
