package processor

import (
	"context"
	"time"

	"github.com/limiar/collector/internal/logger"
)

// Processor é o orquestrador principal do limiar-processor. Ele faz poll
// periódico de raw_messages não processadas, normaliza, classifica e
// persiste em processed_messages.
type Processor struct {
	repo *Repository
	cfg  *Config
	log  logger.Logger
}

// NewProcessor constrói um Processor com as dependências injetadas.
func NewProcessor(repo *Repository, cfg *Config, log logger.Logger) *Processor {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Processor{repo: repo, cfg: cfg, log: log}
}

// Run inicia o loop de processamento. Faz poll a cada PollInterval até
// ctx ser cancelado (SIGTERM/SIGINT). Usa drain mode: se o batch encheu,
// processa imediatamente sem esperar o ticker (drena backlog mais rápido).
func (p *Processor) Run(ctx context.Context) error {
	p.log.Info("🔄 Processor iniciado",
		"poll_interval", p.cfg.PollInterval,
		"batch_size", p.cfg.BatchSize)

	for {
		full := p.processBatch(ctx)

		if full {
			// Batch encheu — há mais mensagens para processar.
			// Continua imediatamente (drain mode).
			continue
		}

		select {
		case <-ctx.Done():
			p.log.Info("🛑 Processor parando (contexto cancelado)")
			return nil
		case <-time.After(p.cfg.PollInterval):
		}
	}
}

// processBatch busca um lote de mensagens não processadas, normaliza cada uma,
// classifica e persiste em transação única. Retorna true se o batch encheu
// (indicando que há mais mensagens para processar — drain mode).
func (p *Processor) processBatch(ctx context.Context) (batchFull bool) {
	start := time.Now()

	msgs, err := p.repo.FetchUnprocessed(ctx, p.cfg.BatchSize)
	if err != nil {
		p.log.Error("❌ Erro ao buscar mensagens não processadas", "erro", err)
		return false
	}

	if len(msgs) == 0 {
		return false
	}

	batchFull = len(msgs) >= p.cfg.BatchSize

	// Normaliza e classifica todas primeiro (CPU-bound, sem I/O).
	var normalized []*NormalizedMessage
	var normFailed int
	for _, raw := range msgs {
		msgLog := p.log.With("canal_id", raw.ChannelID, "msg_id", raw.MessageID)
		nm, err := Normalize(raw)
		if err != nil {
			msgLog.Warn("⚠️ Normalização falhou", "erro", err)
			normFailed++
			continue
		}
		nm.MessageType = string(Classify(nm))
		nm.FeedEligible = nm.MessageType == string(TypeDealComplete) || nm.MessageType == string(TypeDealNoCoupon)
		normalized = append(normalized, nm)
	}

	// Dedup cross-channel: marca mensagens com mesma URL em canais diferentes.
	p.markDuplicates(ctx, normalized)

	// Persiste todas em transação única (10-50x mais rápido que INSERTs individuais).
	saved, saveFailed, saveErr := p.repo.SaveProcessedBatch(ctx, normalized)
	if saveErr != nil {
		p.log.Error("❌ Erro ao salvar batch processado", "erro", saveErr)
	}

	backlog, backlogErr := p.repo.CountUnprocessed(ctx)
	if backlogErr != nil {
		p.log.Warn("⚠️ Erro ao contar backlog", "erro", backlogErr)
	}

	p.log.Info("⚙️ Batch processado",
		"processadas", saved,
		"falharam", normFailed+saveFailed,
		"duração_ms", time.Since(start).Milliseconds(),
		"backlog", backlog)

	return batchFull
}

// markDuplicates detecta mensagens com mesma URL em canais diferentes.
// Apenas duplicatas cross-channel são marcadas (mesmo canal = normal).
func (p *Processor) markDuplicates(ctx context.Context, msgs []*NormalizedMessage) {
	// Intra-batch: detecta se dois canais diferentes no mesmo batch têm a mesma URL
	seen := make(map[string]int64, len(msgs))
	for _, nm := range msgs {
		if nm.URLHash == "" {
			continue
		}
		if prevChannel, ok := seen[nm.URLHash]; ok {
			if prevChannel != nm.ChannelID {
				nm.IsDuplicate = true
			}
		} else {
			seen[nm.URLHash] = nm.ChannelID
		}
	}

	// Cross-batch: checa cada (hash, channelID) contra o banco
	if len(seen) == 0 {
		return
	}
	crossDups, err := p.repo.CrossChannelDuplicates(ctx, seen)
	if err != nil {
		p.log.Warn("⚠️ Erro ao verificar duplicatas cross-channel", "erro", err)
		return
	}
	if len(crossDups) == 0 {
		return
	}
	for _, nm := range msgs {
		if nm.URLHash != "" && crossDups[nm.URLHash] {
			nm.IsDuplicate = true
		}
	}
}
