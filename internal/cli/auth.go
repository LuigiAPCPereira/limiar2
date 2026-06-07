package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newAuthCmd builds the `auth` subcommand: interactive, idempotent
// authentication that persists the session. The interactive wizard for
// collecting missing credentials runs earlier in config.Load() when stdin
// is a TTY. Uses a text-format logger.
func newAuthCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Autenticar o userbot e persistir a sessão (interativo, execute uma vez)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg := p.Config()
			log := p.Logger(cfg.LogFormat)

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			client := p.NewClient(log, repo)

			if err := client.Auth(ctx); err != nil {
				return err
			}
			fmt.Println()
			fmt.Println("  ✅ Autenticação concluída! Sessão persistida.")
			fmt.Println()
			log.Debug("authentication complete; session persisted")
			return nil
		},
	}
}
