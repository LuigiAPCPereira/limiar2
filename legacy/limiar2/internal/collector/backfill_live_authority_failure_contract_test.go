package collector_test

import (
	"context"
	"reflect"
	"testing"
)

func TestLiveAuthority_EvidenceFailureCannotAdvanceSyncOrBackfillProgress(t *testing.T) {
	ctx := context.Background()
	initialSync := sourceSyncState{Pts: 80, Qts: 7, Seq: 12, Date: 123, ChannelPts: map[int64]int{42: 900}}
	initialBackfill := backfillProgress{ChannelID: 42, NextOffsetMessageID: 4500, NewestObservedID: 5500}
	store := newAuthorityStore(initialSync, initialBackfill)
	store.failEvidenceForAcquisition = "live_update"

	live := liveAdmission{evidence: store, sync: store}
	err := live.admit(ctx, evidenceObservation{Acquisition: "live_update", ChannelID: 42, MessageID: 5600}, sourceSyncState{
		Pts:        81,
		Qts:        8,
		Seq:        13,
		Date:       124,
		ChannelPts: map[int64]int{42: 901},
	})
	if err == nil {
		t.Fatal("expected Evidence failure")
	}

	gotSync, progress, evidence, events := store.snapshot()
	if !reflect.DeepEqual(gotSync, initialSync) {
		t.Fatalf("sync advanced after live Evidence failure: got=%+v want=%+v", gotSync, initialSync)
	}
	if got := progress[42]; got != initialBackfill {
		t.Fatalf("live failure mutated backfill progress: got=%+v want=%+v", got, initialBackfill)
	}
	if len(evidence) != 0 || len(events) != 0 {
		t.Fatalf("failed live Evidence leaked durable effects: evidence=%v events=%v", evidence, events)
	}
}

func TestBackfillAuthority_CompletionDoesNotCertifyLiveContinuity(t *testing.T) {
	ctx := context.Background()
	// O sync state propositalmente permanece atrás. Concluir a varredura histórica
	// não possui capability para alterar ou certificar a continuidade live.
	initialSync := sourceSyncState{Pts: 40, Seq: 4, ChannelPts: map[int64]int{42: 400}}
	store := newAuthorityStore(initialSync, backfillProgress{ChannelID: 42, NextOffsetMessageID: 100})
	backfill := backfillAdmission{evidence: store, progress: store}

	completed := backfillProgress{ChannelID: 42, NextOffsetMessageID: 0, NewestObservedID: 9000, Completed: true}
	if err := backfill.admitSnapshot(ctx, evidenceObservation{Acquisition: "history_snapshot", ChannelID: 42, MessageID: 1}, completed); err != nil {
		t.Fatal(err)
	}

	gotSync, progress, _, _ := store.snapshot()
	if !reflect.DeepEqual(gotSync, initialSync) {
		t.Fatalf("history completion certified/mutated live continuity: got=%+v want=%+v", gotSync, initialSync)
	}
	if got := progress[42]; got != completed {
		t.Fatalf("backfill completion=%+v want=%+v", got, completed)
	}
}
