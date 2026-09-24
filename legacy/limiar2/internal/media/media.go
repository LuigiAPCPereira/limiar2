// Package media implementa o subsistema de resolução de imagens do Limiar.
//
// Arquitetura (ADR 011 v3):
//
//   - Inline Thumbnail (Type "i", ~230 bytes) é extraído pelo Processor do
//     payload bruto e persistido em processed_messages.inline_thumb (BLOB).
//     Cobertura universal para mensagens com foto, mas baixa qualidade.
//
//   - Full-res proativo: o Collector baixa a variante "x" (~800px) no momento da
//     chegada (janela onde file_reference MTProto é válido) e entrega ao
//     Cache compartilhado (mesmo processo).
//
//   - Cache persistente: photo_cache guarda JPEG 800px no Turso por photo_id,
//     com TTL fixo e cleanup periódico pelo Processor.
//
//   - API de mídia: fallback em cadeia por processed_messages.id:
//     1. cache.Get(photoID exato) → 200 + X-Source: cache
//     2. repo.GetPhotoData        → 200 + X-Source: photo-cache
//     3. client.DownloadPhoto     → 200 + X-Source: downloaded (apenas quando client != nil)
//     4. repo.GetInlineThumb      → 200 + X-Source: inline-thumb
//     5. ErrNoPhoto               → 404
//
// Constraints: zero arquivos soltos de imagem. Collector/MediaClient é o único
// ponto de download full-res. Dashboard não importa internal/telegram e passa client nil.
//
// Este pacote NÃO importa gotd/td: a implementação concreta de DownloadPhoto
// sobre gotd vive em internal/telegram (AGENTS.md §14.2).
package media

import (
	"context"

	"github.com/limiar/collector/internal/model"
)

// Client abstrai o download de imagens via MTProto.
// A implementação concreta sobre gotd/td vive em internal/telegram.
type Client interface {
	// DownloadPhoto baixa os bytes da imagem usando os campos MTProto.
	// Abre uma conexão por chamada. Para bulk, use DownloadPhotoBatch.
	DownloadPhoto(ctx context.Context, req PhotoDownloadRequest) ([]byte, error)
	// DownloadPhotoBatch baixa múltiplas fotos numa única conexão Telegram.
	// Implementações podem baixar em paralelo; handler não deve depender da ordem
	// de chegada. Muito mais rápido que N chamadas a DownloadPhoto.
	DownloadPhotoBatch(ctx context.Context, reqs []PhotoDownloadRequest, handler func(photoID int64, data []byte, err error)) error
}

// PhotoDownloadRequest carrega os campos MTProto necessários para uma chamada
// upload.GetFile. ChannelID e MessageID são opcionais: quando preenchidos permitem
// renovar o file_reference re-buscando a mensagem do canal (FILE_REFERENCE_EXPIRED).
type PhotoDownloadRequest struct {
	PhotoID       int64
	AccessHash    int64
	FileReference []byte
	DCID          int
	ChannelID     int64 // opcional: canal de origem para renovação de file_reference
	MessageID     int64 // opcional: message_id no canal para renovação
}

// Repository abstrai o acesso ao banco para o subsistema de media.
type Repository interface {
	// GetPhotoID retorna o photo_id (Telegram) de uma mensagem processada.
	// Retorna (0, nil) se a mensagem não existe ou não tem foto.
	GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error)
	// GetInlineThumb retorna o thumbnail inline (Type "i") da mensagem
	// processada informada. Retorna (nil, nil) se não encontrado.
	GetInlineThumb(ctx context.Context, processedMsgID int64) ([]byte, error)
	// GetPhotoData retorna os bytes da foto em cache (photo_cache) pelo photo_id.
	// Retorna (nil, nil) quando não há entrada no cache.
	GetPhotoData(ctx context.Context, photoID int64) ([]byte, error)
	// SavePhotoData persiste (upsert) os bytes de uma foto no cache pelo photo_id.
	SavePhotoData(ctx context.Context, photoID int64, data []byte) error
	// GetPhotoMetadata retorna os campos MTProto de imagem para uma mensagem processada.
	// Usado pelo Resolver para montar InputPhotoFileLocation no download sob demanda.
	GetPhotoMetadata(ctx context.Context, processedMsgID int64) (*model.PhotoMetadata, error)
}
