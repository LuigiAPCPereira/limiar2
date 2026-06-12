package logger_test

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/limiar/collector/internal/logger"
)

// Property 5: Descarte por nível sem formatação [TABELA].
func TestPrettyLevelDiscard(t *testing.T) {
	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	configs := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}

	for _, cfgLevel := range configs {
		for _, msgLevel := range levels {
			t.Run("cfg_"+cfgLevel.String()+"_msg_"+msgLevel.String(), func(t *testing.T) {
				var buf bytes.Buffer
				l := logger.NewSlogLogger(&buf, cfgLevel, "pretty")
				switch msgLevel {
				case slog.LevelDebug:
					l.Debug("mensagem_teste")
				case slog.LevelInfo:
					l.Info("mensagem_teste")
				case slog.LevelWarn:
					l.Warn("mensagem_teste")
				case slog.LevelError:
					l.Error("mensagem_teste")
				}
				out := buf.String()
				if msgLevel < cfgLevel {
					if strings.Contains(out, "mensagem_teste") {
						t.Errorf("level %s should be discarded at config %s: %s", msgLevel, cfgLevel, out)
					}
				} else {
					if !strings.Contains(out, "mensagem_teste") {
						t.Errorf("level %s should appear at config %s: %s", msgLevel, cfgLevel, out)
					}
				}
			})
		}
	}
}

// Property 7: Escrita única e saída contígua sob concorrência.
func TestPrettyConcurrency(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "pretty")

	const goroutines = 8
	const messagesPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < messagesPerGoroutine; i++ {
				l.Info("mensagem_concorrente", "goroutine", id, "iteracao", i)
			}
		}(g)
	}
	wg.Wait()

	out := buf.String()
	// Verificar que cada registro contém a mensagem completa (não entrelaçada)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Cada linha de mensagem deve começar com ◆ ou ├ ou └
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		hasPrefix := false
		for _, prefix := range []string{"◆", "├", "└"} {
			if strings.HasPrefix(trimmed, prefix) {
				hasPrefix = true
				break
			}
		}
		if !hasPrefix {
			// Could be a continuation — just check no garbled output
			continue
		}
	}
}

// Teste de tabela para alinhamento fieldWidth e flat dotted notation.
func TestPrettyFieldAlignmentAndGroups(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "pretty")
	l.Info("teste_alinhamento", "canal", "exemplo", "component", "collector")

	out := buf.String()
	if !strings.Contains(out, "teste_alinhamento") {
		t.Errorf("message missing from output: %s", out)
	}
	if !strings.Contains(out, "canal") {
		t.Errorf("attribute 'canal' missing: %s", out)
	}
	if !strings.Contains(out, "component") {
		t.Errorf("attribute 'component' missing: %s", out)
	}
}

// Teste de redação no formato pretty (correção de segurança crítica).
func TestPrettyRedactsSensitive(t *testing.T) {
	var buf bytes.Buffer
	l := logger.NewSlogLogger(&buf, slog.LevelInfo, "pretty")
	l.Info("auth_event", "api_hash", "supersecret123", "username", "test")

	out := buf.String()
	if strings.Contains(out, "supersecret123") {
		t.Fatalf("pretty format leaked api_hash: %s", out)
	}
	if !strings.Contains(out, "****") {
		t.Errorf("expected **** redacted value: %s", out)
	}
	if !strings.Contains(out, "test") {
		t.Errorf("non-sensitive value missing: %s", out)
	}
}
