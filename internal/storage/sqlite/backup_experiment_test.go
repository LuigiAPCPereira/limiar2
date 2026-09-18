package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBackupSnapshotRestoresOwnedEvidence(t *testing.T) {
	ctx := context.Background()
	_, source := openTestStore(t)
	defer closeTestStore(t, source)

	firstID := appendEvidence(t, source, sampleEvidence([]byte("before-backup")))
	original := readEvidenceRow(t, source.db, firstID[:])
	backupPath := filepath.Join(t.TempDir(), "limiar-backup.db")

	// SQLite documenta o argumento de INTO como uma expressão SQL escalar;
	// manter o bind evita interpolar paths no comando experimental.
	if _, err := source.db.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		t.Fatalf("VACUUM INTO backup: %v", err)
	}

	appendEvidence(t, source, sampleEvidence([]byte("after-backup")))
	requireEqual(t, "source evidence count", mustQueryInt64(t, source.db, `SELECT COUNT(*) FROM evidence`), int64(2))

	restored := mustOpenStoreAt(t, backupPath)
	defer closeTestStore(t, restored)

	requireEqual(t, "backup application_id", mustQueryInt64(t, restored.db, `PRAGMA application_id`), int64(applicationID))
	requireEqual(t, "backup integrity_check", mustQueryString(t, restored.db, `PRAGMA integrity_check`), "ok")
	requireEqual(t, "backup evidence count", mustQueryInt64(t, restored.db, `SELECT COUNT(*) FROM evidence`), int64(1))

	row := readEvidenceRow(t, restored.db, firstID[:])
	requireBytesEqual(t, "backup id", row.id, original.id)
	requireEqual(t, "backup subscription_id", row.subscription, original.subscription)
	requireEqual(t, "backup acquisition", row.acquisition, original.acquisition)
	requireEqual(t, "backup event_kind", row.eventKind, original.eventKind)
	requireEqual(t, "backup source_event_type", row.sourceType, original.sourceType)
	requireEqual(t, "backup source_occurred_at", row.sourceOccurredAt, original.sourceOccurredAt)
	requireEqual(t, "backup received_at", row.receivedAt, original.receivedAt)
	requireEqual(t, "backup payload_format", row.payloadFormat, original.payloadFormat)
	requireEqual(t, "backup payload_schema", row.payloadSchema, original.payloadSchema)
	requireBytesEqual(t, "backup payload", row.payload, original.payload)
	requireBytesEqual(t, "backup payload_sha256", row.hash, original.hash)

	appendedID := appendEvidence(t, restored, sampleEvidence([]byte("after-restore")))
	requireEqual(t, "restored evidence count", mustQueryInt64(t, restored.db, `SELECT COUNT(*) FROM evidence`), int64(2))
	appended := readEvidenceRow(t, restored.db, appendedID[:])
	requireBytesEqual(t, "restored append payload", appended.payload, []byte("after-restore"))
}
