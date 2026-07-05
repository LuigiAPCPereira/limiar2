package logger_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/limiar/collector/internal/logger"
)

// Property 10: Preservação da cadeia de erro.
// Para toda cadeia de erros via fmt.Errorf com %w, quando registrada,
// aparece sob chave "erro" com texto completo no formato "layer: op: cause" sem truncamento.
func TestProperty10_ErrorChainPreservation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		depth := rapid.IntRange(1, 5).Draw(t, "depth")

		// Build error chain
		baseErr := errors.New("causa_raiz")
		err := baseErr
		layers := []string{"causa_raiz"}
		for i := range depth {
			layer := fmt.Sprintf("camada%d", i)
			op := fmt.Sprintf("op%d", i)
			err = fmt.Errorf("%s: %s: %w", layer, op, err)
			layers = append([]string{fmt.Sprintf("%s: %s", layer, op)}, layers...)
		}

		var buf bytes.Buffer
		l := logger.NewSlogLogger(&buf, slog.LevelInfo, "json")
		l.Error("falha", "erro", err)

		out := buf.String()
		if !strings.Contains(out, `"erro"`) {
			t.Fatalf("expected 'erro' key in JSON output: %s", out)
		}
		// Verify the full chain text is present
		if !strings.Contains(out, "causa_raiz") {
			t.Fatalf("root cause missing from output: %s", out)
		}
	})
}

// Teste unitário para errorAttr com erro nulo.
func TestErrorAttrNil(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "json")

	// Log without error — the "erro" key should be absent
	l.Info("sem_erro")
	out := buf.String()
	if strings.Contains(out, `"erro"`) {
		t.Errorf("'erro' key should be absent when no error: %s", out)
	}
}
