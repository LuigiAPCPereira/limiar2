package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func probeSQLitePing(t *testing.T, dsn string) {
	t.Helper()

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		t.Fatalf("sql.Open(%q): %v", dsn, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close(%q): %v", dsn, err)
		}
	}()

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("Ping(%q): %v", dsn, err)
	}
}

func TestSQLiteDriverOpensNativeFilename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native-filename.db")
	if err := createDatabaseFile(path); err != nil {
		t.Fatal(err)
	}

	probeSQLitePing(t, path)
}

func TestSQLiteDriverOpensMinimalFileURI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimal-file-uri.db")
	if err := createDatabaseFile(path); err != nil {
		t.Fatal(err)
	}

	// The upstream driver's documented URI form is file:demo.db. Keep the
	// Windows drive prefix but normalize separators, avoiding a hierarchical
	// file:///C:/... URL until runtime evidence says that form is supported by
	// the pure-Go VFS on Windows.
	uriPath := filepath.ToSlash(path)
	probeSQLitePing(t, "file:"+uriPath)
}

func TestSQLiteDriverOpensProductionURI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production-uri.db")
	if err := createDatabaseFile(path); err != nil {
		t.Fatal(err)
	}

	probeSQLitePing(t, databaseURI(path, false))
}
