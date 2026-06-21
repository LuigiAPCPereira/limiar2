package logger

import (
	"io"
	"log/slog"
)

// SlogLogger é a implementação padrão do Logger, envolvendo (wrapping) log/slog. Este é
// o único lugar na base de código onde um logger concreto é instanciado.
type SlogLogger struct {
	inner *slog.Logger
	level slog.Level // nível mínimo para emitir logs
}

// asserção em tempo de compilação de que SlogLogger satisfaz Logger.
var _ Logger = (*SlogLogger)(nil)

// NewSlogLogger constrói um SlogLogger escrevendo para w no nível (level) fornecido. format
// "json" seleciona um handler JSON; "pretty" seleciona um handler legível e colorido;
// qualquer outro valor seleciona um handler de texto simples. Atributos sensíveis são
// redigidos via redactAttr em todos os formatos.
func NewSlogLogger(w io.Writer, level slog.Level, format string) *SlogLogger {
	opts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactAttr,
	}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "pretty":
		useColors := IsTerminalWriter(w) && !NoColorEnvSet()
		handler = NewPrettyHandler(w, level, useColors)
	default:
		handler = slog.NewTextHandler(w, opts)
	}
	return &SlogLogger{inner: slog.New(handler), level: level}
}

// Debug faz o log em nível debug.
func (l *SlogLogger) Debug(msg string, args ...any) { l.inner.Debug(msg, args...) }

// Info faz o log em nível info.
func (l *SlogLogger) Info(msg string, args ...any) { l.inner.Info(msg, args...) }

// Warn faz o log em nível warn.
func (l *SlogLogger) Warn(msg string, args ...any) { l.inner.Warn(msg, args...) }

// Error faz o log em nível error.
func (l *SlogLogger) Error(msg string, args ...any) { l.inner.Error(msg, args...) }

// With retorna um SlogLogger filho (child) contendo os pares chave/valor fornecidos.
func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{inner: l.inner.With(args...), level: l.level}
}

// WithComponent retorna um SlogLogger filho com o nome do componente vinculado.
func (l *SlogLogger) WithComponent(name string) Logger {
	return &SlogLogger{inner: l.inner.With(attrKeyComponent, name), level: l.level}
}

// IsInfoEnabled retorna true se mensagens Info são emitidas (level <= Info).
func (l *SlogLogger) IsInfoEnabled() bool { return l.level <= slog.LevelInfo }
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
