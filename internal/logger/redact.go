package logger

import (
	"log/slog"
	"strings"
)

// redactedKeys são chaves de atributo cujos valores nunca devem aparecer na saída.
// A comparação é feita sem distinção de maiúsculas/minúsculas (case-insensitive).
var redactedKeys = map[string]struct{}{
	"api_hash":  {},
	"apihash":   {},
	"session":   {},
	"token":     {},
	"password":  {},
	"auth_code": {},
}

const redactedValue = "****"

// redactAttr é a transformação canônica de redação, compartilhada por todos os formatos.
// Substitui o valor de chaves sensíveis por redactedValue, preservando a chave original.
// A comparação de chave é case-insensitive. Atributos não sensíveis são retornados sem alteração.
// Grupos são recursivamente processados para redigir folhas sensíveis em qualquer profundidade.
func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		redacted := make([]slog.Attr, len(attrs))
		for i, ga := range attrs {
			redacted[i] = redactAttr(nil, ga)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redacted...)}
	}
	if _, ok := redactedKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, redactedValue)
	}
	return a
}
