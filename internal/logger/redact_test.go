package logger_test

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/limiar/collector/internal/logger"
)

// Property 1: Redação universal de chaves sensíveis.
// Para todo formato, nível, chave sensível em qualquer variação de caixa,
// tipo escalar e profundidade de grupo, a saída contém **** e não contém o valor original.
func TestProperty1_RedactionUniversal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		format := rapid.SampledFrom([]string{"json", "text", "pretty"}).Draw(t, "format")
		sensitiveKey := rapid.SampledFrom([]string{"api_hash", "API_HASH", "Api_Hash", "session", "TOKEN", "password", "auth_code"}).Draw(t, "key")
		secret := rapid.StringMatching(`[A-Za-z0-9]{6,40}`).Draw(t, "secret")

		var buf bytes.Buffer
		l := logger.NewSlogLogger(&buf, slog.LevelDebug, format)
		l.Info("evento", sensitiveKey, secret)

		out := buf.String()
		if strings.Contains(out, secret) {
			t.Fatalf("format=%s key=%s: leaked secret %q in output: %s", format, sensitiveKey, secret, out)
		}
		if !strings.Contains(out, "****") {
			t.Fatalf("format=%s key=%s: expected **** in output: %s", format, sensitiveKey, out)
		}
	})
}

// Property 2: Preservação de chave e transparência de não sensíveis.
// Para todo atributo, a redação preserva a chave; para toda chave não sensível, o valor é inalterado.
func TestProperty2_PreserveKeyAndTransparent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		format := rapid.SampledFrom([]string{"json", "text", "pretty"}).Draw(t, "format")
		key := rapid.StringMatching(`[a-z_]{3,20}`).Draw(t, "key")
		value := rapid.StringMatching(`[A-Za-z0-9 ]{1,30}`).Draw(t, "value")

		// Skip if key happens to be sensitive
		lowerKey := strings.ToLower(key)
		if slices.Contains([]string{"api_hash", "apihash", "session", "token", "password", "auth_code", "secret", "api_key", "apikey"}, lowerKey) {
			return
		}

		var buf bytes.Buffer
		l := logger.NewSlogLogger(&buf, slog.LevelInfo, format)
		l.Info("evento", key, value)

		out := buf.String()
		if !strings.Contains(out, key) {
			t.Fatalf("format=%s: key %q missing from output: %s", format, key, out)
		}
		if !strings.Contains(out, value) {
			t.Fatalf("format=%s: value %q missing from output: %s", format, value, out)
		}
	})
}
