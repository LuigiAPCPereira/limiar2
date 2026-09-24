package logger

import (
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// Presenter emite Mensagens_de_Apresentação ao operador humano.
// Implementações devem emitir uma linha lógica por chamada, terminada por '\n'.
type Presenter interface {
	Info(text string)
	Success(text string)
	Warning(text string)
	Error(text string)
	Step(text string)
}

// MessageType classifica semanticamente uma mensagem de apresentação.
type MessageType int

const (
	MsgInfo    MessageType = iota
	MsgSuccess
	MsgWarning
	MsgError
	MsgStep
)

// Mapeamento fixo de prefixos e cores ANSI por MessageType.
// Emojis restritos ao conjunto definido no AGENTS.md.
var (
	presenterPrefixes = [5]string{"🌐 ", "✅ ", "⚠️  ", "❌ ", "🔄 "}
	presenterColors   = [5]string{"\x1b[36m", "\x1b[32m", "\x1b[33m", "\x1b[31m", "\x1b[90m"}
	presenterReset    = "\x1b[0m"
)

// TerminalPresenter é a implementação concreta de Presenter para terminais.
// Escreve mensagens formatadas com prefixo de emoji e cor ANSI condicional.
// Thread-safe: mu serializa escritas no writer compartilhado.
type TerminalPresenter struct {
	mu        sync.Mutex
	w         io.Writer
	useColors bool
}

// asserção em tempo de compilação de que TerminalPresenter satisfaz Presenter.
var _ Presenter = (*TerminalPresenter)(nil)

// NewTerminalPresenter constrói um TerminalPresenter escrevendo em w.
// Quando useColors é false, as sequências ANSI são suprimidas.
func NewTerminalPresenter(w io.Writer, useColors bool) *TerminalPresenter {
	return &TerminalPresenter{w: w, useColors: useColors}
}

func (t *TerminalPresenter) Info(text string)    { t.emit(MsgInfo, text) }
func (t *TerminalPresenter) Success(text string) { t.emit(MsgSuccess, text) }
func (t *TerminalPresenter) Warning(text string) { t.emit(MsgWarning, text) }
func (t *TerminalPresenter) Error(text string)   { t.emit(MsgError, text) }
func (t *TerminalPresenter) Step(text string)    { t.emit(MsgStep, text) }

func (t *TerminalPresenter) emit(mt MessageType, text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var buf [512]byte
	b := buf[:0]
	if t.useColors {
		b = append(b, presenterColors[mt]...)
	}
	b = append(b, presenterPrefixes[mt]...)
	if t.useColors {
		b = append(b, presenterReset...)
	}
	b = append(b, text...)
	b = append(b, '\n')
	_, _ = t.w.Write(b)
}

// IsTerminalWriter relata se w é um *os.File apoiado por um terminal (TTY).
func IsTerminalWriter(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// NoColorEnvSet relata se a variável de ambiente NO_COLOR está definida
// (presente com qualquer valor, inclusive vazio).
func NoColorEnvSet() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}
