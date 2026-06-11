package logger

// ResolveFormat decide o formato efetivo de log com base no subcomando,
// na variável de ambiente LIMIAR_LOG_FORMAT e na detecção de TTY.
//
// Regras:
//   - envFormat válido (pretty, text, json) → usa envFormat (precedência)
//   - envFormat inválido → text + warnInvalid=true
//   - envFormat ausente + TTY → pretty
//   - envFormat ausente + non-TTY → text
func ResolveFormat(_ string, envFormat string, logIsTTY bool) (format string, warnInvalid bool) {
	if envFormat != "" {
		switch envFormat {
		case "pretty", "text", "json":
			return envFormat, false
		default:
			return "text", true
		}
	}

	if logIsTTY {
		return "pretty", false
	}
	return "text", false
}
