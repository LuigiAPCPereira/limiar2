package syncstate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type columnSpec struct {
	name    string
	sqlType string
	notNull int
	pk      int
}

var sourceSyncStateColumns = []columnSpec{
	{name: "subscription_id", sqlType: "TEXT", notNull: 1, pk: 1},
	{name: "user_id", sqlType: "INTEGER", notNull: 1, pk: 2},
	{name: "pts", sqlType: "INTEGER", notNull: 1},
	{name: "qts", sqlType: "INTEGER", notNull: 1},
	{name: "date", sqlType: "INTEGER", notNull: 1},
	{name: "seq", sqlType: "INTEGER", notNull: 1},
}

var sourceSyncChannelStateColumns = []columnSpec{
	{name: "subscription_id", sqlType: "TEXT", notNull: 1, pk: 1},
	{name: "user_id", sqlType: "INTEGER", notNull: 1, pk: 2},
	{name: "channel_id", sqlType: "INTEGER", notNull: 1, pk: 3},
	{name: "pts", sqlType: "INTEGER", notNull: 1},
}

// ValidateSchema confirma a forma física mínima aceita pela L3 ADR 007.
// A função somente observa; migrations SQL continuam sendo a única authority.
func ValidateSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("schema de SourceSyncState: conexão ausente")
	}
	if err := validateTable(ctx, db, "source_sync_state", sourceSyncStateColumns); err != nil {
		return err
	}
	if err := validateTable(ctx, db, "source_sync_channel_state", sourceSyncChannelStateColumns); err != nil {
		return err
	}
	return nil
}

func validateTable(ctx context.Context, db *sql.DB, name string, want []columnSpec) error {
	var kind, createSQL string
	err := db.QueryRowContext(ctx,
		"SELECT type, sql FROM main.sqlite_schema WHERE name = ?", name,
	).Scan(&kind, &createSQL)
	if err == sql.ErrNoRows {
		return fmt.Errorf("schema de SourceSyncState: tabela %q ausente", name)
	}
	if err != nil {
		return fmt.Errorf("schema de SourceSyncState: inspecionar tabela %q: %w", name, err)
	}
	if kind != "table" {
		return fmt.Errorf("schema de SourceSyncState: objeto %q type=%q, esperado table", name, kind)
	}
	if !strings.Contains(strings.ToUpper(createSQL), ") STRICT") {
		return fmt.Errorf("schema de SourceSyncState: tabela %q não é STRICT", name)
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", name))
	if err != nil {
		return fmt.Errorf("schema de SourceSyncState: ler colunas de %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	got := make([]columnSpec, 0, len(want))
	for rows.Next() {
		var (
			cid          int
			column       columnSpec
			defaultValue sql.NullString
		)
		if err := rows.Scan(&cid, &column.name, &column.sqlType, &column.notNull, &defaultValue, &column.pk); err != nil {
			return fmt.Errorf("schema de SourceSyncState: scan de %q: %w", name, err)
		}
		got = append(got, column)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("schema de SourceSyncState: finalizar colunas de %q: %w", name, err)
	}
	if len(got) != len(want) {
		return fmt.Errorf("schema de SourceSyncState: tabela %q possui %d colunas, esperado %d", name, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("schema de SourceSyncState: tabela %q coluna %d=%+v, esperado %+v", name, i, got[i], want[i])
		}
	}

	fkRows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%q)", name))
	if err != nil {
		return fmt.Errorf("schema de SourceSyncState: inspecionar foreign keys de %q: %w", name, err)
	}
	defer func() { _ = fkRows.Close() }()
	if fkRows.Next() {
		return fmt.Errorf("schema de SourceSyncState: tabela %q possui foreign key não autorizada", name)
	}
	if err := fkRows.Err(); err != nil {
		return fmt.Errorf("schema de SourceSyncState: finalizar foreign keys de %q: %w", name, err)
	}
	return nil
}
