package evidenceenvelope

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"testing"
)

const evidenceAppendOnlyGuards = `
CREATE TRIGGER evidence_reject_update
BEFORE UPDATE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;
CREATE TRIGGER evidence_reject_delete
BEFORE DELETE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;
`

type evidenceAppender struct {
	db *sql.DB
}

func (a evidenceAppender) Append(payload []byte, receivedAt int64) (evidenceID, error) {
	id, err := newEvidenceID(rand.Reader)
	if err != nil {
		return evidenceID{}, err
	}
	hash := sha256.Sum256(payload)
	_, err = a.db.Exec(`INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, 'telegram:42', 'live_update', 'source_update', 'updates', ?, 'telegram-tl', 'v1', ?, ?)`,
		id[:], receivedAt, payload, hash[:])
	if err != nil {
		return evidenceID{}, err
	}
	return id, nil
}

func TestEvidenceAppendOnlyGuardsRejectMutation(t *testing.T) {
	path := t.TempDir() + "/append-only.db"
	db := openDB(t, path)
	defer db.Close()
	if _, err := db.Exec(evidenceSchema + evidenceAppendOnlyGuards); err != nil {
		t.Fatal(err)
	}

	appender := evidenceAppender{db: db}
	payload := []byte("original")
	id, err := appender.Append(payload, 1_787_103_600_123)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`UPDATE evidence SET payload = x'00' WHERE id = ?`, id[:]); err == nil {
		t.Fatal("UPDATE de Evidence deveria falhar fechado")
	}
	if _, err := db.Exec(`DELETE FROM evidence WHERE id = ?`, id[:]); err == nil {
		t.Fatal("DELETE de Evidence deveria falhar fechado")
	}

	var gotPayload, gotHash []byte
	if err := db.QueryRow(`SELECT payload, payload_sha256 FROM evidence WHERE id = ?`, id[:]).Scan(&gotPayload, &gotHash); err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(payload)
	if !bytes.Equal(gotPayload, payload) || !bytes.Equal(gotHash, wantHash[:]) {
		t.Fatal("tentativa de mutação alterou Evidence persistida")
	}

	if _, err := appender.Append([]byte("second"), 1_787_103_600_124); err != nil {
		t.Fatalf("append posterior aos rejects falhou: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rows=%d, want 2", count)
	}
}

func TestEvidenceAppendOnlyGuardsSurviveReopen(t *testing.T) {
	path := t.TempDir() + "/append-only-reopen.db"
	db := openDB(t, path)
	if _, err := db.Exec(evidenceSchema + evidenceAppendOnlyGuards); err != nil {
		t.Fatal(err)
	}
	appender := evidenceAppender{db: db}
	id, err := appender.Append([]byte("durable"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db = openDB(t, path)
	defer db.Close()
	if _, err := db.Exec(`UPDATE evidence SET received_at = 2 WHERE id = ?`, id[:]); err == nil {
		t.Fatal("UPDATE após reopen deveria continuar bloqueado")
	}
	if _, err := db.Exec(`DELETE FROM evidence WHERE id = ?`, id[:]); err == nil {
		t.Fatal("DELETE após reopen deveria continuar bloqueado")
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q, want ok", integrity)
	}
}
