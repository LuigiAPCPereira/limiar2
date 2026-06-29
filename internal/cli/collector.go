// Package cli — subcomando `collector` com `collector run` para operação isolada.
package cli

import (
	"encoding/json"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/dashboard"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
)

// newCollectorCmd constrói o grupo de subcomandos `collector`.
func newCollectorCmd(p Provider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collector",
		Short: "Comandos do coletor de mensagens do Telegram",
	}
	cmd.AddCommand(newCollectorRunCmd(p))
	return cmd
}

// newCollectorRunCmd constrói `collector run`: daemon do coletor isolado com
// dashboard embutido. Reproduz o comportamento do antigo `limiar-collector run --dashboard`.
func newCollectorRunCmd(p Provider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Executar o coletor isolado (com dashboard embutido)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := p.Config()
			log := p.Logger()
			presenter := p.Presenter()

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			client := p.NewTelegramClient(repo)
			col := p.NewCollector(client, repo)

			// Dashboard embutido
			broker := dashboard.NewBroker()
			port := cfg.DashboardPort
			imageCache := media.NewCache(500, 30*time.Minute)
			mediaResolver := media.NewResolver(repo, imageCache, log)
			srv := dashboard.NewServer(repo, nil, mediaResolver, log, port, broker)
			col.SetOnMessage(func(msg *model.RawMessage) {
				data, err := json.Marshal(msg)
				if err != nil {
					return
				}
				broker.Publish(dashboard.Event{Type: "message", Data: data})
			})

			go func() { _ = srv.ListenAndServe(ctx) }()
			presenter.Info(fmt.Sprintf("Dashboard: http://localhost:%d", port))

			runErr := make(chan error, 1)
			go func() { runErr <- col.Run(ctx) }()

			select {
			case err := <-runErr:
				return err
			case <-ctx.Done():
				log.Info("🛑 Sinal de desligamento recebido; drenando buffers",
					"timeout_segundos", cfg.ShutdownTimeout)
				select {
				case err := <-runErr:
					log.Info("✅ Desligamento graceful completo")
					return err
				case <-time.After(time.Duration(cfg.ShutdownTimeout) * time.Second):
					log.Error("⏰ Timeout de desligamento excedido; forçando saída",
						"timeout_segundos", cfg.ShutdownTimeout)
					return fmt.Errorf("cli: collector_run: timeout de desligamento excedido")
				}
			}
		},
	}
	return cmd
}
