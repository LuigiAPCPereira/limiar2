package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func openTestStore(t *testing.T) (string, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "limiar-new.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return path, store
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
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("database permissions=%#o, want 0600", got)
	}

	if got := store.db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections=%d, want 1", got)
	}

	var journal string
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode=%q, want wal", journal)
	}

	var synchronous, foreignKeys, busyTimeout, userVersion int
	for query, target := range map[string]*int{
		`PRAGMA synchronous`:  &synchronous,
		`PRAGMA foreign_keys`: &foreignKeys,
		`PRAGMA busy_timeout`: &busyTimeout,
		`PRAGMA user_version`: &userVersion,
	} {
		if err := store.db.QueryRow(query).Scan(target); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	if synchronous != 2 {
		t.Fatalf("synchronous=%d, want FULL(2)", synchronous)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d, want 1", foreignKeys)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout=%d, want 5000", busyTimeout)
	}
	if userVersion != 1 {
		t.Fatalf("user_version=%d, want 1", userVersion)
	}

	var gotApplicationID int64
	if err := store.db.QueryRow(`PRAGMA application_id`).Scan(&gotApplicationID); err != nil {
		t.Fatal(err)
	}
	if gotApplicationID != applicationID {
		t.Fatalf("application_id=%d, want %d", gotApplicationID, applicationID)
	}

	var tableSQL string
	if err := store.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='evidence'`).Scan(&tableSQL); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(tableSQL), []byte("STRICT")) {
		t.Fatalf("evidence schema não contém STRICT: %s", tableSQL)
	}
}

func TestOpenRejectsUnownedExistingSQLiteWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open(driverName, databaseURI(path, false))
	if err != nil {
		t.Fatal(err)
	}
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

	before, err := os.ReadFile(path) // #nosec G304 -- path pertence ao TempDir do teste.
	if err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if err == nil {
		_ = store.Close()
		t.Fatal("Open deveria recusar SQLite existente sem application_id do novo storage")
	}

	after, err := os.ReadFile(path) // #nosec G304 -- path pertence ao TempDir do teste.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("arquivo SQLite recusado foi modificado")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("permissões do SQLite recusado=%#o, want 0644", got)
	}

	readOnly, err := sql.Open(driverName, databaseURI(path, true))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = readOnly.Close() }()

	var marker string
	if err := readOnly.QueryRow(`SELECT value FROM legacy_marker`).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if marker != "preserve-me" {
		t.Fatalf("legacy marker=%q, want preserve-me", marker)
	}
	var evidenceTables int
	if err := readOnly.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='evidence'`).Scan(&evidenceTables); err != nil {
		t.Fatal(err)
	}
	if evidenceTables != 0 {
		t.Fatalf("evidence table criada no SQLite recusado: count=%d", evidenceTables)
	}
}

func TestEvidenceAppendRoundTripAndHashAuthority(t *testing.T) {
	_, store := openTestStore(t)
	defer func() { _ = store.Close() }()

	payload := []byte{0x00, 0xff, 0x7b, 0x22, 0x78, 0x22, 0x3a, 0x31, 0x7d}
	wantPayload := append([]byte(nil), payload...)
	evidence := sampleEvidence(payload)

	id, err := store.EvidenceAppender().Append(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if id[6]>>4 != 4 {
		t.Fatalf("UUID version=%x, want 4", id[6]>>4)
	}
	if id[8]>>6 != 2 {
		t.Fatalf("UUID variant=%b, want 10", id[8]>>6)
	}

	// Alterar o buffer do chamador após o retorno não altera a Evidence persistida.
	payload[0] = 0x99

	var gotID, gotPayload, gotHash []byte
	var gotSourceOccurredAt, gotReceivedAt int64
	if err := store.db.QueryRow(`SELECT id, source_occurred_at, received_at, payload, payload_sha256
		FROM evidence WHERE id = ?`, id[:]).Scan(
		&gotID, &gotSourceOccurredAt, &gotReceivedAt, &gotPayload, &gotHash,
	); err != nil {
		t.Fatal(err)
	}

	wantHash := sha256.Sum256(wantPayload)
	if !bytes.Equal(gotID, id[:]) {
		t.Fatalf("id round-trip mudou: got=%x want=%x", gotID, id)
	}
	if !bytes.Equal(gotPayload, wantPayload) {
		t.Fatalf("payload round-trip mudou: got=%x want=%x", gotPayload, wantPayload)
	}
	if !bytes.Equal(gotHash, wantHash[:]) {
		t.Fatalf("payload_sha256=%x, want=%x", gotHash, wantHash)
	}
	if gotSourceOccurredAt != *evidence.SourceOccurredAt || gotReceivedAt != evidence.ReceivedAt {
		t.Fatalf("timestamps source=%d received=%d", gotSourceOccurredAt, gotReceivedAt)
	}
}

func TestEvidenceEntropyFailureIsFailClosed(t *testing.T) {
	_, store := openTestStore(t)
	defer func() { _ = store.Close() }()

	appender := evidenceAppender{db: store.db, entropy: failingEntropy{}}
	id, err := appender.Append(context.Background(), sampleEvidence([]byte("payload")))
	if err == nil {
		t.Fatal("Append deveria falhar quando a entropia falha")
	}
	if id != (EvidenceID{}) {
		t.Fatalf("id em falha=%x, want zero value", id)
	}

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("evidence rows=%d após falha de entropia, want 0", count)
	}
}

