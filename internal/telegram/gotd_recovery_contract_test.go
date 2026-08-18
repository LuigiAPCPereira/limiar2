package telegram_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

const contractUserID int64 = 1001

var (
	errEvidencePersistence = errors.New("evidence persistence failed")
	errStatePersistence    = errors.New("state persistence failed")
)

type durabilityBarrier struct {
	mu    sync.Mutex
	cause error
	done  chan struct{}
}

func newDurabilityBarrier() *durabilityBarrier {
	return &durabilityBarrier{done: make(chan struct{})}
}

func (b *durabilityBarrier) Fail(err error) {
	if err == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.failLocked(err)
}

func (b *durabilityBarrier) GuardStateWrite(write func() error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cause != nil {
		return b.cause
	}

	if err := write(); err != nil {
		b.failLocked(err)
		return err
	}
	return nil
}

func (b *durabilityBarrier) Cause() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cause
}

func (b *durabilityBarrier) Done() <-chan struct{} { return b.done }

func (b *durabilityBarrier) failLocked(err error) {
	if b.cause != nil {
		return
	}
	b.cause = err
	close(b.done)
}

type stateWriteResult struct {
	kind string
	err  error
}

type contractStorage struct {
	mu sync.Mutex

	barrier *durabilityBarrier
	state   updates.State
	found   bool

	channels map[int64]int
	failPts  bool
	results  chan stateWriteResult
}

func newContractStorage(barrier *durabilityBarrier, state updates.State) *contractStorage {
	return &contractStorage{
		barrier:  barrier,
		state:    state,
		found:    true,
		channels: map[int64]int{},
		results:  make(chan stateWriteResult, 32),
	}
}

func (s *contractStorage) GetState(context.Context, int64) (updates.State, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.found, nil
}

func (s *contractStorage) SetState(_ context.Context, _ int64, state updates.State) error {
	return s.write("state", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state = state
		s.found = true
		return nil
	})
}

func (s *contractStorage) SetPts(_ context.Context, _ int64, pts int) error {
	return s.write("pts", func() error {
		if s.failPts {
			return errStatePersistence
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.Pts = pts
		return nil
	})
}

func (s *contractStorage) SetQts(_ context.Context, _ int64, qts int) error {
	return s.write("qts", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.Qts = qts
		return nil
	})
}

func (s *contractStorage) SetDate(_ context.Context, _ int64, date int) error {
	return s.write("date", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.Date = date
		return nil
	})
}

func (s *contractStorage) SetSeq(_ context.Context, _ int64, seq int) error {
	return s.write("seq", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.Seq = seq
		return nil
	})
}

func (s *contractStorage) SetDateSeq(_ context.Context, _ int64, date, seq int) error {
	return s.write("date-seq", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.Date = date
		s.state.Seq = seq
		return nil
	})
}

func (s *contractStorage) GetChannelPts(_ context.Context, _, channelID int64) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pts, ok := s.channels[channelID]
	return pts, ok, nil
}

func (s *contractStorage) SetChannelPts(_ context.Context, _, channelID int64, pts int) error {
	return s.write("channel-pts", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.channels[channelID] = pts
		return nil
	})
}

func (s *contractStorage) ForEachChannels(ctx context.Context, _ int64, f func(context.Context, int64, int) error) error {
	s.mu.Lock()
	copyChannels := make(map[int64]int, len(s.channels))
	for id, pts := range s.channels {
		copyChannels[id] = pts
	}
	s.mu.Unlock()

	for id, pts := range copyChannels {
		if err := f(ctx, id, pts); err != nil {
			return err
		}
	}
	return nil
}

func (s *contractStorage) snapshot() updates.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *contractStorage) write(kind string, fn func() error) error {
	err := s.barrier.GuardStateWrite(fn)
	s.results <- stateWriteResult{kind: kind, err: err}
	return err
}

type contractAPI struct {
	diffCalled chan *tg.UpdatesGetDifferenceRequest
}

func newContractAPI() *contractAPI {
	return &contractAPI{diffCalled: make(chan *tg.UpdatesGetDifferenceRequest, 8)}
}

func (a *contractAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{}, nil
}

func (a *contractAPI) UpdatesGetDifference(_ context.Context, request *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	a.diffCalled <- request
	return &tg.UpdatesDifferenceEmpty{}, nil
}

