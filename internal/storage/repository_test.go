package storage_test

import (
	"context"
	"database/sql"
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/model"
	"github.com/limiar/collector/internal/storage"
)

// newTestRepo abre um banco de dados Tursogo novo em um diretório temporário, executa as migrações,
// e retorna um Repository pronto. Ele registra o cleanup (limpeza) automaticamente.
func newTestRepo(t *testing.T) *storage.Repository {
	t.Helper()
	repo, cleanup, err := openTempRepo()
	if err != nil {
		t.Fatalf("openTempRepo: %v", err)
	}
	t.Cleanup(cleanup)
	return repo
}

// openTempRepoWithDB é como openTempRepo mas também retorna o *sql.DB subjacente,
// para que testes possam semear tabelas (ex.: processed_messages) via SQL direto
// — útil quando nenhum método do Repository insere aquela tabela (ela é escrita
// pelo processor).
func openTempRepoWithDB() (*storage.Repository, *sql.DB, func(), error) {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "limiar-storage-")
	if err != nil {
		return nil, nil, nil, err
	}
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, nil, err
	}
	repo, err := storage.NewRepository(db.DB())
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
	return repo, db.DB(), cleanup, nil
}

func TestPhotoMetadataStats(t *testing.T) {
	repo, db, cleanup, err := openTempRepoWithDB()
	if err != nil {
		t.Fatalf("openTempRepoWithDB: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
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

// openTempRepo abre um Repository apoiado por um banco de dados novo num diretório temporário sem
// requerer *testing.T, para que seja utilizável dentro das closures de propriedades do rapid. O
// cleanup retornado fecha o repo/db e remove o diretório temporário.
func openTempRepo() (*storage.Repository, func(), error) {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "limiar-storage-")
	if err != nil {
		return nil, nil, err
	}
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, err
	}
	repo, err := storage.NewRepository(db.DB())
	if err != nil {
		_ = db.Close()
		_ = os.RemoveAll(dir)
		return nil, nil, err
	}
	cleanup := func() {
		_ = repo.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
	return repo, cleanup, nil
}

func TestOpenRunsMigrations(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "m.db")
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, table := range []string{"sessions", "peers", "channels", "raw_messages", "schema_migrations"} {
		var name string
		row := db.DB().QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		if err := row.Scan(&name); err != nil {
			t.Errorf("table %q missing after migrations: %v", table, err)
		}
	}

	// Verifica que todas as migrações foram registradas em schema_migrations.
	var count int
	if err := db.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count == 0 {
		t.Fatal("schema_migrations should have at least one entry")
	}
}

// TestMigrationTrackingIdempotent garante que reabrir o banco não re-executa
// migrações e não duplica registros em schema_migrations.
func TestMigrationTrackingIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "idem.db")

	// Primeira abertura: aplica todas as migrações.
	db1, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	var count1 int
	if err := db1.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations").Scan(&count1); err != nil {
		t.Fatalf("count #1: %v", err)
	}
	_ = db1.Close()

	// Segunda abertura: não deve duplicar registros.
	db2, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	defer func() { _ = db2.Close() }()

	var count2 int
	if err := db2.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations").Scan(&count2); err != nil {
		t.Fatalf("count #2: %v", err)
	}
	if count2 != count1 {
		t.Fatalf("schema_migrations count changed on reopen: %d → %d", count1, count2)
	}
}

// Funcionalidade: limiar-collector, Propriedade 1: Session Storage Round-Trip.
func TestProperty1SessionRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	rapid.Check(t, func(t *rapid.T) {
		data := rapid.SliceOfN(rapid.Byte(), 1, 512).Draw(t, "session")
		if err := repo.SaveSession(ctx, data); err != nil {
			t.Fatalf("SaveSession: %v", err)
		}
		got, err := repo.LoadSession(ctx)
		if err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		if string(got) != string(data) {
			t.Fatalf("round-trip mismatch: got %v want %v", got, data)
		}
	})
}

// Funcionalidade: limiar-collector, Propriedade 2: Idempotência de Autenticação (Auth Idempotence).
func TestProperty2SessionSingleRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := repo.SaveSession(ctx, []byte{byte(i)}); err != nil {
			t.Fatalf("SaveSession #%d: %v", i, err)
		}
	}
	n, err := repo.CountSessions(ctx)
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if n != 1 {
		t.Fatalf("session count = %d, want 1", n)
	}
}

