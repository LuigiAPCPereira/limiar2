package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/limiar/collector/internal/dashboard"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
	"github.com/limiar/collector/internal/processor"
	"github.com/limiar/collector/internal/storage"
)

func newRunCmd(p Provider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Executar o pipeline completo (collector + processor + dashboard)",
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
			collectorRepo, _ := storage.NewRepository(db.DB())
			defer func() { _ = collectorRepo.Close() }()
			procRepo, _ := storage.NewProcessorRepository(db.DB())
			defer func() { _ = procRepo.Close() }()
			client := p.NewTelegramClient(collectorRepo)
			col := p.NewCollector(client, collectorRepo)
			proc := processor.NewProcessor(procRepo, cfg.ProcessorConfig(), log.WithComponent("processor"))
			broker := dashboard.NewBroker()
			imageCache := p.NewImageCache()
			col.SetMediaDownload(p.NewMediaClient(collectorRepo), imageCache)
			mediaResolver := media.NewResolver(procRepo, imageCache, nil, log.WithComponent("media"))
			srv := dashboard.NewServer(collectorRepo, procRepo, mediaResolver, log.WithComponent("dashboard"), cfg.DashboardPort, broker)
			col.SetOnMessage(func(msg *model.RawMessage) {
				data, _ := json.Marshal(msg)
				broker.Publish(dashboard.Event{Type: "message", Data: data})
			})
			g, gctx := errgroup.WithContext(ctx)
			log.Info("🚀 Limiar orquestrador iniciado", "dashboard", fmt.Sprintf("http://localhost:%d", cfg.DashboardPort))
			presenter.Info("Limiar orquestrador iniciado")
			presenter.Step(fmt.Sprintf("Dashboard: http://localhost:%d", cfg.DashboardPort))
			presenter.Step("Pressione Ctrl+C para encerrar")
			g.Go(func() error { return col.Run(gctx) })
			g.Go(func() error { return proc.Run(gctx) })
			g.Go(func() error { return srv.ListenAndServe(gctx) })
			if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			log.Info("✅ Limiar encerrado gracefulmente")
			return nil
		},
	}
	return cmd
}
