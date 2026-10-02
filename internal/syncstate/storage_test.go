package syncstate_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
	"github.com/LuigiAPCPereira/limiar2/internal/syncstate"
	"github.com/gotd/td/telegram/updates"
)

func openStateStorage(t *testing.T, subscriptionID string) (string, *evidence.Store, *syncstate.Storage) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "limiar-state.db")
	store, err := evidence.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.SourceSyncState(subscriptionID)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return path, store, state
}

func closeStore(t *testing.T, store *evidence.Store) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestSourceSyncStateImplementsGotdStateStorage(t *testing.T) {
	_, store, state := openStateStorage(t, "acq:ofertas")
	defer closeStore(t, store)
	var _ updates.StateStorage = state
}

func TestSourceSyncStateRejectsInvalidSubscriptionID(t *testing.T) {
	store, err := evidence.Open(context.Background(), filepath.Join(t.TempDir(), "invalid.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore(t, store)

	for _, id := range []string{"", "   ", " acq:ofertas", "acq:ofertas "} {
		state, err := store.SourceSyncState(id)
		if !errors.Is(err, syncstate.ErrInvalidSubscriptionID) {
			t.Fatalf("id=%q erro=%v, esperado ErrInvalidSubscriptionID", id, err)
		}
		if state != nil {
			t.Fatalf("id=%q retornou capability", id)
		}
	}
}

func TestSourceSyncStateCommonRoundTripAndPartialSetters(t *testing.T) {
	_, store, state := openStateStorage(t, "acq:ofertas")
	defer closeStore(t, store)
	ctx := context.Background()
	userID := int64(42)

	got, found, err := state.GetState(ctx, userID)
	if err != nil || found || got != (updates.State{}) {
		t.Fatalf("ausência got=%+v found=%v err=%v", got, found, err)
	}

	initial := updates.State{Pts: 1, Qts: 2, Date: 3, Seq: 4}
	if err := state.SetState(ctx, userID, initial); err != nil {
		t.Fatal(err)
	}
	if err := state.SetPts(ctx, userID, 10); err != nil {
		t.Fatal(err)
	}
	if err := state.SetQts(ctx, userID, 20); err != nil {
		t.Fatal(err)
	}
	if err := state.SetDate(ctx, userID, 30); err != nil {
		t.Fatal(err)
	}
	if err := state.SetSeq(ctx, userID, 40); err != nil {
		t.Fatal(err)
	}
	if err := state.SetDateSeq(ctx, userID, 31, 41); err != nil {
		t.Fatal(err)
	}

	got, found, err = state.GetState(ctx, userID)
	want := updates.State{Pts: 10, Qts: 20, Date: 31, Seq: 41}
	if err != nil || !found || got != want {
		t.Fatalf("got=%+v found=%v err=%v, esperado=%+v", got, found, err, want)
	}
}

func TestSourceSyncStatePartialSettersFailWhenCommonStateIsMissing(t *testing.T) {
	_, store, state := openStateStorage(t, "acq:ofertas")
	defer closeStore(t, store)
	ctx := context.Background()
	userID := int64(42)

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "pts", run: func() error { return state.SetPts(ctx, userID, 1) }},
		{name: "qts", run: func() error { return state.SetQts(ctx, userID, 2) }},
		{name: "date", run: func() error { return state.SetDate(ctx, userID, 3) }},
		{name: "seq", run: func() error { return state.SetSeq(ctx, userID, 4) }},
		{name: "date_seq", run: func() error { return state.SetDateSeq(ctx, userID, 5, 6) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, syncstate.ErrStateNotFound) {
				t.Fatalf("erro=%v, esperado ErrStateNotFound", err)
			}
		})
	}

	_, found, err := state.GetState(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("setter parcial fabricou common state")
	}
}

