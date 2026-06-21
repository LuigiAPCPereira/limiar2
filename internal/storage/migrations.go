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

// migrate aplica toda migration embutida na ordem lexical do nome do arquivo.
// Cada migration é executada dentro de uma transação. Migrations já registradas
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

	log.Info("✅ Migrações concluídas", "já_aplicadas", preExisting, "executadas", executed)
	return nil
}
