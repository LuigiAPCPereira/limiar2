package collector

import (
	"context"
	"sync"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
	"github.com/limiar/collector/internal/telegram"
)

// historyPageSize é o tamanho do lote por requisição para as chamadas de messages.getHistory.
// Mantido pequeno para evitar picos de requisições únicas no lado do MTProto; o loop
// de backfill pagina até atingir o limite (ceiling) configurado ou o cursor armazenado.
const historyPageSize = 100

// Repository é o subconjunto de operações de storage de que o collector precisa. Aceitar
// uma interface mantém o collector testável usando fakes.
type Repository interface {
	ListChannels(ctx context.Context) ([]*model.Channel, error)
	SaveRawMessage(ctx context.Context, msg *model.RawMessage) (inserted bool, err error)
	// SaveRawMessageBatch persiste múltiplas mensagens em uma única transação.
	// inserted[i] indica se msgs[i] foi efetivamente inserida (true) ou era duplicata (false).
	// totalInserted é a soma dos true. Em falha, inserted é nil e err não é nil.
	SaveRawMessageBatch(ctx context.Context, msgs []*model.RawMessage) (inserted []bool, totalInserted int, err error)
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
	onMessage      func(*model.RawMessage)
	mediaClient    mediaClient
	imageCache     imageCache

	writeCh chan WriteJob
	wg      sync.WaitGroup

	// Observabilidade: contadores atômicos atualizados pelo dbWriter em cada gravação
	// bem-sucedida. Leia com atomic_LoadInt64 para uma inspeção segura entre goroutines
	// (ex: logs periódicos de estatísticas).
	statsNew       int64
	statsDuplicate int64
}

// SetOnMessage registra um callback invocado após cada gravação bem-sucedida no
// banco de dados. É seguro chamar antes de Run. Passe nil para desabilitar.
func (c *Collector) SetOnMessage(fn func(*model.RawMessage)) {
	c.onMessage = fn
}

// mediaClient abstrai o download de imagens via MTProto (satisfeito por
// telegram.MediaClient que implementa media.MediaClient).
type mediaClient interface {
	DownloadPhoto(ctx context.Context, req media.PhotoDownloadRequest) ([]byte, error)
}

// imageCache abstrai o cache in-memory de imagens (satisfeito por media.ImageCache).
type imageCache interface {
	Put(photoID int64, data []byte)
}

// SetMediaDownload configura o download proativo de imagens. Quando uma mensagem
// com foto é salva, o collector baixa a imagem full-res (janela onde file_reference
// é válido) e armazena no cache compartilhado.
func (c *Collector) SetMediaDownload(client mediaClient, cache imageCache) {
	c.mediaClient = client
	c.imageCache = cache
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
	atomic_StoreInt64(&c.statsNew, 0)
	atomic_StoreInt64(&c.statsDuplicate, 0)

	if err := c.backfill(ctx, channels); err != nil {
		// A falha no backfill é registrada no log, mas não é fatal: a captura ao vivo ainda deve ser executada.
		c.log.Error("📜 Histórico inicial falhou", "erro", err)
	}

	c.log.Info("📊 Backfill concluído",
		"novas", atomic_LoadInt64(&c.statsNew),
		"duplicatas", atomic_LoadInt64(&c.statsDuplicate))

	// Reseta novamente para que as estatísticas da captura ao vivo comecem limpas (não poluídas pelo backfill).
	atomic_StoreInt64(&c.statsNew, 0)
	atomic_StoreInt64(&c.statsDuplicate, 0)

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
			n := atomic_LoadInt64(&c.statsNew)
			d := atomic_LoadInt64(&c.statsDuplicate)
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

// proactiveDownload baixa a imagem full-res de uma mensagem e armazena no cache
// compartilhado. Executa em goroutine para não bloquear o dbWriter.
// O file_reference MTProto é válido apenas na janela de chegada da mensagem —
// este é o único momento confiável para download full-res.
func (c *Collector) proactiveDownload(ctx context.Context, msg *model.RawMessage) {
	req, err := telegram.ExtractPhotoRequest(msg.Payload)
	if err != nil {
		c.log.Debug("extração de foto falhou", "canal_id", msg.ChannelID, "msg_id", msg.MessageID, "erro", err)
		return
	}
	if req == nil {
		return // sem foto na mensagem
	}
	go func() {
		data, err := c.mediaClient.DownloadPhoto(ctx, *req)
		if err != nil {
			c.log.Debug("download proativo falhou",
				"photo_id", req.PhotoID, "erro", err)
			return
		}
		c.imageCache.Put(req.PhotoID, data)
		c.log.Debug("📷 download proativo OK",
			"photo_id", req.PhotoID, "bytes", len(data))
	}()
}
