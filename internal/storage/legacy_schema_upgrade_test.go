package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/limiar/collector/internal/storage"
)

// legacySchemaDDL recria o shape do banco Limiar observado em campo ANTES das
// colunas valid_from/valid_until/flash/recurrence_pattern/recurrence_group_id/
// seasonal_tag em processed_messages e ANTES de expires_at em photo_cache.
// processed_messages termina em is_recurring; photo_cache tem apenas
// photo_id/data/updated_at. É o estado deixado pelas migrações antigas
// 001_initial.sql ... 006_photo_cache.sql.
const legacySchemaDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (id = 1)
);

CREATE TABLE IF NOT EXISTS peers (
    id          INTEGER PRIMARY KEY,
    access_hash INTEGER NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('channel', 'user', 'chat')),
    username    TEXT,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS channels (
    id                INTEGER PRIMARY KEY,
    username          TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL DEFAULT '',
    active            INTEGER NOT NULL DEFAULT 1,
    added_at          TEXT NOT NULL DEFAULT (datetime('now')),
    last_message_id   INTEGER NOT NULL DEFAULT 0,
    last_collected_at TEXT
);

CREATE TABLE IF NOT EXISTS raw_messages (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id     INTEGER NOT NULL REFERENCES channels(id),
    message_id     INTEGER NOT NULL,
    payload        TEXT NOT NULL,
    received_at    TEXT NOT NULL DEFAULT (datetime('now')),
    schema_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(channel_id, message_id)
);

CREATE TABLE IF NOT EXISTS processed_messages (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    raw_message_id     INTEGER NOT NULL REFERENCES raw_messages(id),
    channel_id         INTEGER NOT NULL,
    message_id         INTEGER NOT NULL,
    message_type       TEXT NOT NULL DEFAULT 'commentary',
    text_clean         TEXT NOT NULL DEFAULT '',
    text_length        INTEGER NOT NULL DEFAULT 0,
    media_type         TEXT NOT NULL DEFAULT 'none',
    photo_id           INTEGER NOT NULL DEFAULT 0,
    views              INTEGER NOT NULL DEFAULT 0,
    forwards           INTEGER NOT NULL DEFAULT 0,
    reply_to_msg_id    INTEGER NOT NULL DEFAULT 0,
    has_url            INTEGER NOT NULL DEFAULT 0,
    has_price          INTEGER NOT NULL DEFAULT 0,
    has_coupon         INTEGER NOT NULL DEFAULT 0,
    price_amount       INTEGER,
    price_currency     TEXT DEFAULT 'BRL',
    posted_at          TEXT NOT NULL,
    processed_at       TEXT NOT NULL DEFAULT (datetime('now')),
    price_original     INTEGER DEFAULT 0,
    price_discount     INTEGER DEFAULT 0,
    coupon_code        TEXT DEFAULT '',
    payment_method     TEXT DEFAULT '',
    shipping           TEXT DEFAULT '',
    installments       TEXT DEFAULT '',
    discount_percent   INTEGER DEFAULT 0,
    url_hash           TEXT DEFAULT '',
    merchant           TEXT DEFAULT '',
    product_name       TEXT DEFAULT '',
    synthesis          TEXT DEFAULT '',
    is_duplicate       INTEGER DEFAULT 0,
    feed_eligible      INTEGER DEFAULT 0,
    photo_access_hash  INTEGER DEFAULT 0,
    photo_file_ref     TEXT DEFAULT '',
    photo_dcid         INTEGER DEFAULT 0,
    inline_thumb       BLOB,
    shipping_free      INTEGER DEFAULT 0,
    installments_n     INTEGER DEFAULT 0,
    installments_value INTEGER DEFAULT 0,
    is_recurring       INTEGER DEFAULT 0,
    UNIQUE(channel_id, message_id)
);

CREATE TABLE IF NOT EXISTS photo_cache (
    photo_id   INTEGER PRIMARY KEY,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`

// legacyMigrationVersions são os nomes de arquivo das migrações antigas que um
// banco de campo legado teria registrado em schema_migrations. O último passo
// do histórico antigo é 006_photo_cache.sql — anterior à introdução das colunas
// atuais de validade/flash/recorrência/sazonalidade e de expires_at.
var legacyMigrationVersions = []string{
	"001_initial.sql",
	"002_dashboard_indexes.sql",
	"003_processor_tables.sql",
	"004_dashboard_index.sql",
	"004_schema_update.sql",
	"005_index_optimization.sql",
	"006_photo_cache.sql",
}

// createLegacyTursogoDB materializa em dbPath um banco Tursogo legado: cria o
// arquivo, abre via driver "turso", aplica o schema antigo e registra todas as
// versões antigas em schema_migrations — exatamente o estado que faz
// storage.Open acreditar que está em dia e pular migrações.
func createLegacyTursogoDB(t *testing.T, dbPath string) {
	t.Helper()
	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create legacy db file %q: %v", dbPath, err)
	}
	_ = f.Close()

	db, err := sql.Open("turso", dbPath)
	if err != nil {
		t.Fatalf("sql.Open legacy db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(legacySchemaDDL); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}

	for _, v := range legacyMigrationVersions {
		if _, err := db.Exec(
			`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			t.Fatalf("record legacy migration %s: %v", v, err)
		}
	}
}