func TestLoadSessionAbsentReturnsNotFound(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.LoadSession(context.Background())
	if !stderrors.Is(err, storage.ErrNoSession) {
		t.Fatalf("expected ErrNoSession, got %v", err)
	}
}

// Funcionalidade: limiar-collector, Propriedade 4: Channel CRUD Round-Trip.
func TestProperty4ChannelCRUD(t *testing.T) {
	ctx := context.Background()
	rapid.Check(t, func(t *rapid.T) {
		names := rapid.SliceOfNDistinct(
			rapid.StringMatching(`[a-z][a-z0-9_]{2,15}`),
			1, 6,
			func(s string) string { return s },
		).Draw(t, "usernames")

		r, cleanup, err := openTempRepo()
		if err != nil {
			t.Fatalf("openTempRepo: %v", err)
		}
		defer cleanup()

		for i, name := range names {
			ch := &model.Channel{ID: int64(i + 1), Username: name, Title: name, Active: true}
			if err := r.AddChannel(ctx, ch); err != nil {
				t.Fatalf("AddChannel %q: %v", name, err)
			}
		}

		listed, err := r.ListChannels(ctx)
		if err != nil {
			t.Fatalf("ListChannels: %v", err)
		}
		if len(listed) != len(names) {
			t.Fatalf("listed %d channels, want %d", len(listed), len(names))
		}
		for _, ch := range listed {
			if !ch.Active {
				t.Errorf("channel %q should be active", ch.Username)
			}
		}

		if err := r.RemoveChannel(ctx, names[0]); err != nil {
			t.Fatalf("RemoveChannel: %v", err)
		}
		after, err := r.ListChannels(ctx)
		if err != nil {
			t.Fatalf("ListChannels after remove: %v", err)
		}
		for _, ch := range after {
			if ch.Username == names[0] {
				t.Fatalf("channel %q should have been removed", names[0])
			}
		}
	})
}

func TestRemoveChannelAbsentReturnsNotFound(t *testing.T) {
	repo := newTestRepo(t)
	err := repo.RemoveChannel(context.Background(), "nonexistent")
	if !stderrors.Is(err, apperrors.ErrChannelNotFound) {
		t.Fatalf("expected ErrChannelNotFound, got %v", err)
	}
}

