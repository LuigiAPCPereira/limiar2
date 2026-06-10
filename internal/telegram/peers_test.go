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
		t.Errorf("esperado que Get retornasse falso para peer não existente, obteve verdadeiro")
	}
	if p != nil {
		t.Errorf("esperado que Get retornasse nil para peer não existente, obteve %+v", p)
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
		t.Fatalf("esperado que Get retornasse verdadeiro após Set, obteve falso")
	}
	if p != peer {
		t.Fatalf("esperado que o ponteiro do peer retornado seja idêntico ao ponteiro do peer definido")
	}
}
