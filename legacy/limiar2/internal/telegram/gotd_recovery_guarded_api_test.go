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

const contractChannelID int64 = 77

const (
	recoveryEvidenceBootstrap       = "telegram_sync_bootstrap"
	recoveryEvidenceCommonTooLong   = "telegram_common_difference_too_long"
	recoveryEvidenceChannelTooLong  = "telegram_channel_difference_too_long"
)

type scriptedRecoveryAPI struct {
	mu sync.Mutex

	remoteState  *tg.UpdatesState
	diffs        []tg.UpdatesDifferenceClass
	channelDiffs []tg.UpdatesChannelDifferenceClass

	commonCalls  chan *tg.UpdatesGetDifferenceRequest
	channelCalls chan *tg.UpdatesGetChannelDifferenceRequest
}

func newScriptedRecoveryAPI() *scriptedRecoveryAPI {
	return &scriptedRecoveryAPI{
		remoteState:  &tg.UpdatesState{},
		commonCalls:  make(chan *tg.UpdatesGetDifferenceRequest, 16),
		channelCalls: make(chan *tg.UpdatesGetChannelDifferenceRequest, 16),
	}
}

func (a *scriptedRecoveryAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := *a.remoteState
	return &state, nil
}

func (a *scriptedRecoveryAPI) UpdatesGetDifference(_ context.Context, request *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	a.commonCalls <- request

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.diffs) == 0 {
		return &tg.UpdatesDifferenceEmpty{}, nil
	}
	diff := a.diffs[0]
	a.diffs = a.diffs[1:]
	return diff, nil
}

func (a *scriptedRecoveryAPI) UpdatesGetChannelDifference(_ context.Context, request *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	a.channelCalls <- request

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.channelDiffs) == 0 {
		return &tg.UpdatesChannelDifferenceEmpty{}, nil
	}
	diff := a.channelDiffs[0]
	a.channelDiffs = a.channelDiffs[1:]
	return diff, nil
}

type guardedRecoveryAPI struct {
	upstream updates.API
	barrier  *durabilityBarrier
	persist  func(context.Context, string) error
}

func newGuardedRecoveryAPI(upstream updates.API, barrier *durabilityBarrier, persist func(context.Context, string) error) *guardedRecoveryAPI {
	if persist == nil {
		persist = func(context.Context, string) error { return nil }
	}
	return &guardedRecoveryAPI{
		upstream: upstream,
		barrier:  barrier,
		persist:  persist,
	}
}

