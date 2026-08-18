package storagecontract_test

import (
	"bytes"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "turso.tech/database/tursogo"
)

type engine struct {
	name   string
	driver string
	dsn    func(string) string
}

var engines = []engine{
	{name: "tursogo-v0.7.2", driver: "turso", dsn: func(path string) string { return path }},
	{name: "ncruces-v0.35.3", driver: "sqlite3", dsn: func(path string) string { return "file:" + filepath.ToSlash(path) }},
}

func openDB(t *testing.T, e engine, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open(e.driver, e.dsn(path))
	if err != nil {
		t.Fatalf("sql.Open(%s): %v", e.name, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("Ping(%s): %v", e.name, err)
	}
	return db
}

func configureDurability(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `PRAGMA journal_mode=WAL`)
	mustExec(t, db, `PRAGMA synchronous=FULL`)
	mustExec(t, db, `PRAGMA foreign_keys=ON`)
	mustExec(t, db, `PRAGMA busy_timeout=5000`)
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func createSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `CREATE TABLE evidence_records (
		id TEXT PRIMARY KEY,
		subscription_id TEXT NOT NULL,
		event_kind TEXT NOT NULL,
		received_at TEXT NOT NULL,
		payload_schema TEXT NOT NULL,
		payload BLOB NOT NULL
	)`)
	mustExec(t, db, `CREATE TABLE source_sync_state (
		subscription_id TEXT PRIMARY KEY,
		pts INTEGER NOT NULL
	)`)
	mustExec(t, db, `CREATE TABLE evidence_refs (
		id INTEGER PRIMARY KEY,
		evidence_id TEXT NOT NULL REFERENCES evidence_records(id)
	)`)
	mustExec(t, db, `INSERT INTO source_sync_state(subscription_id, pts) VALUES ('telegram:test', 0)`)
}

