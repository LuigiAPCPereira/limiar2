-- ⚡ Raio: Otimiza listagem geral do dashboard por received_at DESC
CREATE INDEX IF NOT EXISTS idx_raw_messages_received_at
    ON raw_messages(received_at DESC);
