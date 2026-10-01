package telegram

import (
	"context"
	"errors"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
)

// EventType é uma categoria de observabilidade de baixa cardinalidade.
// Os eventos intencionalmente nunca incluem payloads do Telegram, referências de peer,
// bytes de sessão, telefones, OTPs, senhas, hashes de API ou erros brutos.
type EventType string

const (
	EventTypeRuntimeState EventType = "runtime_state"
	EventTypeOperation    EventType = "operation"
)

type RuntimeState string

const (
	RuntimeStateStarting RuntimeState = "starting"
	RuntimeStateReady    RuntimeState = "ready"
	RuntimeStateStopped  RuntimeState = "stopped"
	RuntimeStateFailed   RuntimeState = "failed"
)

type EventOutcome string

const (
	EventOutcomeOK       EventOutcome = "ok"
	EventOutcomeCanceled EventOutcome = "canceled"
	EventOutcomeDeadline EventOutcome = "deadline"
	EventOutcomeError    EventOutcome = "error"
)

// Event contém metadados seguros por construção para o boundary do Telegram.
// IdentityKey é o alias local e não secreto da autorização; nunca representa a identidade
// própria do Telegram nem material de credencial.
type Event struct {
	Type        EventType
	IdentityKey string

	State     RuntimeState
	Operation string
	Outcome   EventOutcome
	ErrorKind ErrorKind

	Duration   time.Duration
	RetryAfter time.Duration
}

// Observer recebe eventos síncronos e de baixa cardinalidade.
//
// Implementações devem ser rápidas e não bloqueantes. Panics do Observer são isolados
// para que telemetria não derrube o boundary do Telegram. O boundary não expõe erros
// brutos por este hook, evitando registro acidental de segredos ou payloads.
type Observer func(Event)

func observe(observer Observer, event Event) {
	if observer == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	observer(event)
}

func eventOutcome(err error) (EventOutcome, ErrorKind, time.Duration) {
	if err == nil {
		return EventOutcomeOK, "", 0
	}
	if errors.Is(err, context.Canceled) {
		return EventOutcomeCanceled, "", 0
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return EventOutcomeDeadline, "", 0
	}

	var opErr *OperationError
	if errors.As(err, &opErr) {
		return EventOutcomeError, opErr.Kind, opErr.RetryAfter
	}
	return EventOutcomeError, "", 0
}


type observedSessionStorage struct {
	delegate    gotdtelegram.SessionStorage
	identityKey string
	observer    Observer
}

func observeSessionStorage(storage gotdtelegram.SessionStorage, identityKey string, observer Observer) gotdtelegram.SessionStorage {
	if storage == nil || observer == nil {
		return storage
	}
	return &observedSessionStorage{
		delegate:    storage,
		identityKey: identityKey,
		observer:    observer,
	}
}

func (s *observedSessionStorage) LoadSession(ctx context.Context) (data []byte, retErr error) {
	startedAt := time.Now()
	defer func() {
		s.observeStorageOperation("session_load", startedAt, retErr)
	}()
	return s.delegate.LoadSession(ctx)
}

func (s *observedSessionStorage) StoreSession(ctx context.Context, data []byte) (retErr error) {
	startedAt := time.Now()
	defer func() {
		s.observeStorageOperation("session_store", startedAt, retErr)
	}()
	return s.delegate.StoreSession(ctx, data)
}

func (s *observedSessionStorage) observeStorageOperation(operation string, startedAt time.Time, err error) {
	if s == nil {
		return
	}
	outcome, kind, retryAfter := eventOutcome(err)
	observe(s.observer, Event{
		Type:        EventTypeOperation,
		IdentityKey: s.identityKey,
		Operation:   operation,
		Outcome:     outcome,
		ErrorKind:   kind,
		Duration:    time.Since(startedAt),
		RetryAfter:  retryAfter,
	})
}
