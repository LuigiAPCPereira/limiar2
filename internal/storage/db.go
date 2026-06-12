// Package storage é responsável por toda a persistência do limiar-collector
// no banco de dados Tursogo embarcado. Todas as instruções SQL residem neste
// pacote; nenhuma outra camada interage diretamente com database/sql.
//
// O driver é o tursogo ("turso"), utilizado através do database/sql apenas
// com o placeholder ?. O uso de SQLite genérico, GORM ou qualquer outro ORM não é permitido.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "turso.tech/database/tursogo" // registra o driver "turso"
)

// driverName é o nome do driver database/sql registrado pelo tursogo.
const driverName = "turso"

// DB encapsula a conexão database/sql do Tursogo e seu ciclo de vida.
type DB struct {
	conn *sql.DB
}

// Open abre (ou cria) o banco de dados Tursogo no dbPath especificado e aplica
// todas as migrações embarcadas. O DB retornado deve ser fechado pelo chamador.
func Open(ctx context.Context, dbPath string) (*DB, error) {
	// Garante que o arquivo do banco seja criado/mantido com
	// permissões restritas (0600) para proteger a sessão do Telegram e as mensagens.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("storage: create db %q: %w", dbPath, err)
		}
		_ = f.Close()
	} else {
		// Reforça permissões restritas mesmo se o arquivo já existir
		if err := os.Chmod(dbPath, 0600); err != nil {
			return nil, fmt.Errorf("storage: chmod db %q: %w", dbPath, err)
		}
	}

	conn, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", dbPath, err)
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: ping: %w", err)
	}

	// WAL mode permite readers e writers concorrentes — necessário quando
	// collector e processor rodam simultaneamente no mesmo limiar.db.
	if _, err := conn.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: wal mode: %w", err)
	}
	// busy_timeout faz o SQLite esperar até 5s quando o banco está ocupado,
	// em vez de retornar SQLITE_BUSY imediatamente.
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: busy_timeout: %w", err)
	}
	// foreign_keys habilita validação de chaves estrangeiras. Sem este PRAGMA,
	// FKs são apenas decorativas (inserções com referências inválidas não falham).
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: foreign_keys: %w", err)
	}

	if err := migrate(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	return &DB{conn: conn}, nil
}

// Conn retorna a conexão *sql.DB subjacente. Apenas a goroutine DBWriter pode emitir
// gravações através dela; gravações concorrentes de múltiplas goroutines são proibidas.
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
