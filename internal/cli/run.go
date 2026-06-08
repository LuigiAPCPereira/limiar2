package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/dashboard"
	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/storage"
)

// newRunCmd builds the `run` subcommand: the non-interactive collector service.
// It uses a JSON-format logger, requires an authenticated session, and shuts
// down gracefully on SIGTERM/SIGINT within the configured timeout.
func newRunCmd(p Provider) *cobra.Command {
	var withDashboard bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the collector service (non-interactive, production-ready)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := p.Config()
			log := p.Logger(cfg.LogFormat)

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			client := p.NewClient(log, repo)
			col := p.NewCollector(client, repo, log)

			if withDashboard {
				broker := dashboard.NewBroker()
				srv := dashboard.NewServer(repo, log, 8080, broker)

				col.SetOnMessage(func(msg *storage.RawMessage) {
					data, err := json.Marshal(msg)
					if err != nil {
						return
					}
					broker.Publish(dashboard.Event{Type: "message", Data: data})
				})

				go func() { _ = srv.ListenAndServe(ctx) }()
				fmt.Fprintf(cmd.OutOrStdout(), "\n  🌐 Dashboard: http://localhost:8080\n\n")
			}

			runErr := make(chan error, 1)
			go func() { runErr <- col.Run(ctx) }()

			select {
			case err := <-runErr:
				return err
			case <-ctx.Done():
				log.Info("🛑 Sinal de desligamento recebido; drenando buffers",
					"timeout_segundos", cfg.ShutdownTimeout)
				timeout := time.Duration(cfg.ShutdownTimeout) * time.Second
				select {
				case err := <-runErr:
					log.Info("✅ Desligamento graceful completo")
					return err
				case <-time.After(timeout):
					log.Error("⏰ Timeout de desligamento excedido; forçando saída",
						"timeout_segundos", cfg.ShutdownTimeout)
					return apperrors.Wrap("cli", "run", context.DeadlineExceeded)
				}
			}
		},
	}

	cmd.Flags().BoolVar(&withDashboard, "dashboard", false, "Iniciar dashboard web em http://localhost:8080")
	return cmd
}
