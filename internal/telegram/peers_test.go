package telegram_test

import (
	"testing"
	"time"

	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

func TestPeerStore_Get(t *testing.T) {
	ps := telegram.NewPeerStore(nil, nil)

	p, ok := ps.Get(123)
	if ok {
		t.Errorf("expected Get to return false for non-existent peer, got true")
	}
	if p != nil {
		t.Errorf("expected Get to return nil for non-existent peer, got %+v", p)
	}
}

func TestPeerStore_Set(t *testing.T) {
	ps := telegram.NewPeerStore(nil, nil)

	peer := &storage.Peer{
		ID:         123,
		AccessHash: 456,
		Type:       "user",
		Username:   "testuser",
		UpdatedAt:  time.Now(),
	}

	ps.Set(peer)

	p, ok := ps.Get(123)
	if !ok {
		t.Errorf("expected Get to return true after Set, got false")
	}
	if p == nil {
		t.Fatalf("expected Get to return non-nil peer, got nil")
	}

	if p.ID != peer.ID {
		t.Errorf("expected ID %d, got %d", peer.ID, p.ID)
	}
	if p.AccessHash != peer.AccessHash {
		t.Errorf("expected AccessHash %d, got %d", peer.AccessHash, p.AccessHash)
	}
	if p.Type != peer.Type {
		t.Errorf("expected Type %q, got %q", peer.Type, p.Type)
	}
	if p.Username != peer.Username {
		t.Errorf("expected Username %q, got %q", peer.Username, p.Username)
	}
	if p.UpdatedAt != peer.UpdatedAt {
		t.Errorf("expected UpdatedAt %v, got %v", peer.UpdatedAt, p.UpdatedAt)
	}
}
