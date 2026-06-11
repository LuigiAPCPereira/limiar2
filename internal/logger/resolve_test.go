package logger_test

import (
	"testing"

	"github.com/limiar/collector/internal/logger"
)

// Property 9: Precedência de formato e fallback [TABELA].
func TestResolveFormat(t *testing.T) {
	cases := []struct {
		name        string
		subcommand  string
		envFormat   string
		isTTY       bool
		wantFormat  string
		wantWarn    bool
	}{
		// env ausente + qualquer subcomando + TTY → pretty
		{"run_tty", "run", "", true, "pretty", false},
		{"auth_tty", "auth", "", true, "pretty", false},
		{"channels_tty", "channels", "", true, "pretty", false},
		{"dashboard_tty", "dashboard", "", true, "pretty", false},
		{"processor_tty", "processor", "", true, "pretty", false},
		// env ausente + qualquer subcomando + non-TTY → text
		{"run_notty", "run", "", false, "text", false},
		{"auth_notty", "auth", "", false, "text", false},
		{"channels_notty", "channels", "", false, "text", false},
		{"dashboard_notty", "dashboard", "", false, "text", false},
		{"processor_notty", "processor", "", false, "text", false},
		// env válido tem precedência
		{"run_env_json", "run", "json", true, "json", false},
		{"run_env_text", "run", "text", true, "text", false},
		{"run_env_pretty", "run", "pretty", false, "pretty", false},
		{"auth_env_json", "auth", "json", true, "json", false},
		{"channels_env_text", "channels", "text", true, "text", false},
		{"dashboard_env_pretty", "dashboard", "pretty", false, "pretty", false},
		// env inválido → text + warn
		{"run_env_invalid", "run", "yaml", true, "text", true},
		{"auth_env_invalid", "auth", "xml", false, "text", true},
		{"channels_env_typo", "channels", "pretto", true, "text", true},
		{"dashboard_env_empty_val", "dashboard", " ", false, "text", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format, warn := logger.ResolveFormat(tc.subcommand, tc.envFormat, tc.isTTY)
			if format != tc.wantFormat {
				t.Errorf("ResolveFormat(%q, %q, %v) format = %q, want %q",
					tc.subcommand, tc.envFormat, tc.isTTY, format, tc.wantFormat)
			}
			if warn != tc.wantWarn {
				t.Errorf("ResolveFormat(%q, %q, %v) warn = %v, want %v",
					tc.subcommand, tc.envFormat, tc.isTTY, warn, tc.wantWarn)
			}
		})
	}
}
