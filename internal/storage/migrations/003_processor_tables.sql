-- migrations/003_processor_tables.sql
-- Tabelas do limiar-processor: normalização, classificação, sintetização.
-- O collector nunca escreve nestas tabelas.

CREATE TABLE IF NOT EXISTS processed_messages (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    raw_message_id   INTEGER NOT NULL REFERENCES raw_messages(id),
    channel_id       INTEGER NOT NULL,
    message_id       INTEGER NOT NULL,
    message_type     TEXT NOT NULL DEFAULT 'commentary',
    text_clean       TEXT NOT NULL DEFAULT '',
    text_length      INTEGER NOT NULL DEFAULT 0,
    media_type       TEXT NOT NULL DEFAULT 'none',
    photo_id         INTEGER NOT NULL DEFAULT 0,
    views            INTEGER NOT NULL DEFAULT 0,
    forwards         INTEGER NOT NULL DEFAULT 0,
    reply_to_msg_id  INTEGER NOT NULL DEFAULT 0,
    has_url          INTEGER NOT NULL DEFAULT 0,
    has_price        INTEGER NOT NULL DEFAULT 0,
    has_coupon       INTEGER NOT NULL DEFAULT 0,
    price_amount     INTEGER,
    price_currency   TEXT DEFAULT 'BRL',
    urgency_signals  TEXT,
    posted_at        TEXT NOT NULL,
    processed_at     TEXT NOT NULL DEFAULT (datetime('now')),
    price_original   INTEGER DEFAULT 0,
    price_discount   INTEGER DEFAULT 0,
    coupon_code      TEXT DEFAULT '',
    payment_method   TEXT DEFAULT '',
    shipping         TEXT DEFAULT '',
    installments     TEXT DEFAULT '',
    discount_percent INTEGER DEFAULT 0,
    url_hash         TEXT DEFAULT '',
    merchant         TEXT DEFAULT '',
    product_name     TEXT DEFAULT '',
    synthesis        TEXT DEFAULT '',
    is_duplicate     INTEGER DEFAULT 0,
    feed_eligible    INTEGER DEFAULT 0,
    UNIQUE(channel_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_processed_messages_channel
    ON processed_messages(channel_id, message_id);

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
