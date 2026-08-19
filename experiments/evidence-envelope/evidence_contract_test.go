package evidenceenvelope

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "turso.tech/database/tursogo"
)

type engine struct {
	name   string
	driver string
	dsn    func(string) string
}

var engines = []engine{
	{name: "ncruces", driver: "sqlite3", dsn: func(path string) string { return "file:" + path }},
	{name: "tursogo", driver: "turso", dsn: func(path string) string { return path }},
}

const evidenceSchema = `
CREATE TABLE evidence (
    seq                   INTEGER PRIMARY KEY,
    evidence_id           BLOB NOT NULL UNIQUE CHECK (length(evidence_id) = 16),
    subscription_id       TEXT NOT NULL CHECK (subscription_id <> ''),
    acquisition           TEXT NOT NULL CHECK (acquisition <> ''),
    event_kind            TEXT NOT NULL CHECK (event_kind <> ''),
    source_event_type     TEXT NOT NULL CHECK (source_event_type <> ''),
    source_occurred_at_ms INTEGER,
    received_at_ms        INTEGER NOT NULL,
    payload_format        TEXT NOT NULL CHECK (payload_format <> ''),
    payload_schema        TEXT NOT NULL CHECK (payload_schema <> ''),
    payload               BLOB NOT NULL,
    payload_sha256        BLOB NOT NULL CHECK (length(payload_sha256) = 32)
);

CREATE INDEX idx_evidence_subscription_seq
    ON evidence(subscription_id, seq);
CREATE INDEX idx_evidence_received_seq
    ON evidence(received_at_ms, seq);

CREATE TABLE evidence_legacy_import (
    evidence_id              BLOB PRIMARY KEY REFERENCES evidence(evidence_id),
    legacy_raw_id            INTEGER NOT NULL,
    legacy_channel_id        INTEGER NOT NULL,
    legacy_message_id        INTEGER NOT NULL,
    legacy_schema_version    INTEGER NOT NULL,
    legacy_received_at_text  TEXT NOT NULL
);

CREATE TRIGGER evidence_no_update
BEFORE UPDATE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;

CREATE TRIGGER evidence_no_delete
BEFORE DELETE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;

CREATE TRIGGER evidence_legacy_import_no_update
BEFORE UPDATE ON evidence_legacy_import
BEGIN
    SELECT RAISE(ABORT, 'legacy provenance is append-only');
END;

CREATE TRIGGER evidence_legacy_import_no_delete
BEFORE DELETE ON evidence_legacy_import
BEGIN
    SELECT RAISE(ABORT, 'legacy provenance is append-only');
END;
`

type record struct {
	ID                 []byte
	SubscriptionID     string
	Acquisition        string
	EventKind          string
	SourceEventType    string
	SourceOccurredAtMS *int64
	ReceivedAtMS       int64
	PayloadFormat      string
	PayloadSchema      string
	Payload            []byte
}

