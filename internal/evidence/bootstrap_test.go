package evidence

import (
	"context"
	"path/filepath"
	"testing"
)

func claimUnmigratedTestDatabase(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	db := mustOpenSQL(t, path, false)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := claimNewDatabase(ctx, db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	requireEqual(t, "user_version antes do restart",
		mustQueryInt64(t, db, `PRAGMA user_version`), int64(0))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenResumesClaimedUnmigratedDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "partially-initialized.db")
	claimUnmigratedTestDatabase(t, ctx, path)

	store := mustOpenStoreAt(t, path)
	defer closeTestStore(t, store)
	requireEqual(t, "application_id",
		mustQueryInt64(t, store.db, `PRAGMA application_id`), int64(applicationID))
	requireEqual(t, "user_version após retomada",
		mustQueryInt64(t, store.db, `PRAGMA user_version`), int64(1))
	requireEqual(t, "evidence table count", mustQueryInt64(t, store.db,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='evidence'`), int64(1))
}

func TestOperationalBaselineSurvivesPhysicalConnectionReplacement(t *testing.T) {
	ctx := context.Background()
	_, store := openTestStore(t)
	defer closeTestStore(t, store)

	if err := verifyOperationalBaseline(ctx, store.db); err != nil {
		t.Fatalf("baseline inicial: %v", err)
	}

	// Zero conexões ociosas força database/sql a fechar a conexão física quando ela
	// volta ao pool. Ao restaurar o limite, a próxima operação abre outra conexão;
	// os _pragma do DSN precisam reaplicar FULL/FKs/busy_timeout automaticamente.
	store.db.SetMaxIdleConns(0)
	store.db.SetMaxIdleConns(1)

	if err := store.db.PingContext(ctx); err != nil {
		t.Fatalf("ping após substituir conexão: %v", err)
	}
	if err := verifyOperationalBaseline(ctx, store.db); err != nil {
		t.Fatalf("baseline após substituir conexão: %v", err)
	}
}
