package recovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram/updates"
)

type stateStorageStub struct {
	mu sync.Mutex

	state updates.State
	found bool

	channelPts   int
	channelFound bool
	channels     map[int64]int

	readErr  error
	writeErr error

	writeCalls int
	readCalls  int

	writeEntered chan struct{}
	writeRelease chan struct{}
}

var _ updates.StateStorage = (*stateStorageStub)(nil)

func (s *stateStorageStub) GetState(context.Context, int64) (updates.State, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readCalls++
	if s.readErr != nil {
		return updates.State{}, false, s.readErr
	}
	return s.state, s.found, nil
}

func (s *stateStorageStub) SetState(context.Context, int64, updates.State) error {
	return s.write()
}

func (s *stateStorageStub) SetPts(context.Context, int64, int) error {
	return s.write()
}

func (s *stateStorageStub) SetQts(context.Context, int64, int) error {
	return s.write()
}

func (s *stateStorageStub) SetDate(context.Context, int64, int) error {
	return s.write()
}

func (s *stateStorageStub) SetSeq(context.Context, int64, int) error {
	return s.write()
}

func (s *stateStorageStub) SetDateSeq(context.Context, int64, int, int) error {
	return s.write()
}

func (s *stateStorageStub) GetChannelPts(context.Context, int64, int64) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readCalls++
	if s.readErr != nil {
		return 0, false, s.readErr
	}
	return s.channelPts, s.channelFound, nil
}

func (s *stateStorageStub) SetChannelPts(context.Context, int64, int64, int) error {
	return s.write()
}

func (s *stateStorageStub) ForEachChannels(
	ctx context.Context,
	_ int64,
	f func(context.Context, int64, int) error,
) error {
	s.mu.Lock()
	s.readCalls++
	readErr := s.readErr
	channels := make(map[int64]int, len(s.channels))
	for channelID, pts := range s.channels {
		channels[channelID] = pts
	}
	s.mu.Unlock()

	if readErr != nil {
		return readErr
	}
	for channelID, pts := range channels {
		if err := f(ctx, channelID, pts); err != nil {
			return err
		}
	}
	return nil
}

func (s *stateStorageStub) write() error {
	s.mu.Lock()
	s.writeCalls++
	entered := s.writeEntered
	release := s.writeRelease
	writeErr := s.writeErr
	s.mu.Unlock()

	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if release != nil {
		<-release
	}
	return writeErr
}

func (s *stateStorageStub) counts() (reads, writes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readCalls, s.writeCalls
}

func newGuardedStateStorageForTest(t *testing.T, inner updates.StateStorage) (*GuardedStateStorage, *DurabilityBarrier) {
	t.Helper()
	barrier := NewDurabilityBarrier()
	guarded, err := NewGuardedStateStorage(inner, barrier)
	if err != nil {
		t.Fatal(err)
	}
	return guarded, barrier
}

func TestNewGuardedStateStorageRejectsMissingDependencies(t *testing.T) {
	inner := &stateStorageStub{}
	barrier := NewDurabilityBarrier()

	tests := []struct {
		name    string
		inner   updates.StateStorage
		barrier *DurabilityBarrier
	}{
		{name: "sem storage", barrier: barrier},
		{name: "sem barrier", inner: inner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGuardedStateStorage(tt.inner, tt.barrier)
			if !errors.Is(err, ErrInvalidGuardedStateStorage) {
				t.Fatalf("erro=%v, esperado ErrInvalidGuardedStateStorage", err)
			}
			if got != nil {
				t.Fatal("dependência ausente retornou adapter")
			}
		})
	}
}

func TestGuardedStateStorageReadsPreserveStateAbsenceAndErrors(t *testing.T) {
	sentinel := errors.New("read indisponível")
	inner := &stateStorageStub{
		state: updates.State{Pts: 7, Qts: 8, Date: 9, Seq: 10},
		found: true,
	}
	guarded, barrier := newGuardedStateStorageForTest(t, inner)

	got, found, err := guarded.GetState(context.Background(), 42)
	if err != nil || !found || got != inner.state {
		t.Fatalf("state=%+v found=%v err=%v", got, found, err)
	}
	if err := barrier.Err(); err != nil {
		t.Fatalf("read bem-sucedido alterou barrier: %v", err)
	}

	inner.mu.Lock()
	inner.found = false
	inner.state = updates.State{}
	inner.mu.Unlock()

	got, found, err = guarded.GetState(context.Background(), 42)
	if err != nil || found || got != (updates.State{}) {
		t.Fatalf("ausência state=%+v found=%v err=%v", got, found, err)
	}

	inner.mu.Lock()
	inner.readErr = sentinel
	inner.mu.Unlock()

	_, found, err = guarded.GetState(context.Background(), 42)
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro=%v, esperado read error", err)
	}
	if found {
		t.Fatal("read error foi reinterpretado como state encontrado")
	}
	if err := barrier.Err(); err != nil {
		t.Fatalf("read error não deveria fechar barrier neste boundary: %v", err)
	}
}