func TestStorageDurabilityContract(t *testing.T) {
	for _, e := range engines {
		e := e
		t.Run(e.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "limiar.db")
			db := openDB(t, e, path)
			configureDurability(t, db)
			createSchema(t, db)

			var journal string
			if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
				t.Fatalf("read journal_mode: %v", err)
			}
			if strings.ToLower(journal) != "wal" {
				t.Fatalf("journal_mode=%q, want WAL", journal)
			}

			var synchronous int
			if err := db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil {
				t.Fatalf("read synchronous: %v", err)
			}
			if synchronous != 2 {
				t.Fatalf("synchronous=%d, want FULL(2)", synchronous)
			}

			var foreignKeys int
			if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
				t.Fatalf("read foreign_keys: %v", err)
			}
			if foreignKeys != 1 {
				t.Fatalf("foreign_keys=%d, want 1", foreignKeys)
			}

			payload := []byte{0x00, 0x01, 0xff, '{', '}', '\n'}
			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("begin evidence: %v", err)
			}
			if _, err := tx.Exec(`INSERT INTO evidence_records(id, subscription_id, event_kind, received_at, payload_schema, payload)
				VALUES (?, ?, ?, ?, ?, ?)`, "e-1", "telegram:test", "update", "2026-08-18T17:00:00-03:00", "telegram/update-v1", payload); err != nil {
				_ = tx.Rollback()
				t.Fatalf("insert evidence: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit evidence: %v", err)
			}

			if err := db.Close(); err != nil {
				t.Fatalf("close after evidence: %v", err)
			}
			db = openDB(t, e, path)
			configureDurability(t, db)

			var gotPayload []byte
			if err := db.QueryRow(`SELECT payload FROM evidence_records WHERE id='e-1'`).Scan(&gotPayload); err != nil {
				t.Fatalf("reload evidence: %v", err)
			}
			if !bytes.Equal(gotPayload, payload) {
				t.Fatalf("payload changed after reopen: got=%v want=%v", gotPayload, payload)
			}

			var pts int
			if err := db.QueryRow(`SELECT pts FROM source_sync_state WHERE subscription_id='telegram:test'`).Scan(&pts); err != nil {
				t.Fatalf("read old pts: %v", err)
			}
			if pts != 0 {
				t.Fatalf("state advanced before explicit state write: %d", pts)
			}

			mustExec(t, db, `UPDATE source_sync_state SET pts=1 WHERE subscription_id='telegram:test'`)
			if err := db.Close(); err != nil {
				t.Fatalf("close after state: %v", err)
			}
			db = openDB(t, e, path)
			configureDurability(t, db)
			defer db.Close()

			if err := db.QueryRow(`SELECT pts FROM source_sync_state WHERE subscription_id='telegram:test'`).Scan(&pts); err != nil {
				t.Fatalf("reload pts: %v", err)
			}
			if pts != 1 {
				t.Fatalf("persisted pts=%d, want 1", pts)
			}

			tx, err = db.Begin()
			if err != nil {
				t.Fatalf("begin rollback: %v", err)
			}
			if _, err := tx.Exec(`INSERT INTO evidence_records(id, subscription_id, event_kind, received_at, payload_schema, payload)
				VALUES ('e-rollback', 'telegram:test', 'update', '2026-08-18T17:01:00-03:00', 'telegram/update-v1', X'01')`); err != nil {
				_ = tx.Rollback()
				t.Fatalf("insert rollback evidence: %v", err)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatalf("rollback: %v", err)
			}
			var rollbackCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM evidence_records WHERE id='e-rollback'`).Scan(&rollbackCount); err != nil {
				t.Fatalf("count rollback evidence: %v", err)
			}
			if rollbackCount != 0 {
				t.Fatalf("rolled-back evidence persisted")
			}

			if _, err := db.Exec(`INSERT INTO evidence_refs(evidence_id) VALUES ('missing')`); err == nil {
				t.Fatalf("foreign key violation was accepted")
			}

			var integrity string
			if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
				t.Fatalf("integrity_check: %v", err)
			}
			if strings.ToLower(integrity) != "ok" {
				t.Fatalf("integrity_check=%q", integrity)
			}
		})
	}
}

func TestBulkEvidenceAndBackupContract(t *testing.T) {
	for _, e := range engines {
		e := e
		t.Run(e.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "limiar.db")
			backup := filepath.Join(dir, "backup.db")
			db := openDB(t, e, path)
			configureDurability(t, db)
			createSchema(t, db)

			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("begin bulk: %v", err)
			}
			stmt, err := tx.Prepare(`INSERT INTO evidence_records(id, subscription_id, event_kind, received_at, payload_schema, payload)
				VALUES (?, 'telegram:test', 'update', '2026-08-18T17:00:00-03:00', 'telegram/update-v1', ?)`)
			if err != nil {
				_ = tx.Rollback()
				t.Fatalf("prepare bulk: %v", err)
			}
			for i := 0; i < 10000; i++ {
				if _, err := stmt.Exec(fmt.Sprintf("bulk-%05d", i), []byte(fmt.Sprintf(`{"i":%d}`, i))); err != nil {
					_ = stmt.Close()
					_ = tx.Rollback()
					t.Fatalf("bulk insert %d: %v", i, err)
				}
			}
			_ = stmt.Close()
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit bulk: %v", err)
			}

			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM evidence_records`).Scan(&count); err != nil {
				t.Fatalf("count bulk: %v", err)
			}
			if count != 10000 {
				t.Fatalf("count=%d, want 10000", count)
			}

			quotedBackup := strings.ReplaceAll(filepath.ToSlash(backup), "'", "''")
			if _, err := db.Exec(`VACUUM INTO '` + quotedBackup + `'`); err != nil {
				t.Fatalf("VACUUM INTO: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close source: %v", err)
			}

			backupDB := openDB(t, e, backup)
			defer backupDB.Close()
			if err := backupDB.QueryRow(`SELECT COUNT(*) FROM evidence_records`).Scan(&count); err != nil {
				t.Fatalf("count backup: %v", err)
			}
			if count != 10000 {
				t.Fatalf("backup count=%d, want 10000", count)
			}
		})
	}
}

func TestCurrentLimiarSynchronousNormalProbe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limiar.db")
	e := engines[0]
	db := openDB(t, e, path)
	defer db.Close()

	_, err := db.Exec(`PRAGMA synchronous=NORMAL`)
	if err != nil {
		t.Logf("Tursogo rejected synchronous=NORMAL as current COMPAT.md suggests: %v", err)
		return
	}

	var synchronous int
	if err := db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil {
		t.Fatalf("read synchronous after NORMAL: %v", err)
	}
	t.Logf("Tursogo accepted synchronous=NORMAL at runtime; readback=%d. Treat as unsupported until documented by upstream.", synchronous)
}
