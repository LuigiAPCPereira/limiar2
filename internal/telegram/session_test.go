package telegram_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"

	gotdsession "github.com/gotd/td/session"
	"pgregory.net/rapid"

	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

func newRepo(t *testing.T) *storage.Repository {
	t.Helper()
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "limiar-tg-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	db, err := storage.Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo, err := storage.NewRepository(db.DB())
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
	})
	return repo
}

// TursoSessionStorage deve satisfazer a interface session.Storage do gotd.
var _ gotdsession.Storage = (*telegram.TursoSessionStorage)(nil)

func TestSessionStoreLoadRoundTrip(t *testing.T) {
	ss := telegram.NewTursoSessionStorage(newRepo(t), nil)
	ctx := context.Background()
	data := []byte(`{"version":1,"data":{}}`)
	if err := ss.StoreSession(ctx, data); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}
	got, err := ss.LoadSession(ctx)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("round-trip mismatch: %q != %q", got, data)
	}
}

// Armazenamento vazio deve propagar o próprio ErrNotFound do gotd para que o fluxo
// de autenticação saiba que deve começar do zero, em vez de tratar como erro grave.
func TestSessionLoadEmptyReturnsGotdNotFound(t *testing.T) {
	ss := telegram.NewTursoSessionStorage(newRepo(t), nil)
	_, err := ss.LoadSession(context.Background())
	if !stderrors.Is(err, gotdsession.ErrNotFound) {
		t.Fatalf("esperado gotd session.ErrNotFound, obteve %v", err)
	}
}

// Funcionalidade: limiar-collector, Propriedade 1: Session Storage Round-Trip (via interface do gotd).
func TestProperty1SessionRoundTripGotd(t *testing.T) {
	ss := telegram.NewTursoSessionStorage(newRepo(t), nil)
	ctx := context.Background()
	rapid.Check(t, func(t *rapid.T) {
		data := rapid.SliceOfN(rapid.Byte(), 1, 256).Draw(t, "data")
		if err := ss.StoreSession(ctx, data); err != nil {
			t.Fatalf("StoreSession: %v", err)
		}
		got, err := ss.LoadSession(ctx)
		if err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		if string(got) != string(data) {
			t.Fatalf("mismatch: %v != %v", got, data)
		}
	})
}

func TestPeerStoreGetSet(t *testing.T) {
	ps := telegram.NewPeerStore(newRepo(t), nil)
	if _, ok := ps.Get(1); ok {
		t.Fatal("armazenamento vazio deveria falhar ao buscar")
	}
	ps.Set(&storage.Peer{ID: 1, AccessHash: 42, Type: "channel"})
	got, ok := ps.Get(1)
	if !ok || got.AccessHash != 42 {
		t.Fatalf("Get após Set falhou: %+v ok=%v", got, ok)
	}
}

func TestPeerStoreFlushAndLoad(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	ps := telegram.NewPeerStore(repo, nil)
	ps.Set(&storage.Peer{ID: 10, AccessHash: 100, Type: "channel", Username: "a"})
	ps.Set(&storage.Peer{ID: 20, AccessHash: 200, Type: "user", Username: "b"})

	if err := ps.FlushToDB(ctx); err != nil {
		t.Fatalf("FlushToDB: %v", err)
	}

	// Novo armazenamento carrega o que foi persistido.
	ps2 := telegram.NewPeerStore(repo, nil)
	if err := ps2.LoadFromDB(ctx); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}
	got, ok := ps2.Get(10)
	if !ok || got.AccessHash != 100 {
		t.Fatalf("peer 10 não foi recarregado: %+v ok=%v", got, ok)
	}
}

// Leitores e escritores concorrentes não devem causar corrida (execute com -race).
func TestPeerStoreConcurrentAccess(t *testing.T) {
	ps := telegram.NewPeerStore(newRepo(t), nil)
	done := make(chan struct{})
	for w := 0; w < 4; w++ {
		go func(base int64) {
			for i := int64(0); i < 200; i++ {
				ps.Set(&storage.Peer{ID: base*1000 + i, AccessHash: i, Type: "channel"})
			}
			done <- struct{}{}
		}(int64(w))
	}
	for r := 0; r < 4; r++ {
		go func() {
			for i := 0; i < 200; i++ {
				_, _ = ps.Get(int64(i))
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
