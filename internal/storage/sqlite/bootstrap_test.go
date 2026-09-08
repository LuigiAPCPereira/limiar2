package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenResumesClaimedUnmigratedDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "partially-initialized.db")

	db, err := sql.Open(driverName, databaseURI(path, false))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA synchronous=FULL`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := claimNewDatabase(ctx, db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}

	var beforeVersion int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&beforeVersion); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if beforeVersion != 0 {
		_ = db.Close()
		t.Fatalf("user_version antes do restart=%d, want 0", beforeVersion)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open não retomou bootstrap parcial: %v", err)
	}
	defer func() { _ = store.Close() }()

	var gotApplicationID int64
	if err := store.db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&gotApplicationID); err != nil {
		t.Fatal(err)
	}
	if gotApplicationID != applicationID {
		t.Fatalf("application_id=%d, want %d", gotApplicationID, applicationID)
	}

	var userVersion int
	if err := store.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	if userVersion != 1 {
		t.Fatalf("user_version após retomada=%d, want 1", userVersion)
	}

	var evidenceTables int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='evidence'`).Scan(&evidenceTables); err != nil {
		t.Fatal(err)
	}
	if evidenceTables != 1 {
		t.Fatalf("evidence table count=%d, want 1", evidenceTables)
	}
}
