package cli

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/dashboard"
)

// newDashboardCmd builds the `dashboard` subcommand: starts an HTTP server for
// inspecting captured messages in a browser.
func newDashboardCmd(p Provider) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Iniciar dashboard web para visualizar mensagens capturadas",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			log := p.Logger("text")
			srv := dashboard.NewServer(repo, log, port)

			fmt.Fprintf(cmd.OutOrStdout(), "\n  🌐 Dashboard: http://localhost:%d\n  Pressione Ctrl+C para encerrar\n\n", port)
			return srv.ListenAndServe(ctx)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "Porta do servidor HTTP")
	return cmd
}
