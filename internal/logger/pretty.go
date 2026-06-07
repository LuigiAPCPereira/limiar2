package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// levelConfig holds the display name and ANSI colour for each slog level.
var levelConfig = map[slog.Level]struct {
	name  string
	color string
}{
	slog.LevelDebug: {"DEBUG", "\x1b[90m"},   // grey
	slog.LevelInfo:  {"INFO ", "\x1b[36m"},  // cyan
	slog.LevelWarn:  {"WARN ", "\x1b[33m"},  // yellow
	slog.LevelError: {"ERROR", "\x1b[31m"},  // red
}

const resetCode = "\x1b[0m"
const fieldWidth = 14 // width for aligned field names

// PrettyHandler writes human-friendly coloured log lines to an io.Writer.
type PrettyHandler struct {
	w         io.Writer
	attrs     []slog.Attr
	minLevel  slog.Level
	useColors bool
}

// compile-time assertion
var _ slog.Handler = (*PrettyHandler)(nil)

// NewPrettyHandler creates a PrettyHandler writing to w. If w is not a TTY,
// colours are disabled.
func NewPrettyHandler(w io.Writer, minLevel slog.Level) *PrettyHandler {
	return &PrettyHandler{
		w:         w,
		minLevel:  minLevel,
		useColors: isTerminalWriter(w),
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

// Handle formats a single slog record into a pretty multi-line block.
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	cfg, ok := levelConfig[r.Level]
	if !ok {
		cfg = levelConfig[slog.LevelInfo]
	}

	var sb strings.Builder

	// collect all attributes: static first, then record attrs
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// main line: ◆ 15:04:05  LEVEL   msg
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

	// fields
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

	// blank line between entries
	sb.WriteString("\n")

	_, err := h.w.Write([]byte(sb.String()))
	return err
}

// WithAttrs returns a new handler that includes the given attributes.
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

// WithGroup returns a new handler with the given group name.
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	// Groups are flattened for simplicity in this pretty handler
	return h
}

// padRight pads s with spaces on the right until it reaches width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// formatValue formats a slog.Value for display.
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