func TestEvidencePhysicalContract(t *testing.T) {
	for _, eng := range engines {
		eng := eng
		t.Run(eng.name, func(t *testing.T) {
			db := openDatabase(t, eng, filepath.Join(t.TempDir(), "evidence.db"))
			defer db.Close()

			t.Run("same payload is replayable and hash is not identity", func(t *testing.T) {
				payload := []byte(`{"Updates":[{"kind":"edit"},{"kind":"delete"}],"Date":123,"Seq":9}`)
				first := baseRecord(fixedID(1), payload)
				second := baseRecord(fixedID(2), payload)
				seq1 := appendEvidence(t, db, first)
				seq2 := appendEvidence(t, db, second)
				if seq2 <= seq1 {
					t.Fatalf("local seq did not advance: first=%d second=%d", seq1, seq2)
				}

				var count, distinctHashes int
				if err := db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT hex(payload_sha256)) FROM evidence WHERE payload_sha256 = (SELECT payload_sha256 FROM evidence WHERE evidence_id = ?)`, first.ID).Scan(&count, &distinctHashes); err != nil {
					t.Fatal(err)
				}
				if count != 2 || distinctHashes != 1 {
					t.Fatalf("same payload did not coexist: count=%d distinct_hashes=%d", count, distinctHashes)
				}
			})

			t.Run("payload is byte exact and composite observation has no message identity column", func(t *testing.T) {
				payload := []byte("{\n  \"Updates\": [1, 2, 3], \"Date\": 123, \"Seq\": 4\n}\n")
				rec := baseRecord(fixedID(3), payload)
				appendEvidence(t, db, rec)

				var got []byte
				if err := db.QueryRow(`SELECT payload FROM evidence WHERE evidence_id = ?`, rec.ID).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, payload) {
					t.Fatalf("payload changed: got=%q want=%q", got, payload)
				}

				columns := tableColumns(t, db, "evidence")
				for _, forbidden := range []string{"channel_id", "message_id", "source_message_id"} {
					if columns[forbidden] {
						t.Fatalf("physical Evidence unexpectedly contains singular message identity column %q", forbidden)
					}
				}
			})

			t.Run("append only is physically enforced", func(t *testing.T) {
				rec := baseRecord(fixedID(4), []byte(`{"kind":"live"}`))
				appendEvidence(t, db, rec)
				if _, err := db.Exec(`UPDATE evidence SET event_kind='mutated' WHERE evidence_id=?`, rec.ID); err == nil {
					t.Fatal("UPDATE unexpectedly succeeded")
				}
				if _, err := db.Exec(`DELETE FROM evidence WHERE evidence_id=?`, rec.ID); err == nil {
					t.Fatal("DELETE unexpectedly succeeded")
				}
			})

			t.Run("format and schema are independent discriminants", func(t *testing.T) {
				payload := []byte(`{"same":"bytes"}`)
				live := baseRecord(fixedID(5), payload)
				live.PayloadFormat = "application/json"
				live.PayloadSchema = "telegram.gotd.updates-class/v1"
				history := baseRecord(fixedID(6), payload)
				history.Acquisition = "history_snapshot"
				history.EventKind = "snapshot"
				history.PayloadFormat = "application/json"
				history.PayloadSchema = "telegram.gotd.message/v1"
				appendEvidence(t, db, live)
				appendEvidence(t, db, history)

				var schemas int
				if err := db.QueryRow(`SELECT COUNT(DISTINCT payload_schema) FROM evidence WHERE payload = ?`, payload).Scan(&schemas); err != nil {
					t.Fatal(err)
				}
				if schemas != 2 {
					t.Fatalf("payload schema collapsed: got %d schemas", schemas)
				}
			})

			t.Run("unknown schema stays readable but decode fails closed", func(t *testing.T) {
				rec := baseRecord(fixedID(7), []byte(`{"future":true}`))
				rec.PayloadSchema = "telegram.future-envelope/v99"
				appendEvidence(t, db, rec)
				stored := readEvidence(t, db, rec.ID)
				if !bytes.Equal(stored.Payload, rec.Payload) {
					t.Fatal("unknown schema payload was not preserved")
				}
				if _, err := decode(stored); !errors.Is(err, errUnsupportedSchema) {
					t.Fatalf("decode error = %v, want unsupported schema", err)
				}
			})

			t.Run("timestamps are unambiguous and source time is optional", func(t *testing.T) {
				sourceMS := int64(1_787_100_000_000)
				rec := baseRecord(fixedID(8), []byte(`{"kind":"timestamp"}`))
				rec.SourceOccurredAtMS = &sourceMS
				rec.ReceivedAtMS = 1_787_100_123_456
				appendEvidence(t, db, rec)
				got := readEvidence(t, db, rec.ID)
				if got.SourceOccurredAtMS == nil || *got.SourceOccurredAtMS != sourceMS || got.ReceivedAtMS != rec.ReceivedAtMS {
					t.Fatalf("timestamp roundtrip mismatch: got=%+v", got)
				}

				withoutSource := baseRecord(fixedID(9), []byte(`{"kind":"no-source-time"}`))
				appendEvidence(t, db, withoutSource)
				if got := readEvidence(t, db, withoutSource.ID); got.SourceOccurredAtMS != nil {
					t.Fatalf("source time should remain NULL, got=%d", *got.SourceOccurredAtMS)
				}
			})

			t.Run("legacy import preserves payload bytes and legacy provenance", func(t *testing.T) {
				legacyPayload := []byte("{ \"Message\": \"payload original\", \"ID\": 99 }\n")
				rec := baseRecord(fixedID(10), legacyPayload)
				rec.Acquisition = "legacy_import"
				rec.EventKind = "legacy_record"
				rec.SourceEventType = "limiar.raw_messages"
				rec.PayloadSchema = "limiar.legacy.raw-message-payload/v1"
				appendEvidence(t, db, rec)
				if _, err := db.Exec(`INSERT INTO evidence_legacy_import(evidence_id, legacy_raw_id, legacy_channel_id, legacy_message_id, legacy_schema_version, legacy_received_at_text) VALUES (?, ?, ?, ?, ?, ?)`, rec.ID, 321, 42, 99, 1, "2026-06-28 10:11:12"); err != nil {
					t.Fatal(err)
				}

				got := readEvidence(t, db, rec.ID)
				if !bytes.Equal(got.Payload, legacyPayload) {
					t.Fatal("legacy payload bytes changed")
				}
				var rawID, channelID, messageID int64
				var version int
				var received string
				if err := db.QueryRow(`SELECT legacy_raw_id, legacy_channel_id, legacy_message_id, legacy_schema_version, legacy_received_at_text FROM evidence_legacy_import WHERE evidence_id=?`, rec.ID).Scan(&rawID, &channelID, &messageID, &version, &received); err != nil {
					t.Fatal(err)
				}
				if rawID != 321 || channelID != 42 || messageID != 99 || version != 1 || received != "2026-06-28 10:11:12" {
					t.Fatalf("legacy provenance mismatch: %d %d %d %d %q", rawID, channelID, messageID, version, received)
				}
			})
	}
}

func TestStableEvidenceIdentityIsIndependentFromLocalSequence(t *testing.T) {
	for _, eng := range engines {
		eng := eng
		t.Run(eng.name, func(t *testing.T) {
			source := openDatabase(t, eng, filepath.Join(t.TempDir(), "source.db"))
			defer source.Close()
			target := openDatabase(t, eng, filepath.Join(t.TempDir(), "target.db"))
			defer target.Close()

			rec := baseRecord(fixedID(20), []byte(`{"portable":true}`))
			sourceSeq := appendEvidence(t, source, rec)
			appendEvidence(t, target, baseRecord(fixedID(21), []byte(`{"prefix":true}`)))
			targetSeq := appendEvidence(t, target, rec)
			if sourceSeq == targetSeq {
				t.Fatalf("test did not create different local sequences: source=%d target=%d", sourceSeq, targetSeq)
			}
			got := readEvidence(t, target, rec.ID)
			if !bytes.Equal(got.ID, rec.ID) || !bytes.Equal(got.Payload, rec.Payload) {
				t.Fatal("stable Evidence identity or payload did not survive re-insertion into another database")
			}
		})
}

func TestKnownDecoderUsesExplicitSchemaInsteadOfPayloadGuessing(t *testing.T) {
	payload := []byte(`{"shape":"ambiguous"}`)
	live := baseRecord(fixedID(30), payload)
	live.PayloadSchema = "telegram.gotd.updates-class/v1"
	history := baseRecord(fixedID(31), payload)
	history.PayloadSchema = "telegram.gotd.message/v1"

	gotLive, err := decode(live)
	if err != nil {
		t.Fatal(err)
	}
	gotHistory, err := decode(history)
	if err != nil {
		t.Fatal(err)
	}
	if gotLive == gotHistory {
		t.Fatalf("explicit schemas collapsed to same decoder output: %q", gotLive)
	}
}

func openDatabase(t *testing.T, eng engine, path string) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open(eng.driver, eng.dsn(path))
	if err != nil {
		t.Fatalf("%s open: %v", eng.name, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("%s ping: %v", eng.name, err)
	}
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=FULL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			t.Fatalf("%s %s: %v", eng.name, pragma, err)
		}
	}
	for _, statement := range splitStatements(evidenceSchema) {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			t.Fatalf("%s schema statement %q: %v", eng.name, statement, err)
		}
	}
	return db
}

func splitStatements(schema string) []string {
	// O harness mantém cada statement separado por uma linha em branco. Triggers contêm
	// semicolons internos, portanto um split ingênuo por ';' seria incorreto.
	blocks := bytes.Split([]byte(schema), []byte("\n\n"))
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		trimmed := bytes.TrimSpace(block)
		if len(trimmed) > 0 {
			out = append(out, string(trimmed))
		}
	}
	return out
}

func baseRecord(id, payload []byte) record {
	return record{
		ID:              id,
		SubscriptionID:  "telegram:channel:42",
		Acquisition:     "live",
		EventKind:       "source_observation",
		SourceEventType: "tg.Updates",
		ReceivedAtMS:    1_787_100_123_456,
		PayloadFormat:   "application/json",
		PayloadSchema:   "telegram.gotd.updates-class/v1",
		Payload:         payload,
	}
}

func appendEvidence(t *testing.T, db *sql.DB, rec record) int64 {
	t.Helper()
	hash := sha256.Sum256(rec.Payload)
	var source any
	if rec.SourceOccurredAtMS != nil {
		source = *rec.SourceOccurredAtMS
	}
	if _, err := db.Exec(`INSERT INTO evidence(evidence_id, subscription_id, acquisition, event_kind, source_event_type, source_occurred_at_ms, received_at_ms, payload_format, payload_schema, payload, payload_sha256) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rec.ID, rec.SubscriptionID, rec.Acquisition, rec.EventKind, rec.SourceEventType, source, rec.ReceivedAtMS, rec.PayloadFormat, rec.PayloadSchema, rec.Payload, hash[:]); err != nil {
		t.Fatal(err)
	}
	var seq int64
	if err := db.QueryRow(`SELECT seq FROM evidence WHERE evidence_id=?`, rec.ID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	return seq
}

func readEvidence(t *testing.T, db *sql.DB, id []byte) record {
	t.Helper()
	var rec record
	var source sql.NullInt64
	var hash []byte
	if err := db.QueryRow(`SELECT evidence_id, subscription_id, acquisition, event_kind, source_event_type, source_occurred_at_ms, received_at_ms, payload_format, payload_schema, payload, payload_sha256 FROM evidence WHERE evidence_id=?`, id).Scan(&rec.ID, &rec.SubscriptionID, &rec.Acquisition, &rec.EventKind, &rec.SourceEventType, &source, &rec.ReceivedAtMS, &rec.PayloadFormat, &rec.PayloadSchema, &rec.Payload, &hash); err != nil {
		t.Fatal(err)
	}
	if source.Valid {
		v := source.Int64
		rec.SourceOccurredAtMS = &v
	}
	want := sha256.Sum256(rec.Payload)
	if !bytes.Equal(hash, want[:]) {
		t.Fatal("persisted payload hash does not match payload")
	}
	return rec
}

func tableColumns(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

func fixedID(seed byte) []byte {
	id := make([]byte, 16)
	for i := range id {
		id[i] = seed + byte(i)
	}
	return id
}

var errUnsupportedSchema = errors.New("unsupported payload schema")

func decode(rec record) (string, error) {
	if rec.PayloadFormat != "application/json" {
		return "", fmt.Errorf("%w: format %s", errUnsupportedSchema, rec.PayloadFormat)
	}
	if !json.Valid(rec.Payload) {
		return "", errors.New("invalid json payload")
	}
	switch rec.PayloadSchema {
	case "telegram.gotd.updates-class/v1":
		return "updates-container", nil
	case "telegram.gotd.message/v1":
		return "history-message-snapshot", nil
	case "limiar.legacy.raw-message-payload/v1":
		return "legacy-raw-payload", nil
	default:
		return "", fmt.Errorf("%w: %s", errUnsupportedSchema, rec.PayloadSchema)
	}
}
