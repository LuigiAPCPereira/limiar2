package telegram

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/acquisition"
	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type liveAppenderStub struct {
	items []evidence.Evidence
	err   error
}

func (a *liveAppenderStub) Append(_ context.Context, item evidence.Evidence) (evidence.EvidenceID, error) {
	a.items = append(a.items, item)
	if a.err != nil {
		return evidence.EvidenceID{}, a.err
	}
	var id evidence.EvidenceID
	id[0] = byte(len(a.items))
	return id, nil
}

func newLiveAdmissionForTest(t *testing.T, appender evidence.EvidenceAppender, ids ...string) *acquisition.ConfiguredAdmission {
	t.Helper()

	core, err := acquisition.NewAdmission(appender)
	if err != nil {
		t.Fatal(err)
	}
	subscriptions := make([]acquisition.Subscription, 0, len(ids))
	for _, id := range ids {
		subscriptions = append(subscriptions, acquisition.Subscription{ID: id})
	}
	configured, err := acquisition.NewConfiguredAdmission(core, subscriptions)
	if err != nil {
		t.Fatal(err)
	}
	return configured
}

func sampleLiveUpdates() tg.UpdatesClass {
	return &tg.Updates{}
}

func TestNewLiveIngressRejectsMissingDependencies(t *testing.T) {
	appender := &liveAppenderStub{}
	admission := newLiveAdmissionForTest(t, appender, "acq:ofertas")
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	forward := gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil })

	tests := []struct {
		name       string
		admission  *acquisition.ConfiguredAdmission
		classifier LiveSubscriptionClassifier
		forward    gotdtelegram.UpdateHandler
		now        func() time.Time
	}{
		{name: "sem admission", classifier: classifier, forward: forward, now: time.Now},
		{name: "sem classifier", admission: admission, forward: forward, now: time.Now},
		{name: "sem forward", admission: admission, classifier: classifier, now: time.Now},
		{name: "sem relógio", admission: admission, classifier: classifier, forward: forward},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newLiveIngressWithClock(tt.admission, tt.classifier, tt.forward, tt.now)
			if !errors.Is(err, ErrInvalidLiveIngress) {
				t.Fatalf("erro=%v, esperado ErrInvalidLiveIngress", err)
			}
			if got != nil {
				t.Fatal("dependência ausente retornou LiveIngress")
			}
		})
	}
}

func TestLiveIngressPersistsEncodedEnvelopeBeforeForward(t *testing.T) {
	appender := &liveAppenderStub{}
	admission := newLiveAdmissionForTest(t, appender, "acq:ofertas")
	updates := sampleLiveUpdates()

	var classified tg.UpdatesClass
	classifier := LiveSubscriptionClassifierFunc(func(_ context.Context, got tg.UpdatesClass) ([]string, error) {
		classified = got
		return []string{"acq:ofertas"}, nil
	})

	var forwarded tg.UpdatesClass
	forward := gotdtelegram.UpdateHandlerFunc(func(_ context.Context, got tg.UpdatesClass) error {
		if len(appender.items) != 1 {
			t.Fatalf("forward ocorreu com %d Evidence persistidas, esperado 1", len(appender.items))
		}
		forwarded = got
		return nil
	})

	receivedAt := time.Date(2026, 10, 1, 23, 59, 58, 123000000, time.FixedZone("test", -3*60*60))
	ingress, err := newLiveIngressWithClock(admission, classifier, forward, func() time.Time { return receivedAt })
	if err != nil {
		t.Fatal(err)
	}

	if err := ingress.Handle(context.Background(), updates); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if classified != updates {
		t.Fatal("classifier não recebeu o mesmo envelope do gotd")
	}
	if forwarded != updates {
		t.Fatal("forward não recebeu o mesmo envelope do gotd")
	}
	if len(appender.items) != 1 {
		t.Fatalf("appends=%d, esperado 1", len(appender.items))
	}

	item := appender.items[0]
	if item.SubscriptionID != "acq:ofertas" {
		t.Fatalf("subscription_id=%q", item.SubscriptionID)
	}
	if item.Acquisition != liveAcquisition {
		t.Fatalf("acquisition=%q", item.Acquisition)
	}
	if item.EventKind != liveEventKind {
		t.Fatalf("event_kind=%q", item.EventKind)
	}
	if item.SourceEventType == "" {
		t.Fatal("source_event_type vazio")
	}
	if item.SourceOccurredAt != nil {
		t.Fatal("source_occurred_at foi inventado no ingress")
	}
	if item.ReceivedAt != receivedAt.UTC().UnixMilli() {
		t.Fatalf("received_at=%d, esperado=%d", item.ReceivedAt, receivedAt.UTC().UnixMilli())
	}
	if item.PayloadFormat != livePayloadFormat {
		t.Fatalf("payload_format=%q", item.PayloadFormat)
	}
	if item.PayloadSchema != livePayloadSchema {
		t.Fatalf("payload_schema=%q", item.PayloadSchema)
	}
	if len(item.Payload) == 0 {
		t.Fatal("payload TL serializado vazio")
	}
}

