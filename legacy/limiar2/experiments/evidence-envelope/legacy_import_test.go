package evidenceenvelope

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	sqlitestore "github.com/limiar/collector/internal/storage/sqlite"
)

const legacyRawSchema = `
CREATE TABLE channels (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 1,
    added_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_message_id INTEGER NOT NULL DEFAULT 0,
    last_collected_at TEXT
);
CREATE TABLE raw_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES channels(id),
    message_id INTEGER NOT NULL,
    payload TEXT NOT NULL,
    received_at TEXT NOT NULL DEFAULT (datetime('now')),
    schema_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(channel_id, message_id)
);
`

const legacyImportLedgerSchema = `
CREATE TABLE legacy_import_ledger (
    source_sha256   BLOB    NOT NULL CHECK(length(source_sha256) = 32),
    raw_message_id  INTEGER NOT NULL,
    channel_id      INTEGER NOT NULL,
    message_id      INTEGER NOT NULL,
    evidence_id     BLOB    NOT NULL CHECK(length(evidence_id) = 16),
    payload_sha256  BLOB    NOT NULL CHECK(length(payload_sha256) = 32),
    PRIMARY KEY(source_sha256, raw_message_id),
    UNIQUE(evidence_id),
    FOREIGN KEY(evidence_id) REFERENCES evidence(id)
) STRICT, WITHOUT ROWID;
`

func fileSHA256(path string) ([32]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(b), nil
}

func openReadOnly(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func openCurrentStorageAuditTarget(t *testing.T, path string) *sql.DB {
	t.Helper()

	store, err := sqlitestore.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open current sqlite storage: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close current sqlite storage: %v", err)
	}

	db := openDB(t, path)
	if _, err := db.Exec(legacyImportLedgerSchema); err != nil {
		_ = db.Close()
		t.Fatalf("create experimental import ledger: %v", err)
	}
	return db
}

func importLegacyRawMessages(source, target *sql.DB, sourceSHA [32]byte) (int, error) {
	rows, err := source.Query(`SELECT id, channel_id, message_id, payload, received_at, schema_version FROM raw_messages ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	imported := 0
	for rows.Next() {
		var rowID, channelID, messageID, schemaVersion int64
		var payloadText, receivedText string
		if err := rows.Scan(&rowID, &channelID, &messageID, &payloadText, &receivedText, &schemaVersion); err != nil {
			return imported, err
		}

		var exists int
		if err := target.QueryRow(`SELECT COUNT(*) FROM legacy_import_ledger WHERE source_sha256 = ? AND raw_message_id = ?`, sourceSHA[:], rowID).Scan(&exists); err != nil {
			return imported, err
		}
		if exists != 0 {
			continue
		}

		receivedAt, err := time.ParseInLocation("2006-01-02 15:04:05", receivedText, time.UTC)
		if err != nil {
			return imported, fmt.Errorf("legacy row %d received_at: %w", rowID, err)
		}
		payload := []byte(payloadText)
		payloadHash := sha256.Sum256(payload)
		id, err := newEvidenceID(rand.Reader)
		if err != nil {
			return imported, err
		}

		tx, err := target.Begin()
		if err != nil {
			return imported, err
		}
		rollback := func(cause error) (int, error) {
			_ = tx.Rollback()
			return imported, cause
		}
		if _, err := tx.Exec(`INSERT INTO evidence(
			id, subscription_id, acquisition, event_kind, source_event_type,
			received_at, payload_format, payload_schema, payload, payload_sha256
		) VALUES (?, ?, 'legacy_import', 'source_snapshot', 'raw_messages', ?, 'telegram-json', ?, ?, ?)`,
			id[:], fmt.Sprintf("telegram:channel:%d", channelID), receivedAt.UnixMilli(), fmt.Sprintf("legacy-v%d", schemaVersion), payload, payloadHash[:]); err != nil {
			return rollback(err)
		}
		if _, err := tx.Exec(`INSERT INTO legacy_import_ledger(
			source_sha256, raw_message_id, channel_id, message_id, evidence_id, payload_sha256
		) VALUES (?, ?, ?, ?, ?, ?)`, sourceSHA[:], rowID, channelID, messageID, id[:], payloadHash[:]); err != nil {
			return rollback(err)
		}
		if err := tx.Commit(); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, rows.Err()
}

func TestLegacyImportIsSideBySideAuditableAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "legacy.db")
	targetPath := filepath.Join(dir, "new.db")

	legacy := openDB(t, legacyPath)
	if _, err := legacy.Exec(legacyRawSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO channels(id, username, title) VALUES (42, 'ofertas', 'Ofertas')`); err != nil {
		t.Fatal(err)
	}
	payloads := [][]byte{
		[]byte(`{"_":"message","id":101,"message":"oferta A"}`),
		{0x7b, 0x22, 0x78, 0x22, 0x3a, 0x22, 0xc3, 0xa7, 0x22, 0x7d},
	}
	for i, payload := range payloads {
		if _, err := legacy.Exec(`INSERT INTO raw_messages(channel_id, message_id, payload, received_at, schema_version) VALUES (42, ?, ?, ?, 1)`,
			101+i, string(payload), fmt.Sprintf("2026-08-19 12:00:0%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	before, err := fileSHA256(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := openReadOnly(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	target := openCurrentStorageAuditTarget(t, targetPath)
	defer target.Close()

	imported, err := importLegacyRawMessages(source, target, before)
	if err != nil {
		t.Fatal(err)
	}
	if imported != len(payloads) {
		t.Fatalf("imported=%d, want %d", imported, len(payloads))
	}
	imported, err = importLegacyRawMessages(source, target, before)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 0 {
		t.Fatalf("retry imported=%d, want 0", imported)
	}

	after, err := fileSHA256(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("legacy source file changed during side-by-side import")
	}

	rows, err := target.Query(`SELECT l.raw_message_id, l.channel_id, l.message_id, e.payload, e.payload_sha256, l.payload_sha256
		FROM legacy_import_ledger l JOIN evidence e ON e.id = l.evidence_id ORDER BY l.raw_message_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	idx := 0
	for rows.Next() {
		var rowID, channelID, messageID int64
		var payload, evidenceHash, ledgerHash []byte
		if err := rows.Scan(&rowID, &channelID, &messageID, &payload, &evidenceHash, &ledgerHash); err != nil {
			t.Fatal(err)
		}
		if channelID != 42 || messageID != int64(101+idx) {
			t.Fatalf("ledger origin mismatch row=%d channel=%d message=%d", rowID, channelID, messageID)
		}
		wantHash := sha256.Sum256(payloads[idx])
		if !bytes.Equal(payload, payloads[idx]) || !bytes.Equal(evidenceHash, wantHash[:]) || !bytes.Equal(ledgerHash, wantHash[:]) {
			t.Fatalf("payload/hash reconciliation failed for legacy row %d", rowID)
		}
		idx++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if idx != len(payloads) {
		t.Fatalf("reconciled=%d, want %d", idx, len(payloads))
	}

	var integrity string
	if err := target.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q, want ok", integrity)
	}
}
