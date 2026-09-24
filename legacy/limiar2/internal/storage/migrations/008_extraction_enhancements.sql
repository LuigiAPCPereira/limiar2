-- migrations/008_extraction_enhancements.sql
-- Sprint 1: campos estruturados de extração determinística.
--
-- O schema novo já nasce completo em 001_initial.sql. Bancos antigos recebem
-- colunas ausentes pelo reparo condicional em internal/storage/migrations.go,
-- pois Turso não aceita ALTER TABLE ADD COLUMN duplicado.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
