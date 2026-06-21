package telegram

import (
	"context"
	"fmt"
	"sync"

	"github.com/limiar/collector/internal/logger"
)

// Update é uma atualização do Telegram com metadados de roteamento extraídos (channelID,
// messageID) e o payload JSON bruto. O dispatcher transporta Updates do
// callback do gotd para os handlers registrados.
type Update struct {
	ChannelID int64
	MessageID int64
	Payload   []byte
}

// UpdateHandler é o contrato Observer: um consumidor de atualizações do Telegram.
// As implementações devem ser seguras para uso concorrente.
type UpdateHandler interface {
	HandleUpdate(ctx context.Context, update Update) error
}

// Dispatcher é o Observer que distribui (fan-out) as atualizações recebidas para todos
// os handlers registrados. Cada handler possui um canal com buffer drenado por uma
// goroutine dedicada, de forma que um handler lento não consiga bloquear os demais.
// Panics nos handlers são recuperados e registrados no limite da goroutine.
type Dispatcher struct {
	bufferSize int
	log        logger.Logger

	handlers []UpdateHandler
	chans    []chan Update
	wg       sync.WaitGroup

	mu      sync.Mutex
	started bool
}

// NewDispatcher cria um dispatcher cujos canais por handler comportam bufferSize
// atualizações. log pode ser nil (tratado como NopLogger). Registre os handlers antes
// de chamar Start.
func NewDispatcher(bufferSize int, log logger.Logger) *Dispatcher {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Dispatcher{bufferSize: bufferSize, log: log}
}

// Register adiciona um handler. Deve ser chamado antes de Start.
func (d *Dispatcher) Register(h UpdateHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = append(d.handlers, h)
}

// Start inicia uma goroutine consumidora por handler registrado. Chamar Start
// mais de uma vez não tem efeito (no-op).
func (d *Dispatcher) Start(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return
	}
	d.started = true

	d.chans = make([]chan Update, len(d.handlers))
	for i, h := range d.handlers {
		ch := make(chan Update, d.bufferSize)
		d.chans[i] = ch
		d.wg.Add(1)
		go d.consume(ctx, h, ch)
	}
}

// consume drena o canal de um handler até que ele seja fechado, recuperando-se de qualquer
// panic que o handler levante, para que uma única atualização ruim não cause falha (crash) no processo.
func (d *Dispatcher) consume(ctx context.Context, h UpdateHandler, ch <-chan Update) {
	defer d.wg.Done()
	for update := range ch {
		d.invoke(ctx, h, update)
	}
}

// invoke chama um handler com recuperação de panic e registro (logging) de erro.
func (d *Dispatcher) invoke(ctx context.Context, h UpdateHandler, update Update) {
	defer func() {
		if r := recover(); r != nil {
			// Evitar logar o objeto de panic bruto para não vazar a sessão.
			// Formatamos como erro para preservar a string do panic sem imprimir
			// os conteúdos literais que o objeto poderia conter caso fosse impresso
			// pela reflexão do logger.
			d.log.Error("💥 Panic no handler recuperado", "erro", fmt.Errorf("panic: %v", r))
		}
	}()
	if err := h.HandleUpdate(ctx, update); err != nil {
		d.log.Error("❌ Handler retornou erro", "erro", err)
	}
}

// Dispatch entrega uma atualização para o canal de cada handler via envio não bloqueante.
// Se o buffer de um handler estiver cheio, o update é descartado para esse handler com
// aviso de log, sem bloquear os demais handlers (evita cascata de bloqueio).
func (d *Dispatcher) Dispatch(ctx context.Context, update Update) {
	for i, ch := range d.chans {
		select {
		case ch <- update:
		case <-ctx.Done():
			return
		default:
			d.log.Warn("⚠️ Buffer do handler cheio, update descartado", "handler", i)
		}
	}
}

// Shutdown fecha todos os canais de handler e aguarda que toda goroutine consumidora
// drene seu buffer e saia.
func (d *Dispatcher) Shutdown(_ context.Context) error {
	d.mu.Lock()
	if !d.started {
		d.mu.Unlock()
		return nil
	}
	chans := d.chans
	d.chans = nil
	d.started = false
	d.mu.Unlock()

	for _, ch := range chans {
		close(ch)
	}
	d.wg.Wait()
	return nil
}
