-- migrations/006_media_metadata.sql
-- Metadados MTProto para resolução de imagens sob demanda (ADR 011).
-- O processor extrai esses campos do payload bruto; o MediaResolver
-- os usa para download via upload.GetFile no limiar-api.

ALTER TABLE processed_messages ADD COLUMN photo_access_hash INTEGER DEFAULT 0;
ALTER TABLE processed_messages ADD COLUMN photo_file_ref TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN photo_dcid INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_processed_media
    ON processed_messages(photo_id) WHERE photo_id > 0;
