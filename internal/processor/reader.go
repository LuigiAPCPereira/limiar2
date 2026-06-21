package processor

import (
	"context"

	"github.com/limiar/collector/internal/model"
)

// ProcessedReader reúne as queries de leitura sobre dados processados (processed_messages).
// O dashboard e a CLI de mídia são os consumidores desta interface.
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

	// GetInlineThumb retorna o thumbnail inline (Type "i") de uma mensagem pelo photo_id.
	// Retorna (nil, nil) se a mensagem não tem inline thumb ou não existe.
	GetInlineThumb(ctx context.Context, photoID int64) ([]byte, error)

	// PhotoMetadataStats agrega a cobertura de metadados de foto.
	PhotoMetadataStats(ctx context.Context) (model.PhotoStats, error)
}

// Compile-time check: *Repository satisfaz ProcessedReader.
var _ ProcessedReader = (*Repository)(nil)
