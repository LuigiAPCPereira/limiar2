package acquisition

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
)

type scriptedAppender struct {
	items  []evidence.Evidence
	failAt int
	err    error
	order  *[]string
}

func (a *scriptedAppender) Append(_ context.Context, item evidence.Evidence) (evidence.EvidenceID, error) {
	a.items = append(a.items, item)
	if a.order != nil {
		*a.order = append(*a.order, "append:"+item.SubscriptionID)
	}
	if a.failAt > 0 && len(a.items) == a.failAt {
		return evidence.EvidenceID{}, a.err
	}
	var id evidence.EvidenceID
	id[0] = byte(len(a.items))
	return id, nil
}

func configuredEvidence() evidence.Evidence {
	item := admissionEvidence()
	item.SubscriptionID = ""
	return item
}

func newConfiguredAdmissionForTest(t *testing.T, appender evidence.EvidenceAppender, subscriptions ...string) *ConfiguredAdmission {
	t.Helper()

	core, err := NewAdmission(appender)
	if err != nil {
		t.Fatal(err)
	}
	configured := make([]Subscription, 0, len(subscriptions))
	for _, id := range subscriptions {
		configured = append(configured, Subscription{ID: id})
	}
	admission, err := NewConfiguredAdmission(core, configured)
	if err != nil {
		t.Fatal(err)
	}
	return admission
}

func TestNewConfiguredAdmissionRejectsInvalidSnapshot(t *testing.T) {
	core, err := NewAdmission(&scriptedAppender{})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		subscriptions []Subscription
	}{
		{name: "sem subscriptions"},
		{name: "id vazio", subscriptions: []Subscription{{ID: ""}}},
		{name: "somente whitespace", subscriptions: []Subscription{{ID: "   "}}},
		{name: "whitespace nas bordas", subscriptions: []Subscription{{ID: " acq:ofertas "}}},
		{name: "id duplicado", subscriptions: []Subscription{{ID: "acq:ofertas"}, {ID: "acq:ofertas"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admission, err := NewConfiguredAdmission(core, tt.subscriptions)
			if !errors.Is(err, ErrInvalidSubscriptionConfig) {
				t.Fatalf("erro=%v, esperado ErrInvalidSubscriptionConfig", err)
			}
			if admission != nil {
				t.Fatal("configuração inválida retornou boundary de admissão")
			}
		})
	}
}

func TestNewConfiguredAdmissionRejectsMissingCore(t *testing.T) {
	admission, err := NewConfiguredAdmission(nil, []Subscription{{ID: "acq:ofertas"}})
	if !errors.Is(err, ErrInvalidSubscriptionConfig) {
		t.Fatalf("erro=%v, esperado ErrInvalidSubscriptionConfig", err)
	}
	if admission != nil {
		t.Fatal("core ausente retornou boundary de admissão")
	}
}

func TestConfiguredAdmissionInjectsExactConfiguredIdentity(t *testing.T) {
	appender := &scriptedAppender{}
	admission := newConfiguredAdmissionForTest(t, appender, "acq:ofertas-v1")

	forwardCalls := 0
	if err := admission.Admit(
		context.Background(),
		[]string{"acq:ofertas-v1"},
		configuredEvidence(),
		func(context.Context) error {
			forwardCalls++
			return nil
		},
	); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	if forwardCalls != 1 {
		t.Fatalf("forward chamado %d vezes, esperado 1", forwardCalls)
	}
	if len(appender.items) != 1 {
		t.Fatalf("appends=%d, esperado 1", len(appender.items))
	}
	if got := appender.items[0].SubscriptionID; got != "acq:ofertas-v1" {
		t.Fatalf("subscription_id=%q, esperado %q", got, "acq:ofertas-v1")
	}
}

func TestConfiguredAdmissionValidatesSelectionBeforeAnyAppend(t *testing.T) {
	tests := []struct {
		name       string
		selected   []string
		item       evidence.Evidence
		configured []string
	}{
		{
			name:       "nenhuma aplicável",
			selected:   nil,
			item:       configuredEvidence(),
			configured: []string{"acq:ofertas"},
		},
		{
			name:       "não configurada",
			selected:   []string{"acq:desconhecida"},
			item:       configuredEvidence(),
			configured: []string{"acq:ofertas"},
		},
		{
			name:       "seleção duplicada",
			selected:   []string{"acq:ofertas", "acq:ofertas"},
			item:       configuredEvidence(),
			configured: []string{"acq:ofertas"},
		},
		{
			name:       "whitespace na seleção",
			selected:   []string{" acq:ofertas"},
			item:       configuredEvidence(),
			configured: []string{"acq:ofertas"},
		},
		{
			name:     "evidence já contextualizada",
			selected: []string{"acq:ofertas"},
			item: func() evidence.Evidence {
				item := configuredEvidence()
				item.SubscriptionID = "acq:externa"
				return item
			}(),
			configured: []string{"acq:ofertas"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appender := &scriptedAppender{}
			admission := newConfiguredAdmissionForTest(t, appender, tt.configured...)

			forwardCalled := false
			err := admission.Admit(context.Background(), tt.selected, tt.item, func(context.Context) error {
				forwardCalled = true
				return nil
			})
			if !errors.Is(err, ErrInvalidSubscriptionSelection) {
				t.Fatalf("erro=%v, esperado ErrInvalidSubscriptionSelection", err)
			}
			if len(appender.items) != 0 {
				t.Fatalf("houve %d appends antes de rejeitar a seleção", len(appender.items))
			}
			if forwardCalled {
				t.Fatal("forward foi chamado com seleção inválida")
			}
		})
	}
}