func (a *guardedRecoveryAPI) UpdatesGetState(ctx context.Context) (*tg.UpdatesState, error) {
	state, err := a.upstream.UpdatesGetState(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.record(ctx, recoveryEvidenceBootstrap); err != nil {
		return nil, err
	}
	return state, nil
}

func (a *guardedRecoveryAPI) UpdatesGetDifference(ctx context.Context, request *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	diff, err := a.upstream.UpdatesGetDifference(ctx, request)
	if err != nil {
		return nil, err
	}
	if _, ok := diff.(*tg.UpdatesDifferenceTooLong); ok {
		if err := a.record(ctx, recoveryEvidenceCommonTooLong); err != nil {
			return nil, err
		}
	}
	return diff, nil
}

func (a *guardedRecoveryAPI) UpdatesGetChannelDifference(ctx context.Context, request *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	diff, err := a.upstream.UpdatesGetChannelDifference(ctx, request)
	if err != nil {
		return nil, err
	}
	if _, ok := diff.(*tg.UpdatesChannelDifferenceTooLong); ok {
		if err := a.record(ctx, recoveryEvidenceChannelTooLong); err != nil {
			return nil, err
		}
	}
	return diff, nil
}

func (a *guardedRecoveryAPI) record(ctx context.Context, kind string) error {
	if err := a.persist(ctx, kind); err != nil {
		a.barrier.Fail(err)
		return err
	}
	return nil
}

type contractAccessHasher struct {
	mu     sync.Mutex
	hashes map[int64]int64
}

func newContractAccessHasher() *contractAccessHasher {
	return &contractAccessHasher{hashes: map[int64]int64{}}
}

func (h *contractAccessHasher) SetChannelAccessHash(_ context.Context, _ int64, channelID, accessHash int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hashes[channelID] = accessHash
	return nil
}

func (h *contractAccessHasher) GetChannelAccessHash(_ context.Context, _ int64, channelID int64) (int64, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hash, ok := h.hashes[channelID]
	return hash, ok, nil
}

func TestGuardedRecoveryAPI_CommonTooLongEvidenceFailureBlocksResponse(t *testing.T) {
	barrier := newDurabilityBarrier()
	upstream := newScriptedRecoveryAPI()
	upstream.diffs = []tg.UpdatesDifferenceClass{&tg.UpdatesDifferenceTooLong{Pts: 5}}
	api := newGuardedRecoveryAPI(upstream, barrier, func(_ context.Context, kind string) error {
		if kind != recoveryEvidenceCommonTooLong {
			t.Fatalf("evidence kind = %q", kind)
		}
		return errEvidencePersistence
	})

	diff, err := api.UpdatesGetDifference(context.Background(), &tg.UpdatesGetDifferenceRequest{})
	if !errors.Is(err, errEvidencePersistence) {
		t.Fatalf("error = %v, want Evidence failure", err)
	}
	if diff != nil {
		t.Fatalf("too-long response escaped guard after Evidence failure: %T", diff)
	}
	if !errors.Is(barrier.Cause(), errEvidencePersistence) {
		t.Fatalf("barrier cause = %v", barrier.Cause())
	}
}

func TestGuardedRecoveryAPI_ChannelTooLongEvidenceFailureBlocksResponse(t *testing.T) {
	barrier := newDurabilityBarrier()
	upstream := newScriptedRecoveryAPI()
	upstream.channelDiffs = []tg.UpdatesChannelDifferenceClass{&tg.UpdatesChannelDifferenceTooLong{}}
	api := newGuardedRecoveryAPI(upstream, barrier, func(_ context.Context, kind string) error {
		if kind != recoveryEvidenceChannelTooLong {
			t.Fatalf("evidence kind = %q", kind)
		}
		return errEvidencePersistence
	})

	diff, err := api.UpdatesGetChannelDifference(context.Background(), &tg.UpdatesGetChannelDifferenceRequest{})
	if !errors.Is(err, errEvidencePersistence) {
		t.Fatalf("error = %v, want Evidence failure", err)
	}
	if diff != nil {
		t.Fatalf("channel too-long response escaped guard after Evidence failure: %T", diff)
	}
}

func TestGuardedRecoveryAPI_BootstrapEvidenceFailurePreventsStateAdoption(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	storage.mu.Lock()
	storage.found = false
	storage.mu.Unlock()

	upstream := newScriptedRecoveryAPI()
	upstream.remoteState = &tg.UpdatesState{Pts: 42, Date: 10, Seq: 3}
	api := newGuardedRecoveryAPI(upstream, barrier, func(_ context.Context, kind string) error {
		if kind != recoveryEvidenceBootstrap {
			t.Fatalf("evidence kind = %q", kind)
		}
		return errEvidencePersistence
	})

	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})

	err := manager.Run(context.Background(), api, contractUserID, updates.AuthOptions{})
	if !errors.Is(err, errEvidencePersistence) {
		t.Fatalf("Run error = %v, want Evidence failure", err)
	}
	if got := storage.snapshot().Pts; got != 0 {
		t.Fatalf("bootstrap state advanced to pts=%d after Evidence failure", got)
	}
}

