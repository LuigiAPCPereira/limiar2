package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/limiar/collector/internal/logger"
)

type noopHandler struct{}

func (n *noopHandler) HandleUpdate(ctx context.Context, u Update) error {
	return nil
}

func BenchmarkDispatcher(b *testing.B) {
	d := NewDispatcher(1024, logger.NopLogger{})
	d.Register(&noopHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	u := Update{
		ChannelID: 1,
		MessageID: 2,
		Payload:   []byte(`{}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(ctx, u)
	}

	// Wait a tiny bit for consumer to drain
	time.Sleep(10 * time.Millisecond)
}

func BenchmarkDispatcher_DispatchFullBuffer(b *testing.B) {
	d := NewDispatcher(1, logger.NopLogger{})
	d.Register(&noopHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Preenche o buffer
	d.chans = []chan Update{make(chan Update, 1)}
	d.chans[0] <- Update{}

	u := Update{
		ChannelID: 1,
		MessageID: 2,
		Payload:   []byte(`{}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(ctx, u)
	}
}
