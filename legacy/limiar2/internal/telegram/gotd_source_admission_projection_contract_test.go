package telegram_test

import (
	"context"
	"errors"
	"testing"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

func TestSourceAdmission_DurableLiveEvidenceAllowsStateAdvanceWhenOrderedHandlerFails(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newContractAPI()
	manager := updates.New(updates.Config{
		Storage: storage,
		Handler: gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
			return errors.New("derived projection failed")
		}),
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

	persisted := false
	admission := &sourceAdmissionHandler{
		barrier: barrier,
		next:    manager,
		persist: func(context.Context, []byte) error {
			persisted = true
			return nil
		},
	}
	input := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateDeleteMessages{Messages: []int{2}, Pts: 2, PtsCount: 1},
		&tg.UpdateEditMessage{Message: &tg.Message{ID: 2, Message: "edited"}, Pts: 1, PtsCount: 1},
	}}
	if err := admission.Handle(context.Background(), input); err != nil {
		t.Fatalf("source admission: %v", err)
	}
	if !persisted {
		t.Fatal("source Evidence was not persisted")
	}

	first := waitStateWrite(t, storage, "pts")
	second := waitStateWrite(t, storage, "pts")
	if first.err != nil || second.err != nil {
		t.Fatalf("SetPts errors: first=%v second=%v", first.err, second.err)
	}
	if got := storage.snapshot().Pts; got != 2 {
		t.Fatalf("persisted pts=%d, want 2", got)
	}
	if barrier.Cause() != nil {
		t.Fatalf("derived handler failure unexpectedly closed source durability barrier: %v", barrier.Cause())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop")
	}
}

func TestRecoveryAdmission_ChannelDifferenceWithContentEvidencePrecedesChannelState(t *testing.T) {
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
	upstream.channelDiffs = []tg.UpdatesChannelDifferenceClass{
		&tg.UpdatesChannelDifference{
			Final:       true,
			Pts:         11,
			NewMessages: []tg.MessageClass{&tg.MessageEmpty{ID: 99}},
		},
	}
	api := newSourcePreservingRecoveryAPI(upstream, barrier, func(_ context.Context, kind string, payload []byte) error {
		switch kind {
		case recoveryEvidenceDifference:
			return nil
		case recoveryEvidenceChannelDifference:
			if len(payload) == 0 {
				t.Fatal("empty channel recovery Evidence")
			}
			order.add("evidence:channel-content")
			return nil
		default:
			t.Fatalf("unexpected recovery kind=%q", kind)
			return nil
		}
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
	if len(events) < 2 || events[0] != "evidence:channel-content" || events[1] != "state:channel" {
		cancel()
		t.Fatalf("order=%v, want content Evidence before channel state", events)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop")
	}
}
