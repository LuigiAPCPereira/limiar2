-- migrations/007_legacy_schema_compat.sql
-- Marca a compatibilidade do schema consolidado com bancos legados.
-- O upgrade condicional de colunas é executado por internal/storage/migrations.go,
-- porque bancos consolidados novos já possuem essas colunas e ALTER TABLE ADD
-- COLUMN precisa ser condicionado por PRAGMA table_info.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
