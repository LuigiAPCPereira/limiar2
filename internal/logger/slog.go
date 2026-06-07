package logger

import (
	"io"
	"log/slog"
	"os"

	"golang.org/x/term"
)

// redactedKeys are attribute keys whose values must never reach log output.
var redactedKeys = map[string]struct{}{
	"api_hash":  {},
	"apihash":   {},
	"session":   {},
	"token":     {},
	"password":  {},
	"auth_code": {},
}

const redactedValue = "****"

// SlogLogger is the default Logger implementation, wrapping log/slog. This is
// the only place in the codebase where a concrete logger is instantiated.
type SlogLogger struct {
	inner *slog.Logger
}

// compile-time assertion that SlogLogger satisfies Logger.
var _ Logger = (*SlogLogger)(nil)

// NewSlogLogger builds a SlogLogger writing to w at the given level. format
// "json" selects a JSON handler; "pretty" selects a coloured human-readable
// handler (falls back to plain text when w is not a TTY); any other value
// selects a plain text handler. Sensitive attributes are redacted from output.
func NewSlogLogger(w io.Writer, level slog.Level, format string) *SlogLogger {
	opts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactSensitive,
	}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "pretty":
		if isTerminalWriter(w) {
			handler = NewPrettyHandler(w, level)
		} else {
			// Fallback to plain text when stdout is piped or redirected
			handler = slog.NewTextHandler(w, opts)
		}
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	inner := slog.New(handler)
	// Add service identity to every JSON log record
	if format == "json" {
		inner = inner.With("service", "limiar-collector")
	}
	return &SlogLogger{inner: inner}
}

// isTerminalWriter reports whether w is an *os.File backed by a terminal.
func isTerminalWriter(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// redactSensitive replaces values of known-sensitive keys with a fixed marker.
func redactSensitive(_ []string, a slog.Attr) slog.Attr {
	if _, ok := redactedKeys[a.Key]; ok {
		return slog.String(a.Key, redactedValue)
	}
	return a
}

// Debug logs at debug level.
func (l *SlogLogger) Debug(msg string, args ...any) { l.inner.Debug(msg, args...) }

// Info logs at info level.
func (l *SlogLogger) Info(msg string, args ...any) { l.inner.Info(msg, args...) }

// Warn logs at warn level.
func (l *SlogLogger) Warn(msg string, args ...any) { l.inner.Warn(msg, args...) }

// Error logs at error level.
func (l *SlogLogger) Error(msg string, args ...any) { l.inner.Error(msg, args...) }

// With returns a child SlogLogger carrying the given key/value pairs.
func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{inner: l.inner.With(args...)}
}

// WithComponent returns a child SlogLogger with the component name bound.
func (l *SlogLogger) WithComponent(name string) Logger {
	return &SlogLogger{inner: l.inner.With("component", name)}
}

// ParseLevel maps a config log level string to slog.Level, defaulting to Info
// for unrecognized values (Validate rejects those before this is reached).
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
