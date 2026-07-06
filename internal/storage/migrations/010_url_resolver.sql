-- 010_url_resolver.sql
-- Sprint 3: URL Resolver — cache persistente de resoluções e colunas
-- de URL canônica/title/resolved em processed_messages.
--
-- Colunas novas de processed_messages ficam no schema consolidado (001) e no
-- reparo condicional de migrations.go para bancos legados. Não use ALTER TABLE
-- aqui para evitar erro de coluna duplicada em bancos novos.

CREATE TABLE IF NOT EXISTS url_resolutions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    original_url  TEXT NOT NULL UNIQUE,
    canonical_url TEXT NOT NULL DEFAULT '',
    merchant      TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL DEFAULT '',
    unresolved    INTEGER NOT NULL DEFAULT 0,
    resolved_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_url_resolutions_canonical
    ON url_resolutions(canonical_url) WHERE canonical_url != '';
