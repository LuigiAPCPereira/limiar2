package collector_test

import (
	"context"
	"testing"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/telegram"
)

func BenchmarkHandleUpdate(b *testing.B) {
	writeCh := make(chan collector.WriteJob, b.N)
	monitored := map[int64]struct{}{1: {}}
	h := collector.NewMessageHandler(collector.NoopClassifier{}, writeCh, monitored, logger.NopLogger{})
	ctx := context.Background()

	update := telegram.Update{
		ChannelID: 1,
		MessageID: 42,
		Payload:   []byte(`{"test":true}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.HandleUpdate(ctx, update)
	}
}
