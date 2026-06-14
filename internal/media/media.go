// Package media implementa o subsistema de resolução de imagens do Limiar.
//
// Arquitetura (ADR 011 v2):
//
//   - Inline Thumbnail (Type "i", ~230 bytes) é extraído pelo Processor do
//     payload bruto e persistido em processed_messages.inline_thumb (BLOB).
//     Cobertura: 100% das mensagens com foto. Source of truth universal.
//
//   - Full-res proativo: o Collector baixa o maior size (y/x) no momento da
//     chegada (janela onde file_reference MTProto é válido) e entrega ao
//     cache via ImageCache compartilhado (mesmo processo).
//
//   - Cache in-memory: github.com/hashicorp/golang-lru/v2/expirable.
//     LRU bounded (500 entradas) + TTL (30min). Compartilhado entre
//     Collector, Processor e API.
//
//   - API de mídia (Wave 4): fallback em cadeia:
//     1. cache.Get(photoID)    → 200 + X-Source: collector-cache
//     2. DB inline_thumb       → 200 + X-Source: inline-thumb
//     3. 404
//
// Constraints: zero escrita em disco para imagens. Collector é único ponto
// de download full-res. Inline thumb sempre disponível no DB.
//
// Este pacote NÃO importa gotd/td: a implementação concreta de DownloadPhoto
// sobre gotd vive em internal/telegram (AGENTS.md §14.2).
package media

import (
	"context"
)

// ImagePayload carrega os bytes de uma imagem baixada pelo Collector.
// Enviada do Collector para o cache compartilhado.
type ImagePayload struct {
	PhotoID int64
	Data    []byte
}

// MediaClient abstrai o download de imagens via MTProto.
// A implementação concreta sobre gotd/td vive em internal/telegram.
type MediaClient interface {
	// DownloadPhoto baixa os bytes da imagem usando os campos MTProto.
	// Usado pelo Collector no momento da chegada da mensagem (janela onde
	// file_reference é válido).
	DownloadPhoto(ctx context.Context, req PhotoDownloadRequest) ([]byte, error)
}

// PhotoDownloadRequest carrega os campos MTProto necessários para uma chamada
// upload.GetFile.
type PhotoDownloadRequest struct {
	PhotoID       int64
	AccessHash    int64
	FileReference []byte
	DCID          int
}

// MediaRepository abstrai o acesso ao banco para o subsistema de media.
type MediaRepository interface {
	// GetPhotoID retorna o photo_id (Telegram) de uma mensagem processada.
	// Retorna (0, nil) se a mensagem não existe ou não tem foto.
	GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error)
	// GetInlineThumb retorna o thumbnail inline (Type "i") de uma mensagem
	// pelo photo_id. Retorna (nil, nil) se não encontrado.
	GetInlineThumb(ctx context.Context, photoID int64) ([]byte, error)
}
