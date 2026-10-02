package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/acquisition"
	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
	"github.com/LuigiAPCPereira/limiar2/internal/recovery"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

type recoveryAppenderStub struct {
	items []evidence.Evidence
	err   error
}

func (a *recoveryAppenderStub) Append(_ context.Context, item evidence.Evidence) (evidence.EvidenceID, error) {
	a.items = append(a.items, item)
	if a.err != nil {
		return evidence.EvidenceID{}, a.err
	}
	var id evidence.EvidenceID
	id[0] = byte(len(a.items))
	return id, nil
}

type recoveryAPIStub struct {
	state       *tg.UpdatesState
	stateErr    error
	diff        tg.UpdatesDifferenceClass
	diffErr     error
	channelDiff tg.UpdatesChannelDifferenceClass
	channelErr  error

	stateCalls   int
	diffCalls    int
	channelCalls int
}

var _ updates.API = (*recoveryAPIStub)(nil)

func (a *recoveryAPIStub) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	a.stateCalls++
	return a.state, a.stateErr
}

func (a *recoveryAPIStub) UpdatesGetDifference(context.Context, *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	a.diffCalls++
	return a.diff, a.diffErr
}

func (a *recoveryAPIStub) UpdatesGetChannelDifference(context.Context, *tg.UpdatesGetChannelDifferenceRequest) (tg.UpdatesChannelDifferenceClass, error) {
	a.channelCalls++
	return a.channelDiff, a.channelErr
}

func newRecoveryAdmissionForTest(t *testing.T, appender evidence.EvidenceAppender, subscriptionID string) *acquisition.ConfiguredAdmission {
	t.Helper()

	core, err := acquisition.NewAdmission(appender)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := acquisition.NewConfiguredAdmission(core, []acquisition.Subscription{{ID: subscriptionID}})
	if err != nil {
		t.Fatal(err)
	}
	return admission
}

func newGuardedRecoveryAPIForTest(
	t *testing.T,
	upstream updates.API,
	appender evidence.EvidenceAppender,
	barrier *recovery.DurabilityBarrier,
) *GuardedRecoveryAPI {
	t.Helper()

	admission := newRecoveryAdmissionForTest(t, appender, "acq:ofertas")
	api, err := newGuardedRecoveryAPIWithClock(
		upstream,
		admission,
		"acq:ofertas",
		barrier,
		func() time.Time { return time.Date(2026, 10, 2, 12, 0, 0, 123000000, time.FixedZone("test", -3*60*60)) },
	)
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func TestNewGuardedRecoveryAPIRejectsMissingDependencies(t *testing.T) {
	upstream := &recoveryAPIStub{}
	appender := &recoveryAppenderStub{}
	admission := newRecoveryAdmissionForTest(t, appender, "acq:ofertas")
	barrier := recovery.NewDurabilityBarrier()

	tests := []struct {
		name           string
		upstream       updates.API
		admission      *acquisition.ConfiguredAdmission
		subscriptionID string
		barrier        *recovery.DurabilityBarrier
		now            func() time.Time
	}{
		{name: "sem upstream", admission: admission, subscriptionID: "acq:ofertas", barrier: barrier, now: time.Now},
		{name: "sem admission", upstream: upstream, subscriptionID: "acq:ofertas", barrier: barrier, now: time.Now},
		{name: "sem subscription", upstream: upstream, admission: admission, barrier: barrier, now: time.Now},
		{name: "subscription com whitespace", upstream: upstream, admission: admission, subscriptionID: " acq:ofertas ", barrier: barrier, now: time.Now},
		{name: "sem barrier", upstream: upstream, admission: admission, subscriptionID: "acq:ofertas", now: time.Now},
		{name: "sem relógio", upstream: upstream, admission: admission, subscriptionID: "acq:ofertas", barrier: barrier},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newGuardedRecoveryAPIWithClock(
				tt.upstream, tt.admission, tt.subscriptionID, tt.barrier, tt.now,
			)
			if !errors.Is(err, ErrInvalidRecoveryAPI) {
				t.Fatalf("erro=%v, esperado ErrInvalidRecoveryAPI", err)
			}
			if got != nil {
				t.Fatal("configuração inválida retornou adapter")
			}
		})
	}
}

