package processor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limiar/collector/internal/storage"
)

func openTempProcessorRepo() (*storage.ProcessorRepository, *storage.DB, func(), error) {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "limiar-processor-")
	if err != nil {
		return nil, nil, nil, err
	}
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.Open(ctx, dbPath, nil)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, nil, err
	}
	repo, err := storage.NewProcessorRepository(db.DB())
	if err != nil {
		_ = db.Close()
		_ = os.RemoveAll(dir)
		return nil, nil, nil, err
	}
	cleanup := func() {
		_ = repo.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return repo, db, cleanup, nil
}

func _TestPhotoMetadataStats(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}
	// Cadeia FK (foreign_keys=ON): channel → raw_message → processed_message.
	mustExec(`INSERT INTO channels (id, username, title, active) VALUES (1, 'c1', 'C', 1)`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (1, 1, 101, '{}')`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (2, 1, 102, '{}')`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (3, 1, 103, '{}')`)
	// #1: metadados MTProto completos.
	mustExec(`INSERT INTO processed_messages (raw_message_id, channel_id, message_id, posted_at, photo_id, photo_access_hash, photo_file_ref, photo_dcid)
		VALUES (1, 1, 101, '2026-01-01T00:00:00Z', 100, 999, 'ref', 2)`)
	// #2: photo_id presente, mas access_hash/file_ref/dcid ausentes (parcial).
	mustExec(`INSERT INTO processed_messages (raw_message_id, channel_id, message_id, posted_at, photo_id)
		VALUES (2, 1, 102, '2026-01-01T00:00:00Z', 200)`)
	// #3: sem foto.
	mustExec(`INSERT INTO processed_messages (raw_message_id, channel_id, message_id, posted_at)
		VALUES (3, 1, 103, '2026-01-01T00:00:00Z')`)

	stats, err := repo.PhotoMetadataStats(ctx)
	if err != nil {
		t.Fatalf("PhotoMetadataStats: %v", err)
	}
	if stats.TotalProcessed != 3 {
		t.Errorf("TotalProcessed = %d, quer 3", stats.TotalProcessed)
	}
	if stats.WithPhoto != 2 {
		t.Errorf("WithPhoto = %d, quer 2", stats.WithPhoto)
	}
	if stats.CompleteMTProto != 1 {
		t.Errorf("CompleteMTProto = %d, quer 1", stats.CompleteMTProto)
	}
}

func TestGetPhotoMetadata_DerivesExactIDsFromRawPayload(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}

	const (
		exactPhotoID      int64 = 4985843018296396847
		exactAccessHash   int64 = -7524180780117537531
		roundedPhotoID    int64 = 4985843018296396800
		roundedAccessHash int64 = -7524180780117537792
	)

	rawPayload := `{
		"ID": 95232,
		"Message": "produto com foto",
		"Media": {
			"Photo": {
				"ID": 4985843018296396847,
				"AccessHash": -7524180780117537531,
				"FileReference": "AlCZaRIAAXQAaiW+TljSnSegJJ4ybQVsJymwX1s=",
				"DCID": 1
			}
		}
	}`

	mustExec(`INSERT INTO channels (id, username, title, active) VALUES (1, 'c1', 'C', 1)`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (1, 1, 95232, ?)`, rawPayload)
	mustExec(`INSERT INTO processed_messages (
		id, raw_message_id, channel_id, message_id, posted_at,
		photo_id, photo_access_hash, photo_file_ref, photo_dcid
	) VALUES (1, 1, 1, 95232, '2026-01-01T00:00:00Z', ?, ?, 'rounded-ref', 1)`,
		roundedPhotoID, roundedAccessHash)

	meta, err := repo.GetPhotoMetadata(ctx, 1)
	if err != nil {
		t.Fatalf("GetPhotoMetadata: %v", err)
	}
	if meta.PhotoID != exactPhotoID {
		t.Errorf("PhotoID = %d, want %d", meta.PhotoID, exactPhotoID)
	}
	if meta.AccessHash != exactAccessHash {
		t.Errorf("AccessHash = %d, want %d", meta.AccessHash, exactAccessHash)
	}
	if meta.FileReference != "AlCZaRIAAXQAaiW+TljSnSegJJ4ybQVsJymwX1s=" {
		t.Errorf("FileReference = %q", meta.FileReference)
	}
}

