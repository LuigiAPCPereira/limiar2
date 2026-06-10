package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// levelConfig contém o nome de exibição e a cor ANSI para cada nível do slog.
var levelConfig = map[slog.Level]struct {
	name  string
	color string
}{
	slog.LevelDebug: {"DEBUG", "\x1b[90m"}, // grey
	slog.LevelInfo:  {"INFO ", "\x1b[36m"}, // cyan
	slog.LevelWarn:  {"WARN ", "\x1b[33m"}, // yellow
	slog.LevelError: {"ERROR", "\x1b[31m"}, // red
}

const resetCode = "\x1b[0m"
const fieldWidth = 14 // largura para alinhar nomes de campos

// PrettyHandler escreve linhas de log coloridas amigáveis para leitura humana em um io.Writer.
type PrettyHandler struct {
	w         io.Writer
	attrs     []slog.Attr
	minLevel  slog.Level
	useColors bool
}

// asserção em tempo de compilação
var _ slog.Handler = (*PrettyHandler)(nil)

// NewPrettyHandler cria um PrettyHandler escrevendo em w. Se w não for um TTY,
// as cores são desativadas.
func NewPrettyHandler(w io.Writer, minLevel slog.Level) *PrettyHandler {
	return &PrettyHandler{
		w:         w,
		minLevel:  minLevel,
		useColors: isTerminalWriter(w),
	}
}

// Enabled relata se o handler lida com registros no nível fornecido.
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

// Handle formata um único registro slog em um bloco multilinhas bem formatado.
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	cfg, ok := levelConfig[r.Level]
	if !ok {
		cfg = levelConfig[slog.LevelInfo]
	}

	var sb strings.Builder

	// coleta todos os atributos: estáticos primeiro, depois os atributos do registro
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// linha principal: ◆ 15:04:05  NÍVEL   msg
	timeStr := r.Time.Format("15:04:05")
	if h.useColors {
		sb.WriteString(cfg.color)
	}
	sb.WriteString("◆ ")
	sb.WriteString(timeStr)
	sb.WriteString("  ")
	sb.WriteString(cfg.name)
	sb.WriteString("  ")
	if h.useColors {
		sb.WriteString(resetCode)
	}
	sb.WriteString(r.Message)
	sb.WriteString("\n")

	// campos
	for i, a := range allAttrs {
		prefix := "├"
		if i == len(allAttrs)-1 {
			prefix = "└"
		}
		if h.useColors {
			sb.WriteString(cfg.color)
		}
		sb.WriteString(prefix)
		if h.useColors {
			sb.WriteString(resetCode)
		}
		sb.WriteString(" ")
		sb.WriteString(padRight(a.Key, fieldWidth))
		sb.WriteString(formatValue(a.Value))
		sb.WriteString("\n")
	}

	// linha em branco entre os registros
	sb.WriteString("\n")

	_, err := h.w.Write([]byte(sb.String()))
	return err
}

// WithAttrs retorna um novo handler que inclui os atributos fornecidos.
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &PrettyHandler{
		w:         h.w,
		attrs:     newAttrs,
		minLevel:  h.minLevel,
		useColors: h.useColors,
	}
}

// WithGroup retorna um novo handler com o nome de grupo fornecido.
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	// Grupos são achatados (flattened) por simplicidade neste pretty handler
	return h
}

// padRight preenche s com espaços à direita até que atinja a largura (width).
func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// formatValue formata um slog.Value para exibição.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return fmt.Sprintf("%d", v.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", v.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", v.Float64())
	case slog.KindBool:
		return fmt.Sprintf("%t", v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", v.Any())
	}
}