func TestGuardedRecoveryAPIPreservesRemoteBaselineBeforeReturn(t *testing.T) {
	state := &tg.UpdatesState{Pts: 42, Qts: 3, Date: 10, Seq: 7}
	upstream := &recoveryAPIStub{state: state}
	appender := &recoveryAppenderStub{}
	barrier := recovery.NewDurabilityBarrier()
	api := newGuardedRecoveryAPIForTest(t, upstream, appender, barrier)

	got, err := api.UpdatesGetState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != state {
		t.Fatal("wrapper não devolveu a mesma resposta de baseline")
	}
	if len(appender.items) != 1 {
		t.Fatalf("Evidence=%d, esperado 1", len(appender.items))
	}

	item := appender.items[0]
	if item.SubscriptionID != "acq:ofertas" {
		t.Fatalf("subscription_id=%q", item.SubscriptionID)
	}
	if item.Acquisition != recoveryAcquisition || item.EventKind != recoveryEventBaseline {
		t.Fatalf("metadata recovery=%q event=%q", item.Acquisition, item.EventKind)
	}
	if item.SourceEventType == "" || item.PayloadFormat != recoveryPayloadFormat || item.PayloadSchema != recoveryPayloadSchema {
		t.Fatalf("metadata física incompleta: %+v", item)
	}
	if item.SourceOccurredAt != nil {
		t.Fatal("source_occurred_at foi inventado para baseline")
	}
	wantReceivedAt := time.Date(2026, 10, 2, 12, 0, 0, 123000000, time.FixedZone("test", -3*60*60)).UTC().UnixMilli()
	if item.ReceivedAt != wantReceivedAt {
		t.Fatalf("received_at=%d, esperado=%d", item.ReceivedAt, wantReceivedAt)
	}
	if len(item.Payload) == 0 {
		t.Fatal("baseline TL serializado vazio")
	}
	if err := barrier.Err(); err != nil {
		t.Fatalf("baseline durável fechou barrier: %v", err)
	}
}

func TestGuardedRecoveryAPIEvidenceFailureBlocksResponseAndClosesBarrier(t *testing.T) {
	persistErr := errors.New("Evidence indisponível")
	upstream := &recoveryAPIStub{state: &tg.UpdatesState{Pts: 42}}
	appender := &recoveryAppenderStub{err: persistErr}
	barrier := recovery.NewDurabilityBarrier()
	api := newGuardedRecoveryAPIForTest(t, upstream, appender, barrier)

	state, err := api.UpdatesGetState(context.Background())
	if state != nil {
		t.Fatal("baseline escapou após falha de Evidence")
	}
	if !errors.Is(err, recovery.ErrDurabilityBarrierClosed) || !errors.Is(err, persistErr) {
		t.Fatalf("erro=%v não preserva barrier + causa", err)
	}
	if barrierErr := barrier.Err(); !errors.Is(barrierErr, persistErr) {
		t.Fatalf("barrier=%v não preserva falha de Evidence", barrierErr)
	}
}

func TestGuardedRecoveryAPICommonDifferenceClassification(t *testing.T) {
	tests := []struct {
		name      string
		diff      tg.UpdatesDifferenceClass
		wantItems int
		wantKind  string
	}{
		{name: "empty", diff: &tg.UpdatesDifferenceEmpty{}, wantItems: 0},
		{
			name: "difference sem observações",
			diff: &tg.UpdatesDifference{State: tg.UpdatesState{Pts: 1}},
			wantItems: 0,
		},
		{
			name: "difference com mensagem",
			diff: &tg.UpdatesDifference{
				NewMessages: []tg.MessageClass{&tg.MessageEmpty{ID: 1}},
				State:       tg.UpdatesState{Pts: 1},
			},
			wantItems: 1,
			wantKind: recoveryEventObservation,
		},
		{
			name: "slice com update",
			diff: &tg.UpdatesDifferenceSlice{
				OtherUpdates:      []tg.UpdateClass{&tg.UpdateDeleteMessages{Messages: []int{1}, Pts: 1, PtsCount: 1}},
				IntermediateState: tg.UpdatesState{Pts: 1},
			},
			wantItems: 1,
			wantKind: recoveryEventObservation,
		},
		{
			name:      "too long",
			diff:      &tg.UpdatesDifferenceTooLong{Pts: 99},
			wantItems: 1,
			wantKind:  recoveryEventDiscontinuity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &recoveryAPIStub{diff: tt.diff}
			appender := &recoveryAppenderStub{}
			api := newGuardedRecoveryAPIForTest(t, upstream, appender, recovery.NewDurabilityBarrier())

			got, err := api.UpdatesGetDifference(context.Background(), &tg.UpdatesGetDifferenceRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.diff {
				t.Fatal("wrapper não devolveu a mesma resposta")
			}
			if len(appender.items) != tt.wantItems {
				t.Fatalf("Evidence=%d, esperado=%d", len(appender.items), tt.wantItems)
			}
			if tt.wantItems > 0 {
				if appender.items[0].EventKind != tt.wantKind || len(appender.items[0].Payload) == 0 {
					t.Fatalf("Evidence=%+v", appender.items[0])
				}
			}
		})
	}
}

