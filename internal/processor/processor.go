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
// ctx ser cancelado (SIGTERM/SIGINT).
func (p *Processor) Run(ctx context.Context) error {
	p.log.Info("🔄 Processor iniciado",
		"poll_interval", p.cfg.PollInterval,
		"batch_size", p.cfg.BatchSize)

	// Processa backlog existente imediatamente no startup.
	p.processBatch(ctx)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("🛑 Processor parando (contexto cancelado)")
			return nil
		case <-ticker.C:
			p.processBatch(ctx)
		}
	}
}

// processBatch busca um lote de mensagens não processadas, normaliza cada uma,
// classifica e persiste. Erros em mensagens individuais são logados e não
// interrompem o batch.
func (p *Processor) processBatch(ctx context.Context) {
	start := time.Now()

	msgs, err := p.repo.FetchUnprocessed(ctx, p.cfg.BatchSize)
	if err != nil {
		p.log.Error("❌ Erro ao buscar mensagens não processadas", "erro", err)
		return
	}

	if len(msgs) == 0 {
		return
	}

	var processed, failed int
	for _, raw := range msgs {
		nm, err := Normalize(raw)
		if err != nil {
			p.log.Warn("⚠️ Normalização falhou",
				"raw_id", raw.ID,
				"msg_id", raw.MessageID,
				"erro", err)
			failed++
			continue
		}

		nm.MessageType = string(Classify(nm))

		if err := p.repo.SaveProcessed(ctx, nm); err != nil {
			p.log.Error("❌ Persistência falhou",
				"msg_id", nm.MessageID,
				"erro", err)
			failed++
			continue
		}
		processed++
	}

	backlog, _ := p.repo.CountUnprocessed(ctx)

	p.log.Info("⚙️ Batch processado",
		"processadas", processed,
		"falharam", failed,
		"duração_ms", time.Since(start).Milliseconds(),
		"backlog", backlog)
}
