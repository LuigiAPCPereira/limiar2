package cli

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	apperrors "github.com/limiar/collector/internal/errors"
)

// newRunCmd builds the `run` subcommand: the non-interactive collector service.
// It uses a JSON-format logger, requires an authenticated session, and shuts
// down gracefully on SIGTERM/SIGINT within the configured timeout.
func newRunCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the collector service (non-interactive, production-ready)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := p.Config()
			log := p.Logger(cfg.LogFormat)

			// Signal-aware root context: SIGTERM/SIGINT cancels it.
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			client := p.NewClient(log, repo)
			col := p.NewCollector(client, repo, log)

			// Run in a goroutine so we can enforce the shutdown timeout once the
			// context is cancelled by a signal.
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
}
