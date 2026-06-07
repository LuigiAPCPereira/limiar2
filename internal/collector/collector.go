package collector

import (
	"context"
	"sync"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// historyBatchSize is the number of recent messages backfilled on a channel's
// first run (last_message_id == 0).
const historyBatchSize = 20

// historyResumeLimit is the max messages fetched per channel on resume runs.
const historyResumeLimit = 100

// Repository is the subset of storage operations the collector needs. Accepting
// an interface keeps the collector testable with a fake.
type Repository interface {
	ListChannels(ctx context.Context) ([]*storage.Channel, error)
	SaveRawMessage(ctx context.Context, msg *storage.RawMessage) error
	UpdateChannelLastMessage(ctx context.Context, channelID, messageID int64, collectedAt time.Time) error
}

// Collector orchestrates capture: it backfills history, registers the message
// handler, runs the Telegram client, and serializes all writes through a
// single DBWriter goroutine.
type Collector struct {
	client        telegram.TelegramClient
	repo          Repository
	classifier    Classifier
	log           logger.Logger
	writeBuffer   int
	maxWriteRetry int

	writeCh chan WriteJob
	wg      sync.WaitGroup
}

// NewCollector builds a collector. writeBuffer sizes the fan-in channel;
// maxWriteRetry bounds DB write retries before a job is reported failed.
func NewCollector(
	client telegram.TelegramClient,
	repo Repository,
	classifier Classifier,
	log logger.Logger,
	writeBuffer int,
	maxWriteRetry int,
) *Collector {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Collector{
		client:        client,
		repo:          repo,
		classifier:    classifier,
		log:           log,
		writeBuffer:   writeBuffer,
		maxWriteRetry: maxWriteRetry,
	}
}

// Run starts the DBWriter, backfills first-run history, registers the handler,
// and runs the client until ctx is cancelled, then drains cleanly.
func (c *Collector) Run(ctx context.Context) error {
	c.writeCh = make(chan WriteJob, c.writeBuffer)

	c.wg.Add(1)
	go c.dbWriter(ctx)

	if err := c.client.LoadPeers(ctx); err != nil {
		c.shutdown()
		return apperrors.Wrap("collector", "load_peers", err)
	}

	channels, err := c.repo.ListChannels(ctx)
	if err != nil {
		c.shutdown()
		return apperrors.Wrap("collector", "list_channels", err)
	}

	if err := c.backfill(ctx, channels); err != nil {
		// Backfill failure is logged, not fatal: live capture should still run.
		c.log.Error("📜 Histórico inicial falhou", "erro", err)
	}

	// Build a set of monitored channel IDs for filtering live updates
	monitoredSet := make(map[int64]struct{}, len(channels))
	for _, ch := range channels {
		if ch.Active {
			monitoredSet[ch.ID] = struct{}{}
		}
	}

	handler := NewMessageHandler(c.classifier, c.writeCh, monitoredSet, c.log)
	c.client.AddUpdateHandler(handler)

	c.log.Info("📡 Captura ao vivo iniciada", "canais", len(channels))

	runErr := c.client.Run(ctx)

	// Client returned (ctx cancelled or fatal): drain and close.
	c.shutdown()

	if runErr != nil {
		return apperrors.Wrap("collector", "run", runErr)
	}
	return nil
}

// backfill fetches history for every active channel. First-run channels get
// the recent batch; resume channels fetch only messages newer than the stored
// cursor (minID = LastMessageID). Duplicates are silently ignored by the DB.
func (c *Collector) backfill(ctx context.Context, channels []*storage.Channel) error {
	for _, ch := range channels {
		if !ch.Active {
			continue
		}
		minID := ch.LastMessageID
		limit := historyBatchSize
		if minID > 0 {
			limit = historyResumeLimit
		}
		c.log.Info("📜 Buscando histórico", "canal", ch.Username, "desde_msg_id", minID)
		msgs, err := c.client.FetchHistory(ctx, ch.ID, minID, limit)
		if err != nil {
			return apperrors.Wrap("collector", "fetch_history", err)
		}
		var maxID int64
		for _, m := range msgs {
			job := WriteJob{Message: &storage.RawMessage{
				ChannelID:     ch.ID,
				MessageID:     m.MessageID,
				Payload:       m.Payload,
				ReceivedAt:    time.Now().UTC(),
				SchemaVersion: schemaVersion,
			}}
			select {
			case c.writeCh <- job:
			case <-ctx.Done():
				return ctx.Err()
			}
			if m.MessageID > maxID {
				maxID = m.MessageID
			}
		}
		if maxID > 0 {
			if err := c.repo.UpdateChannelLastMessage(ctx, ch.ID, maxID, time.Now().UTC()); err != nil {
				return apperrors.Wrap("collector", "advance_cursor", err)
			}
		}
		c.log.Info("📜 Histórico carregado",
			"canal", ch.Username, "mensagens", len(msgs), "max_msg_id", maxID)
	}
	return nil
}

// dbWriter is the single goroutine that writes to the database (fan-in). It
// drains writeCh until closed, retrying each write up to maxWriteRetry times.
func (c *Collector) dbWriter(ctx context.Context) {
	defer c.wg.Done()
	for job := range c.writeCh {
		c.writeWithRetry(ctx, job)
	}
}

// writeWithRetry persists one job, retrying on failure. A persistent failure
// is logged (never silently dropped) and reported via job.ErrCh if present.
func (c *Collector) writeWithRetry(ctx context.Context, job WriteJob) {
	var lastErr error
	for attempt := 0; attempt <= c.maxWriteRetry; attempt++ {
		if err := c.repo.SaveRawMessage(ctx, job.Message); err != nil {
			lastErr = err
			continue
		}
		if err := c.repo.UpdateChannelLastMessage(ctx, job.Message.ChannelID, job.Message.MessageID, job.Message.ReceivedAt); err != nil {
			c.log.Warn("⚠️  Cursor não atualizado", "canal_id", job.Message.ChannelID, "erro", err)
		}
		return
	}

	// Persistent failure: log the lost message explicitly, never drop silently.
	c.log.Error("❌ Mensagem perdida após retries",
		"canal_id", job.Message.ChannelID,
		"mensagem_id", job.Message.MessageID,
		"erro", lastErr)
	if job.ErrCh != nil {
		job.ErrCh <- apperrors.Wrap("collector", "db_write", apperrors.ErrDBWriteFailed)
	}
}

// shutdown closes the write channel and waits for the DBWriter to drain.
func (c *Collector) shutdown() {
	close(c.writeCh)
	c.wg.Wait()
}
