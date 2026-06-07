// Package errors defines the limiar-collector error domain: named sentinel
// errors that callers match with errors.Is, and a single Wrap helper that
// adds layer/operation context while preserving the wrapped error's identity.
//
// Layers must wrap every error that crosses a package boundary via Wrap so
// that messages read "layer: operation: cause" and sentinel identity is
// retained through arbitrary nesting.
package errors

import (
	stderrors "errors"
	"fmt"
)

// Sentinel errors for the collector domain. Callers compare against these
// with errors.Is rather than matching on message strings.
var (
	// ErrNotAuthenticated indicates no valid Telegram session is persisted.
	ErrNotAuthenticated = stderrors.New("not authenticated")
	// ErrChannelNotFound indicates a requested channel is absent from storage.
	ErrChannelNotFound = stderrors.New("channel not found")
	// ErrSessionCorrupted indicates a stored session could not be decoded.
	ErrSessionCorrupted = stderrors.New("session corrupted")
	// ErrDBWriteFailed indicates a database write failed after all retries.
	ErrDBWriteFailed = stderrors.New("database write failed")
	// ErrMaxRetriesExceeded indicates a retry loop exhausted its budget.
	ErrMaxRetriesExceeded = stderrors.New("max retries exceeded")
)

// Wrap annotates err with the originating layer and operation, preserving the
// wrapped error for errors.Is/errors.As. It returns nil when err is nil so it
// can be used directly in return statements.
//
// The resulting message has the form "layer: op: cause".
func Wrap(layer, op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s: %w", layer, op, err)
}
