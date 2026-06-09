package logger

import (
	"io"
	"log/slog"
	"os"

	"golang.org/x/term"
)

// redactedKeys são chaves de atributos cujos valores nunca devem chegar à saída do log.
var redactedKeys = map[string]struct{}{
	"api_hash":  {},
	"apihash":   {},
	"session":   {},
	"token":     {},
	"password":  {},
	"auth_code": {},
}

const redactedValue = "****"

// SlogLogger é a implementação padrão do Logger, envolvendo (wrapping) log/slog. Este é
// o único lugar na base de código onde um logger concreto é instanciado.
type SlogLogger struct {
	inner *slog.Logger
}

// asserção em tempo de compilação de que SlogLogger satisfaz Logger.
var _ Logger = (*SlogLogger)(nil)

// NewSlogLogger constrói um SlogLogger escrevendo para w no nível (level) fornecido. format
// "json" seleciona um handler JSON; "pretty" seleciona um handler legível e colorido
// (faz fallback para texto simples quando w não é um TTY); qualquer outro valor
// seleciona um handler de texto simples. Atributos sensíveis são omitidos (redacted) da saída.
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
			// Fallback para texto simples quando stdout é canalizado ou redirecionado
			handler = slog.NewTextHandler(w, opts)
		}
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	inner := slog.New(handler)
	// Adiciona a identidade do serviço a cada registro de log JSON
	if format == "json" {
		inner = inner.With("service", "limiar-collector")
	}
	return &SlogLogger{inner: inner}
}

// isTerminalWriter relata se w é um *os.File apoiado por um terminal.
func isTerminalWriter(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// redactSensitive substitui valores de chaves sabidamente sensíveis por um marcador fixo.
func redactSensitive(_ []string, a slog.Attr) slog.Attr {
	if _, ok := redactedKeys[a.Key]; ok {
		return slog.String(a.Key, redactedValue)
	}
	return a
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
	return &SlogLogger{inner: l.inner.With(args...)}
}

// WithComponent retorna um SlogLogger filho com o nome do componente vinculado.
func (l *SlogLogger) WithComponent(name string) Logger {
	return &SlogLogger{inner: l.inner.With("component", name)}
}

// ParseLevel mapeia uma string de nível de log da configuração para slog.Level, definindo como padrão Info
// para valores não reconhecidos (Validate rejeita esses valores antes que este ponto seja alcançado).
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