func TestRepeatedEvidenceCoexists(t *testing.T) {
	_, store := openTestStore(t)
	defer func() { _ = store.Close() }()

	evidence := sampleEvidence([]byte("same-payload"))
	first, err := store.EvidenceAppender().Append(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.EvidenceAppender().Append(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("replays receberam o mesmo id: %x", first)
	}

	var count, distinctHashes, distinctTimes int
	if err := store.db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT payload_sha256), COUNT(DISTINCT received_at) FROM evidence`).Scan(
		&count, &distinctHashes, &distinctTimes,
	); err != nil {
		t.Fatal(err)
	}
	if count != 2 || distinctHashes != 1 || distinctTimes != 1 {
		t.Fatalf("rows=%d hashes=%d times=%d, want 2/1/1", count, distinctHashes, distinctTimes)
	}
}

func TestEvidencePhysicalConstraintsRejectInvalidShape(t *testing.T) {
	_, store := openTestStore(t)
	defer func() { _ = store.Close() }()

	insert := `INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, 's', 'live', 'update', 'updates', 1, 'telegram-tl', 'v1', x'01', ?)`

	if _, err := store.db.Exec(insert, make([]byte, 15), make([]byte, 32)); err == nil {
		t.Fatal("schema deveria recusar EvidenceID de 15 bytes")
	}
	if _, err := store.db.Exec(insert, make([]byte, 16), make([]byte, 31)); err == nil {
		t.Fatal("schema deveria recusar payload_sha256 de 31 bytes")
	}
}

func TestEvidencePhysicalExportImportPreservesIdentityAndHash(t *testing.T) {
	_, source := openTestStore(t)
	defer func() { _ = source.Close() }()
	_, target := openTestStore(t)
	defer func() { _ = target.Close() }()

	id, err := source.EvidenceAppender().Append(context.Background(), sampleEvidence([]byte("export-import")))
	if err != nil {
		t.Fatal(err)
	}

	var (
		gotID, payload, hash                                      []byte
		subscription, acquisition, eventKind, sourceType          string
		payloadFormat, payloadSchema                              string
		sourceOccurredAt                                          sql.NullInt64
		receivedAt                                                int64
	)
	if err := source.db.QueryRow(`SELECT id, subscription_id, acquisition, event_kind, source_event_type,
		source_occurred_at, received_at, payload_format, payload_schema, payload, payload_sha256
		FROM evidence WHERE id = ?`, id[:]).Scan(
		&gotID, &subscription, &acquisition, &eventKind, &sourceType,
		&sourceOccurredAt, &receivedAt, &payloadFormat, &payloadSchema, &payload, &hash,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := target.db.Exec(`INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		source_occurred_at, received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		gotID, subscription, acquisition, eventKind, sourceType,
		sourceOccurredAt, receivedAt, payloadFormat, payloadSchema, payload, hash,
	); err != nil {
		t.Fatal(err)
	}

	var importedID, importedPayload, importedHash []byte
	if err := target.db.QueryRow(`SELECT id, payload, payload_sha256 FROM evidence WHERE id = ?`, id[:]).Scan(
		&importedID, &importedPayload, &importedHash,
	); err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(importedPayload)
	if !bytes.Equal(importedID, id[:]) || !bytes.Equal(importedPayload, payload) || !bytes.Equal(importedHash, hash) {
		t.Fatal("export/import não preservou identidade, payload ou hash")
	}
	if !bytes.Equal(importedHash, wantHash[:]) {
		t.Fatal("hash importado não corresponde aos bytes importados")
	}
}

func TestEvidenceAppendOnlyGuardsSurviveReopen(t *testing.T) {
	path, store := openTestStore(t)
	id, err := store.EvidenceAppender().Append(context.Background(), sampleEvidence([]byte("durable")))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`UPDATE evidence SET payload = x'00' WHERE id = ?`, id[:]); err == nil {
		t.Fatal("UPDATE de Evidence deveria falhar")
	}
	if _, err := store.db.Exec(`DELETE FROM evidence WHERE id = ?`, id[:]); err == nil {
		t.Fatal("DELETE de Evidence deveria falhar")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	if _, err := reopened.db.Exec(`UPDATE evidence SET received_at = 1 WHERE id = ?`, id[:]); err == nil {
		t.Fatal("UPDATE após reopen deveria continuar bloqueado")
	}
	if _, err := reopened.db.Exec(`DELETE FROM evidence WHERE id = ?`, id[:]); err == nil {
		t.Fatal("DELETE após reopen deveria continuar bloqueado")
	}

	var count int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("evidence rows=%d após reopen, want 1", count)
	}

	var integrity string
	if err := reopened.db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q, want ok", integrity)
	}

	var userVersion int
	if err := reopened.db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	if userVersion != 1 {
		t.Fatalf("user_version=%d após reopen, want 1", userVersion)
	}
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
			got, err := migrationVersion(tt.name)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("migrationVersion(%q)=%d, want error", tt.name, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("migrationVersion(%q)=%d, want %d", tt.name, got, tt.want)
			}
		})
	}
}
