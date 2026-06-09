-- ⚡ Raio: Otimiza listagem do dashboard filtrada por canal e geral
CREATE INDEX IF NOT EXISTS idx_raw_messages_channel_received_at
    ON raw_messages(channel_id, received_at DESC);

CREATE INDEX IF NOT EXISTS idx_raw_messages_received_at
    ON raw_messages(received_at DESC);
