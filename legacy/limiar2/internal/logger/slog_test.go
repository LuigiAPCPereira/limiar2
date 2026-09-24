package logger_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/limiar/collector/internal/logger"
)

func TestJSONFormatProducesJSON(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "json")
	l.Info("hello", "key", "value")
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Errorf("expected JSON output with msg field, got: %s", out)
	}
}

func TestTextFormatProducesText(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "text")
	l.Info("hello", "key", "value")
	out := buf.String()
	if strings.Contains(out, `"msg"`) {
		t.Errorf("text format should not produce JSON, got: %s", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected message in text output, got: %s", out)
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelWarn, "json")
	l.Info("suppressed")
	l.Warn("shown")
	out := buf.String()
	if strings.Contains(out, "suppressed") {
		t.Errorf("info should be filtered at warn level: %s", out)
	}
	if !strings.Contains(out, "shown") {
		t.Errorf("warn should pass at warn level: %s", out)
	}
}

// Funcionalidade: limiar-collector, Propriedade 14 (metade do logger): Ocultação de Dados Sensíveis (Sensitive Data Masking).
// Para qualquer valor de api_hash / session / token, nenhum registro de log o conterá literalmente.
func TestProperty14LoggerRedactsSensitiveKeys(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret := rapid.StringMatching(`[A-Za-z0-9]{6,40}`).Draw(t, "secret")
		key := rapid.SampledFrom([]string{"api_hash", "session", "token", "password"}).Draw(t, "key")

		var buf bytes.Buffer
		l := logger.NewSlogLogger(&buf, slog.LevelDebug, "json")
		l.Info("event", key, secret)

		if strings.Contains(buf.String(), secret) {
			t.Fatalf("logger leaked sensitive %s=%q: %s", key, secret, buf.String())
		}
	})
}

func TestWithAddsContextFields(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "json").With("component", "collector")
	l.Info("started")
	if !strings.Contains(buf.String(), `"component":"collector"`) {
		t.Errorf("With() context field missing: %s", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"bogus": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := logger.ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}
