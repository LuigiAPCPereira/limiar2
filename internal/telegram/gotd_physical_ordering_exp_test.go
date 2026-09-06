package telegram_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	_ "turso.tech/database/tursogo"
)

const managerPhysicalOrderingSchema = `
CREATE TABLE source_sync_state (
    user_id INTEGER PRIMARY KEY,
    pts     INTEGER NOT NULL,
    qts     INTEGER NOT NULL,
    date    INTEGER NOT NULL,
    seq     INTEGER NOT NULL
);
CREATE TABLE evidence (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    payload BLOB NOT NULL
);
`

type managerPhysicalStorage struct {
	db      *sql.DB
	barrier *durabilityBarrier
	results chan stateWriteResult
}

func openManagerPhysicalDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("turso", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=FULL`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}

func newManagerPhysicalStorage(t *testing.T, initial updates.State) (string, *sql.DB, *managerPhysicalStorage) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manager-ordering.db")
	db := openManagerPhysicalDB(t, path)
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

func (s *managerPhysicalStorage) GetState(ctx context.Context, userID int64) (updates.State, bool, error) {
	var state updates.State
	err := s.db.QueryRowContext(ctx, `SELECT pts, qts, date, seq FROM source_sync_state WHERE user_id=?`, userID).
		Scan(&state.Pts, &state.Qts, &state.Date, &state.Seq)
	if errors.Is(err, sql.ErrNoRows) {
		return updates.State{}, false, nil
	}
	return state, err == nil, err
}

func (s *managerPhysicalStorage) SetState(ctx context.Context, userID int64, state updates.State) error {
	return s.write("state", func() error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO source_sync_state(user_id, pts, qts, date, seq) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(user_id) DO UPDATE SET pts=excluded.pts, qts=excluded.qts, date=excluded.date, seq=excluded.seq`,
			userID, state.Pts, state.Qts, state.Date, state.Seq)
		return err
	})
}

func (s *managerPhysicalStorage) SetPts(ctx context.Context, userID int64, pts int) error {
	return s.write("pts", func() error {
		_, err := s.db.ExecContext(ctx, `UPDATE source_sync_state SET pts=? WHERE user_id=?`, pts, userID)
		return err
	})
}

func (s *managerPhysicalStorage) SetQts(ctx context.Context, userID int64, qts int) error {
	return s.write("qts", func() error {
		_, err := s.db.ExecContext(ctx, `UPDATE source_sync_state SET qts=? WHERE user_id=?`, qts, userID)
		return err
	})
}

func (s *managerPhysicalStorage) SetDate(ctx context.Context, userID int64, date int) error {
	return s.write("date", func() error {
		_, err := s.db.ExecContext(ctx, `UPDATE source_sync_state SET date=? WHERE user_id=?`, date, userID)
		return err
	})
}

func (s *managerPhysicalStorage) SetSeq(ctx context.Context, userID int64, seq int) error {
	return s.write("seq", func() error {
		_, err := s.db.ExecContext(ctx, `UPDATE source_sync_state SET seq=? WHERE user_id=?`, seq, userID)
		return err
	})
}

func (s *managerPhysicalStorage) SetDateSeq(ctx context.Context, userID int64, date, seq int) error {
	return s.write("date-seq", func() error {
		_, err := s.db.ExecContext(ctx, `UPDATE source_sync_state SET date=?, seq=? WHERE user_id=?`, date, seq, userID)
		return err
	})
}

func (s *managerPhysicalStorage) GetChannelPts(context.Context, int64, int64) (int, bool, error) {
	return 0, false, nil
}

func (s *managerPhysicalStorage) SetChannelPts(context.Context, int64, int64, int) error {
	return nil
}

func (s *managerPhysicalStorage) ForEachChannels(context.Context, int64, func(context.Context, int64, int) error) error {
	return nil
}

func (s *managerPhysicalStorage) write(kind string, fn func() error) error {
	err := s.barrier.GuardStateWrite(fn)
	s.results <- stateWriteResult{kind: kind, err: err}
	return err
}

func (s *managerPhysicalStorage) appendEvidence(ctx context.Context, payload []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO evidence(payload) VALUES (?)`, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func startManagerWithPhysicalStorage(t *testing.T, storage *managerPhysicalStorage, handler gotdtelegram.UpdateHandler) runningManager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	manager := updates.New(updates.Config{Storage: storage, Handler: handler})
	api := newContractAPI()
	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{
			OnStart: func(context.Context) { close(started) },
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("updates.Manager did not start")
	}
	return runningManager{manager: manager, cancel: cancel, done: done}
}

func waitPhysicalStateWrite(t *testing.T, storage *managerPhysicalStorage, kind string) stateWriteResult {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case result := <-storage.results:
			if result.kind == kind {
				return result
			}
		case <-deadline:
			t.Fatalf("physical state write %q was not attempted", kind)
		}
	}
}

func readPhysicalSnapshot(t *testing.T, db *sql.DB) (evidenceCount int, pts int) {
	t.Helper()
	if err := db.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&evidenceCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT pts FROM source_sync_state WHERE user_id=?`, contractUserID).Scan(&pts); err != nil {
		t.Fatal(err)
	}
	return evidenceCount, pts
}

func TestEXP009_ManagerPhysicalOrdering_EvidenceCommitPrecedesPtsAdvance(t *testing.T) {
	path, db, storage := newManagerPhysicalStorage(t, updates.State{Pts: 7})
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

	reopened := openManagerPhysicalDB(t, path)
	defer func() { _ = reopened.Close() }()
	evidence, pts := readPhysicalSnapshot(t, reopened)
	if evidence != 1 || pts != 8 {
		t.Fatalf("reopened evidence=%d pts=%d, want evidence=1 pts=8", evidence, pts)
	}
}

func TestEXP009_ManagerPhysicalOrdering_FailureAfterEvidenceCommitKeepsOldPts(t *testing.T) {
	path, db, storage := newManagerPhysicalStorage(t, updates.State{Pts: 7})
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

	reopened := openManagerPhysicalDB(t, path)
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
