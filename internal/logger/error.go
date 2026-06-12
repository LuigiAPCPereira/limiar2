package logger

import "log/slog"

// errorAttr produz o atributo de erro sob a chave estável attrKeyError ("erro").
// Retorna ok=false quando err é nil, para que o chamador possa omitir o atributo.
// Preserva o texto completo da cadeia "layer: op: cause" sem truncar.
//nolint:unused
func errorAttr(err error) (slog.Attr, bool) {
	if err == nil {
		return slog.Attr{}, false
	}
	return slog.String(attrKeyError, err.Error()), true
}
