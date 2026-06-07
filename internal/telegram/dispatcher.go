package telegram

import (
	"context"
	"sync"

	"github.com/limiar/collector/internal/logger"
)

// Update is a Telegram update with extracted routing metadata (channelID,
// messageID) and the raw JSON payload. The dispatcher transports Updates from
// the gotd callback to registered handlers.
type Update struct {
	ChannelID int64
	MessageID int64
	Payload   []byte
}

// UpdateHandler is the Observer contract: a consumer of Telegram updates.
// Implementations must be safe for concurrent use.
type UpdateHandler interface {
	HandleUpdate(ctx context.Context, update Update) error
}

// Dispatcher is the Observer that fans incoming updates out to every
// registered handler. Each handler owns a buffered channel drained by a
// dedicated goroutine, so a slow handler cannot block its siblings. Handler
// panics are recovered and logged at the goroutine boundary.
type Dispatcher struct {
	bufferSize int
	log        logger.Logger

	handlers []UpdateHandler
	chans    []chan Update
	wg       sync.WaitGroup

	mu      sync.Mutex
	started bool
}

// NewDispatcher creates a dispatcher whose per-handler channels hold bufferSize
// updates. log may be nil (treated as NopLogger). Register handlers before
// calling Start.
func NewDispatcher(bufferSize int, log logger.Logger) *Dispatcher {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Dispatcher{bufferSize: bufferSize, log: log}
}

// Register adds a handler. It must be called before Start.
func (d *Dispatcher) Register(h UpdateHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = append(d.handlers, h)
}

// Start launches one consumer goroutine per registered handler. Calling Start
// more than once is a no-op.
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

// consume drains one handler's channel until it is closed, recovering from any
// panic the handler raises so a single bad update cannot crash the process.
func (d *Dispatcher) consume(ctx context.Context, h UpdateHandler, ch <-chan Update) {
	defer d.wg.Done()
	for update := range ch {
		d.invoke(ctx, h, update)
	}
}

// invoke calls a handler with panic recovery and error logging.
func (d *Dispatcher) invoke(ctx context.Context, h UpdateHandler, update Update) {
	defer func() {
		if r := recover(); r != nil {
			d.log.Error("💥 Panic no handler recuperado", "panic", r)
		}
	}()
	if err := h.HandleUpdate(ctx, update); err != nil {
		d.log.Error("❌ Handler retornou erro", "erro", err)
	}
}

// Dispatch delivers an update to every handler's channel. It returns once the
// update is enqueued for all handlers (blocking only if a buffer is full).
func (d *Dispatcher) Dispatch(ctx context.Context, update Update) {
	for _, ch := range d.chans {
		select {
		case ch <- update:
		case <-ctx.Done():
			return
		}
	}
}

// Shutdown closes all handler channels and waits for every consumer goroutine
// to drain its buffer and exit.
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
