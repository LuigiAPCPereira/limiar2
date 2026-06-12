-- Índice para otimizar MAX(raw_message_id) no FetchUnprocessed.
-- Sem este índice, o Turso faz SCAN (full scan) na tabela processed_messages
-- a cada poll interval (default 5s). Com o índice, passa a SEARCH O(log n).
CREATE INDEX IF NOT EXISTS idx_processed_raw_message_id
    ON processed_messages(raw_message_id);

-- Remove índices redundantes que duplicam UNIQUE constraints.
-- A constraint UNIQUE já cria um índice implícito. O índice explícito
-- apenas dobra o custo de escrita sem benefício de leitura.
DROP INDEX IF EXISTS idx_raw_messages_channel;
DROP INDEX IF EXISTS idx_processed_messages_channel;
DROP INDEX IF EXISTS idx_channels_username;
