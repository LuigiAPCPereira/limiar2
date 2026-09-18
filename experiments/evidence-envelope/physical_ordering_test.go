package evidenceenvelope

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

const authorityStateSchema = `
CREATE TABLE source_sync_state (
    subscription_id TEXT NOT NULL PRIMARY KEY,
    pts             INTEGER NOT NULL CHECK(pts >= 0)
) STRICT, WITHOUT ROWID;

CREATE TABLE backfill_progress (
    subscription_id TEXT NOT NULL PRIMARY KEY,
    last_message_id INTEGER NOT NULL CHECK(last_message_id >= 0)
) STRICT, WITHOUT ROWID;
`

var errAfterEvidenceCommit = errors.New("injected failure after evidence commit")

type physicalOrderingStore struct {
	db *sql.DB
}

func (s physicalOrderingStore) appendEvidence(ctx context.Context, subscription string, payload []byte) (evidenceID, error) {
	id, err := newEvidenceID(rand.Reader)
	if err != nil {
		return evidenceID{}, err
	}
	hash := sha256.Sum256(payload)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return evidenceID{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, ?, 'live_update', 'source_update', 'updates', 1, 'telegram-tl', 'v1', ?, ?)`,
		id[:], subscription, payload, hash[:]); err != nil {
		return evidenceID{}, err
	}
	if err := tx.Commit(); err != nil {
		return evidenceID{}, err
	}
	return id, nil
}

func (s physicalOrderingStore) persistEvidenceThenSourceState(ctx context.Context, subscription string, payload []byte, pts int64, failAfterEvidence bool) error {
	if _, err := s.appendEvidence(ctx, subscription, payload); err != nil {
		return err
	}
	if failAfterEvidence {
		return errAfterEvidenceCommit
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO source_sync_state(subscription_id, pts) VALUES (?, ?)
		ON CONFLICT(subscription_id) DO UPDATE SET pts=excluded.pts`, subscription, pts)
	return err
}

func (s physicalOrderingStore) persistEvidenceThenBackfillProgress(ctx context.Context, subscription string, payload []byte, lastMessageID int64, failAfterEvidence bool) error {
	if _, err := s.appendEvidence(ctx, subscription, payload); err != nil {
		return err
	}
	if failAfterEvidence {
		return errAfterEvidenceCommit
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO backfill_progress(subscription_id, last_message_id) VALUES (?, ?)
		ON CONFLICT(subscription_id) DO UPDATE SET last_message_id=excluded.last_message_id`, subscription, lastMessageID)
	return err
}

func openPhysicalOrderingStore(t *testing.T) (string, *sql.DB, physicalOrderingStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "physical-ordering.db")
	db := openDB(t, path)
	if _, err := db.Exec(evidenceSchema + authorityStateSchema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return path, db, physicalOrderingStore{db: db}
}

func evidenceCount(t *testing.T, db *sql.DB, subscription string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM evidence WHERE subscription_id=?`, subscription).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPhysicalOrdering_SourceSyncStateNeverAdvancesWithoutEvidence(t *testing.T) {
	ctx := context.Background()
	path, db, store := openPhysicalOrderingStore(t)
	const subscription = "telegram:channel:42"

	if _, err := db.Exec(`INSERT INTO source_sync_state(subscription_id, pts) VALUES (?, 7)`, subscription); err != nil {
		t.Fatal(err)
	}
	if err := store.persistEvidenceThenSourceState(ctx, subscription, []byte("update-1"), 8, true); !errors.Is(err, errAfterEvidenceCommit) {
		t.Fatalf("failure injection error=%v", err)
	}
	if got := evidenceCount(t, db, subscription); got != 1 {
		t.Fatalf("evidence count=%d, want 1", got)
	}
	var pts int64
	if err := db.QueryRow(`SELECT pts FROM source_sync_state WHERE subscription_id=?`, subscription).Scan(&pts); err != nil {
		t.Fatal(err)
	}
	if pts != 7 {
		t.Fatalf("pts=%d, want old state 7 after injected failure", pts)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openDB(t, path)
	defer reopened.Close()
	if got := evidenceCount(t, reopened, subscription); got != 1 {
		t.Fatalf("reopened evidence count=%d, want 1", got)
	}
	if err := reopened.QueryRow(`SELECT pts FROM source_sync_state WHERE subscription_id=?`, subscription).Scan(&pts); err != nil {
		t.Fatal(err)
	}
	if pts != 7 {
		t.Fatalf("reopened pts=%d, want 7", pts)
	}
}

func TestPhysicalOrdering_BackfillProgressNeverAdvancesWithoutEvidence(t *testing.T) {
	ctx := context.Background()
	path, db, store := openPhysicalOrderingStore(t)
	const subscription = "telegram:channel:99"

	if _, err := db.Exec(`INSERT INTO backfill_progress(subscription_id, last_message_id) VALUES (?, 100)`, subscription); err != nil {
		t.Fatal(err)
	}
	if err := store.persistEvidenceThenBackfillProgress(ctx, subscription, []byte("history-101"), 101, true); !errors.Is(err, errAfterEvidenceCommit) {
		t.Fatalf("failure injection error=%v", err)
	}
	if got := evidenceCount(t, db, subscription); got != 1 {
		t.Fatalf("evidence count=%d, want 1", got)
	}
	var lastMessageID int64
	if err := db.QueryRow(`SELECT last_message_id FROM backfill_progress WHERE subscription_id=?`, subscription).Scan(&lastMessageID); err != nil {
		t.Fatal(err)
	}
	if lastMessageID != 100 {
		t.Fatalf("last_message_id=%d, want old progress 100 after injected failure", lastMessageID)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openDB(t, path)
	defer reopened.Close()
	if got := evidenceCount(t, reopened, subscription); got != 1 {
		t.Fatalf("reopened evidence count=%d, want 1", got)
	}
	if err := reopened.QueryRow(`SELECT last_message_id FROM backfill_progress WHERE subscription_id=?`, subscription).Scan(&lastMessageID); err != nil {
		t.Fatal(err)
	}
	if lastMessageID != 100 {
		t.Fatalf("reopened last_message_id=%d, want 100", lastMessageID)
	}
}

func TestPhysicalOrdering_SuccessAdvancesOnlyAfterEvidenceCommit(t *testing.T) {
	ctx := context.Background()
	_, db, store := openPhysicalOrderingStore(t)
	defer db.Close()

	if err := store.persistEvidenceThenSourceState(ctx, "telegram:live", []byte("update"), 55, false); err != nil {
		t.Fatal(err)
	}
	if err := store.persistEvidenceThenBackfillProgress(ctx, "telegram:history", []byte("history"), 321, false); err != nil {
		t.Fatal(err)
	}

	if got := evidenceCount(t, db, "telegram:live"); got != 1 {
		t.Fatalf("live evidence count=%d, want 1", got)
	}
	if got := evidenceCount(t, db, "telegram:history"); got != 1 {
		t.Fatalf("history evidence count=%d, want 1", got)
	}
	var pts, lastMessageID int64
	if err := db.QueryRow(`SELECT pts FROM source_sync_state WHERE subscription_id='telegram:live'`).Scan(&pts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT last_message_id FROM backfill_progress WHERE subscription_id='telegram:history'`).Scan(&lastMessageID); err != nil {
		t.Fatal(err)
	}
	if pts != 55 || lastMessageID != 321 {
		t.Fatalf("state/progress pts=%d last_message_id=%d", pts, lastMessageID)
	}

	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q", integrity)
	}
}
