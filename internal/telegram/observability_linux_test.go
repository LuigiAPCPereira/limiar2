//go:build linux

package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	querymessages "github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestRuntimeObserverEmitsLifecycleWithoutSecrets(t *testing.T) {
	t.Parallel()

	var events []Event
	r := newRuntimeForTest(
		AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 998877665544},
		time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 998877665544}, nil
		},
	)
	r.observer = func(event Event) {
		events = append(events, event)
	}

	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); err != nil {
		t.Fatalf("Run() error=%v", err)
	}

	if len(events) != 3 {
		t.Fatalf("events=%+v, want starting/ready/stopped", events)
	}
	if events[0].State != RuntimeStateStarting ||
		events[1].State != RuntimeStateReady ||
		events[2].State != RuntimeStateStopped {
		t.Fatalf("unexpected lifecycle order: %+v", events)
	}
	for _, event := range events {
		if event.IdentityKey != "primary" {
			t.Fatalf("IdentityKey=%q, want primary", event.IdentityKey)
		}
		rendered := fmt.Sprintf("%+v", event)
		if strings.Contains(rendered, "998877665544") {
			t.Fatalf("event leaked Telegram self user id: %s", rendered)
		}
	}
	if events[2].Outcome != EventOutcomeOK {
		t.Fatalf("stopped outcome=%q, want ok", events[2].Outcome)
	}
}

func TestRuntimeObserverEmitsFailedOnReadinessFailure(t *testing.T) {
	t.Parallel()

	var events []Event
	r := newRuntimeForTest(
		AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42},
		time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{}, nil
		},
	)
	r.observer = func(event Event) {
		events = append(events, event)
	}

	err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil })
	if !errors.Is(err, ErrRebootstrapRequired) {
		t.Fatalf("Run() error=%v, want ErrRebootstrapRequired", err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%+v, want starting/failed", events)
	}
	if events[0].State != RuntimeStateStarting || events[1].State != RuntimeStateFailed {
		t.Fatalf("unexpected events=%+v", events)
	}
	if events[1].Outcome != EventOutcomeError {
		t.Fatalf("failed outcome=%q, want error", events[1].Outcome)
	}
}

func TestQueryObserverDoesNotExposeReferenceOrRawError(t *testing.T) {
	t.Parallel()

	const (
		sensitiveRef = "@private-offers-source"
		sensitiveErr = "raw-upstream-sensitive-details"
	)
	q, err := newQueryClientWithFuncs(
		1,
		10,
		8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return nil, errors.New(sensitiveErr)
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	q.identityKey = "primary"

	var events []Event
	q.observer = func(event Event) {
		events = append(events, event)
	}

	if _, err := q.ResolvePeer(context.Background(), PeerRef{Value: sensitiveRef}); err == nil {
		t.Fatal("ResolvePeer() error=nil, want failure")
	}
	if len(events) != 1 {
		t.Fatalf("events=%+v, want one operation event", events)
	}
	event := events[0]
	if event.Type != EventTypeOperation || event.Operation != "resolve_peer" {
		t.Fatalf("event=%+v", event)
	}
	if event.Outcome != EventOutcomeError || event.ErrorKind != ErrorKindInternal {
		t.Fatalf("event outcome/class=%+v", event)
	}
	rendered := fmt.Sprintf("%+v", event)
	if strings.Contains(rendered, sensitiveRef) || strings.Contains(rendered, sensitiveErr) {
		t.Fatalf("observer event leaked request/upstream error: %s", rendered)
	}
}

func TestQueryObserverPreservesFloodMetadataOnly(t *testing.T) {
	t.Parallel()

	q, err := newQueryClientWithFuncs(
		1,
		10,
		8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 77}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return nil, tgerr.New(420, "FLOOD_WAIT_17")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	q.identityKey = "primary"

	var events []Event
	q.observer = func(event Event) {
		events = append(events, event)
	}

	desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"})
	if err != nil {
		t.Fatal(err)
	}
	events = nil

	if _, err := q.History(context.Background(), desc.Key, HistoryRequest{Limit: 1}); err == nil {
		t.Fatal("History() error=nil, want FLOOD_WAIT")
	}
	if len(events) != 1 {
		t.Fatalf("events=%+v, want one history event", events)
	}
	event := events[0]
	if event.Operation != "history" ||
		event.ErrorKind != ErrorKindFloodWait ||
		event.RetryAfter != 17*time.Second ||
		event.Outcome != EventOutcomeError {
		t.Fatalf("event=%+v", event)
	}
}

func TestObserverPanicDoesNotBreakTelegramBoundary(t *testing.T) {
	t.Parallel()

	observe(func(Event) {
		panic("telemetry backend failed")
	}, Event{Type: EventTypeRuntimeState, State: RuntimeStateStarting})
}


type observerSessionStorage struct {
	data     []byte
	loadErr  error
	storeErr error
}

func (s *observerSessionStorage) LoadSession(context.Context) ([]byte, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return append([]byte(nil), s.data...), nil
}

func (s *observerSessionStorage) StoreSession(_ context.Context, data []byte) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.data = append(s.data[:0], data...)
	return nil
}

func TestObservedSessionStorageNeverExposesBytesOrRawError(t *testing.T) {
	t.Parallel()

	const (
		sessionSecret = "opaque-auth-key-material"
		rawError      = "filesystem-path-and-sensitive-details"
	)
	base := &observerSessionStorage{data: []byte(sessionSecret)}
	var events []Event
	storage := observeSessionStorage(base, "primary", func(event Event) {
		events = append(events, event)
	})

	got, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sessionSecret {
		t.Fatalf("LoadSession()=%q", got)
	}
	if err := storage.StoreSession(context.Background(), []byte(sessionSecret+"-new")); err != nil {
		t.Fatal(err)
	}

	base.loadErr = errors.New(rawError)
	if _, err := storage.LoadSession(context.Background()); err == nil {
		t.Fatal("LoadSession() error=nil, want failure")
	}

	if len(events) != 3 {
		t.Fatalf("events=%+v, want load/store/failed-load", events)
	}
	if events[0].Operation != "session_load" ||
		events[1].Operation != "session_store" ||
		events[2].Operation != "session_load" {
		t.Fatalf("operations=%+v", events)
	}
	if events[2].Outcome != EventOutcomeError {
		t.Fatalf("failed load outcome=%q, want error", events[2].Outcome)
	}
	for _, event := range events {
		rendered := fmt.Sprintf("%+v", event)
		if strings.Contains(rendered, sessionSecret) || strings.Contains(rendered, rawError) {
			t.Fatalf("session observer leaked sensitive material: %s", rendered)
		}
	}
}

func TestEventOutcomePreservesCancellationCategories(t *testing.T) {
	t.Parallel()

	outcome, kind, retry := eventOutcome(context.Canceled)
	if outcome != EventOutcomeCanceled || kind != "" || retry != 0 {
		t.Fatalf("canceled=(%q,%q,%s)", outcome, kind, retry)
	}
	outcome, kind, retry = eventOutcome(context.DeadlineExceeded)
	if outcome != EventOutcomeDeadline || kind != "" || retry != 0 {
		t.Fatalf("deadline=(%q,%q,%s)", outcome, kind, retry)
	}
}
