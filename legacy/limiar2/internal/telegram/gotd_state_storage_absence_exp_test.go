package telegram_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestStateStoragePartialSettersFailWhenUserStateMissing(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missing-state.db")
	db := openManagerPhysicalDB(t, path)
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(managerPhysicalOrderingSchema); err != nil {
		t.Fatal(err)
	}

	storage := &managerPhysicalStorage{
		db:      db,
		barrier: newDurabilityBarrier(),
		results: make(chan stateWriteResult, 16),
	}
	const missingUserID int64 = 909

	setters := []struct {
		name string
		set  func() error
	}{
		{name: "pts", set: func() error { return storage.SetPts(ctx, missingUserID, 1) }},
		{name: "qts", set: func() error { return storage.SetQts(ctx, missingUserID, 2) }},
		{name: "date", set: func() error { return storage.SetDate(ctx, missingUserID, 3) }},
		{name: "seq", set: func() error { return storage.SetSeq(ctx, missingUserID, 4) }},
		{name: "date-seq", set: func() error { return storage.SetDateSeq(ctx, missingUserID, 5, 6) }},
	}

	for _, setter := range setters {
		t.Run(setter.name, func(t *testing.T) {
			if err := setter.set(); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("setter error=%v, want sql.ErrNoRows", err)
			}
		})
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_sync_state WHERE user_id=?`, missingUserID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("missing user state was fabricated: rows=%d", count)
	}
}
