package acquisition

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
)

type appenderStub struct {
	order *[]string
	got   evidence.Evidence
	id    evidence.EvidenceID
	err   error
	calls int
}

func (a *appenderStub) Append(_ context.Context, item evidence.Evidence) (evidence.EvidenceID, error) {
	a.calls++
	if a.order != nil {
		*a.order = append(*a.order, "append")
	}
	a.got = item
	if a.err != nil {
		return evidence.EvidenceID{}, a.err
	}
	return a.id, nil
}

func admissionEvidence() evidence.Evidence {
	sourceOccurredAt := int64(1_790_000_000_000)
	return evidence.Evidence{
		SubscriptionID:   "fixture:subscription",
		Acquisition:      "live_update",
		EventKind:        "source_update",
		SourceEventType:  "updates",
		SourceOccurredAt: &sourceOccurredAt,
		ReceivedAt:       1_790_000_000_123,
		PayloadFormat:    "telegram-tl",
		PayloadSchema:    "v1",
		Payload:          []byte{0x01, 0x02, 0x03},
	}
}

func TestAdmissionPersistsBeforeForward(t *testing.T) {
	var order []string
	appender := &appenderStub{
		order: &order,
		id:    evidence.EvidenceID{1},
	}
	admission, err := NewAdmission(appender)
	if err != nil {
		t.Fatal(err)
	}
	item := admissionEvidence()

	forwardCalls := 0
	if err := admission.Admit(context.Background(), item, func(context.Context) error {
		forwardCalls++
		order = append(order, "forward")
		return nil
	}); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	if forwardCalls != 1 {
		t.Fatalf("forward chamado %d vezes, esperado 1", forwardCalls)
	}
	if !reflect.DeepEqual(order, []string{"append", "forward"}) {
		t.Fatalf("ordem=%v, esperado append antes de forward", order)
	}
	if !reflect.DeepEqual(appender.got, item) {
		t.Fatalf("Evidence recebida pelo appender mudou: got=%+v esperado=%+v", appender.got, item)
	}
}

func TestAdmissionAppendFailurePreventsForward(t *testing.T) {
	errPersist := errors.New("falha de persistência")
	var order []string
	appender := &appenderStub{order: &order, err: errPersist}
	admission, err := NewAdmission(appender)
	if err != nil {
		t.Fatal(err)
	}

	forwardCalled := false
	err = admission.Admit(context.Background(), admissionEvidence(), func(context.Context) error {
		forwardCalled = true
		order = append(order, "forward")
		return nil
	})
	if !errors.Is(err, errPersist) {
		t.Fatalf("erro=%v, esperado falha de persistência", err)
	}
	if forwardCalled {
		t.Fatal("forward foi chamado após falha de persistência")
	}
	if !reflect.DeepEqual(order, []string{"append"}) {
		t.Fatalf("ordem=%v, esperado somente append", order)
	}
}

func TestAdmissionForwardFailureOccursAfterDurability(t *testing.T) {
	errForward := errors.New("falha downstream")
	var order []string
	appender := &appenderStub{order: &order, id: evidence.EvidenceID{2}}
	admission, err := NewAdmission(appender)
	if err != nil {
		t.Fatal(err)
	}

	err = admission.Admit(context.Background(), admissionEvidence(), func(context.Context) error {
		order = append(order, "forward")
		return errForward
	})
	if !errors.Is(err, errForward) {
		t.Fatalf("erro=%v, esperado falha downstream", err)
	}
	if !reflect.DeepEqual(order, []string{"append", "forward"}) {
		t.Fatalf("ordem=%v, esperado Evidence durável antes da falha downstream", order)
	}
	if appender.calls != 1 {
		t.Fatalf("append chamado %d vezes, esperado 1", appender.calls)
	}
}

func TestAdmissionRejectsNilForwardBeforeAppend(t *testing.T) {
	appender := &appenderStub{}
	admission, err := NewAdmission(appender)
	if err != nil {
		t.Fatal(err)
	}

	err = admission.Admit(context.Background(), admissionEvidence(), nil)
	if err == nil {
		t.Fatal("Admit deveria rejeitar forward nil")
	}
	if appender.calls != 0 {
		t.Fatalf("append chamado %d vezes apesar de forward inválido", appender.calls)
	}
}

func TestNewAdmissionRejectsNilAppender(t *testing.T) {
	admission, err := NewAdmission(nil)
	if err == nil {
		t.Fatal("NewAdmission deveria rejeitar EvidenceAppender nil")
	}
	if admission != nil {
		t.Fatal("NewAdmission retornou instância com EvidenceAppender nil")
	}
}
