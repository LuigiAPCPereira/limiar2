// Package sqlite implementa o novo storage local definido pelos ADRs 019 e 020.
// O pacote legado internal/storage permanece separado enquanto a migração side-by-side
// estiver em andamento.
package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const (
	driverName    = "sqlite3"
	applicationID = 0x4c494d32 // "LIM2": identifica somente o banco novo da Rebaseline.
)

// Store possui o ciclo de vida do novo banco SQLite sem expor *sql.DB aos consumidores.
type Store struct {
	db *sql.DB
}

// Open abre ou cria o novo banco, aplica a baseline operacional do ADR 019 e executa
// migrations SQL versionadas antes de retornar capabilities aos consumidores.
//
// Um arquivo existente e não vazio só é aberto para escrita quando já carrega o
// application_id do novo storage. Isso faz o boundary falhar fechado caso o chamador
// forneça por engano o path do banco legado ou de outro SQLite, preservando a regra
// side-by-side do ADR 019.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite storage: caminho do banco vazio")
	}

	needsClaim := false
	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return nil, fmt.Errorf("sqlite storage: caminho do banco %q é um diretório", path)
	case err == nil && info.Size() > 0:
		if err := verifyExistingDatabase(ctx, path); err != nil {
			return nil, err
		}
	case err == nil:
		// Arquivo vazio é equivalente a um banco ainda não inicializado e pode ser
		// reivindicado pelo novo storage antes de qualquer migration.
		needsClaim = true
	case os.IsNotExist(err):
		f, createErr := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- path fornecido pela configuração validada do chamador.
		if createErr != nil {
			return nil, fmt.Errorf("sqlite storage: criar %q: %w", path, createErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			return nil, fmt.Errorf("sqlite storage: fechar arquivo criado %q: %w", path, closeErr)
		}
		needsClaim = true
	default:
		return nil, fmt.Errorf("sqlite storage: stat %q: %w", path, err)
	}

	// Só normaliza permissões depois de provar que um arquivo existente pertence ao
	// novo storage. Um banco legado rejeitado não sofre chmod nem migration.
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("sqlite storage: chmod %q: %w", path, err)
	}

	db, err := sql.Open(driverName, databaseURI(path, false))
	if err != nil {
		return nil, fmt.Errorf("sqlite storage: abrir %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	closeWith := func(cause error) (*Store, error) {
		_ = db.Close()
		return nil, cause
	}

	if err := db.PingContext(ctx); err != nil {
		return closeWith(fmt.Errorf("sqlite storage: ping: %w", err))
	}

	// FULL é aplicado antes do primeiro write do arquivo novo. O application_id é um
	// marker de ownership/lifecycle, não schema; gravá-lo antes das migrations torna
	// um bootstrap parcialmente concluído retomável sem permitir tocar SQLite legado.
	if _, err := db.ExecContext(ctx, `PRAGMA synchronous=FULL`); err != nil {
		return closeWith(fmt.Errorf("sqlite storage: aplicar PRAGMA synchronous=FULL: %w", err))
	}
	if needsClaim {
		if err := claimNewDatabase(ctx, db); err != nil {
			return closeWith(err)
		}
	}

	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return closeWith(fmt.Errorf("sqlite storage: aplicar %s: %w", pragma, err))
		}
	}

	if err := migrate(ctx, db); err != nil {
		return closeWith(fmt.Errorf("sqlite storage: migrations: %w", err))
	}

	return &Store{db: db}, nil
}

func claimNewDatabase(ctx context.Context, db *sql.DB) error {
	pragma := fmt.Sprintf(`PRAGMA application_id = %d`, applicationID)
	if _, err := db.ExecContext(ctx, pragma); err != nil {
		return fmt.Errorf("sqlite storage: reivindicar banco novo: %w", err)
	}

	var got int64
	if err := db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&got); err != nil {
		return fmt.Errorf("sqlite storage: verificar application_id após claim: %w", err)
	}
	if got != applicationID {
		return fmt.Errorf("sqlite storage: application_id após claim=%d, esperado=%d", got, applicationID)
	}
	return nil
}

func verifyExistingDatabase(ctx context.Context, path string) error {
	db, err := sql.Open(driverName, databaseURI(path, true))
	if err != nil {
		return fmt.Errorf("sqlite storage: inspecionar banco existente %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() { _ = db.Close() }()

	var got int64
	if err := db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&got); err != nil {
		return fmt.Errorf("sqlite storage: ler application_id de %q: %w", path, err)
	}
	if got != applicationID {
		return fmt.Errorf("sqlite storage: recusar banco existente %q: application_id=%d não pertence ao novo storage", path, got)
	}
	return nil
}

func databaseURI(path string, readOnly bool) string {
	u := &url.URL{Scheme: "file", Path: path}
	if readOnly {
		q := u.Query()
		q.Set("mode", "ro")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// Close fecha o novo banco.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("sqlite storage: fechar: %w", err)
	}
	return nil
}

// EvidenceID é a identidade física UUIDv4 de 16 bytes definida pelo ADR 020.
type EvidenceID [16]byte

// Evidence contém o envelope físico mínimo aceito para uma nova admissão de Evidence.
// Timestamps já chegam normalizados como Unix milissegundos no boundary de persistência.
type Evidence struct {
	SubscriptionID   string
	Acquisition      string
	EventKind        string
	SourceEventType  string
	SourceOccurredAt *int64
	ReceivedAt       int64
	PayloadFormat    string
	PayloadSchema    string
	Payload          []byte
}

// EvidenceAppender é a capability estreita de append do ADR 020.
type EvidenceAppender interface {
	Append(context.Context, Evidence) (EvidenceID, error)
}

// EvidenceAppender retorna somente a capability de append; a conexão SQL permanece
// privada ao Store.
func (s *Store) EvidenceAppender() EvidenceAppender {
	return evidenceAppender{db: s.db, entropy: rand.Reader}
}

type evidenceAppender struct {
	db      *sql.DB
	entropy io.Reader
}

func newEvidenceID(r io.Reader) (EvidenceID, error) {
	var id EvidenceID
	if _, err := io.ReadFull(r, id[:]); err != nil {
		return EvidenceID{}, fmt.Errorf("ler entropia do evidence id: %w", err)
	}

	// RFC 9562 UUIDv4: versão 4 e variant 10xx.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}

func (a evidenceAppender) Append(ctx context.Context, evidence Evidence) (EvidenceID, error) {
	id, err := newEvidenceID(a.entropy)
	if err != nil {
		return EvidenceID{}, err
	}

	// O mesmo snapshot de bytes alimenta o hash e o INSERT, evitando uma segunda
	// autoridade de payload_sha256 no chamador.
	payload := append([]byte(nil), evidence.Payload...)
	hash := sha256.Sum256(payload)

	var sourceOccurredAt any
	if evidence.SourceOccurredAt != nil {
		sourceOccurredAt = *evidence.SourceOccurredAt
	}

	_, err = a.db.ExecContext(ctx, `INSERT INTO evidence(
		id, subscription_id, acquisition, event_kind, source_event_type,
		source_occurred_at, received_at, payload_format, payload_schema, payload, payload_sha256
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id[:], evidence.SubscriptionID, evidence.Acquisition, evidence.EventKind,
		evidence.SourceEventType, sourceOccurredAt, evidence.ReceivedAt,
		evidence.PayloadFormat, evidence.PayloadSchema, payload, hash[:])
	if err != nil {
		return EvidenceID{}, fmt.Errorf("sqlite storage: append evidence: %w", err)
	}

	return id, nil
}
