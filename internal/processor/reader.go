package processor

import (
	"context"

	"github.com/limiar/collector/internal/model"
)

// Store reúne as operações de persistência necessárias pelo orquestrador do
// processor. A implementação concreta vive em internal/storage para manter todo
// SQL de produção dentro do pacote storage.
type Store interface {
	FetchUnprocessed(ctx context.Context, limit int) ([]*model.RawMessage, error)
	CountUnprocessed(ctx context.Context) (int64, error)
	SaveProcessed(ctx context.Context, msg *NormalizedMessage) error
	SaveProcessedBatch(ctx context.Context, msgs []*NormalizedMessage) (saved, failed int, err error)
	CrossChannelDuplicates(ctx context.Context, pairs map[string]int64) (map[string]bool, error)
	CleanExpiredPhotoCache(ctx context.Context) (int64, error)
}

// ProcessedReader reúne as queries de leitura sobre dados processados
// (processed_messages). O dashboard consome apenas esta interface read-only.
type ProcessedReader interface {
	// ListProcessedMessages retorna mensagens processadas com paginação e filtro
	// opcional por tipo e canal. Resultados ordenados por posted_at DESC.
	ListProcessedMessages(ctx context.Context, channelID int64, msgType string, limit, offset int) ([]*model.ProcessedMessage, error)

	// CountProcessedByType retorna contagem de mensagens processadas agrupadas por message_type.
	CountProcessedByType(ctx context.Context) ([]model.ProcessedTypeStats, error)

	// CountProcessedMessages retorna o total de mensagens processadas.
	CountProcessedMessages(ctx context.Context) (int64, error)

	// GetPhotoMetadata retorna os campos MTProto de imagem para uma mensagem processada.
	GetPhotoMetadata(ctx context.Context, processedMsgID int64) (*model.PhotoMetadata, error)

	// GetPhotoID retorna o photo_id (Telegram) de uma mensagem processada.
	// Retorna (0, nil) se a mensagem não existe ou não tem foto.
	GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error)

	// GetInlineThumb retorna o thumbnail inline (Type "i") da mensagem processada.
	// Retorna (nil, nil) se a mensagem não tem inline thumb ou não existe.
	GetInlineThumb(ctx context.Context, processedMsgID int64) ([]byte, error)

	// PhotoMetadataStats agrega a cobertura de metadados de foto.
	PhotoMetadataStats(ctx context.Context) (model.PhotoStats, error)

	// GetPhotoData retorna os bytes da foto em cache (photo_cache) pelo photo_id.
	// Retorna (nil, nil) quando não há entrada no cache.
	GetPhotoData(ctx context.Context, photoID int64) ([]byte, error)
}
