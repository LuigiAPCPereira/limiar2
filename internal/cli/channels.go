package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// newChannelsCmd constrói o grupo de subcomandos `channels`: list / add / remove.
func newChannelsCmd(p Provider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channels",
		Short: "Gerenciar canais monitorados (requer autenticação)",
	}
	cmd.AddCommand(
		newChannelsListCmd(p),
		newChannelsAddCmd(p),
		newChannelsRemoveCmd(p),
	)
	return cmd
}

// withAuthenticatedStore abre o repositório, verifica a autenticação e invoca
// fn com o repo.
func withAuthenticatedStore(
	cmd *cobra.Command,
	p Provider,
	fn func(repo *storage.Repository, log logger.Logger, presenter logger.Presenter) error,
) error {
	ctx := cmd.Context()
	cfg := p.Config()

	format, _ := logger.ResolveFormat("channels", cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
	log := p.Logger(format)
	presenter := p.Presenter()

	repo, closeStore, err := p.OpenStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()

	client := p.NewClient(log, repo)
	authed, err := client.IsAuthenticated(ctx)
	if err != nil {
		return err
	}
	if !authed {
		presenter.Warning("Sessão não autenticada. Execute 'limiar-collector auth' primeiro.")
		return apperrors.Wrap("cli", "channels", apperrors.ErrNotAuthenticated)
	}
	return fn(repo, log, presenter)
}

func newChannelsListCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Listar canais monitorados",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withAuthenticatedStore(cmd, p, func(repo *storage.Repository, _ logger.Logger, presenter logger.Presenter) error {
				channels, err := repo.ListChannels(cmd.Context())
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				_, _ = fmt.Fprintln(out)
				fmt.Fprintln(out, "  📋 Canais monitorados")
				fmt.Fprintln(out, "  ──────────────────────────────────────")
				if len(channels) == 0 {
					fmt.Fprintln(out, "  Nenhum canal configurado.")
					fmt.Fprintln(out, "  Use 'channels add <username ou link>' para adicionar.")
					fmt.Fprintln(out)
					return nil
				}
				for _, ch := range channels {
					status := "✅ ativo"
					if !ch.Active {
						status = "⏸  pausado"
					}
					_, _ = fmt.Fprintf(out, "  • @%-30s  id:%-12d  %s\n", ch.Username, ch.ID, status)
				}
				fmt.Fprintln(out, "  ──────────────────────────────────────")
				_, _ = fmt.Fprintf(out, "  Total: %d canal(is)\n", len(channels))
				fmt.Fprintln(out)
				return nil
			})
		},
	}
}

func newChannelsAddCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "add <@username | https://t.me/username>",
		Short: "Resolver e adicionar um canal pelo username ou link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg := p.Config()

			format, _ := logger.ResolveFormat("channels", cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
			log := p.Logger(format)
			presenter := p.Presenter()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			presenter.Step("Conectando ao Telegram...")

			client := p.NewClient(log, repo)
			peer, err := client.ResolveChannelChecked(ctx, args[0])
			if err != nil {
				return err
			}
			ch := &storage.Channel{
				ID:       peer.ID,
				Username: peer.Username,
				Title:    peer.Username,
				Active:   true,
			}
			if err := repo.AddChannel(ctx, ch); err != nil {
				return err
			}
			if err := repo.SavePeer(ctx, peer); err != nil {
				return err
			}
			presenter.Success(fmt.Sprintf("Canal @%s adicionado (id: %d)", peer.Username, peer.ID))
			return nil
		},
	}
}

func newChannelsRemoveCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <@username | https://t.me/username>",
		Short: "Remover um canal monitorado pelo username ou link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withAuthenticatedStore(cmd, p, func(repo *storage.Repository, _ logger.Logger, presenter logger.Presenter) error {
				username := telegram.NormalizeUsername(args[0])
				if err := repo.RemoveChannel(cmd.Context(), username); err != nil {
					return err
				}
				presenter.Success(fmt.Sprintf("Canal @%s removido.", username))
				return nil
			})
		},
	}
}
