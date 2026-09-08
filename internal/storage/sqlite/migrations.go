package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("ler user_version: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("ler diretório de migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		if version != current+1 {
			return fmt.Errorf("gap de migration: atual=%d próxima=%d (%s)", current, version, name)
		}

		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("ler migration %s: %w", name, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("iniciar migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("executar migration %s: %w", name, err)
		}

		var applied int
		if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&applied); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("verificar user_version após %s: %w", name, err)
		}
		if applied != version {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s declarou user_version=%d, esperado=%d", name, applied, version)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
		current = version
	}

	return nil
}

func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("nome de migration inválido %q", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("versão de migration inválida em %q", name)
	}
	return version, nil
}
