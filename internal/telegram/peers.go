package telegram

import (
	"context"
	"sync"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
)

// PeerStore é um cache em memória de peers do Telegram protegido por um RWMutex,
// apoiado pelo repositório Tursogo para persistência entre reinicializações.
// Leituras concorrentes não bloqueiam umas às outras.
type PeerStore struct {
	mu    sync.RWMutex
	peers map[int64]*storage.Peer
	repo  *storage.Repository
	log   logger.Logger
}

// NewPeerStore constrói um armazenamento de peers vazio apoiado pelo repo. log pode ser nil
// (tratado como NopLogger).
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

// Get retorna o peer em cache para o id especificado e se ele estava presente.
func (ps *PeerStore) Get(id int64) (*storage.Peer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.peers[id]
	return p, ok
}

// Set insere ou substitui um peer no cache em memória.
func (ps *PeerStore) Set(p *storage.Peer) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers[p.ID] = p
}

// LoadFromDB substitui o cache em memória pelos peers persistidos no armazenamento.
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

// FlushToDB persiste todos os peers em cache. Ele tira um snapshot sob um read lock (bloqueio de leitura)
// para que uma gravação demorada não bloqueie leitores concorrentes.
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
