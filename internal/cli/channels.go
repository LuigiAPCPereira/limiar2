package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// newChannelsCmd builds the `channels` subcommand group: list / add / remove.
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

// withAuthenticatedStore opens the store, verifies authentication, and invokes
// fn with the repo.
func withAuthenticatedStore(
	cmd *cobra.Command,
	p Provider,
	fn func(repo *storage.Repository, log logging) error,
) error {
	ctx := cmd.Context()
	cfg := p.Config()
	log := p.Logger(cfg.LogFormat)

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
		fmt.Fprintln(cmd.OutOrStdout(), "\n  ⚠️  Sessão não autenticada. Execute 'limiar-collector auth' primeiro.")
		return apperrors.Wrap("cli", "channels", apperrors.ErrNotAuthenticated)
	}
	return fn(repo, log)
}

// logging is the minimal logger surface the channels handlers use.
type logging interface {
	Info(msg string, args ...any)
}

func newChannelsListCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Listar canais monitorados",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withAuthenticatedStore(cmd, p, func(repo *storage.Repository, log logging) error {
				channels, err := repo.ListChannels(cmd.Context())
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				fmt.Fprintln(out)
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
					fmt.Fprintf(out, "  • @%-30s  id:%-12d  %s\n", ch.Username, ch.ID, status)
				}
				fmt.Fprintln(out, "  ──────────────────────────────────────")
				fmt.Fprintf(out, "  Total: %d canal(is)\n", len(channels))
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
			log := p.Logger(cfg.LogFormat)
			out := cmd.OutOrStdout()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			fmt.Fprintln(out, "\n  📡 Conectando ao Telegram...")

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
			fmt.Fprintln(out)
			fmt.Fprintln(out, "  ✅ Canal adicionado com sucesso!")
			fmt.Fprintf(out, "  • Username : @%s\n", peer.Username)
			fmt.Fprintf(out, "  • ID       : %d\n", peer.ID)
			fmt.Fprintln(out)
			log.Info("channel added", "id", peer.ID, "username", peer.Username)
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
			return withAuthenticatedStore(cmd, p, func(repo *storage.Repository, log logging) error {
				username := telegram.NormalizeUsername(args[0])
				if err := repo.RemoveChannel(cmd.Context(), username); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n  ✅ Canal @%s removido.\n\n", username)
				log.Info("channel removed", "username", username)
				return nil
			})
		},
	}
}

