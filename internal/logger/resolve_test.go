package logger_test

import (
	"testing"

	"github.com/limiar/collector/internal/logger"
)

// Property 9: Precedência de formato e fallback [TABELA].
func TestResolveFormat(t *testing.T) {
	cases := []struct {
		name       string
		envFormat  string
		isTTY      bool
		wantFormat string
	}{
		// env ausente + TTY → pretty
		{"tty", "", true, "pretty"},
		// env ausente + non-TTY → text
		{"notty", "", false, "text"},
		// env válido tem precedência
		{"env_json", "json", true, "json"},
		{"env_text", "text", true, "text"},
		{"env_pretty", "pretty", false, "pretty"},
		// env inválido → fallback para text
		{"env_invalid", "yaml", true, "text"},
		{"env_typo", "pretto", true, "text"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format := logger.ResolveFormat(tc.envFormat, tc.isTTY)
			if format != tc.wantFormat {
				t.Errorf("ResolveFormat(%q, %v) = %q, want %q",
					tc.envFormat, tc.isTTY, format, tc.wantFormat)
			}
		})
	}
}
