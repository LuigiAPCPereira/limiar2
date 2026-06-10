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

// newRunCmd constrói o subcomando `run`: o serviço coletor não interativo.
// Ele utiliza um logger no formato JSON, requer uma sessão autenticada e é
// encerrado de forma graciosa (graceful shutdown) com SIGTERM/SIGINT dentro do tempo limite configurado.
func newRunCmd(p Provider) *cobra.Command {
	var withDashboard bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Executar o serviço coletor (não interativo, pronto para produção)",
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
