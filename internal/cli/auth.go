package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/logger"
)

// newAuthCmd constrói o subcomando `auth`: autenticação interativa e idempotente
// que persiste a sessão. O assistente interativo para coletar
// credenciais ausentes é executado antecipadamente em config.Load() quando o stdin
// é um TTY. Utiliza um logger no formato texto.
func newAuthCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Autenticar o userbot e persistir a sessão (interativo, execute uma vez)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg := p.Config()

			format, _ := logger.ResolveFormat("auth", cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
			log := p.Logger(format)
			presenter := p.Presenter()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			client := p.NewClient(log, repo)

			if err := client.Auth(ctx); err != nil {
				return err
			}
			presenter.Success("Autenticação concluída! Sessão persistida.")
			return nil
		},
	}
}
