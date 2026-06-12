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

// historyPageSize é o tamanho do lote por requisição para as chamadas de messages.getHistory.
// Mantido pequeno para evitar picos de requisições únicas no lado do MTProto; o loop
// de backfill pagina até atingir o limite (ceiling) configurado ou o cursor armazenado.
const historyPageSize = 100

// Repository é o subconjunto de operações de storage de que o collector precisa. Aceitar
// uma interface mantém o collector testável usando fakes.
type Repository interface {
	ListChannels(ctx context.Context) ([]*storage.Channel, error)
	SaveRawMessage(ctx context.Context, msg *storage.RawMessage) (inserted bool, err error)
	// SaveRawMessageBatch persiste múltiplas mensagens em uma única transação.
	// inserted[i] indica se msgs[i] foi efetivamente inserida (true) ou era duplicata (false).
	// totalInserted é a soma dos true. Em falha, inserted é nil e err não é nil.
	SaveRawMessageBatch(ctx context.Context, msgs []*storage.RawMessage) (inserted []bool, totalInserted int, err error)
	UpdateChannelLastMessage(ctx context.Context, channelID, messageID int64, collectedAt time.Time) error
}

// defaultFlushInterval é a janela padrão de acúmulo de jobs no dbWriter antes do
// flush em transação única. 10ms é o ponto de equilíbrio empírico entre latência
// (o dashboard vê mensagens em até ~10ms) e throughput (rajadas de backfill
// colapsam centenas de fsyncs em 1 fsync só).
const defaultFlushInterval = 10 * time.Millisecond

// Collector orquestra a captura: ele faz o backfill do histórico, registra o handler
// de mensagens, executa o client do Telegram e serializa todas as gravações através de uma
// única goroutine DBWriter.
type Collector struct {
	client         telegram.TelegramClient
	repo           Repository
	classifier     Classifier
	log            logger.Logger
	writeBuffer    int
	maxWriteRetry  int
	historyMax     int
	historyMaxDays int
	flushInterval  time.Duration
	onMessage      func(*storage.RawMessage)

	writeCh chan WriteJob
	wg      sync.WaitGroup

	// Observabilidade: contadores atômicos atualizados pelo dbWriter em cada gravação
	// bem-sucedida. Leia com atomic.LoadInt64 para uma inspeção segura entre goroutines
	// (ex: logs periódicos de estatísticas).
	statsNew       int64
	statsDuplicate int64
}

// SetOnMessage registra um callback invocado após cada gravação bem-sucedida no
// banco de dados. É seguro chamar antes de Run. Passe nil para desabilitar.
func (c *Collector) SetOnMessage(fn func(*storage.RawMessage)) {
	c.onMessage = fn
}

// NewCollector constrói um collector. writeBuffer define o tamanho do canal de fan-in;
// maxWriteRetry limita as repetições de gravações no DB antes que um job seja dado como falho;
// historyMax limita o número de mensagens de backfill por canal a cada execução;
// historyMaxDays é o limite temporal — mensagens mais velhas que "agora" menos essa
// quantidade de dias são puladas e a paginação para quando a página abrange essa data.
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
		flushInterval:  defaultFlushInterval,
	}
}

// Run inicia o DBWriter, faz o backfill do histórico da primeira execução, registra o handler,
// e executa o client até que ctx seja cancelado, então realiza um dreno (drain) limpo.
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

	// Reseta os contadores de observabilidade para esta execução.
	atomic.StoreInt64(&c.statsNew, 0)
	atomic.StoreInt64(&c.statsDuplicate, 0)

	if err := c.backfill(ctx, channels); err != nil {
		// A falha no backfill é registrada no log, mas não é fatal: a captura ao vivo ainda deve ser executada.
		c.log.Error("📜 Histórico inicial falhou", "erro", err)
	}

	c.log.Info("📊 Backfill concluído",
		"novas", atomic.LoadInt64(&c.statsNew),
		"duplicatas", atomic.LoadInt64(&c.statsDuplicate))

	// Reseta novamente para que as estatísticas da captura ao vivo comecem limpas (não poluídas pelo backfill).
	atomic.StoreInt64(&c.statsNew, 0)
	atomic.StoreInt64(&c.statsDuplicate, 0)

	// Constrói um conjunto (set) de IDs de canais monitorados para filtrar atualizações ao vivo
	monitoredSet := make(map[int64]struct{}, len(channels))
	for _, ch := range channels {
		if ch.Active {
			monitoredSet[ch.ID] = struct{}{}
		}
	}

	handler := NewMessageHandler(c.classifier, c.writeCh, monitoredSet, c.log)
	c.client.AddUpdateHandler(handler)

	c.log.Info("📡 Captura ao vivo iniciada", "canais", len(channels))

	// Log de observabilidade periódico durante a captura ao vivo (a cada 5 minutos).
	statsCtx, statsCancel := context.WithCancel(ctx)
	defer statsCancel()
	go c.statsLoop(statsCtx)

	runErr := c.client.Run(ctx)

	// O Client retornou (ctx cancelado ou fatal): drene e feche.
	c.shutdown()

	if runErr != nil {
		return apperrors.Wrap("collector", "run", runErr)
	}
	return nil
}

// backfill busca o histórico para cada canal ativo, paginando até o limite
// configurado (c.historyMax) ou até que o cursor armazenado (LastMessageID) seja
// atingido. SaveRawMessage é idempotente (ON CONFLICT DO NOTHING na constraint
// única (channel_id, message_id)), portanto buscar novamente mensagens que
// já foram persistidas é seguro — elas são silenciosamente descartadas pelo DB.
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