func TestLiveIngressClassifierFailureHasNoSideEffects(t *testing.T) {
	appender := &liveAppenderStub{}
	admission := newLiveAdmissionForTest(t, appender, "acq:ofertas")
	errClassify := errors.New("classificação indisponível")
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return nil, errClassify
	})
	forwardCalled := false
	forward := gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		forwardCalled = true
		return nil
	})

	ingress, err := NewLiveIngress(admission, classifier, forward)
	if err != nil {
		t.Fatal(err)
	}

	err = ingress.Handle(context.Background(), sampleLiveUpdates())
	if !errors.Is(err, errClassify) {
		t.Fatalf("erro=%v, esperado falha do classifier", err)
	}
	if len(appender.items) != 0 {
		t.Fatalf("appends=%d após falha de classificação", len(appender.items))
	}
	if forwardCalled {
		t.Fatal("forward ocorreu após falha de classificação")
	}
}

func TestLiveIngressUnknownSubscriptionDoesNotForward(t *testing.T) {
	appender := &liveAppenderStub{}
	admission := newLiveAdmissionForTest(t, appender, "acq:ofertas")
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:desconhecida"}, nil
	})
	forwardCalled := false
	forward := gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		forwardCalled = true
		return nil
	})

	ingress, err := NewLiveIngress(admission, classifier, forward)
	if err != nil {
		t.Fatal(err)
	}

	err = ingress.Handle(context.Background(), sampleLiveUpdates())
	if !errors.Is(err, acquisition.ErrInvalidSubscriptionSelection) {
		t.Fatalf("erro=%v, esperado seleção inválida", err)
	}
	if len(appender.items) != 0 {
		t.Fatalf("appends=%d com subscription desconhecida", len(appender.items))
	}
	if forwardCalled {
		t.Fatal("forward ocorreu com subscription desconhecida")
	}
}

func TestLiveIngressAppendFailureDoesNotForward(t *testing.T) {
	errPersist := errors.New("storage indisponível")
	appender := &liveAppenderStub{err: errPersist}
	admission := newLiveAdmissionForTest(t, appender, "acq:ofertas")
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	forwardCalled := false
	forward := gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		forwardCalled = true
		return nil
	})

	ingress, err := NewLiveIngress(admission, classifier, forward)
	if err != nil {
		t.Fatal(err)
	}

	err = ingress.Handle(context.Background(), sampleLiveUpdates())
	if !errors.Is(err, errPersist) {
		t.Fatalf("erro=%v, esperado falha do append", err)
	}
	if forwardCalled {
		t.Fatal("forward ocorreu após falha do append")
	}
}

func TestLiveIngressPersistsAllOverlappingSubscriptionsBeforeForward(t *testing.T) {
	appender := &liveAppenderStub{}
	admission := newLiveAdmissionForTest(t, appender, "acq:varejo", "acq:celulares")
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:varejo", "acq:celulares"}, nil
	})

	forward := gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		if len(appender.items) != 2 {
			t.Fatalf("forward ocorreu com %d Evidence persistidas, esperado 2", len(appender.items))
		}
		return nil
	})

	ingress, err := NewLiveIngress(admission, classifier, forward)
	if err != nil {
		t.Fatal(err)
	}
	if err := ingress.Handle(context.Background(), sampleLiveUpdates()); err != nil {
		t.Fatal(err)
	}

	got := []string{appender.items[0].SubscriptionID, appender.items[1].SubscriptionID}
	want := []string{"acq:varejo", "acq:celulares"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subscriptions=%v, esperado=%v", got, want)
	}
}

func TestLiveIngressRejectsNilEnvelope(t *testing.T) {
	appender := &liveAppenderStub{}
	admission := newLiveAdmissionForTest(t, appender, "acq:ofertas")
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	forwardCalled := false
	forward := gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error {
		forwardCalled = true
		return nil
	})

	ingress, err := NewLiveIngress(admission, classifier, forward)
	if err != nil {
		t.Fatal(err)
	}

	err = ingress.Handle(context.Background(), nil)
	if !errors.Is(err, ErrInvalidLiveIngress) {
		t.Fatalf("erro=%v, esperado ErrInvalidLiveIngress", err)
	}
	if len(appender.items) != 0 || forwardCalled {
		t.Fatal("envelope nil produziu side effect")
	}
}
