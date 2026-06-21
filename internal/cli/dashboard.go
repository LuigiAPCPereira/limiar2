package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/dashboard"
	"github.com/limiar/collector/internal/logger"
)

// newDashboardCmd constrói o subcomando `dashboard`: inicia um servidor HTTP para
// inspecionar as mensagens capturadas em um navegador.
func newDashboardCmd(p Provider) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Iniciar dashboard web para visualizar mensagens capturadas",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			cfg := p.Config()
			if port == 0 {
				port = cfg.DashboardPort
			}
			format := logger.ResolveFormat(cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
			log := p.Logger(format)
			presenter := p.Presenter()

			repo, closeStore, err := p.OpenStore(ctx, log)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			srv := dashboard.NewServer(repo, nil, log, port, nil)
			presenter.Info(fmt.Sprintf("Dashboard: http://localhost:%d", port))
			presenter.Step("Pressione Ctrl+C para encerrar")
			return srv.ListenAndServe(ctx)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "Porta do servidor HTTP (0 = usa LIMIAR_DASHBOARD_PORT ou 8080)")
	return cmd
}