func TestSaveRawMessagePersistsJSON(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	ch := &model.Channel{ID: 100, Username: "promos", Title: "Promos", Active: true}
	if err := repo.AddChannel(ctx, ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	msg := &model.RawMessage{
		ChannelID:     100,
		MessageID:     5,
		Payload:       []byte(`{"_":"message","id":5}`),
		ReceivedAt:    time.Now().UTC(),
		SchemaVersion: 1,
	}
	if inserted, err := repo.SaveRawMessage(ctx, msg); err != nil {
		t.Fatalf("SaveRawMessage: %v", err)
	} else if !inserted {
		t.Fatalf("first SaveRawMessage should report inserted=true")
	}
	if inserted, err := repo.SaveRawMessage(ctx, msg); err != nil {
		t.Fatalf("SaveRawMessage (re-insert): %v", err)
	} else if inserted {
		t.Fatalf("second SaveRawMessage should report inserted=false (duplicate)")
	}

	n, err := repo.CountRawMessages(ctx)
	if err != nil {
		t.Fatalf("CountRawMessages: %v", err)
	}
	if n != 1 {
		t.Fatalf("raw_messages count = %d, want 1", n)
	}
}

func TestSaveRawMessageDedupsByChannelAndMessageID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	ch := &model.Channel{ID: 101, Username: "dup", Title: "Dup", Active: true}
	if err := repo.AddChannel(ctx, ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	msg := &model.RawMessage{ChannelID: 101, MessageID: 7, Payload: []byte(`{}`), ReceivedAt: time.Now().UTC(), SchemaVersion: 1}
	insertedFlags := make([]bool, 3)
	for i := 0; i < 3; i++ {
		inserted, err := repo.SaveRawMessage(ctx, msg)
		if err != nil {
			t.Fatalf("SaveRawMessage #%d: %v", i, err)
		}
		insertedFlags[i] = inserted
	}
	if !insertedFlags[0] {
		t.Fatalf("first SaveRawMessage should report inserted=true")
	}
	for i := 1; i < 3; i++ {
		if insertedFlags[i] {
			t.Fatalf("SaveRawMessage #%d should report inserted=false (duplicate)", i)
		}
	}
	n, err := repo.CountRawMessages(ctx)
	if err != nil {
		t.Fatalf("CountRawMessages: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected dedup to 1 row, got %d", n)
	}
}

// Funcionalidade: limiar-collector, Propriedade 7: Atualização de Metadados de Canal na Persistência.
func TestProperty7ChannelMetadataUpdate(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	ch := &model.Channel{ID: 200, Username: "deals", Title: "Deals", Active: true}
	if err := repo.AddChannel(ctx, ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	collectedAt := time.Now().UTC().Truncate(time.Second)
	if err := repo.UpdateChannelLastMessage(ctx, 200, 42, collectedAt); err != nil {
		t.Fatalf("UpdateChannelLastMessage: %v", err)
	}

	got, err := repo.GetChannel(ctx, 200)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if got.LastMessageID != 42 {
		t.Errorf("LastMessageID = %d, want 42", got.LastMessageID)
	}
	if got.LastCollectedAt.IsZero() {
		t.Error("LastCollectedAt should be set")
	}
}

func TestPeerRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	p := &model.Peer{ID: 777, AccessHash: 999, Type: "channel", Username: "x"}
	if err := repo.SavePeer(ctx, p); err != nil {
		t.Fatalf("SavePeer: %v", err)
	}
	peers, err := repo.LoadPeers(ctx)
	if err != nil {
		t.Fatalf("LoadPeers: %v", err)
	}
	if len(peers) != 1 || peers[0].ID != 777 || peers[0].AccessHash != 999 {
		t.Fatalf("peer round-trip failed: %+v", peers)
	}
}

// Funcionalidade: limiar-collector, Propriedade 15: SaveRawMessageBatch persiste múltiplas
// mensagens em transação única com flags inserted[] corretas e idempotência.
func TestSaveRawMessageBatch(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Canal necessário para FK constraint.
	ch := &model.Channel{ID: 1, Username: "batch_test", Title: "Batch", Active: true}
	if err := repo.AddChannel(ctx, ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	now := time.Now().UTC()

	t.Run("todas inseridas", func(t *testing.T) {
		msgs := []*model.RawMessage{
			{ChannelID: 1, MessageID: 1, Payload: []byte(`{"a":1}`), ReceivedAt: now, SchemaVersion: 1},
			{ChannelID: 1, MessageID: 2, Payload: []byte(`{"a":2}`), ReceivedAt: now, SchemaVersion: 1},
			{ChannelID: 1, MessageID: 3, Payload: []byte(`{"a":3}`), ReceivedAt: now, SchemaVersion: 1},
		}
		inserted, totalInserted, err := repo.SaveRawMessageBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("SaveRawMessageBatch: %v", err)
		}
		if len(inserted) != 3 {
			t.Fatalf("len(inserted) = %d; want 3", len(inserted))
		}
		for i, ins := range inserted {
			if !ins {
				t.Errorf("inserted[%d] = false; want true", i)
			}
		}
		if totalInserted != 3 {
			t.Errorf("totalInserted = %d; want 3", totalInserted)
		}
	})

	t.Run("duplicatas no batch", func(t *testing.T) {
		// Mesmo (channel_id, message_id) deve ser deduplicado.
		msgs := []*model.RawMessage{
			{ChannelID: 1, MessageID: 10, Payload: []byte(`{}`), ReceivedAt: now, SchemaVersion: 1},
			{ChannelID: 1, MessageID: 10, Payload: []byte(`{}`), ReceivedAt: now, SchemaVersion: 1},
		}
		inserted, totalInserted, err := repo.SaveRawMessageBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("SaveRawMessageBatch: %v", err)
		}
		if len(inserted) != 2 {
			t.Fatalf("len(inserted) = %d; want 2", len(inserted))
		}
		if !inserted[0] {
			t.Error("inserted[0] = false; want true (first occurrence)")
		}
		// ON CONFLICT DO NOTHING → segunda ocorrência é duplicata.
		if inserted[1] {
			t.Error("inserted[1] = true; want false (duplicate within batch)")
		}
		if totalInserted != 1 {
			t.Errorf("totalInserted = %d; want 1", totalInserted)
		}
	})

	t.Run("batch vazio", func(t *testing.T) {
		inserted, totalInserted, err := repo.SaveRawMessageBatch(ctx, nil)
		if err != nil {
			t.Fatalf("SaveRawMessageBatch(nil): %v", err)
		}
		if inserted != nil {
			t.Errorf("inserted = %v; want nil", inserted)
		}
		if totalInserted != 0 {
			t.Errorf("totalInserted = %d; want 0", totalInserted)
		}
	})

	t.Run("elemento único", func(t *testing.T) {
		msgs := []*model.RawMessage{
			{ChannelID: 1, MessageID: 20, Payload: []byte(`{"single":true}`), ReceivedAt: now, SchemaVersion: 1},
		}
		inserted, totalInserted, err := repo.SaveRawMessageBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("SaveRawMessageBatch: %v", err)
		}
		if len(inserted) != 1 || !inserted[0] {
			t.Fatalf("inserted = %v; want [true]", inserted)
		}
		if totalInserted != 1 {
			t.Errorf("totalInserted = %d; want 1", totalInserted)
		}
	})

	t.Run("idempotência cross-batch", func(t *testing.T) {
		msgs := []*model.RawMessage{
			{ChannelID: 1, MessageID: 30, Payload: []byte(`{"idem":true}`), ReceivedAt: now, SchemaVersion: 1},
		}
		// Primeiro batch: insere.
		inserted, _, err := repo.SaveRawMessageBatch(ctx, msgs)
		if err != nil || len(inserted) != 1 || !inserted[0] {
			t.Fatalf("primeiro batch: inserted=%v err=%v", inserted, err)
		}
		// Segundo batch: mesma mensagem → duplicata.
		inserted, totalInserted, err := repo.SaveRawMessageBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("segundo batch: %v", err)
		}
		if inserted[0] {
			t.Error("segundo batch: inserted[0] = true; want false (duplicate)")
		}
		if totalInserted != 0 {
			t.Errorf("segundo batch: totalInserted = %d; want 0", totalInserted)
		}
	})
}

