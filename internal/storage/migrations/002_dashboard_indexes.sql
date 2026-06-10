-- ⚡ Raio: Otimiza listagem do dashboard filtrada por canal
CREATE INDEX IF NOT EXISTS idx_raw_messages_channel_received_at
    ON raw_messages(channel_id, received_at DESC);
