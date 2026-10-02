CREATE TABLE source_sync_state (
    subscription_id TEXT    NOT NULL CHECK(length(subscription_id) > 0),
    user_id         INTEGER NOT NULL,
    pts             INTEGER NOT NULL,
    qts             INTEGER NOT NULL,
    date            INTEGER NOT NULL,
    seq             INTEGER NOT NULL,
    PRIMARY KEY (subscription_id, user_id)
) STRICT;

CREATE TABLE source_sync_channel_state (
    subscription_id TEXT    NOT NULL CHECK(length(subscription_id) > 0),
    user_id         INTEGER NOT NULL,
    channel_id      INTEGER NOT NULL,
    pts             INTEGER NOT NULL,
    PRIMARY KEY (subscription_id, user_id, channel_id)
) STRICT;

PRAGMA user_version = 2;
