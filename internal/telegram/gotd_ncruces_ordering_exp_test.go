//go:build exp010_ncruces

package telegram_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func openManagerNcrucesDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=FULL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			t.Fatalf("%s: %v", pragma, err)
		}
	}
	return db
}

func newManagerNcrucesStorage(t *testing.T, initial updates.State) (string, *sql.DB, *managerPhysicalStorage) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manager-ncruces-ordering.db")
	db := openManagerNcrucesDB(t, path)
	if _, err := db.Exec(managerPhysicalOrderingSchema); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO source_sync_state(user_id, pts, qts, date, seq) VALUES (?, ?, ?, ?, ?)`,
		contractUserID, initial.Pts, initial.Qts, initial.Date, initial.Seq); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return path, db, &managerPhysicalStorage{
		db:      db,
		barrier: newDurabilityBarrier(),
		results: make(chan stateWriteResult, 32),
	}
}

func TestEXP010_ManagerNcrucesOrdering_EvidenceCommitPrecedesPtsAdvance(t *testing.T) {
	path, db, storage := newManagerNcrucesStorage(t, updates.State{Pts: 7})
	running := startManagerWithPhysicalStorage(t, storage, gotdtelegram.UpdateHandlerFunc(func(ctx context.Context, _ tg.UpdatesClass) error {
		return storage.appendEvidence(ctx, []byte("live-update"))
	}))

	if err := running.manager.Handle(context.Background(), ptsUpdate(8)); err != nil {
		running.stop(t)
		_ = db.Close()
		t.Fatalf("Handle returned error: %v", err)
	}
	if result := waitPhysicalStateWrite(t, storage, "pts"); result.err != nil {
		running.stop(t)
		_ = db.Close()
		t.Fatalf("SetPts failed: %v", result.err)
	}
	running.stop(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openManagerNcrucesDB(t, path)
	defer func() { _ = reopened.Close() }()
	evidence, pts := readPhysicalSnapshot(t, reopened)
	if evidence != 1 || pts != 8 {
		t.Fatalf("reopened evidence=%d pts=%d, want evidence=1 pts=8", evidence, pts)
	}
}

func TestEXP010_ManagerNcrucesOrdering_FailureAfterEvidenceCommitKeepsOldPts(t *testing.T) {
	path, db, storage := newManagerNcrucesStorage(t, updates.State{Pts: 7})
	running := startManagerWithPhysicalStorage(t, storage, gotdtelegram.UpdateHandlerFunc(func(ctx context.Context, _ tg.UpdatesClass) error {
		if err := storage.appendEvidence(ctx, []byte("live-update")); err != nil {
			return err
		}
		storage.barrier.Fail(errEvidencePersistence)
		return errEvidencePersistence
	}))

	if err := running.manager.Handle(context.Background(), ptsUpdate(8)); err != nil {
		running.stop(t)
		_ = db.Close()
		t.Fatalf("Handle unexpectedly propagated handler error: %v", err)
	}
	result := waitPhysicalStateWrite(t, storage, "pts")
	if !errors.Is(result.err, errEvidencePersistence) {
		running.stop(t)
		_ = db.Close()
		t.Fatalf("SetPts error=%v, want evidence persistence failure", result.err)
	}
	running.stop(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openManagerNcrucesDB(t, path)
	defer func() { _ = reopened.Close() }()
	evidence, pts := readPhysicalSnapshot(t, reopened)
	if evidence != 1 || pts != 7 {
		t.Fatalf("reopened evidence=%d pts=%d, want evidence=1 pts=7", evidence, pts)
	}
	var integrity string
	if err := reopened.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q", integrity)
	}
}
