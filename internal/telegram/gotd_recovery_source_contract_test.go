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

type scriptedDifferenceAPI struct {
	mu    sync.Mutex
	diffs []tg.UpdatesDifferenceClass
	calls chan *tg.UpdatesGetDifferenceRequest
}

func newScriptedDifferenceAPI(diffs ...tg.UpdatesDifferenceClass) *scriptedDifferenceAPI {
	return &scriptedDifferenceAPI{
		diffs: append([]tg.UpdatesDifferenceClass(nil), diffs...),
		calls: make(chan *tg.UpdatesGetDifferenceRequest, 8),
	}
}

func (a *scriptedDifferenceAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{}, nil
}

func (a *scriptedDifferenceAPI) UpdatesGetDifference(_ context.Context, request *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	a.calls <- request

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.diffs) == 0 {
		return &tg.UpdatesDifferenceEmpty{}, nil
	}
	diff := a.diffs[0]
	a.diffs = a.diffs[1:]
	return diff, nil
}

func (a *scriptedDifferenceAPI) UpdatesGetChannelDifference(context.Context, *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	return &tg.UpdatesChannelDifferenceEmpty{}, nil
}

func TestGotdRecoveryContract_CommonGapTriggersDifferenceAfterStartup(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newContractAPI()

	running := startContractManager(t, storage, api, gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		return nil
	}))
	defer running.stop(t)

	// Manager.Run always performs getDifference during startup. Drain that call
	// so the assertion below cannot mistake it for gap recovery.
	select {
	case <-api.diffCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("startup getDifference was not observed")
	}

	if err := running.manager.Handle(context.Background(), ptsUpdate(2)); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	select {
	case request := <-api.diffCalled:
		if request.Pts != 0 {
			t.Fatalf("gap getDifference pts = %d, want persisted/internal starting pts 0", request.Pts)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("common PTS gap did not trigger a second updates.getDifference")
	}
}

func TestGotdRecoveryContract_DifferenceHandlerFailureLeavesPersistedStateOld(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newScriptedDifferenceAPI(&tg.UpdatesDifference{
		NewMessages: []tg.MessageClass{&tg.MessageEmpty{ID: 1}},
		State:       tg.UpdatesState{Pts: 1, Date: 1},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	superviseBarrier(barrier, cancel)

	handled := make(chan struct{}, 1)
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
			barrier.Fail(errEvidencePersistence)
			handled <- struct{}{}
			return errEvidencePersistence
		}),
	})

	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{})
	}()

	select {
	case <-handled:
	case <-time.After(3 * time.Second):
		t.Fatal("recovered message was not dispatched")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop after Evidence failure during getDifference")
	}

	if got := storage.snapshot().Pts; got != 0 {
		t.Fatalf("persisted pts advanced to %d after failed recovered Evidence", got)
	}

	result := waitStateWrite(t, storage, "state")
	if !errors.Is(result.err, errEvidencePersistence) {
		t.Fatalf("SetState error = %v, want closed Evidence barrier", result.err)
	}
}

func TestGotdRecoveryContract_DifferenceTooLongCallbackIsTooLate(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newScriptedDifferenceAPI(
		&tg.UpdatesDifferenceTooLong{Pts: 5},
		&tg.UpdatesDifferenceEmpty{},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	superviseBarrier(barrier, cancel)

	tooLong := make(chan struct{}, 1)
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
			return nil
		}),
		OnTooLong: func() {
			// This intentionally models the tempting callback-only design. The
			// gotd source writes PTS before invoking this callback.
			barrier.Fail(errors.New("sync discontinuity observed by callback"))
			tooLong <- struct{}{}
		},
	})

	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{})
	}()

	select {
	case <-tooLong:
	case <-time.After(3 * time.Second):
		t.Fatal("OnTooLong was not called")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop after too-long callback closed the barrier")
	}

	// This is the important source-contract proof: callback-only handling is
	// too late. The manager has already persisted the remote PTS.
	if got := storage.snapshot().Pts; got != 5 {
		t.Fatalf("persisted pts = %d, want 5 to prove callback occurs after state advance", got)
	}
}
