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
