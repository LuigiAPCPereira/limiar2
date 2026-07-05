package collector

import (
	"testing"

	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/telegram"
)

// BenchmarkHandleUpdate mede alocações por chamada no hot path do MessageHandler.
// IsInfoEnabled() evitando alocações variádicas quando o logger é NopLogger.
func BenchmarkHandleUpdate(b *testing.B) {
	writeCh := make(chan WriteJob, 128)
	ctx := b.Context()

	// Drena o canal em background para não bloquear o benchmark.
	go func() {
		for {
			select {
			case <-writeCh:
			case <-ctx.Done():
				return
			}
		}
	}()

	monitored := map[int64]struct{}{1: {}}
	h := NewMessageHandler(NoopClassifier{}, writeCh, monitored, logger.NopLogger{})

	update := telegram.Update{
		ChannelID: 1,
		MessageID: 42,
		Payload:   []byte(`{"example":true}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.HandleUpdate(ctx, update)
	}
}