func TestGuardedStateStorageAllWritesPassThroughBarrier(t *testing.T) {
	inner := &stateStorageStub{}
	guarded, barrier := newGuardedStateStorageForTest(t, inner)
	ctx := context.Background()

	writes := []func() error{
		func() error { return guarded.SetState(ctx, 42, updates.State{}) },
		func() error { return guarded.SetPts(ctx, 42, 1) },
		func() error { return guarded.SetQts(ctx, 42, 2) },
		func() error { return guarded.SetDate(ctx, 42, 3) },
		func() error { return guarded.SetSeq(ctx, 42, 4) },
		func() error { return guarded.SetDateSeq(ctx, 42, 5, 6) },
		func() error { return guarded.SetChannelPts(ctx, 42, 100, 7) },
	}

	for i, write := range writes {
		if err := write(); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	_, calls := inner.counts()
	if calls != len(writes) {
		t.Fatalf("writes internas=%d, esperado=%d", calls, len(writes))
	}

	cause := errors.New("Evidence indisponível")
	barrier.Close(cause)

	for i, write := range writes {
		err := write()
		if !errors.Is(err, ErrDurabilityBarrierClosed) || !errors.Is(err, cause) {
			t.Fatalf("write %d após close=%v", i, err)
		}
	}
	_, after := inner.counts()
	if after != calls {
		t.Fatalf("storage recebeu writes após close: antes=%d depois=%d", calls, after)
	}
}

func TestGuardedStateStorageWriteFailureClosesBarrierBeforeReturn(t *testing.T) {
	writeErr := errors.New("sqlite write falhou")
	inner := &stateStorageStub{writeErr: writeErr}
	guarded, barrier := newGuardedStateStorageForTest(t, inner)

	err := guarded.SetPts(context.Background(), 42, 10)
	if !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("erro=%v, esperado barrier fechada", err)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("erro=%v não preserva falha da write", err)
	}

	after := barrier.Err()
	if !errors.Is(after, ErrDurabilityBarrierClosed) || !errors.Is(after, writeErr) {
		t.Fatalf("barrier não estava fechada ao retornar: %v", after)
	}

	inner.mu.Lock()
	inner.writeErr = nil
	inner.mu.Unlock()

	if err := guarded.SetPts(context.Background(), 42, 11); !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("write posterior=%v, esperado barrier fechada", err)
	}
	_, writes := inner.counts()
	if writes != 1 {
		t.Fatalf("storage recebeu %d writes; esperado somente a write que falhou", writes)
	}
}

func TestGuardedStateStorageRejectsReadsAfterBarrierClosed(t *testing.T) {
	cause := errors.New("lifecycle encerrado")
	inner := &stateStorageStub{
		found:        true,
		state:        updates.State{Pts: 1},
		channelFound: true,
		channelPts:   2,
		channels:     map[int64]int{100: 3},
	}
	guarded, barrier := newGuardedStateStorageForTest(t, inner)
	barrier.Close(cause)

	if _, _, err := guarded.GetState(context.Background(), 42); !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("GetState=%v", err)
	}
	if _, _, err := guarded.GetChannelPts(context.Background(), 42, 100); !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("GetChannelPts=%v", err)
	}
	if err := guarded.ForEachChannels(context.Background(), 42, func(context.Context, int64, int) error {
		return nil
	}); !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("ForEachChannels=%v", err)
	}

	reads, _ := inner.counts()
	if reads != 0 {
		t.Fatalf("storage recebeu %d reads após lifecycle fechado", reads)
	}
}

func TestGuardedStateStorageWriteAndCloseAreLinearizable(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	inner := &stateStorageStub{
		writeEntered: entered,
		writeRelease: release,
	}
	guarded, barrier := newGuardedStateStorageForTest(t, inner)

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- guarded.SetPts(context.Background(), 42, 10)
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("write não entrou no storage")
	}

	closeCause := errors.New("Evidence falhou")
	closeDone := make(chan bool, 1)
	go func() {
		closeDone <- barrier.Close(closeCause)
	}()

	select {
	case <-closeDone:
		t.Fatal("Close atravessou write protegida; check+write não são linearizáveis")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	if err := <-writeDone; err != nil {
		t.Fatalf("write que linearizou antes do Close falhou: %v", err)
	}
	if won := <-closeDone; !won {
		t.Fatal("Close deveria fechar a barrier depois da write")
	}
	if err := barrier.Err(); !errors.Is(err, closeCause) {
		t.Fatalf("barrier final=%v, esperado causa externa", err)
	}
}
