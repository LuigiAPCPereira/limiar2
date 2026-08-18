package telegram_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

const (
	recoveryEvidenceDifference        = "telegram_get_difference_response"
	recoveryEvidenceChannelDifference = "telegram_get_channel_difference_response"
)

type sourceAdmissionHandler struct {
	barrier *durabilityBarrier
	persist func(context.Context, []byte) error
	next    gotdtelegram.UpdateHandler
}

func (h *sourceAdmissionHandler) Handle(ctx context.Context, u tg.UpdatesClass) error {
	payload, err := json.Marshal(u)
	if err != nil {
		h.barrier.Fail(err)
		return err
	}
	if err := h.persist(ctx, payload); err != nil {
		h.barrier.Fail(err)
		return err
	}
	return h.next.Handle(ctx, u)
}

type orderedPtsStorage struct {
	*contractStorage
	order *orderedRecoveryEvents
}

func (s *orderedPtsStorage) SetPts(ctx context.Context, userID int64, pts int) error {
	s.order.add("state:pts")
	return s.contractStorage.SetPts(ctx, userID, pts)
}

type sourcePreservingRecoveryAPI struct {
	upstream updates.API
	barrier  *durabilityBarrier
	persist  func(context.Context, string, []byte) error
}

func newSourcePreservingRecoveryAPI(upstream updates.API, barrier *durabilityBarrier, persist func(context.Context, string, []byte) error) *sourcePreservingRecoveryAPI {
	return &sourcePreservingRecoveryAPI{upstream: upstream, barrier: barrier, persist: persist}
}

func (a *sourcePreservingRecoveryAPI) UpdatesGetState(ctx context.Context) (*tg.UpdatesState, error) {
	state, err := a.upstream.UpdatesGetState(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		a.barrier.Fail(err)
		return nil, err
	}
	if err := a.persist(ctx, recoveryEvidenceBootstrap, payload); err != nil {
		a.barrier.Fail(err)
		return nil, err
	}
	return state, nil
}

func (a *sourcePreservingRecoveryAPI) UpdatesGetDifference(ctx context.Context, req *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	diff, err := a.upstream.UpdatesGetDifference(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := a.record(ctx, recoveryEvidenceDifference, diff); err != nil {
		return nil, err
	}
	return diff, nil
}

func (a *sourcePreservingRecoveryAPI) UpdatesGetChannelDifference(ctx context.Context, req *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	diff, err := a.upstream.UpdatesGetChannelDifference(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := a.record(ctx, recoveryEvidenceChannelDifference, diff); err != nil {
		return nil, err
	}
	return diff, nil
}

func (a *sourcePreservingRecoveryAPI) record(ctx context.Context, kind string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		a.barrier.Fail(err)
		return err
	}
	if err := a.persist(ctx, kind, payload); err != nil {
		a.barrier.Fail(err)
		return err
	}
	return nil
}

func TestSourceAdmission_LiveCompoundEnvelopeIsDurableBeforeManagerStateAdvance(t *testing.T) {
	barrier := newDurabilityBarrier()
	baseStorage := newContractStorage(barrier, updates.State{})
	order := &orderedRecoveryEvents{}
	storage := &orderedPtsStorage{contractStorage: baseStorage, order: order}
	api := newContractAPI()

	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{OnStart: func(context.Context) { close(started) }})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not start")
	}
	select {
	case <-api.diffCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("startup recovery not observed")
	}

	original := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateDeleteMessages{Messages: []int{777, 778}, Pts: 2, PtsCount: 1},
			&tg.UpdateEditMessage{Message: &tg.Message{ID: 777, Message: "edited"}, Pts: 1, PtsCount: 1},
		},
		Date: 123456,
		Seq:  0,
	}
	wantPayload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var persisted []byte
	admission := &sourceAdmissionHandler{
		barrier: barrier,
		next:    manager,
		persist: func(_ context.Context, payload []byte) error {
			persisted = append([]byte(nil), payload...)
			order.add("evidence:live-envelope")
			return nil
		},
	}

	if err := admission.Handle(context.Background(), original); err != nil {
		t.Fatalf("source admission: %v", err)
	}
	if !bytes.Equal(persisted, wantPayload) {
		t.Fatalf("persisted source envelope differs from pre-manager encoding")
	}
	result := waitStateWrite(t, baseStorage, "pts")
	if result.err != nil {
		t.Fatalf("SetPts: %v", result.err)
	}
	if got := baseStorage.snapshot().Pts; got != 2 {
		t.Fatalf("persisted pts=%d, want 2", got)
	}
	events := order.snapshot()
	if len(events) < 2 || events[0] != "evidence:live-envelope" || events[1] != "state:pts" {
		t.Fatalf("order=%v, want live Evidence before state", events)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop")
	}
}

