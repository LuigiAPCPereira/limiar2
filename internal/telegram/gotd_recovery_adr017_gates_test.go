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

var errStateRead = errors.New("state read failed")

type orderedRecoveryEvents struct {
	mu     sync.Mutex
	events []string
}

func (o *orderedRecoveryEvents) add(event string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)
}

func (o *orderedRecoveryEvents) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.events...)
}

type orderedChannelStorage struct {
	*contractStorage
	order *orderedRecoveryEvents
}

func (s *orderedChannelStorage) SetChannelPts(ctx context.Context, userID, channelID int64, pts int) error {
	s.order.add("state:channel")
	return s.contractStorage.SetChannelPts(ctx, userID, channelID, pts)
}

type orderedStateStorage struct {
	*contractStorage
	order     *orderedRecoveryEvents
	persisted chan updates.State
	expected  updates.State
}

func (s *orderedStateStorage) signalIfExpected() {
	if s.persisted == nil {
		return
	}
	snapshot := s.snapshot()
	if snapshot != s.expected {
		return
	}
	select {
	case s.persisted <- snapshot:
	default:
	}
}

func (s *orderedStateStorage) SetState(ctx context.Context, userID int64, state updates.State) error {
	s.order.add("state:user")
	if err := s.contractStorage.SetState(ctx, userID, state); err != nil {
		return err
	}
	s.signalIfExpected()
	return nil
}

func (s *orderedStateStorage) SetPts(ctx context.Context, userID int64, pts int) error {
	if err := s.contractStorage.SetPts(ctx, userID, pts); err != nil {
		return err
	}
	s.signalIfExpected()
	return nil
}

func (s *orderedStateStorage) SetQts(ctx context.Context, userID int64, qts int) error {
	if err := s.contractStorage.SetQts(ctx, userID, qts); err != nil {
		return err
	}
	s.signalIfExpected()
	return nil
}

func (s *orderedStateStorage) SetDate(ctx context.Context, userID int64, date int) error {
	if err := s.contractStorage.SetDate(ctx, userID, date); err != nil {
		return err
	}
	s.signalIfExpected()
	return nil
}

func (s *orderedStateStorage) SetSeq(ctx context.Context, userID int64, seq int) error {
	if err := s.contractStorage.SetSeq(ctx, userID, seq); err != nil {
		return err
	}
	s.signalIfExpected()
	return nil
}

func (s *orderedStateStorage) SetDateSeq(ctx context.Context, userID int64, date, seq int) error {
	if err := s.contractStorage.SetDateSeq(ctx, userID, date, seq); err != nil {
		return err
	}
	s.signalIfExpected()
	return nil
}

type failingStateReadStorage struct {
	*contractStorage
}

func (s *failingStateReadStorage) GetState(context.Context, int64) (updates.State, bool, error) {
	return updates.State{}, false, errStateRead
}

type stateCountingAPI struct {
	*scriptedRecoveryAPI
	mu         sync.Mutex
	stateCalls int
}

func (a *stateCountingAPI) UpdatesGetState(ctx context.Context) (*tg.UpdatesState, error) {
	a.mu.Lock()
	a.stateCalls++
	a.mu.Unlock()
	return a.scriptedRecoveryAPI.UpdatesGetState(ctx)
}

func (a *stateCountingAPI) calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stateCalls
}

