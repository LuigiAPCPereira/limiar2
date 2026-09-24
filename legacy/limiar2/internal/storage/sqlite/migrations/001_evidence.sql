CREATE TABLE evidence (
    id                 BLOB    NOT NULL PRIMARY KEY CHECK(length(id) = 16),
    subscription_id    TEXT    NOT NULL CHECK(length(subscription_id) > 0),
    acquisition        TEXT    NOT NULL CHECK(length(acquisition) > 0),
    event_kind         TEXT    NOT NULL CHECK(length(event_kind) > 0),
    source_event_type  TEXT    NOT NULL CHECK(length(source_event_type) > 0),
    source_occurred_at INTEGER,
    received_at        INTEGER NOT NULL,
    payload_format     TEXT    NOT NULL CHECK(length(payload_format) > 0),
    payload_schema     TEXT    NOT NULL CHECK(length(payload_schema) > 0),
    payload            BLOB    NOT NULL,
    payload_sha256     BLOB    NOT NULL CHECK(length(payload_sha256) = 32)
) STRICT;

CREATE TRIGGER evidence_no_update
BEFORE UPDATE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;

CREATE TRIGGER evidence_no_delete
BEFORE DELETE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;

PRAGMA user_version = 1;