// backfillChannel pagina através do histórico de um canal começando pelas mensagens
// mais recentes e caminhando para trás. Duas condições de parada se aplicam:
//   - limite numérico (c.historyMax): rede de segurança contra canais hiperativos
//   - limite temporal (agora - c.historyMaxDays): regra de negócio que diz que promoções
//     antigas perdem valor e não devem ser coletadas
//
// O messages.getHistory do Telegram suporta MinID (id > MinID, no servidor) e
// OffsetID (id < OffsetID). Nós usamos MinID=cursor na primeira requisição para descartar
// tudo igual-ou-abaixo do cursor armazenado no lado do servidor, depois usamos OffsetID nas
// requisições subsequentes para paginar retroativamente através da janela retornada. O loop
// termina em caso de: página vazia, qualquer das condições de parada, ou cancelamento de contexto.
// SaveRawMessage é idempotente (ON CONFLICT DO NOTHING no (channel_id, message_id)
// único), então buscar uma mensagem já persistida é um no-op silencioso na camada do DB.
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
			// MinID no servidor descarta tudo com id <= cursor na primeira
			// requisição, então recebemos apenas mensagens mais novas que o cursor.
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
			// Pula mensagens iguais ou abaixo do cursor armazenado (já persistidas
			// em uma execução anterior).
			if cursor > 0 && m.MessageID <= cursor {
				continue
			}
			// Pula mensagens mais antigas que o limite temporal (cutoff).
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

		// Se a mensagem mais antiga nesta página estiver no cursor armazenado ou abaixo dele,
		// todas as mensagens mais recentes que o cursor foram cobertas — pare.
		if cursor > 0 && pageMinID <= cursor {
			break
		}
		// Se a mensagem mais antiga nesta página for mais antiga que o cutoff (limite temporal),
		// todas as mensagens dentro da janela temporal foram cobertas — pare (todas
		// as páginas subsequentes seriam ainda mais antigas).
		if !pageMinDate.IsZero() && pageMinDate.Before(cutoff) {
			break
		}

		// Caminhe para trás: a próxima página buscará mensagens mais velhas que pageMinID.
		offsetID = pageMinID
	}

	return totalFetched, globalMaxID, nil
}

// dbWriter é a única goroutine que escreve no banco de dados (fan-in). Ele
// drena o writeCh até ser fechado, acumulando jobs em micro-lotes dentro de
// uma janela de tempo (flushInterval) para disparar uma única transação por
// lote em vez de uma transação por mensagem. Isso colapsa N fsyncs em 1 fsync,
// reduzindo a latência de escrita em ordens de magnitude durante backfill
// (milhares de mensagens) sem prejudicar a latência percebida pelo dashboard
// (~flushInterval de atraso adicional).
func (c *Collector) dbWriter(ctx context.Context) {
	defer c.wg.Done()

	batch := make([]WriteJob, 0, cap(c.writeCh))
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
			if len(batch) >= cap(c.writeCh) {
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
	msgs := make([]*storage.RawMessage, len(batch))
	for i, job := range batch {
		msgs[i] = job.Message
	}

	inserted, err := c.writeBatchWithRetry(ctx, msgs)

	// Caminho feliz: lote commitado. Avança cursores, atualiza stats, dispara callbacks.
	if err == nil {
		for i, job := range batch {
			if inserted[i] {
				atomic.AddInt64(&c.statsNew, 1)
			} else {
				atomic.AddInt64(&c.statsDuplicate, 1)
			}
			if !job.Backfill && job.Message.ChannelID != 0 {
				if updErr := c.repo.UpdateChannelLastMessage(ctx, job.Message.ChannelID, job.Message.MessageID, job.Message.ReceivedAt); updErr != nil {
					c.log.Warn("⚠️  Cursor não atualizado", "canal_id", job.Message.ChannelID, "erro", updErr)
				}
			}
			if c.onMessage != nil {
				c.onMessage(job.Message)
			}
		}
		return
	}

	// Falha persistente do lote: cai para escrita individual (writeWithRetry),
	// que tem seu próprio loop de retry. Menor throughput, mas maximiza
	// recuperação de mensagens sob falha transitória do DB.
	c.log.Warn("⚠️  Lote falhou, caindo para escrita individual",
		"tamanho_lote", len(batch), "erro", err)
	for _, job := range batch {
		c.writeWithRetry(ctx, job)
	}
}

// writeBatchWithRetry tenta commitar um lote em transação única até maxWriteRetry+1
// vezes. Idempotente graças ao ON CONFLICT DO NOTHING — retries não criam
// duplicatas. Retorna nil no sucesso, o último erro após retries esgotados.
func (c *Collector) writeBatchWithRetry(ctx context.Context, msgs []*storage.RawMessage) ([]bool, error) {
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

	// Falha persistente: registra explicitamente a mensagem perdida no log, nunca a descarta silenciosamente.
	c.log.Error("❌ Mensagem perdida após retries",
		"canal_id", job.Message.ChannelID,
		"mensagem_id", job.Message.MessageID,
		"erro", lastErr)
	if job.ErrCh != nil {
		job.ErrCh <- apperrors.Wrap("collector", "db_write", apperrors.ErrDBWriteFailed)
	}
}

// statsLoop registra periodicamente contadores de observabilidade (mensagens novas vs. duplicadas
// persistidas) para que os operadores possam observar a saúde da coleta sem
// esperar pelo encerramento (shutdown). Retorna quando ctx é cancelado.
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

// shutdown fecha o canal de escrita e aguarda o dreno (drain) do DBWriter.
func (c *Collector) shutdown() {
	close(c.writeCh)
	c.wg.Wait()
}
