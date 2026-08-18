package storagecontract_test

import (
	"path/filepath"
	"testing"
)

func TestEvidenceCommitSurvivesRejectedStateWrite(t *testing.T) {
	for _, e := range engines {
		e := e
		t.Run(e.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "limiar.db")
			db := openDB(t, e, path)
			configureDurability(t, db)
			createSchema(t, db)
			mustExec(t, db, `CREATE TRIGGER reject_pts_2 BEFORE UPDATE OF pts ON source_sync_state
				WHEN NEW.pts = 2 BEGIN SELECT RAISE(ABORT, 'injected state failure'); END`)

			mustExec(t, db, `INSERT INTO evidence_records(id, subscription_id, event_kind, received_at, payload_schema, payload)
				VALUES ('e-state-fail', 'telegram:test', 'update', '2026-08-18T17:02:00-03:00', 'telegram/update-v1', X'02')`)

			if _, err := db.Exec(`UPDATE source_sync_state SET pts=2 WHERE subscription_id='telegram:test'`); err == nil {
				t.Fatal("injected state write unexpectedly succeeded")
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close after rejected state write: %v", err)
			}

			db = openDB(t, e, path)
			configureDurability(t, db)
			defer db.Close()

			var evidenceCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM evidence_records WHERE id='e-state-fail'`).Scan(&evidenceCount); err != nil {
				t.Fatalf("read committed Evidence: %v", err)
			}
			if evidenceCount != 1 {
				t.Fatalf("committed Evidence count=%d, want 1", evidenceCount)
			}

			var pts int
			if err := db.QueryRow(`SELECT pts FROM source_sync_state WHERE subscription_id='telegram:test'`).Scan(&pts); err != nil {
				t.Fatalf("read state after rejected write: %v", err)
			}
			if pts != 0 {
				t.Fatalf("state advanced to %d after rejected write; want 0", pts)
			}
		})
	}
}
