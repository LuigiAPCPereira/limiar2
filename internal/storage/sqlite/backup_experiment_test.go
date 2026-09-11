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
	backupPath := filepath.Join(t.TempDir(), "limiar-backup.db")

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
	requireBytesEqual(t, "backup payload", row.payload, []byte("before-backup"))
}
