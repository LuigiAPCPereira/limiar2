package evidenceenvelope

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

const backfillProgressSemanticsSchema = `
CREATE TABLE backfill_progress_semantics (
    subscription_id TEXT NOT NULL PRIMARY KEY,
    last_message_id INTEGER NOT NULL CHECK(last_message_id >= 0),
    completed       INTEGER NOT NULL DEFAULT 0 CHECK(completed IN (0, 1))
) STRICT, WITHOUT ROWID;
`

type backfillProgress struct {
	lastMessageID int64
	completed     bool
}

func openBackfillProgressDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backfill-progress-semantics.db")
	db := openDB(t, path)
	if _, err := db.Exec(backfillProgressSemanticsSchema); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return path, db
}

func readBackfillProgress(db *sql.DB, subscription string) (backfillProgress, bool, error) {
	var progress backfillProgress
	var completed int
	err := db.QueryRow(`SELECT last_message_id, completed FROM backfill_progress_semantics WHERE subscription_id=?`, subscription).
		Scan(&progress.lastMessageID, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return backfillProgress{}, false, nil
	}
	if err != nil {
		return backfillProgress{}, false, err
	}
	progress.completed = completed == 1
	return progress, true, nil
}

func completedInt(completed bool) int {
	if completed {
		return 1
	}
	return 0
}

func updateBackfillProgress(tx *sql.Tx, subscription string, lastMessageID int64, completed int) (bool, error) {
	result, err := tx.Exec(`UPDATE backfill_progress_semantics
		SET last_message_id=?, completed=?
		WHERE subscription_id=? AND last_message_id <= ?`,
		lastMessageID, completed, subscription, lastMessageID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func backfillProgressExists(tx *sql.Tx, subscription string) (bool, error) {
	var exists int
	if err := tx.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM backfill_progress_semantics WHERE subscription_id=?
	)`, subscription).Scan(&exists); err != nil {
		return false, err
	}
	return exists == 1, nil
}

func insertBackfillProgress(tx *sql.Tx, subscription string, lastMessageID int64, completed int) error {
	_, err := tx.Exec(`INSERT INTO backfill_progress_semantics(subscription_id, last_message_id, completed)
		VALUES (?, ?, ?)`, subscription, lastMessageID, completed)
	return err
}

func advanceBackfillProgress(db *sql.DB, subscription string, lastMessageID int64, completed bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	completedValue := completedInt(completed)
	updated, err := updateBackfillProgress(tx, subscription, lastMessageID, completedValue)
	if err != nil {
		return err
	}
	if updated {
		return tx.Commit()
	}

	exists, err := backfillProgressExists(tx, subscription)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("backfill progress regression rejected")
	}
	if err := insertBackfillProgress(tx, subscription, lastMessageID, completedValue); err != nil {
		return err
	}
	return tx.Commit()
}

func requireBackfillProgress(t *testing.T, db *sql.DB, subscription string, want backfillProgress) {
	t.Helper()
	got, found, err := readBackfillProgress(db, subscription)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("progress for %q not found", subscription)
	}
	if got != want {
		t.Fatalf("progress for %q=%+v, want %+v", subscription, got, want)
	}
}

func TestBackfillProgressAbsenceIsDistinctFromZero(t *testing.T) {
	_, db := openBackfillProgressDB(t)
	defer func() { _ = db.Close() }()

	const subscription = "telegram:channel:42"
	progress, found, err := readBackfillProgress(db, subscription)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("unexpected progress for absent subscription: %+v", progress)
	}

	if err := advanceBackfillProgress(db, subscription, 0, false); err != nil {
		t.Fatal(err)
	}
	requireBackfillProgress(t, db, subscription, backfillProgress{lastMessageID: 0, completed: false})
}

func TestBackfillProgressRejectsRegressionAndPersistsAfterReopen(t *testing.T) {
	path, db := openBackfillProgressDB(t)
	const subscription = "telegram:channel:99"

	if err := advanceBackfillProgress(db, subscription, 100, false); err != nil {
		t.Fatal(err)
	}
	if err := advanceBackfillProgress(db, subscription, 125, true); err != nil {
		t.Fatal(err)
	}
	if err := advanceBackfillProgress(db, subscription, 120, false); err == nil {
		t.Fatal("expected regression to be rejected")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openDB(t, path)
	defer func() { _ = reopened.Close() }()
	requireBackfillProgress(t, reopened, subscription, backfillProgress{lastMessageID: 125, completed: true})
}

func TestBackfillProgressIsolatedBySubscription(t *testing.T) {
	_, db := openBackfillProgressDB(t)
	defer func() { _ = db.Close() }()

	if err := advanceBackfillProgress(db, "telegram:channel:1", 10, false); err != nil {
		t.Fatal(err)
	}
	if err := advanceBackfillProgress(db, "telegram:channel:2", 20, true); err != nil {
		t.Fatal(err)
	}

	requireBackfillProgress(t, db, "telegram:channel:1", backfillProgress{lastMessageID: 10, completed: false})
	requireBackfillProgress(t, db, "telegram:channel:2", backfillProgress{lastMessageID: 20, completed: true})
}
