package collector

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// historyPageSize is the per-request batch size for messages.getHistory calls.
// Kept small to avoid single-request spikes on the MTProto side; the backfill
// loop paginates until it hits the configured ceiling or the stored cursor.
const historyPageSize = 100

// Repository is the subset of storage operations the collector needs. Accepting
// an interface keeps the collector testable with a fake.
type Repository interface {
	ListChannels(ctx context.Context) ([]*storage.Channel, error)
	SaveRawMessage(ctx context.Context, msg *storage.RawMessage) (inserted bool, err error)
	UpdateChannelLastMessage(ctx context.Context, channelID, messageID int64, collectedAt time.Time) error
}

// Collector orchestrates capture: it backfills history, registers the message
// handler, runs the Telegram client, and serializes all writes through a
// single DBWriter goroutine.
type Collector struct {
	client         telegram.TelegramClient
	repo           Repository
	classifier     Classifier
	log            logger.Logger
	writeBuffer    int
	maxWriteRetry  int
	historyMax     int
	historyMaxDays int
	onMessage      func(*storage.RawMessage)

	writeCh chan WriteJob
	wg      sync.WaitGroup

	// Observability: atomic counters updated by dbWriter on every successful
	// save. Read with atomic.LoadInt64 for safe cross-goroutine inspection
	// (e.g. by periodic stats logs).
	statsNew       int64
	statsDuplicate int64
}

// SetOnMessage registers a callback invoked after each successful database
// write. It is safe to call before Run. Pass nil to disable.
func (c *Collector) SetOnMessage(fn func(*storage.RawMessage)) {
	c.onMessage = fn
}