func TestSourceSyncStateIsolatesSubscriptionAndUser(t *testing.T) {
	store, err := evidence.Open(context.Background(), filepath.Join(t.TempDir(), "isolated.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore(t, store)
	first, err := store.SourceSyncState("acq:varejo")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SourceSyncState("acq:celulares")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := first.SetState(ctx, 42, updates.State{Pts: 1, Qts: 2, Date: 3, Seq: 4}); err != nil {
		t.Fatal(err)
	}
	if err := first.SetState(ctx, 43, updates.State{Pts: 11, Qts: 12, Date: 13, Seq: 14}); err != nil {
		t.Fatal(err)
	}
	if err := second.SetState(ctx, 42, updates.State{Pts: 101, Qts: 102, Date: 103, Seq: 104}); err != nil {
		t.Fatal(err)
	}

	got, found, err := first.GetState(ctx, 42)
	if err != nil || !found || got.Pts != 1 {
		t.Fatalf("first/user42 got=%+v found=%v err=%v", got, found, err)
	}
	got, found, err = first.GetState(ctx, 43)
	if err != nil || !found || got.Pts != 11 {
		t.Fatalf("first/user43 got=%+v found=%v err=%v", got, found, err)
	}
	got, found, err = second.GetState(ctx, 42)
	if err != nil || !found || got.Pts != 101 {
		t.Fatalf("second/user42 got=%+v found=%v err=%v", got, found, err)
	}
}

func TestSourceSyncChannelStateDistinguishesMissingFromZeroAndScopesEnumeration(t *testing.T) {
	store, err := evidence.Open(context.Background(), filepath.Join(t.TempDir(), "channels.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore(t, store)
	first, err := store.SourceSyncState("acq:varejo")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SourceSyncState("acq:celulares")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	pts, found, err := first.GetChannelPts(ctx, 42, 10)
	if err != nil || found || pts != 0 {
		t.Fatalf("ausente pts=%d found=%v err=%v", pts, found, err)
	}
	if err := first.SetChannelPts(ctx, 42, 10, 0); err != nil {
		t.Fatal(err)
	}
	if err := first.SetChannelPts(ctx, 42, 11, 110); err != nil {
		t.Fatal(err)
	}
	if err := first.SetChannelPts(ctx, 43, 12, 120); err != nil {
		t.Fatal(err)
	}
	if err := second.SetChannelPts(ctx, 42, 10, 999); err != nil {
		t.Fatal(err)
	}

	pts, found, err = first.GetChannelPts(ctx, 42, 10)
	if err != nil || !found || pts != 0 {
		t.Fatalf("zero pts=%d found=%v err=%v", pts, found, err)
	}

	var got [][2]int64
	if err := first.ForEachChannels(ctx, 42, func(callbackCtx context.Context, channelID int64, pts int) error {
		if callbackCtx != ctx {
			t.Fatal("callback recebeu context diferente")
		}
		got = append(got, [2]int64{channelID, int64(pts)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := [][2]int64{{10, 0}, {11, 110}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("channels=%v, esperado=%v", got, want)
	}
}

func TestSourceSyncStateForEachChannelsPropagatesCallbackError(t *testing.T) {
	_, store, state := openStateStorage(t, "acq:ofertas")
	defer closeStore(t, store)
	ctx := context.Background()
	if err := state.SetChannelPts(ctx, 42, 10, 100); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("parar enumeração")
	err := state.ForEachChannels(ctx, 42, func(context.Context, int64, int) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro=%v não preserva callback", err)
	}
}

func TestSourceSyncStateSurvivesCloseAndReopen(t *testing.T) {
	path, store, state := openStateStorage(t, "acq:ofertas")
	ctx := context.Background()
	wantState := updates.State{Pts: 1, Qts: 2, Date: 3, Seq: 4}
	if err := state.SetState(ctx, 42, wantState); err != nil {
		t.Fatal(err)
	}
	if err := state.SetChannelPts(ctx, 42, 100, 0); err != nil {
		t.Fatal(err)
	}
	closeStore(t, store)

	reopened, err := evidence.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore(t, reopened)
	reopenedState, err := reopened.SourceSyncState("acq:ofertas")
	if err != nil {
		t.Fatal(err)
	}
	gotState, found, err := reopenedState.GetState(ctx, 42)
	if err != nil || !found || gotState != wantState {
		t.Fatalf("state após reopen=%+v found=%v err=%v", gotState, found, err)
	}
	pts, found, err := reopenedState.GetChannelPts(ctx, 42, 100)
	if err != nil || !found || pts != 0 {
		t.Fatalf("channel após reopen pts=%d found=%v err=%v", pts, found, err)
	}
}

func TestSourceSyncStateReadFailureIsNotAbsence(t *testing.T) {
	_, store, state := openStateStorage(t, "acq:ofertas")
	closeStore(t, store)

	_, found, err := state.GetState(context.Background(), 42)
	if err == nil {
		t.Fatal("read em storage fechado deveria falhar")
	}
	if found {
		t.Fatal("falha de read foi reinterpretada como state encontrado")
	}
}
