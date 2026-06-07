package telegram

import (
	"context"
	"sync"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
)

// PeerStore is an in-memory cache of Telegram peers guarded by an RWMutex,
// backed by the Tursogo repository for persistence across restarts. Concurrent
// reads do not block one another.
type PeerStore struct {
	mu    sync.RWMutex
	peers map[int64]*storage.Peer
	repo  *storage.Repository
	log   logger.Logger
}

// NewPeerStore builds an empty peer store backed by repo. log may be nil
// (treated as NopLogger).
func NewPeerStore(repo *storage.Repository, log logger.Logger) *PeerStore {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &PeerStore{
		peers: make(map[int64]*storage.Peer),
		repo:  repo,
		log:   log,
	}
}

// Get returns the cached peer for id and whether it was present.
func (ps *PeerStore) Get(id int64) (*storage.Peer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.peers[id]
	return p, ok
}

// Set inserts or replaces a peer in the in-memory cache.
func (ps *PeerStore) Set(p *storage.Peer) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers[p.ID] = p
}

// LoadFromDB replaces the in-memory cache with peers persisted in storage.
func (ps *PeerStore) LoadFromDB(ctx context.Context) error {
	peers, err := ps.repo.LoadPeers(ctx)
	if err != nil {
		return apperrors.Wrap("telegram", "peers_load", err)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers = make(map[int64]*storage.Peer, len(peers))
	for _, p := range peers {
		ps.peers[p.ID] = p
	}
	ps.log.Debug("👥 Peers carregados", "total", len(peers))
	return nil
}

// FlushToDB persists every cached peer. It snapshots under a read lock so a
// long write does not block concurrent readers.
func (ps *PeerStore) FlushToDB(ctx context.Context) error {
	ps.mu.RLock()
	snapshot := make([]*storage.Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		snapshot = append(snapshot, p)
	}
	ps.mu.RUnlock()

	for _, p := range snapshot {
		if err := ps.repo.SavePeer(ctx, p); err != nil {
			return apperrors.Wrap("telegram", "peers_flush", err)
		}
	}
	ps.log.Debug("💾 Peers salvos no banco", "total", len(snapshot))
	return nil
}
