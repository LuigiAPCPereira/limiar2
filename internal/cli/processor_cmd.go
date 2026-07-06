// Package cli — subcomandos `processor run` e `processor reprocess`.
package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/dashboard"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/processor"
	"github.com/limiar/collector/internal/storage"
)

// newProcessorCmd constrói o grupo de subcomandos `processor`.
func newProcessorCmd(p Provider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "processor",
		Short: "Comandos do processador de mensagens",
	}
	cmd.AddCommand(
		newProcessorRunCmd(p),
		newReprocessCmd(p),
	)
	return cmd
}

// newProcessorRunCmd constrói `processor run`: daemon do processor isolado com
// dashboard embutido. Reproduz o comportamento do antigo `limiar-processor --dashboard`.
func newProcessorRunCmd(p Provider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Executar o processador isolado (com dashboard embutido)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := p.Config()
			log := p.Logger()
			presenter := p.Presenter()

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			db, err := storage.Open(ctx, cfg.DBPath, log.WithComponent("storage"))
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			procRepo, err := storage.NewProcessorRepository(db.DB())
			if err != nil {
				return fmt.Errorf("limiar: criar processor repository: %w", err)
			}
			defer func() { _ = procRepo.Close() }()

			proc := processor.NewProcessor(procRepo, cfg.ProcessorConfig(), log.WithComponent("processor"))

			// Dashboard embutido
			collectorRepo, err := storage.NewRepository(db.DB())
			if err != nil {
				return fmt.Errorf("limiar: criar dashboard repository: %w", err)
			}
			defer func() { _ = collectorRepo.Close() }()

			imageCache := media.NewCache(500, 30*time.Minute)
			mediaResolver := media.NewResolver(procRepo, imageCache, nil, log.WithComponent("media"))
			srv := dashboard.NewServer(collectorRepo, procRepo, mediaResolver, log.WithComponent("dashboard"), cfg.DashboardPort, nil)

			go func() { _ = srv.ListenAndServe(ctx) }()
			presenter.Info(fmt.Sprintf("Dashboard: http://localhost:%d", cfg.DashboardPort))

			runErr := make(chan error, 1)
			go func() { runErr <- proc.Run(ctx) }()

			select {
			case err := <-runErr:
				return err
			case <-ctx.Done():
				log.Info("🛑 Sinal de desligamento recebido")
				select {
				case err := <-runErr:
					log.Info("✅ Processor encerrado gracefulmente")
					return err
				case <-time.After(time.Duration(cfg.ShutdownTimeout) * time.Second):
					log.Error("⏰ Timeout de desligamento excedido")
					return fmt.Errorf("cli: processor_run: timeout de desligamento excedido")
				}
			}
		},
	}
	return cmd
}

