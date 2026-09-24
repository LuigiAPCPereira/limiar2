package logger

import (
	"fmt"
	"os"
)

// ResolveFormat decide o formato efetivo de log com base na
// variável de ambiente LIMIAR_LOG_FORMAT e na detecção de TTY.
//
// Regras:
//   - envFormat válido (pretty, text, json) → usa envFormat (precedência)
//   - envFormat inválido → fallback para text + warning em stderr
//   - envFormat ausente + TTY → pretty
//   - envFormat ausente + non-TTY → text
func ResolveFormat(envFormat string, logIsTTY bool) string {
	if envFormat != "" {
		switch envFormat {
		case "pretty", "text", "json":
			return envFormat
		default:
			fmt.Fprintf(os.Stderr, "limiar: LIMIAR_LOG_FORMAT=%q inválido; usando \"text\"\n", envFormat)
			return "text"
		}
	}

	if logIsTTY {
		return "pretty"
	}
	return "text"
}
