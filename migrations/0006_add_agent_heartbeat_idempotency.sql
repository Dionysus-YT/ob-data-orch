ALTER TABLE agents ADD COLUMN last_heartbeat_request_id TEXT;
ALTER TABLE agents ADD COLUMN last_heartbeat_request_digest TEXT CHECK (
    last_heartbeat_request_digest IS NULL
    OR length(last_heartbeat_request_digest) = 64
);