// newReprocessCmd constrói `processor reprocess`: reprocessa mensagens raw existentes.
func newReprocessCmd(p Provider) *cobra.Command {
	var (
		reprocessAll     bool
		msgID            int64
		channel          string
		resolveURLs      bool
		resolveURLsLimit int
	)

	cmd := &cobra.Command{
		Use:   "reprocess",
		Short: "Reprocessar mensagens raw existentes",
		Long: `Reprocessa mensagens de raw_messages: executa normalização, classificação
e atualiza processed_messages. Útil após correções no classificador/normalizador.

Modos:
  --all               Reprocessa todas as mensagens raw do DB
  --id <int64>        Reprocessa mensagem específica por raw_message ID
  --channel <username> Reprocessa todas as msgs de um canal`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !reprocessAll && msgID == 0 && channel == "" {
				return fmt.Errorf("especifique --all, --id ou --channel")
			}

			log := p.Logger()
			presenter := p.Presenter()

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			cfg := p.Config()
			db, err := storage.Open(ctx, cfg.DBPath, log.WithComponent("storage"))
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			procRepo, err := storage.NewProcessorRepository(db.DB())
			if err != nil {
				return fmt.Errorf("limiar: criar processor repository: %w", err)
			}
			defer func() { _ = procRepo.Close() }()

			collectorRepo, err := storage.NewRepository(db.DB())
			if err != nil {
				return fmt.Errorf("limiar: criar collector repository: %w", err)
			}
			defer func() { _ = collectorRepo.Close() }()

			presenter.Info("🔄 Iniciando reprocessamento...")

			start := time.Now()
			var processed, failed int

			options := reprocessOptions{resolveURLs: resolveURLs, resolveURLsLimit: resolveURLsLimit}
			if options.resolveURLsLimit < 0 {
				return fmt.Errorf("resolve-urls-limit deve ser >= 0")
			}

			switch {
			case msgID > 0:
				processed, failed, err = reprocessSingle(ctx, collectorRepo, procRepo, msgID, presenter, options)
			case channel != "":
				processed, failed, err = reprocessChannel(ctx, collectorRepo, procRepo, channel, presenter, options)
			case reprocessAll:
				processed, failed, err = reprocessAllMsgs(ctx, collectorRepo, procRepo, presenter, options)
			}

			if err != nil {
				return err
			}

			elapsed := time.Since(start)
			presenter.Success(fmt.Sprintf(
				"Reprocessamento concluído: %d processadas, %d falharam, %s",
				processed, failed, elapsed.Round(time.Millisecond)))
			log.Info("🔄 Reprocessamento concluído",
				"processadas", processed, "falharam", failed, "duração", elapsed)
			return nil
		},
	}

	cmd.Flags().BoolVar(&reprocessAll, "all", false, "Reprocessar todas as mensagens raw")
	cmd.Flags().Int64Var(&msgID, "id", 0, "Reprocessar mensagem específica por raw_message ID")
	cmd.Flags().StringVar(&channel, "channel", "", "Reprocessar todas as msgs de um canal (username)")
	cmd.Flags().BoolVar(&resolveURLs, "resolve-urls", false, "Resolver URLs durante o reprocessamento (rede; desativado por padrão)")
	cmd.Flags().IntVar(&resolveURLsLimit, "resolve-urls-limit", 0, "Limite de URLs resolvidas nesta execução (0 = sem limite quando --resolve-urls)")
	return cmd
}

type reprocessOptions struct {
	resolveURLs      bool
	resolveURLsLimit int
}

// reprocessSingle reprocessa uma única mensagem pelo ID.
func reprocessSingle(
	ctx context.Context,
	collectorRepo *storage.Repository,
	procRepo *storage.ProcessorRepository,
	rawID int64,
	presenter interface{ Step(string) },
	options reprocessOptions,
) (processed, failed int, err error) {
	msg, err := collectorRepo.GetMessageByID(ctx, rawID)
	if err != nil {
		return 0, 0, fmt.Errorf("mensagem %d não encontrada: %w", rawID, err)
	}

	nm, err := processor.Normalize(msg)
	if err != nil {
		presenter.Step(fmt.Sprintf("❌ Falha ao normalizar msg %d: %v", rawID, err))
		return 0, 1, nil
	}
	nm.MessageType = string(processor.Classify(nm))
	nm.IsPromotional = processor.IsPromotionalMessageType(nm.MessageType)
	if options.resolveURLs {
		resolver := processor.NewURLResolver(procRepo, nil, nil)
		if options.resolveURLsLimit == 0 || options.resolveURLsLimit > 0 {
			processor.EnrichURLResolution(ctx, nm, resolver, nil)
		}
	}

	if err := procRepo.SaveProcessed(ctx, nm); err != nil {
		presenter.Step(fmt.Sprintf("❌ Falha ao salvar msg %d: %v", rawID, err))
		return 0, 1, nil
	}

	presenter.Step(fmt.Sprintf("✅ Msg %d reprocessada → %s", rawID, nm.MessageType))
	return 1, 0, nil
}

