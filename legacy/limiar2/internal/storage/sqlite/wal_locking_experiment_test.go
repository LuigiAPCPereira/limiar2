package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestWALReaderSnapshotDoesNotBlockEvidenceAppend(t *testing.T) {
	path, store := openTestStore(t)
	defer closeTestStore(t, store)

	appendEvidence(t, store, sampleEvidence([]byte("before-reader-snapshot")))

	observer := mustOpenSQL(t, path, true)
	observer.SetMaxOpenConns(1)
	observer.SetMaxIdleConns(1)
	defer func() { _ = observer.Close() }()

	requireEqual(t, "observer journal_mode", mustQueryString(t, observer, `PRAGMA journal_mode`), "wal")

	readTx, err := observer.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = readTx.Rollback() }()

	var count int64
	if err := readTx.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	requireEqual(t, "reader snapshot initial count", count, int64(1))

	writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := store.EvidenceAppender().Append(writeCtx, sampleEvidence([]byte("during-reader-snapshot"))); err != nil {
		t.Fatalf("append com reader WAL ativo: %v", err)
	}

	if err := readTx.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	requireEqual(t, "reader snapshot remains stable", count, int64(1))

	if err := readTx.Commit(); err != nil {
		t.Fatal(err)
	}
	requireEqual(t, "observer sees committed append after snapshot",
		mustQueryInt64(t, observer, `SELECT COUNT(*) FROM evidence`), int64(2))
}
