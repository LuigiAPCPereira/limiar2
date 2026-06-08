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
// optional error channel for write-failure notification.
type WriteJob struct {
	Message *storage.RawMessage
	ErrCh   chan<- error
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

// HandleUpdate adapts the raw update into a RawMessage and enqueues it
// for persistence. The original payload bytes are preserved verbatim.
// Updates from non-monitored channels are silently discarded.
func (h *MessageHandler) HandleUpdate(ctx context.Context, update telegram.Update) error {
	// Filter out updates from non-monitored channels
	if update.ChannelID != 0 {
		if _, ok := h.monitoredChannels[update.ChannelID]; !ok {
			return nil // silently discard non-monitored channel updates
		}
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