// Funcionalidade: limiar-collector, Propriedade 16: SavePeersBatch persiste múltiplos
// peers em transação única com upsert (INSERT OR REPLACE).
func TestSavePeersBatch(t *testing.T) {
	t.Run("todas inseridas", func(t *testing.T) {
		repo := newTestRepo(t)
		ctx := context.Background()
		peers := []*model.Peer{
			{ID: 1, AccessHash: 100, Type: "channel", Username: "ch1"},
			{ID: 2, AccessHash: 200, Type: "channel", Username: "ch2"},
			{ID: 3, AccessHash: 300, Type: "user", Username: ""},
		}
		if err := repo.SavePeersBatch(ctx, peers); err != nil {
			t.Fatalf("SavePeersBatch: %v", err)
		}
		loaded, err := repo.LoadPeers(ctx)
		if err != nil {
			t.Fatalf("LoadPeers: %v", err)
		}
		if len(loaded) != 3 {
			t.Fatalf("len(loaded) = %d; want 3", len(loaded))
		}
	})

	t.Run("upsert substitui existente", func(t *testing.T) {
		repo := newTestRepo(t)
		ctx := context.Background()
		peers := []*model.Peer{
			{ID: 10, AccessHash: 999, Type: "channel", Username: "original"},
		}
		if err := repo.SavePeersBatch(ctx, peers); err != nil {
			t.Fatalf("primeiro SavePeersBatch: %v", err)
		}
		updated := []*model.Peer{
			{ID: 10, AccessHash: 111, Type: "channel", Username: "updated"},
		}
		if err := repo.SavePeersBatch(ctx, updated); err != nil {
			t.Fatalf("segundo SavePeersBatch: %v", err)
		}
		loaded, err := repo.LoadPeers(ctx)
		if err != nil {
			t.Fatalf("LoadPeers: %v", err)
		}
		if len(loaded) != 1 {
			t.Fatalf("len(loaded) = %d; want 1 (upsert, não insert extra)", len(loaded))
		}
		if loaded[0].AccessHash != 111 || loaded[0].Username != "updated" {
			t.Errorf("peer = %+v; want AccessHash=111 Username=updated", loaded[0])
		}
	})

	t.Run("batch vazio", func(t *testing.T) {
		repo := newTestRepo(t)
		ctx := context.Background()
		if err := repo.SavePeersBatch(ctx, nil); err != nil {
			t.Fatalf("SavePeersBatch(nil): %v", err)
		}
	if err := repo.SavePeersBatch(ctx, []*model.Peer{}); err != nil {
			t.Fatalf("SavePeersBatch(empty): %v", err)
		}
	})
}
