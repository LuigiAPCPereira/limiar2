-- migrations/008_processor_new_fields.sql
ALTER TABLE processed_messages ADD COLUMN shipping_free INTEGER NOT NULL DEFAULT 0;
ALTER TABLE processed_messages ADD COLUMN installments_n INTEGER;
ALTER TABLE processed_messages ADD COLUMN installments_value REAL;
