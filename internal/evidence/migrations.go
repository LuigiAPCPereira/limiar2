package evidence

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

type migrationSpec struct {
	name    string
	version int
}

func migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("ler user_version: %w", err)
	}

	specs, err := loadMigrationSpecs()
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return fmt.Errorf("nenhuma migration disponível")
	}
	latest := specs[len(specs)-1].version
	if current > latest {
		return fmt.Errorf("user_version=%d maior que a última migration disponível=%d", current, latest)
	}

	for _, spec := range specs {
		if spec.version <= current {
			continue
		}
		if spec.version != current+1 {
			return fmt.Errorf("gap de migration: atual=%d próxima=%d (%s)", current, spec.version, spec.name)
		}
		if err := applyMigration(ctx, db, spec); err != nil {
			return err
		}
		current = spec.version
	}

	return nil
}

func loadMigrationSpecs() ([]migrationSpec, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("ler diretório de migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	return parseMigrationSpecs(names)
}

func parseMigrationSpecs(names []string) ([]migrationSpec, error) {
	specs := make([]migrationSpec, 0, len(names))
	seen := make(map[int]string, len(names))

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return nil, err
		}
		if previous, ok := seen[version]; ok {
			return nil, fmt.Errorf("versão de migration duplicada: %d (%s e %s)", version, previous, name)
		}
		seen[version] = name
		specs = append(specs, migrationSpec{name: name, version: version})
	}

	return specs, nil
}

func applyMigration(ctx context.Context, db *sql.DB, spec migrationSpec) error {
	body, err := migrationsFS.ReadFile("migrations/" + spec.name)
	if err != nil {
		return fmt.Errorf("ler migration %s: %w", spec.name, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar migration %s: %w", spec.name, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("executar migration %s: %w", spec.name, err)
	}

	var applied int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&applied); err != nil {
		return fmt.Errorf("verificar user_version após %s: %w", spec.name, err)
	}
	if applied != spec.version {
		return fmt.Errorf("migration %s declarou user_version=%d, esperado=%d", spec.name, applied, spec.version)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", spec.name, err)
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
