package telegram

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

var testExtractMessagesUpdate = &tg.MessagesChannelMessages{
	Messages: make([]tg.MessageClass, 1000),
}

func init() {
	for i := 0; i < 1000; i++ {
		testExtractMessagesUpdate.Messages[i] = &tg.Message{
			ID:      i,
			Date:    int(time.Now().Unix()),
			Message: "hello this is a long enough text to benchmark serialization and allocations accurately",
		}
	}
}

func BenchmarkExtractMessages(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = extractMessages(testExtractMessagesUpdate)
	}
}