func TestGotdRecoveryContract_ChannelTooLongTriggersChannelDifferenceAfterStartup(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	storage.mu.Lock()
	storage.channels[contractChannelID] = 10
	storage.mu.Unlock()

	hasher := newContractAccessHasher()
	if err := hasher.SetChannelAccessHash(context.Background(), contractUserID, contractChannelID, 999); err != nil {
		t.Fatal(err)
	}

	api := newScriptedRecoveryAPI()
	api.channelDiffs = []tg.UpdatesChannelDifferenceClass{
		&tg.UpdatesChannelDifferenceEmpty{Pts: 10},
		&tg.UpdatesChannelDifferenceEmpty{Pts: 10},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage:      storage,
		AccessHasher: hasher,
		Handler:      gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})

	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{
			OnStart: func(context.Context) { close(started) },
		})
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("updates.Manager did not start")
	}

	select {
	case <-api.channelCalls:
	case <-time.After(3 * time.Second):
		t.Fatal("startup getChannelDifference was not observed")
	}

	if err := manager.Handle(context.Background(), &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateChannelTooLong{ChannelID: contractChannelID}},
	}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	select {
	case <-api.channelCalls:
	case <-time.After(3 * time.Second):
		t.Fatal("UpdateChannelTooLong did not trigger another getChannelDifference")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("updates.Manager did not stop")
	}
}

func TestGotdRecoveryContract_RestartReplaysFromPersistedOldState(t *testing.T) {
	firstBarrier := newDurabilityBarrier()
	firstStorage := newContractStorage(firstBarrier, updates.State{})
	firstAPI := newContractAPI()
	firstHandled := make(chan struct{}, 1)

	first := startContractManager(t, firstStorage, firstAPI, gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		firstBarrier.Fail(errEvidencePersistence)
		firstHandled <- struct{}{}
		return errEvidencePersistence
	}))

	// Drain startup recovery so the first live update is the operation under test.
	select {
	case <-firstAPI.diffCalled:
	case <-time.After(3 * time.Second):
		first.stop(t)
		t.Fatal("startup getDifference was not observed")
	}

	if err := first.manager.Handle(context.Background(), ptsUpdate(1)); err != nil {
		first.stop(t)
		t.Fatalf("Handle returned error: %v", err)
	}
	select {
	case <-firstHandled:
	case <-time.After(3 * time.Second):
		first.stop(t)
		t.Fatal("first attempt did not reach handler")
	}
	firstWrite := waitStateWrite(t, firstStorage, "pts")
	if !errors.Is(firstWrite.err, errEvidencePersistence) {
		first.stop(t)
		t.Fatalf("first SetPts error = %v", firstWrite.err)
	}
	first.stop(t)

	oldState := firstStorage.snapshot()
	if oldState.Pts != 0 {
		t.Fatalf("persisted state advanced before restart: %+v", oldState)
	}

	secondBarrier := newDurabilityBarrier()
	secondStorage := newContractStorage(secondBarrier, oldState)
	secondAPI := newScriptedDifferenceAPI(&tg.UpdatesDifference{
		NewMessages: []tg.MessageClass{&tg.MessageEmpty{ID: 1}},
		State:       tg.UpdatesState{Pts: 1, Date: 1},
	})

	replayedPts := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage: secondStorage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(_ context.Context, u tg.UpdatesClass) error {
			updatesEnvelope, ok := u.(*tg.Updates)
			if !ok || len(updatesEnvelope.Updates) == 0 {
				return nil
			}
			if msg, ok := updatesEnvelope.Updates[0].(*tg.UpdateNewMessage); ok {
				replayedPts <- msg.Pts
			}
			return nil
		}),
	})

	go func() {
		done <- manager.Run(ctx, secondAPI, contractUserID, updates.AuthOptions{})
	}()

	select {
	case pts := <-replayedPts:
		if pts != -1 {
			cancel()
			t.Fatalf("recovered message pts = %d, want stateless -1", pts)
		}
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("restart did not replay recovered message")
	}

	stateWrite := waitStateWrite(t, secondStorage, "state")
	if stateWrite.err != nil {
		cancel()
		t.Fatalf("replayed SetState failed: %v", stateWrite.err)
	}
	if got := secondStorage.snapshot().Pts; got != 1 {
		cancel()
		t.Fatalf("persisted pts after replay = %d, want 1", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("restarted manager did not stop")
	}
}
