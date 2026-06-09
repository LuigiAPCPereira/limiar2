// Package storage possui toda a persistência do limiar-collector contra o
// banco de dados embutido Tursogo. Toda instrução SQL reside neste pacote; nenhuma
// outra camada toca o database/sql diretamente.
//
// O driver é o tursogo ("turso"), usado através de database/sql usando ? como
// o único token (marcador) para parâmetros. Nenhum SQLite, GORM ou outro ORM é permitido.
package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "turso.tech/database/tursogo" // registers the "turso" driver
)

// driverName é o driver database/sql registrado pelo tursogo.
const driverName = "turso"

// DB envolve a conexão database/sql do Tursogo e o seu ciclo de vida.
type DB struct {
	conn *sql.DB
}

// Open abre (ou cria) o banco de dados Tursogo em dbPath e aplica todas as
// migrations embutidas. O DB retornado deve ser fechado pelo chamador.
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

// Conn retorna o *sql.DB subjacente. Apenas a goroutine DBWriter pode emitir
// gravações através dele; gravações concorrentes de múltiplas goroutines são proibidas.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Close fecha a conexão com o banco de dados.
func (db *DB) Close() error {
	if err := db.conn.Close(); err != nil {
		return fmt.Errorf("storage: close: %w", err)
	}
	return nil
}
