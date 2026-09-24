package logger_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/limiar/collector/internal/logger"
)

func BenchmarkPrettyHandler(b *testing.B) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "pretty")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		l.Info("mensagem de benchmark",
			"canal_id", 1001234567890,
			"component", "collector",
			"msg_id", i,
		)
	}
}

func BenchmarkJSONHandler(b *testing.B) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "json")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		l.Info("mensagem de benchmark",
			"canal_id", 1001234567890,
			"component", "collector",
			"msg_id", i,
		)
	}
}