func TestADR017Gate_ChannelDifferenceTooLongEvidencePrecedesStateAdvance(t *testing.T) {
	barrier := newDurabilityBarrier()
	baseStorage := newContractStorage(barrier, updates.State{})
	baseStorage.mu.Lock()
	baseStorage.channels[contractChannelID] = 10
	baseStorage.mu.Unlock()

	order := &orderedRecoveryEvents{}
	storage := &orderedChannelStorage{contractStorage: baseStorage, order: order}

	hasher := newContractAccessHasher()
	if err := hasher.SetChannelAccessHash(context.Background(), contractUserID, contractChannelID, 999); err != nil {
		t.Fatal(err)
	}

	dialog := &tg.Dialog{Peer: &tg.PeerChannel{ChannelID: contractChannelID}}
	dialog.SetPts(42)

	upstream := newScriptedRecoveryAPI()
	upstream.channelDiffs = []tg.UpdatesChannelDifferenceClass{
		&tg.UpdatesChannelDifferenceTooLong{Dialog: dialog},
	}
	api := newGuardedRecoveryAPI(upstream, barrier, func(_ context.Context, kind string) error {
		if kind != recoveryEvidenceChannelTooLong {
			t.Fatalf("evidence kind = %q, want %q", kind, recoveryEvidenceChannelTooLong)
		}
		order.add("evidence:channel-too-long")
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tooLong := make(chan struct{}, 1)
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage:      storage,
		AccessHasher: hasher,
		Handler:      gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
		OnChannelTooLong: func(channelID int64) {
			if channelID == contractChannelID {
				tooLong <- struct{}{}
			}
		},
	})

	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{})
	}()

	select {
	case <-tooLong:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("ChannelDifferenceTooLong was not processed")
	}

	baseStorage.mu.Lock()
	persistedPts := baseStorage.channels[contractChannelID]
	baseStorage.mu.Unlock()
	if persistedPts != 42 {
		cancel()
		t.Fatalf("persisted channel pts = %d, want 42", persistedPts)
	}

	events := order.snapshot()
	if len(events) < 2 || events[0] != "evidence:channel-too-long" || events[1] != "state:channel" {
		cancel()
		t.Fatalf("recovery order = %v, want Evidence before channel state", events)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("updates.Manager did not stop")
	}
}

func TestADR017Gate_ExplicitForgetPersistsEvidenceBeforeBaselineReplacement(t *testing.T) {
	barrier := newDurabilityBarrier()
	baseStorage := newContractStorage(barrier, updates.State{Pts: 7, Date: 1, Seq: 1})
	order := &orderedRecoveryEvents{}
	remoteBaseline := updates.State{Pts: 42, Date: 10, Seq: 3}
	persisted := make(chan updates.State, 1)
	storage := &orderedStateStorage{
		contractStorage: baseStorage,
		order:           order,
		persisted:       persisted,
		expected:        remoteBaseline,
	}

	upstream := newScriptedRecoveryAPI()
	upstream.remoteState = &tg.UpdatesState{Pts: remoteBaseline.Pts, Qts: remoteBaseline.Qts, Date: remoteBaseline.Date, Seq: remoteBaseline.Seq}
	api := newGuardedRecoveryAPI(upstream, barrier, func(_ context.Context, kind string) error {
		if kind != recoveryEvidenceBootstrap {
			t.Fatalf("evidence kind = %q", kind)
		}
		order.add("evidence:explicit-resync")
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})

	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{
			Forget:  true,
			OnStart: func(context.Context) { close(started) },
		})
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("explicit resync did not start")
	}

	var state updates.State
	select {
	case state = <-persisted:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("complete remote baseline was not persisted after explicit resync")
	}

	if state != remoteBaseline {
		cancel()
		t.Fatalf("persisted state = %+v, want remote baseline %+v", state, remoteBaseline)
	}

	events := order.snapshot()
	if len(events) < 2 || events[0] != "evidence:explicit-resync" || events[1] != "state:user" {
		cancel()
		t.Fatalf("resync order = %v, want Evidence before state replacement", events)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("updates.Manager did not stop")
	}
}

func TestADR017Gate_StateReadFailureNeverFallsBackToRemoteBootstrap(t *testing.T) {
	barrier := newDurabilityBarrier()
	baseStorage := newContractStorage(barrier, updates.State{Pts: 7})
	storage := &failingStateReadStorage{contractStorage: baseStorage}
	upstream := &stateCountingAPI{scriptedRecoveryAPI: newScriptedRecoveryAPI()}

	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})

	err := manager.Run(context.Background(), upstream, contractUserID, updates.AuthOptions{})
	if !errors.Is(err, errStateRead) {
		t.Fatalf("Run error = %v, want state read failure", err)
	}
	if got := upstream.calls(); got != 0 {
		t.Fatalf("UpdatesGetState calls = %d, want 0 after state read failure", got)
	}
	if got := baseStorage.snapshot().Pts; got != 7 {
		t.Fatalf("persisted pts changed to %d after state read failure", got)
	}
}
