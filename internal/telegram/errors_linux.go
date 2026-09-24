//go:build linux

package telegram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/gotd/td/tgerr"
)

type ErrorKind string

const (
	ErrorKindUnauthorized   ErrorKind = "unauthorized"
	ErrorKindPeerUnavailable ErrorKind = "peer_unavailable"
	ErrorKindAccessDenied    ErrorKind = "access_denied"
	ErrorKindFloodWait       ErrorKind = "flood_wait"
	ErrorKindTemporary       ErrorKind = "temporary"
	ErrorKindInternal        ErrorKind = "internal"
)

// OperationError is the stable semantic error surface exposed by the Telegram
// boundary. The wrapped upstream error remains available through errors.Is/As.
type OperationError struct {
	Operation  string
	Kind       ErrorKind
	RetryAfter time.Duration
	Err        error
}

func (e *OperationError) Error() string {
	if e == nil {
		return "telegram operation error"
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("telegram %s: %s (retry after %s)", e.Operation, e.Kind, e.RetryAfter)
	}
	return fmt.Sprintf("telegram %s: %s", e.Operation, e.Kind)
}

func (e *OperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func classifyTelegramError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	if wait, ok := tgerr.AsFloodWait(err); ok {
		return &OperationError{
			Operation:  operation,
			Kind:       ErrorKindFloodWait,
			RetryAfter: wait,
			Err:        err,
		}
	}

	if tgerr.Is(err,
		"AUTH_KEY_UNREGISTERED",
		"AUTH_KEY_INVALID",
		"SESSION_REVOKED",
		"SESSION_EXPIRED",
		"USER_DEACTIVATED",
		"USER_DEACTIVATED_BAN",
	) {
		return &OperationError{Operation: operation, Kind: ErrorKindUnauthorized, Err: err}
	}

	if tgerr.Is(err,
		"PEER_ID_INVALID",
		"USERNAME_INVALID",
		"USERNAME_NOT_OCCUPIED",
		"CHANNEL_INVALID",
		"CHAT_ID_INVALID",
		"USER_ID_INVALID",
	) {
		return &OperationError{Operation: operation, Kind: ErrorKindPeerUnavailable, Err: err}
	}

	if tgerr.Is(err,
		"CHANNEL_PRIVATE",
		"CHAT_ADMIN_REQUIRED",
		"USER_PRIVACY_RESTRICTED",
		"CHAT_WRITE_FORBIDDEN",
	) {
		return &OperationError{Operation: operation, Kind: ErrorKindAccessDenied, Err: err}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return &OperationError{Operation: operation, Kind: ErrorKindTemporary, Err: err}
	}
	if rpcErr, ok := tgerr.As(err); ok && rpcErr.Code >= 500 {
		return &OperationError{Operation: operation, Kind: ErrorKindTemporary, Err: err}
	}

	return &OperationError{Operation: operation, Kind: ErrorKindInternal, Err: err}
}