func TestGuardedRecoveryAPIChannelDifferenceClassification(t *testing.T) {
	tests := []struct {
		name      string
		diff      tg.UpdatesChannelDifferenceClass
		wantItems int
		wantKind  string
	}{
		{name: "empty", diff: &tg.UpdatesChannelDifferenceEmpty{}, wantItems: 0},
		{
			name: "difference sem observações",
			diff: &tg.UpdatesChannelDifference{Pts: 1},
			wantItems: 0,
		},
		{
			name: "difference com mensagem",
			diff: &tg.UpdatesChannelDifference{
				Pts:         1,
				NewMessages: []tg.MessageClass{&tg.MessageEmpty{ID: 1}},
			},
			wantItems: 1,
			wantKind: recoveryEventObservation,
		},
		{
			name:      "too long",
			diff:      &tg.UpdatesChannelDifferenceTooLong{},
			wantItems: 1,
			wantKind:  recoveryEventDiscontinuity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &recoveryAPIStub{channelDiff: tt.diff}
			appender := &recoveryAppenderStub{}
			api := newGuardedRecoveryAPIForTest(t, upstream, appender, recovery.NewDurabilityBarrier())

			got, err := api.UpdatesGetChannelDifference(context.Background(), &tg.UpdatesGetChannelDifferenceRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.diff {
				t.Fatal("wrapper não devolveu a mesma resposta")
			}
			if len(appender.items) != tt.wantItems {
				t.Fatalf("Evidence=%d, esperado=%d", len(appender.items), tt.wantItems)
			}
			if tt.wantItems > 0 {
				if appender.items[0].EventKind != tt.wantKind || len(appender.items[0].Payload) == 0 {
					t.Fatalf("Evidence=%+v", appender.items[0])
				}
			}
		})
	}
}

func TestGuardedRecoveryAPIUnknownOrNilResponseFailsClosed(t *testing.T) {
	t.Run("common nil", func(t *testing.T) {
		upstream := &recoveryAPIStub{diff: nil}
		barrier := recovery.NewDurabilityBarrier()
		api := newGuardedRecoveryAPIForTest(t, upstream, &recoveryAppenderStub{}, barrier)

		got, err := api.UpdatesGetDifference(context.Background(), &tg.UpdatesGetDifferenceRequest{})
		if got != nil {
			t.Fatal("resposta nil produziu valor")
		}
		if !errors.Is(err, recovery.ErrDurabilityBarrierClosed) || !errors.Is(err, ErrUnknownRecoveryResponse) {
			t.Fatalf("erro=%v", err)
		}
	})

	t.Run("channel nil", func(t *testing.T) {
		upstream := &recoveryAPIStub{channelDiff: nil}
		barrier := recovery.NewDurabilityBarrier()
		api := newGuardedRecoveryAPIForTest(t, upstream, &recoveryAppenderStub{}, barrier)

		got, err := api.UpdatesGetChannelDifference(context.Background(), &tg.UpdatesGetChannelDifferenceRequest{})
		if got != nil {
			t.Fatal("resposta nil produziu valor")
		}
		if !errors.Is(err, recovery.ErrDurabilityBarrierClosed) || !errors.Is(err, ErrUnknownRecoveryResponse) {
			t.Fatalf("erro=%v", err)
		}
	})
}

func TestGuardedRecoveryAPIUpstreamErrorDoesNotCloseBarrier(t *testing.T) {
	upstreamErr := errors.New("RPC temporariamente indisponível")
	upstream := &recoveryAPIStub{diffErr: upstreamErr}
	barrier := recovery.NewDurabilityBarrier()
	api := newGuardedRecoveryAPIForTest(t, upstream, &recoveryAppenderStub{}, barrier)

	got, err := api.UpdatesGetDifference(context.Background(), &tg.UpdatesGetDifferenceRequest{})
	if got != nil {
		t.Fatal("RPC com erro retornou response")
	}
	if !errors.Is(err, upstreamErr) {
		t.Fatalf("erro=%v, esperado upstream error", err)
	}
	if barrierErr := barrier.Err(); barrierErr != nil {
		t.Fatalf("upstream error fechou barrier: %v", barrierErr)
	}
}

func TestGuardedRecoveryAPIClosedBarrierPreventsUpstreamCall(t *testing.T) {
	cause := errors.New("lifecycle encerrado")
	upstream := &recoveryAPIStub{state: &tg.UpdatesState{}}
	barrier := recovery.NewDurabilityBarrier()
	barrier.Close(cause)
	api := newGuardedRecoveryAPIForTest(t, upstream, &recoveryAppenderStub{}, barrier)

	got, err := api.UpdatesGetState(context.Background())
	if got != nil {
		t.Fatal("barrier fechada retornou baseline")
	}
	if !errors.Is(err, recovery.ErrDurabilityBarrierClosed) || !errors.Is(err, cause) {
		t.Fatalf("erro=%v", err)
	}
	if upstream.stateCalls != 0 {
		t.Fatalf("upstream chamado %d vezes após lifecycle fechado", upstream.stateCalls)
	}
}
