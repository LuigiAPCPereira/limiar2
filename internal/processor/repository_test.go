package processor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limiar/collector/internal/model"
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

func TestSaveProcessed_RoundTripsSprint1ExtractionFields(t *testing.T) {
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

	postedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := &NormalizedMessage{
		RawMessageID: 1,
		ChannelID:    1,
		MessageID:    101,
		MessageType:  "deal_complete",
		Text:         "Produto R$ 99 no pix Cupom: AEBR2 ou IFPL90V1",
		PostedAt:     postedAt,
		ProcessedAt:  postedAt,
		HasCoupon:    true,
		CouponCode:   "AEBR2",
		CouponCodes: []model.Coupon{
			{Code: "AEBR2", DiscountType: "code_only"},
			{Code: "IFPL90V1", DiscountType: "code_only"},
		},
		Modifiers: []model.Modifier{
			{Type: "payment", Value: "pix"},
			{Type: "shipping", Value: "frete_gratis"},
		},
		VirtualCurrency: &model.VirtualCurrency{Platform: "shopee", Amount: 1853, Type: "discount"},
		WebpageURL:      "https://s.shopee.com.br/produto",
		WebpageTitle:    "Produto Shopee",
		WebpageDesc:     "Descrição do card",
		IsPromotional:   true,
	}
	if err := repo.SaveProcessed(ctx, msg); err != nil {
		t.Fatalf("SaveProcessed: %v", err)
	}

	got, err := repo.ListProcessedMessages(ctx, 0, "", 10, 0)
	if err != nil {
		t.Fatalf("ListProcessedMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, quer 1", len(got))
	}
	pm := got[0]
	if pm.CouponCode != "AEBR2" {
		t.Fatalf("CouponCode = %q, quer AEBR2", pm.CouponCode)
	}
	if len(pm.CouponCodes) != 2 || pm.CouponCodes[1].Code != "IFPL90V1" {
		t.Fatalf("CouponCodes = %#v, quer dois códigos estruturados", pm.CouponCodes)
	}
	if len(pm.Modifiers) != 2 || pm.Modifiers[0] != (model.Modifier{Type: "payment", Value: "pix"}) {
		t.Fatalf("Modifiers = %#v", pm.Modifiers)
	}
	if pm.VirtualCurrency == nil || pm.VirtualCurrency.Amount != 1853 || pm.VirtualCurrency.Platform != "shopee" {
		t.Fatalf("VirtualCurrency = %#v", pm.VirtualCurrency)
	}
	if pm.WebpageURL != msg.WebpageURL || pm.WebpageTitle != msg.WebpageTitle || pm.WebpageDesc != msg.WebpageDesc {
		t.Fatalf("webpage = (%q,%q,%q), quer (%q,%q,%q)", pm.WebpageURL, pm.WebpageTitle, pm.WebpageDesc, msg.WebpageURL, msg.WebpageTitle, msg.WebpageDesc)
	}
	if !pm.IsPromotional {
		t.Fatal("IsPromotional = false, quer true")
	}
}

// TestSaveProcessed_RoundTripsProductNameAndConfidence garante que SaveProcessed
// persiste ProductNameConfidence (score 0.0–1.0 do CRE do Sprint 2, ADR 014)
// junto com ProductName, e que ListProcessedMessages devolve ambos sem perda.
// O CRE exige que o score viaje do normalizer até a leitura: abaixo do
// threshold o nome vem vazio, mas o score é preservado para a Fase 3 (LLM)
// saber quando intervir.
func TestSaveProcessed_RoundTripsProductNameAndConfidence(t *testing.T) {
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

	postedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := &NormalizedMessage{
		RawMessageID:          1,
		ChannelID:             1,
		MessageID:             101,
		MessageType:           "deal_complete",
		Text:                  "Anker Caixa de Som Soundcore Select 4 go",
		PostedAt:              postedAt,
		ProcessedAt:           postedAt,
		ProductName:           "Anker Caixa de Som Soundcore Select 4 go",
		ProductNameConfidence: 0.85,
	}
	if err := repo.SaveProcessed(ctx, msg); err != nil {
		t.Fatalf("SaveProcessed: %v", err)
	}

	got, err := repo.ListProcessedMessages(ctx, 0, "", 10, 0)
	if err != nil {
		t.Fatalf("ListProcessedMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, quer 1", len(got))
	}
	pm := got[0]
	if pm.ProductName != msg.ProductName {
		t.Fatalf("ProductName = %q, quer %q", pm.ProductName, msg.ProductName)
	}
	if pm.ProductNameConfidence != msg.ProductNameConfidence {
		t.Fatalf("ProductNameConfidence = %v, quer %v", pm.ProductNameConfidence, msg.ProductNameConfidence)
	}
}

