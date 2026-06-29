package processor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/limiar/collector/internal/storage"
)

func openTempProcessorRepo() (*Repository, *storage.DB, func(), error) {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "limiar-processor-")
	if err != nil {
		return nil, nil, nil, err
	}
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.Open(ctx, dbPath, nil)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, nil, err
	}
	repo, err := NewRepository(db.DB())
	if err != nil {
		_ = db.Close()
		_ = os.RemoveAll(dir)
		return nil, nil, nil, err
	}
	cleanup := func() {
		_ = repo.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return repo, db, cleanup, nil
}

func _TestPhotoMetadataStats(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}
	// Cadeia FK (foreign_keys=ON): channel → raw_message → processed_message.
	mustExec(`INSERT INTO channels (id, username, title, active) VALUES (1, 'c1', 'C', 1)`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (1, 1, 101, '{}')`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (2, 1, 102, '{}')`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (3, 1, 103, '{}')`)
	// #1: metadados MTProto completos.
	mustExec(`INSERT INTO processed_messages (raw_message_id, channel_id, message_id, posted_at, photo_id, photo_access_hash, photo_file_ref, photo_dcid)
		VALUES (1, 1, 101, '2026-01-01T00:00:00Z', 100, 999, 'ref', 2)`)
	// #2: photo_id presente, mas access_hash/file_ref/dcid ausentes (parcial).
	mustExec(`INSERT INTO processed_messages (raw_message_id, channel_id, message_id, posted_at, photo_id)
		VALUES (2, 1, 102, '2026-01-01T00:00:00Z', 200)`)
	// #3: sem foto.
	mustExec(`INSERT INTO processed_messages (raw_message_id, channel_id, message_id, posted_at)
		VALUES (3, 1, 103, '2026-01-01T00:00:00Z')`)

	stats, err := repo.PhotoMetadataStats(ctx)
	if err != nil {
		t.Fatalf("PhotoMetadataStats: %v", err)
	}
	if stats.TotalProcessed != 3 {
		t.Errorf("TotalProcessed = %d, quer 3", stats.TotalProcessed)
	}
	if stats.WithPhoto != 2 {
		t.Errorf("WithPhoto = %d, quer 2", stats.WithPhoto)
	}
	if stats.CompleteMTProto != 1 {
		t.Errorf("CompleteMTProto = %d, quer 1", stats.CompleteMTProto)
	}
}
