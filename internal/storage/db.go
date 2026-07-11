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

	"github.com/limiar/collector/internal/logger"
)

// driverName é o nome do driver database/sql registrado pelo tursogo.
const driverName = "turso"

// DB encapsula a conexão database/sql do Tursogo e seu ciclo de vida.
type DB struct {
	conn *sql.DB
}

// Open abre (ou cria) o banco de dados Tursogo no dbPath especificado e aplica
// todas as migrações embarcadas. O DB retornado deve ser fechado pelo chamador.
// Durante a abertura, emite logs de progresso via stderr para visibilidade
// pré-logger estruturado (o logger ainda não está configurado neste ponto).
// Open abre (ou cria) o banco de dados Tursogo no dbPath especificado e aplica
// todas as migrações embarcadas. O DB retornado deve ser fechado pelo chamador.
// log pode ser nil (usa NopLogger).
func Open(ctx context.Context, dbPath string, log logger.Logger) (*DB, error) {
	if log == nil {
		log = logger.NopLogger{}
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 — path from validated config
		if err != nil {
			return nil, fmt.Errorf("storage: create db %q: %w", dbPath, err)
		}
		_ = f.Close()
		log.Info("📦 Banco criado", "caminho", dbPath)
	}

	// Forçar permissões estritas em bancos já existentes para proteger contra umasks permissivos
	if err := os.Chmod(dbPath, 0o600); err != nil {
		log.Warn("Não foi possível garantir permissões restritas (0o600) no banco de dados", "caminho", dbPath, "erro", err)
	}

	log.Info("🔌 Abrindo banco", "caminho", dbPath, "driver", driverName)
	conn, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", dbPath, err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: ping: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: wal mode: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA synchronous=NORMAL`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: synchronous: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA cache_size=-65536`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: cache_size: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: busy_timeout: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: foreign_keys: %w", err)
	}

	if err := migrate(ctx, conn, log); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	log.Info("✅ Banco pronto", "caminho", dbPath, "journal", "WAL", "busy_timeout_ms", 5000, "foreign_keys", true)
	return &DB{conn: conn}, nil
}

// DB retorna a conexão *sql.DB subjacente. Apenas a goroutine DBWriter pode emitir
// gravações através dela; gravações concorrentes de múltiplas goroutines são proibidas.
func (db *DB) DB() *sql.DB {
	return db.conn
}

// Close fecha a conexão com o banco de dados.
func (db *DB) Close() error {
	if err := db.conn.Close(); err != nil {
		return fmt.Errorf("storage: close: %w", err)
	}
	return nil
}
