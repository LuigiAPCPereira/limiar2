-- migrations/009_cre_confidence.sql
-- Sprint 2: Candidate Ranking Engine — persiste confidence score de
-- product_name para que a Fase 3 (LLM) saiba quais mensagens precisam
-- de inferência (confidence abaixo do threshold).
--
-- O schema novo já nasce completo em 001_initial.sql. Bancos antigos recebem
-- colunas ausentes pelo reparo condicional em internal/storage/migrations.go,
-- pois Turso não aceita ALTER TABLE ADD COLUMN duplicado.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