func (a *contractAPI) UpdatesGetChannelDifference(context.Context, *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	return &tg.UpdatesChannelDifferenceEmpty{}, nil
}

type runningManager struct {
	manager *updates.Manager
	cancel  context.CancelFunc
	done    chan error
}

func startContractManager(t *testing.T, storage *contractStorage, api *contractAPI, handler gotdtelegram.UpdateHandler) runningManager {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Handler: handler,
		Storage: storage,
	})

	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{
			OnStart: func(context.Context) { close(started) },
		})
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("updates.Manager did not start")
	}

	return runningManager{manager: manager, cancel: cancel, done: done}
}

func (m runningManager) stop(t *testing.T) {
	t.Helper()
	m.cancel()
	select {
	case <-m.done:
	case <-time.After(3 * time.Second):
		t.Fatal("updates.Manager did not stop after cancellation")
	}
}

func superviseBarrier(barrier *durabilityBarrier, cancel context.CancelFunc) {
	go func() {
		<-barrier.Done()
		cancel()
	}()
}

func ptsUpdate(pts int) tg.UpdatesClass {
	return &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateDeleteMessages{
				Messages: []int{1},
				Pts:      pts,
				PtsCount: 1,
			},
		},
	}
}

func waitStateWrite(t *testing.T, storage *contractStorage, kind string) stateWriteResult {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case result := <-storage.results:
			if result.kind == kind {
				return result
			}
		case <-deadline:
			t.Fatalf("state write %q was not attempted", kind)
		}
	}
}

func TestGotdRecoveryContract_DurableEvidenceAllowsPtsAdvance(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newContractAPI()
	handled := make(chan struct{}, 1)

	running := startContractManager(t, storage, api, gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		handled <- struct{}{}
		return nil
	}))
	defer running.stop(t)

	if err := running.manager.Handle(context.Background(), ptsUpdate(1)); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(3 * time.Second):
		t.Fatal("handler was not called")
	}

	result := waitStateWrite(t, storage, "pts")
	if result.err != nil {
		t.Fatalf("SetPts failed: %v", result.err)
	}
	if got := storage.snapshot().Pts; got != 1 {
		t.Fatalf("persisted pts = %d, want 1", got)
	}
}

func TestGotdRecoveryContract_HandlerFailureBlocksPtsAdvance(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newContractAPI()
	handled := make(chan struct{}, 1)

	running := startContractManager(t, storage, api, gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		barrier.Fail(errEvidencePersistence)
		handled <- struct{}{}
		return errEvidencePersistence
	}))
	defer running.stop(t)

	if err := running.manager.Handle(context.Background(), ptsUpdate(1)); err != nil {
		t.Fatalf("Handle unexpectedly propagated handler error: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(3 * time.Second):
		t.Fatal("handler was not called")
	}

	result := waitStateWrite(t, storage, "pts")
	if !errors.Is(result.err, errEvidencePersistence) {
		t.Fatalf("SetPts error = %v, want evidence persistence failure", result.err)
	}
	if got := storage.snapshot().Pts; got != 0 {
		t.Fatalf("persisted pts advanced to %d after failed Evidence", got)
	}
}

func TestGotdRecoveryContract_StateWriteFailureTripsBarrier(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	storage.failPts = true
	api := newContractAPI()

	running := startContractManager(t, storage, api, gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		return nil
	}))
	defer running.stop(t)

	if err := running.manager.Handle(context.Background(), ptsUpdate(1)); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	result := waitStateWrite(t, storage, "pts")
	if !errors.Is(result.err, errStatePersistence) {
		t.Fatalf("SetPts error = %v, want injected state persistence failure", result.err)
	}
	if !errors.Is(barrier.Cause(), errStatePersistence) {
		t.Fatalf("barrier cause = %v, want state persistence failure", barrier.Cause())
	}
	if got := storage.snapshot().Pts; got != 0 {
		t.Fatalf("persisted pts advanced to %d after failed state write", got)
	}
}

func TestGotdRecoveryContract_BarrierCancelsManager(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newContractAPI()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
			barrier.Fail(errEvidencePersistence)
			return errEvidencePersistence
		}),
	})
	superviseBarrier(barrier, cancel)

	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{
			OnStart: func(context.Context) { close(started) },
		})
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("updates.Manager did not start")
	}

	if err := manager.Handle(context.Background(), ptsUpdate(1)); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("supervisor did not cancel updates.Manager after barrier failure")
	}
}
