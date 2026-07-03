package telegram_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/limiar/collector/internal/telegram"
)

// countingHandler registra quantas atualizações ele recebeu.
type countingHandler struct {
	count atomic_Int64
	got   chan struct{}
}

func (h *countingHandler) HandleUpdate(_ context.Context, _ telegram.Update) error {
	h.count.Add(1)
	if h.got != nil {
		h.got <- struct{}{}
	}
	return nil
}

// panicHandler sempre levanta um panic, para verificar se o dispatcher isola e se recupera.
type panicHandler struct{}

func (panicHandler) HandleUpdate(_ context.Context, _ telegram.Update) error {
	panic("boom")
}


func TestDispatcherFansOutToAllHandlers(t *testing.T) {
	d := telegram.NewDispatcher(256, nil)
	h1 := &countingHandler{got: make(chan struct{}, 10)}
	h2 := &countingHandler{got: make(chan struct{}, 10)}
	d.Register(h1)
	d.Register(h2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	d.Dispatch(ctx, telegram.Update{ChannelID: 1, MessageID: 1, Payload: []byte(`{}`)})

	waitN(t, h1.got, 1)
	waitN(t, h2.got, 1)

	if h1.count.Load() != 1 || h2.count.Load() != 1 {
		t.Fatalf("fan-out failed: h1=%d h2=%d", h1.count.Load(), h2.count.Load())
	}
	_ = d.Shutdown(ctx)
}

func TestDispatcherDeliversManyUpdates(t *testing.T) {
	d := telegram.NewDispatcher(512, nil) // buffer > n para evitar descarte no envio não bloqueante
	h := &countingHandler{got: make(chan struct{}, 1000)}
	d.Register(h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	const n = 500
	for i := 0; i < n; i++ {
		d.Dispatch(ctx, telegram.Update{Payload: []byte(`{}`)})
	}
	waitN(t, h.got, n)
	if h.count.Load() != n {
		t.Fatalf("delivered %d, want %d", h.count.Load(), n)
	}
	_ = d.Shutdown(ctx)
}

// Um handler em panic não deve derrubar o dispatcher; os outros handlers continuam trabalhando.
func TestDispatcherRecoversFromHandlerPanic(t *testing.T) {
	d := telegram.NewDispatcher(256, nil)
	good := &countingHandler{got: make(chan struct{}, 10)}
	d.Register(panicHandler{})
	d.Register(good)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	d.Dispatch(ctx, telegram.Update{Payload: []byte(`{}`)})
	waitN(t, good.got, 1)

	if good.count.Load() != 1 {
		t.Fatalf("good handler should still receive despite sibling panic")
	}
	_ = d.Shutdown(ctx)
}

func TestDispatcherShutdownIsClean(t *testing.T) {
	d := telegram.NewDispatcher(256, nil)
	d.Register(&countingHandler{})
	ctx := context.Background()
	d.Start(ctx)
	if err := d.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// Dispatch concorrente a partir de múltiplos produtores deve ser livre de corrida (execute com -race).
func TestDispatcherConcurrentDispatch(t *testing.T) {
	d := telegram.NewDispatcher(1024, nil) // buffer > 4*250 para evitar descarte no envio não bloqueante
	h := &countingHandler{got: make(chan struct{}, 4000)}
	d.Register(h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	var wg sync.WaitGroup
	for p := 0; p < 4; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				d.Dispatch(ctx, telegram.Update{Payload: []byte(`{}`)})
			}
		}()
	}
	wg.Wait()
	waitN(t, h.got, 1000)
	_ = d.Shutdown(ctx)
}

// waitN bloqueia até que n sinais cheguem em ch ou o teste atinja o tempo limite (timeout).
func waitN(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("timed out waiting for signal %d/%d", i+1, n)
		}
	}
}
