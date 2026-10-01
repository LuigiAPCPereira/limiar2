package telegram

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gotd/td/tgerr"
)

func TestClassifyTelegramErrorPreservesContext(t *testing.T) {
	t.Parallel()

	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		got := classifyTelegramError("history", err)
		if !errors.Is(got, err) {
			t.Fatalf("classify(%v)=%v", err, got)
		}
		var opErr *OperationError
		if errors.As(got, &opErr) {
			t.Fatalf("erro de contexto foi encapsulado indevidamente como OperationError: %+v", opErr)
		}
	}
}

func TestClassifyTelegramErrorFloodWait(t *testing.T) {
	t.Parallel()

	source := tgerr.New(420, "FLOOD_WAIT_17")
	got := classifyTelegramError("history", source)

	var opErr *OperationError
	if !errors.As(got, &opErr) {
		t.Fatalf("erro=%v; esperado OperationError", got)
	}
	if opErr.Kind != ErrorKindFloodWait || opErr.RetryAfter != 17*time.Second {
		t.Fatalf("OperationError=%+v", opErr)
	}
	if !errors.Is(got, source) {
		t.Fatal("erro de origem encapsulado não foi preservado")
	}
}

func TestClassifyTelegramErrorSemanticCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		esperado ErrorKind
	}{
		{"unauthorized", tgerr.New(401, "AUTH_KEY_UNREGISTERED"), ErrorKindUnauthorized},
		{"revoked", tgerr.New(401, "SESSION_REVOKED"), ErrorKindUnauthorized},
		{"duplicated auth key", tgerr.New(406, "AUTH_KEY_DUPLICATED"), ErrorKindUnauthorized},
		{"peer", tgerr.New(400, "PEER_ID_INVALID"), ErrorKindPeerUnavailable},
		{"username", tgerr.New(400, "USERNAME_NOT_OCCUPIED"), ErrorKindPeerUnavailable},
		{"access", tgerr.New(406, "CHANNEL_PRIVATE"), ErrorKindAccessDenied},
		{"server", tgerr.New(500, "INTERNAL"), ErrorKindTemporary},
		{"unknown-rpc", tgerr.New(400, "SOME_NEW_TELEGRAM_ERROR"), ErrorKindInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTelegramError("resolve_peer", tt.err)
			var opErr *OperationError
			if !errors.As(got, &opErr) {
				t.Fatalf("erro=%v; esperado OperationError", got)
			}
			if opErr.Kind != tt.esperado {
				t.Fatalf("kind=%q, esperado %q", opErr.Kind, tt.esperado)
			}
		})
	}
}

func TestClassifyTelegramErrorNetworkIsTemporary(t *testing.T) {
	t.Parallel()

	source := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("falha de rede simulada")}
	got := classifyTelegramError("history", source)

	var opErr *OperationError
	if !errors.As(got, &opErr) {
		t.Fatalf("erro=%v; esperado OperationError", got)
	}
	if opErr.Kind != ErrorKindTemporary {
		t.Fatalf("kind=%q; esperado temporary", opErr.Kind)
	}
}

func TestOperationErrorStringDoesNotEchoUpstreamPayload(t *testing.T) {
	t.Parallel()

	source := errors.New("detalhes-upstream-sensiveis")
	err := &OperationError{
		Operation: "history",
		Kind:      ErrorKindInternal,
		Err:       source,
	}
	if got := err.Error(); got != "telegram history: internal" {
		t.Fatalf("Error()=%q", got)
	}
	if !errors.Is(err, source) {
		t.Fatal("Unwrap() não preservou a origem")
	}
}
