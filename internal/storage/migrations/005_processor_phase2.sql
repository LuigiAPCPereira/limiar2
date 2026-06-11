-- migrations/005_processor_phase2.sql
-- Fase 2 do limiar-processor: dual price, modificadores de promoção,
-- sintetização, cupons, dedup cross-channel.

-- Preço dual (De X por Y)
ALTER TABLE processed_messages ADD COLUMN price_original INTEGER DEFAULT 0;
ALTER TABLE processed_messages ADD COLUMN price_discount INTEGER DEFAULT 0;

-- Cupom extraído
ALTER TABLE processed_messages ADD COLUMN coupon_code TEXT DEFAULT '';

-- Modificadores de promoção
ALTER TABLE processed_messages ADD COLUMN payment_method TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN shipping TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN installments TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN discount_percent INTEGER DEFAULT 0;

-- Dedup cross-channel e classificação
ALTER TABLE processed_messages ADD COLUMN url_hash TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN merchant TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN product_name TEXT DEFAULT '';

-- Síntese JSON (SynthesizedPromotion serializada)
ALTER TABLE processed_messages ADD COLUMN synthesis TEXT DEFAULT '';

-- Índices para dedup e busca
CREATE INDEX IF NOT EXISTS idx_processed_url_hash
    ON processed_messages(url_hash) WHERE url_hash != '';

CREATE INDEX IF NOT EXISTS idx_processed_merchant
    ON processed_messages(merchant) WHERE merchant != '';
