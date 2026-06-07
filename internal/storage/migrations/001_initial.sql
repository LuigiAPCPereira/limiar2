-- migrations/001_initial.sql
-- Initial schema for limiar-collector (Phase 1).
-- Driver: tursogo ("turso") via database/sql. Placeholders use ? only.

CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (id = 1)  -- single-row constraint: only one session ever
);

CREATE TABLE IF NOT EXISTS peers (
    id          INTEGER PRIMARY KEY,
    access_hash INTEGER NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('channel', 'user', 'chat')),
    username    TEXT,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS channels (
    id                INTEGER PRIMARY KEY,
    username          TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL DEFAULT '',
    active            INTEGER NOT NULL DEFAULT 1,
    added_at          TEXT NOT NULL DEFAULT (datetime('now')),
    last_message_id   INTEGER NOT NULL DEFAULT 0,
    last_collected_at TEXT
);

CREATE TABLE IF NOT EXISTS raw_messages (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id     INTEGER NOT NULL REFERENCES channels(id),
    message_id     INTEGER NOT NULL,
    payload        TEXT NOT NULL,  -- raw gotd/td update serialized as JSON
    received_at    TEXT NOT NULL DEFAULT (datetime('now')),
    schema_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(channel_id, message_id)  -- safe-persistence dedup per channel
);

CREATE INDEX IF NOT EXISTS idx_raw_messages_channel
    ON raw_messages(channel_id, message_id);

CREATE INDEX IF NOT EXISTS idx_channels_username
    ON channels(username);
