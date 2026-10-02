// Package syncstate implementa a capability física de SourceSyncState do Limiar 3
// conforme a L3 ADR 007. O adapter é scoped por Acquisition Subscription e satisfaz
// diretamente o contrato updates.StateStorage do gotd sem expor *sql.DB.
package syncstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/gotd/td/telegram/updates"
)

var (
	ErrInvalidStorage        = errors.New("source sync state: storage inválido")
	ErrInvalidSubscriptionID = errors.New("source sync state: subscription_id inválido")
	ErrStateNotFound         = errors.New("source sync state: common state ausente")
)

// Storage persiste a continuidade live de uma única Acquisition Subscription.
type Storage struct {
	db             *sql.DB
	subscriptionID string
}

var _ updates.StateStorage = (*Storage)(nil)

// New cria uma capability scoped por subscription_id.
func New(db *sql.DB, subscriptionID string) (*Storage, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: conexão ausente", ErrInvalidStorage)
	}
	if err := validateSubscriptionID(subscriptionID); err != nil {
		return nil, err
	}
	return &Storage{db: db, subscriptionID: subscriptionID}, nil
}

func validateSubscriptionID(id string) error {
	trimmed := strings.TrimSpace(id)
	switch {
	case trimmed == "":
		return fmt.Errorf("%w: valor vazio", ErrInvalidSubscriptionID)
	case trimmed != id:
		return fmt.Errorf("%w: whitespace nas bordas", ErrInvalidSubscriptionID)
	default:
		return nil
	}
}

// GetState lê o common state da partition subscription + user.
// Ausência é found=false; falha física permanece erro.
func (s *Storage) GetState(ctx context.Context, userID int64) (updates.State, bool, error) {
	if err := s.validate(); err != nil {
		return updates.State{}, false, err
	}
	var state updates.State
	err := s.db.QueryRowContext(ctx,
		"SELECT pts, qts, date, seq FROM source_sync_state WHERE subscription_id = ? AND user_id = ?",
		s.subscriptionID, userID,
	).Scan(&state.Pts, &state.Qts, &state.Date, &state.Seq)
	if err == sql.ErrNoRows {
		return updates.State{}, false, nil
	}
	if err != nil {
		return updates.State{}, false, fmt.Errorf("source sync state: ler common state: %w", err)
	}
	return state, true, nil
}

// SetState cria ou substitui atomicamente o common state completo.
func (s *Storage) SetState(ctx context.Context, userID int64, state updates.State) error {
	if err := s.validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO source_sync_state(subscription_id, user_id, pts, qts, date, seq) "+
			"VALUES (?, ?, ?, ?, ?, ?) "+
			"ON CONFLICT(subscription_id, user_id) DO UPDATE SET "+
			"pts = excluded.pts, qts = excluded.qts, date = excluded.date, seq = excluded.seq",
		s.subscriptionID, userID, state.Pts, state.Qts, state.Date, state.Seq)
	if err != nil {
		return fmt.Errorf("source sync state: gravar common state completo: %w", err)
	}
	return nil
}

func (s *Storage) SetPts(ctx context.Context, userID int64, pts int) error {
	return s.updateCommon(ctx, userID,
		"UPDATE source_sync_state SET pts = ? WHERE subscription_id = ? AND user_id = ?",
		pts, s.subscriptionID, userID)
}

func (s *Storage) SetQts(ctx context.Context, userID int64, qts int) error {
	return s.updateCommon(ctx, userID,
		"UPDATE source_sync_state SET qts = ? WHERE subscription_id = ? AND user_id = ?",
		qts, s.subscriptionID, userID)
}

func (s *Storage) SetDate(ctx context.Context, userID int64, date int) error {
	return s.updateCommon(ctx, userID,
		"UPDATE source_sync_state SET date = ? WHERE subscription_id = ? AND user_id = ?",
		date, s.subscriptionID, userID)
}