// reprocessChannel reprocessa todas as mensagens de um canal.
func reprocessChannel(
	ctx context.Context,
	collectorRepo *storage.Repository,
	procRepo *storage.ProcessorRepository,
	username string,
	presenter interface{ Step(string) },
	options reprocessOptions,
) (processed, failed int, err error) {
	channels, err := collectorRepo.ListChannels(ctx)
	if err != nil {
		return 0, 0, err
	}

	var channelID int64
	for _, ch := range channels {
		if ch.Username == username {
			channelID = ch.ID
			break
		}
	}
	if channelID == 0 {
		return 0, 0, fmt.Errorf("canal @%s não encontrado", username)
	}

	return reprocessByQuery(ctx, collectorRepo, procRepo, channelID, presenter, options)
}

// reprocessAllMsgs reprocessa todas as mensagens raw.
func reprocessAllMsgs(
	ctx context.Context,
	collectorRepo *storage.Repository,
	procRepo *storage.ProcessorRepository,
	presenter interface{ Step(string) },
	options reprocessOptions,
) (processed, failed int, err error) {
	return reprocessByQuery(ctx, collectorRepo, procRepo, 0, presenter, options)
}

const reprocessBatchSize = 100

// reprocessByQuery faz o reprocessamento paginado. channelID=0 → todas.
func reprocessByQuery(
	ctx context.Context,
	collectorRepo *storage.Repository,
	procRepo *storage.ProcessorRepository,
	channelID int64,
	presenter interface{ Step(string) },
	options reprocessOptions,
) (totalProcessed, totalFailed int, err error) {
	// Contar total para progresso.
	total, err := countForReprocess(ctx, collectorRepo, channelID)
	if err != nil {
		return 0, 0, err
	}

	var resolver *processor.URLResolver
	if options.resolveURLs {
		resolver = processor.NewURLResolver(procRepo, nil, nil)
	}
	resolvedURLs := 0
	offset := 0
	for {
		if ctx.Err() != nil {
			return totalProcessed, totalFailed, nil
		}

		msgs, err := collectorRepo.ListMessages(ctx, channelID, reprocessBatchSize, offset)
		if err != nil {
			return totalProcessed, totalFailed, err
		}
		if len(msgs) == 0 {
			break
		}

		var normalized []*processor.NormalizedMessage
		for _, raw := range msgs {
			nm, err := processor.Normalize(raw)
			if err != nil {
				totalFailed++
				continue
			}
			nm.MessageType = string(processor.Classify(nm))
			nm.IsPromotional = processor.IsPromotionalMessageType(nm.MessageType)
			if resolver != nil && (options.resolveURLsLimit == 0 || resolvedURLs < options.resolveURLsLimit) {
				if processor.EnrichURLResolution(ctx, nm, resolver, nil) {
					resolvedURLs++
				}
			}
			normalized = append(normalized, nm)
		}

		saved, saveFailed, err := procRepo.SaveProcessedBatch(ctx, normalized)
		if err != nil {
			return totalProcessed, totalFailed, err
		}
		totalProcessed += saved
		totalFailed += saveFailed

		if total > 0 {
			pct := 100 * float64(totalProcessed+totalFailed) / float64(total)
			presenter.Step(fmt.Sprintf("Reprocessando... %d/%d (%.1f%%)",
				totalProcessed+totalFailed, total, pct))
		}

		offset += len(msgs)
		if len(msgs) < reprocessBatchSize {
			break
		}
	}
	return totalProcessed, totalFailed, nil
}

// countForReprocess conta mensagens para progress bar.
func countForReprocess(ctx context.Context, repo *storage.Repository, channelID int64) (int64, error) {
	if channelID == 0 {
		return repo.CountRawMessages(ctx)
	}
	stats, err := repo.CountMessagesByChannel(ctx)
	if err != nil {
		return 0, err
	}
	for _, s := range stats {
		if s.ChannelID == channelID {
			return s.MessageCount, nil
		}
	}
	return 0, nil
}
