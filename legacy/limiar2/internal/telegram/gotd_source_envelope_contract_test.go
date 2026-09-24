package telegram_test

import (
	"context"
	"testing"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

// TestADR017Gate_PostManagerHandlerDoesNotPreserveSourceEnvelope documents a
// source-contract property of gotd/td v0.161.0 that matters to ADRs 016/017:
// updates.Manager is an ordering/recovery component, not a transparent raw
// envelope transport. A source envelope can be reordered/split and top-level
// metadata can be lost before the configured handler is invoked.
func TestADR017Gate_PostManagerHandlerDoesNotPreserveSourceEnvelope(t *testing.T) {
	barrier := newDurabilityBarrier()
	storage := newContractStorage(barrier, updates.State{})
	api := newContractAPI()
	handled := make(chan *tg.Updates, 4)

	running := startContractManager(t, storage, api, gotdtelegram.UpdateHandlerFunc(func(_ context.Context, u tg.UpdatesClass) error {
		batch, ok := u.(*tg.Updates)
		if !ok {
			t.Errorf("handler recebeu %T, want *tg.Updates", u)
			return nil
		}
		handled <- batch
		return nil
	}))
	defer running.stop(t)

	// Run performs startup recovery. Drain it so only the live envelope below is
	// observed by this assertion.
	select {
	case <-api.diffCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("startup getDifference was not observed")
	}

	original := &tg.Updates{
		Updates: []tg.UpdateClass{
			// Deliberately reverse PTS order. gotd sorts PTS updates before
			// applying them and can dispatch them in separate handler calls.
			&tg.UpdateDeleteMessages{Messages: []int{2}, Pts: 2, PtsCount: 1},
			&tg.UpdateDeleteMessages{Messages: []int{1}, Pts: 1, PtsCount: 1},
		},
		Date: 123456,
		Seq:  0,
	}

	if err := running.manager.Handle(context.Background(), original); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	var batches []*tg.Updates
	deadline := time.After(3 * time.Second)
	for len(batches) < 2 {
		select {
		case batch := <-handled:
			batches = append(batches, batch)
		case <-deadline:
			t.Fatalf("handler calls = %d, want 2 reconstructed batches", len(batches))
		}
	}

	if batches[0] == original || batches[1] == original {
		t.Fatal("original source envelope unexpectedly reached the post-manager handler")
	}
	for i, batch := range batches {
		if batch.Date != 0 {
			t.Fatalf("batch[%d].Date = %d, want 0 to prove source envelope metadata was not preserved", i, batch.Date)
		}
		if len(batch.Updates) != 1 {
			t.Fatalf("batch[%d] updates = %d, want 1", i, len(batch.Updates))
		}
	}

	first, ok := batches[0].Updates[0].(*tg.UpdateDeleteMessages)
	if !ok {
		t.Fatalf("first reconstructed update = %T", batches[0].Updates[0])
	}
	second, ok := batches[1].Updates[0].(*tg.UpdateDeleteMessages)
	if !ok {
		t.Fatalf("second reconstructed update = %T", batches[1].Updates[0])
	}
	if first.Pts != 1 || second.Pts != 2 {
		t.Fatalf("post-manager PTS order = [%d %d], want [1 2] after original [2 1]", first.Pts, second.Pts)
	}

	if got := storage.snapshot().Pts; got != 2 {
		t.Fatalf("persisted pts = %d, want 2", got)
	}
}
