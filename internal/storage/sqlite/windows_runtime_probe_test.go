package sqlite

import (
	"context"
	"database/sql"
	"net/url"
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

	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uriPath = "/" + uriPath
	}
	u := &url.URL{Scheme: "file", Path: uriPath}
	probeSQLitePing(t, u.String())
}

func TestSQLiteDriverOpensProductionURI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production-uri.db")
	if err := createDatabaseFile(path); err != nil {
		t.Fatal(err)
	}

	probeSQLitePing(t, databaseURI(path, false))
}
