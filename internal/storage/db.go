// Package storage owns all persistence for limiar-collector against the
// embedded Tursogo database. Every SQL statement lives in this package; no
// other layer touches database/sql directly.
//
// The driver is tursogo ("turso"), used through database/sql with ? as the
// only placeholder token. No SQLite, GORM, or other ORM is permitted.
package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "turso.tech/database/tursogo" // registers the "turso" driver
)

// driverName is the database/sql driver registered by tursogo.
const driverName = "turso"

// DB wraps the Tursogo database/sql connection and its lifecycle.
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the Tursogo database at dbPath and applies all
// embedded migrations. The returned DB must be closed by the caller.
func Open(ctx context.Context, dbPath string) (*DB, error) {
	conn, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", dbPath, err)
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: ping: %w", err)
	}
	if err := migrate(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	return &DB{conn: conn}, nil
}

// Conn returns the underlying *sql.DB. Only the DBWriter goroutine may issue
// writes through it; concurrent writes from multiple goroutines are forbidden.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Close closes the database connection.
func (db *DB) Close() error {
	if err := db.conn.Close(); err != nil {
		return fmt.Errorf("storage: close: %w", err)
	}
	return nil
}