func TestListPendingPhotoMetadataForBackfillUsesExactRawIDs(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}

	rawPayload := `{
		"ID": 95232,
		"Message": "produto com foto",
		"Media": {
			"Photo": {
				"ID": 4985843018296396847,
				"AccessHash": -7524180780117537531,
				"FileReference": "AlCZaRIAAXQAaiW+TljSnSegJJ4ybQVsJymwX1s=",
				"DCID": 1
			}
		}
	}`

	mustExec(`INSERT INTO channels (id, username, title, active) VALUES (1, 'c1', 'C', 1)`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (1, 1, 95232, ?)`, rawPayload)
	mustExec(`INSERT INTO processed_messages (
		id, raw_message_id, channel_id, message_id, posted_at,
		photo_id, photo_access_hash, photo_file_ref, photo_dcid
	) VALUES (1, 1, 1, 95232, '2026-01-01T00:00:00Z', 4985843018296396800, -7524180780117537792, 'rounded-ref', 1)`)

	photos, err := repo.ListPendingPhotoMetadataForBackfill(ctx, 0)
	if err != nil {
		t.Fatalf("ListPendingPhotoMetadataForBackfill: %v", err)
	}
	if len(photos) != 1 {
		t.Fatalf("len(photos) = %d, want 1", len(photos))
	}
	if photos[0].PhotoID != 4985843018296396847 {
		t.Errorf("PhotoID = %d, want exact raw id", photos[0].PhotoID)
	}
	if photos[0].AccessHash != -7524180780117537531 {
		t.Errorf("AccessHash = %d, want exact raw access_hash", photos[0].AccessHash)
	}
	if err := repo.SavePhotoData(ctx, photos[0].PhotoID, []byte("cached")); err != nil {
		t.Fatalf("SavePhotoData: %v", err)
	}
	photos, err = repo.ListPendingPhotoMetadataForBackfill(ctx, 0)
	if err != nil {
		t.Fatalf("ListPendingPhotoMetadataForBackfill after cache: %v", err)
	}
	if len(photos) != 0 {
		t.Fatalf("len(photos) after cache = %d, want 0", len(photos))
	}
}

// --- Regressão: SaveProcessedBatch atômico (sem commit parcial) ---

// Regressão: SaveProcessedBatch deve ser atômico — uma falha individual de
// inserção (aqui, violação de FK em raw_message_id inexistente) aborta o batch,
// faz rollback e retorna erro, sem persistir nenhuma das mensagens. Antes da
// correção, o batch cometia as mensagens válidas e apenas contabilizava a
// falha em `failed`, deixando estado parcialmente gravado — o reprocessamento
// via FetchUnprocessed (baseado em MAX(raw_message_id)) então "pulava" as
// mensagens já cometidas, perdendo a atômica do batch.
func TestSaveProcessedBatch_AbortsOnIndividualError_NoPartialCommit(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}
	// Apenas raw_message_id 1 e 2 existem (foreign_keys=ON): 999 viola a FK.
	mustExec(`INSERT INTO channels (id, username, title, active) VALUES (1, 'c1', 'C', 1)`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (1, 1, 101, '{}')`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (2, 1, 102, '{}')`)

	now := time.Now()
	// Mensagem inválida (raw_message_id=999) colocada NO MEIO do batch: prova
	// que nem a mensagem anterior nem a posterior são cometidas (rollback total).
	batch := []*NormalizedMessage{
		{RawMessageID: 1, ChannelID: 1, MessageID: 101, PostedAt: now},   // válida
		{RawMessageID: 999, ChannelID: 1, MessageID: 103, PostedAt: now}, // viola FK
		{RawMessageID: 2, ChannelID: 1, MessageID: 102, PostedAt: now},   // válida
	}

	saved, _, err := repo.SaveProcessedBatch(ctx, batch)

	// Falha individual deve ser surfaced como erro — nunca silenciosa.
	if err == nil {
		t.Fatal("SaveProcessedBatch: esperado erro na falha individual; got nil")
	}
	// Nenhuma mensagem persistida — atomicidade, sem commit parcial.
	if saved != 0 {
		t.Errorf("saved = %d; quer 0 (batch abortado)", saved)
	}
	var n int64
	if qerr := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_messages`).Scan(&n); qerr != nil {
		t.Fatalf("count processed_messages: %v", qerr)
	}
	if n != 0 {
		t.Errorf("processed_messages tem %d linhas; quer 0 (rollback impediu commit parcial)", n)
	}
}

