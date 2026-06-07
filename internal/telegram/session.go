// Package telegram is the Facade over gotd/td (MTProto). It hides all gotd/td
// types behind the TelegramClient interface so the CLI and collector layers
// never import gotd/td directly. Session and peer storage are implemented on
// top of the Tursogo-backed storage.Repository.
package telegram

import (
	"context"
	stderrors "errors"

	gotdsession "github.com/gotd/td/session"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
)

// TursoSessionStorage implements gotd's session.Storage on top of the Tursogo
// repository, persisting the MTProto session in the single sessions row.
type TursoSessionStorage struct {
	repo *storage.Repository
	log  logger.Logger
}

// compile-time assertion that we satisfy the gotd contract.
var _ gotdsession.Storage = (*TursoSessionStorage)(nil)

// NewTursoSessionStorage builds a session store backed by repo. log may be nil
// (treated as NopLogger).
func NewTursoSessionStorage(repo *storage.Repository, log logger.Logger) *TursoSessionStorage {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &TursoSessionStorage{repo: repo, log: log}
}

// LoadSession returns the persisted session bytes. When no session exists it
// returns gotd's session.ErrNotFound, which the auth flow treats as "start
// fresh" rather than a hard failure.
func (s *TursoSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	data, err := s.repo.LoadSession(ctx)
	if stderrors.Is(err, storage.ErrNoSession) {
		return nil, gotdsession.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.Wrap("telegram", "load_session", err)
	}
	return data, nil
}

// StoreSession persists the session bytes, replacing any prior session.
func (s *TursoSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	if err := s.repo.SaveSession(ctx, data); err != nil {
		return apperrors.Wrap("telegram", "store_session", err)
	}
	s.log.Debug("🔑 Sessão persistida", "bytes", len(data))
	return nil
}
