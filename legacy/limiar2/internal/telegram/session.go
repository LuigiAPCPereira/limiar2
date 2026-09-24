// Package telegram é a Fachada (Facade) sobre o gotd/td (MTProto). Ele esconde
// todos os tipos do gotd/td por trás da interface TelegramClient, de modo que as
// camadas CLI e collector nunca importem o gotd/td diretamente. O armazenamento de sessão
// e de peers (pares) é implementado sobre o storage.Repository apoiado pelo Tursogo.
package telegram

import (
	"context"
	stderrors "errors"

	gotdsession "github.com/gotd/td/session"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
)

// TursoSessionStorage implementa session.Storage do gotd sobre o repositório
// Tursogo, persistindo a sessão MTProto em uma única linha (row) de sessions.
type TursoSessionStorage struct {
	repo *storage.Repository
	log  logger.Logger
}

// asserção em tempo de compilação de que satisfazemos o contrato do gotd.
var _ gotdsession.Storage = (*TursoSessionStorage)(nil)

// NewTursoSessionStorage constrói um armazenamento de sessão apoiado pelo repo.
// log pode ser nil (tratado como NopLogger).
func NewTursoSessionStorage(repo *storage.Repository, log logger.Logger) *TursoSessionStorage {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &TursoSessionStorage{repo: repo, log: log}
}

// LoadSession retorna os bytes da sessão persistida. Quando não há sessão,
// ele retorna session.ErrNotFound do gotd, o qual o fluxo de autenticação trata
// como "início do zero" (start fresh) em vez de uma falha grave (hard failure).
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

// StoreSession persiste os bytes da sessão, substituindo qualquer sessão anterior.
func (s *TursoSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	if err := s.repo.SaveSession(ctx, data); err != nil {
		return apperrors.Wrap("telegram", "store_session", err)
	}
	s.log.Debug("🔑 Sessão persistida", "bytes", len(data))
	return nil
}