func (s *Storage) SetSeq(ctx context.Context, userID int64, seq int) error {
	return s.updateCommon(ctx, userID,
		"UPDATE source_sync_state SET seq = ? WHERE subscription_id = ? AND user_id = ?",
		seq, s.subscriptionID, userID)
}

func (s *Storage) SetDateSeq(ctx context.Context, userID int64, date, seq int) error {
	return s.updateCommon(ctx, userID,
		"UPDATE source_sync_state SET date = ?, seq = ? WHERE subscription_id = ? AND user_id = ?",
		date, seq, s.subscriptionID, userID)
}

func (s *Storage) updateCommon(ctx context.Context, userID int64, query string, args ...any) error {
	if err := s.validate(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("source sync state: atualizar common state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("source sync state: verificar cardinalidade do common state: %w", err)
	}
	switch affected {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("%w: subscription_id=%q user_id=%d", ErrStateNotFound, s.subscriptionID, userID)
	default:
		return fmt.Errorf("source sync state: cardinalidade inesperada no common state: %d linhas", affected)
	}
}

// GetChannelPts preserva a distinção entre ausência e pts=0.
func (s *Storage) GetChannelPts(ctx context.Context, userID, channelID int64) (int, bool, error) {
	if err := s.validate(); err != nil {
		return 0, false, err
	}
	var pts int
	err := s.db.QueryRowContext(ctx,
		"SELECT pts FROM source_sync_channel_state WHERE subscription_id = ? AND user_id = ? AND channel_id = ?",
		s.subscriptionID, userID, channelID,
	).Scan(&pts)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("source sync state: ler channel pts: %w", err)
	}
	return pts, true, nil
}

// SetChannelPts cria ou substitui o state completo de um canal.
func (s *Storage) SetChannelPts(ctx context.Context, userID, channelID int64, pts int) error {
	if err := s.validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO source_sync_channel_state(subscription_id, user_id, channel_id, pts) "+
			"VALUES (?, ?, ?, ?) "+
			"ON CONFLICT(subscription_id, user_id, channel_id) DO UPDATE SET pts = excluded.pts",
		s.subscriptionID, userID, channelID, pts)
	if err != nil {
		return fmt.Errorf("source sync state: gravar channel pts: %w", err)
	}
	return nil
}

type channelState struct {
	channelID int64
	pts       int
}

// ForEachChannels enumera somente a partition desta subscription e do userID recebido.
// O cursor SQL é fechado antes dos callbacks para não reter a única conexão lógica do
// Store durante código externo potencialmente reentrante.
func (s *Storage) ForEachChannels(
	ctx context.Context,
	userID int64,
	f func(ctx context.Context, channelID int64, pts int) error,
) error {
	if err := s.validate(); err != nil {
		return err
	}
	if f == nil {
		return fmt.Errorf("%w: callback de channels ausente", ErrInvalidStorage)
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT channel_id, pts FROM source_sync_channel_state "+
			"WHERE subscription_id = ? AND user_id = ? ORDER BY channel_id",
		s.subscriptionID, userID)
	if err != nil {
		return fmt.Errorf("source sync state: enumerar channels: %w", err)
	}

	channels := make([]channelState, 0)
	for rows.Next() {
		var item channelState
		if err := rows.Scan(&item.channelID, &item.pts); err != nil {
			_ = rows.Close()
			return fmt.Errorf("source sync state: ler channel enumerado: %w", err)
		}
		channels = append(channels, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("source sync state: finalizar enumeração de channels: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("source sync state: fechar enumeração de channels: %w", err)
	}

	for _, item := range channels {
		if err := f(ctx, item.channelID, item.pts); err != nil {
			return fmt.Errorf("source sync state: callback do channel %d: %w", item.channelID, err)
		}
	}
	return nil
}

func (s *Storage) validate() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("%w: capability não inicializada", ErrInvalidStorage)
	}
	return nil
}
