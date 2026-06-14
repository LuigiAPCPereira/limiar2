-- Migration 007: inline thumbnail para imagens de mensagens.
-- O Telegram inclui um thumbnail inline (Type "i", ~230 bytes) no payload de
-- cada mensagem com foto. Este thumbnail é sempre disponível (não depende de
-- file_reference MTProto) e serve como fallback universal para o frontend.
-- Ver ADR 011 (substitui abordagem MTProxy sob demanda).

ALTER TABLE processed_messages ADD COLUMN inline_thumb BLOB;

CREATE INDEX IF NOT EXISTS idx_processed_inline_thumb
    ON processed_messages(photo_id) WHERE inline_thumb IS NOT NULL;
