package storage_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/storage"
)

// newTestRepo opens a fresh Tursogo database in a temp dir, runs migrations,
// and returns a ready Repository. It registers cleanup automatically.
func newTestRepo(t *testing.T) *storage.Repository {
	t.Helper()
	repo, cleanup, err := openTempRepo()
	if err != nil {
		t.Fatalf("openTempRepo: %v", err)
	}
	t.Cleanup(cleanup)
	return repo
}

// openTempRepo opens a Repository backed by a fresh temp-dir database without
// requiring *testing.T, so it is usable inside rapid property closures. The
// returned cleanup closes the repo/db and removes the temp dir.
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
	repo, err := storage.NewRepository(db.Conn())
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
	defer db.Close()

	for _, table := range []string{"sessions", "peers", "channels", "raw_messages"} {
		var name string
		row := db.Conn().QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		if err := row.Scan(&name); err != nil {
			t.Errorf("table %q missing after migrations: %v", table, err)
		}
	}
}

// Feature: limiar-collector, Property 1: Session Storage Round-Trip.
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

// Feature: limiar-collector, Property 2: Auth Idempotence.
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

// Feature: limiar-collector, Property 4: Channel CRUD Round-Trip.
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
			ch := &storage.Channel{ID: int64(i + 1), Username: name, Title: name, Active: true}
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

	ch := &storage.Channel{ID: 100, Username: "promos", Title: "Promos", Active: true}
	if err := repo.AddChannel(ctx, ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	msg := &storage.RawMessage{
		ChannelID:     100,
		MessageID:     5,
		Payload:       []byte(`{"_":"message","id":5}`),
		ReceivedAt:    time.Now().UTC(),
		SchemaVersion: 1,
	}
	if err := repo.SaveRawMessage(ctx, msg); err != nil {
		t.Fatalf("SaveRawMessage: %v", err)
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
	ch := &storage.Channel{ID: 101, Username: "dup", Title: "Dup", Active: true}
	if err := repo.AddChannel(ctx, ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	msg := &storage.RawMessage{ChannelID: 101, MessageID: 7, Payload: []byte(`{}`), ReceivedAt: time.Now().UTC(), SchemaVersion: 1}
	for i := 0; i < 3; i++ {
		if err := repo.SaveRawMessage(ctx, msg); err != nil {
			t.Fatalf("SaveRawMessage #%d: %v", i, err)
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

// Feature: limiar-collector, Property 7: Channel Metadata Update on Persist.
func TestProperty7ChannelMetadataUpdate(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	ch := &storage.Channel{ID: 200, Username: "deals", Title: "Deals", Active: true}
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
	p := &storage.Peer{ID: 777, AccessHash: 999, Type: "channel", Username: "x"}
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
