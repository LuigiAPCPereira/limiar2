package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/limiar/collector/internal/logger"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ensureMigrationsTable cria a tabela de controle de migrações caso ela não exista.
// Chamada antes de qualquer verificação de versão.
func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

// migrationApplied verifica se uma versão específica já foi aplicada.
func migrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return count > 0, nil
}

type schemaColumn struct {
	table      string
	name       string
	definition string
}

var currentSchemaColumns = []schemaColumn{
	{table: "processed_messages", name: "valid_from", definition: "TEXT"},
	{table: "processed_messages", name: "valid_until", definition: "TEXT"},
	{table: "processed_messages", name: "flash", definition: "INTEGER DEFAULT 0"},
	{table: "processed_messages", name: "recurrence_pattern", definition: "TEXT DEFAULT ''"},
	{table: "processed_messages", name: "recurrence_group_id", definition: "INTEGER DEFAULT 0"},
	{table: "processed_messages", name: "seasonal_tag", definition: "TEXT DEFAULT ''"},
	{table: "photo_cache", name: "expires_at", definition: "TEXT"},
}

var currentSchemaIndexes = []string{
	`CREATE INDEX IF NOT EXISTS idx_processed_flash
		ON processed_messages(flash) WHERE flash = 1`,
	`CREATE INDEX IF NOT EXISTS idx_processed_valid_until
		ON processed_messages(valid_until) WHERE valid_until IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_processed_recurrence_group
		ON processed_messages(recurrence_group_id) WHERE recurrence_group_id != 0`,
	`CREATE INDEX IF NOT EXISTS idx_photo_cache_expires
		ON photo_cache(expires_at) WHERE expires_at IS NOT NULL`,
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return false, err
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
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func ensureCurrentSchemaCompatibility(ctx context.Context, db *sql.DB, log logger.Logger) error {
	added := 0
	for _, col := range currentSchemaColumns {
		ok, err := columnExists(ctx, db, col.table, col.name)
		if err != nil {
			return fmt.Errorf("check column %s.%s: %w", col.table, col.name, err)
		}
		if ok {
			continue
		}
		stmt := fmt.Sprintf(`ALTER TABLE "%s" ADD COLUMN "%s" %s`, col.table, col.name, col.definition)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("add column %s.%s: %w", col.table, col.name, err)
		}
		added++
	}

	for _, stmt := range currentSchemaIndexes {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure current schema index: %w", err)
		}
	}
	if added > 0 {
		log.Info("✅ Schema legado atualizado", "colunas_adicionadas", added)
	}
	return nil
}

// migrate aplica toda migration embutida na ordem lexical do nome do arquivo.
// Cada migration é executada dentro de uma transação; versões já registradas são puladas.
func migrate(ctx context.Context, db *sql.DB, log logger.Logger) error {
	if log == nil {
		log = logger.NopLogger{}
	}
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	log.Info("🔍 Verificando migrações", "total", len(names))
	preExisting := 0
	executed := 0

	for _, name := range names {
		migrated, err := migrationApplied(ctx, db, name)
		if err != nil {
			return err
		}
		if migrated {
			preExisting++
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		log.Info("⚙️ Aplicando migração", "nome", name, "bytes", len(sqlBytes))
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx %s: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %s: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
		executed++
	}

	if err := ensureCurrentSchemaCompatibility(ctx, db, log); err != nil {
		return err
	}

	log.Info("✅ Migrações concluídas", "já_aplicadas", preExisting, "executadas", executed)
	return nil
}