// columnPresent checa via PRAGMA table_info se a coluna existe na tabela.
// Retorna false (não error) para ausência, permitindo que o teste relate
// precisamente qual coluna faltou.
func columnPresent(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(%s): %v", table, err)
	}
	return false
}

// TestOpenUpgradesLegacySchemaForProcessorRepository reproduz o bug do banco
// legado do Limiar: schema_migrations marca as migrações antigas (até
// 006_photo_cache.sql) como aplicadas, mas processed_messages não possui as
// colunas atuais (valid_from, valid_until, flash, recurrence_pattern,
// recurrence_group_id, seasonal_tag) e photo_cache não possui expires_at.
//
// Ao reabrir esse banco com storage.Open, o migrate() vê as versões antigas
// registradas e as pula — nenhuma migração compatível adiciona as colunas
// ausentes — de modo que NewProcessorRepository falha ao preparar o INSERT que
// referencia essas colunas, impedindo "limiar media backfill" de popular
// photo_cache. A expectativa deste teste (contrato pós-correção) é que Open
// leve o schema legado ao shape atual e que NewProcessorRepository tenha sucesso.
func TestOpenUpgradesLegacySchemaForProcessorRepository(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyTursogoDB(t, dbPath)

	db, err := storage.Open(ctx, dbPath, nil)
	if err != nil {
		t.Fatalf("storage.Open on legacy db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Antes da correção, este passo falha: o INSERT preparado referencia
	// valid_from/valid_until/flash/recurrence_pattern/recurrence_group_id/
	// seasonal_tag, que não existem no schema legado.
	repo, err := storage.NewProcessorRepository(db.DB())
	if err != nil {
		t.Fatalf("NewProcessorRepository on legacy db: %v", err)
	}
	defer func() { _ = repo.Close() }()

	for _, col := range []string{
		"valid_from",
		"valid_until",
		"flash",
		"recurrence_pattern",
		"recurrence_group_id",
		"seasonal_tag",
		"coupon_codes",
		"modifiers",
		"virtual_currency",
		"webpage_url",
		"webpage_title",
		"webpage_desc",
		"is_promotional",
	} {
		if !columnPresent(t, db.DB(), "processed_messages", col) {
			t.Errorf("processed_messages.%s missing after Open on legacy db", col)
		}
	}

	if !columnPresent(t, db.DB(), "photo_cache", "expires_at") {
		t.Errorf("photo_cache.expires_at missing after Open on legacy db")
	}
}

// TestOpenUpgradesLegacySchemaAddsProductNameConfidence garante que um banco
// legado (criado pelas migrações antigas até 006_photo_cache.sql, sem
// product_name_confidence) recebe a coluna ao reabrir com storage.Open. O
// upgrade vem de ensureCurrentSchemaCompatibility/currentSchemaColumns, não de
// uma migration versionada (Turso/SQLite não aceita ALTER TABLE ADD COLUMN
// duplicado). Sem esse reparo, o CRE do Sprint 2 não persiste o score em bancos
// de campo já existentes.
func TestOpenUpgradesLegacySchemaAddsProductNameConfidence(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy-cre.db")
	createLegacyTursogoDB(t, dbPath)

	// Premissa: o banco legado realmente nasce sem product_name_confidence.
	dbRaw, err := sql.Open("turso", dbPath)
	if err != nil {
		t.Fatalf("sql.Open legacy raw: %v", err)
	}
	if columnPresent(t, dbRaw, "processed_messages", "product_name_confidence") {
		_ = dbRaw.Close()
		t.Fatalf("premissa violada: banco legado já possui product_name_confidence")
	}
	if err := dbRaw.Close(); err != nil {
		t.Fatalf("close raw legacy db: %v", err)
	}

	db, err := storage.Open(ctx, dbPath, nil)
	if err != nil {
		t.Fatalf("storage.Open on legacy db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if !columnPresent(t, db.DB(), "processed_messages", "product_name_confidence") {
		t.Errorf("processed_messages.product_name_confidence missing after Open on legacy db")
	}
}

func TestOpenUpgradesLegacySchemaAddsURLResolverFields(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy-url-resolver.db")
	createLegacyTursogoDB(t, dbPath)

	dbRaw, err := sql.Open("turso", dbPath)
	if err != nil {
		t.Fatalf("sql.Open legacy raw: %v", err)
	}
	for _, col := range []string{"canonical_url", "url_title", "url_resolved"} {
		if columnPresent(t, dbRaw, "processed_messages", col) {
			_ = dbRaw.Close()
			t.Fatalf("premissa violada: banco legado já possui %s", col)
		}
	}
	if err := dbRaw.Close(); err != nil {
		t.Fatalf("close raw legacy db: %v", err)
	}

	db, err := storage.Open(ctx, dbPath, nil)
	if err != nil {
		t.Fatalf("storage.Open on legacy db: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, col := range []string{"canonical_url", "url_title", "url_resolved"} {
		if !columnPresent(t, db.DB(), "processed_messages", col) {
			t.Errorf("processed_messages.%s missing after Open on legacy db", col)
		}
	}
	var table string
	if err := db.DB().QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
		"url_resolutions").Scan(&table); err != nil {
		t.Fatalf("url_resolutions table missing after legacy Open: %v", err)
	}
}