// Complemento: um batch totalmente válido é cometido por inteiro (saved == N,
// failed == 0, sem erro). Trava o caminho feliz para que a mudança de
// abort-on-error não regreda o fluxo normal de processamento.
func TestSaveProcessedBatch_CommitsValidBatch(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO channels (id, username, title, active) VALUES (1, 'c1', 'C', 1)`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (1, 1, 101, '{}')`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (2, 1, 102, '{}')`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (3, 1, 103, '{}')`)

	now := time.Now()
	batch := []*NormalizedMessage{
		{RawMessageID: 1, ChannelID: 1, MessageID: 101, PostedAt: now},
		{RawMessageID: 2, ChannelID: 1, MessageID: 102, PostedAt: now},
		{RawMessageID: 3, ChannelID: 1, MessageID: 103, PostedAt: now},
	}

	saved, failed, err := repo.SaveProcessedBatch(ctx, batch)
	if err != nil {
		t.Fatalf("SaveProcessedBatch: %v", err)
	}
	if saved != 3 {
		t.Errorf("saved = %d; quer 3", saved)
	}
	if failed != 0 {
		t.Errorf("failed = %d; quer 0", failed)
	}
}

// --- Regressão: photo_cache cleanup remove apenas expirados (expires_at) ---

// ensureExpiresAtColumn adiciona a coluna expires_at a photo_cache se ainda
// não existir. A coluna é introduzida pela migration do contrato (#3); o teste
// é defensivo para rodar tanto antes quanto depois da migration.
func ensureExpiresAtColumn(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(photo_cache)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma: %v", err)
		}
		if name == "expires_at" {
			return // coluna já existe (migration aplicada)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pragma: %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE photo_cache ADD COLUMN expires_at TEXT`); err != nil {
		t.Fatalf("add column expires_at: %v", err)
	}
}

// Regressão: CleanExpiredPhotoCache deve remover APENAS entradas expiradas
// (expires_at < agora), preservando entradas ainda válidas e entradas sem
// expiração (NULL = nunca expira). Antes do contrato não havia TTL/cleanup,
// então o photo_cache crescia indefinidamente.
func TestCleanExpiredPhotoCache_RemovesOnlyExpired(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()
	ensureExpiresAtColumn(t, ctx, db.DB())

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	blob := []byte{0xde, 0xad, 0xbe, 0xef}
	// 10: expirado (passado). 20: válido (futuro). 30: sem expiração (NULL).
	exec(`INSERT INTO photo_cache (photo_id, data, expires_at) VALUES (?, ?, ?)`, 10, blob, "2020-01-01 00:00:00")
	exec(`INSERT INTO photo_cache (photo_id, data, expires_at) VALUES (?, ?, ?)`, 20, blob, "2099-01-01 00:00:00")
	exec(`INSERT INTO photo_cache (photo_id, data) VALUES (?, ?)`, 30, blob) // expires_at NULL

	deleted, err := repo.CleanExpiredPhotoCache(ctx)
	if err != nil {
		t.Fatalf("CleanExpiredPhotoCache: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d; quer 1 (apenas a entrada expirada)", deleted)
	}

	count := func(photoID int64) int64 {
		t.Helper()
		var n int64
		if qerr := db.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM photo_cache WHERE photo_id = ?`, photoID).Scan(&n); qerr != nil {
			t.Fatalf("count photo %d: %v", photoID, qerr)
		}
		return n
	}
	if count(10) != 0 {
		t.Errorf("photo 10 (expirada) permaneceu; deveria ter sido removida")
	}
	if count(20) != 1 {
		t.Errorf("photo 20 (válida) foi removida; deveria permanecer")
	}
	if count(30) != 1 {
		t.Errorf("photo 30 (sem expiração) foi removida; NULL nunca expira")
	}
}
