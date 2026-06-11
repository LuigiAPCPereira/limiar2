package logger_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/limiar/collector/internal/logger"
)

// Property 3: Injetividade de prefixos do TerminalPresenter [TABELA].
// Para todo par de MessageType distintos, prefixos distintos; pertencentes ao conjunto AGENTS.md.
func TestPresenterPrefixInjectivity(t *testing.T) {
	allowedPrefixes := map[string]bool{
		"📡 ": true, "📩 ": true, "📜 ": true, "🔄 ": true,
		"❌ ": true, "✅ ": true, "🛑 ": true, "⏰ ": true,
		"🌐 ": true, "⚠️  ": true,
	}

	type testCase struct {
		name    string
		emit    func(p logger.Presenter)
		prefix  string
	}

	cases := []testCase{
		{"Info", func(p logger.Presenter) { p.Info("teste") }, "🌐 "},
		{"Success", func(p logger.Presenter) { p.Success("teste") }, "✅ "},
		{"Warning", func(p logger.Presenter) { p.Warning("teste") }, "⚠️  "},
		{"Error", func(p logger.Presenter) { p.Error("teste") }, "❌ "},
		{"Step", func(p logger.Presenter) { p.Step("teste") }, "🔄 "},
	}

	// Check injectivity: all prefixes distinct
	prefixSet := make(map[string]string)
	for _, tc := range cases {
		if existing, ok := prefixSet[tc.prefix]; ok {
			t.Errorf("prefix %q shared by %s and %s", tc.prefix, existing, tc.name)
		}
		prefixSet[tc.prefix] = tc.name

		if !allowedPrefixes[tc.prefix] {
			t.Errorf("%s: prefix %q not in AGENTS.md allowed set", tc.name, tc.prefix)
		}
	}

	// Check each method emits the correct prefix
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := logger.NewTerminalPresenter(&buf, false)
			tc.emit(p)
			out := buf.String()
			if !strings.Contains(out, tc.prefix) {
				t.Errorf("expected prefix %q in output: %s", tc.prefix, out)
			}
			if !strings.Contains(out, "teste") {
				t.Errorf("expected text 'teste' in output: %s", out)
			}
		})
	}
}

// Property 4: Supressão de ANSI preservando prefixo e texto [TABELA].
// Para todo MessageType e todo texto sem nova linha, quando useColors=false,
// a saída não contém \x1b e contém o prefixo e o texto completo.
func TestPresenterANSISuppression(t *testing.T) {
	types := []struct {
		name string
		emit func(p logger.Presenter)
	}{
		{"Info", func(p logger.Presenter) { p.Info("msg teste") }},
		{"Success", func(p logger.Presenter) { p.Success("msg teste") }},
		{"Warning", func(p logger.Presenter) { p.Warning("msg teste") }},
		{"Error", func(p logger.Presenter) { p.Error("msg teste") }},
		{"Step", func(p logger.Presenter) { p.Step("msg teste") }},
	}

	for _, tc := range types {
		t.Run(tc.name+"_noColor", func(t *testing.T) {
			var buf bytes.Buffer
			p := logger.NewTerminalPresenter(&buf, false)
			tc.emit(p)
			out := buf.String()
			if strings.Contains(out, "\x1b") {
				t.Errorf("ANSI escape found with useColors=false: %q", out)
			}
			if !strings.Contains(out, "msg teste") {
				t.Errorf("text missing from output: %q", out)
			}
			if !strings.HasSuffix(out, "\n") {
				t.Errorf("output not terminated by newline: %q", out)
			}
			// Count newlines — must be exactly one
			if strings.Count(out, "\n") != 1 {
				t.Errorf("expected exactly 1 newline, got %d: %q", strings.Count(out, "\n"), out)
			}
		})

		t.Run(tc.name+"_withColor", func(t *testing.T) {
			var buf bytes.Buffer
			p := logger.NewTerminalPresenter(&buf, true)
			tc.emit(p)
			out := buf.String()
			if !strings.Contains(out, "\x1b[") {
				t.Errorf("ANSI escape missing with useColors=true: %q", out)
			}
			if !strings.Contains(out, "msg teste") {
				t.Errorf("text missing from output: %q", out)
			}
		})
	}
}
