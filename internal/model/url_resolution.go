package model

import "time"

// URLResolution representa uma resolução cacheada de URL feita pelo processor.
// OriginalURL é a chave estável do cache; CanonicalURL preserva uma versão limpa
// sem tracking/affiliate params. Unresolved marca timeouts/bloqueios/falhas sem
// impedir o processamento da mensagem.
type URLResolution struct {
	OriginalURL  string
	CanonicalURL string
	Merchant     string
	Title        string
	ResolvedAt   time.Time
	Unresolved   bool
}