func TestSourceAdmission_FailurePreventsManagerStateAdvance(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})
	api := newContractAPI()
	running := startContractManager(t, storage, api, gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }))
	defer running.stop(t)
	manager = running.manager
	select {
	case <-api.diffCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("startup recovery not observed")
	}

	admission := &sourceAdmissionHandler{
		barrier: barrier,
		next:    manager,
		persist: func(context.Context, []byte) error { return errEvidencePersistence },
	}
	input := &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateDeleteMessages{Messages: []int{1}, Pts: 1, PtsCount: 1}}}
	if err := admission.Handle(context.Background(), input); !errors.Is(err, errEvidencePersistence) {
		t.Fatalf("admission error=%v", err)
	}
	if got := storage.snapshot().Pts; got != 0 {
		t.Fatalf("state advanced to %d after admission failure", got)
	}
	select {
	case result := <-storage.results:
		t.Fatalf("unexpected state write after failed admission: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRecoveryAdmission_CommonDifferenceEvidencePrecedesStateAdvance(t *testing.T) {
	barrier := newDurabilityBarrier()
	baseStorage := newContractStorage(barrier, updates.State{})
	order := &orderedRecoveryEvents{}
	storage := &orderedStateStorage{contractStorage: baseStorage, order: order}
	upstream := newScriptedDifferenceAPI(&tg.UpdatesDifference{
		NewMessages: []tg.MessageClass{&tg.MessageEmpty{ID: 1}},
		State:       tg.UpdatesState{Pts: 1, Date: 1},
	})
	api := newSourcePreservingRecoveryAPI(upstream, barrier, func(_ context.Context, kind string, payload []byte) error {
		if kind != recoveryEvidenceDifference {
			t.Fatalf("kind=%q", kind)
		}
		if len(payload) == 0 {
			t.Fatal("empty recovery Evidence")
		}
		order.add("evidence:common-difference")
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
			// Ordered output is allowed to fail: source/recovery Evidence is
			// already durable and derived state can be rebuilt later.
			return errors.New("projection failed")
		}),
	})
	go func() { done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{}) }()

	result := waitStateWrite(t, baseStorage, "state")
	if result.err != nil {
		t.Fatalf("SetState: %v", result.err)
	}
	if got := baseStorage.snapshot().Pts; got != 1 {
		t.Fatalf("persisted pts=%d, want 1", got)
	}
	events := order.snapshot()
	if len(events) < 2 || events[0] != "evidence:common-difference" || events[1] != "state:user" {
		t.Fatalf("order=%v, want recovery Evidence before state", events)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop")
	}
}

func TestRecoveryAdmission_FailureBlocksDifferenceResponseAndStateAdvance(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	upstream := newScriptedDifferenceAPI(&tg.UpdatesDifference{State: tg.UpdatesState{Pts: 1, Date: 1}})
	api := newSourcePreservingRecoveryAPI(upstream, barrier, func(context.Context, string, []byte) error {
		return errEvidencePersistence
	})

	diff, err := api.UpdatesGetDifference(context.Background(), &tg.UpdatesGetDifferenceRequest{})
	if !errors.Is(err, errEvidencePersistence) {
		t.Fatalf("error=%v, want Evidence persistence failure", err)
	}
	if diff != nil {
		t.Fatalf("recovery response escaped after Evidence failure: %T", diff)
	}
	if !errors.Is(barrier.Cause(), errEvidencePersistence) {
		t.Fatalf("barrier cause=%v", barrier.Cause())
	}
	if got := storage.snapshot().Pts; got != 0 {
		t.Fatalf("state advanced to %d", got)
	}
}

func TestRecoveryAdmission_ChannelDifferenceEvidencePrecedesChannelState(t *testing.T) {
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
	upstream := newScriptedRecoveryAPI()
	upstream.channelDiffs = []tg.UpdatesChannelDifferenceClass{&tg.UpdatesChannelDifferenceEmpty{Pts: 11}}
	api := newSourcePreservingRecoveryAPI(upstream, barrier, func(_ context.Context, kind string, payload []byte) error {
		if kind != recoveryEvidenceChannelDifference {
			t.Fatalf("kind=%q", kind)
		}
		if len(payload) == 0 {
			t.Fatal("empty channel recovery Evidence")
		}
		order.add("evidence:channel-difference")
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	manager := updates.New(updates.Config{
		Storage:      storage,
		AccessHasher: hasher,
		Handler:      gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
	})
	go func() { done <- manager.Run(ctx, api, contractUserID, updates.AuthOptions{}) }()

	result := waitStateWrite(t, baseStorage, "channel-pts")
	if result.err != nil {
		cancel()
		t.Fatalf("SetChannelPts: %v", result.err)
	}
	baseStorage.mu.Lock()
	pts := baseStorage.channels[contractChannelID]
	baseStorage.mu.Unlock()
	if pts != 11 {
		cancel()
		t.Fatalf("channel pts=%d, want 11", pts)
	}
	events := order.snapshot()
	if len(events) < 2 || events[0] != "evidence:channel-difference" || events[1] != "state:channel" {
		cancel()
		t.Fatalf("order=%v, want channel recovery Evidence before state", events)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop")
	}
}

var _ gotdtelegram.UpdateHandler = (*sourceAdmissionHandler)(nil)
var _ updates.API = (*sourcePreservingRecoveryAPI)(nil)
var _ = sync.Mutex{}
