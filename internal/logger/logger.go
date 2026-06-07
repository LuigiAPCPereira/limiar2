// Package logger defines the Logger interface injected into every layer of
// limiar-collector and a default slog-backed implementation.
//
// The interface keeps call sites decoupled from the logging backend: swapping
// slog for zerolog or zap later requires changing only this package. No other
// package may instantiate a concrete logger.
package logger

// Logger is the logging contract injected into every layer via constructors.
// Implementations must be safe for concurrent use.
type Logger interface {
	// Debug logs at debug level. args are alternating key/value pairs.
	Debug(msg string, args ...any)
	// Info logs at info level. args are alternating key/value pairs.
	Info(msg string, args ...any)
	// Warn logs at warn level. args are alternating key/value pairs.
	Warn(msg string, args ...any)
	// Error logs at error level. args are alternating key/value pairs.
	Error(msg string, args ...any)
	// With returns a child Logger that includes the given key/value pairs in
	// every subsequent record.
	With(args ...any) Logger
	// WithComponent returns a child Logger with the component name bound.
	WithComponent(name string) Logger
}
