package logger

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// levelConfig contém o nome de exibição e a cor ANSI para cada nível do slog.
var levelConfig = map[slog.Level]struct {
	name  string
	color string
}{
	slog.LevelDebug: {"DEBUG", "\x1b[90m"},
	slog.LevelInfo:  {"INFO ", "\x1b[36m"},
	slog.LevelWarn:  {"WARN ", "\x1b[33m"},
	slog.LevelError: {"ERROR", "\x1b[31m"},
}

const prettyResetCode = "\x1b[0m"
const fieldWidth = 14

// PrettyHandler escreve linhas de log coloridas amigáveis para leitura humana em um io.Writer.
// Seguro para uso concorrente: um mutex compartilhado serializa a escrita de cada registro.
type PrettyHandler struct {
	w         io.Writer
	mu        *sync.Mutex
	attrs     []slog.Attr
	groups    []string
	minLevel  slog.Level
	useColors bool
}

// asserção em tempo de compilação
var _ slog.Handler = (*PrettyHandler)(nil)

// NewPrettyHandler cria um PrettyHandler escrevendo em w.
// useColors controla a emissão de sequências ANSI.
func NewPrettyHandler(w io.Writer, minLevel slog.Level, useColors bool) *PrettyHandler {
	return &PrettyHandler{
		w:         w,
		mu:        &sync.Mutex{},
		minLevel:  minLevel,
		useColors: useColors,
	}
}

// Enabled relata se o handler lida com registros no nível fornecido.
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

// Handle formata um único registro slog em um bloco multilinhas bem formatado.
// Uma única operação Write sob mutex garante atomicidade do registro.
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < h.minLevel {
		return nil
	}

	cfg, ok := levelConfig[r.Level]
	if !ok {
		cfg = levelConfig[slog.LevelInfo]
	}

	var buf bytes.Buffer

	// linha principal: ◆ 15:04:05  NÍVEL   msg
	timeStr := r.Time.Format("15:04:05")
	if h.useColors {
		buf.WriteString(cfg.color)
	}
	buf.WriteString("◆ ")
	buf.WriteString(timeStr)
	buf.WriteString("  ")
	buf.WriteString(cfg.name)
	buf.WriteString("  ")
	if h.useColors {
		buf.WriteString(prettyResetCode)
	}
	buf.WriteString(r.Message)
	buf.WriteByte('\n')

	// atributos estáticos (pré-redigidos em WithAttrs) + atributos do registro
	allAttrs := make([]flatAttr, 0, len(h.attrs)+r.NumAttrs())
	for _, a := range h.attrs {
		collectFlatAttrs(h.groups, a, &allAttrs)
	}
	r.Attrs(func(a slog.Attr) bool {
		a = redactAttr(h.groups, a)
		collectFlatAttrs(h.groups, a, &allAttrs)
		return true
	})

	last := len(allAttrs) - 1
	for i, fa := range allAttrs {
		prefix := "├"
		if i == last {
			prefix = "└"
		}
		if h.useColors {
			buf.WriteString(cfg.color)
		}
		buf.WriteString(prefix)
		if h.useColors {
			buf.WriteString(prettyResetCode)
		}
		buf.WriteByte(' ')
		padRightRunes(&buf, fa.key, fieldWidth)
		buf.Write(fa.value)
		buf.WriteByte('\n')
	}

	h.mu.Lock()
	_, err := h.w.Write(buf.Bytes())
	h.mu.Unlock()
	return err
}

// WithAttrs retorna um novo handler que inclui os atributos fornecidos (pré-redigidos).
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	for i, a := range attrs {
		newAttrs[len(h.attrs)+i] = redactAttr(h.groups, a)
	}
	return &PrettyHandler{
		w:         h.w,
		mu:        h.mu,
		attrs:     newAttrs,
		groups:    h.groups,
		minLevel:  h.minLevel,
		useColors: h.useColors,
	}
}

// WithGroup retorna um novo handler com o nome de grupo empilhado.
// Grupos são renderizados em flat dotted notation (ex: auth.api_hash).
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &PrettyHandler{
		w:         h.w,
		mu:        h.mu,
		attrs:     h.attrs,
		groups:    newGroups,
		minLevel:  h.minLevel,
		useColors: h.useColors,
	}
}

// flatAttr representa um atributo já resolvido em chave pontuada e valor formatado.
type flatAttr struct {
	key   string
	value []byte
}

// collectFlatAttrs expande grupos em notação pontuada e redige recursivamente.
func collectFlatAttrs(groups []string, a slog.Attr, out *[]flatAttr) {
	if a.Value.Kind() == slog.KindGroup {
		subGroups := append(groups, a.Key)
		for _, ga := range a.Value.Group() {
			collectFlatAttrs(subGroups, ga, out)
		}
		return
	}
	a = redactAttr(groups, a)
	key := dottedKey(groups, a.Key)
	*out = append(*out, flatAttr{key: key, value: formatScalar(a.Value)})
}

// dottedKey constrói a chave em flat dotted notation a partir do caminho de grupos.
func dottedKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	var buf strings.Builder
	for _, g := range groups {
		buf.WriteString(g)
		buf.WriteByte('.')
	}
	buf.WriteString(key)
	return buf.String()
}

// padRightRunes preenche s com espaços até atingir a largura em runes.
// Garante no mínimo 2 espaços de separação entre chave e valor.
// Usa utf8.RuneCountInString para acomodar chaves pt-BR com caracteres multibyte.
func padRightRunes(buf *bytes.Buffer, s string, width int) {
	buf.WriteString(s)
	n := utf8.RuneCountInString(s)
	padding := max(width-n, 2)
	for range padding {
		buf.WriteByte(' ')
	}
}

// formatScalar formata um slog.Value para exibição sem reflexão.
func formatScalar(v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		return []byte(v.String())
	case slog.KindInt64:
		return strconv.AppendInt(nil, v.Int64(), 10)
	case slog.KindUint64:
		return strconv.AppendUint(nil, v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.AppendFloat(nil, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		return strconv.AppendBool(nil, v.Bool())
	case slog.KindDuration:
		return []byte(v.Duration().String())
	case slog.KindTime:
		return []byte(v.Time().Format(time.RFC3339))
	default:
		return []byte(v.String())
	}
}