func TestConfiguredAdmissionPersistsEveryApplicableSubscriptionBeforeForward(t *testing.T) {
	var order []string
	appender := &scriptedAppender{order: &order}
	admission := newConfiguredAdmissionForTest(t, appender, "acq:varejo", "acq:celulares")

	err := admission.Admit(
		context.Background(),
		[]string{"acq:varejo", "acq:celulares"},
		configuredEvidence(),
		func(context.Context) error {
			order = append(order, "forward")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}

	wantOrder := []string{"append:acq:varejo", "append:acq:celulares", "forward"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("ordem=%v, esperado=%v", order, wantOrder)
	}
	if len(appender.items) != 2 {
		t.Fatalf("appends=%d, esperado 2", len(appender.items))
	}
	if appender.items[0].SubscriptionID == appender.items[1].SubscriptionID {
		t.Fatal("subscriptions sobrepostas perderam identidade distinta")
	}
}

func TestConfiguredAdmissionPartialAppendFailureBlocksForward(t *testing.T) {
	errPersist := errors.New("falha na segunda persistência")
	var order []string
	appender := &scriptedAppender{
		failAt: 2,
		err:    errPersist,
		order:  &order,
	}
	admission := newConfiguredAdmissionForTest(t, appender, "acq:varejo", "acq:celulares")

	forwardCalled := false
	err := admission.Admit(
		context.Background(),
		[]string{"acq:varejo", "acq:celulares"},
		configuredEvidence(),
		func(context.Context) error {
			forwardCalled = true
			order = append(order, "forward")
			return nil
		},
	)
	if !errors.Is(err, errPersist) {
		t.Fatalf("erro=%v, esperado falha de persistência", err)
	}
	if forwardCalled {
		t.Fatal("forward ocorreu após falha parcial de append")
	}

	wantOrder := []string{"append:acq:varejo", "append:acq:celulares"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("ordem=%v, esperado=%v", order, wantOrder)
	}
	if len(appender.items) != 2 {
		t.Fatalf("tentativas de append=%d, esperado 2", len(appender.items))
	}
	if got := appender.items[0].SubscriptionID; got != "acq:varejo" {
		t.Fatalf("primeira Evidence perdeu identidade durável: %q", got)
	}
}

func TestConfiguredAdmissionReplayPreservesConfiguredIdentity(t *testing.T) {
	appender := &scriptedAppender{}
	admission := newConfiguredAdmissionForTest(t, appender, "acq:ofertas")

	for attempt := 0; attempt < 2; attempt++ {
		if err := admission.Admit(
			context.Background(),
			[]string{"acq:ofertas"},
			configuredEvidence(),
			func(context.Context) error { return nil },
		); err != nil {
			t.Fatalf("tentativa %d: %v", attempt+1, err)
		}
	}

	if len(appender.items) != 2 {
		t.Fatalf("appends=%d, esperado 2 para replay", len(appender.items))
	}
	for i, item := range appender.items {
		if item.SubscriptionID != "acq:ofertas" {
			t.Fatalf("append %d subscription_id=%q", i+1, item.SubscriptionID)
		}
	}
}

func TestConfiguredAdmissionRejectsNilForwardBeforeAppend(t *testing.T) {
	appender := &scriptedAppender{}
	admission := newConfiguredAdmissionForTest(t, appender, "acq:ofertas")

	err := admission.Admit(context.Background(), []string{"acq:ofertas"}, configuredEvidence(), nil)
	if !errors.Is(err, ErrInvalidSubscriptionSelection) {
		t.Fatalf("erro=%v, esperado ErrInvalidSubscriptionSelection", err)
	}
	if len(appender.items) != 0 {
		t.Fatalf("houve %d appends com forward inválido", len(appender.items))
	}
}
