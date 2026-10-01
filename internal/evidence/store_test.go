package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) {
	return 0, errors.New("entropia indisponível")
}

type evidenceRow struct {
	id               []byte
	subscription     string
	acquisition      string
	eventKind        string
	sourceType       string
	sourceOccurredAt sql.NullInt64
	receivedAt       int64
	payloadFormat    string
	payloadSchema    string
	payload          []byte
	hash             []byte
}

func openTestStore(t *testing.T) (string, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "limiar-new.db")
	return path, mustOpenStoreAt(t, path)
}

func mustOpenStoreAt(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func closeTestStore(t *testing.T, store *Store) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func mustQueryInt64(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var got int64
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return got
}

func mustQueryString(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var got string
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return got
}

func requireEqual[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s=%v, esperado %v", name, got, want)
	}
}

func requirePOSIXPermissions(t *testing.T, name, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	requireEqual(t, name, info.Mode().Perm(), want)
}

func requireBytesEqual(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("%s mudou: got=%x want=%x", name, got, want)
	}
}

func requireContains(t *testing.T, name, got, want string) {
	t.Helper()
	if !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("%s não contém %q: %s", name, want, got)
	}
}

func requireExecRejected(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err == nil {
		t.Fatalf("comando deveria ser rejeitado: %s", query)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path) // #nosec G304 -- paths dos testes pertencem ao TempDir.
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func mustOpenSQL(t *testing.T, path string, readOnly bool) *sql.DB {
	t.Helper()
	db, err := sql.Open(driverName, databaseURI(path, readOnly))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func appendEvidence(t *testing.T, store *Store, evidence Evidence) EvidenceID {
	t.Helper()
	id, err := store.EvidenceAppender().Append(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func readEvidenceRow(t *testing.T, db *sql.DB, id []byte) evidenceRow {
	t.Helper()
	var row evidenceRow
	if err := db.QueryRow(`SELECT id, subscription_id, acquisition, event_kind, source_event_type,
		source_occurred_at, received_at, payload_format, payload_schema, payload, payload_sha256
		FROM evidence WHERE id = ?`, id).Scan(
		&row.id, &row.subscription, &row.acquisition, &row.eventKind, &row.sourceType,
		&row.sourceOccurredAt, &row.receivedAt, &row.payloadFormat, &row.payloadSchema,
		&row.payload, &row.hash,
	); err != nil {
		t.Fatal(err)
	}
	return row
}

func insertEvidenceRow(t *testing.T, db *sql.DB, row evidenceRow) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		source_occurred_at, received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.id, row.subscription, row.acquisition, row.eventKind, row.sourceType,
		row.sourceOccurredAt, row.receivedAt, row.payloadFormat, row.payloadSchema,
		row.payload, row.hash)
	if err != nil {
		t.Fatal(err)
	}
}

func createLegacyDatabase(t *testing.T, path string) {
	t.Helper()
	legacy := mustOpenSQL(t, path, false)
	if _, err := legacy.Exec(`CREATE TABLE legacy_marker(value TEXT NOT NULL); INSERT INTO legacy_marker(value) VALUES ('preserve-me')`); err != nil {
		_ = legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sampleEvidence(payload []byte) Evidence {
	sourceOccurredAt := int64(1_787_103_000_000)
	return Evidence{
		SubscriptionID:   "telegram:channel:42",
		Acquisition:      "live_update",
		EventKind:        "source_update",
		SourceEventType:  "updates",
		SourceOccurredAt: &sourceOccurredAt,
		ReceivedAt:       1_787_103_600_123,
		PayloadFormat:    "telegram-tl",
		PayloadSchema:    "v1",
		Payload:          payload,
	}
}

func TestOpenAppliesADR019BaselineAndMigration(t *testing.T) {
	path, store := openTestStore(t)
	defer closeTestStore(t, store)

	requirePOSIXPermissions(t, "database permissions", path, 0o600)
	requireEqual(t, "MaxOpenConnections", store.db.Stats().MaxOpenConnections, 1)
	requireEqual(t, "journal_mode", mustQueryString(t, store.db, `PRAGMA journal_mode`), "wal")
	requireEqual(t, "synchronous", mustQueryInt64(t, store.db, `PRAGMA synchronous`), int64(2))
	requireEqual(t, "foreign_keys", mustQueryInt64(t, store.db, `PRAGMA foreign_keys`), int64(1))
	requireEqual(t, "busy_timeout", mustQueryInt64(t, store.db, `PRAGMA busy_timeout`), int64(5000))
	requireEqual(t, "user_version", mustQueryInt64(t, store.db, `PRAGMA user_version`), int64(1))
	requireEqual(t, "application_id", mustQueryInt64(t, store.db, `PRAGMA application_id`), int64(applicationID))
	requireContains(t, "evidence schema", mustQueryString(t, store.db,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='evidence'`), "STRICT")
}

func TestOpenRejectsUnownedExistingSQLiteWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyDatabase(t, path)
	before := mustReadFile(t, path)

	store, err := Open(context.Background(), path)
	if err == nil {
		_ = store.Close()
		t.Fatal("Open deveria recusar SQLite existente sem application_id do novo storage")
	}

	requireBytesEqual(t, "arquivo SQLite recusado", mustReadFile(t, path), before)
	requirePOSIXPermissions(t, "permissões do SQLite recusado", path, 0o644)

	readOnly := mustOpenSQL(t, path, true)
	defer func() { _ = readOnly.Close() }()
	requireEqual(t, "legacy marker", mustQueryString(t, readOnly, `SELECT value FROM legacy_marker`), "preserve-me")
	requireEqual(t, "evidence table count", mustQueryInt64(t, readOnly,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='evidence'`), int64(0))
}

func TestEvidenceAppendRoundTripAndHashAuthority(t *testing.T) {
	_, store := openTestStore(t)
	defer closeTestStore(t, store)

	payload := []byte{0x00, 0xff, 0x7b, 0x22, 0x78, 0x22, 0x3a, 0x31, 0x7d}
	wantPayload := append([]byte(nil), payload...)
	evidence := sampleEvidence(payload)
	id := appendEvidence(t, store, evidence)

	requireEqual(t, "UUID version", id[6]>>4, byte(4))
	requireEqual(t, "UUID variant", id[8]>>6, byte(2))

	// Alterar o buffer do chamador após o retorno não altera a Evidence persistida.
	payload[0] = 0x99
	row := readEvidenceRow(t, store.db, id[:])
	wantHash := sha256.Sum256(wantPayload)

	requireBytesEqual(t, "id round-trip", row.id, id[:])
	requireBytesEqual(t, "payload round-trip", row.payload, wantPayload)
	requireBytesEqual(t, "payload_sha256", row.hash, wantHash[:])
	requireEqual(t, "source_occurred_at", row.sourceOccurredAt.Int64, *evidence.SourceOccurredAt)
	requireEqual(t, "received_at", row.receivedAt, evidence.ReceivedAt)
}

func TestEvidenceEntropyFailureIsFailClosed(t *testing.T) {
	_, store := openTestStore(t)
	defer closeTestStore(t, store)

	appender := evidenceAppender{db: store.db, entropy: failingEntropy{}}
	id, err := appender.Append(context.Background(), sampleEvidence([]byte("payload")))
	if err == nil {
		t.Fatal("Append deveria falhar quando a entropia falha")
	}
	requireEqual(t, "id em falha", id, EvidenceID{})
	requireEqual(t, "evidence rows após falha de entropia",
		mustQueryInt64(t, store.db, `SELECT COUNT(*) FROM evidence`), int64(0))
}

func TestRepeatedEvidenceCoexists(t *testing.T) {
	_, store := openTestStore(t)
	defer closeTestStore(t, store)

	evidence := sampleEvidence([]byte("same-payload"))
	first := appendEvidence(t, store, evidence)
	second := appendEvidence(t, store, evidence)
	if first == second {
		t.Fatalf("replays receberam o mesmo id: %x", first)
	}

	var count, distinctHashes, distinctTimes int64
	if err := store.db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT payload_sha256), COUNT(DISTINCT received_at) FROM evidence`).Scan(
		&count, &distinctHashes, &distinctTimes,
	); err != nil {
		t.Fatal(err)
	}
	requireEqual(t, "rows", count, int64(2))
	requireEqual(t, "distinct hashes", distinctHashes, int64(1))
	requireEqual(t, "distinct times", distinctTimes, int64(1))
}

func TestEvidencePhysicalConstraintsRejectInvalidShape(t *testing.T) {
	_, store := openTestStore(t)
	defer closeTestStore(t, store)

	insert := `INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, 's', 'live', 'update', 'updates', 1, 'telegram-tl', 'v1', x'01', ?)`
	requireExecRejected(t, store.db, insert, make([]byte, 15), make([]byte, 32))
	requireExecRejected(t, store.db, insert, make([]byte, 16), make([]byte, 31))
}

func TestEvidencePhysicalExportImportPreservesIdentityAndHash(t *testing.T) {
	_, source := openTestStore(t)
	defer closeTestStore(t, source)
	_, target := openTestStore(t)
	defer closeTestStore(t, target)

	id := appendEvidence(t, source, sampleEvidence([]byte("export-import")))
	exported := readEvidenceRow(t, source.db, id[:])
	insertEvidenceRow(t, target.db, exported)
	imported := readEvidenceRow(t, target.db, id[:])
	wantHash := sha256.Sum256(imported.payload)

	requireBytesEqual(t, "imported id", imported.id, id[:])
	requireBytesEqual(t, "imported payload", imported.payload, exported.payload)
	requireBytesEqual(t, "imported hash", imported.hash, exported.hash)
	requireBytesEqual(t, "recomputed imported hash", imported.hash, wantHash[:])
}

func TestEvidenceAppendOnlyGuardsSurviveReopen(t *testing.T) {
	path, store := openTestStore(t)
	id := appendEvidence(t, store, sampleEvidence([]byte("durable")))

	requireExecRejected(t, store.db, `UPDATE evidence SET payload = x'00' WHERE id = ?`, id[:])
	requireExecRejected(t, store.db, `DELETE FROM evidence WHERE id = ?`, id[:])
	closeTestStore(t, store)

	reopened := mustOpenStoreAt(t, path)
	defer closeTestStore(t, reopened)
	requireExecRejected(t, reopened.db, `UPDATE evidence SET received_at = 1 WHERE id = ?`, id[:])
	requireExecRejected(t, reopened.db, `DELETE FROM evidence WHERE id = ?`, id[:])
	requireEqual(t, "evidence rows após reopen", mustQueryInt64(t, reopened.db,
		`SELECT COUNT(*) FROM evidence`), int64(1))
	requireEqual(t, "integrity_check", mustQueryString(t, reopened.db, `PRAGMA integrity_check`), "ok")
	requireEqual(t, "user_version após reopen", mustQueryInt64(t, reopened.db, `PRAGMA user_version`), int64(1))
}

func TestMigrationVersion(t *testing.T) {
	tests := []struct {
		name    string
		want    int
		wantErr bool
	}{
		{name: "001_evidence.sql", want: 1},
		{name: "010_projection.sql", want: 10},
		{name: "evidence.sql", wantErr: true},
		{name: "000_invalid.sql", wantErr: true},
		{name: "1.sql", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMigrationVersion(t, tt.name, tt.want, tt.wantErr)
		})
	}
}

func assertMigrationVersion(t *testing.T, name string, want int, wantErr bool) {
	t.Helper()
	got, err := migrationVersion(name)
	if wantErr {
		if err == nil {
			t.Fatalf("migrationVersion(%q)=%d, want error", name, got)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	requireEqual(t, "migration version", got, want)
}
