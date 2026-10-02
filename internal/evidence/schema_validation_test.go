package evidence

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenRejectsMissingOrMisplacedEvidenceSchemaWithoutRepair(t *testing.T) {
	cases := []struct {
		name         string
		mutation     string
		object       string
		objectType   string
		table        string
		preservesRow bool
	}{
		{name: "tabela removida", mutation: `DROP TABLE evidence`, object: "evidence"},
		{name: "guard de update removido", mutation: `DROP TRIGGER evidence_no_update`, object: "evidence_no_update", preservesRow: true},
		{name: "guard de delete removido", mutation: `DROP TRIGGER evidence_no_delete`, object: "evidence_no_delete", preservesRow: true},
		{name: "view no lugar da tabela", mutation: `ALTER TABLE evidence RENAME TO evidence_saved; CREATE VIEW evidence AS SELECT * FROM evidence_saved`, object: "evidence", objectType: "view", preservesRow: true},
		{name: "guard de update em outra tabela", mutation: `CREATE TABLE other_evidence (id INTEGER); DROP TRIGGER evidence_no_update; CREATE TRIGGER evidence_no_update BEFORE UPDATE ON other_evidence BEGIN SELECT RAISE(ABORT, 'guard'); END`, object: "evidence_no_update", objectType: "trigger", table: "other_evidence", preservesRow: true},
		{name: "guard de delete em outra tabela", mutation: `CREATE TABLE other_evidence (id INTEGER); DROP TRIGGER evidence_no_delete; CREATE TRIGGER evidence_no_delete BEFORE DELETE ON other_evidence BEGIN SELECT RAISE(ABORT, 'guard'); END`, object: "evidence_no_delete", objectType: "trigger", table: "other_evidence", preservesRow: true},
		{name: "common state removido", mutation: `DROP TABLE source_sync_state`, object: "source_sync_state", preservesRow: true},
		{name: "channel state removido", mutation: `DROP TABLE source_sync_channel_state`, object: "source_sync_channel_state", preservesRow: true},
		{name: "common state com shape incorreto", mutation: `ALTER TABLE source_sync_state RENAME TO source_sync_state_saved; CREATE TABLE source_sync_state (subscription_id TEXT NOT NULL, user_id INTEGER NOT NULL, pts INTEGER NOT NULL, qts INTEGER NOT NULL, date INTEGER NOT NULL, seq TEXT NOT NULL, PRIMARY KEY(subscription_id, user_id)) STRICT`, object: "source_sync_state", objectType: "table", table: "source_sync_state", preservesRow: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema.db")
			store := mustOpenStoreAt(t, path)
			id := appendEvidence(t, store, sampleEvidence([]byte("preserved")))
			expected := readEvidenceRow(t, store.db, id[:])
			closeTestStore(t, store)

			db := mustOpenSQL(t, path, false)
			if _, err := db.Exec(tc.mutation); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			reopened, err := Open(context.Background(), path)
			if err == nil {
				closeTestStore(t, reopened)
				t.Fatal("Open aceitou schema sem tabela/guard esperado")
			}
			if !strings.Contains(err.Error(), tc.object) {
				t.Fatalf("erro sem objeto %q: %v", tc.object, err)
			}

			inspect := mustOpenSQL(t, path, true)
			defer func() { _ = inspect.Close() }()
			var objectType, table string
			err = inspect.QueryRow(`SELECT type, tbl_name FROM sqlite_schema WHERE name = ?`, tc.object).Scan(&objectType, &table)
			if tc.objectType == "" {
				if err != sql.ErrNoRows {
					t.Fatalf("objeto %q foi reconstruído: type=%q table=%q err=%v", tc.object, objectType, table, err)
				}
			} else if err != nil || objectType != tc.objectType || (tc.table != "" && table != tc.table) {
				t.Fatalf("objeto %q mudou após rejeição: type=%q table=%q err=%v", tc.object, objectType, table, err)
			}
			if tc.preservesRow {
				got := readEvidenceRow(t, inspect, id[:])
				if !reflect.DeepEqual(got, expected) {
					t.Fatal("linha de Evidence mudou após rejeição")
				}
			}
		})
	}
}

func TestOpenRejectsFutureUserVersionWithoutRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	store := mustOpenStoreAt(t, path)
	id := appendEvidence(t, store, sampleEvidence([]byte("preserved")))
	expected := readEvidenceRow(t, store.db, id[:])
	closeTestStore(t, store)

	db := mustOpenSQL(t, path, false)
	if _, err := db.Exec(`PRAGMA user_version = 999`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(context.Background(), path)
	if err == nil {
		closeTestStore(t, reopened)
		t.Fatal("Open aceitou user_version futuro")
	}
	if !strings.Contains(err.Error(), "user_version") {
		t.Fatalf("erro sem user_version: %v", err)
	}

	inspect := mustOpenSQL(t, path, true)
	defer func() { _ = inspect.Close() }()
	requireEqual(t, "user_version", mustQueryInt64(t, inspect, `PRAGMA user_version`), int64(999))
	got := readEvidenceRow(t, inspect, id[:])
	if !reflect.DeepEqual(got, expected) {
		t.Fatal("linha de Evidence mudou após rejeição")
	}
}
