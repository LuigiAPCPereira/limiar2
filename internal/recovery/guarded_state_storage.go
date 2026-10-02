package recovery

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/telegram/updates"
)

var ErrInvalidGuardedStateStorage = errors.New("recovery: GuardedStateStorage inválido")

// GuardedStateStorage protege mutações de SourceSyncState com a DurabilityBarrier.
//
// Reads não possuem side effect e não precisam compartilhar a região crítica das writes.
// Mesmo assim, uma barrier já fechada faz o adapter falhar rápido, pois o lifecycle atual
// já é terminal.
type GuardedStateStorage struct {
	inner   updates.StateStorage
	barrier *DurabilityBarrier
}

var _ updates.StateStorage = (*GuardedStateStorage)(nil)

// NewGuardedStateStorage compõe uma StateStorage física com a barrier daquele lifecycle.
func NewGuardedStateStorage(
	inner updates.StateStorage,
	barrier *DurabilityBarrier,
) (*GuardedStateStorage, error) {
	if inner == nil {
		return nil, fmt.Errorf("%w: StateStorage ausente", ErrInvalidGuardedStateStorage)
	}
	if barrier == nil {
		return nil, fmt.Errorf("%w: DurabilityBarrier ausente", ErrInvalidGuardedStateStorage)
	}
	return &GuardedStateStorage{inner: inner, barrier: barrier}, nil
}

func (s *GuardedStateStorage) GetState(
	ctx context.Context,
	userID int64,
) (updates.State, bool, error) {
	if err := s.readable(); err != nil {
		return updates.State{}, false, err
	}
	return s.inner.GetState(ctx, userID)
}

func (s *GuardedStateStorage) SetState(
	ctx context.Context,
	userID int64,
	state updates.State,
) error {
	return s.write(func() error {
		return s.inner.SetState(ctx, userID, state)
	})
}

func (s *GuardedStateStorage) SetPts(ctx context.Context, userID int64, pts int) error {
	return s.write(func() error {
		return s.inner.SetPts(ctx, userID, pts)
	})
}

func (s *GuardedStateStorage) SetQts(ctx context.Context, userID int64, qts int) error {
	return s.write(func() error {
		return s.inner.SetQts(ctx, userID, qts)
	})
}

func (s *GuardedStateStorage) SetDate(ctx context.Context, userID int64, date int) error {
	return s.write(func() error {
		return s.inner.SetDate(ctx, userID, date)
	})
}

func (s *GuardedStateStorage) SetSeq(ctx context.Context, userID int64, seq int) error {
	return s.write(func() error {
		return s.inner.SetSeq(ctx, userID, seq)
	})
}

func (s *GuardedStateStorage) SetDateSeq(
	ctx context.Context,
	userID int64,
	date, seq int,
) error {
	return s.write(func() error {
		return s.inner.SetDateSeq(ctx, userID, date, seq)
	})
}

func (s *GuardedStateStorage) GetChannelPts(
	ctx context.Context,
	userID, channelID int64,
) (int, bool, error) {
	if err := s.readable(); err != nil {
		return 0, false, err
	}
	return s.inner.GetChannelPts(ctx, userID, channelID)
}

func (s *GuardedStateStorage) SetChannelPts(
	ctx context.Context,
	userID, channelID int64,
	pts int,
) error {
	return s.write(func() error {
		return s.inner.SetChannelPts(ctx, userID, channelID, pts)
	})
}

func (s *GuardedStateStorage) ForEachChannels(
	ctx context.Context,
	userID int64,
	f func(ctx context.Context, channelID int64, pts int) error,
) error {
	if err := s.readable(); err != nil {
		return err
	}
	return s.inner.ForEachChannels(ctx, userID, f)
}

func (s *GuardedStateStorage) readable() error {
	if s == nil || s.inner == nil || s.barrier == nil {
		return fmt.Errorf("%w: adapter não inicializado", ErrInvalidGuardedStateStorage)
	}
	if err := s.barrier.Err(); err != nil {
		return fmt.Errorf("recovery: ler state após fechamento do lifecycle: %w", err)
	}
	return nil
}

func (s *GuardedStateStorage) write(operation func() error) error {
	if s == nil || s.inner == nil || s.barrier == nil {
		return fmt.Errorf("%w: adapter não inicializado", ErrInvalidGuardedStateStorage)
	}
	return s.barrier.Guard(operation)
}
