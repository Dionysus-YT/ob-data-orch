INSERT INTO audit_events(
    audit_event_id, actor_type, actor_id, action, object_type, object_id,
    result, request_id, safe_diff_json, occurred_at
)
SELECT
    'migration-0003-disable-' || data_source_id,
    'SYSTEM',
    'migration-0003',
    'DATA_SOURCE_DISABLED_UNVERIFIED',
    'DATA_SOURCE',
    data_source_id,
    'SUCCEEDED',
    'migration-0003',
    '{"reason":"connection_test_required"}',
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
FROM data_sources
WHERE state = 'ENABLED' AND COALESCE(last_test_status, '') <> 'SUCCEEDED';

UPDATE data_sources
SET state = 'DISABLED',
    revision = revision + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE state = 'ENABLED' AND COALESCE(last_test_status, '') <> 'SUCCEEDED';
