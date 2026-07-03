-- migrations/001_initial.sql
-- Schema consolidado para banco novo do Limiar.
-- Driver: tursogo ("turso") via database/sql. Placeholders usam ? apenas.

CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (id = 1)
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
    payload        TEXT NOT NULL,
    received_at    TEXT NOT NULL DEFAULT (datetime('now')),
    schema_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(channel_id, message_id)
);

CREATE TABLE IF NOT EXISTS processed_messages (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    raw_message_id     INTEGER NOT NULL REFERENCES raw_messages(id),
    channel_id         INTEGER NOT NULL,
    message_id         INTEGER NOT NULL,
    message_type       TEXT NOT NULL DEFAULT 'commentary',
    text_clean         TEXT NOT NULL DEFAULT '',
    text_length        INTEGER NOT NULL DEFAULT 0,
    media_type         TEXT NOT NULL DEFAULT 'none',
    photo_id           INTEGER NOT NULL DEFAULT 0,
    views              INTEGER NOT NULL DEFAULT 0,
    forwards           INTEGER NOT NULL DEFAULT 0,
    reply_to_msg_id    INTEGER NOT NULL DEFAULT 0,
    has_url            INTEGER NOT NULL DEFAULT 0,
    has_price          INTEGER NOT NULL DEFAULT 0,
    has_coupon         INTEGER NOT NULL DEFAULT 0,
    price_amount       INTEGER,
    price_currency     TEXT DEFAULT 'BRL',
    posted_at          TEXT NOT NULL,
    processed_at       TEXT NOT NULL DEFAULT (datetime('now')),
    price_original     INTEGER DEFAULT 0,
    price_discount     INTEGER DEFAULT 0,
    coupon_code        TEXT DEFAULT '',
    payment_method     TEXT DEFAULT '',
    shipping           TEXT DEFAULT '',
    installments       TEXT DEFAULT '',
    discount_percent   INTEGER DEFAULT 0,
    url_hash           TEXT DEFAULT '',
    merchant           TEXT DEFAULT '',
    product_name       TEXT DEFAULT '',
    synthesis          TEXT DEFAULT '',
    is_duplicate       INTEGER DEFAULT 0,
    feed_eligible      INTEGER DEFAULT 0,
    photo_access_hash  INTEGER DEFAULT 0,
    photo_file_ref     TEXT DEFAULT '',
    photo_dcid         INTEGER DEFAULT 0,
    inline_thumb       BLOB,
    shipping_free      INTEGER DEFAULT 0,
    installments_n     INTEGER DEFAULT 0,
    installments_value INTEGER DEFAULT 0,
    is_recurring       INTEGER DEFAULT 0,
    valid_from         TEXT,
    valid_until        TEXT,
    flash              INTEGER DEFAULT 0,
    recurrence_pattern TEXT DEFAULT '',
    recurrence_group_id INTEGER DEFAULT 0,
    seasonal_tag       TEXT DEFAULT '',
    UNIQUE(channel_id, message_id)
);

CREATE TABLE IF NOT EXISTS photo_cache (
    photo_id    INTEGER PRIMARY KEY,
    data        BLOB NOT NULL,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_raw_messages_channel_received_at
    ON raw_messages(channel_id, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_raw_messages_received_at
    ON raw_messages(received_at DESC);

CREATE INDEX IF NOT EXISTS idx_processed_messages_type
    ON processed_messages(message_type);
CREATE INDEX IF NOT EXISTS idx_processed_messages_posted
    ON processed_messages(posted_at DESC);
CREATE INDEX IF NOT EXISTS idx_processed_url_hash
    ON processed_messages(url_hash) WHERE url_hash != '';
CREATE INDEX IF NOT EXISTS idx_processed_merchant
    ON processed_messages(merchant) WHERE merchant != '';
CREATE INDEX IF NOT EXISTS idx_processed_feed
    ON processed_messages(feed_eligible, posted_at DESC) WHERE feed_eligible = 1;
CREATE INDEX IF NOT EXISTS idx_processed_raw_message_id
    ON processed_messages(raw_message_id);
CREATE INDEX IF NOT EXISTS idx_processed_shipping_free
    ON processed_messages(shipping_free) WHERE shipping_free = 1;
CREATE INDEX IF NOT EXISTS idx_processed_recurring
    ON processed_messages(is_recurring) WHERE is_recurring = 1;
CREATE INDEX IF NOT EXISTS idx_processed_has_photo
    ON processed_messages(photo_id) WHERE photo_id > 0;
CREATE INDEX IF NOT EXISTS idx_processed_flash
    ON processed_messages(flash) WHERE flash = 1;
CREATE INDEX IF NOT EXISTS idx_processed_valid_until
    ON processed_messages(valid_until) WHERE valid_until IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_processed_recurrence_group
    ON processed_messages(recurrence_group_id) WHERE recurrence_group_id != 0;
CREATE INDEX IF NOT EXISTS idx_photo_cache_expires
    ON photo_cache(expires_at) WHERE expires_at IS NOT NULL;