// NewCollector builds a collector. writeBuffer sizes the fan-in channel;
// maxWriteRetry bounds DB write retries before a job is reported failed;
// historyMax caps the number of messages backfilled per channel on each run;
// historyMaxDays is the temporal cutoff — messages older than now minus this
// many days are skipped and pagination stops when the page straddles it.
func NewCollector(
	client telegram.TelegramClient,
	repo Repository,
	classifier Classifier,
	log logger.Logger,
	writeBuffer int,
	maxWriteRetry int,
	historyMax int,
	historyMaxDays int,
) *Collector {
	if log == nil {
		log = logger.NopLogger{}
	}
	if historyMax <= 0 {
		historyMax = historyPageSize
	}
	if historyMaxDays <= 0 {
		historyMaxDays = 30
	}
	return &Collector{
		client:         client,
		repo:           repo,
		classifier:     classifier,
		log:            log,
		writeBuffer:    writeBuffer,
		maxWriteRetry:  maxWriteRetry,
		historyMax:     historyMax,
		historyMaxDays: historyMaxDays,
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

	// Reset observability counters for this run.
	atomic.StoreInt64(&c.statsNew, 0)
	atomic.StoreInt64(&c.statsDuplicate, 0)

	if err := c.backfill(ctx, channels); err != nil {
		// Backfill failure is logged, not fatal: live capture should still run.
		c.log.Error("📜 Histórico inicial falhou", "erro", err)
	}

	c.log.Info("📊 Backfill concluído",
		"novas", atomic.LoadInt64(&c.statsNew),
		"duplicatas", atomic.LoadInt64(&c.statsDuplicate))

	// Reset again so live-capture stats start clean (not polluted by backfill).
	atomic.StoreInt64(&c.statsNew, 0)
	atomic.StoreInt64(&c.statsDuplicate, 0)

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

	// Periodic observability log during live capture (every 5 minutes).
	statsCtx, statsCancel := context.WithCancel(ctx)
	defer statsCancel()
	go c.statsLoop(statsCtx)

	runErr := c.client.Run(ctx)

	// Client returned (ctx cancelled or fatal): drain and close.
	c.shutdown()

	if runErr != nil {
		return apperrors.Wrap("collector", "run", runErr)
	}
	return nil
}

// backfill fetches history for every active channel, paginating until the
// configured ceiling (c.historyMax) or the stored cursor (LastMessageID) is
// reached. SaveRawMessage is idempotent (ON CONFLICT DO NOTHING on the
// (channel_id, message_id) unique constraint), so re-fetching messages that
// were already persisted is safe — they are silently dropped by the DB.
func (c *Collector) backfill(ctx context.Context, channels []*storage.Channel) error {
	for _, ch := range channels {
		if !ch.Active {
			continue
		}
		c.log.Info("📜 Buscando histórico",
			"canal", ch.Username,
			"desde_msg_id", ch.LastMessageID,
			"teto", c.historyMax,
			"desde_data", time.Now().AddDate(0, 0, -c.historyMaxDays).Format("2006-01-02"))

		fetched, maxID, err := c.backfillChannel(ctx, ch)
		if err != nil {
			return apperrors.Wrap("collector", "fetch_history", err)
		}

		if maxID > 0 {
			if err := c.repo.UpdateChannelLastMessage(ctx, ch.ID, maxID, time.Now().UTC()); err != nil {
				return apperrors.Wrap("collector", "advance_cursor", err)
			}
		}

		c.log.Info("📜 Histórico carregado",
			"canal", ch.Username,
			"total", fetched,
			"max_msg_id", maxID)
	}
	return nil
}

// backfillChannel pages through one channel's history starting from the newest
// messages and walking backward. Two stop conditions apply:
//   - numeric ceiling (c.historyMax): safety net against hyper-active channels
//   - temporal cutoff (now - c.historyMaxDays): the business rule that old
//     promotions lose value and should not be collected
//
// Telegram's messages.getHistory supports MinID (id > MinID, server-side) and
// OffsetID (id < OffsetID). We use MinID=cursor on the first request to drop
// everything at-or-below the stored cursor server-side, then OffsetID on
// subsequent requests to page backward through the returned window. The loop
// terminates on: empty page, either stop condition, or context cancellation.
// SaveRawMessage is idempotent (ON CONFLICT DO NOTHING on the unique
// (channel_id, message_id)), so re-fetching a message already persisted is a
// silent no-op at the DB layer.
func (c *Collector) backfillChannel(ctx context.Context, ch *storage.Channel) (fetched, maxID int64, err error) {
	cursor := ch.LastMessageID
	ceiling := int64(c.historyMax)
	cutoff := time.Now().AddDate(0, 0, -c.historyMaxDays)
	var totalFetched int64
	var globalMaxID int64
	offsetID := int64(0)
	firstPage := true

	for {
		if ctx.Err() != nil {
			return totalFetched, globalMaxID, ctx.Err()
		}

		remaining := ceiling - totalFetched
		if remaining <= 0 {
			break
		}
		pageSize := historyPageSize
		if remaining < int64(pageSize) {
			pageSize = int(remaining)
		}

		var msgs []telegram.HistoryMessage
		var fetchErr error
		if firstPage && cursor > 0 {
			// Server-side MinID drops everything id <= cursor on the first
			// request, so we only receive messages newer than the cursor.
			msgs, fetchErr = c.client.FetchHistory(ctx, ch.ID, cursor, pageSize)
		} else {
			msgs, fetchErr = c.client.FetchHistoryWithOffset(ctx, ch.ID, offsetID, pageSize)
		}
		if fetchErr != nil {
			return totalFetched, globalMaxID, fetchErr
		}
		firstPage = false
		if len(msgs) == 0 {
			break
		}

		pageMinID := msgs[0].MessageID
		pageMaxID := msgs[0].MessageID
		pageMinDate := msgs[0].Date
		for _, m := range msgs {
			if m.MessageID < pageMinID {
				pageMinID = m.MessageID
			}
			if m.MessageID > pageMaxID {
				pageMaxID = m.MessageID
			}
			if m.Date.Before(pageMinDate) {
				pageMinDate = m.Date
			}
		}

		for _, m := range msgs {
			// Skip messages at or below the stored cursor (already persisted
			// in a previous run).
			if cursor > 0 && m.MessageID <= cursor {
				continue
			}
			// Skip messages older than the temporal cutoff.
			if !m.Date.IsZero() && m.Date.Before(cutoff) {
				continue
			}
			job := WriteJob{
				Message: &storage.RawMessage{
					ChannelID:     ch.ID,
					MessageID:     m.MessageID,
					Payload:       m.Payload,
					ReceivedAt:    time.Now().UTC(),
					SchemaVersion: schemaVersion,
				},
				Backfill: true,
			}
			select {
			case c.writeCh <- job:
			case <-ctx.Done():
				return totalFetched, globalMaxID, ctx.Err()
			}
			totalFetched++
		}

		if pageMaxID > globalMaxID {
			globalMaxID = pageMaxID
		}

		// If the oldest message in this page is at or below the stored cursor,
		// every message newer than the cursor has been covered — stop.
		if cursor > 0 && pageMinID <= cursor {
			break
		}
		// If the oldest message in this page is older than the cutoff, every
		// message within the temporal window has been covered — stop (all
		// subsequent pages would be even older).
		if !pageMinDate.IsZero() && pageMinDate.Before(cutoff) {
			break
		}

		// Walk backward: next page fetches messages older than pageMinID.
		offsetID = pageMinID
	}

	return totalFetched, globalMaxID, nil
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
// Cursor advancement is skipped for backfill jobs — the backfill orchestrator
// owns it — and for live jobs whose ChannelID is zero (system events that
// should have been filtered upstream; belt-and-suspenders).
func (c *Collector) writeWithRetry(ctx context.Context, job WriteJob) {
	var lastErr error
	for attempt := 0; attempt <= c.maxWriteRetry; attempt++ {
		inserted, err := c.repo.SaveRawMessage(ctx, job.Message)
		if err != nil {
			lastErr = err
			continue
		}
		if inserted {
			atomic.AddInt64(&c.statsNew, 1)
		} else {
			atomic.AddInt64(&c.statsDuplicate, 1)
		}
		if !job.Backfill && job.Message.ChannelID != 0 {
			if err := c.repo.UpdateChannelLastMessage(ctx, job.Message.ChannelID, job.Message.MessageID, job.Message.ReceivedAt); err != nil {
				c.log.Warn("⚠️  Cursor não atualizado", "canal_id", job.Message.ChannelID, "erro", err)
			}
		}
		if c.onMessage != nil {
			c.onMessage(job.Message)
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

// statsLoop periodically logs observability counters (new vs. duplicate
// messages persisted) so operators can watch collection health without
// waiting for shutdown. It returns when ctx is cancelled.
func (c *Collector) statsLoop(ctx context.Context) {
	const interval = 5 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n := atomic.LoadInt64(&c.statsNew)
			d := atomic.LoadInt64(&c.statsDuplicate)
			if n == 0 && d == 0 {
				continue
			}
			c.log.Info("📊 Estatísticas da captura ao vivo",
				"novas", n, "duplicatas", d)
		}
	}
}

// shutdown closes the write channel and waits for the DBWriter to drain.
func (c *Collector) shutdown() {
	close(c.writeCh)
	c.wg.Wait()
}
