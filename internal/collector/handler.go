package collector

import (
	"context"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// schemaVersion é a versão do schema do payload marcada em toda mensagem
// capturada na Fase 1.
const schemaVersion = 1

// WriteJob é uma unidade de trabalho para o DBWriter: uma mensagem a persistir e um
// canal de erro opcional para notificação de falha de gravação. Backfill é verdadeiro (true)
// para jobs produzidos pelo loop de backfill do histórico; o dbWriter pula o avanço
// do cursor para esses jobs porque o orquestrador do backfill avança o cursor
// uma vez por canal com o id máximo verdadeiro (evitando a corrida da ordem descendente).
type WriteJob struct {
	Message  *storage.RawMessage
	ErrCh    chan<- error
	Backfill bool
}

// MessageHandler é o Adaptador e Observador: ele converte um update bruto em uma
// RawMessage e a encaminha para o DBWriter via writeCh. É sem estado (stateless) e
// seguro para invocação concorrente sem locks.
type MessageHandler struct {
	classifier        Classifier
	writeCh           chan<- WriteJob
	log               logger.Logger
	monitoredChannels map[int64]struct{}
}

// NewMessageHandler constrói um handler que classifica usando o classifier e emite
// jobs para writeCh. monitoredChannels é um conjunto (set) de IDs de canais a aceitar; atualizações
// de outros canais são silenciosamente descartadas. log pode ser nil (tratado como NopLogger).
func NewMessageHandler(classifier Classifier, writeCh chan<- WriteJob, monitoredChannels map[int64]struct{}, log logger.Logger) *MessageHandler {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &MessageHandler{
		classifier:        classifier,
		writeCh:           writeCh,
		log:               log,
		monitoredChannels: monitoredChannels,
	}
}

// HandleUpdate adapta o update bruto (raw) em uma RawMessage e a enfileira
// para persistência. Os bytes originais do payload são preservados literalmente.
// Atualizações de canais não monitorados são descartadas silenciosamente.
func (h *MessageHandler) HandleUpdate(ctx context.Context, update telegram.Update) error {
	// Filtra atualizações de canais não monitorados. channel_id=0 cobre mensagens diretas (DMs),
	// eventos do sistema e qualquer atualização que não seja uma mensagem de canal — todos
	// os quais devem ser descartados na Fase 1 (a coleção é restrita apenas aos canais
	// monitorados).
	if _, ok := h.monitoredChannels[update.ChannelID]; !ok {
		return nil
	}

	msg := &storage.RawMessage{
		ChannelID:     update.ChannelID,
		MessageID:     update.MessageID,
		Payload:       update.Payload,
		ReceivedAt:    time.Now().UTC(),
		SchemaVersion: schemaVersion,
	}

	classified, err := h.classifier.Classify(ctx, msg)
	if err != nil {
		return apperrors.Wrap("collector", "classify", err)
	}

	h.log.Info("📩 Mensagem ao vivo", "canal_id", update.ChannelID, "msg_id", update.MessageID)

	select {
	case h.writeCh <- WriteJob{Message: classified}:
		return nil
	case <-ctx.Done():
		return apperrors.Wrap("collector", "handle_update", ctx.Err())
	}
}
