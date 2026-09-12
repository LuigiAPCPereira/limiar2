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
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const (
	driverName    = "sqlite3"
	applicationID = 0x4c494d32 // LIM2 em ASCII: L=4c, I=49, M=4d, 2=32.
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
	needsClaim, err := prepareDatabaseFile(ctx, path)
	if err != nil {
		return nil, err
	}

	db, err := openWritableDatabase(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := initializeDatabase(ctx, db, needsClaim); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func prepareDatabaseFile(ctx context.Context, path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("sqlite storage: caminho do banco vazio")
	}

	info, err := os.Stat(path)
	if err == nil {
		return prepareExistingDatabase(ctx, path, info)
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("sqlite storage: stat %q: %w", path, err)
	}
	if err := createDatabaseFile(path); err != nil {
		return false, err
	}
	return true, nil
}

func prepareExistingDatabase(ctx context.Context, path string, info os.FileInfo) (bool, error) {
	if info.IsDir() {
		return false, fmt.Errorf("sqlite storage: caminho do banco %q é um diretório", path)
	}

	needsClaim := info.Size() == 0
	if !needsClaim {
		if err := verifyExistingDatabase(ctx, path); err != nil {
			return false, err
		}
	}

	// Só normaliza permissões depois de provar que um arquivo não vazio pertence ao
	// novo storage. Um banco legado rejeitado não sofre chmod nem migration.
	if err := os.Chmod(path, 0o600); err != nil {
		return false, fmt.Errorf("sqlite storage: chmod %q: %w", path, err)
	}
	return needsClaim, nil
}

func createDatabaseFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- path fornecido pela configuração validada do chamador.
	if err != nil {
		return fmt.Errorf("sqlite storage: criar %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("sqlite storage: fechar arquivo criado %q: %w", path, err)
	}
	return nil
}

func openWritableDatabase(ctx context.Context, path string) (*sql.DB, error) {
	// databaseURI injeta os PRAGMAs connection-local via _pragma. O driver ncruces
	// reaplica esses parâmetros sempre que database/sql abre uma conexão física nova,
	// preservando FULL/FKs/busy_timeout mesmo se a conexão do pool for substituída.
	db, err := sql.Open(driverName, databaseURI(path, false))
	if err != nil {
		return nil, fmt.Errorf("sqlite storage: abrir %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite storage: ping: %w", err)
	}
	return db, nil
}

func initializeDatabase(ctx context.Context, db *sql.DB, needsClaim bool) error {
	// O application_id é ownership/lifecycle, não schema. Ele é persistido antes de
	// entrar em WAL/migrations para que um bootstrap já reivindicado possa ser retomado
	// sem permitir que o novo boundary escreva em um SQLite legado.
	if needsClaim {
		if err := claimNewDatabase(ctx, db); err != nil {
			return err
		}
	}
	if err := enableWAL(ctx, db); err != nil {
		return err
	}
	if err := verifyOperationalBaseline(ctx, db); err != nil {
		return err
	}
	if err := migrate(ctx, db); err != nil {
		return fmt.Errorf("sqlite storage: migrations: %w", err)
	}
	return nil
}

func enableWAL(ctx context.Context, db *sql.DB) error {
	// journal_mode é persistente no arquivo. PRAGMA journal_mode=WAL pode responder
	// com outro modo quando a troca não é possível, portanto ausência de erro não basta.
	var journal string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode=WAL`).Scan(&journal); err != nil {
		return fmt.Errorf("sqlite storage: aplicar WAL: %w", err)
	}
	if journal != "wal" {
		return fmt.Errorf("sqlite storage: journal_mode=%q, esperado=wal", journal)
	}
	return nil
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

func verifyOperationalBaseline(ctx context.Context, db *sql.DB) error {
	var journal string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return fmt.Errorf("sqlite storage: verificar journal_mode: %w", err)
	}
	if journal != "wal" {
		return fmt.Errorf("sqlite storage: journal_mode=%q após configuração, esperado=wal", journal)
	}

	checks := []struct {
		name  string
		query string
		want  int
	}{
		{name: "synchronous", query: `PRAGMA synchronous`, want: 2},
		{name: "foreign_keys", query: `PRAGMA foreign_keys`, want: 1},
		{name: "busy_timeout", query: `PRAGMA busy_timeout`, want: 5000},
	}
	for _, check := range checks {
		var got int
		if err := db.QueryRowContext(ctx, check.query).Scan(&got); err != nil {
			return fmt.Errorf("sqlite storage: verificar %s: %w", check.name, err)
		}
		if got != check.want {
			return fmt.Errorf("sqlite storage: %s=%d, esperado=%d", check.name, got, check.want)
		}
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
	q := url.Values{}
	if readOnly {
		q.Set("mode", "ro")
	} else {
		// O driver ncruces/go-sqlite3 executa cada _pragma sempre que uma conexão física é aberta.
		// Busy timeout vem primeiro conforme a recomendação upstream; WAL não entra
		// aqui porque só pode ser ativado depois do claim explícito do arquivo novo.
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "foreign_keys(ON)")
		q.Add("_pragma", "synchronous(FULL)")
	}

	// Em Windows, o driver aceita paths absolutos de drive na forma file:C:/... .
	// Serializar C:\... por url.URL produz uma URI hierárquica file:///C:/..., que
	// o VFS do ncruces/go-sqlite3 não abre neste boundary. Restringimos a adaptação
	// a drive letters para não inferir sem Evidence a semântica de UNC paths.
	volume := filepath.VolumeName(path)
	if len(volume) == 2 && volume[1] == ':' {
		uri := "file:" + filepath.ToSlash(path)
		if rawQuery := q.Encode(); rawQuery != "" {
			uri += "?" + rawQuery
		}
		return uri
	}

	u := &url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
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