// TestSaveProcessedBatch_PreservesProductNameConfidence garante que
// SaveProcessedBatch persiste ProductNameConfidence para múltiplas mensagens de
// uma vez, sem zerar ou trocar o score entre linhas. Cada mensagem carrega seu
// próprio ProductName + Confidence — incluindo o caso abaixo do threshold
// (nome vazio, score preservado para a Fase 3).
func TestSaveProcessedBatch_PreservesProductNameConfidence(t *testing.T) {
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

	// postedAt distinto por mensagem para ordem determinística em posted_at DESC.
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	msgs := []*NormalizedMessage{
		{
			RawMessageID: 1, ChannelID: 1, MessageID: 101, MessageType: "deal_complete",
			Text: "Anker Soundcore Select 4 go", PostedAt: base.Add(2 * time.Second), ProcessedAt: base,
			ProductName: "Anker Soundcore Select 4 go", ProductNameConfidence: 0.85,
		},
		{
			RawMessageID: 2, ChannelID: 1, MessageID: 102, MessageType: "deal_complete",
			Text: "Apple iPhone 16 (128 GB)", PostedAt: base.Add(1 * time.Second), ProcessedAt: base,
			ProductName: "Apple iPhone 16 (128 GB)", ProductNameConfidence: 0.62,
		},
		{
			RawMessageID: 3, ChannelID: 1, MessageID: 103, MessageType: "commentary",
			Text: "NOVO CUPOM AMAZON", PostedAt: base, ProcessedAt: base,
			// Abaixo do threshold (0.45): nome vazio, score persistido para a Fase 3.
			ProductName: "", ProductNameConfidence: 0.28,
		},
	}
	saved, failed, err := repo.SaveProcessedBatch(ctx, msgs)
	if err != nil {
		t.Fatalf("SaveProcessedBatch: %v", err)
	}
	if saved != len(msgs) || failed != 0 {
		t.Fatalf("saved=%d failed=%d, quer saved=%d failed=0", saved, failed, len(msgs))
	}

	got, err := repo.ListProcessedMessages(ctx, 0, "", 10, 0)
	if err != nil {
		t.Fatalf("ListProcessedMessages: %v", err)
	}
	if len(got) != len(msgs) {
		t.Fatalf("len(got) = %d, quer %d", len(got), len(msgs))
	}

	// Ordem de List é posted_at DESC; mapeamos por ProductName para ficar
	// independente da ordem de retorno (cada nome é único no corpus abaixo).
	wantByName := make(map[string]float64, len(msgs))
	for _, m := range msgs {
		wantByName[m.ProductName] = m.ProductNameConfidence
	}
	for _, pm := range got {
		wantConf, ok := wantByName[pm.ProductName]
		if !ok {
			t.Fatalf("ProductName inesperado no round-trip: %q", pm.ProductName)
		}
		if pm.ProductNameConfidence != wantConf {
			t.Fatalf("ProductNameConfidence de %q = %v, quer %v", pm.ProductName, pm.ProductNameConfidence, wantConf)
		}
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
	defer func() { _ = rows.Close() }()
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
