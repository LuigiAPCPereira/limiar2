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
		t.Fatalf("expected Get to return true after Set, got false")
	}
	if p != peer {
		t.Fatalf("expected returned peer pointer to be identical to the set peer pointer")
	}
}
