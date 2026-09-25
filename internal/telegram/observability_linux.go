//go:build linux

package telegram

import (
	"context"
	"errors"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
)

// EventType is a low-cardinality observability category.
// Events intentionally never include Telegram payloads, peer references,
// session bytes, phone numbers, OTPs, passwords, API hashes or raw errors.
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

// Event is safe-by-construction metadata for the Telegram boundary.
// IdentityKey is the local non-secret authorization alias, never Telegram self
// identity or credential material.
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

// Observer receives synchronous, low-cardinality events.
//
// Implementations must be fast and non-blocking. Observer panics are isolated
// so telemetry cannot take down the Telegram boundary. The boundary does not
// expose raw errors through this hook to prevent accidental secret/payload
// logging.
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
