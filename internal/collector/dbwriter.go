package collector

import (
	"context"
	"sync/atomic"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/model"
)

// dbWriter é a única goroutine que escreve no banco de dados (fan-in). Ele
// drena o writeCh até ser fechado, acumulando jobs em micro-lotes dentro de
// uma janela configurável (flushInterval) antes do flush em transação única.
// Lotes cheios (len(batch) >= limite) disparam flush imediato sem esperar o timer.
func (c *Collector) dbWriter(ctx context.Context) {
	defer c.wg.Done()

	limit := cap(c.writeCh)
	if limit == 0 {
		limit = 100
	}
	batch := make([]WriteJob, 0, limit)
	var timer *time.Timer
	var timerC <-chan time.Time

	for {
		select {
		case job, ok := <-c.writeCh:
			if !ok {
				// Canal fechado: drena o lote pendente e sai.
				if len(batch) > 0 {
					c.flush(ctx, batch)
				}
				return
			}
			// Primeiro job do lote: arma o timer de flush.
			if len(batch) == 0 && timerC == nil {
				if timer == nil {
					timer = time.NewTimer(c.flushInterval)
				} else {
					timer.Reset(c.flushInterval)
				}
				timerC = timer.C
			}
			batch = append(batch, job)
			// Lote cheio: flush imediato sem esperar o timer.
			if len(batch) >= limit {
				c.flush(ctx, batch)
				batch = batch[:0]
				if timer != nil && !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timerC = nil
			}
		case <-timerC:
			c.flush(ctx, batch)
			batch = batch[:0]
			timerC = nil
		}
	}
}

// flush persiste um lote acumulado de WriteJobs em uma única transação e
// dispara callbacks (onMessage) para cada mensagem persistida. Jobs marcados
// como Backfill não avançam o cursor — o orquestrador do backfill é dono
// disso (evita a corrida da ordem descendente). Falhas de lote caem para
// escrita individual com retry (writeWithRetry) para maximizar recuperação.
func (c *Collector) flush(ctx context.Context, batch []WriteJob) {
	if len(batch) == 0 {
		return
	}

	// Separa mensagens para o commit em lote.
	msgs := make([]*model.RawMessage, len(batch))
	for i, job := range batch {
		msgs[i] = job.Message
	}

	inserted, err := c.writeBatchWithRetry(ctx, msgs)

	// Caminho feliz: lote commitado. Atualiza stats e dispara callbacks
	// para cada mensagem persistida. Cursores de canal são consolidados
	// (1 UPDATE por canal com o maior MessageID do lote) para evitar N
	// fsyncs de cursor que anulariam o ganho do batch transacional.
	if err == nil {
		type cursorInfo struct {
			msgID int64
			recAt time.Time
		}
		latestCursors := make(map[int64]cursorInfo)

		for i, job := range batch {
			if inserted[i] {
				atomic.AddInt64(&c.statsNew, 1)
				// Cursor só avança para mensagens efetivamente inseridas.
				// Duplicatas (ON CONFLICT DO NOTHING) não representam progresso
				// de coleta e podem carregar MessageID menor que o máximo já
				// persistido no lote — avançar nelas poderia regredir o cursor.
				if !job.Backfill && job.Message.ChannelID != 0 {
					if existing, ok := latestCursors[job.Message.ChannelID]; !ok || job.Message.MessageID > existing.msgID {
						latestCursors[job.Message.ChannelID] = cursorInfo{
							msgID: job.Message.MessageID,
							recAt: job.Message.ReceivedAt,
						}
					}
				}
			} else {
				atomic.AddInt64(&c.statsDuplicate, 1)
			}
			if c.onMessage != nil {
				c.onMessage(job.Message)
			}
		}

		for chID, info := range latestCursors {
			if updErr := c.repo.UpdateChannelLastMessage(ctx, chID, info.msgID, info.recAt); updErr != nil {
				if _, isNop := c.log.(logger.NopLogger); !isNop {
					c.log.Warn("⚠️  Cursor não atualizado", "canal_id", chID, "erro", updErr)
				}
			}
		}
		return
	}

	// Falha persistente do lote: cai para escrita individual (writeWithRetry),
	// que tem seu próprio loop de retry. Menor throughput, mas maximiza
	// recuperação de mensagens sob falha transitória do DB.
	if _, isNop := c.log.(logger.NopLogger); !isNop {
		c.log.Warn("⚠️  Lote falhou, caindo para escrita individual",
			"tamanho_lote", len(batch), "erro", err)
	}
	for _, job := range batch {
		c.writeWithRetry(ctx, job)
	}
}

// writeBatchWithRetry tenta commitar um lote em transação única até maxWriteRetry+1
// vezes. Idempotente graças ao ON CONFLICT DO NOTHING — retries não criam
// duplicatas. Retorna nil no sucesso, o último erro após retries esgotados.
func (c *Collector) writeBatchWithRetry(ctx context.Context, msgs []*model.RawMessage) ([]bool, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxWriteRetry; attempt++ {
		inserted, _, err := c.repo.SaveRawMessageBatch(ctx, msgs)
		if err == nil {
			return inserted, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// writeWithRetry persiste um job, com retentativa (retry) em caso de falha. Uma falha persistente
// é registrada no log (nunca descartada silenciosamente) e reportada via job.ErrCh, se presente.
// O avanço do cursor é pulado para jobs de backfill — o orquestrador de backfill
// é dono disso — e para jobs ao vivo cujo ChannelID é zero (eventos de sistema que
// deveriam ter sido filtrados upstream).
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
			// Cursor só avança para mensagens efetivamente inseridas.
			// Duplicatas (ON CONFLICT DO NOTHING) não representam progresso
			// de coleta.
			if !job.Backfill && job.Message.ChannelID != 0 {
				if err := c.repo.UpdateChannelLastMessage(ctx, job.Message.ChannelID, job.Message.MessageID, job.Message.ReceivedAt); err != nil {
					if _, isNop := c.log.(logger.NopLogger); !isNop {
						c.log.Warn("⚠️  Cursor não atualizado", "canal_id", job.Message.ChannelID, "erro", err)
					}
				}
			}
		} else {
			atomic.AddInt64(&c.statsDuplicate, 1)
		}
		if c.onMessage != nil {
			c.onMessage(job.Message)
		}
		// Download proativo de imagem full-res (assíncrono).
		// O file_reference MTProto é válido apenas na janela de chegada da mensagem.
		if c.mediaClient != nil && c.imageCache != nil {
			c.proactiveDownload(ctx, job.Message)
		}
		return
	}

	// Falha persistente: registra explicitamente a mensagem perdida no log, nunca a descarta silenciosamente.
	if _, isNop := c.log.(logger.NopLogger); !isNop {
		c.log.Error("❌ Mensagem perdida após retries",
			"canal_id", job.Message.ChannelID,
			"mensagem_id", job.Message.MessageID,
			"erro", lastErr)
	}
	if job.ErrCh != nil {
		job.ErrCh <- apperrors.Wrap("collector", "db_write", apperrors.ErrDBWriteFailed)
	}
}
