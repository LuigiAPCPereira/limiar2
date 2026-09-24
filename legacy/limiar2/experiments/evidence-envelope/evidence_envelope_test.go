package evidenceenvelope

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

type evidenceID [16]byte

func newEvidenceID(r io.Reader) (evidenceID, error) {
	var id evidenceID
	if _, err := io.ReadFull(r, id[:]); err != nil {
		return evidenceID{}, fmt.Errorf("read evidence id entropy: %w", err)
	}
	// RFC 9562 UUIDv4: versão 4 + variant 10xx. A identidade continua randômica;
	// tempo de aquisição permanece um campo separado do envelope.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

const evidenceSchema = `
CREATE TABLE evidence (
    id                 BLOB    NOT NULL PRIMARY KEY CHECK(length(id) = 16),
    subscription_id    TEXT    NOT NULL CHECK(length(subscription_id) > 0),
    acquisition        TEXT    NOT NULL CHECK(length(acquisition) > 0),
    event_kind         TEXT    NOT NULL CHECK(length(event_kind) > 0),
    source_event_type  TEXT    NOT NULL CHECK(length(source_event_type) > 0),
    source_occurred_at INTEGER,
    received_at        INTEGER NOT NULL,
    payload_format     TEXT    NOT NULL CHECK(length(payload_format) > 0),
    payload_schema     TEXT    NOT NULL CHECK(length(payload_schema) > 0),
    payload            BLOB    NOT NULL,
    payload_sha256     BLOB    NOT NULL CHECK(length(payload_sha256) = 32)
) STRICT, WITHOUT ROWID;
CREATE INDEX evidence_received_idx ON evidence(received_at, id);
`

func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
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

func TestEvidenceID_UUIDv4ShapeAndUniqueness(t *testing.T) {
	const total = 100_000
	seen := make(map[evidenceID]struct{}, total)
	for range total {
		id, err := newEvidenceID(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if id[6]>>4 != 4 {
			t.Fatalf("version bits = %x, want 4", id[6]>>4)
		}
		if id[8]>>6 != 2 {
			t.Fatalf("variant bits = %b, want 10", id[8]>>6)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("collision after %d generated ids", len(seen))
		}
		seen[id] = struct{}{}
	}
}

func TestEvidenceID_EntropyFailureIsFailClosed(t *testing.T) {
	id, err := newEvidenceID(failingReader{})
	if err == nil {
		t.Fatal("expected entropy error")
	}
	if id != (evidenceID{}) {
		t.Fatalf("id on failure = %x, want zero value + error", id)
	}
}

func TestEvidenceEnvelopeConstraintsAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.db")
	db := openDB(t, path)
	defer db.Close()
	if _, err := db.Exec(evidenceSchema); err != nil {
		t.Fatal(err)
	}

	id, err := newEvidenceID(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x00, 0xff, 0x7b, 0x22, 0x78, 0x22, 0x3a, 0x31, 0x7d}
	hash := sha256.Sum256(payload)
	const receivedAt int64 = 1_787_103_600_123

	_, err = db.Exec(`INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		source_occurred_at, received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id[:], "telegram:channel:42", "live_update", "source_update", "updates",
		nil, receivedAt, "telegram-tl", "v1", payload, hash[:])
	if err != nil {
		t.Fatal(err)
	}

	var gotID, gotPayload, gotHash []byte
	var gotReceived int64
	var sourceOccurred sql.NullInt64
	if err := db.QueryRow(`SELECT id, source_occurred_at, received_at, payload, payload_sha256 FROM evidence WHERE id = ?`, id[:]).
		Scan(&gotID, &sourceOccurred, &gotReceived, &gotPayload, &gotHash); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotID, id[:]) || !bytes.Equal(gotPayload, payload) || !bytes.Equal(gotHash, hash[:]) {
		t.Fatal("binary round-trip changed id/payload/hash")
	}
	if sourceOccurred.Valid || gotReceived != receivedAt {
		t.Fatalf("timestamp round-trip source=%v received=%d", sourceOccurred, gotReceived)
	}

	if _, err := db.Exec(`INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, 's', 'live', 'u', 'x', 1, 'f', 'v1', x'01', zeroblob(32))`, make([]byte, 15)); err == nil {
		t.Fatal("expected CHECK failure for 15-byte id")
	}
	if _, err := db.Exec(`INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, 's', 'live', 'u', 'x', 1, 'f', 'v1', x'01', zeroblob(31))`, make([]byte, 16)); err == nil {
		t.Fatal("expected CHECK failure for 31-byte hash")
	}
}

func TestEvidenceExportImportPreservesIdentity(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	targetPath := filepath.Join(t.TempDir(), "target.db")
	source := openDB(t, sourcePath)
	defer source.Close()
	target := openDB(t, targetPath)
	defer target.Close()
	for _, db := range []*sql.DB{source, target} {
		if _, err := db.Exec(evidenceSchema); err != nil {
			t.Fatal(err)
		}
	}

	const total = 1_000
	want := make(map[evidenceID][32]byte, total)
	tx, err := source.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, 'telegram:42', 'history_snapshot', 'source_snapshot', 'messages.getHistory', ?, 'telegram-tl', 'v1', ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := range total {
		id, err := newEvidenceID(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte(fmt.Sprintf("payload-%04d", i))
		h := sha256.Sum256(payload)
		if _, err := stmt.Exec(id[:], int64(1_700_000_000_000+i), payload, h[:]); err != nil {
			t.Fatal(err)
		}
		want[id] = h
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	rows, err := source.Query(`SELECT id, subscription_id, acquisition, event_kind, source_event_type,
		source_occurred_at, received_at, payload_format, payload_schema, payload, payload_sha256 FROM evidence`)
	if err != nil {
		t.Fatal(err)
	}
	targetTx, err := target.Begin()
	if err != nil {
		t.Fatal(err)
	}
	insert, err := targetTx.Prepare(`INSERT INTO evidence VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, payload, hash []byte
		var subscription, acquisition, eventKind, sourceType, format, schema string
		var sourceOccurred sql.NullInt64
		var received int64
		if err := rows.Scan(&id, &subscription, &acquisition, &eventKind, &sourceType,
			&sourceOccurred, &received, &format, &schema, &payload, &hash); err != nil {
			t.Fatal(err)
		}
		if _, err := insert.Exec(id, subscription, acquisition, eventKind, sourceType,
			sourceOccurred, received, format, schema, payload, hash); err != nil {
			t.Fatal(err)
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	_ = insert.Close()
	if err := targetTx.Commit(); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := target.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != total {
		t.Fatalf("imported rows=%d, want %d", count, total)
	}
	for id, hash := range want {
		var got []byte
		if err := target.QueryRow(`SELECT payload_sha256 FROM evidence WHERE id=?`, id[:]).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, hash[:]) {
			t.Fatalf("hash changed for id %x", id)
		}
	}
}

func TestReceivedTimeIsIndependentFromIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order.db")
	db := openDB(t, path)
	defer db.Close()
	if _, err := db.Exec(evidenceSchema); err != nil {
		t.Fatal(err)
	}
	const sameMillisecond int64 = 1_787_103_600_123
	for range 100 {
		id, err := newEvidenceID(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte("same-time")
		h := sha256.Sum256(payload)
		if _, err := db.Exec(`INSERT INTO evidence(id, subscription_id, acquisition, event_kind,
			source_event_type, received_at, payload_format, payload_schema, payload, payload_sha256)
			VALUES (?, 's', 'live', 'u', 'x', ?, 'f', 'v1', ?, ?)`, id[:], sameMillisecond, payload, h[:]); err != nil {
			t.Fatal(err)
		}
	}
	var count, distinctTimes int
	if err := db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT received_at) FROM evidence`).Scan(&count, &distinctTimes); err != nil {
		t.Fatal(err)
	}
	if count != 100 || distinctTimes != 1 {
		t.Fatalf("count=%d distinctTimes=%d", count, distinctTimes)
	}
}

func TestWithoutRowIDIsSupportedAndDiagnosticOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.db")
	db := openDB(t, path)
	defer db.Close()
	if _, err := db.Exec(evidenceSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT rowid FROM evidence LIMIT 1`); err == nil {
		t.Fatal("expected rowid to be unavailable for WITHOUT ROWID table")
	}

	// O tamanho é somente diagnóstico. O teste registra, mas não cria threshold arquitetural.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("empty schema database size=%d bytes", info.Size())
}

func TestIntegrityAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "integrity.db")
	db := openDB(t, path)
	if _, err := db.Exec(evidenceSchema); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	reopened := openDB(t, path)
	defer reopened.Close()
	var integrity string
	if err := reopened.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q", integrity)
	}
}
