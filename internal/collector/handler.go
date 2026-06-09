package collector

import (
	"context"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// schemaVersion is the payload schema version stamped on every captured
// message in Phase 1.
const schemaVersion = 1

// WriteJob is a unit of work for the DBWriter: a message to persist and an
// optional error channel for write-failure notification. Backfill is true for
// jobs produced by the history backfill loop; the dbWriter skips cursor
// advancement for those because the backfill orchestrator advances the cursor
// once per channel with the true max id (avoiding the descending-order race).
type WriteJob struct {
	Message  *storage.RawMessage
	ErrCh    chan<- error
	Backfill bool
}

// MessageHandler is the Adapter and Observer: it converts a raw update into a
// RawMessage and forwards it to the DBWriter via writeCh. It is stateless and
// safe for concurrent invocation without locks.
type MessageHandler struct {
	classifier        Classifier
	writeCh           chan<- WriteJob
	log               logger.Logger
	monitoredChannels map[int64]struct{}
}

// NewMessageHandler builds a handler that classifies via classifier and emits
// jobs to writeCh. monitoredChannels is a set of channel IDs to accept; updates
// from other channels are silently discarded. log may be nil (treated as NopLogger).
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

// HandleUpdate adapta o update bruto em um RawMessage e o enfileira
// para persistência. Os bytes do payload original são preservados na íntegra.
// Updates de canais não monitorados são descartados silenciosamente.
func (h *MessageHandler) HandleUpdate(ctx context.Context, update telegram.Update) error {
	// Filtra updates de canais não monitorados. channel_id=0 abrange DMs,
	// eventos do sistema e qualquer update que não seja uma mensagem de canal — todos
	// os quais devem ser descartados na Fase 1 (a coleta é restrita apenas a canais
	// monitorados).

	// A validação do channel monitorado acontece ANTES da alocação do
	// struct *storage.RawMessage ou de outros processamentos, funcionando
	// como um retorno antecipado (early return) no caminho crítico
	// para evitar alocações e ciclos de GC.
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
